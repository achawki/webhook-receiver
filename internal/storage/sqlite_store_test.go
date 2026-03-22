package storage_test

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/achawki/webhook-receiver/internal/model"
	"github.com/achawki/webhook-receiver/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
const testEncryptionKeyHex = "3031323334353637383961626364656630313233343536373839616263646566"

func TestNewSQLiteStoreCreatesDatabaseAndLoadsEmptyState(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "webhook-receiver.db")

	store, err := storage.NewSQLiteStore(storePath, testEncryptionKey)

	require.NoError(t, err)
	require.NotNil(t, store)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	webhooks, err := store.ListWebhooks()
	require.NoError(t, err)
	assert.Empty(t, webhooks)
}

func TestSQLiteStorePersistsWebhooksAndMessages(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "webhook-receiver.db")
	store, err := storage.NewSQLiteStore(storePath, testEncryptionKey)
	require.NoError(t, err)

	webhook := model.NewWebhook("username", "password", "X-Webhook-Token", "token", "X-Hub-Signature-256", "secret")
	webhookID, err := store.InsertWebhook(webhook)
	require.NoError(t, err)

	message := model.NewMessage(http.MethodPost, "/hooks/"+webhookID, "source=test", `{"hello":"world"}`, map[string][]string{
		"Content-Type": {"application/json"},
	})
	require.NoError(t, store.InsertMessage(webhookID, message))
	require.NoError(t, store.Close())

	reloadedStore, err := storage.NewSQLiteStore(storePath, testEncryptionKey)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, reloadedStore.Close())
	})

	reloadedWebhook, err := reloadedStore.GetWebhook(webhookID)
	require.NoError(t, err)
	assert.Equal(t, webhookID, reloadedWebhook.ID)
	assert.Equal(t, "username", reloadedWebhook.Username)
	assert.Equal(t, "X-Webhook-Token", reloadedWebhook.TokenName)
	assert.Equal(t, "X-Hub-Signature-256", reloadedWebhook.HMACHeader)
	assert.False(t, reloadedWebhook.ExpiresAt.IsZero())
	assert.True(t, reloadedWebhook.ExpiresAt.After(time.Now().UTC().Add(47*time.Hour)))
	assert.True(t, reloadedWebhook.HasBasicAuth())
	assert.True(t, reloadedWebhook.HasHeaderToken())
	assert.True(t, reloadedWebhook.HasHMAC())

	messagePage, err := reloadedStore.GetMessagePageForWebhook(webhookID, 1, 25, model.MessageOutcomeAll)
	require.NoError(t, err)
	require.Len(t, messagePage.Messages, 1)
	assert.Equal(t, 1, messagePage.Page)
	assert.Equal(t, 25, messagePage.PageSize)
	assert.Equal(t, 1, messagePage.TotalMessages)
	assert.Equal(t, http.MethodPost, messagePage.Messages[0].Method)
	assert.Equal(t, "/hooks/"+webhookID, messagePage.Messages[0].Path)
	assert.Equal(t, "source=test", messagePage.Messages[0].Query)
	assert.Equal(t, `{"hello":"world"}`, messagePage.Messages[0].Payload)
	assert.Equal(t, http.StatusOK, messagePage.Messages[0].StatusCode)
	assert.Empty(t, messagePage.Messages[0].ErrorMessage)
}

func TestSQLiteStoreDoesNotPersistHMACSecretAsPlaintext(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "webhook-receiver.db")
	store, err := storage.NewSQLiteStore(storePath, testEncryptionKey)
	require.NoError(t, err)

	webhook := model.NewWebhook("", "", "", "", "X-Hub-Signature-256", "super-secret-hmac-value")
	_, err = store.InsertWebhook(webhook)
	require.NoError(t, err)
	require.NoError(t, store.Close())

	content, err := os.ReadFile(storePath)
	require.NoError(t, err)
	assert.False(t, bytes.Contains(content, []byte("super-secret-hmac-value")))
}

