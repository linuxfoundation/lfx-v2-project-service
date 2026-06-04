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
	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const defaultRequestTimeout = time.Second * 10

// MessageBuilder is the builder for the message and sends it to the NATS server.
type MessageBuilder struct {
	NatsConn INatsConn
}

// sendMessage sends the message to the NATS server.
func (m *MessageBuilder) sendMessage(ctx context.Context, subject string, data []byte, sync bool) error {
	if sync {
		_, err := m.requestMessage(ctx, subject, data, defaultRequestTimeout)
		if err != nil {
			slog.ErrorContext(ctx, "error requesting message from NATS", constants.ErrKey, err, "subject", subject)
			return err
		}
		slog.DebugContext(ctx, "sent and received response from NATS synchronously", "subject", subject)
		return nil
	}

	// Send message asynchronously.
	err := m.publishMessage(ctx, subject, data)
	if err != nil {
		slog.ErrorContext(ctx, "error sending message to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}
	slog.DebugContext(ctx, "sent message to NATS asynchronously", "subject", subject)
	return nil
}

// publishMessage publishes a message to NATS asynchronously with an OTel producer span.
func (m *MessageBuilder) publishMessage(ctx context.Context, subject string, data []byte) error {
	ctx, span := tracer.Start(ctx, "nats.publish",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
			attribute.Int("messaging.message.body.size", len(data)),
		),
	)
	defer span.End()

	msg := nats.NewMsg(subject)
	msg.Header = make(nats.Header)
	msg.Data = data
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(msg.Header))

	if err := m.NatsConn.PublishMsg(msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	span.SetStatus(codes.Ok, "")
	return nil
}

// requestMessage requests a message from NATS synchronously with an OTel client span.
func (m *MessageBuilder) requestMessage(ctx context.Context, subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	ctx, span := tracer.Start(ctx, "nats.request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
			attribute.Int("messaging.message.body.size", len(data)),
		),
	)
	defer span.End()

	msg := nats.NewMsg(subject)
	msg.Header = make(nats.Header)
	msg.Data = data
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(msg.Header))

	reply, err := m.NatsConn.RequestMsgWithContext(ctx, msg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	span.SetStatus(codes.Ok, "")
	return reply, nil
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
		payload = string(data)
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

	slog.DebugContext(ctx, "constructed indexer message", "subject", subject, "message", string(messageBytes), "message_json", message)

	return m.sendMessage(ctx, subject, messageBytes, sync)
}

// SendIndexerMessage sends indexer messages to NATS for search indexing.
func (m *MessageBuilder) SendIndexerMessage(ctx context.Context, subject string, message interface{}, sync bool) error {
	switch msg := message.(type) {
	case indexerTypes.IndexerMessageEnvelope:
		var dataBytes []byte
		if msg.Action == indexerConstants.ActionDeleted {
			// For delete, Data is the UID string — pass as raw bytes so sendIndexerMessage
			// can convert back to string without an extra JSON-encoding layer.
			if uid, ok := msg.Data.(string); ok {
				dataBytes = []byte(uid)
			} else {
				var err error
				dataBytes, err = json.Marshal(msg.Data)
				if err != nil {
					slog.ErrorContext(ctx, "error marshalling message data into JSON", constants.ErrKey, err)
					return err
				}
			}
		} else {
			var err error
			dataBytes, err = json.Marshal(msg.Data)
			if err != nil {
				slog.ErrorContext(ctx, "error marshalling message data into JSON", constants.ErrKey, err)
				return err
			}
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
	case fgatypes.GenericFGAMessage:
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

	err = m.publishMessage(ctx, subject, messageBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error publishing project event message to NATS", constants.ErrKey, err, "subject", subject)
		return err
	}

	slog.DebugContext(ctx, "published project event message to NATS", "subject", subject)
	return nil
}

// SendInviteRequest sends a request to the invite service for a user who does
// not yet have an LFID and returns the invite UID from the reply.
func (m *MessageBuilder) SendInviteRequest(ctx context.Context, req inviteapi.SendInviteRequest) (domain.InviteResult, error) {
	data, err := json.Marshal(req)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling invite request into JSON", constants.ErrKey, err)
		return domain.InviteResult{}, err
	}

	ctx, span := tracer.Start(ctx, "nats.request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", inviteapi.SendInviteSubject),
			attribute.Int("messaging.message.body.size", len(data)),
		),
	)
	defer span.End()

	inviteMsg := nats.NewMsg(inviteapi.SendInviteSubject)
	inviteMsg.Header = make(nats.Header)
	inviteMsg.Data = data
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(inviteMsg.Header))

	reply, err := m.NatsConn.RequestMsgWithContext(ctx, inviteMsg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		slog.ErrorContext(ctx, "invite service request failed", constants.ErrKey, err)
		return domain.InviteResult{}, fmt.Errorf("invite service request: %w", err)
	}
	span.SetStatus(codes.Ok, "")

	var resp inviteapi.SendInviteResponse
	if len(reply.Data) > 0 {
		if jsonErr := json.Unmarshal(reply.Data, &resp); jsonErr != nil {
			slog.ErrorContext(ctx, "error unmarshalling invite response", constants.ErrKey, jsonErr)
			return domain.InviteResult{}, fmt.Errorf("invite service response: %w", jsonErr)
		}
		if resp.Error != "" {
			return domain.InviteResult{}, fmt.Errorf("invite service error: %s", resp.Error)
		}
	}

	if resp.Invite == nil || resp.Invite.UID == "" {
		return domain.InviteResult{}, fmt.Errorf("invite service returned success but invite_uid is empty")
	}
	result := domain.InviteResult{
		InviteUID:      resp.Invite.UID,
		RecipientEmail: resp.Invite.Email,
		ExpiresAt:      resp.Invite.ExpiresAt,
	}
	slog.DebugContext(ctx, "invite service replied", "invite_uid", result.InviteUID, "expires_at", result.ExpiresAt)
	return result, nil
}

// SendEmailRequest sends a request to the email service and waits for a reply.
func (m *MessageBuilder) SendEmailRequest(ctx context.Context, req emailapi.SendEmailRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling email request into JSON", constants.ErrKey, err)
		return err
	}

	ctx, emailSpan := tracer.Start(ctx, "nats.request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", emailapi.SendEmailSubject),
			attribute.Int("messaging.message.body.size", len(data)),
		),
	)
	defer emailSpan.End()

	emailMsg := nats.NewMsg(emailapi.SendEmailSubject)
	emailMsg.Header = make(nats.Header)
	emailMsg.Data = data
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(emailMsg.Header))

	reply, err := m.NatsConn.RequestMsgWithContext(ctx, emailMsg)
	if err != nil {
		emailSpan.RecordError(err)
		emailSpan.SetStatus(codes.Error, err.Error())
		slog.ErrorContext(ctx, "email service request failed", constants.ErrKey, err)
		return fmt.Errorf("email service request: %w", err)
	}
	emailSpan.SetStatus(codes.Ok, "")

	if len(reply.Data) > 0 {
		var errResp emailapi.SendEmailErrorResponse
		if jsonErr := json.Unmarshal(reply.Data, &errResp); jsonErr == nil && errResp.Error != "" {
			return fmt.Errorf("email service error: %s", errResp.Error)
		}
	}

	return nil
}
