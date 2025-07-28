// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/go-viper/mapstructure/v2"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// MessageBuilder is the builder for the message and sends it to the NATS server.
type MessageBuilder struct {
	NatsConn INatsConn
}

// sendMessage sends the message to the NATS server.
func (m *MessageBuilder) sendMessage(ctx context.Context, subject string, data []byte) error {
	err := m.NatsConn.Publish(subject, data)
	if err != nil {
		slog.ErrorContext(ctx, "error sending message to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent message to NATS", "subject", subject)
	return nil
}

// sendIndexerMessage sends the message to the NATS server for the indexer.
func (m *MessageBuilder) sendIndexerMessage(ctx context.Context, subject string, action models.MessageAction, data []byte) error {
	headers := make(map[string]string)
	if authorization, ok := ctx.Value(constants.AuthorizationContextID).(string); ok {
		headers[constants.AuthorizationHeader] = authorization
	}
	if principal, ok := ctx.Value(constants.PrincipalContextID).(string); ok {
		headers[constants.XOnBehalfOfHeader] = principal
	}

	var jsonData any
	if err := json.Unmarshal(data, &jsonData); err != nil {
		slog.ErrorContext(ctx, "error unmarshalling data into JSON", constants.ErrKey, err, "subject", subject)
		return err
	}

	// Decode the JSON data into a map[string]any since that is what the indexer expects.
	var payload map[string]any
	config := mapstructure.DecoderConfig{
		TagName: "json",
		Result:  &payload,
	}
	decoder, err := mapstructure.NewDecoder(&config)
	if err != nil {
		slog.ErrorContext(ctx, "error creating decoder", constants.ErrKey, err, "subject", subject)
		return err
	}
	err = decoder.Decode(jsonData)
	if err != nil {
		slog.ErrorContext(ctx, "error decoding data", constants.ErrKey, err, "subject", subject)
		return err
	}

	message := models.ProjectMessage{
		Action:  action,
		Headers: headers,
		Data:    payload,
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling message into JSON", constants.ErrKey, err, "subject", subject)
		return err
	}

	return m.sendMessage(ctx, subject, messageBytes)
}

// SendIndexProject sends the message to the NATS server for the project indexing.
func (m *MessageBuilder) SendIndexProject(ctx context.Context, action models.MessageAction, data []byte) error {
	return m.sendIndexerMessage(ctx, constants.IndexProjectSubject, action, data)
}

// SendIndexProjectSettings sends the message to the NATS server for the project settings indexing.
func (m *MessageBuilder) SendIndexProjectSettings(ctx context.Context, action models.MessageAction, data []byte) error {
	return m.sendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, action, data)
}

// SendUpdateAccessProject sends the message to the NATS server for the access control updates.
func (m *MessageBuilder) SendUpdateAccessProject(ctx context.Context, data []byte) error {
	return m.sendMessage(ctx, constants.UpdateAccessProjectSubject, data)
}

// SendUpdateAccessProjectSettings sends the message to the NATS server for the access control updates.
func (m *MessageBuilder) SendUpdateAccessProjectSettings(ctx context.Context, data []byte) error {
	return m.sendMessage(ctx, constants.UpdateAccessProjectSettingsSubject, data)
}

// SendDeleteAllAccessProject sends the message to the NATS server for the access control deletion.
func (m *MessageBuilder) SendDeleteAllAccessProject(ctx context.Context, data []byte) error {
	return m.sendMessage(ctx, constants.DeleteAllAccessSubject, data)
}

// SendDeleteAllAccessProjectSettings sends the message to the NATS server for the access control deletion.
func (m *MessageBuilder) SendDeleteAllAccessProjectSettings(ctx context.Context, data []byte) error {
	return m.sendMessage(ctx, constants.DeleteAllAccessProjectSettingsSubject, data)
}
