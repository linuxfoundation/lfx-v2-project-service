// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package service contains use-case orchestrators that sit between the Goa
// presentation layer and the domain port implementations. Each orchestrator
// delegates directly to a port.MemberReader, adding structured logging and
// error propagation without duplicating business logic.
package service

import (
	"context"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
)

// MemberReader defines the use-case interface for project-scoped membership
// read operations. It mirrors port.MemberReader closely but is defined here so
// the presentation layer depends on the use-case package rather than the port
// package directly.
type MemberReader interface {
	ListTiersForProject(ctx context.Context, projectSFID string) ([]*model.MembershipTier, error)
	GetTier(ctx context.Context, tierUID string) (*model.MembershipTier, error)
	ListMembershipsForProject(ctx context.Context, projectUID string, filters model.MembershipFilters, pageSize int) (model.MembershipPage, error)
	GetMembership(ctx context.Context, membershipUID string) (*model.ProjectMembership, error)
	ListKeyContactsForMembership(ctx context.Context, membershipUID string) ([]*model.KeyContact, error)
	GetKeyContact(ctx context.Context, keyContactUID string) (*model.KeyContact, error)
	ListKeyContactsForOrg(ctx context.Context, orgSFID string) ([]*model.KeyContact, error)
}

// memberReaderOrchestratorOption defines a functional option for configuring a
// memberReaderOrchestrator.
type memberReaderOrchestratorOption func(*memberReaderOrchestrator)

// WithMemberReader sets the underlying port.MemberReader on the orchestrator.
func WithMemberReader(reader port.MemberReader) memberReaderOrchestratorOption {
	return func(r *memberReaderOrchestrator) {
		r.memberReader = reader
	}
}

// memberReaderOrchestrator wraps a port.MemberReader with structured logging
// and satisfies the MemberReader use-case interface.
type memberReaderOrchestrator struct {
	memberReader port.MemberReader
}

// ListTiersForProject returns all MembershipTier records for the given project.
func (rc *memberReaderOrchestrator) ListTiersForProject(ctx context.Context, projectSFID string) ([]*model.MembershipTier, error) {
	slog.DebugContext(ctx, "executing list tiers for project use case",
		"project_sfid", projectSFID,
	)

	tiers, err := rc.memberReader.ListTiersForProject(ctx, projectSFID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list tiers for project",
			"error", err,
			"project_sfid", projectSFID,
		)
		return nil, err
	}

	slog.DebugContext(ctx, "tiers retrieved successfully",
		"project_sfid", projectSFID,
		"tier_count", len(tiers),
	)
	return tiers, nil
}

// GetTier returns the MembershipTier identified by tierUID.
func (rc *memberReaderOrchestrator) GetTier(ctx context.Context, tierUID string) (*model.MembershipTier, error) {
	slog.DebugContext(ctx, "executing get tier use case", "tier_uid", tierUID)

	tier, err := rc.memberReader.GetTier(ctx, tierUID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get tier",
			"error", err,
			"tier_uid", tierUID,
		)
		return nil, err
	}

	slog.DebugContext(ctx, "tier retrieved successfully", "tier_uid", tierUID)
	return tier, nil
}

// ListMembershipsForProject returns a single page of ProjectMembership records
// for the given project, filtered and ordered by the supplied predicates.
func (rc *memberReaderOrchestrator) ListMembershipsForProject(ctx context.Context, projectUID string, filters model.MembershipFilters, pageSize int) (model.MembershipPage, error) {
	slog.DebugContext(ctx, "executing list memberships for project use case",
		"project_uid", projectUID,
		"filter_tier_uid", filters.TierUID,
		"sort_order", filters.EffectiveSortOrder(),
		"page_token_set", filters.PageToken != "",
		"page_size", pageSize,
	)

	page, err := rc.memberReader.ListMembershipsForProject(ctx, projectUID, filters, pageSize)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list memberships for project",
			"error", err,
			"project_uid", projectUID,
		)
		return model.MembershipPage{}, err
	}

	slog.DebugContext(ctx, "memberships page retrieved successfully",
		"project_uid", projectUID,
		"count", len(page.Memberships),
		"total_size", page.TotalSize,
		"has_next_page", page.NextPageToken != "",
	)
	return page, nil
}

// GetMembership returns the ProjectMembership identified by membershipUID.
func (rc *memberReaderOrchestrator) GetMembership(ctx context.Context, membershipUID string) (*model.ProjectMembership, error) {
	slog.DebugContext(ctx, "executing get membership use case", "membership_uid", membershipUID)

	membership, err := rc.memberReader.GetMembership(ctx, membershipUID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get membership",
			"error", err,
			"membership_uid", membershipUID,
		)
		return nil, err
	}

	slog.DebugContext(ctx, "membership retrieved successfully", "membership_uid", membershipUID)
	return membership, nil
}

// ListKeyContactsForMembership returns all KeyContact records for the given
// membership.
func (rc *memberReaderOrchestrator) ListKeyContactsForMembership(ctx context.Context, membershipUID string) ([]*model.KeyContact, error) {
	slog.DebugContext(ctx, "executing list key contacts for membership use case",
		"membership_uid", membershipUID,
	)

	contacts, err := rc.memberReader.ListKeyContactsForMembership(ctx, membershipUID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list key contacts for membership",
			"error", err,
			"membership_uid", membershipUID,
		)
		return nil, err
	}

	slog.DebugContext(ctx, "key contacts retrieved successfully",
		"membership_uid", membershipUID,
		"contact_count", len(contacts),
	)
	return contacts, nil
}

// GetKeyContact returns the KeyContact identified by keyContactUID.
func (rc *memberReaderOrchestrator) GetKeyContact(ctx context.Context, keyContactUID string) (*model.KeyContact, error) {
	slog.DebugContext(ctx, "executing get key contact use case", "key_contact_uid", keyContactUID)

	contact, err := rc.memberReader.GetKeyContact(ctx, keyContactUID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get key contact",
			"error", err,
			"key_contact_uid", keyContactUID,
		)
		return nil, err
	}

	slog.DebugContext(ctx, "key contact retrieved successfully", "key_contact_uid", keyContactUID)
	return contact, nil
}

// ListKeyContactsForOrg returns all KeyContact records across every membership
// backed by the given 18-char Account SFID (b2b_org UID).
func (rc *memberReaderOrchestrator) ListKeyContactsForOrg(ctx context.Context, orgSFID string) ([]*model.KeyContact, error) {
	slog.DebugContext(ctx, "executing list key contacts for org use case", "org_sfid", orgSFID)

	contacts, err := rc.memberReader.ListKeyContactsForOrg(ctx, orgSFID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list key contacts for org",
			"error", err,
			"org_sfid", orgSFID,
		)
		return nil, err
	}

	slog.DebugContext(ctx, "key contacts for org retrieved successfully",
		"org_sfid", orgSFID,
		"contact_count", len(contacts),
	)
	return contacts, nil
}

// NewMemberReaderOrchestrator creates a new memberReaderOrchestrator. The
// WithMemberReader option is required; the constructor panics if it is omitted.
func NewMemberReaderOrchestrator(opts ...memberReaderOrchestratorOption) MemberReader {
	rc := &memberReaderOrchestrator{}
	for _, opt := range opts {
		opt(rc)
	}
	if rc.memberReader == nil {
		panic("member reader is required: use WithMemberReader option")
	}
	return rc
}
