package handler_test

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/achawki/webhook-receiver/internal/handler"
	"github.com/achawki/webhook-receiver/internal/storage/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestWebhookHandlerWithUnsupportedHTTPMethod(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPatch, "localhost/api/webhooks", nil)

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}

func TestWebhookHandlerWithInvalidInput(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPost, "localhost/api/webhooks", bytes.NewBuffer([]byte("invalid")))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestWebhookHandlerWithInvalidJson(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPost, "localhost/api/webhooks", bytes.NewBuffer([]byte(`{"invalid: "not valid"}`)))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestWebhookHandlerWithIncompleteJson(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPost, "localhost/api/webhooks", bytes.NewBuffer([]byte(`{"incomplete": []`)))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestWebhookHandlerWithUnknownField(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPost, "localhost/api/webhooks", bytes.NewBuffer([]byte(`{"unknown": "unknown"}`)))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	reponseBody, _ := ioutil.ReadAll(w.Body)

	assert.Equal(t, "{\"message\": \"json: unknown field \"unknown\"\"}", string(reponseBody))
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestWebhookHandlerWithWrongInputType(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPost, "localhost/api/webhooks", bytes.NewBuffer([]byte(`{"username": []}`)))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestWebhookHandlerWithValidationFailing(t *testing.T) {
	handler := handler.NewHandler(nil)
	request, _ := http.NewRequest(http.MethodPost, "localhost/api/webhooks", bytes.NewBuffer([]byte(`{"username": "username"}`)))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Result().StatusCode)
}

func TestWebhookHandlerWithValidInput(t *testing.T) {
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("InsertWebhook", mock.Anything).Return("id", nil)
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodPost, "localhost/api/webhooks", bytes.NewBuffer([]byte(`{}`)))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)
	reponseBody, _ := ioutil.ReadAll(w.Body)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "{\"id\":\"id\"}\n", string(reponseBody))
}

func TestWebhookHandlerWithDatabaseError(t *testing.T) {
	mockStorage := new(mocks.WebhookStorage)
	mockStorage.On("InsertWebhook", mock.Anything).Return("", errors.New("Database error"))
	handler := handler.NewHandler(mockStorage)
	request, _ := http.NewRequest(http.MethodPost, "localhost/api/webhooks", bytes.NewBuffer([]byte(`{}`)))

	w := httptest.NewRecorder()
	handler.WebhookHandler(w, request)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
}