func TestSQLiteStorePersistsRejectedMessageResult(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "webhook-receiver.db")
	store, err := storage.NewSQLiteStore(storePath, testEncryptionKey)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	webhook := model.NewWebhook("", "", "", "", "", "")
	webhookID, err := store.InsertWebhook(webhook)
	require.NoError(t, err)

	message := model.NewMessage(http.MethodPost, "/hooks/"+webhookID, "", `{"hello":"world"}`, map[string][]string{
		"Content-Type": {"application/json"},
	})
	message.MarkRejected(http.StatusUnauthorized, "Missing basic auth credentials")
	require.NoError(t, store.InsertMessage(webhookID, message))

	messagePage, err := store.GetMessagePageForWebhook(webhookID, 1, 25, model.MessageOutcomeAll)
	require.NoError(t, err)
	require.Len(t, messagePage.Messages, 1)
	assert.Equal(t, http.StatusUnauthorized, messagePage.Messages[0].StatusCode)
	assert.Equal(t, "Missing basic auth credentials", messagePage.Messages[0].ErrorMessage)
}

func TestNewSQLiteStoreRejectsInvalidEncryptionKey(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "webhook-receiver.db")

	_, err := storage.NewSQLiteStore(storePath, "not-a-valid-key")

	require.Error(t, err)
}

func TestNewSQLiteStoreAcceptsHexEncodedEncryptionKey(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "webhook-receiver.db")

	store, err := storage.NewSQLiteStore(storePath, testEncryptionKeyHex)

	require.NoError(t, err)
	require.NotNil(t, store)
	require.NoError(t, store.Close())
}

func TestSQLiteStoreHidesExpiredWebhook(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "webhook-receiver.db")
	store, err := storage.NewSQLiteStore(storePath, testEncryptionKey)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	webhook := model.NewWebhook("", "", "", "", "", "")
	webhook.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	webhookID, err := store.InsertWebhook(webhook)
	require.NoError(t, err)

	_, err = store.GetWebhook(webhookID)
	require.Error(t, err)
	_, ok := err.(*storage.WebhookNotFoundError)
	assert.True(t, ok)

	messages, err := store.GetMessagePageForWebhook(webhookID, 1, 25, model.MessageOutcomeAll)
	require.Error(t, err)
	assert.Nil(t, messages)
}

func TestSQLiteStoreDeletesExpiredWebhook(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "webhook-receiver.db")
	store, err := storage.NewSQLiteStore(storePath, testEncryptionKey)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	webhook := model.NewWebhook("", "", "", "", "", "")
	webhook.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	webhookID, err := store.InsertWebhook(webhook)
	require.NoError(t, err)

	deletedCount, err := store.DeleteExpiredWebhooks()
	require.NoError(t, err)
	assert.Equal(t, 1, deletedCount)

	_, err = store.GetWebhook(webhookID)
	require.Error(t, err)
	_, ok := err.(*storage.WebhookNotFoundError)
	assert.True(t, ok)
}

func TestSQLiteStorePaginatesMessagesNewestFirst(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "webhook-receiver.db")
	store, err := storage.NewSQLiteStore(storePath, testEncryptionKey)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	webhook := model.NewWebhook("", "", "", "", "", "")
	webhookID, err := store.InsertWebhook(webhook)
	require.NoError(t, err)

	require.NoError(t, store.InsertMessage(webhookID, model.NewMessage(http.MethodPost, "/hooks/"+webhookID, "", `{"message":"first"}`, nil)))
	require.NoError(t, store.InsertMessage(webhookID, model.NewMessage(http.MethodPost, "/hooks/"+webhookID, "", `{"message":"second"}`, nil)))
	require.NoError(t, store.InsertMessage(webhookID, model.NewMessage(http.MethodPost, "/hooks/"+webhookID, "", `{"message":"third"}`, nil)))

	firstPage, err := store.GetMessagePageForWebhook(webhookID, 1, 2, model.MessageOutcomeAll)
	require.NoError(t, err)
	require.Len(t, firstPage.Messages, 2)
	assert.Equal(t, 3, firstPage.TotalMessages)
	assert.Equal(t, 2, firstPage.TotalPages)
	assert.True(t, firstPage.HasNextPage)
	assert.False(t, firstPage.HasPreviousPage)
	assert.Equal(t, `{"message":"third"}`, firstPage.Messages[0].Payload)
	assert.Equal(t, `{"message":"second"}`, firstPage.Messages[1].Payload)

	secondPage, err := store.GetMessagePageForWebhook(webhookID, 2, 2, model.MessageOutcomeAll)
	require.NoError(t, err)
	require.Len(t, secondPage.Messages, 1)
	assert.False(t, secondPage.HasNextPage)
	assert.True(t, secondPage.HasPreviousPage)
	assert.Equal(t, `{"message":"first"}`, secondPage.Messages[0].Payload)
}

