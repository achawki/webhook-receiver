package handler

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/achawki/webhook-receiver/internal/storage"
)

const maxRequestBodyBytes = 1 << 20
const defaultRateLimitRequests = 300
const defaultRateLimitWindow = 5 * time.Minute

//go:embed templates/*.gohtml
var templateFS embed.FS

//go:embed assets/*
var assetFS embed.FS

// Handler handles incoming HTTP requests.
type Handler struct {
	storage        storage.WebhookStorage
	templates      *template.Template
	assets         http.Handler
	limiter        *ipRateLimiter
	publicBaseURL  string
	clientIPHeader string
}

// Option configures a handler.
type Option func(*Handler)

// WithRateLimit overrides the default per-IP request limit.
func WithRateLimit(limit int, window time.Duration) Option {
	return func(h *Handler) {
		h.limiter = newIPRateLimiter(limit, window)
	}
}

// WithPublicBaseURL sets the absolute base URL returned in webhook capability URLs.
func WithPublicBaseURL(baseURL string) Option {
	return func(h *Handler) {
		h.publicBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
}

// WithClientIPHeader configures an optional request header that contains the client IP.
func WithClientIPHeader(headerName string) Option {
	return func(h *Handler) {
		h.clientIPHeader = strings.TrimSpace(headerName)
	}
}

// NewHandler creates and initializes handler.
func NewHandler(storage storage.WebhookStorage, options ...Option) *Handler {
	templates := template.Must(template.ParseFS(templateFS, "templates/*.gohtml"))
	assetsSubFS, err := fs.Sub(assetFS, "assets")
	if err != nil {
		panic(err)
	}
	handler := &Handler{
		storage:   storage,
		templates: templates,
		assets:    http.FileServer(http.FS(assetsSubFS)),
		limiter:   newIPRateLimiter(defaultRateLimitRequests, defaultRateLimitWindow),
	}
	for _, option := range options {
		option(handler)
	}

	return handler
}

// Register attaches all API, ingest, and UI routes to the provided mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("/favicon.svg", h.assets)
	mux.Handle("/favicon.ico", h.assets)
	mux.Handle("/apple-touch-icon.png", h.assets)
	mux.HandleFunc("/", h.HomeHandler)
	mux.HandleFunc("/webhooks", h.WebhooksPageHandler)
	mux.HandleFunc("/webhooks/", h.WebhookPageHandler)
	mux.HandleFunc("/hooks/", h.HookHandler)
	mux.HandleFunc("/api/webhooks", h.WebhookHandler)
	mux.HandleFunc("/api/webhooks/", h.MessageHandler)
}

// UnknownHandler handles requests for unknown endpoints and returns 404
func (h *Handler) UnknownHandler(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func (h *Handler) unknownWebhookHandler(w http.ResponseWriter, webhookID string) {
	h.writeJSON(w, http.StatusNotFound, map[string]string{
		"message": fmt.Sprintf("Webhook with ID: %s does not exist", webhookID),
	})
}

func (h *Handler) badRequestHandler(w http.ResponseWriter, message string) {
	h.writeJSON(w, http.StatusBadRequest, map[string]string{"message": message})
}

func (h *Handler) validationErrorHandler(w http.ResponseWriter, message string) {
	h.writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": message})
}

func (h *Handler) internalServerErrorHandler(w http.ResponseWriter, message string) {
	h.writeJSON(w, http.StatusInternalServerError, map[string]string{"message": message})
}

func (h *Handler) unauthorizedHandler(w http.ResponseWriter) {
	h.writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Request did not satisfy the configured webhook authorization"})
}

func (h *Handler) tooManyRequestsHandler(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/hooks/") {
		h.writeJSON(w, http.StatusTooManyRequests, map[string]string{"message": "Too many requests from this IP. Please retry later."})
		return
	}

	http.Error(w, "Too many requests from this IP. Please retry later.", http.StatusTooManyRequests)
}

