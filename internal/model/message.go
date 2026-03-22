package model

import (
	"net/http"
	"strings"
	"time"
)

// MessageOutcome filters captured messages by delivery result.
type MessageOutcome string

const (
	// MessageOutcomeAll returns every captured message.
	MessageOutcomeAll MessageOutcome = "all"
	// MessageOutcomeAccepted returns only successful deliveries.
	MessageOutcomeAccepted MessageOutcome = "accepted"
	// MessageOutcomeRejected returns only rejected deliveries.
	MessageOutcomeRejected MessageOutcome = "rejected"
)

// Message represents a webhook message
type Message struct {
	Method       string              `json:"method"`
	Path         string              `json:"path"`
	Query        string              `json:"query,omitempty"`
	Payload      string              `json:"payload"`
	Headers      map[string][]string `json:"headers"`
	Time         time.Time           `json:"time"`
	StatusCode   int                 `json:"statusCode"`
	ErrorMessage string              `json:"error,omitempty"`
}

// MessagePage represents a single page of captured webhook messages.
type MessagePage struct {
	Messages        []*Message `json:"messages"`
	Page            int        `json:"page"`
	PageSize        int        `json:"pageSize"`
	TotalMessages   int        `json:"totalMessages"`
	TotalPages      int        `json:"totalPages"`
	HasNextPage     bool       `json:"hasNextPage"`
	HasPreviousPage bool       `json:"hasPreviousPage"`
}

// ParseMessageOutcome validates and normalizes the requested outcome filter.
func ParseMessageOutcome(value string) (MessageOutcome, bool) {
	switch MessageOutcome(strings.ToLower(strings.TrimSpace(value))) {
	case "", MessageOutcomeAll:
		return MessageOutcomeAll, true
	case MessageOutcomeAccepted:
		return MessageOutcomeAccepted, true
	case MessageOutcomeRejected:
		return MessageOutcomeRejected, true
	default:
		return "", false
	}
}

// NewMessage creates a new message with the current timestamp.
func NewMessage(method string, path string, query string, payload string, headers map[string][]string) *Message {
	return &Message{
		Method:     method,
		Path:       path,
		Query:      query,
		Payload:    payload,
		Headers:    headers,
		Time:       time.Now().UTC(),
		StatusCode: http.StatusOK,
	}
}

// MarkRejected stores the receiver response for a rejected delivery attempt.
func (m *Message) MarkRejected(statusCode int, errorMessage string) {
	m.StatusCode = statusCode
	m.ErrorMessage = errorMessage
}

// Rejected indicates whether the captured delivery was rejected by the receiver.
func (m *Message) Rejected() bool {
	return m.StatusCode >= http.StatusBadRequest || m.ErrorMessage != ""
}
