package handler

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/achawki/webhook-receiver/internal/model"
	"github.com/achawki/webhook-receiver/internal/storage"
)

type homePageData struct {
	PageTitle string
	Error     string
}

type webhookPageData struct {
	PageTitle  string
	Webhook    webhookCardView
	Requests   []requestView
	Outcome    outcomeFilterView
	Pagination paginationView
}

type webhookCardView struct {
	ID              string
	AuthModes       []string
	DetailPath      string
	PublicIngestURL string
	MessagesURL     string
	ExpiresAt       string
}

type requestView struct {
	Method       string
	Path         string
	Query        string
	Payload      string
	Time         string
	Headers      []headerView
	StatusCode   int
	StatusText   string
	Rejected     bool
	ErrorMessage string
}

type paginationView struct {
	CurrentPage     int
	PageSize        int
	TotalMessages   int
	TotalPages      int
	Outcome         string
	HasNextPage     bool
	HasPreviousPage bool
	NextPageURL     string
	PreviousPageURL string
}

type outcomeFilterView struct {
	Current     string
	AllURL      string
	AcceptedURL string
	RejectedURL string
}

type headerView struct {
	Name   string
	Values string
}

// HomeHandler renders the landing page.
func (h *Handler) HomeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" || r.Method != http.MethodGet {
		h.UnknownHandler(w, r)
		return
	}

	if !h.allowRequest(w, r) {
		return
	}

	h.renderHomePage(w, r, "", http.StatusOK)
}

// WebhooksPageHandler handles the HTML form for creating webhooks.
func (h *Handler) WebhooksPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/webhooks" {
		h.UnknownHandler(w, r)
		return
	}

	if !h.allowRequest(w, r) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		http.Redirect(w, r, "/", http.StatusSeeOther)
	case http.MethodPost:
		h.webhookFormPOSTHandler(w, r)
	default:
		h.UnknownHandler(w, r)
	}
}

// WebhookPageHandler renders a detail page for a single webhook.
func (h *Handler) WebhookPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.UnknownHandler(w, r)
		return
	}

	webhookID := h.retrieveWebhookIDFromDetailPath(r.URL.Path)
	if webhookID == "" {
		h.UnknownHandler(w, r)
		return
	}

	if !h.allowRequest(w, r) {
		return
	}

	webhook, err := h.storage.GetWebhook(webhookID)
	if err != nil {
		log.Printf("Could not retrieve webhook for detail page: %s", err)
		switch err.(type) {
		case *storage.WebhookNotFoundError:
			h.renderHomePage(w, r, fmt.Sprintf("Webhook with ID: %s does not exist", webhookID), http.StatusNotFound)
		default:
			http.Error(w, "Could not retrieve webhook", http.StatusInternalServerError)
		}
		return
	}

	page, pageSize, outcome, err := messagePageFromQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	messagePage, err := h.storage.GetMessagePageForWebhook(webhookID, page, pageSize, outcome)
	if err != nil {
		log.Printf("Could not retrieve messages for detail page: %s", err)
		http.Error(w, "Could not retrieve requests", http.StatusInternalServerError)
		return
	}

	data := webhookPageData{
		PageTitle:  fmt.Sprintf("Webhook %s", webhookID),
		Webhook:    h.buildWebhookCardView(r, webhook),
		Requests:   buildRequestViews(messagePage.Messages),
		Outcome:    buildOutcomeFilterView(webhookID, pageSize, outcome),
		Pagination: buildPaginationView(webhookID, messagePage, outcome),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "webhook.gohtml", data); err != nil {
		log.Printf("Could not render webhook page: %s", err)
		http.Error(w, "Could not render page", http.StatusInternalServerError)
	}
}

