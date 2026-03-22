package handler_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/achawki/webhook-receiver/internal/handler"
	"github.com/achawki/webhook-receiver/internal/model"
	"github.com/achawki/webhook-receiver/internal/storage"
	"github.com/achawki/webhook-receiver/internal/storage/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMessageHandlerWithUnsupportedHTTPMethod(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPatch, "http://localhost/api/webhooks/id/messages", nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)
	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}

func TestMessageHandlerWithWrongPath(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodGet, "http://localhost/api/webhooks/id/messages/wrong", nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)
	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}

func TestMessageHandlerWithoutMessagePath(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodGet, "http://localhost/api/webhooks/id/", nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)
	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}

func TestMessageHandlerWithMissingID(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodGet, "http://localhost/api/webhooks/messages", nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)
	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}

func TestMessageHandlerWithUnknownWebhook(t *testing.T) {
	webhookID := "webhookID"
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(nil, &storage.WebhookNotFoundError{WebhookId: webhookID})
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost/api/webhooks/%s/messages", webhookID), nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
	assert.JSONEq(t, `{"message":"Webhook with ID: webhookID does not exist"}`, w.Body.String())
}

func TestMessageHandlerInternalServerError(t *testing.T) {
	webhookID := "webhookID"
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(nil, errors.New("Database Error"))
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost/api/webhooks/%s/messages", webhookID), nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
}

func TestMessageHandlerGETMessagesInternalServerError(t *testing.T) {
	webhookID := "webhookID"
	webhook := &model.Webhook{ID: webhookID}
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	mockStorage.On("GetMessagePageForWebhook", webhookID, 1, 25, model.MessageOutcomeAll).Return(nil, errors.New("Database Error"))
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost/api/webhooks/%s/messages", webhookID), nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
}

