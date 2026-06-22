// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// MembershipBatchReader fetches multiple project memberships in a single SOQL
// query. Used by the CDC consumer to replace per-record sObject fan-out with
// batched IN-clause fetches.
type MembershipBatchReader interface {
	// FetchMembershipsBySFIDs returns all project membership Assets whose
	// Salesforce Id matches one of the given SFIDs, subject to the same
	// IsDeleted = false and Product2.Family = 'Membership' filters applied by
	// the single-record fetch path. IDs absent from the result set have been
	// soft-deleted or no longer qualify. The second return value is the subset
	// of requested SFIDs that were present in the SOQL result but could not be
	// converted to domain objects; callers must NOT treat these as absent.
	FetchMembershipsBySFIDs(ctx context.Context, sfids []string) ([]*model.ProjectMembership, []string, error)
}
