// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// OrgRoleAssignedNotification carries the data needed to notify an existing
// LFID user that they have been granted access to an org dashboard.
type OrgRoleAssignedNotification struct {
	RecipientEmail string
	OrgName        string
	Role           model.B2BOrgRole
}

// OrgRoleNotifier sends role-assignment notification emails for users who
// already have an LFID account (no invite needed, direct access grant).
type OrgRoleNotifier interface {
	NotifyRoleAssigned(ctx context.Context, n OrgRoleAssignedNotification) error
}
