// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// MessageBuilder is the builder for the transaction and sends it to the NATS server.
type MessageBuilder struct {
	NatsConn       INatsConn
	LfxEnvironment constants.LFXEnvironment
}

// SendIndexProject sends the transaction to the NATS server for the project indexing.
func (m *MessageBuilder) SendIndexProject(ctx context.Context, action TransactionAction, data []byte) error {
	subject := fmt.Sprintf("%s%s", m.LfxEnvironment, constants.IndexProjectSubject)

	headers := make(map[string]string)
	if authorization, ok := ctx.Value(constants.AuthorizationContextID).(string); ok {
		headers["authorization"] = authorization
	}
	if principal, ok := ctx.Value(constants.PrincipalContextID).(string); ok {
		headers["x-on-behalf-of"] = principal
	}

	transaction := ProjectTransaction{
		Action:  action,
		Headers: headers,
		Data:    data,
	}

	transactionBytes, err := json.Marshal(transaction)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling transaction into JSON", constants.ErrKey, err, "subject", subject)
		return err
	}

	err = m.NatsConn.Publish(subject, transactionBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error sending transaction to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent transaction to NATS for data indexing", "subject", subject)
	return nil
}

// SendIndexProjectSettings sends the transaction to the NATS server for the project settings indexing.
func (m *MessageBuilder) SendIndexProjectSettings(ctx context.Context, action TransactionAction, data []byte) error {
	subject := fmt.Sprintf("%s%s", m.LfxEnvironment, constants.IndexProjectSettingsSubject)

	headers := make(map[string]string)
	if authorization, ok := ctx.Value(constants.AuthorizationContextID).(string); ok {
		headers["authorization"] = authorization
	}
	if principal, ok := ctx.Value(constants.PrincipalContextID).(string); ok {
		headers["x-on-behalf-of"] = principal
	}

	transaction := ProjectTransaction{
		Action:  action,
		Headers: headers,
		Data:    data,
	}

	transactionBytes, err := json.Marshal(transaction)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling transaction into JSON", constants.ErrKey, err, "subject", subject)
		return err
	}

	err = m.NatsConn.Publish(subject, transactionBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error sending transaction to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent transaction to NATS for data indexing", "subject", subject)
	return nil
}

// SendUpdateAccessProject sends the transaction to the NATS server for the access control updates.
func (m *MessageBuilder) SendUpdateAccessProject(ctx context.Context, data []byte) error {
	// Send the transaction to the NATS server for the access control updates.
	subject := fmt.Sprintf("%s%s", m.LfxEnvironment, constants.UpdateAccessProjectSubject)
	err := m.NatsConn.Publish(subject, data)
	if err != nil {
		slog.ErrorContext(ctx, "error sending transaction to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent transaction to NATS for project access control updates", "subject", subject)
	return nil
}

// SendUpdateAccessProjectSettings sends the transaction to the NATS server for the access control updates.
func (m *MessageBuilder) SendUpdateAccessProjectSettings(ctx context.Context, data []byte) error {
	// Send the transaction to the NATS server for the access control updates.
	subject := fmt.Sprintf("%s%s", m.LfxEnvironment, constants.UpdateAccessProjectSettingsSubject)
	err := m.NatsConn.Publish(subject, data)
	if err != nil {
		slog.ErrorContext(ctx, "error sending transaction to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent transaction to NATS for project access control updates", "subject", subject)
	return nil
}

// SendDeleteAllAccessProject sends the transaction to the NATS server for the access control deletion.
func (m *MessageBuilder) SendDeleteAllAccessProject(ctx context.Context, data []byte) error {
	// Send the transaction to the NATS server for the access control deletion.
	subject := fmt.Sprintf("%s%s", m.LfxEnvironment, constants.DeleteAllAccessSubject)
	err := m.NatsConn.Publish(subject, data)
	if err != nil {
		slog.ErrorContext(ctx, "error sending transaction to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent transaction to NATS for project access control deletion", "subject", subject)
	return nil
}

// SendDeleteAllAccessProjectSettings sends the transaction to the NATS server for the access control deletion.
func (m *MessageBuilder) SendDeleteAllAccessProjectSettings(ctx context.Context, data []byte) error {
	// Send the transaction to the NATS server for the access control deletion.
	subject := fmt.Sprintf("%s%s", m.LfxEnvironment, constants.DeleteAllAccessProjectSettingsSubject)
	err := m.NatsConn.Publish(subject, data)
	if err != nil {
		slog.ErrorContext(ctx, "error sending transaction to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent transaction to NATS for project access control deletion", "subject", subject)
	return nil
}
