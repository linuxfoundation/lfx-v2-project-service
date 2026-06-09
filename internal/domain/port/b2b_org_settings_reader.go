// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// B2BOrgSettingsReader reads b2b_org access-control settings from the
// "org-settings" NATS KV bucket. Authoritative state, no MaxAge eviction.
type B2BOrgSettingsReader interface {
	// GetSettings returns the settings for the given b2b_org UID and the
	// current KV revision (needed for optimistic-locking on UpdateSettings).
	// Returns (nil, 0, nil) when no record exists yet.
	GetSettings(ctx context.Context, orgUID string) (*model.B2BOrgSettings, uint64, error)

	// ListSettingsOrgUIDs returns the org UIDs for which settings records exist
	// in the KV bucket. Returns an empty slice when the bucket is empty.
	ListSettingsOrgUIDs(ctx context.Context) ([]string, error)

	// LookupInviteOrgUID returns the orgUID recorded for the given InviteUUID in
	// the secondary index. Returns a NotFound error on an index miss (the invite
	// may predate the index or the write may have been best-effort lost).
	LookupInviteOrgUID(ctx context.Context, inviteUUID string) (string, error)
}