func TestSQLiteStoreClampsPageBeyondLastPage(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "webhook-receiver.db")
	store, err := storage.NewSQLiteStore(storePath, testEncryptionKey)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	webhookID, err := store.InsertWebhook(model.NewWebhook("", "", "", "", "", ""))
	require.NoError(t, err)

	require.NoError(t, store.InsertMessage(webhookID, model.NewMessage(http.MethodPost, "/hooks/"+webhookID, "", `{"message":"first"}`, nil)))
	require.NoError(t, store.InsertMessage(webhookID, model.NewMessage(http.MethodPost, "/hooks/"+webhookID, "", `{"message":"second"}`, nil)))
	require.NoError(t, store.InsertMessage(webhookID, model.NewMessage(http.MethodPost, "/hooks/"+webhookID, "", `{"message":"third"}`, nil)))

	page, err := store.GetMessagePageForWebhook(webhookID, 999, 2, model.MessageOutcomeAll)
	require.NoError(t, err)
	require.Len(t, page.Messages, 1)
	assert.Equal(t, 2, page.Page)
	assert.Equal(t, 2, page.TotalPages)
	assert.True(t, page.HasPreviousPage)
	assert.False(t, page.HasNextPage)
	assert.Equal(t, `{"message":"first"}`, page.Messages[0].Payload)
}

func TestSQLiteStoreFiltersMessagesByOutcome(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "webhook-receiver.db")
	store, err := storage.NewSQLiteStore(storePath, testEncryptionKey)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	webhookID, err := store.InsertWebhook(model.NewWebhook("", "", "", "", "", ""))
	require.NoError(t, err)

	acceptedMessage := model.NewMessage(http.MethodPost, "/hooks/"+webhookID, "", `{"message":"accepted"}`, nil)
	rejectedMessage := model.NewMessage(http.MethodPost, "/hooks/"+webhookID, "", `{"message":"rejected"}`, nil)
	rejectedMessage.MarkRejected(http.StatusUnauthorized, "Missing basic auth credentials")

	require.NoError(t, store.InsertMessage(webhookID, acceptedMessage))
	require.NoError(t, store.InsertMessage(webhookID, rejectedMessage))

	acceptedPage, err := store.GetMessagePageForWebhook(webhookID, 1, 25, model.MessageOutcomeAccepted)
	require.NoError(t, err)
	require.Len(t, acceptedPage.Messages, 1)
	assert.Equal(t, `{"message":"accepted"}`, acceptedPage.Messages[0].Payload)

	rejectedPage, err := store.GetMessagePageForWebhook(webhookID, 1, 25, model.MessageOutcomeRejected)
	require.NoError(t, err)
	require.Len(t, rejectedPage.Messages, 1)
	assert.Equal(t, `{"message":"rejected"}`, rejectedPage.Messages[0].Payload)
}

func TestSQLiteStorePrunesMessagesBeyondMaximum(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "webhook-receiver.db")
	store, err := storage.NewSQLiteStore(storePath, testEncryptionKey)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	webhookID, err := store.InsertWebhook(model.NewWebhook("", "", "", "", "", ""))
	require.NoError(t, err)

	for i := 0; i < 101; i++ {
		payload := fmt.Sprintf(`{"message":"%03d"}`, i)
		require.NoError(t, store.InsertMessage(webhookID, model.NewMessage(http.MethodPost, "/hooks/"+webhookID, "", payload, nil)))
	}

	page, err := store.GetMessagePageForWebhook(webhookID, 1, 100, model.MessageOutcomeAll)
	require.NoError(t, err)
	require.Len(t, page.Messages, 100)
	assert.Equal(t, 100, page.TotalMessages)
	assert.Equal(t, `{"message":"100"}`, page.Messages[0].Payload)
	assert.Equal(t, `{"message":"001"}`, page.Messages[len(page.Messages)-1].Payload)
}
