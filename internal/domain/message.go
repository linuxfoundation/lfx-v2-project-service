// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
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

// MessageBuilder is a interface for the message builder.
type MessageBuilder interface {
	SendIndexProject(ctx context.Context, action models.MessageAction, data models.ProjectBase) error
	SendDeleteIndexProject(ctx context.Context, data string) error
	SendIndexProjectSettings(ctx context.Context, action models.MessageAction, data models.ProjectSettings) error
	SendDeleteIndexProjectSettings(ctx context.Context, data string) error
	SendUpdateAccessProject(ctx context.Context, data models.ProjectAccessMessage) error
	SendDeleteAllAccessProject(ctx context.Context, data string) error
}
