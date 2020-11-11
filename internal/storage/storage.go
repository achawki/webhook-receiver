package storage

import (
	"fmt"

	"github.com/achawki/webhook-receiver/internal/model"
)

//go:generate mockery --name WebhookStorage

// WebhookStorage is a storage for webhooks and messages
type WebhookStorage interface {
	InsertWebhook(webhook *model.Webhook) (string, error)
	GetWebhook(id string) (*model.Webhook, error)
	InsertMessage(webhookID string, message *model.Message) error
	GetMessagesForWebhook(webhookID string) ([]*model.Message, error)
}

// WebhookNotFoundError is and error in case webhook does not exist
type WebhookNotFoundError struct {
	WebhookId string
}

// Error function for WebhookNotFoundError
func (e *WebhookNotFoundError) Error() string {
	return fmt.Sprintf("Webhook with ID %s not found", e.WebhookId)
}
