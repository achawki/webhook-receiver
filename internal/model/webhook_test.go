package model_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	hmacHeader := "X-Hub-Signature-256"
	hmacSecret := "super-secret"
	webhookInput := &model.WebhookInput{
		Username:   username,
		Password:   password,
		TokenName:  token,
		TokenValue: tokenValue,
		HMACHeader: hmacHeader,
		HMACSecret: hmacSecret,
	}
	webhook := model.NewWebhookFromInput(webhookInput)

	assert.Equal(t, username, webhook.Username)
	assert.Equal(t, token, webhook.TokenName)
	assert.Equal(t, hmacHeader, webhook.HMACHeader)
}

func TestValidateWithEmptyWebhook(t *testing.T) {
	webhook := model.NewWebhook("", "", "", "", "", "")
	err := webhook.Validate()

	assert.Nil(t, err)
}

func TestValidateWithMissingPassword(t *testing.T) {
	webhook := model.NewWebhook("username", "", "", "", "", "")
	err := webhook.Validate()

	assert.NotNil(t, err)
}

func TestValidateWithMissingUsername(t *testing.T) {
	webhook := model.NewWebhook("", "password", "", "", "", "")
	err := webhook.Validate()

	assert.NotNil(t, err)
}

func TestValidateWithMissingTokenName(t *testing.T) {
	webhook := model.NewWebhook("", "", "token", "", "", "")
	err := webhook.Validate()

	assert.NotNil(t, err)
}

func TestValidateWithMissingTokenValue(t *testing.T) {
	webhook := model.NewWebhook("", "", "", "tokenVal", "", "")
	err := webhook.Validate()

	assert.NotNil(t, err)
}

func TestValidateWithMissingHMACHeader(t *testing.T) {
	webhook := model.NewWebhook("", "", "", "", "", "secret")
	err := webhook.Validate()

	assert.NotNil(t, err)
}

func TestValidateWithMissingHMACSecret(t *testing.T) {
	webhook := model.NewWebhook("", "", "", "", "X-Hub-Signature-256", "")
	err := webhook.Validate()

	assert.NotNil(t, err)
}

func TestValidateAuthorizationWithBasicAuth(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "", "", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.SetBasicAuth("username", "password")
	result := webhook.ValidateAuthorization(request, nil)

	assert.True(t, result)
}

func TestValidateAuthorizationWithIncorrectPassword(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "", "", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.SetBasicAuth("username", "incorrect")
	result := webhook.ValidateAuthorization(request, nil)

	assert.False(t, result)
}

func TestValidateAuthorizationWithIncorrectUsername(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "", "", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.SetBasicAuth("incorrect", "password")
	result := webhook.ValidateAuthorization(request, nil)

	assert.False(t, result)
}

func TestValidateAuthorizationWithoutBasicAuthSet(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "", "", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	result := webhook.ValidateAuthorization(request, nil)

	assert.False(t, result)
}

func TestValidateAuthorizationWithoutAuthorization(t *testing.T) {
	webhook := model.NewWebhook("", "", "", "", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	result := webhook.ValidateAuthorization(request, nil)

	assert.True(t, result)
}

func TestValidateAuthorizationWithToken(t *testing.T) {
	webhook := model.NewWebhook("", "", "tokenName", "tokenValue", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.Header.Add("tokenName", "tokenValue")
	result := webhook.ValidateAuthorization(request, nil)

	assert.True(t, result)
}

func TestValidateAuthorizationTokenWithoutHeader(t *testing.T) {
	webhook := model.NewWebhook("", "", "tokenName", "tokenValue", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	result := webhook.ValidateAuthorization(request, nil)

	assert.False(t, result)
}

func TestValidateAuthorizationTokenWithWrongValue(t *testing.T) {
	webhook := model.NewWebhook("", "", "tokenName", "tokenValue", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.Header.Add("tokenName", "incorrect")
	result := webhook.ValidateAuthorization(request, nil)

	assert.False(t, result)
}

func TestValidateAuthorizationWithBasicAuthAndTokenRequiresBoth(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "X-Webhook-Token", "token-value", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.SetBasicAuth("username", "password")
	request.Header.Add("X-Webhook-Token", "token-value")

	result := webhook.ValidateAuthorization(request, nil)

	assert.True(t, result)
}

func TestValidateAuthorizationWithBasicAuthAndTokenFailsIfTokenMissing(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "X-Webhook-Token", "token-value", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.SetBasicAuth("username", "password")

	result := webhook.ValidateAuthorization(request, nil)

	assert.False(t, result)
}

func TestValidateAuthorizationWithHMAC(t *testing.T) {
	body := []byte(`{"event":"delivered"}`)
	webhook := model.NewWebhook("", "", "", "", "X-Hub-Signature-256", "secret")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.Header.Add("X-Hub-Signature-256", signedBody(body, "secret"))

	result := webhook.ValidateAuthorization(request, body)

	assert.True(t, result)
}

func TestValidateAuthorizationWithIncorrectHMAC(t *testing.T) {
	body := []byte(`{"event":"delivered"}`)
	webhook := model.NewWebhook("", "", "", "", "X-Hub-Signature-256", "secret")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.Header.Add("X-Hub-Signature-256", signedBody(body, "wrong"))

	result := webhook.ValidateAuthorization(request, body)

	assert.False(t, result)
}

func TestAuthorizationFailureWithMissingBasicAuth(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "", "", "", "")
	request, _ := http.NewRequest(http.MethodPost, "", nil)

	failure := webhook.AuthorizationFailure(request, nil)

	assert.Equal(t, "Missing basic auth credentials", failure)
}

func TestAuthorizationFailureWithIncorrectHMAC(t *testing.T) {
	body := []byte(`{"event":"delivered"}`)
	webhook := model.NewWebhook("", "", "", "", "X-Hub-Signature-256", "secret")
	request, _ := http.NewRequest(http.MethodPost, "", nil)
	request.Header.Add("X-Hub-Signature-256", signedBody(body, "wrong"))

	failure := webhook.AuthorizationFailure(request, body)

	assert.Equal(t, `HMAC signature in "X-Hub-Signature-256" did not match`, failure)
}

func TestValidateReadAuthorizationWithBasicAuthAndHeaderToken(t *testing.T) {
	webhook := model.NewWebhook("username", "password", "X-Webhook-Token", "token", "", "")
	request, _ := http.NewRequest(http.MethodGet, "", nil)
	request.SetBasicAuth("username", "password")
	request.Header.Set("X-Webhook-Token", "token")

	assert.True(t, webhook.ValidateReadAuthorization(request))
}

func TestValidateReadAuthorizationIgnoresHMACOnlyConfiguration(t *testing.T) {
	webhook := model.NewWebhook("", "", "", "", "X-Hub-Signature-256", "secret")
	request, _ := http.NewRequest(http.MethodGet, "", nil)

	assert.True(t, webhook.ValidateReadAuthorization(request))
}

func signedBody(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)

	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
