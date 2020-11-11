package model_test

import (
	"net/http"
	"testing"

	"github.com/achawki/webhook-receiver/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestNewWebhookFromInput(t *testing.T) {
	username := "username"
	password := "password"
	token := "token"
	tokenValue := "tokenValue"
	webhookInput := &model.WebhookInput{Username: username, Password: password, TokenName: token, TokenValue: tokenValue}
	webhook := model.NewWebhookFromInput(webhookInput)

	assert.Equal(t, username, webhook.Username)
	assert.Equal(t, token, webhook.TokenName)
}

func TestValidateWithEmptyWebhook(t *testing.T) {
	webhook := model.NewWebhook("", "", "", "")
	err := webhook.Validate()

	assert.Nil(t, err)
}

func TestValidateWithMissingPassword(t *testing.T) {
	webhook := model.NewWebhook("username", "", "", "")
	err := webhook.Validate()

	assert.NotNil(t, err)
}

func TestValidateWithMissingUsername(t *testing.T) {
	webhook := model.NewWebhook("", "password", "", "")
	err := webhook.Validate()

	assert.NotNil(t, err)
}

func TestValidateWithMissingTokenName(t *testing.T) {
	webhook := model.NewWebhook("", "", "token", "")
	err := webhook.Validate()

	assert.NotNil(t, err)
}

func TestValidateWithMissingTokenValue(t *testing.T) {
	webhook := model.NewWebhook("", "", "", "tokenVal")
	err := webhook.Validate()

	assert.NotNil(t, err)
}

func TestValidateAuthorizationWithBasicAuth(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.SetBasicAuth("username", "password")
	result := webhook.ValidateAuthorization(request)

	assert.True(t, result)
}

func TestValidateAuthorizationWithIncorrectPassword(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.SetBasicAuth("username", "incorrect")
	result := webhook.ValidateAuthorization(request)

	assert.False(t, result)
}

func TestValidateAuthorizationWithIncorrectUsername(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.SetBasicAuth("incorrect", "password")
	result := webhook.ValidateAuthorization(request)

	assert.False(t, result)
}

func TestValidateAuthorizationWithoutBasicAuthSet(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	result := webhook.ValidateAuthorization(request)

	assert.False(t, result)
}

func TestValidateAuthorizationWithoutAuthorization(t *testing.T) {
	webhook := model.NewWebhook("", "", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	result := webhook.ValidateAuthorization(request)

	assert.True(t, result)
}

func TestValidateAuthorizationWithToken(t *testing.T) {
	webhook := model.NewWebhook("", "", "tokenName", "tokenValue")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.Header.Add("tokenName", "tokenValue")
	result := webhook.ValidateAuthorization(request)

	assert.True(t, result)
}

func TestValidateAuthorizationTokenWithoutHeader(t *testing.T) {
	webhook := model.NewWebhook("", "", "tokenName", "tokenValue")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	result := webhook.ValidateAuthorization(request)

	assert.False(t, result)
}

func TestValidateAuthorizationTokenWithWrongValue(t *testing.T) {
	webhook := model.NewWebhook("", "", "tokenName", "tokenValue")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.Header.Add("tokenName", "incorrect")
	result := webhook.ValidateAuthorization(request)

	assert.False(t, result)
}
