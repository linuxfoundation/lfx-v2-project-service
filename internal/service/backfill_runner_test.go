// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"sync"
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

	runner := svc.NewRunner(iter, mock.NewMockB2BOrgReader(), mock.NewMockProjectMembershipReader(), nil, nil, pub, nil)
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

	runner := svc.NewRunner(iter, mock.NewMockB2BOrgReader(), mock.NewMockProjectMembershipReader(), nil, nil, pub, nil)
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

	runner := svc.NewRunner(iter, mock.NewMockB2BOrgReader(), mock.NewMockProjectMembershipReader(), nil, nil, pub, nil)
	req := svc.BackfillRequest{RunID: "test-run", Types: []string{"b2b_org", "project_membership"}}
	runner.Run(context.Background(), req)

	// b2b_org fails → 0 publishes; project_membership succeeds → 1 publish
	assert.Equal(t, int32(1), publishCount.Load(), "error in one type must not stop other types")
}

func TestBackfillRunner_SinceFilter_PassedThroughToIterator(t *testing.T) {
	since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	var capturedSince *time.Time

	iter := &capturingSinceIterator{capturedSince: &capturedSince}
	runner := svc.NewRunner(iter, mock.NewMockB2BOrgReader(), mock.NewMockProjectMembershipReader(), nil, nil, mock.NewMockMemberPublisher(), nil)
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

	runner := svc.NewRunner(&mock.MockBackfillIterator{}, b2bReader, mock.NewMockProjectMembershipReader(), nil, nil, pub, nil)
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
	runner := svc.NewRunner(&mock.MockBackfillIterator{}, mock.NewMockB2BOrgReader(), mock.NewMockProjectMembershipReader(), nil, nil, pub, nil)
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

// seededB2BOrgReaderWithChildren returns orgs with configurable child relationships and tracks fetch calls.
type seededB2BOrgReaderWithChildren struct {
	orgs             []*model.B2BOrg
	children         map[string][]string // parentUID → childUIDs
	fetchCallCount   atomic.Int32
	fetchedUIDs      map[string]bool // tracks which UIDs were fetched
	fetchedUIDsMutex sync.Mutex
}

func (r *seededB2BOrgReaderWithChildren) GetB2BOrg(ctx context.Context, uid string) (*model.B2BOrg, error) {
	for _, org := range r.orgs {
		if org.UID == uid {
			return org, nil
		}
	}
	return nil, pkgerrors.NewNotFound("b2b org not found")
}

func (r *seededB2BOrgReaderWithChildren) FetchChildUIDsByParentUID(_ context.Context, parentUID string) ([]string, error) {
	r.fetchCallCount.Add(1)
	r.fetchedUIDsMutex.Lock()
	if r.fetchedUIDs != nil {
		r.fetchedUIDs[parentUID] = true
	}
	r.fetchedUIDsMutex.Unlock()
	if r.children != nil {
		if uids, ok := r.children[parentUID]; ok {
			return uids, nil
		}
	}
	return []string{}, nil
}

func (r *seededB2BOrgReaderWithChildren) getFetchCallCount() int32 {
	return r.fetchCallCount.Load()
}

// ── Children field in backfill tests ───────────────────────────────────────

func TestBackfillRunner_B2BOrgs_PopulatesChildrenFromCache(t *testing.T) {
	// Parent org with two children
	parentOrg := &model.B2BOrg{UID: "parent-uid"}
	child1Org := &model.B2BOrg{UID: "child-1-uid"}
	child2Org := &model.B2BOrg{UID: "child-2-uid"}

	var publishCount atomic.Int32
	pub := &countingPublisher{count: &publishCount}

	childReader := &seededB2BOrgReaderWithChildren{
		orgs: []*model.B2BOrg{parentOrg, child1Org, child2Org},
		children: map[string][]string{
			"parent-uid":  {"child-1-uid", "child-2-uid"},
			"child-1-uid": {},
			"child-2-uid": {},
		},
		fetchedUIDs: map[string]bool{},
	}

	iter := &mock.MockBackfillIterator{
		B2BOrgs: [][]*model.B2BOrg{{parentOrg, child1Org, child2Org}},
	}

	runner := svc.NewRunner(iter, childReader, mock.NewMockProjectMembershipReader(), nil, nil, pub, nil)
	req := svc.BackfillRequest{RunID: "test-run", Types: []string{"b2b_org"}}
	runner.Run(context.Background(), req)

	// All 3 orgs should be published (parent + 2 children)
	assert.Equal(t, int32(3), publishCount.Load(), "should publish all orgs")

	// Parent should have children populated
	assert.True(t, childReader.fetchedUIDs["parent-uid"], "parent UID should be fetched for children")
	// Child UIDs should also be fetched (for their own children) but not others
	assert.True(t, childReader.fetchedUIDs["child-1-uid"], "child-1 UID should be fetched")
	assert.True(t, childReader.fetchedUIDs["child-2-uid"], "child-2 UID should be fetched")
}

func TestBackfillRunner_B2BOrgs_MemoizesFetchesPerPage(t *testing.T) {
	// Two parent orgs in same page sharing the same parent — should only fetch once
	parentOrg := &model.B2BOrg{UID: "shared-parent-uid"}
	child1 := &model.B2BOrg{UID: "child-1-uid", ParentUID: "shared-parent-uid"}
	child2 := &model.B2BOrg{UID: "child-2-uid", ParentUID: "shared-parent-uid"}

	var publishCount atomic.Int32
	pub := &countingPublisher{count: &publishCount}

	childReader := &seededB2BOrgReaderWithChildren{
		orgs: []*model.B2BOrg{parentOrg, child1, child2},
		children: map[string][]string{
			"shared-parent-uid": {"child-1-uid", "child-2-uid"},
			"child-1-uid":       {},
			"child-2-uid":       {},
		},
		fetchedUIDs: map[string]bool{},
	}

	// Put all 3 orgs in the same page to trigger memoization
	iter := &mock.MockBackfillIterator{
		B2BOrgs: [][]*model.B2BOrg{{parentOrg, child1, child2}},
	}

	runner := svc.NewRunner(iter, childReader, mock.NewMockProjectMembershipReader(), nil, nil, pub, nil)
	req := svc.BackfillRequest{RunID: "test-run", Types: []string{"b2b_org"}}
	runner.Run(context.Background(), req)

	// Even though we have 3 orgs, the shared parent should only be fetched once
	// Fetch calls should be: shared-parent-uid (once), child-1-uid (once), child-2-uid (once) = 3 total
	expectedFetches := int32(3)
	assert.Equal(t, expectedFetches, childReader.getFetchCallCount(),
		"should memoize parent children fetches within a page")
}

func TestBackfillRunner_TargetedB2BOrg_PopulatesChildren(t *testing.T) {
	const orgUID = "parent-org-uid"
	org := &model.B2BOrg{UID: orgUID}

	var publishCount atomic.Int32
	pub := &countingPublisher{count: &publishCount}

	childReader := &seededB2BOrgReaderWithChildren{
		orgs: []*model.B2BOrg{org},
		children: map[string][]string{
			orgUID: {"child-1", "child-2"},
		},
		fetchedUIDs: map[string]bool{},
	}

	runner := svc.NewRunner(&mock.MockBackfillIterator{}, childReader, mock.NewMockProjectMembershipReader(), nil, nil, pub, nil)
	req := svc.BackfillRequest{
		RunID: "test-run",
		Items: []svc.ReindexItem{{Type: "b2b_org", UID: orgUID}},
	}
	runner.Run(context.Background(), req)

	assert.Equal(t, int32(1), publishCount.Load(), "targeted mode should publish the org")
	assert.True(t, childReader.fetchedUIDs[orgUID], "should fetch children for targeted org")
}

// ── B2BOrgSettings backfill tests ─────────────────────────────────────────────

func TestBackfillRunner_Settings_FullMode_PublishesOnePerUID(t *testing.T) {
	const uid1 = "00000000-0000-0000-0000-000000000001"
	const uid2 = "00000000-0000-0000-0000-000000000002"

	org1 := &model.B2BOrg{UID: uid1}
	org2 := &model.B2BOrg{UID: uid2}
	settings1 := &model.B2BOrgSettings{UID: uid1, Writers: []model.B2BOrgUser{{Username: "auth0|alice", Email: "alice@acme.com", InvitedAs: "writer", InviteStatus: model.InviteStatusAccepted}}}
	settings2 := &model.B2BOrgSettings{UID: uid2, Writers: []model.B2BOrgUser{{Username: "auth0|bob", Email: "bob@acme.com", InvitedAs: "writer", InviteStatus: model.InviteStatusAccepted}}}

	settingsStore := mock.NewMockB2BOrgSettings()
	settingsStore.Seed(uid1, settings1, 1)
	settingsStore.Seed(uid2, settings2, 1)

	b2bReader := &seededB2BOrgReaderForBackfill{org: org1}
	b2bMultiReader := &multiOrgReader{orgs: map[string]*model.B2BOrg{uid1: org1, uid2: org2}}

	var publishCount atomic.Int32
	pub := &countingPublisher{count: &publishCount}

	runner := svc.NewRunner(&mock.MockBackfillIterator{}, b2bMultiReader, mock.NewMockProjectMembershipReader(), nil, settingsStore, pub, nil)
	req := svc.BackfillRequest{RunID: "test-run", Types: []string{"b2b_org_settings"}}
	runner.Run(context.Background(), req)

	assert.Equal(t, int32(2), publishCount.Load(), "should publish one indexer message per settings UID")
	_ = b2bReader
}

func TestBackfillRunner_Settings_TargetedMode_PublishesOne(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000011"

	org := &model.B2BOrg{UID: uid}
	settings := &model.B2BOrgSettings{UID: uid, Writers: []model.B2BOrgUser{{Username: "auth0|alice", Email: "alice@acme.com", InvitedAs: "writer", InviteStatus: model.InviteStatusAccepted}}}

	settingsStore := mock.NewMockB2BOrgSettings()
	settingsStore.Seed(uid, settings, 1)

	var publishCount atomic.Int32
	pub := &countingPublisher{count: &publishCount}

	runner := svc.NewRunner(&mock.MockBackfillIterator{}, &seededB2BOrgReaderForBackfill{org: org}, mock.NewMockProjectMembershipReader(), nil, settingsStore, pub, nil)
	req := svc.BackfillRequest{
		RunID: "test-run",
		Items: []svc.ReindexItem{{Type: "b2b_org_settings", UID: uid}},
	}
	runner.Run(context.Background(), req)

	assert.Equal(t, int32(1), publishCount.Load(), "targeted settings backfill should publish exactly one message")
}

func TestBackfillRunner_Settings_TargetedMode_OrgNotFound_Skips(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000022"

	settingsStore := mock.NewMockB2BOrgSettings()
	settingsStore.Seed(uid, &model.B2BOrgSettings{UID: uid}, 1)

	var publishCount atomic.Int32
	pub := &countingPublisher{count: &publishCount}

	// MockB2BOrgReader always returns not-found
	runner := svc.NewRunner(&mock.MockBackfillIterator{}, mock.NewMockB2BOrgReader(), mock.NewMockProjectMembershipReader(), nil, settingsStore, pub, nil)
	req := svc.BackfillRequest{
		RunID: "test-run",
		Items: []svc.ReindexItem{{Type: "b2b_org_settings", UID: uid}},
	}
	runner.Run(context.Background(), req)

	assert.Equal(t, int32(0), publishCount.Load(), "org-not-found must not publish")
}

func TestBackfillRunner_Settings_TargetedMode_SettingsAbsent_Skips(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000033"

	org := &model.B2BOrg{UID: uid}
	// settingsStore is empty — GetSettings returns (nil, 0, nil)
	settingsStore := mock.NewMockB2BOrgSettings()

	var publishCount atomic.Int32
	pub := &countingPublisher{count: &publishCount}

	runner := svc.NewRunner(&mock.MockBackfillIterator{}, &seededB2BOrgReaderForBackfill{org: org}, mock.NewMockProjectMembershipReader(), nil, settingsStore, pub, nil)
	req := svc.BackfillRequest{
		RunID: "test-run",
		Items: []svc.ReindexItem{{Type: "b2b_org_settings", UID: uid}},
	}
	runner.Run(context.Background(), req)

	assert.Equal(t, int32(0), publishCount.Load(), "absent settings must not publish")
}

// multiOrgReader returns a different org per UID.
type multiOrgReader struct {
	orgs map[string]*model.B2BOrg
}

func (r *multiOrgReader) GetB2BOrg(_ context.Context, uid string) (*model.B2BOrg, error) {
	if org, ok := r.orgs[uid]; ok {
		return org, nil
	}
	return nil, pkgerrors.NewNotFound("b2b org not found")
}

func (r *multiOrgReader) FetchChildUIDsByParentUID(_ context.Context, _ string) ([]string, error) {
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
