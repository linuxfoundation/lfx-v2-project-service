// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-viper/mapstructure/v2"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go"
)

const defaultRequestTimeout = time.Second * 10

// MessageBuilder is the builder for the message and sends it to the NATS server.
type MessageBuilder struct {
	NatsConn INatsConn
}

// sendMessage sends the message to the NATS server.
func (m *MessageBuilder) sendMessage(ctx context.Context, subject string, data []byte, sync bool) error {
	if sync {
		_, err := m.requestMessage(subject, data, defaultRequestTimeout)
		if err != nil {
			slog.ErrorContext(ctx, "error requesting message from NATS", constants.ErrKey, err, "subject", subject)
			return err
		}
		slog.DebugContext(ctx, "sent and received response from NATS synchronously", "subject", subject)
		return nil
	}

	// Send message asynchronously.
	err := m.publishMessage(subject, data)
	if err != nil {
		slog.ErrorContext(ctx, "error sending message to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent message to NATS asynchronously", "subject", subject)
	return nil
}

// publishMessage publishes a message to NATS asynchronously.
func (m *MessageBuilder) publishMessage(subject string, data []byte) error {
	return m.NatsConn.Publish(subject, data)
}

// requestMessage requests a message from NATS synchronously.
func (m *MessageBuilder) requestMessage(subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
	return m.NatsConn.Request(subject, data, timeout)
}

// sendIndexerMessage sends the message to the NATS server for the indexer.
func (m *MessageBuilder) sendIndexerMessage(
	ctx context.Context,
	subject string,
	action indexerConstants.MessageAction,
	data []byte,
	tags []string,
	indexingConfig *indexerTypes.IndexingConfig,
	sync bool,
) error {
	headers := make(map[string]string)
	if authorization, ok := ctx.Value(constants.AuthorizationContextID).(string); ok {
		headers[constants.AuthorizationHeader] = authorization
	}
	if principal, ok := ctx.Value(constants.PrincipalContextID).(string); ok {
		headers[constants.XOnBehalfOfHeader] = principal
	}

	var payload any
	switch action {
	case indexerConstants.ActionCreated, indexerConstants.ActionUpdated:
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
	case indexerConstants.ActionDeleted:
		// The data should just be a string of the UID being deleted.
		payload = data
	}

	message := indexerTypes.IndexerMessageEnvelope{
		Action:         action,
		Headers:        headers,
		Data:           payload,
		Tags:           tags,
		IndexingConfig: indexingConfig,
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling message into JSON", constants.ErrKey, err, "subject", subject)
		return err
	}

	slog.DebugContext(ctx, "constructed indexer message", "subject", subject)

	return m.sendMessage(ctx, subject, messageBytes, sync)
}

// SendIndexerMessage sends indexer messages to NATS for search indexing.
func (m *MessageBuilder) SendIndexerMessage(ctx context.Context, subject string, message interface{}, sync bool) error {
	switch msg := message.(type) {
	case indexerTypes.IndexerMessageEnvelope:
		dataBytes, err := json.Marshal(msg.Data)
		if err != nil {
			slog.ErrorContext(ctx, "error marshalling message data into JSON", constants.ErrKey, err)
			return err
		}
		return m.sendIndexerMessage(ctx, subject, msg.Action, dataBytes, msg.Tags, msg.IndexingConfig, sync)

	case string:
		// For delete operations, the message is just the UID string
		return m.sendIndexerMessage(ctx, subject, indexerConstants.ActionDeleted, []byte(msg), nil, nil, sync)

	default:
		slog.ErrorContext(ctx, "unsupported indexer message type", "type", fmt.Sprintf("%T", message))
		return fmt.Errorf("unsupported indexer message type: %T", message)
	}
}

// SendAccessMessage sends access control messages to NATS using the generic FGA sync format.
func (m *MessageBuilder) SendAccessMessage(ctx context.Context, subject string, message interface{}, sync bool) error {
	switch msg := message.(type) {
	case models.GenericFGAMessage:
		messageBytes, err := json.Marshal(msg)
		if err != nil {
			slog.ErrorContext(ctx, "error marshalling FGA message into JSON", constants.ErrKey, err)
			return err
		}
		return m.sendMessage(ctx, subject, messageBytes, sync)

	default:
		slog.ErrorContext(ctx, "unsupported access message type", "type", fmt.Sprintf("%T", message))
		return fmt.Errorf("unsupported access message type: %T", message)
	}
}

// SendProjectEventMessage sends project event messages to NATS asynchronously.
// This is used for publishing events like project settings updates, project creation, etc.
func (m *MessageBuilder) SendProjectEventMessage(ctx context.Context, subject string, message any) error {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project event message into JSON", constants.ErrKey, err, "subject", subject)
		return err
	}

	err = m.publishMessage(subject, messageBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error publishing project event message to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}

	slog.DebugContext(ctx, "published project event message to NATS", "subject", subject)
	return nil
}
