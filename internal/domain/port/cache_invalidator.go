// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import "context"

// CacheInvalidator removes a single record from the sObject REST cache so the
// next read fetches a fresh copy from Salesforce.
//
// It is a thin seam between the CDC orchestrator (internal/service) and the
// infrastructure layer (salesforce.SObjectClient) so the orchestrator can
// trigger cache eviction without importing salesforce-internal types.
type CacheInvalidator interface {
	// InvalidateB2BOrg evicts the cached b2b_org record for the given v2 UID.
	// A missing entry is treated as a no-op (already evicted).
	InvalidateB2BOrg(ctx context.Context, uid string) error

	// InvalidateProjectMembership evicts the cached project_membership record
	// for the given v2 UID. A missing entry is treated as a no-op.
	InvalidateProjectMembership(ctx context.Context, uid string) error

	// InvalidateKeyContact evicts the cached key_contact record for the given
	// v2 UID. A missing entry is treated as a no-op.
	InvalidateKeyContact(ctx context.Context, uid string) error
}