func (h *Handler) webhookFormPOSTHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := r.ParseForm(); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.renderHomePage(w, r, "Form submission is too large", http.StatusBadRequest)
			return
		}
		h.renderHomePage(w, r, "Could not parse form submission", http.StatusBadRequest)
		return
	}

	webhookInput := &model.WebhookInput{
		Username:   r.FormValue("username"),
		Password:   r.FormValue("password"),
		TokenName:  r.FormValue("tokenName"),
		TokenValue: r.FormValue("tokenValue"),
		HMACHeader: r.FormValue("hmacHeader"),
		HMACSecret: r.FormValue("hmacSecret"),
	}

	webhook := model.NewWebhookFromInput(webhookInput)
	if err := webhook.Validate(); err != nil {
		h.renderHomePage(w, r, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	id, err := h.storage.InsertWebhook(webhook)
	if err != nil {
		log.Printf("Could not create webhook from form: %s", err)
		h.renderHomePage(w, r, "Could not create webhook", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/webhooks/%s", id), http.StatusSeeOther)
}

func (h *Handler) renderHomePage(w http.ResponseWriter, r *http.Request, errorMessage string, statusCode int) {
	data := homePageData{
		PageTitle: "Webhook Receiver",
		Error:     errorMessage,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := h.templates.ExecuteTemplate(w, "home.gohtml", data); err != nil {
		log.Printf("Could not render home page: %s", err)
		http.Error(w, "Could not render page", http.StatusInternalServerError)
	}
}

func (h *Handler) buildWebhookCardView(r *http.Request, webhook *model.Webhook) webhookCardView {
	baseURL := h.requestBaseURL(r)
	return webhookCardView{
		ID:              webhook.ID,
		AuthModes:       authModesForWebhook(webhook),
		DetailPath:      fmt.Sprintf("/webhooks/%s", webhook.ID),
		PublicIngestURL: capabilityURL(baseURL, fmt.Sprintf("/hooks/%s", webhook.ID)),
		MessagesURL:     capabilityURL(baseURL, fmt.Sprintf("/api/webhooks/%s/messages", webhook.ID)),
		ExpiresAt:       webhook.ExpiresAt.Format(timeLayout),
	}
}

func buildRequestViews(messages []*model.Message) []requestView {
	requests := make([]requestView, 0, len(messages))
	for _, message := range messages {
		requests = append(requests, requestView{
			Method:       message.Method,
			Path:         message.Path,
			Query:        message.Query,
			Payload:      message.Payload,
			Time:         message.Time.Format(timeLayout),
			Headers:      buildHeaderViews(message.Headers),
			StatusCode:   message.StatusCode,
			StatusText:   http.StatusText(message.StatusCode),
			Rejected:     message.Rejected(),
			ErrorMessage: message.ErrorMessage,
		})
	}

	return requests
}

func buildPaginationView(webhookID string, page *model.MessagePage, outcome model.MessageOutcome) paginationView {
	view := paginationView{
		CurrentPage:     page.Page,
		PageSize:        page.PageSize,
		TotalMessages:   page.TotalMessages,
		TotalPages:      page.TotalPages,
		Outcome:         string(outcome),
		HasNextPage:     page.HasNextPage,
		HasPreviousPage: page.HasPreviousPage,
	}

	if page.HasPreviousPage {
		view.PreviousPageURL = detailPageURL(webhookID, page.Page-1, page.PageSize, outcome)
	}
	if page.HasNextPage {
		view.NextPageURL = detailPageURL(webhookID, page.Page+1, page.PageSize, outcome)
	}

	return view
}

func buildOutcomeFilterView(webhookID string, pageSize int, outcome model.MessageOutcome) outcomeFilterView {
	return outcomeFilterView{
		Current:     string(outcome),
		AllURL:      detailPageURL(webhookID, 1, pageSize, model.MessageOutcomeAll),
		AcceptedURL: detailPageURL(webhookID, 1, pageSize, model.MessageOutcomeAccepted),
		RejectedURL: detailPageURL(webhookID, 1, pageSize, model.MessageOutcomeRejected),
	}
}

func detailPageURL(webhookID string, page int, pageSize int, outcome model.MessageOutcome) string {
	queryParts := []string{
		fmt.Sprintf("page=%d", page),
		fmt.Sprintf("pageSize=%d", pageSize),
	}
	if outcome != "" && outcome != model.MessageOutcomeAll {
		queryParts = append(queryParts, fmt.Sprintf("outcome=%s", outcome))
	}

	return fmt.Sprintf("/webhooks/%s?%s", webhookID, strings.Join(queryParts, "&"))
}

func buildHeaderViews(headers map[string][]string) []headerView {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	headerEntries := make([]headerView, 0, len(names))
	for _, name := range names {
		headerEntries = append(headerEntries, headerView{
			Name:   name,
			Values: strings.Join(headers[name], ", "),
		})
	}

	return headerEntries
}

func authModesForWebhook(webhook *model.Webhook) []string {
	authModes := []string{}
	if webhook.HasBasicAuth() {
		authModes = append(authModes, fmt.Sprintf("Basic auth (%s)", webhook.Username))
	}
	if webhook.HasHeaderToken() {
		authModes = append(authModes, fmt.Sprintf("Header token (%s)", webhook.TokenName))
	}
	if webhook.HasHMAC() {
		authModes = append(authModes, fmt.Sprintf("HMAC SHA-256 (%s)", webhook.HMACHeader))
	}
	if len(authModes) == 0 {
		authModes = append(authModes, "No request authentication")
	}

	return authModes
}

func (h *Handler) retrieveWebhookIDFromDetailPath(path string) string {
	segments := cleanPathSegments(path)
	if len(segments) != 2 {
		return ""
	}

	if segments[0] != "webhooks" {
		return ""
	}

	return segments[1]
}

const timeLayout = "2006-01-02 15:04:05 MST"
