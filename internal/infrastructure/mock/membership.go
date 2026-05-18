// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package mock provides mock implementations of domain ports for testing.
package mock

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// MockMembershipRepository provides a mock implementation of port.MemberReader
// for testing. Data is stored in-memory and pre-seeded with sample records.
type MockMembershipRepository struct {
	tiers       map[string]*model.MembershipTier
	memberships map[string]*model.ProjectMembership
	contacts    map[string]*model.KeyContact
	mu          sync.RWMutex
}

// NewMockMembershipRepository creates a new mock repository pre-seeded with
// sample data covering a single project, one tier, one membership, and one key
// contact.
func NewMockMembershipRepository() *MockMembershipRepository {
	now := time.Now()

	mock := &MockMembershipRepository{
		tiers:       make(map[string]*model.MembershipTier),
		memberships: make(map[string]*model.ProjectMembership),
		contacts:    make(map[string]*model.KeyContact),
	}

	// Sample tier (Product2).
	sampleTier := &model.MembershipTier{
		UID:         "tier-1",
		ProjectUID:  "project-uid-1",
		ProjectSlug: "linux-foundation",
		Name:        "Gold Membership",
		Family:      "Membership",
		ProductType: "Corporate",
		CreatedAt:   now.Add(-48 * time.Hour),
		UpdatedAt:   now,
	}
	mock.tiers[sampleTier.UID] = sampleTier

	// Sample membership (Asset).
	sampleMembership := &model.ProjectMembership{
		UID:              "membership-1",
		TierUID:          "tier-1",
		ProjectUID:       "project-uid-1",
		ProjectSlug:      "linux-foundation",
		Status:           "Active",
		Year:             "2025",
		Tier:             "Gold",
		AutoRenew:        true,
		RenewalType:      "Annual",
		Price:            50000,
		AnnualFullPrice:  50000,
		PaymentFrequency: "Annual",
		StartDate:        "2025-01-01T00:00:00Z",
		EndDate:          "2025-12-31T23:59:59Z",
		CompanyName:      "Example Corp",
		CompanyLogoURL:   "https://example.com/logo.png",
		CompanyDomain:    "https://example.com",
		TierName:         "Gold Membership",
		TierFamily:       "Membership",
		TierProductType:  "Corporate",
		CreatedAt:        now.Add(-24 * time.Hour),
		UpdatedAt:        now,
	}
	mock.memberships[sampleMembership.UID] = sampleMembership

	// Sample key contact (Project_Role__c).
	sampleContact := &model.KeyContact{
		UID:            "contact-role-1",
		MembershipUID:  "membership-1",
		TierUID:        "tier-1",
		ProjectUID:     "project-uid-1",
		ProjectSlug:    "linux-foundation",
		Role:           "Primary Contact",
		Status:         "Active",
		BoardMember:    false,
		PrimaryContact: true,
		FirstName:      "John",
		LastName:       "Doe",
		Title:          "CTO",
		Email:          "john.doe@example.com",
		CompanyName:    "Example Corp",
		CompanyLogoURL: "https://example.com/logo.png",
		CompanyDomain:  "https://example.com",
		CreatedAt:      now.Add(-24 * time.Hour),
		UpdatedAt:      now,
	}
	mock.contacts[sampleContact.UID] = sampleContact

	return mock
}

