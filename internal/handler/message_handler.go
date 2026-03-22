package handler

import (
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/achawki/webhook-receiver/internal/model"
	"github.com/achawki/webhook-receiver/internal/storage"
)

const (
	defaultMessagesPage = 1
	defaultMessagesSize = 25
	maxMessagesPageSize = 100
)

// MessageHandler handles requests for the captured-messages endpoint.
func (h *Handler) MessageHandler(w http.ResponseWriter, r *http.Request) {
	webhookID := h.retrieveWebhookIDFromMessagePath(r.URL.Path)
	if webhookID == "" {
		h.UnknownHandler(w, r)
		return
	}

	if r.Method != http.MethodGet {
		h.UnknownHandler(w, r)
		return
	}

	if !h.allowRequest(w, r) {
		return
	}

	webhook, err := h.storage.GetWebhook(webhookID)
	if err != nil {
		log.Printf("Could not retrieve webhook: %s", err)
		switch err.(type) {
		case *storage.WebhookNotFoundError:
			h.unknownWebhookHandler(w, webhookID)
		default:
			h.internalServerErrorHandler(w, "Could not retrieve webhook")
		}
		return
	}

	h.messagesGETHandler(w, r, webhook)
}

// HookHandler accepts incoming webhook deliveries on /hooks/{id}[/*].
func (h *Handler) HookHandler(w http.ResponseWriter, r *http.Request) {
	webhookID := h.retrieveWebhookIDFromHookPath(r.URL.Path)
	if webhookID == "" {
		h.UnknownHandler(w, r)
		return
	}

	if !h.allowRequest(w, r) {
		return
	}

	webhook, err := h.storage.GetWebhook(webhookID)
	if err != nil {
		log.Printf("Could not retrieve webhook: %s", err)
		switch err.(type) {
		case *storage.WebhookNotFoundError:
			h.unknownWebhookHandler(w, webhookID)
		default:
			h.internalServerErrorHandler(w, "Could not retrieve webhook")
		}
		return
	}

	h.ingestRequest(w, r, webhook)
}

func (h *Handler) ingestRequest(w http.ResponseWriter, r *http.Request, webhook *model.Webhook) {
	requestBody, err := readRequestBody(w, r)
	if err != nil {
		log.Printf("Could not read request body: %s", err)
		h.badRequestHandler(w, "Could not read request body")
		return
	}

	headers := sanitizedHeaders(r.Header, webhook)
	authFailure := webhook.AuthorizationFailure(r, requestBody)
	if authFailure != "" {
		log.Printf("Not authorized to access webhook with ID: %s", webhook.ID)
		rejectedMessage := model.NewMessage(r.Method, r.URL.Path, r.URL.RawQuery, string(requestBody), headers)
		rejectedMessage.MarkRejected(http.StatusUnauthorized, authFailure)
		if err := h.storage.InsertMessage(webhook.ID, rejectedMessage); err != nil {
			log.Printf("Could not insert rejected webhook request %s", err)
		}
		h.unauthorizedHandler(w)
		return
	}

	message := model.NewMessage(r.Method, r.URL.Path, r.URL.RawQuery, string(requestBody), headers)

	err = h.storage.InsertMessage(webhook.ID, message)
	if err != nil {
		log.Printf("Could not insert webhook message: %s", err)
		h.internalServerErrorHandler(w, "Something went wrong")
		return
	}
	log.Printf("Inserted message for webhook %s", webhook.ID)
}

func sanitizedHeaders(headers http.Header, webhook *model.Webhook) map[string][]string {
	sanitized := headers.Clone()
	sanitized.Del("Authorization")
	if webhook != nil {
		sanitized.Del(webhook.TokenName)
		sanitized.Del(webhook.HMACHeader)
	}
	for name := range sanitized {
		if shouldHideCapturedHeader(name) {
			sanitized.Del(name)
		}
	}

	return sanitized
}

func shouldHideCapturedHeader(name string) bool {
	canonicalName := http.CanonicalHeaderKey(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(canonicalName, "Fly-"):
		return true
	case strings.HasPrefix(canonicalName, "X-Forwarded-"):
		return true
	case canonicalName == "Via":
		return true
	case canonicalName == "X-Request-Start":
		return true
	default:
		return false
	}
}

