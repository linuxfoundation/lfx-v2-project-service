// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// MessageBuilder is the builder for the message and sends it to the NATS server.
type MessageBuilder struct {
	NatsConn INatsConn
}

// SendIndexProject sends the message to the NATS server for the project indexing.
func (m *MessageBuilder) SendIndexProject(ctx context.Context, action models.MessageAction, data []byte) error {
	subject := constants.IndexProjectSubject

	headers := make(map[string]string)
	if authorization, ok := ctx.Value(constants.AuthorizationContextID).(string); ok {
		headers["authorization"] = authorization
	}
	if principal, ok := ctx.Value(constants.PrincipalContextID).(string); ok {
		headers["x-on-behalf-of"] = principal
	}

	message := models.ProjectMessage{
		Action:  action,
		Headers: headers,
		Data:    data,
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling message into JSON", constants.ErrKey, err, "subject", subject)
		return err
	}

	err = m.NatsConn.Publish(subject, messageBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error sending message to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent message to NATS for data indexing", "subject", subject)
	return nil
}

// SendIndexProjectSettings sends the message to the NATS server for the project settings indexing.
func (m *MessageBuilder) SendIndexProjectSettings(ctx context.Context, action models.MessageAction, data []byte) error {
	subject := constants.IndexProjectSettingsSubject

	headers := make(map[string]string)
	if authorization, ok := ctx.Value(constants.AuthorizationContextID).(string); ok {
		headers["authorization"] = authorization
	}
	if principal, ok := ctx.Value(constants.PrincipalContextID).(string); ok {
		headers["x-on-behalf-of"] = principal
	}

	message := models.ProjectMessage{
		Action:  action,
		Headers: headers,
		Data:    data,
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling message into JSON", constants.ErrKey, err, "subject", subject)
		return err
	}

	err = m.NatsConn.Publish(subject, messageBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error sending message to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent message to NATS for data indexing", "subject", subject)
	return nil
}

// SendUpdateAccessProject sends the message to the NATS server for the access control updates.
func (m *MessageBuilder) SendUpdateAccessProject(ctx context.Context, data []byte) error {
	// Send the message to the NATS server for the access control updates.
	subject := constants.UpdateAccessProjectSubject
	err := m.NatsConn.Publish(subject, data)
	if err != nil {
		slog.ErrorContext(ctx, "error sending message to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent message to NATS for project access control updates", "subject", subject)
	return nil
}

// SendUpdateAccessProjectSettings sends the message to the NATS server for the access control updates.
func (m *MessageBuilder) SendUpdateAccessProjectSettings(ctx context.Context, data []byte) error {
	// Send the message to the NATS server for the access control updates.
	subject := constants.UpdateAccessProjectSettingsSubject
	err := m.NatsConn.Publish(subject, data)
	if err != nil {
		slog.ErrorContext(ctx, "error sending message to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent message to NATS for project access control updates", "subject", subject)
	return nil
}

// SendDeleteAllAccessProject sends the message to the NATS server for the access control deletion.
func (m *MessageBuilder) SendDeleteAllAccessProject(ctx context.Context, data []byte) error {
	// Send the message to the NATS server for the access control deletion.
	subject := constants.DeleteAllAccessSubject
	err := m.NatsConn.Publish(subject, data)
	if err != nil {
		slog.ErrorContext(ctx, "error sending message to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent message to NATS for project access control deletion", "subject", subject)
	return nil
}

// SendDeleteAllAccessProjectSettings sends the message to the NATS server for the access control deletion.
func (m *MessageBuilder) SendDeleteAllAccessProjectSettings(ctx context.Context, data []byte) error {
	// Send the message to the NATS server for the access control deletion.
	subject := constants.DeleteAllAccessProjectSettingsSubject
	err := m.NatsConn.Publish(subject, data)
	if err != nil {
		slog.ErrorContext(ctx, "error sending message to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent message to NATS for project access control deletion", "subject", subject)
	return nil
}
