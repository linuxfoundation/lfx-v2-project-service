// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"
)

// Message represents a domain message interface
type Message interface {
	Subject() string
	Data() []byte
	Respond(data []byte) error
}

// MessageHandler defines how the service handles incoming messages
type MessageHandler interface {
	HandleMessage(ctx context.Context, msg Message)
}

// MessageBuilder is a generic interface for publishing messages to NATS.
type MessageBuilder interface {
	PublishIndexerMessage(ctx context.Context, subject string, message interface{}) error
	PublishAccessMessage(ctx context.Context, subject string, message interface{}) error
}
