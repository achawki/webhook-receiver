// Code generated by mockery v2.3.0. DO NOT EDIT.

package mocks

import (
	model "github.com/achawki/webhook-receiver/internal/model"
	mock "github.com/stretchr/testify/mock"
)

// WebhookStorage is an autogenerated mock type for the WebhookStorage type
type WebhookStorage struct {
	mock.Mock
}

// GetMessagesForWebhook provides a mock function with given fields: webhookID
func (_m *WebhookStorage) GetMessagesForWebhook(webhookID string) ([]*model.Message, error) {
	ret := _m.Called(webhookID)

	var r0 []*model.Message
	if rf, ok := ret.Get(0).(func(string) []*model.Message); ok {
		r0 = rf(webhookID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*model.Message)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(webhookID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetWebhook provides a mock function with given fields: id
func (_m *WebhookStorage) GetWebhook(id string) (*model.Webhook, error) {
	ret := _m.Called(id)

	var r0 *model.Webhook
	if rf, ok := ret.Get(0).(func(string) *model.Webhook); ok {
		r0 = rf(id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.Webhook)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// InsertMessage provides a mock function with given fields: webhookID, message
func (_m *WebhookStorage) InsertMessage(webhookID string, message *model.Message) error {
	ret := _m.Called(webhookID, message)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, *model.Message) error); ok {
		r0 = rf(webhookID, message)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// InsertWebhook provides a mock function with given fields: webhook
func (_m *WebhookStorage) InsertWebhook(webhook *model.Webhook) (string, error) {
	ret := _m.Called(webhook)

	var r0 string
	if rf, ok := ret.Get(0).(func(*model.Webhook) string); ok {
		r0 = rf(webhook)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(*model.Webhook) error); ok {
		r1 = rf(webhook)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