func (h *Handler) allowRequest(w http.ResponseWriter, r *http.Request) bool {
	if h.limiter == nil {
		return true
	}

	clientIP := h.clientIP(r)
	if allowed, _ := h.limiter.Allow(clientIP); allowed {
		return true
	}

	h.tooManyRequestsHandler(w, r)
	return false
}

func (h *Handler) writeJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "could not write response", http.StatusInternalServerError)
	}
}

func cleanPathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "/")
}

func (h *Handler) requestBaseURL(r *http.Request) string {
	if h.publicBaseURL != "" {
		return h.publicBaseURL
	}

	if !safeLocalRequestBaseURL(r) {
		return ""
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

func capabilityURL(baseURL string, path string) string {
	if baseURL == "" {
		return path
	}

	return baseURL + path
}

type ipRateLimiter struct {
	mu          sync.Mutex
	limit       int
	window      time.Duration
	entries     map[string]*rateLimitEntry
	lastCleanup time.Time
}

type rateLimitEntry struct {
	count       int
	windowStart time.Time
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	if limit <= 0 {
		limit = defaultRateLimitRequests
	}
	if window <= 0 {
		window = defaultRateLimitWindow
	}

	return &ipRateLimiter{
		limit:       limit,
		window:      window,
		entries:     map[string]*rateLimitEntry{},
		lastCleanup: time.Now().UTC(),
	}
}

func (l *ipRateLimiter) Allow(ip string) (bool, int) {
	now := time.Now().UTC()

	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastCleanup) >= l.window {
		for existingIP, entry := range l.entries {
			if now.Sub(entry.windowStart) >= l.window {
				delete(l.entries, existingIP)
			}
		}
		l.lastCleanup = now
	}

	entry, ok := l.entries[ip]
	if !ok || now.Sub(entry.windowStart) >= l.window {
		l.entries[ip] = &rateLimitEntry{
			count:       1,
			windowStart: now,
		}
		return true, 1
	}

	if entry.count >= l.limit {
		return false, entry.count
	}

	entry.count++
	return true, entry.count
}

func (h *Handler) clientIP(r *http.Request) string {
	if h.clientIPHeader != "" {
		headerValue := strings.TrimSpace(r.Header.Get(h.clientIPHeader))
		if parsedIP := net.ParseIP(headerValue); parsedIP != nil {
			return parsedIP.String()
		}
	}

	_, remoteValue := remoteRequestIP(r)
	if remoteValue != "" {
		return remoteValue
	}

	return "unknown"
}

func remoteRequestIP(r *http.Request) (net.IP, string) {
	trimmedRemoteAddr := strings.TrimSpace(r.RemoteAddr)
	if trimmedRemoteAddr == "" {
		return nil, ""
	}

	if host, _, err := net.SplitHostPort(trimmedRemoteAddr); err == nil && host != "" {
		if parsedIP := net.ParseIP(host); parsedIP != nil {
			return parsedIP, parsedIP.String()
		}
		return nil, host
	}

	if parsedIP := net.ParseIP(trimmedRemoteAddr); parsedIP != nil {
		return parsedIP, parsedIP.String()
	}

	return nil, trimmedRemoteAddr
}

func safeLocalRequestBaseURL(r *http.Request) bool {
	host := strings.ToLower(hostWithoutPort(r.Host))
	if host == "" {
		return false
	}

	if host != "localhost" {
		if parsedHostIP := net.ParseIP(host); parsedHostIP == nil || !parsedHostIP.IsLoopback() {
			return false
		}
	}

	remoteIP, remoteValue := remoteRequestIP(r)
	if remoteValue == "" {
		return false
	}

	return remoteIP != nil && remoteIP.IsLoopback()
}

func hostWithoutPort(hostport string) string {
	trimmed := strings.TrimSpace(hostport)
	if trimmed == "" {
		return ""
	}

	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		return host
	}

	return strings.Trim(trimmed, "[]")
}
