package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/achawki/webhook-receiver/internal/handler"
	"github.com/achawki/webhook-receiver/internal/model"
	"github.com/achawki/webhook-receiver/internal/storage/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestWebhookHandlerWithUnsupportedHTTPMethod(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPatch, "http://localhost/api/webhooks", nil)

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}

func TestWebhookHandlerRejectsGET(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodGet, "http://localhost/api/webhooks", nil)

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}

func TestWebhookHandlerWithInvalidInput(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/api/webhooks", bytes.NewBuffer([]byte("invalid")))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestWebhookHandlerWithInvalidJson(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/api/webhooks", bytes.NewBuffer([]byte(`{"invalid: "not valid"}`)))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestWebhookHandlerWithIncompleteJson(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/api/webhooks", bytes.NewBuffer([]byte(`{"incomplete": []`)))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestWebhookHandlerWithUnknownField(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/api/webhooks", bytes.NewBuffer([]byte(`{"unknown": "unknown"}`)))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)

	assert.JSONEq(t, `{"message":"json: unknown field \"unknown\""}`, w.Body.String())
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestWebhookHandlerWithWrongInputType(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/api/webhooks", bytes.NewBuffer([]byte(`{"username": []}`)))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestWebhookHandlerWithValidationFailing(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/api/webhooks", bytes.NewBuffer([]byte(`{"username": "username"}`)))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Result().StatusCode)
}

func TestWebhookHandlerWithValidInput(t *testing.T) {
	expectedExpiry := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("InsertWebhook", mock.Anything).Return("id", nil).Run(func(args mock.Arguments) {
		webhook := args.Get(0).(*model.Webhook)
		webhook.ExpiresAt = expectedExpiry
	})
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/api/webhooks", bytes.NewBuffer([]byte(`{}`)))
	request.RemoteAddr = "127.0.0.1:1234"

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)

	var response struct {
		ID          string    `json:"id"`
		DetailURL   string    `json:"detailUrl"`
		HookURL     string    `json:"hookUrl"`
		MessagesURL string    `json:"messagesUrl"`
		ExpiresAt   time.Time `json:"expiresAt"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "id", response.ID)
	assert.Equal(t, "http://localhost/webhooks/id", response.DetailURL)
	assert.Equal(t, "http://localhost/hooks/id", response.HookURL)
	assert.Equal(t, "http://localhost/api/webhooks/id/messages", response.MessagesURL)
	assert.True(t, response.ExpiresAt.Equal(expectedExpiry))
}

func TestWebhookHandlerIgnoresForwardedHostByDefault(t *testing.T) {
	expectedExpiry := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("InsertWebhook", mock.Anything).Return("id", nil).Run(func(args mock.Arguments) {
		webhook := args.Get(0).(*model.Webhook)
		webhook.ExpiresAt = expectedExpiry
	})
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/api/webhooks", bytes.NewBuffer([]byte(`{}`)))
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Forwarded-Host", "evil.example")
	request.Header.Set("X-Forwarded-Proto", "https")

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)

	var response struct {
		DetailURL string `json:"detailUrl"`
		HookURL   string `json:"hookUrl"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "http://localhost/webhooks/id", response.DetailURL)
	assert.Equal(t, "http://localhost/hooks/id", response.HookURL)
}

func TestWebhookHandlerReturnsRelativeURLsWithoutPublicBaseURL(t *testing.T) {
	expectedExpiry := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("InsertWebhook", mock.Anything).Return("id", nil).Run(func(args mock.Arguments) {
		webhook := args.Get(0).(*model.Webhook)
		webhook.ExpiresAt = expectedExpiry
	})
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/api/webhooks", bytes.NewBuffer([]byte(`{}`)))
	request.Host = "evil.example"
	request.RemoteAddr = "198.51.100.10:1234"

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)

	var response struct {
		DetailURL   string `json:"detailUrl"`
		HookURL     string `json:"hookUrl"`
		MessagesURL string `json:"messagesUrl"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "/webhooks/id", response.DetailURL)
	assert.Equal(t, "/hooks/id", response.HookURL)
	assert.Equal(t, "/api/webhooks/id/messages", response.MessagesURL)
}

func TestWebhookHandlerUsesConfiguredPublicBaseURL(t *testing.T) {
	expectedExpiry := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("InsertWebhook", mock.Anything).Return("id", nil).Run(func(args mock.Arguments) {
		webhook := args.Get(0).(*model.Webhook)
		webhook.ExpiresAt = expectedExpiry
	})
	handler := handler.NewHandler(mockStorage, handler.WithPublicBaseURL("https://hooks.example.com"))
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/api/webhooks", bytes.NewBuffer([]byte(`{}`)))
	request.Header.Set("X-Forwarded-Host", "evil.example")

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)

	var response struct {
		DetailURL   string `json:"detailUrl"`
		HookURL     string `json:"hookUrl"`
		MessagesURL string `json:"messagesUrl"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "https://hooks.example.com/webhooks/id", response.DetailURL)
	assert.Equal(t, "https://hooks.example.com/hooks/id", response.HookURL)
	assert.Equal(t, "https://hooks.example.com/api/webhooks/id/messages", response.MessagesURL)
}

func TestWebhookHandlerWithDatabaseError(t *testing.T) {
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("InsertWebhook", mock.Anything).Return("", errors.New("Database error"))
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/api/webhooks", bytes.NewBuffer([]byte(`{}`)))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
}
