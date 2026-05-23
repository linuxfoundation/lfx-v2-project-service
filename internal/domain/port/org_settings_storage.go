// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// OrgSettingsStorage persists b2b_org access-control principals (writers,
// auditors, and pending invites). Stored in the "org-settings" KV bucket with
// no MaxAge eviction — authoritative state, never silently dropped.
// Pattern mirrors lfx-v2-committee-service "committee-settings" bucket.
//
// org-membership tracking (b2b_org.membership FGA refs) and key-contact invite
// state are out of scope here — handled by dedicated tickets and buckets.
type OrgSettingsStorage interface {
	// GetOrgSettings returns the settings for the given b2b_org UID and the
	// current KV revision (needed for optimistic-locking on PutOrgSettings).
	// Returns (nil, 0, nil) when no record exists yet.
	GetOrgSettings(ctx context.Context, orgUID string) (*model.OrgSettings, uint64, error)

	// PutOrgSettings persists settings for a b2b_org. When revision > 0, uses
	// optimistic-locking (kv.Update); when revision == 0, unconditional put.
	// Returns a conflict error when the revision is stale.
	PutOrgSettings(ctx context.Context, orgUID string, settings *model.OrgSettings, revision uint64) error
}
