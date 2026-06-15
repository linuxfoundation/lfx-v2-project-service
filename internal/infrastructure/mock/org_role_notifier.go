// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package mock

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
)

// noopOrgRoleNotifier provides a no-op mock implementation of port.OrgRoleNotifier
// for local development when MESSAGING_SOURCE=mock.
type noopOrgRoleNotifier struct{}

// NewNoopOrgRoleNotifier returns a no-op OrgRoleNotifier that always succeeds.
func NewNoopOrgRoleNotifier() port.OrgRoleNotifier {
	return &noopOrgRoleNotifier{}
}

// NotifyRoleAssigned always returns nil. Satisfies port.OrgRoleNotifier for
// local development without a live email service.
func (n *noopOrgRoleNotifier) NotifyRoleAssigned(_ context.Context, _ port.OrgRoleAssignedNotification) error {
	return nil
}
