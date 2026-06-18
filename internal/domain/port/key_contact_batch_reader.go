// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// KeyContactBatchReader fetches multiple key contacts in a single SOQL query.
// Used by the CDC consumer to replace per-record sObject fan-out with batched
// IN-clause fetches.
type KeyContactBatchReader interface {
	// FetchKeyContactsBySFIDs returns all Project_Role__c records whose
	// Salesforce Id matches one of the given SFIDs, subject to the same
	// IsDeleted = false filter applied by the single-record fetch path. IDs
	// absent from the result set have been soft-deleted. The second return
	// value is the subset of requested SFIDs that were present in the SOQL
	// result but could not be converted to domain objects; callers must NOT
	// treat these as absent.
	FetchKeyContactsBySFIDs(ctx context.Context, sfids []string) ([]*model.KeyContact, []string, error)
}
