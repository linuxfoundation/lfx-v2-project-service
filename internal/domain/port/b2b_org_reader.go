// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// B2BOrgReader provides read access to B2BOrg (Salesforce Account) records.
// Implementations are responsible for resolving the UUID from the Salesforce
// Account.Id and populating all fields of the returned model.B2BOrg.
type B2BOrgReader interface {
	// GetB2BOrg returns the B2BOrg identified by its v2 UUID. Returns an error
	// wrapping ErrNotFound if no record exists for the given uid.
	GetB2BOrg(ctx context.Context, uid string) (*model.B2BOrg, error)

	// SearchB2BOrgs returns a single page of B2BOrg records filtered by the
	// supplied predicates. The page size and continuation token are encoded in
	// filters.PageToken (empty = first page). The returned B2BOrgPage carries
	// an opaque NextPageToken for the caller to pass on the next request; an
	// empty NextPageToken means this is the last page.
	SearchB2BOrgs(ctx context.Context, filters model.B2BOrgFilters, pageSize int) (model.B2BOrgPage, error)
}
