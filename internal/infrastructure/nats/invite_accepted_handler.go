// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"log/slog"

	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/nats-io/nats.go"

	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/redaction"
)

// SubscribeInviteAccepted registers a NATS core queue subscription on
// inviteapi.InviteServiceAcceptedSubject. Each message is decoded into an
// InviteServiceAcceptedEvent and dispatched to handler. The queue group
// "lfx-v2-member-service" ensures exactly one API replica handles each event.
//
// The returned *nats.Subscription can be Drain()ed on shutdown. A non-nil error
// means the subscription could not be established.
func SubscribeInviteAccepted(
	conn *nats.Conn,
	handler func(context.Context, inviteapi.InviteServiceAcceptedEvent) error,
) (*nats.Subscription, error) {
	sub, err := conn.QueueSubscribe(
		inviteapi.InviteServiceAcceptedSubject,
		"lfx-v2-member-service",
		func(msg *nats.Msg) {
			// Inject a service-identity bearer so FGA/indexer calls downstream
			// carry a recognised principal (mirrors committee message_handler.go:903).
			ctx := context.WithValue(context.Background(), constants.AuthorizationContextID, constants.ServiceAccountBearer)

			var ev inviteapi.InviteServiceAcceptedEvent
			if err := json.Unmarshal(msg.Data, &ev); err != nil {
				slog.WarnContext(ctx, "invite_accepted: failed to decode event", "error", err)
				return
			}
			slog.DebugContext(ctx, "invite_accepted: received",
				"resource_type", ev.Resource.Type,
				"resource_uid", ev.Resource.UID,
				"recipient", redaction.RedactEmail(ev.Recipient.Email),
			)
			if err := handler(ctx, ev); err != nil {
				slog.WarnContext(ctx, "invite_accepted: handle error", "error", err)
			}
		},
	)
	if err != nil {
		return nil, err
	}

	slog.Info("subscribed to invite_accepted events",
		"subject", inviteapi.InviteServiceAcceptedSubject,
	)

	return sub, nil
}
