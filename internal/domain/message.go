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

// MessageBuilder is a generic interface for sending messages to NATS.
type MessageBuilder interface {
	SendIndexerMessage(ctx context.Context, subject string, message any, sync bool) error
	SendAccessMessage(ctx context.Context, subject string, message any, sync bool) error
}
