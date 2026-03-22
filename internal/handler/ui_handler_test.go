package handler_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/achawki/webhook-receiver/internal/handler"
	"github.com/achawki/webhook-receiver/internal/model"
	"github.com/achawki/webhook-receiver/internal/storage"
	"github.com/achawki/webhook-receiver/internal/storage/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRegisterServesEmbeddedAssets(t *testing.T) {
	h := handler.NewHandler(nil)
	mux := http.NewServeMux()
	h.Register(mux)

	for _, path := range []string{"/favicon.svg", "/favicon.ico", "/apple-touch-icon.png"} {
		req, _ := http.NewRequest(http.MethodGet, "http://localhost"+path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Result().StatusCode, path)
		assert.NotEmpty(t, w.Body.Bytes(), path)
	}

	req, _ := http.NewRequest(http.MethodGet, "http://localhost/webhooks", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusSeeOther, w.Result().StatusCode)
	assert.Equal(t, "/", w.Result().Header.Get("Location"))
}

func TestWebhooksPageHandlerPOSTRedirectsToDetailPage(t *testing.T) {
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("InsertWebhook", mock.MatchedBy(func(webhook *model.Webhook) bool {
		return webhook.Username == "username" &&
			webhook.HasBasicAuth() &&
			webhook.TokenName == "X-Webhook-Token" &&
			webhook.HasHeaderToken() &&
			webhook.HMACHeader == "X-Hub-Signature-256" &&
			webhook.HasHMAC()
	})).Return("webhook-123", nil)

	h := handler.NewHandler(mockStorage)
	form := url.Values{
		"username":   {"username"},
		"password":   {"password"},
		"tokenName":  {"X-Webhook-Token"},
		"tokenValue": {"token"},
		"hmacHeader": {"X-Hub-Signature-256"},
		"hmacSecret": {"secret"},
	}
	req, _ := http.NewRequest(http.MethodPost, "http://localhost/webhooks", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	h.WebhooksPageHandler(w, req)

	assert.Equal(t, http.StatusSeeOther, w.Result().StatusCode)
	assert.Equal(t, "/webhooks/webhook-123", w.Result().Header.Get("Location"))
	mockStorage.AssertExpectations(t)
}

func TestWebhooksPageHandlerPOSTValidationErrorRendersHome(t *testing.T) {
	h := handler.NewHandler(nil)
	form := url.Values{
		"username": {"username"},
	}
	req, _ := http.NewRequest(http.MethodPost, "http://localhost/webhooks", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	h.WebhooksPageHandler(w, req)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Result().StatusCode)
	assert.Contains(t, w.Body.String(), "Create Webhook")
	assert.Contains(t, w.Body.String(), "username and password must be both set or both empty")
}

func TestWebhooksPageHandlerRejectsOversizedForms(t *testing.T) {
	h := handler.NewHandler(nil)
	oversizedValue := strings.Repeat("a", 2<<20)
	form := url.Values{
		"username": {oversizedValue},
	}
	req, _ := http.NewRequest(http.MethodPost, "http://localhost/webhooks", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	h.WebhooksPageHandler(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
	assert.Contains(t, w.Body.String(), "Form submission is too large")
}

func TestWebhookPageHandlerRendersDetailPageWithPaginationAndFilters(t *testing.T) {
	webhookID := "webhook-123"
	webhook := model.NewWebhook("alice", "password", "X-Webhook-Token", "token", "X-Hub-Signature-256", "secret")
	webhook.ID = webhookID
	webhook.ExpiresAt = time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)

	message := model.NewMessage(http.MethodPost, "/hooks/"+webhookID, "source=test", `{"hello":"world"}`, map[string][]string{
		"X-Trace-Id":   {"trace-1"},
		"Content-Type": {"application/json"},
	})
	message.Time = time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	message.MarkRejected(http.StatusUnauthorized, "Missing basic auth credentials")

	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	mockStorage.On("GetMessagePageForWebhook", webhookID, 2, 10, model.MessageOutcomeRejected).Return(&model.MessagePage{
		Messages:        []*model.Message{message},
		Page:            2,
		PageSize:        10,
		TotalMessages:   11,
		TotalPages:      2,
		HasNextPage:     false,
		HasPreviousPage: true,
	}, nil)

	h := handler.NewHandler(mockStorage, handler.WithPublicBaseURL("https://hooks.example.com"))
	req, _ := http.NewRequest(http.MethodGet, "http://localhost/webhooks/"+webhookID+"?page=2&pageSize=10&outcome=rejected", nil)

	w := httptest.NewRecorder()
	h.WebhookPageHandler(w, req)

	require.Equal(t, http.StatusOK, w.Result().StatusCode)
	body := w.Body.String()
	assert.Contains(t, body, "Webhook "+webhookID)
	assert.Contains(t, body, "https://hooks.example.com/hooks/"+webhookID)
	assert.Contains(t, body, "https://hooks.example.com/api/webhooks/"+webhookID+"/messages")
	assert.Contains(t, body, "Basic auth (alice)")
	assert.Contains(t, body, "Header token (X-Webhook-Token)")
	assert.Contains(t, body, "HMAC SHA-256 (X-Hub-Signature-256)")
	assert.Contains(t, body, "Showing page 2 of 2 for rejected messages. Total messages: 11")
	assert.Contains(t, body, "/webhooks/"+webhookID+"?page=1&amp;pageSize=10")
	assert.Contains(t, body, "/webhooks/"+webhookID+"?page=1&amp;pageSize=10&amp;outcome=accepted")
	assert.Contains(t, body, "/webhooks/"+webhookID+"?page=1&amp;pageSize=10&amp;outcome=rejected")
	assert.Contains(t, body, "401 Unauthorized")
	assert.Contains(t, body, "Missing basic auth credentials")
	assert.Contains(t, body, "Content-Type")
	assert.Contains(t, body, "X-Trace-Id")
	assert.Contains(t, body, "2026-03-21 12:00:00 UTC")
	mockStorage.AssertExpectations(t)
}

func TestWebhookPageHandlerRejectsInvalidPagination(t *testing.T) {
	webhookID := "webhook-123"
	webhook := model.NewWebhook("", "", "", "", "", "")
	webhook.ID = webhookID

	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)

	h := handler.NewHandler(mockStorage)
	req, _ := http.NewRequest(http.MethodGet, "http://localhost/webhooks/"+webhookID+"?page=0", nil)

	w := httptest.NewRecorder()
	h.WebhookPageHandler(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
	assert.Contains(t, w.Body.String(), "page must be a positive integer")
	mockStorage.AssertExpectations(t)
}

func TestWebhookPageHandlerUsesRelativeURLsWithoutPublicBaseURL(t *testing.T) {
	webhookID := "webhook-123"
	webhook := model.NewWebhook("", "", "", "", "", "")
	webhook.ID = webhookID
	webhook.ExpiresAt = time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)

	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	mockStorage.On("GetMessagePageForWebhook", webhookID, 1, 25, model.MessageOutcomeAll).Return(&model.MessagePage{
		Messages:        []*model.Message{},
		Page:            1,
		PageSize:        25,
		TotalMessages:   0,
		TotalPages:      0,
		HasNextPage:     false,
		HasPreviousPage: false,
	}, nil)

	h := handler.NewHandler(mockStorage)
	req, _ := http.NewRequest(http.MethodGet, "http://localhost/webhooks/"+webhookID, nil)
	req.Host = "evil.example"
	req.RemoteAddr = "198.51.100.10:1234"

	w := httptest.NewRecorder()
	h.WebhookPageHandler(w, req)

	require.Equal(t, http.StatusOK, w.Result().StatusCode)
	body := w.Body.String()
	assert.Contains(t, body, "/hooks/"+webhookID)
	assert.Contains(t, body, "/api/webhooks/"+webhookID+"/messages")
	assert.NotContains(t, body, "evil.example")
	mockStorage.AssertExpectations(t)
}

func TestWebhookPageHandlerShowsDescriptiveMessageForMissingWebhook(t *testing.T) {
	webhookID := "missing-webhook"
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(nil, &storage.WebhookNotFoundError{WebhookId: webhookID})

	h := handler.NewHandler(mockStorage)
	req, _ := http.NewRequest(http.MethodGet, "http://localhost/webhooks/"+webhookID, nil)

	w := httptest.NewRecorder()
	h.WebhookPageHandler(w, req)

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
	assert.Contains(t, w.Body.String(), "Webhook with ID: missing-webhook does not exist")
	assert.Contains(t, w.Body.String(), "Create Webhook")
	mockStorage.AssertExpectations(t)
}
