package handler_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

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
	reponseBody, _ := ioutil.ReadAll(w.Body)

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
	assert.Equal(t, "{\"message\": \"Webhook with ID: webhookID does not exist\"}", string(reponseBody))
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
	mockStorage.On("GetMessagesForWebhook", webhookID).Return(nil, errors.New("Database Error"))
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost/api/webhooks/%s/messages", webhookID), nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
}

func TestMessageHandlerGETMessages(t *testing.T) {
	webhookID := "webhookID"
	webhook := &model.Webhook{ID: webhookID}
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	mockStorage.On("GetMessagesForWebhook", webhookID).Return([]*model.Message{}, nil)
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost/api/webhooks/%s/messages", webhookID), nil)

	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)
	reponseBody, _ := ioutil.ReadAll(w.Body)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "[]\n", string(reponseBody))
}

func TestMessageHandlerPOSTMessagesUnauthorized(t *testing.T) {
	webhookID := "webhookID"
	webhook := model.NewWebhookFromInput(&model.WebhookInput{Username: "username", Password: "password"})
	webhook.ID = webhookID
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost/api/webhooks/%s/messages", webhookID), bytes.NewBuffer([]byte("{}")))
	request.SetBasicAuth("username", "wrong password")
	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)
}

func TestMessageHandlerPOSTMessagesWithBasicAuth(t *testing.T) {
	webhookID := "webhookID"
	webhook := model.NewWebhookFromInput(&model.WebhookInput{Username: "username", Password: "password"})
	webhook.ID = webhookID
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	mockStorage.On("InsertMessage", webhookID, mock.Anything).Return(nil)
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost/api/webhooks/%s/messages", webhookID), bytes.NewBuffer([]byte("{}")))
	request.SetBasicAuth("username", "password")
	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}

func TestMessageHandlerPOSTMessagesWithInternalServerErrorOnInsert(t *testing.T) {
	webhookID := "webhookID"
	webhook := model.NewWebhookFromInput(&model.WebhookInput{})
	webhook.ID = webhookID
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	mockStorage.On("InsertMessage", webhookID, mock.Anything).Return(errors.New("Database Error"))
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost/api/webhooks/%s/messages", webhookID), bytes.NewBuffer([]byte("{}")))
	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
}

func TestMessageHandlerPOSTMessagesTokenHeaderRemoved(t *testing.T) {
	webhookID := "webhookID"
	webhook := model.NewWebhookFromInput(&model.WebhookInput{TokenName: "token", TokenValue: "value"})
	webhook.ID = webhookID
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("GetWebhook", webhookID).Return(webhook, nil)
	mockStorage.On("InsertMessage", webhookID, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		message := args.Get(1).(*model.Message)
		assert.Len(t, message.Headers, 0)
	})

	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost/api/webhooks/%s/messages", webhookID), bytes.NewBuffer([]byte("{}")))
	request.Header.Add("token", "value")
	w := httptest.NewRecorder()
	handler.MessageHandler(w, request)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}
