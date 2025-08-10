// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"fmt"
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
	message := models.IndexerMessageEnvelope{
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


// PublishIndexerMessage publishes indexer messages to NATS for search indexing.
func (m *MessageBuilder) PublishIndexerMessage(ctx context.Context, subject string, message interface{}) error {
	switch msg := message.(type) {
	case models.ProjectIndexerMessage:
		dataBytes, err := json.Marshal(msg.Data)
		if err != nil {
			slog.ErrorContext(ctx, "error marshalling project data into JSON", constants.ErrKey, err)
			return err
		}
		return m.sendIndexerMessage(ctx, subject, msg.Action, dataBytes, msg.Tags)
	
	case models.ProjectSettingsIndexerMessage:
		dataBytes, err := json.Marshal(msg.Data)
		if err != nil {
			slog.ErrorContext(ctx, "error marshalling project settings data into JSON", constants.ErrKey, err)
			return err
		}
		return m.sendIndexerMessage(ctx, subject, msg.Action, dataBytes, msg.Tags)
	
	case string:
		// For delete operations, the message is just the UID string
		return m.sendIndexerMessage(ctx, subject, models.ActionDeleted, []byte(msg), nil)
	
	default:
		slog.ErrorContext(ctx, "unsupported indexer message type", "type", fmt.Sprintf("%T", message))
		return fmt.Errorf("unsupported indexer message type: %T", message)
	}
}

// PublishAccessMessage publishes access control messages to NATS.
func (m *MessageBuilder) PublishAccessMessage(ctx context.Context, subject string, message interface{}) error {
	switch msg := message.(type) {
	case models.ProjectAccessMessage:
		dataBytes, err := json.Marshal(msg.Data)
		if err != nil {
			slog.ErrorContext(ctx, "error marshalling access message data into JSON", constants.ErrKey, err)
			return err
		}
		return m.sendMessage(ctx, subject, dataBytes)
	
	case string:
		// For delete operations, the message is just the UID string
		return m.sendMessage(ctx, subject, []byte(msg))
	
	default:
		slog.ErrorContext(ctx, "unsupported access message type", "type", fmt.Sprintf("%T", message))
		return fmt.Errorf("unsupported access message type: %T", message)
	}
}

