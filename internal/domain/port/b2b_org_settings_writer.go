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
	// when revision == 0, unconditional put. Returns a conflict error on stale revision.
	UpdateSettings(ctx context.Context, settings *model.B2BOrgSettings, revision uint64) error
}