// ListTiersForProject returns all MembershipTier records whose ProjectUID matches
// the given v2 project UID.
func (m *MockMembershipRepository) ListTiersForProject(ctx context.Context, projectUID string) ([]*model.MembershipTier, error) {
	slog.DebugContext(ctx, "mock: listing tiers for project", "project_uid", projectUID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*model.MembershipTier
	for _, t := range m.tiers {
		if t.ProjectUID == projectUID {
			result = append(result, t)
		}
	}

	return result, nil
}

// GetTier returns the MembershipTier identified by tierUID.
func (m *MockMembershipRepository) GetTier(ctx context.Context, tierUID string) (*model.MembershipTier, error) {
	slog.DebugContext(ctx, "mock: getting tier", "uid", tierUID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	tier, exists := m.tiers[tierUID]
	if !exists {
		return nil, errors.NewNotFound(fmt.Sprintf("tier with UID %s not found", tierUID))
	}

	return tier, nil
}

// ListMembershipsForProject returns a MembershipPage of ProjectMembership
// records whose ProjectUID matches the given v2 project UID, filtered
// in-memory by the supplied MembershipFilters predicates.
//
// The mock does not implement real cursor-based pagination — pageSize and
// PageToken are accepted but only pageSize is applied as a simple slice cap.
// SortOrder is applied in-memory: name→alpha by CompanyName,
// newest→CreatedAt desc, last_modified→UpdatedAt desc.
func (m *MockMembershipRepository) ListMembershipsForProject(ctx context.Context, projectUID string, filters model.MembershipFilters, pageSize int) (model.MembershipPage, error) {
	slog.DebugContext(ctx, "mock: listing memberships for project",
		"project_uid", projectUID,
		"filter_tier_uid", filters.TierUID,
		"sort_order", filters.EffectiveSortOrder(),
		"page_size", pageSize,
	)

	m.mu.RLock()
	defer m.mu.RUnlock()

	filterMap := make(map[string]string)
	// TierUID is a SOQL-level filter; approximate in-memory by matching the
	// TierUID field directly on the membership record.
	if filters.TierUID != "" {
		filterMap["tier_uid"] = filters.TierUID
	}
	// Status is not exposed as a filter — hardcoded to active members only,
	// mirroring the hardcoded AND Status = 'Active' in the SOQL base query.
	filterMap["status"] = "Active"

	var result []*model.ProjectMembership
	for _, ms := range m.memberships {
		if ms.ProjectUID != projectUID {
			continue
		}
		if !matchesMockMemberFilters(ms, filterMap, "") {
			continue
		}
		// CompanyNameSearch mirrors the SOQL LIKE predicate pushed down in the
		// real implementation. Match only on CompanyName (case-insensitive).
		if filters.CompanyNameSearch != "" &&
			!strings.Contains(strings.ToLower(ms.CompanyName), filters.CompanyNameSearch) {
			continue
		}
		result = append(result, ms)
	}

	// Apply in-memory sort to mirror SOQL ORDER BY behaviour.
	sortMockMemberships(result, filters.EffectiveSortOrder())

	// Apply a simple page cap (no real cursor support in the mock).
	if pageSize > 0 && len(result) > pageSize {
		result = result[:pageSize]
	}

	return model.MembershipPage{
		Memberships:   result,
		NextPageToken: "", // mock never paginates
		TotalSize:     len(result),
	}, nil
}

// GetMembership returns the ProjectMembership identified by membershipUID.
func (m *MockMembershipRepository) GetMembership(ctx context.Context, membershipUID string) (*model.ProjectMembership, error) {
	slog.DebugContext(ctx, "mock: getting membership", "uid", membershipUID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	ms, exists := m.memberships[membershipUID]
	if !exists {
		return nil, errors.NewNotFound(fmt.Sprintf("membership with UID %s not found", membershipUID))
	}

	return ms, nil
}

// ListKeyContactsForMembership returns all KeyContact records whose MembershipUID
// matches the given membership UID.
func (m *MockMembershipRepository) ListKeyContactsForMembership(ctx context.Context, membershipUID string) ([]*model.KeyContact, error) {
	slog.DebugContext(ctx, "mock: listing key contacts for membership", "membership_uid", membershipUID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*model.KeyContact
	for _, c := range m.contacts {
		if c.MembershipUID == membershipUID {
			result = append(result, c)
		}
	}

	return result, nil
}

// GetKeyContact returns the KeyContact identified by keyContactUID.
func (m *MockMembershipRepository) GetKeyContact(ctx context.Context, keyContactUID string) (*model.KeyContact, error) {
	slog.DebugContext(ctx, "mock: getting key contact", "uid", keyContactUID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	c, exists := m.contacts[keyContactUID]
	if !exists {
		return nil, errors.NewNotFound(fmt.Sprintf("key contact with UID %s not found", keyContactUID))
	}

	return c, nil
}

// IsReady always returns nil for mock implementations.
func (m *MockMembershipRepository) IsReady(_ context.Context) error {
	return nil
}

// sortMockMemberships sorts a slice of ProjectMembership records in-place
// according to the given SortOrder, mirroring the SOQL ORDER BY clauses used
// in the real Salesforce implementation.
func sortMockMemberships(ms []*model.ProjectMembership, order model.SortOrder) {
	if len(ms) < 2 {
		return
	}
	switch order {
	case model.SortOrderName:
		// ORDER BY Account.Name ASC NULLS LAST
		for i := 1; i < len(ms); i++ {
			for j := i; j > 0 && strings.ToLower(ms[j-1].CompanyName) > strings.ToLower(ms[j].CompanyName); j-- {
				ms[j-1], ms[j] = ms[j], ms[j-1]
			}
		}
	case model.SortOrderLastModified:
		// ORDER BY LastModifiedDate DESC NULLS LAST
		for i := 1; i < len(ms); i++ {
			for j := i; j > 0 && ms[j-1].UpdatedAt.Before(ms[j].UpdatedAt); j-- {
				ms[j-1], ms[j] = ms[j], ms[j-1]
			}
		}
	default:
		// SortOrderNewest: ORDER BY CreatedDate DESC NULLS LAST
		for i := 1; i < len(ms); i++ {
			for j := i; j > 0 && ms[j-1].CreatedAt.Before(ms[j].CreatedAt); j-- {
				ms[j-1], ms[j] = ms[j], ms[j-1]
			}
		}
	}
}

// matchesMockMemberFilters is retained for use by mock list helpers that support
// free-text search and filter expressions over ProjectMembership records.
func matchesMockMemberFilters(ms *model.ProjectMembership, filters map[string]string, search string) bool {
	if search != "" {
		searchLower := strings.ToLower(search)
		found := strings.Contains(strings.ToLower(ms.CompanyName), searchLower) ||
			strings.Contains(strings.ToLower(ms.ProjectSlug), searchLower) ||
			strings.Contains(strings.ToLower(ms.Tier), searchLower) ||
			strings.Contains(strings.ToLower(ms.TierName), searchLower)

		if !found {
			return false
		}
	}

	for key, value := range filters {
		switch strings.ToLower(key) {
		case "status":
			// Status is hardcoded to "Active" in the filterMap; this case
			// mirrors the hardcoded AND Status = 'Active' in the SOQL base query.
			if !strings.EqualFold(ms.Status, value) {
				return false
			}
		case "tier_uid":
			if ms.TierUID != value {
				return false
			}
		case "project_slug":
			if !strings.EqualFold(ms.ProjectSlug, value) {
				return false
			}
		case "company_name":
			if !strings.Contains(strings.ToLower(ms.CompanyName), strings.ToLower(value)) {
				return false
			}
		}
	}

	return true
}

// MockB2BOrgReader provides a no-op mock implementation of port.B2BOrgReader
// for local development when REPOSITORY_SOURCE=mock.
type MockB2BOrgReader struct{}

// NewMockB2BOrgReader creates a new MockB2BOrgReader.
func NewMockB2BOrgReader() *MockB2BOrgReader {
	return &MockB2BOrgReader{}
}

// GetB2BOrg always returns not-found. Satisfies port.B2BOrgReader for local
// development without Salesforce.
func (m *MockB2BOrgReader) GetB2BOrg(_ context.Context, _ string) (*model.B2BOrg, error) {
	return nil, errors.NewNotFound("b2b org not found in mock")
}

// MockB2BOrgWriter is a stub implementation of port.B2BOrgWriter for local
// development when REPOSITORY_SOURCE=mock. All methods return NotImplemented.
type MockB2BOrgWriter struct{}

// NewMockB2BOrgWriter creates a new MockB2BOrgWriter.
func NewMockB2BOrgWriter() *MockB2BOrgWriter {
	return &MockB2BOrgWriter{}
}

// CreateB2BOrg always returns not-implemented.
func (m *MockB2BOrgWriter) CreateB2BOrg(_ context.Context, _ string, _ model.B2BOrgInput) (*model.B2BOrg, error) {
	return nil, errors.NewNotImplemented("create-b2b-org not implemented in mock")
}

// UpdateB2BOrg always returns not-implemented.
func (m *MockB2BOrgWriter) UpdateB2BOrg(_ context.Context, _ string, _ model.B2BOrgInput) (*model.B2BOrg, error) {
	return nil, errors.NewNotImplemented("update-b2b-org not implemented in mock")
}

// MockMemberPublisher is a no-op implementation of port.MemberPublisher for
// local development when MESSAGING_SOURCE=mock. All messages are logged but
// not published to NATS.
type MockMemberPublisher struct{}

// NewMockMemberPublisher creates a new MockMemberPublisher.
func NewMockMemberPublisher() *MockMemberPublisher {
	return &MockMemberPublisher{}
}

// Indexer logs the message and returns nil.
func (m *MockMemberPublisher) Indexer(ctx context.Context, subject string, _ any, _ bool) error {
	slog.DebugContext(ctx, "mock: indexer publish (no-op)", "subject", subject)
	return nil
}

// Access logs the message and returns nil.
func (m *MockMemberPublisher) Access(ctx context.Context, subject string, _ any, _ bool) error {
	slog.DebugContext(ctx, "mock: access publish (no-op)", "subject", subject)
	return nil
}

// MockKeyContactWriter is a stub implementation of port.KeyContactWriter for
// local development when REPOSITORY_SOURCE=mock. All methods return NotImplemented.
type MockKeyContactWriter struct{}

// NewMockKeyContactWriter creates a new MockKeyContactWriter.
func NewMockKeyContactWriter() *MockKeyContactWriter {
	return &MockKeyContactWriter{}
}

// CreateKeyContact always returns not-implemented.
func (m *MockKeyContactWriter) CreateKeyContact(_ context.Context, _ model.KeyContactInput) (*model.KeyContact, error) {
	return nil, errors.NewNotImplemented("create-key-contact not implemented in mock")
}

// UpdateKeyContact always returns not-implemented.
func (m *MockKeyContactWriter) UpdateKeyContact(_ context.Context, _ string, _ model.KeyContactInput) (*model.KeyContact, error) {
	return nil, errors.NewNotImplemented("update-key-contact not implemented in mock")
}

// DeleteKeyContact always returns not-implemented.
func (m *MockKeyContactWriter) DeleteKeyContact(_ context.Context, _ string, _ string) error {
	return errors.NewNotImplemented("delete-key-contact not implemented in mock")
}

// MockProjectMembershipReader is a stub implementation of port.ProjectMembershipReader
// for local development when REPOSITORY_SOURCE=mock.
type MockProjectMembershipReader struct{}

// NewMockProjectMembershipReader creates a new MockProjectMembershipReader.
func NewMockProjectMembershipReader() *MockProjectMembershipReader {
	return &MockProjectMembershipReader{}
}

// AssembleProjectMembership always returns not-found.
func (m *MockProjectMembershipReader) AssembleProjectMembership(_ context.Context, _ string) (*model.ProjectMembership, time.Time, error) {
	return nil, time.Time{}, errors.NewNotFound("project membership not found in mock")
}
