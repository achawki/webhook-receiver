package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/achawki/webhook-receiver/internal/model"
)

type createdWebhookResponse struct {
	ID          string    `json:"id"`
	DetailURL   string    `json:"detailUrl"`
	HookURL     string    `json:"hookUrl"`
	MessagesURL string    `json:"messagesUrl"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// WebhookHandler handles request for webhook endpoint.
// POST creates a new webhook.
func (h *Handler) WebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/webhooks" {
		h.UnknownHandler(w, r)
		return
	}

	if r.Method != http.MethodPost {
		h.UnknownHandler(w, r)
		return
	}

	if !h.allowRequest(w, r) {
		return
	}

	webhookInput, err := decodeWebhookJSONInput(w, r)
	if err != nil {
		log.Printf("Error occurred during json processing: %s", err)
		message := processDecodingError(err)
		h.badRequestHandler(w, message)
		return
	}

	h.createWebhook(w, r, webhookInput)
}

func (h *Handler) createWebhook(w http.ResponseWriter, r *http.Request, webhookInput *model.WebhookInput) {
	webhook := model.NewWebhookFromInput(webhookInput)

	err := webhook.Validate()
	if err != nil {
		log.Printf("Error occurred during webhook validation %s", err)
		h.validationErrorHandler(w, err.Error())
		return
	}

	id, err := h.storage.InsertWebhook(webhook)
	if err != nil {
		log.Printf("Error occurred while inserting webhook %s", err)
		h.internalServerErrorHandler(w, "Error occurred while inserting webhook.")
		return
	}
	webhook.ID = id
	log.Printf("Inserted webhook with ID %s successfully", id)

	baseURL := h.requestBaseURL(r)
	h.writeJSON(w, http.StatusOK, createdWebhookResponse{
		ID:          webhook.ID,
		DetailURL:   capabilityURL(baseURL, "/webhooks/"+webhook.ID),
		HookURL:     capabilityURL(baseURL, "/hooks/"+webhook.ID),
		MessagesURL: capabilityURL(baseURL, "/api/webhooks/"+webhook.ID+"/messages"),
		ExpiresAt:   webhook.ExpiresAt,
	})
}

func decodeWebhookJSONInput(w http.ResponseWriter, r *http.Request) (*model.WebhookInput, error) {
	var webhookInput model.WebhookInput

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	defer func() {
		_ = r.Body.Close()
	}()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&webhookInput); err != nil {
		return nil, err
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, errors.New("body must only contain a single json object")
		}
		return nil, err
	}

	return &webhookInput, nil
}

func processDecodingError(err error) string {
	switch err.(type) {
	case *json.SyntaxError:
		return "Body contains malformed json"
	case *json.UnmarshalTypeError:
		return "Wrong type provided for input"
	default:
		if errors.Is(err, io.EOF) {
			return "Request body must not be empty"
		}
		if strings.HasPrefix(err.Error(), "json: unknown field") {
			return err.Error()
		}
		if strings.Contains(err.Error(), "http: request body too large") {
			return "Request body is too large"
		}
		return "Could not process body. Invalid input"
	}
}
