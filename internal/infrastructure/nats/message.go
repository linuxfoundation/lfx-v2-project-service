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
func (m *MessageBuilder) sendIndexerMessage(ctx context.Context, subject string, action models.MessageAction, data []byte, tags []string) error {
	headers := make(map[string]string)
	if authorization, ok := ctx.Value(constants.AuthorizationContextID).(string); ok {
		headers[constants.AuthorizationHeader] = authorization
	}
	if principal, ok := ctx.Value(constants.PrincipalContextID).(string); ok {
		headers[constants.XOnBehalfOfHeader] = principal
	}

	var payload any
	switch action {
	case models.ActionCreated, models.ActionUpdated:
		// The data should be a JSON object.
		var jsonData any
		if err := json.Unmarshal(data, &jsonData); err != nil {
			slog.ErrorContext(ctx, "error unmarshalling data into JSON", constants.ErrKey, err, "subject", subject)
			return err
		}

		// Decode the JSON data into a map[string]any since that is what the indexer expects.
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
	case models.ActionDeleted:
		// The data should just be a string of the UID being deleted.
		payload = data
	}

	// TODO: use the model from the indexer service to keep the message body consistent.
	// Ticket https://linuxfoundation.atlassian.net/browse/LFXV2-147
	message := models.ProjectIndexerMessage{
		Action:  action,
		Headers: headers,
		Data:    payload,
		Tags:    tags,
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling message into JSON", constants.ErrKey, err, "subject", subject)
		return err
	}

	slog.DebugContext(ctx, "constructed indexer message", "subject", subject, "message", string(messageBytes))

	return m.sendMessage(ctx, subject, messageBytes)
}

// setIndexerTags sets the tags for the indexer.
func (m *MessageBuilder) setIndexerTags(tags ...string) []string {
	return tags
}

// SendIndexProject sends the message to the NATS server for the project indexing.
func (m *MessageBuilder) SendIndexProject(ctx context.Context, action models.MessageAction, data models.ProjectBase) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling data into JSON", constants.ErrKey, err)
		return err
	}

	tags := m.setIndexerTags(data.UID, data.Name, data.Slug, data.Description)

	return m.sendIndexerMessage(ctx, constants.IndexProjectSubject, action, dataBytes, tags)
}

// SendDeleteIndexProject sends the message to the NATS server for the project indexing.
func (m *MessageBuilder) SendDeleteIndexProject(ctx context.Context, data string) error {
	return m.sendIndexerMessage(ctx, constants.IndexProjectSubject, models.ActionDeleted, []byte(data), nil)
}

// SendIndexProjectSettings sends the message to the NATS server for the project settings indexing.
func (m *MessageBuilder) SendIndexProjectSettings(ctx context.Context, action models.MessageAction, data models.ProjectSettings) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling data into JSON", constants.ErrKey, err)
		return err
	}

	tags := m.setIndexerTags(data.UID, data.MissionStatement)

	return m.sendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, action, dataBytes, tags)
}

// SendDeleteIndexProjectSettings sends the message to the NATS server for the project settings indexing.
func (m *MessageBuilder) SendDeleteIndexProjectSettings(ctx context.Context, data string) error {
	return m.sendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, models.ActionDeleted, []byte(data), nil)
}

// SendUpdateAccessProject sends the message to the NATS server for the access control updates.
func (m *MessageBuilder) SendUpdateAccessProject(ctx context.Context, data models.ProjectAccessMessage) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling data into JSON", constants.ErrKey, err)
		return err
	}

	return m.sendMessage(ctx, constants.UpdateAccessProjectSubject, dataBytes)
}

// SendDeleteAllAccessProject sends the message to the NATS server for the access control deletion.
func (m *MessageBuilder) SendDeleteAllAccessProject(ctx context.Context, data string) error {
	return m.sendMessage(ctx, constants.DeleteAllAccessSubject, []byte(data))
}
