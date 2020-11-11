package storage

import (
	"sync"

	"github.com/achawki/webhook-receiver/internal/model"
	"github.com/google/uuid"
)

// InMemoryStore is an in-memory storage implementation using a map
type InMemoryStore struct {
	webhooksMap     map[string]*model.Webhook
	webhookMessages map[string][]*model.Message
	webhookLock     *sync.RWMutex
	messageLock     *sync.RWMutex
}

// NewInMemoryStore creates and initializes an InMemoryStore
func NewInMemoryStore() *InMemoryStore {
	webhooksMap := make(map[string]*model.Webhook)
	webhookMessages := make(map[string][]*model.Message)
	webhhokLock := &sync.RWMutex{}
	messageLock := &sync.RWMutex{}
	return &InMemoryStore{webhooksMap: webhooksMap, webhookMessages: webhookMessages, webhookLock: webhhokLock, messageLock: messageLock}
}

// InsertWebhook inserts provided webhooks
func (i *InMemoryStore) InsertWebhook(webhook *model.Webhook) (string, error) {
	webhookUuid := uuid.New().String()
	webhook.ID = webhookUuid
	i.webhookLock.Lock()
	defer i.webhookLock.Unlock()
	i.webhooksMap[webhookUuid] = webhook

	return webhookUuid, nil
}

// GetWebhook retrieves webhook with given ID
func (i *InMemoryStore) GetWebhook(id string) (*model.Webhook, error) {
	var webhook *model.Webhook
	var ok bool
	i.webhookLock.RLock()
	defer i.webhookLock.RUnlock()

	if webhook, ok = i.webhooksMap[id]; !ok {
		return nil, &WebhookNotFoundError{WebhookId: id}
	}
	return webhook, nil
}

// InsertMessage inserts message for given webhook ID
func (i *InMemoryStore) InsertMessage(webhookID string, message *model.Message) error {
	_, err := i.GetWebhook(webhookID)
	if err != nil {
		return err
	}

	i.messageLock.Lock()
	defer i.messageLock.Unlock()

	messages := []*model.Message{}
	if existingMessages, ok := i.webhookMessages[webhookID]; ok {
		messages = existingMessages
	}

	messages = append(messages, message)
	i.webhookMessages[webhookID] = messages

	return nil
}

// GetMessagesForWebhook retrieves messages for given webhook ID
func (i *InMemoryStore) GetMessagesForWebhook(webhookId string) ([]*model.Message, error) {
	_, err := i.GetWebhook(webhookId)
	if err != nil {
		return nil, err
	}

	i.messageLock.RLock()
	defer i.messageLock.RUnlock()

	if existingMessages, ok := i.webhookMessages[webhookId]; ok {
		return existingMessages, nil
	}

	return []*model.Message{}, nil
}
