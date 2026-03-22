package model_test

import (
	"net/http"
	"testing"

	"github.com/achawki/webhook-receiver/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestMessageRejected(t *testing.T) {
	message := model.NewMessage(http.MethodPost, "/hooks/id", "", `{"hello":"world"}`, nil)
	assert.False(t, message.Rejected())

	message.MarkRejected(http.StatusUnauthorized, "Missing basic auth credentials")
	assert.True(t, message.Rejected())
}
