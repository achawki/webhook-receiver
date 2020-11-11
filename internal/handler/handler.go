package handler

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/achawki/webhook-receiver/internal/storage"
)

// Handler handels incoming requests
type Handler struct {
	storage           storage.WebhookStorage
	messageHandlerrgx *regexp.Regexp
}

// NewHandler creates and initializes handler
func NewHandler(storage storage.WebhookStorage) *Handler {
	messageHandlerrgx := regexp.MustCompile(`^/api/webhooks/(.*)/messages$`)
	return &Handler{storage: storage, messageHandlerrgx: messageHandlerrgx}
}

// UnknownHandler handles requests for unknown endpoints and returns 404
func (h *Handler) UnknownHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}

func (h *Handler) unknownWebhookHandler(w http.ResponseWriter, r *http.Request, webhookID string) {
	w.Header().Set("Content-Type", "application/json")
	message := fmt.Sprintf(`{"message": "Webhook with ID: %s does not exist"}`, webhookID)
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(message))
}

func (h *Handler) badRequestHandler(w http.ResponseWriter, message string) {
	errorMessage := fmt.Sprintf(`{"message": "%s"}`, message)
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(errorMessage))
}

func (h *Handler) validationErrorHandler(w http.ResponseWriter, message string) {
	errorMessage := fmt.Sprintf(`{"message": "%s"}`, message)
	w.WriteHeader(http.StatusUnprocessableEntity)
	w.Write([]byte(errorMessage))
}

func (h *Handler) internalServerErrorHandler(w http.ResponseWriter, message string) {
	errorMessage := fmt.Sprintf(`{"message": "%s"}`, message)
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(errorMessage))
}

func (h *Handler) unauthorizedHandler(w http.ResponseWriter) {
	w.WriteHeader(http.StatusUnauthorized)
}