func TestMessageHandlerGETMessages(t *testing.T) {
	webhookID := "webhookID"
	webhook := &model.Webhook{ID: webhookID, ExpiresAt: time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)}
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
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost/api/webhooks/%s/messages", webhookID), nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	var response struct {
		WebhookID       string           `json:"webhookId"`
		ExpiresAt       string           `json:"expiresAt"`
		Outcome         string           `json:"outcome"`
		Messages        []*model.Message `json:"messages"`
		Page            int              `json:"page"`
		PageSize        int              `json:"pageSize"`
		TotalMessages   int              `json:"totalMessages"`
		TotalPages      int              `json:"totalPages"`
		HasNextPage     bool             `json:"hasNextPage"`
		HasPreviousPage bool             `json:"hasPreviousPage"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, webhookID, response.WebhookID)
	assert.Equal(t, "2026-03-23T12:00:00Z", response.ExpiresAt)
	assert.Equal(t, "all", response.Outcome)
	assert.Empty(t, response.Messages)
	assert.Equal(t, 1, response.Page)
	assert.Equal(t, 25, response.PageSize)
}

func TestMessageHandlerGETMessagesWithPagination(t *testing.T) {
	webhookID := "webhookID"
	webhook := &model.Webhook{ID: webhookID, ExpiresAt: time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)}
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	mockStorage.On("GetMessagePageForWebhook", webhookID, 2, 10, model.MessageOutcomeAll).Return(&model.MessagePage{
		Messages: []*model.Message{
			{Method: http.MethodPost, Path: "/hooks/" + webhookID, Payload: `{"hello":"world"}`, StatusCode: http.StatusUnauthorized, ErrorMessage: "Missing basic auth credentials"},
		},
		Page:            2,
		PageSize:        10,
		TotalMessages:   11,
		TotalPages:      2,
		HasNextPage:     false,
		HasPreviousPage: true,
	}, nil)
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost/api/webhooks/%s/messages?page=2&pageSize=10", webhookID), nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Contains(t, w.Body.String(), `"page":2`)
	assert.Contains(t, w.Body.String(), `"pageSize":10`)
	assert.Contains(t, w.Body.String(), `"totalMessages":11`)
	assert.Contains(t, w.Body.String(), `"outcome":"all"`)
	assert.Contains(t, w.Body.String(), `"statusCode":401`)
	assert.Contains(t, w.Body.String(), `"error":"Missing basic auth credentials"`)
}

func TestMessageHandlerGETMessagesWithOutcomeFilter(t *testing.T) {
	webhookID := "webhookID"
	webhook := &model.Webhook{ID: webhookID, ExpiresAt: time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)}
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	mockStorage.On("GetMessagePageForWebhook", webhookID, 1, 25, model.MessageOutcomeRejected).Return(&model.MessagePage{
		Messages: []*model.Message{
			{Method: http.MethodPost, Path: "/hooks/" + webhookID, Payload: `{"hello":"world"}`, StatusCode: http.StatusUnauthorized, ErrorMessage: "Missing basic auth credentials"},
		},
		Page:            1,
		PageSize:        25,
		TotalMessages:   1,
		TotalPages:      1,
		HasNextPage:     false,
		HasPreviousPage: false,
	}, nil)
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost/api/webhooks/%s/messages?outcome=rejected", webhookID), nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Contains(t, w.Body.String(), `"outcome":"rejected"`)
}

func TestMessageHandlerGETMessagesRejectsInvalidPagination(t *testing.T) {
	webhookID := "webhookID"
	webhook := &model.Webhook{ID: webhookID}
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost/api/webhooks/%s/messages?page=0", webhookID), nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
	assert.JSONEq(t, `{"message":"page must be a positive integer"}`, w.Body.String())
}

func TestMessageHandlerGETMessagesRejectsInvalidOutcome(t *testing.T) {
	webhookID := "webhookID"
	webhook := &model.Webhook{ID: webhookID}
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost/api/webhooks/%s/messages?outcome=broken", webhookID), nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
	assert.JSONEq(t, `{"message":"outcome must be one of all, accepted, rejected"}`, w.Body.String())
}

func TestMessageHandlerRejectsPOST(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/api/webhooks/id/messages", bytes.NewBuffer([]byte("{}")))

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}

func TestHookHandlerWithHMAC(t *testing.T) {
	webhookID := "webhookID"
	body := []byte(`{"hello":"world"}`)
	webhook := model.NewWebhookFromInput(&model.WebhookInput{HMACHeader: "X-Hub-Signature-256", HMACSecret: "secret"})
	webhook.ID = webhookID
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	mockStorage.On("InsertMessage", webhookID, mock.MatchedBy(func(message *model.Message) bool {
		return message.StatusCode == http.StatusOK &&
			message.ErrorMessage == "" &&
			message.Payload == string(body)
	})).Return(nil)
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("http://localhost/hooks/%s", webhookID), bytes.NewBuffer(body))
	request.Header.Add("X-Hub-Signature-256", signBody(body, "secret"))

	w := httptest.NewRecorder()
	handler.HookHandler(w, request)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}

func TestHookHandlerStoresAuthorizationFailure(t *testing.T) {
	webhookID := "webhookID"
	body := []byte(`{"hello":"world"}`)
	webhook := model.NewWebhookFromInput(&model.WebhookInput{Username: "username", Password: "password"})
	webhook.ID = webhookID
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	mockStorage.On("InsertMessage", webhookID, mock.MatchedBy(func(message *model.Message) bool {
		_, hasAuthorizationHeader := message.Headers["Authorization"]
		return message.StatusCode == http.StatusUnauthorized &&
			message.ErrorMessage == "Missing basic auth credentials" &&
			message.Method == http.MethodPost &&
			message.Path == "/hooks/"+webhookID &&
			message.Payload == string(body) &&
			!hasAuthorizationHeader
	})).Return(nil)
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost/hooks/%s", webhookID), bytes.NewBuffer(body))

	w := httptest.NewRecorder()
	handler.HookHandler(w, request)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)
	assert.JSONEq(t, `{"message":"Request did not satisfy the configured webhook authorization"}`, w.Body.String())
	mockStorage.AssertExpectations(t)
}

func TestHookHandlerHidesInfrastructureHeadersFromStoredMessages(t *testing.T) {
	webhookID := "webhookID"
	body := []byte(`{"hello":"world"}`)
	webhook := model.NewWebhookFromInput(&model.WebhookInput{})
	webhook.ID = webhookID
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	mockStorage.On("InsertMessage", webhookID, mock.MatchedBy(func(message *model.Message) bool {
		_, hasFlyRegion := message.Headers["Fly-Region"]
		_, hasForwardedFor := message.Headers["X-Forwarded-For"]
		_, hasVia := message.Headers["Via"]
		_, hasRequestStart := message.Headers["X-Request-Start"]
		userAgentValues, hasUserAgent := message.Headers["User-Agent"]

		return message.StatusCode == http.StatusOK &&
			!hasFlyRegion &&
			!hasForwardedFor &&
			!hasVia &&
			!hasRequestStart &&
			hasUserAgent &&
			assert.ObjectsAreEqual([]string{"integration-test"}, userAgentValues)
	})).Return(nil)
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost/hooks/%s", webhookID), bytes.NewBuffer(body))
	request.Header.Set("Fly-Region", "ams")
	request.Header.Set("X-Forwarded-For", "203.0.113.10")
	request.Header.Set("Via", "1.1 fly.io")
	request.Header.Set("X-Request-Start", "123456789")
	request.Header.Set("User-Agent", "integration-test")

	w := httptest.NewRecorder()
	handler.HookHandler(w, request)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	mockStorage.AssertExpectations(t)
}

func TestHomeHandlerRendersUI(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodGet, "http://localhost/", nil)

	w := httptest.NewRecorder()
	handler.HomeHandler(w, request)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Contains(t, w.Body.String(), "Create Webhook")
	assert.Contains(t, w.Body.String(), "48 hours")
	assert.Contains(t, w.Body.String(), "newest 100 captured requests")
}

func TestHomeHandlerRateLimitsByIP(t *testing.T) {
	handler := handler.NewHandler(nil, handler.WithRateLimit(1, time.Hour))
	request, _ := http.NewRequest(http.MethodGet, "http://localhost/", nil)

	first := httptest.NewRecorder()
	handler.HomeHandler(first, request)
	assert.Equal(t, http.StatusOK, first.Result().StatusCode)

	second := httptest.NewRecorder()
	handler.HomeHandler(second, request)
	assert.Equal(t, http.StatusTooManyRequests, second.Result().StatusCode)
}

func TestHomeHandlerIgnoresClientIPHeaderByDefault(t *testing.T) {
	handler := handler.NewHandler(nil, handler.WithRateLimit(1, time.Hour))

	firstRequest, _ := http.NewRequest(http.MethodGet, "http://localhost/", nil)
	firstRequest.RemoteAddr = "198.51.100.10:1234"
	firstRequest.Header.Set("Fly-Client-IP", "203.0.113.10")

	secondRequest, _ := http.NewRequest(http.MethodGet, "http://localhost/", nil)
	secondRequest.RemoteAddr = "198.51.100.10:4321"
	secondRequest.Header.Set("Fly-Client-IP", "203.0.113.11")

	first := httptest.NewRecorder()
	handler.HomeHandler(first, firstRequest)
	assert.Equal(t, http.StatusOK, first.Result().StatusCode)

	second := httptest.NewRecorder()
	handler.HomeHandler(second, secondRequest)
	assert.Equal(t, http.StatusTooManyRequests, second.Result().StatusCode)
}

func TestHomeHandlerUsesConfiguredClientIPHeader(t *testing.T) {
	handler := handler.NewHandler(
		nil,
		handler.WithRateLimit(1, time.Hour),
		handler.WithClientIPHeader("Fly-Client-IP"),
	)

	firstRequest, _ := http.NewRequest(http.MethodGet, "http://localhost/", nil)
	firstRequest.RemoteAddr = "127.0.0.1:1234"
	firstRequest.Header.Set("Fly-Client-IP", "203.0.113.10")

	secondRequest, _ := http.NewRequest(http.MethodGet, "http://localhost/", nil)
	secondRequest.RemoteAddr = "127.0.0.1:4321"
	secondRequest.Header.Set("Fly-Client-IP", "203.0.113.11")

	first := httptest.NewRecorder()
	handler.HomeHandler(first, firstRequest)
	assert.Equal(t, http.StatusOK, first.Result().StatusCode)

	second := httptest.NewRecorder()
	handler.HomeHandler(second, secondRequest)
	assert.Equal(t, http.StatusOK, second.Result().StatusCode)
}

func TestHomeHandlerFallsBackToRemoteAddrWhenConfiguredHeaderMissing(t *testing.T) {
	handler := handler.NewHandler(
		nil,
		handler.WithRateLimit(1, time.Hour),
		handler.WithClientIPHeader("Fly-Client-IP"),
	)

	firstRequest, _ := http.NewRequest(http.MethodGet, "http://localhost/", nil)
	firstRequest.RemoteAddr = "198.51.100.10:1234"

	secondRequest, _ := http.NewRequest(http.MethodGet, "http://localhost/", nil)
	secondRequest.RemoteAddr = "198.51.100.10:4321"

	first := httptest.NewRecorder()
	handler.HomeHandler(first, firstRequest)
	assert.Equal(t, http.StatusOK, first.Result().StatusCode)

	second := httptest.NewRecorder()
	handler.HomeHandler(second, secondRequest)
	assert.Equal(t, http.StatusTooManyRequests, second.Result().StatusCode)
}

func signBody(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)

	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
