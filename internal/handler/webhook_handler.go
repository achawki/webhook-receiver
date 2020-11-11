package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/achawki/webhook-receiver/internal/model"
)

// WebhookHandler handles request for webhook endpoint.
// Currently only POST is supported
func (h *Handler) WebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.UnknownHandler(w, r)
		return
	}

	var webhookInput model.WebhookInput

	r.Body = http.MaxBytesReader(w, r.Body, 1048576)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	err := decoder.Decode(&webhookInput)
	if err != nil {
		log.Printf("Error occurred during json processing: %s", err)
		message := processDecodingError(err)
		h.badRequestHandler(w, message)
		return
	}

	webhook := model.NewWebhookFromInput(&webhookInput)

	err = webhook.Validate()
	if err != nil {
		log.Printf("Error occurred during webhook validation %s", err)
		h.validationErrorHandler(w, err.Error())
		return
	}

	id, err := h.storage.InsertWebhook(webhook)
	if err != nil {
		log.Printf("Error occurred while inserting webhhok %s", err)
		h.internalServerErrorHandler(w, "Error occurred while inserting webhook.")
		return
	}
	webhook.ID = id
	log.Printf("Inserted webhook with ID %s successfully", id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhook)
}

func processDecodingError(err error) string {
	switch err.(type) {
	case *json.SyntaxError:
		return "Body contains malformed json"
	case *json.UnmarshalTypeError:
		return "Wrong type provided for input"
	default:
		if strings.HasPrefix(err.Error(), "json: unknown field") {
			return err.Error()
		}
		return "Could not process body. Invalid input"
	}
}