func (h *Handler) messagesGETHandler(w http.ResponseWriter, r *http.Request, webhook *model.Webhook) {
	page, pageSize, outcome, err := messagePageFromQuery(r)
	if err != nil {
		h.badRequestHandler(w, err.Error())
		return
	}

	log.Printf("Retrieving messages for webhook %s", webhook.ID)
	messagePage, err := h.storage.GetMessagePageForWebhook(webhook.ID, page, pageSize, outcome)
	if err != nil {
		log.Printf("Could not retrieve messages for webhook %s: %s", webhook.ID, err)
		h.internalServerErrorHandler(w, "Something went wrong")
		return
	}

	h.writeJSON(w, http.StatusOK, struct {
		WebhookID       string           `json:"webhookId"`
		ExpiresAt       string           `json:"expiresAt"`
		Outcome         string           `json:"outcome"`
		Messages        []*model.Message `json:"messages"`
		Page            int              `json:"page"`
		PageSize        int              `json:"pageSize"`
		TotalMessages   int              `json:"totalMessages"`
		TotalPages      int              `json:"totalPages"`
		HasNextPage     bool             `json:"hasNextPage"`
		HasPreviousPage bool             `json:"hasPreviousPage"`
	}{
		WebhookID:       webhook.ID,
		ExpiresAt:       webhook.ExpiresAt.Format(time.RFC3339Nano),
		Outcome:         string(outcome),
		Messages:        messagePage.Messages,
		Page:            messagePage.Page,
		PageSize:        messagePage.PageSize,
		TotalMessages:   messagePage.TotalMessages,
		TotalPages:      messagePage.TotalPages,
		HasNextPage:     messagePage.HasNextPage,
		HasPreviousPage: messagePage.HasPreviousPage,
	})
}

func readRequestBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	defer func() {
		_ = r.Body.Close()
	}()

	return io.ReadAll(r.Body)
}

func (h *Handler) retrieveWebhookIDFromMessagePath(path string) string {
	segments := cleanPathSegments(path)
	if len(segments) != 4 {
		return ""
	}

	if segments[0] != "api" || segments[1] != "webhooks" || segments[3] != "messages" {
		return ""
	}

	return segments[2]
}

func (h *Handler) retrieveWebhookIDFromHookPath(path string) string {
	segments := cleanPathSegments(path)
	if len(segments) < 2 {
		return ""
	}

	if segments[0] != "hooks" {
		return ""
	}

	return segments[1]
}

func messagePageFromQuery(r *http.Request) (int, int, model.MessageOutcome, error) {
	page := defaultMessagesPage
	pageSize := defaultMessagesSize
	outcome := model.MessageOutcomeAll

	if pageValue := r.URL.Query().Get("page"); pageValue != "" {
		parsedPage, err := strconv.Atoi(pageValue)
		if err != nil || parsedPage < 1 {
			return 0, 0, "", errInvalidPagination("page")
		}
		page = parsedPage
	}

	if pageSizeValue := r.URL.Query().Get("pageSize"); pageSizeValue != "" {
		parsedPageSize, err := strconv.Atoi(pageSizeValue)
		if err != nil || parsedPageSize < 1 || parsedPageSize > maxMessagesPageSize {
			return 0, 0, "", errInvalidPagination("pageSize")
		}
		pageSize = parsedPageSize
	}

	if outcomeValue := r.URL.Query().Get("outcome"); outcomeValue != "" {
		parsedOutcome, ok := model.ParseMessageOutcome(outcomeValue)
		if !ok {
			return 0, 0, "", &paginationError{message: "outcome must be one of all, accepted, rejected"}
		}
		outcome = parsedOutcome
	}

	return page, pageSize, outcome, nil
}

func errInvalidPagination(field string) error {
	if field == "pageSize" {
		return &paginationError{message: "pageSize must be a positive integer no larger than 100"}
	}

	return &paginationError{message: "page must be a positive integer"}
}

type paginationError struct {
	message string
}

func (e *paginationError) Error() string {
	return e.message
}
