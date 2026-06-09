// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// B2BOrgSettingsWriter persists b2b_org access-control settings to the
// "org-settings" NATS KV bucket. Authoritative state, no MaxAge eviction.
type B2BOrgSettingsWriter interface {
	// UpdateSettings persists settings for a b2b_org. The org UID is carried
	// in settings.UID. When revision > 0, uses optimistic-locking (kv.Update);
	// when revision == 0, uses an exclusive create (fails with Conflict if a
	// concurrent first-write already landed). Returns Conflict on any revision mismatch.
	UpdateSettings(ctx context.Context, settings *model.B2BOrgSettings, revision uint64) error

	// PutInviteIndex writes (or overwrites) the InviteUUID→orgUID secondary-index entry.
	// Best-effort: callers log and continue on error.
	PutInviteIndex(ctx context.Context, inviteUUID, orgUID string) error

	// DeleteInviteIndex removes the InviteUUID→orgUID secondary-index entry.
	// Not-found is tolerated (idempotent). Best-effort: callers log and continue on error.
	DeleteInviteIndex(ctx context.Context, inviteUUID string) error
}
