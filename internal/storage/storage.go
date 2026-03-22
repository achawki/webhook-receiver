package storage

import (
	"fmt"

	"github.com/achawki/webhook-receiver/internal/model"
)

//go:generate mockery --name WebhookStorage

// WebhookStorage stores webhooks and captured messages.
type WebhookStorage interface {
	InsertWebhook(webhook *model.Webhook) (string, error)
	GetWebhook(id string) (*model.Webhook, error)
	ListWebhooks() ([]*model.Webhook, error)
	InsertMessage(webhookID string, message *model.Message) error
	GetMessagePageForWebhook(webhookID string, page int, pageSize int, outcome model.MessageOutcome) (*model.MessagePage, error)
}

// WebhookNotFoundError indicates that a webhook does not exist.
type WebhookNotFoundError struct {
	WebhookId string
}

// Error implements the error interface.
func (e *WebhookNotFoundError) Error() string {
	return fmt.Sprintf("Webhook with ID %s not found", e.WebhookId)
}
