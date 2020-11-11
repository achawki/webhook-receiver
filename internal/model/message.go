package model

import "time"

// Message represents a webhook message
type Message struct {
	Payload string
	Headers map[string][]string
	Time    time.Time
}

// NewMessage creates a new messages with current timestamp
func NewMessage(payload string, headers map[string][]string) *Message {
	return &Message{Payload: payload, Headers: headers, Time: time.Now()}
}
