package handler

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/achawki/webhook-receiver/internal/model"
	"github.com/achawki/webhook-receiver/internal/storage"
)

// MessageHandler handles request for messages endpoint
// GET and POST are currently supported
func (h *Handler) MessageHandler(w http.ResponseWriter, r *http.Request) {
	webhookID := h.retrieveWebhookIDFromPath(r.URL.Path)
	if webhookID == "" {
		h.UnknownHandler(w, r)
		return
	}

	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		h.UnknownHandler(w, r)
		return
	}

	webhook, err := h.storage.GetWebhook(webhookID)
	if err != nil {
		log.Printf("Could not retrieve webhook: %s", err)
		switch err.(type) {
		case *storage.WebhookNotFoundError:
			h.unknownWebhookHandler(w, r, webhookID)
		default:
			h.internalServerErrorHandler(w, "Could not retrieve webhook")
		}
		return
	}

	authorized := webhook.ValidateAuthorization(r)
	if !authorized {
		log.Printf("Not authorized to access webhook with ID: %s", webhook.ID)
		h.unauthorizedHandler(w)
		return
	}

	if r.Method == http.MethodPost {
		h.messagePOSTHandler(w, r, webhook)
	} else if r.Method == http.MethodGet {
		h.messagesGETHandler(w, r, webhook)
	}
}

func (h *Handler) messagePOSTHandler(w http.ResponseWriter, r *http.Request, webhook *model.Webhook) {
	r.Body = http.MaxBytesReader(w, r.Body, 1048576)
	requestBody, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		h.internalServerErrorHandler(w, "Something went wrong")
		return
	}

	headers := r.Header
	delete(headers, "Authorization")
	if webhook.TokenName != "" {
		// headers are stores capitalized first letter
		delete(headers, strings.Title(webhook.TokenName))
	}
	message := model.NewMessage(string(requestBody), headers)

	err = h.storage.InsertMessage(webhook.ID, message)
	if err != nil {
		log.Printf("Could not insert webhook %s", err)
		h.internalServerErrorHandler(w, "Something went wrong")
		return
	}
	log.Printf("Inserted message for webhook %s", webhook.ID)
}

func (h *Handler) messagesGETHandler(w http.ResponseWriter, r *http.Request, webhook *model.Webhook) {
	log.Printf("Retrieving messages for webhook %s", webhook.ID)
	messages, err := h.storage.GetMessagesForWebhook(webhook.ID)
	if err != nil {
		log.Printf("Could not retrieve messages for webhook %s: %s", webhook.ID, err)
		h.internalServerErrorHandler(w, "Something went wrong")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func (h *Handler) retrieveWebhookIDFromPath(path string) string {
	result := h.messageHandlerrgx.FindStringSubmatch(path)

	if len(result) < 2 {
		return ""
	}

	return result[1]
}
