// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"
	"time"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
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

// InviteResult carries the data returned by the invite service on send_invite.
type InviteResult struct {
	InviteUID      string
	RecipientEmail string
	ExpiresAt      time.Time
}

// MessageBuilder is a generic interface for sending messages to NATS.
type MessageBuilder interface {
	SendIndexerMessage(ctx context.Context, subject string, message any, sync bool) error
	PublishAccessMessage(ctx context.Context, subject string, message fgatypes.GenericFGAMessage) error
	SendProjectEventMessage(ctx context.Context, subject string, message any) error
	SendEmailRequest(ctx context.Context, req emailapi.SendEmailRequest) error
	SendInviteRequest(ctx context.Context, req inviteapi.SendInviteRequest) (InviteResult, error)
}
