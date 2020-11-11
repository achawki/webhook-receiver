package storage_test

import (
	"testing"

	"github.com/achawki/webhook-receiver/internal/model"
	"github.com/achawki/webhook-receiver/internal/storage"
	"github.com/stretchr/testify/assert"
)

func TestNewInMemoryStore(t *testing.T) {
	inMemoryStore := storage.NewInMemoryStore()

	assert.NotNil(t, inMemoryStore)
}

func TestInsertWebhook(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "token", "secret")
	inMemoryStore := storage.NewInMemoryStore()
	uuid, err := inMemoryStore.InsertWebhook(webhook)

	assert.Nil(t, err)
	assert.NotEmpty(t, uuid)
	assert.NotEmpty(t, webhook.ID)
}

func TestGetwebhook(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "token", "secret")
	inMemoryStore := storage.NewInMemoryStore()
	uuid, insertErr := inMemoryStore.InsertWebhook(webhook)
	retrievedWebhook, err := inMemoryStore.GetWebhook(uuid)

	assert.Nil(t, insertErr)
	assert.Nil(t, err)
	assert.NotNil(t, retrievedWebhook)
	assert.Equal(t, uuid, retrievedWebhook.ID)
}

func TestGetwebhookForNonExistingID(t *testing.T) {
	inMemoryStore := storage.NewInMemoryStore()
	_, err := inMemoryStore.GetWebhook("unknown")
	_, ok := err.(*storage.WebhookNotFoundError)

	assert.True(t, ok)
	assert.EqualError(t, err, "Webhook with ID unknown not found")
}

func TestInsertMessage(t *testing.T) {
	inMemoryStore, webhookId := setupInMemoryStoreWithWebhook()
	err := inMemoryStore.InsertMessage(webhookId, &model.Message{})

	assert.Nil(t, err)
}
func TestInsertMessageForNonExistingWebhook(t *testing.T) {
	inMemoryStore := storage.NewInMemoryStore()
	err := inMemoryStore.InsertMessage("unknown", &model.Message{})
	_, ok := err.(*storage.WebhookNotFoundError)

	assert.True(t, ok)
	assert.EqualError(t, err, "Webhook with ID unknown not found")
}

func TestGetMessagesForWebhook(t *testing.T) {
	inMemoryStore, webhookId := setupInMemoryStoreWithWebhook()
	payload := "test"
	payload2 := "test2"
	inMemoryStore.InsertMessage(webhookId, model.NewMessage(payload, nil))
	inMemoryStore.InsertMessage(webhookId, model.NewMessage(payload2, nil))

	messages, err := inMemoryStore.GetMessagesForWebhook(webhookId)

	assert.Nil(t, err)
	assert.Equal(t, 2, len(messages))
	assert.Equal(t, payload, messages[0].Payload)
	assert.Equal(t, payload2, messages[1].Payload)
}

func TestGetMessagesForUnknownWebhhok(t *testing.T) {
	inMemoryStore := storage.NewInMemoryStore()
	_, err := inMemoryStore.GetMessagesForWebhook("unknown")
	_, ok := err.(*storage.WebhookNotFoundError)

	assert.True(t, ok)
	assert.EqualError(t, err, "Webhook with ID unknown not found")
}

func TestGetMessagesForEmptyWebhook(t *testing.T) {
	inMemoryStore, webhookId := setupInMemoryStoreWithWebhook()
	messages, err := inMemoryStore.GetMessagesForWebhook(webhookId)
	assert.Nil(t, err)
	assert.Empty(t, messages)
}

func setupInMemoryStoreWithWebhook() (*storage.InMemoryStore, string) {
	webhook := model.NewWebhook("username", "password", "token", "secret")
	inMemoryStore := storage.NewInMemoryStore()
	id, _ := inMemoryStore.InsertWebhook(webhook)

	return inMemoryStore, id
}
