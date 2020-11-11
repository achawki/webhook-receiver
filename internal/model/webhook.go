package model

import (
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// WebhookInput used for unmarshaling user input
type WebhookInput struct {
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	TokenName  string `json:"tokenName,omitempty"`
	TokenValue string `json:"tokenValue,omitempty"`
}

// Webhook is used after input has been unmarshaled and will be provided to the user as well
type Webhook struct {
	Username   string `json:"username,omitempty"`
	password   string
	TokenName  string `json:"tokenName,omitempty"`
	tokenValue string
	ID         string `json:"id"`
}

// NewWebhookFromInput creates Webhook instance based on input
func NewWebhookFromInput(webhookInput *WebhookInput) *Webhook {
	return NewWebhook(webhookInput.Username, webhookInput.Password, webhookInput.TokenName, webhookInput.TokenValue)
}

// NewWebhook creates webhook based on input
func NewWebhook(username string, password string, tokenName string, tokenValue string) *Webhook {
	webhook := &Webhook{Username: strings.TrimSpace(username), TokenName: strings.TrimSpace(tokenName)}

	if password != "" {
		webhook.password = hashPassword(password)
	}

	if tokenValue != "" {
		webhook.tokenValue = hashPassword(tokenValue)
	}

	return webhook
}

// Validate validates whether token and/or username are correctly provided
func (w *Webhook) Validate() error {
	if (w.Username != "" && w.password == "") || (w.Username == "" && w.password != "") {
		return errors.New("username and password must be both set or both empty")
	}

	if (w.TokenName != "" && w.tokenValue == "") || (w.TokenName == "" && w.tokenValue != "") {
		return errors.New("token name and value must be both set or both empty")
	}

	return nil
}

// ValidateAuthorization validates authorization based on provided request
func (w *Webhook) ValidateAuthorization(r *http.Request) bool {
	if w.password != "" {
		user, password, ok := r.BasicAuth()
		if !ok {
			return false
		}
		if w.Username != user {
			return false
		}
		err := bcrypt.CompareHashAndPassword([]byte(w.password), []byte(password))
		return err == nil
	}

	if w.tokenValue != "" {
		tokenValue := r.Header.Get(w.TokenName)
		if tokenValue == "" {
			return false
		}
		err := bcrypt.CompareHashAndPassword([]byte(w.tokenValue), []byte(tokenValue))
		return err == nil
	}

	return true
}

func hashPassword(password string) string {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return ""
	}
	return string(hashedPassword)
}
