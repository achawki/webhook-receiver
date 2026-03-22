package model

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// WebhookInput is used for unmarshaling user input.
type WebhookInput struct {
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	TokenName  string `json:"tokenName,omitempty"`
	TokenValue string `json:"tokenValue,omitempty"`
	HMACHeader string `json:"hmacHeader,omitempty"`
	HMACSecret string `json:"hmacSecret,omitempty"`
}

// Webhook is the validated runtime representation of a configured receiver.
type Webhook struct {
	Username   string `json:"username,omitempty"`
	password   string
	TokenName  string `json:"tokenName,omitempty"`
	tokenValue string
	HMACHeader string `json:"hmacHeader,omitempty"`
	hmacSecret string
	ID         string    `json:"id"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// NewWebhookFromInput creates Webhook instance based on input
func NewWebhookFromInput(webhookInput *WebhookInput) *Webhook {
	if webhookInput == nil {
		webhookInput = &WebhookInput{}
	}

	return NewWebhook(
		webhookInput.Username,
		webhookInput.Password,
		webhookInput.TokenName,
		webhookInput.TokenValue,
		webhookInput.HMACHeader,
		webhookInput.HMACSecret,
	)
}

// NewWebhook creates webhook based on input
func NewWebhook(username string, password string, tokenName string, tokenValue string, hmacHeader string, hmacSecret string) *Webhook {
	webhook := &Webhook{
		Username:   strings.TrimSpace(username),
		TokenName:  strings.TrimSpace(tokenName),
		HMACHeader: strings.TrimSpace(hmacHeader),
	}

	if password != "" {
		webhook.password = hashPassword(password)
	}

	if tokenValue != "" {
		webhook.tokenValue = hashPassword(tokenValue)
	}

	if hmacSecret != "" {
		webhook.hmacSecret = hmacSecret
	}

	return webhook
}

// NewStoredWebhook reconstructs a webhook from persisted storage values.
func NewStoredWebhook(id string, username string, passwordHash string, tokenName string, tokenValueHash string, hmacHeader string, hmacSecret string, expiresAt time.Time) *Webhook {
	return &Webhook{
		Username:   strings.TrimSpace(username),
		password:   passwordHash,
		TokenName:  strings.TrimSpace(tokenName),
		tokenValue: tokenValueHash,
		HMACHeader: strings.TrimSpace(hmacHeader),
		hmacSecret: hmacSecret,
		ID:         id,
		ExpiresAt:  expiresAt.UTC(),
	}
}

// Validate validates whether token and/or username are correctly provided
func (w *Webhook) Validate() error {
	if (w.Username != "" && w.password == "") || (w.Username == "" && w.password != "") {
		return errors.New("username and password must be both set or both empty")
	}

	if (w.TokenName != "" && w.tokenValue == "") || (w.TokenName == "" && w.tokenValue != "") {
		return errors.New("token name and value must be both set or both empty")
	}

	if (w.HMACHeader != "" && w.hmacSecret == "") || (w.HMACHeader == "" && w.hmacSecret != "") {
		return errors.New("hmac header and secret must be both set or both empty")
	}

	return nil
}

// HasBasicAuth indicates whether basic auth is configured for the webhook.
func (w *Webhook) HasBasicAuth() bool {
	return w.Username != "" && w.password != ""
}

// HasHeaderToken indicates whether header token validation is configured.
func (w *Webhook) HasHeaderToken() bool {
	return w.TokenName != "" && w.tokenValue != ""
}

// HasHMAC indicates whether HMAC validation is configured.
func (w *Webhook) HasHMAC() bool {
	return w.HMACHeader != "" && w.hmacSecret != ""
}

// PasswordHash returns the stored password hash.
func (w *Webhook) PasswordHash() string {
	return w.password
}

// TokenValueHash returns the stored header-token hash.
func (w *Webhook) TokenValueHash() string {
	return w.tokenValue
}

// HMACSecret returns the configured HMAC secret.
func (w *Webhook) HMACSecret() string {
	return w.hmacSecret
}

// ValidateAuthorization validates authorization based on provided request
func (w *Webhook) ValidateAuthorization(r *http.Request, body []byte) bool {
	return w.AuthorizationFailure(r, body) == ""
}

// AuthorizationFailure returns a human-readable authorization failure or an empty string on success.
func (w *Webhook) AuthorizationFailure(r *http.Request, body []byte) string {
	if failure := w.readAuthorizationFailure(r); failure != "" {
		return failure
	}

	if w.HasHMAC() {
		signature := r.Header.Get(w.HMACHeader)
		if signature == "" {
			return fmt.Sprintf("Missing HMAC signature header %q", w.HMACHeader)
		}
		if !validateHMAC(body, w.hmacSecret, signature) {
			return fmt.Sprintf("HMAC signature in %q did not match", w.HMACHeader)
		}
	}

	return ""
}

// ValidateReadAuthorization validates the auth schemes that can sensibly protect reads.
func (w *Webhook) ValidateReadAuthorization(r *http.Request) bool {
	return w.readAuthorizationFailure(r) == ""
}

func (w *Webhook) readAuthorizationFailure(r *http.Request) string {
	if w.HasBasicAuth() {
		user, password, ok := r.BasicAuth()
		if !ok {
			return "Missing basic auth credentials"
		}
		if w.Username != user {
			return "Basic auth username did not match"
		}
		err := bcrypt.CompareHashAndPassword([]byte(w.password), []byte(password))
		if err != nil {
			return "Basic auth password did not match"
		}
	}

	if w.HasHeaderToken() {
		tokenValue := r.Header.Get(w.TokenName)
		if tokenValue == "" {
			return fmt.Sprintf("Missing required header token %q", w.TokenName)
		}
		err := bcrypt.CompareHashAndPassword([]byte(w.tokenValue), []byte(tokenValue))
		if err != nil {
			return fmt.Sprintf("Header token %q did not match", w.TokenName)
		}
	}

	return ""
}

func validateHMAC(body []byte, secret string, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))
	normalizedSignature := strings.TrimSpace(strings.ToLower(signature))
	normalizedSignature = strings.TrimPrefix(normalizedSignature, "sha256=")

	return hmac.Equal([]byte(normalizedSignature), []byte(expectedSignature))
}

func hashPassword(password string) string {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return ""
	}
	return string(hashedPassword)
}
