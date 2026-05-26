// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	membershipservice "github.com/linuxfoundation/lfx-v2-member-service/gen/membership_service"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/mock"
	svc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

func TestBackfillRunner_FullMode_PublishesAllTypes(t *testing.T) {
	org := &model.B2BOrg{UID: "org-1"}
	pm := &model.ProjectMembership{UID: "pm-1"}
	kc := &model.KeyContact{UID: "kc-1"}

	var publishCount atomic.Int32
	pub := &countingPublisher{count: &publishCount}

	iter := &mock.MockBackfillIterator{
		B2BOrgs:     [][]*model.B2BOrg{{org}},
		Memberships: [][]*model.ProjectMembership{{pm}},
		KeyContacts: [][]*model.KeyContact{{kc}},
	}

	runner := svc.NewRunner(iter, mock.NewMockB2BOrgReader(), mock.NewMockProjectMembershipReader(), nil, pub, nil)
	req := svc.BackfillRequest{RunID: "test-run", Types: []string{"b2b_org", "project_membership", "key_contact"}}
	runner.Run(context.Background(), req)

	// 3 records × 1 publish each
	assert.Equal(t, int32(3), publishCount.Load(), "should publish one message per record")
}

func TestBackfillRunner_DryRun_DoesNotPublish(t *testing.T) {
	org := &model.B2BOrg{UID: "org-1"}

	var publishCount atomic.Int32
	pub := &countingPublisher{count: &publishCount}

	iter := &mock.MockBackfillIterator{
		B2BOrgs: [][]*model.B2BOrg{{org}},
	}

	runner := svc.NewRunner(iter, mock.NewMockB2BOrgReader(), mock.NewMockProjectMembershipReader(), nil, pub, nil)
	req := svc.BackfillRequest{RunID: "test-run", Types: []string{"b2b_org"}, DryRun: true}
	runner.Run(context.Background(), req)

	assert.Equal(t, int32(0), publishCount.Load(), "dry_run must not publish")
}

func TestBackfillRunner_MidRunError_OtherTypesStillRun(t *testing.T) {
	iterErr := errors.New("salesforce timeout")
	pm := &model.ProjectMembership{UID: "pm-1"}

	var publishCount atomic.Int32
	pub := &countingPublisher{count: &publishCount}

	iter := &mock.MockBackfillIterator{
		B2BErr:      iterErr,                            // b2b_org fails
		Memberships: [][]*model.ProjectMembership{{pm}}, // project_membership succeeds
	}

	runner := svc.NewRunner(iter, mock.NewMockB2BOrgReader(), mock.NewMockProjectMembershipReader(), nil, pub, nil)
	req := svc.BackfillRequest{RunID: "test-run", Types: []string{"b2b_org", "project_membership"}}
	runner.Run(context.Background(), req)

	// b2b_org fails → 0 publishes; project_membership succeeds → 1 publish
	assert.Equal(t, int32(1), publishCount.Load(), "error in one type must not stop other types")
}

func TestBackfillRunner_SinceFilter_PassedThroughToIterator(t *testing.T) {
	since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	var capturedSince *time.Time

	iter := &capturingSinceIterator{capturedSince: &capturedSince}
	runner := svc.NewRunner(iter, mock.NewMockB2BOrgReader(), mock.NewMockProjectMembershipReader(), nil, mock.NewMockMemberPublisher(), nil)
	req := svc.BackfillRequest{RunID: "test-run", Types: []string{"b2b_org"}, Since: &since}
	runner.Run(context.Background(), req)

	require.NotNil(t, capturedSince, "since must be forwarded to the iterator")
	assert.Equal(t, since, *capturedSince)
}

func TestBackfillRunner_TargetedMode_FetchesLiveSObjectAndPublishes(t *testing.T) {
	const orgUID = "00000000-0000-0000-0000-000000000001"
	org := &model.B2BOrg{UID: orgUID}
	b2bReader := &seededB2BOrgReaderForBackfill{org: org}

	var publishCount atomic.Int32
	pub := &countingPublisher{count: &publishCount}

	runner := svc.NewRunner(&mock.MockBackfillIterator{}, b2bReader, mock.NewMockProjectMembershipReader(), nil, pub, nil)
	req := svc.BackfillRequest{
		RunID: "test-run",
		Items: []svc.ReindexItem{{Type: "b2b_org", UID: orgUID}},
	}
	runner.Run(context.Background(), req)

	assert.Equal(t, int32(1), publishCount.Load(), "targeted mode should publish the fetched record")
}

func TestBackfillRunner_TargetedMode_NotFoundIsSkipped(t *testing.T) {
	const orgUID = "00000000-0000-0000-0000-000000000002"

	var publishCount atomic.Int32
	pub := &countingPublisher{count: &publishCount}

	// MockB2BOrgReader always returns not-found
	runner := svc.NewRunner(&mock.MockBackfillIterator{}, mock.NewMockB2BOrgReader(), mock.NewMockProjectMembershipReader(), nil, pub, nil)
	req := svc.BackfillRequest{
		RunID: "test-run",
		Items: []svc.ReindexItem{{Type: "b2b_org", UID: orgUID}},
	}
	runner.Run(context.Background(), req)

	assert.Equal(t, int32(0), publishCount.Load(), "not-found items must not publish")
}

// ── Helpers ────────────────────────────────────────────────────────────────

// countingPublisher counts how many times Indexer is called.
type countingPublisher struct {
	count *atomic.Int32
}

func (p *countingPublisher) Indexer(_ context.Context, _ string, _ any, _ bool) error {
	p.count.Add(1)
	return nil
}

func (p *countingPublisher) Access(_ context.Context, _ string, _ any, _ bool) error { return nil }

// capturingSinceIterator captures the since parameter passed to IterB2BOrgs.
type capturingSinceIterator struct {
	capturedSince **time.Time
}

func (c *capturingSinceIterator) IterB2BOrgs(_ context.Context, since *time.Time, _ func([]*model.B2BOrg) error) error {
	*c.capturedSince = since
	return nil
}

func (c *capturingSinceIterator) IterProjectMemberships(_ context.Context, _ *time.Time, _ func([]*model.ProjectMembership) error) error {
	return nil
}

func (c *capturingSinceIterator) IterKeyContacts(_ context.Context, _ *time.Time, _ func([]*model.KeyContact) error) error {
	return nil
}

// seededB2BOrgReaderForBackfill returns a fixed org for any UID.
type seededB2BOrgReaderForBackfill struct{ org *model.B2BOrg }

func (r *seededB2BOrgReaderForBackfill) GetB2BOrg(_ context.Context, _ string) (*model.B2BOrg, error) {
	return r.org, nil
}

func (r *seededB2BOrgReaderForBackfill) FetchChildUIDsByParentUID(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

// ── ValidateAndBuildRequest ──────────────────────────────────────────────────

func TestValidateAndBuildRequest_Since_ZonelessTimestamp_ReturnsValidationError(t *testing.T) {
	payload := &membershipservice.AdminReindexPayload{
		Since: strPtr("2026-05-20T00:00:00"), // no zone offset — must be rejected
	}
	_, err := svc.ValidateAndBuildRequest(payload)
	require.Error(t, err, "zone-less RFC 3339 timestamp must be rejected")
	var valErr pkgerrors.Validation
	assert.ErrorAs(t, err, &valErr, "expected Validation error, got: %v", err)
}
