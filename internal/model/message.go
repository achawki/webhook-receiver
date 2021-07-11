package model

import "time"

// Message represents a webhook message
type Message struct {
	Payload string              `json:"payload"`
	Headers map[string][]string `json:"headers"`
	Time    time.Time           `json:"time"`
}

// NewMessage creates a new messages with current timestamp
func NewMessage(payload string, headers map[string][]string) *Message {
	return &Message{Payload: payload, Headers: headers, Time: time.Now()}
}
