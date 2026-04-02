// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// MemberReader provides read access to project-scoped membership data.
// All methods follow the drill-down hierarchy: tiers per project, memberships
// per project, and key contacts per membership.
type MemberReader interface {
	// ListTiersForProject returns all MembershipTier records for the given
	// v2 project UID.
	ListTiersForProject(ctx context.Context, projectUID string) ([]*model.MembershipTier, error)

	// GetTier returns a single MembershipTier by its UID.
	GetTier(ctx context.Context, tierUID string) (*model.MembershipTier, error)

	// ListMembershipsForProject returns a single page of ProjectMembership
	// records for the given v2 project UID, filtered and ordered by the
	// supplied predicates. The page size and continuation token are encoded in
	// filters.PageToken (empty = first page). The returned MembershipPage
	// carries an opaque NextPageToken for the caller to pass on the next
	// request; an empty NextPageToken means this is the last page.
	ListMembershipsForProject(ctx context.Context, projectUID string, filters model.MembershipFilters, pageSize int) (model.MembershipPage, error)

	// GetMembership returns a single ProjectMembership by its UID.
	GetMembership(ctx context.Context, membershipUID string) (*model.ProjectMembership, error)

	// ListKeyContactsForMembership returns all KeyContact records for
	// the given membership UID.
	ListKeyContactsForMembership(ctx context.Context, membershipUID string) ([]*model.KeyContact, error)

	// GetKeyContact returns a single KeyContact by its UID.
	GetKeyContact(ctx context.Context, keyContactUID string) (*model.KeyContact, error)

	// ListMembershipsForB2BOrg returns a single page of ProjectMembership
	// records for the given v2 B2BOrg UID across all projects. The page size
	// and continuation token are encoded in filters.PageToken (empty = first
	// page). The returned MembershipPage carries an opaque NextPageToken for
	// the caller to pass on the next request; an empty NextPageToken means
	// this is the last page.
	ListMembershipsForB2BOrg(ctx context.Context, b2bOrgUID string, filters model.MembershipFilters, pageSize int) (model.MembershipPage, error)

	// IsReady reports whether the underlying storage is reachable.
	IsReady(ctx context.Context) error
}
