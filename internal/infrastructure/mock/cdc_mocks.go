// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package mock

import (
	"context"
	"time"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
)

// compile-time interface checks.
var (
	_ port.MemberReader            = (*MockControllableMemberReader)(nil)
	_ port.CacheInvalidator        = (*MockCacheInvalidator)(nil)
	_ port.ProjectMembershipReader = (*MockControllableProjectMembershipReader)(nil)
	_ port.MembershipBatchReader   = (*MockMembershipBatchReader)(nil)
	_ port.KeyContactBatchReader   = (*MockKeyContactBatchReader)(nil)
	_ port.AccountBatchReader      = (*MockAccountBatchReader)(nil)
	_ port.SalesforceQuotaGauge    = (*MockSalesforceQuotaGauge)(nil)
)

// MockControllableMemberReader is a test double for port.MemberReader that
// returns caller-supplied values for GetMembership and GetKeyContact.
// All other MemberReader methods are no-ops that return empty results.
//
// Use when a test needs to inject a specific record or a specific error for a
// single-record lookup, without pre-seeding the full MockMembershipRepository.
type MockControllableMemberReader struct {
	Membership *model.ProjectMembership
	Contact    *model.KeyContact
	MemberErr  error
	ContactErr error
}

func (r *MockControllableMemberReader) GetMembership(_ context.Context, _ string) (*model.ProjectMembership, error) {
	return r.Membership, r.MemberErr
}
func (r *MockControllableMemberReader) GetKeyContact(_ context.Context, _ string) (*model.KeyContact, error) {
	return r.Contact, r.ContactErr
}
func (r *MockControllableMemberReader) ListTiersForProject(_ context.Context, _ string) ([]*model.MembershipTier, error) {
	return nil, nil
}
func (r *MockControllableMemberReader) GetTier(_ context.Context, _ string) (*model.MembershipTier, error) {
	return nil, nil
}
func (r *MockControllableMemberReader) ListMembershipsForProject(_ context.Context, _ string, _ model.MembershipFilters, _ int) (model.MembershipPage, error) {
	return model.MembershipPage{}, nil
}
func (r *MockControllableMemberReader) ListKeyContactsForMembership(_ context.Context, _ string) ([]*model.KeyContact, error) {
	return nil, nil
}
func (r *MockControllableMemberReader) ListKeyContactsForOrg(_ context.Context, _ string) ([]*model.KeyContact, error) {
	return nil, nil
}
func (r *MockControllableMemberReader) IsReady(_ context.Context) error { return nil }

// MockControllableProjectMembershipReader is a test double for
// port.ProjectMembershipReader that returns a caller-supplied ProjectMembership
// or error from AssembleProjectMembership.
//
// Use in CDC consumer tests to verify that the sObject re-fetch path is taken
// after cache invalidation, independently of the membership-cache SOQL path.
type MockControllableProjectMembershipReader struct {
	Membership  *model.ProjectMembership
	AssembleErr error
}

func (r *MockControllableProjectMembershipReader) AssembleProjectMembership(_ context.Context, _ string) (*model.ProjectMembership, time.Time, error) {
	return r.Membership, time.Time{}, r.AssembleErr
}

// MockCacheInvalidator is a test double for port.CacheInvalidator that counts
// calls per entity type and optionally returns a configured error.
//
// Use when a test needs to assert that cache invalidation was triggered the
// correct number of times, or to simulate an invalidation failure.
type MockCacheInvalidator struct {
	B2BOrgCalls     int
	MembershipCalls int
	KeyContactCalls int

	// InvalidateErr is returned by all three methods when non-nil.
	InvalidateErr error
}

func (c *MockCacheInvalidator) InvalidateB2BOrg(_ context.Context, _ string) error {
	c.B2BOrgCalls++
	return c.InvalidateErr
}
func (c *MockCacheInvalidator) InvalidateProjectMembership(_ context.Context, _ string) error {
	c.MembershipCalls++
	return c.InvalidateErr
}
func (c *MockCacheInvalidator) InvalidateKeyContact(_ context.Context, _ string) error {
	c.KeyContactCalls++
	return c.InvalidateErr
}

// MockMembershipBatchReader is a test double for port.MembershipBatchReader
// that returns a caller-supplied slice or error from FetchMembershipsBySFIDs.
type MockMembershipBatchReader struct {
	Memberships []*model.ProjectMembership
	Err         error
}

func (r *MockMembershipBatchReader) FetchMembershipsBySFIDs(_ context.Context, _ []string) ([]*model.ProjectMembership, []string, error) {
	return r.Memberships, nil, r.Err
}

// MockKeyContactBatchReader is a test double for port.KeyContactBatchReader
// that returns a caller-supplied slice or error from FetchKeyContactsBySFIDs.
type MockKeyContactBatchReader struct {
	Contacts []*model.KeyContact
	Err      error
}

func (r *MockKeyContactBatchReader) FetchKeyContactsBySFIDs(_ context.Context, _ []string) ([]*model.KeyContact, []string, error) {
	return r.Contacts, nil, r.Err
}

// MockAccountBatchReader is a test double for port.AccountBatchReader that
// returns a caller-supplied slice or error from FetchAccountsBySFIDs.
type MockAccountBatchReader struct {
	Orgs []*model.B2BOrg
	Err  error
}

func (r *MockAccountBatchReader) FetchAccountsBySFIDs(_ context.Context, _ []string) ([]*model.B2BOrg, []string, error) {
	return r.Orgs, nil, r.Err
}

// MockSalesforceQuotaGauge is a test double for port.SalesforceQuotaGauge.
// Set Current and Limit to simulate quota states; both default to -1
// (unobserved — the guard fails open).
type MockSalesforceQuotaGauge struct {
	Current int64
	Limit   int64
}

func NewMockSalesforceQuotaGauge() *MockSalesforceQuotaGauge {
	return &MockSalesforceQuotaGauge{Current: -1, Limit: -1}
}

func (g *MockSalesforceQuotaGauge) APIUsage() (current, limit int64) {
	return g.Current, g.Limit
}
