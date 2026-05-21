// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"time"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/salesforce"
)

// BackfillIterator provides paged SOQL iterators for full and since-filtered
// backfill modes. Each method calls fn once per page of converted records.
type BackfillIterator interface {
	IterB2BOrgs(ctx context.Context, since *time.Time, fn func([]*model.B2BOrg) error) error
	IterProjectMemberships(ctx context.Context, since *time.Time, fn func([]*model.ProjectMembership) error) error
	IterKeyContacts(ctx context.Context, since *time.Time, fn func([]*model.KeyContact) error) error
}

// salesforceBackfillIterator is the production BackfillIterator backed by the
// Salesforce SOQL repos.
type salesforceBackfillIterator struct {
	accounts    *salesforce.AccountRepo
	memberships *salesforce.MembershipRepo
	keyContacts *salesforce.KeyContactRepo
}

func newSalesforceBackfillIterator(
	accounts *salesforce.AccountRepo,
	memberships *salesforce.MembershipRepo,
	keyContacts *salesforce.KeyContactRepo,
) BackfillIterator {
	return &salesforceBackfillIterator{
		accounts:    accounts,
		memberships: memberships,
		keyContacts: keyContacts,
	}
}

func (s *salesforceBackfillIterator) IterB2BOrgs(ctx context.Context, since *time.Time, fn func([]*model.B2BOrg) error) error {
	return s.accounts.IterB2BOrgs(ctx, since, fn)
}

func (s *salesforceBackfillIterator) IterProjectMemberships(ctx context.Context, since *time.Time, fn func([]*model.ProjectMembership) error) error {
	return s.memberships.IterProjectMemberships(ctx, since, fn)
}

func (s *salesforceBackfillIterator) IterKeyContacts(ctx context.Context, since *time.Time, fn func([]*model.KeyContact) error) error {
	return s.keyContacts.IterKeyContacts(ctx, since, fn)
}

// mockBackfillIterator is the test BackfillIterator.
type mockBackfillIterator struct {
	b2bOrgs     [][]*model.B2BOrg
	memberships [][]*model.ProjectMembership
	keyContacts [][]*model.KeyContact
	b2bErr      error
	pmErr       error
	kcErr       error
}

func (m *mockBackfillIterator) IterB2BOrgs(_ context.Context, _ *time.Time, fn func([]*model.B2BOrg) error) error {
	for _, page := range m.b2bOrgs {
		if err := fn(page); err != nil {
			return err
		}
	}
	return m.b2bErr
}

func (m *mockBackfillIterator) IterProjectMemberships(_ context.Context, _ *time.Time, fn func([]*model.ProjectMembership) error) error {
	for _, page := range m.memberships {
		if err := fn(page); err != nil {
			return err
		}
	}
	return m.pmErr
}

func (m *mockBackfillIterator) IterKeyContacts(_ context.Context, _ *time.Time, fn func([]*model.KeyContact) error) error {
	for _, page := range m.keyContacts {
		if err := fn(page); err != nil {
			return err
		}
	}
	return m.kcErr
}
