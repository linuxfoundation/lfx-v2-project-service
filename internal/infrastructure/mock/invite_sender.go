// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package mock

import (
	"context"

	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
)

// noopInviteSender provides a no-op mock implementation of port.InviteSender
// for local development when MESSAGING_SOURCE=mock.
type noopInviteSender struct{}

// NewNoopInviteSender returns a no-op InviteSender that always succeeds with
// empty results.
func NewNoopInviteSender() port.InviteSender {
	return &noopInviteSender{}
}

// SendInvite always returns empty InviteResult and nil error. Satisfies
// port.InviteSender for local development without invite-service.
func (n *noopInviteSender) SendInvite(_ context.Context, _ inviteapi.SendInviteRequest) (port.InviteResult, error) {
	return port.InviteResult{}, nil
}
