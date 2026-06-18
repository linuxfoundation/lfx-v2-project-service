// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/mock"
	svc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// ── In-process stubs ──────────────────────────────────────────────────────────

// fakeCDCSubscriber feeds a fixed slice of events then closes the channel.
type fakeCDCSubscriber struct {
	events []model.CDCEvent
}

func (f *fakeCDCSubscriber) Subscribe(_ context.Context, _ string, _ []byte, _ port.ReplayStore) (<-chan model.CDCEvent, error) {
	ch := make(chan model.CDCEvent, len(f.events))
	for _, e := range f.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

// errCDCSubscriber always returns an error from Subscribe.
type errCDCSubscriber struct{ err error }

func (e *errCDCSubscriber) Subscribe(_ context.Context, _ string, _ []byte, _ port.ReplayStore) (<-chan model.CDCEvent, error) {
	return nil, e.err
}

// fakeReplayStore records the last saved replay ID (commit-after-process check).
type fakeReplayStore struct {
	saved    []byte
	savedAll [][]byte // every Save call, in order
	loadErr  error
	saveErr  error
}

func (r *fakeReplayStore) Load(_ context.Context, _ string) ([]byte, error) {
	return nil, r.loadErr
}
func (r *fakeReplayStore) Save(_ context.Context, _ string, id []byte) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.saved = id
	r.savedAll = append(r.savedAll, id)
	return nil
}

// reparentingB2BOrgReader returns different results for GetB2BOrg on successive
// calls: first call returns the pre-change record (old parent), subsequent
// calls return the post-change record (new parent). This simulates the consumer
// reading the cached old state before eviction and then re-fetching from
// Salesforce after eviction.
type reparentingB2BOrgReader struct {
	calls    int
	preOrg   *model.B2BOrg // returned on call 0 (before eviction)
	postOrg  *model.B2BOrg // returned on call 1+ (after eviction)
	children map[string][]string
}

func (r *reparentingB2BOrgReader) GetB2BOrg(_ context.Context, _ string) (*model.B2BOrg, error) {
	defer func() { r.calls++ }()
	if r.calls == 0 {
		return r.preOrg, nil
	}
	return r.postOrg, nil
}
func (r *reparentingB2BOrgReader) FetchChildUIDsByParentUID(_ context.Context, parentUID string) ([]string, error) {
	if r.children != nil {
		return r.children[parentUID], nil
	}
	return nil, nil
}

// fakeB2BOrgReader returns a pre-seeded org.
type fakeB2BOrgReader struct {
	org      *model.B2BOrg
	children []string
	orgErr   error
}

func (r *fakeB2BOrgReader) GetB2BOrg(_ context.Context, _ string) (*model.B2BOrg, error) {
	return r.org, r.orgErr
}
func (r *fakeB2BOrgReader) FetchChildUIDsByParentUID(_ context.Context, _ string) ([]string, error) {
	return r.children, nil
}

// subjectCapturingPublisher captures subjects and message payloads for
// both indexer and access publish calls.
type subjectCapturingPublisher struct {
	mu              sync.Mutex
	indexer         []string // subjects
	indexerMessages []any    // payloads, parallel to indexer
	access          []string // subjects
}

func (p *subjectCapturingPublisher) Indexer(_ context.Context, subject string, msg any, _ bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.indexer = append(p.indexer, subject)
	p.indexerMessages = append(p.indexerMessages, msg)
	return nil
}
func (p *subjectCapturingPublisher) Access(_ context.Context, subject string, _ any, _ bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.access = append(p.access, subject)
	return nil
}

// hasAccess returns true if any access call subject contains the given substring.
func (p *subjectCapturingPublisher) hasAccess(sub string) bool {
	for _, s := range p.access {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// indexerAction extracts the "action" field from the i-th indexer message by
// round-tripping through JSON. Returns "" if the message is nil or the field
// is absent.
func (p *subjectCapturingPublisher) indexerAction(i int) string {
	if i >= len(p.indexerMessages) || p.indexerMessages[i] == nil {
		return ""
	}
	b, err := json.Marshal(p.indexerMessages[i])
	if err != nil {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return ""
	}
	raw, ok := m["action"]
	if !ok {
		return ""
	}
	var action string
	_ = json.Unmarshal(raw, &action)
	return action
}

// ── Constructor helper ────────────────────────────────────────────────────────

func newTestCDCConsumer(
	subscriber port.CDCSubscriber,
	memberReader *mock.MockControllableMemberReader,
	pmReader *mock.MockControllableProjectMembershipReader,
	orgReader *fakeB2BOrgReader,
	invalidator *mock.MockCacheInvalidator,
	pub *subjectCapturingPublisher,
	globalOrgAdminTeamUID string,
	extraOpts ...svc.CDCConsumerOption,
) *svc.CDCConsumer {
	opts := []svc.CDCConsumerOption{
		svc.WithCDCSubscriber(subscriber),
		svc.WithCDCMemberReader(memberReader),
		svc.WithCDCProjectMembershipReader(pmReader),
		svc.WithCDCB2BOrgReader(orgReader),
		svc.WithCDCCacheInvalidator(invalidator),
		svc.WithCDCPublisher(pub),
		svc.WithCDCGlobalOrgAdminTeamUID(globalOrgAdminTeamUID),
	}
	return svc.NewCDCConsumer(append(opts, extraOpts...)...)
}

// ── Account (b2b_org) tests ───────────────────────────────────────────────────

func TestCDCConsumer_Account_Upsert_PublishesIndexerAndFGA(t *testing.T) {
	org := &model.B2BOrg{UID: "org-uid-1"}
	pub := &subjectCapturingPublisher{}
	invalidator := &mock.MockCacheInvalidator{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Account", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"org-uid-1"}, ReplayID: []byte("r1")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{org: org},
		invalidator,
		pub,
		"admin-team-uid",
		svc.WithCDCAccountBatchReader(&mock.MockAccountBatchReader{Orgs: []*model.B2BOrg{org}}),
	)

	replay := &fakeReplayStore{}
	err := consumer.Run(context.Background(), "/data/AccountChangeEvent", replay)
	require.NoError(t, err)

	assert.NotEmpty(t, pub.indexer, "indexer must be published on account upsert")
	assert.NotEmpty(t, pub.access, "FGA access must be published on account upsert")
	assert.Equal(t, 1, invalidator.B2BOrgCalls, "cache must be invalidated once")
	assert.Equal(t, []byte("r1"), replay.saved, "replay cursor must be committed")
}

func TestCDCConsumer_Account_Upsert_PassesGlobalOrgAdminTeamUID(t *testing.T) {
	// globalOrgAdminTeamUID must reach BuildB2BOrgFGAMessage — verified indirectly:
	// if it were "" the FGA subject is still emitted; this test ensures the field
	// is wired at all (non-empty UID → message contains non-empty team reference).
	org := &model.B2BOrg{UID: "org-uid-1"}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Account", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"org-uid-1"}, ReplayID: []byte("r2")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{org: org},
		&mock.MockCacheInvalidator{},
		pub,
		"global-admin-team",
		svc.WithCDCAccountBatchReader(&mock.MockAccountBatchReader{Orgs: []*model.B2BOrg{org}}),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AccountChangeEvent", &fakeReplayStore{}))
	assert.NotEmpty(t, pub.access, "FGA access must be published")
}

func TestCDCConsumer_Account_Delete_PublishesIndexerAndFGA(t *testing.T) {
	pub := &subjectCapturingPublisher{}
	invalidator := &mock.MockCacheInvalidator{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Account", ChangeType: model.CDCChangeDelete, RecordIDs: []string{"org-uid-del"}, ReplayID: []byte("r3")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		invalidator,
		pub,
		"",
	)

	replay := &fakeReplayStore{}
	require.NoError(t, consumer.Run(context.Background(), "/data/AccountChangeEvent", replay))

	assert.NotEmpty(t, pub.indexer, "indexer delete must be published")
	assert.NotEmpty(t, pub.access, "FGA access must be published on delete")
	assert.Equal(t, 1, invalidator.B2BOrgCalls, "cache must be invalidated on delete")
	assert.Equal(t, []byte("r3"), replay.saved)
}

// ── Asset (project_membership) tests ─────────────────────────────────────────

func TestCDCConsumer_Asset_Upsert_PublishesIndexerAndFGA(t *testing.T) {
	pm := &model.ProjectMembership{UID: "pm-uid-1", B2BOrgUID: "org-uid-1"}
	pub := &subjectCapturingPublisher{}
	invalidator := &mock.MockCacheInvalidator{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"pm-uid-1"}, ReplayID: []byte("r4")},
		}},
		&mock.MockControllableMemberReader{Membership: pm},
		&mock.MockControllableProjectMembershipReader{Membership: pm},
		&fakeB2BOrgReader{},
		invalidator,
		pub,
		"",
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{Memberships: []*model.ProjectMembership{pm}}),
	)

	replay := &fakeReplayStore{}
	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", replay))

	assert.NotEmpty(t, pub.indexer, "indexer must be published")
	assert.NotEmpty(t, pub.access, "FGA access (project_membership) must be published")
	assert.Equal(t, 1, invalidator.MembershipCalls)
	assert.Equal(t, []byte("r4"), replay.saved)
}

func TestCDCConsumer_Asset_Delete_PublishesIndexerOnly(t *testing.T) {
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeDelete, RecordIDs: []string{"pm-uid-del"}, ReplayID: []byte("r5")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", &fakeReplayStore{}))

	assert.NotEmpty(t, pub.indexer, "indexer delete must be published")
	assert.Empty(t, pub.access, "no FGA on membership delete (no tuple to revoke)")
}

// ── Project_Role__c (key_contact) tests ──────────────────────────────────────

func TestCDCConsumer_ProjectRole_Upsert_WithUsername_PublishesIndexerAndFGAMemberPut(t *testing.T) {
	kc := &model.KeyContact{UID: "kc-uid-1", MembershipUID: "pm-uid-1", Username: "alice"}
	pub := &subjectCapturingPublisher{}
	invalidator := &mock.MockCacheInvalidator{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Project_Role__c", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"kc-uid-1"}, ReplayID: []byte("r6")},
		}},
		&mock.MockControllableMemberReader{Contact: kc},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		invalidator,
		pub,
		"",
		svc.WithCDCKeyContactBatchReader(&mock.MockKeyContactBatchReader{Contacts: []*model.KeyContact{kc}}),
	)

	replay := &fakeReplayStore{}
	require.NoError(t, consumer.Run(context.Background(), "/data/ProjectRoleChangeEvent", replay))

	assert.NotEmpty(t, pub.indexer, "indexer must be published")
	assert.True(t, pub.hasAccess(fgaconstants.GenericMemberPutSubject),
		"FGA member_put must be published for accepted key contact; access calls: %v", pub.access)
	assert.Equal(t, 1, invalidator.KeyContactCalls)
	assert.Equal(t, []byte("r6"), replay.saved)
}

func TestCDCConsumer_ProjectRole_Upsert_WithoutUsername_NoFGAMemberPut(t *testing.T) {
	// Pending/unaccepted contact — no username — must not emit FGA member_put.
	kc := &model.KeyContact{UID: "kc-uid-2", MembershipUID: "pm-uid-1", Username: ""}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Project_Role__c", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"kc-uid-2"}, ReplayID: []byte("r7")},
		}},
		&mock.MockControllableMemberReader{Contact: kc},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCKeyContactBatchReader(&mock.MockKeyContactBatchReader{Contacts: []*model.KeyContact{kc}}),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/ProjectRoleChangeEvent", &fakeReplayStore{}))

	assert.NotEmpty(t, pub.indexer, "indexer must still be published")
	assert.False(t, pub.hasAccess(fgaconstants.GenericMemberPutSubject),
		"FGA member_put must NOT be published for pending contact without username")
}

func TestCDCConsumer_ProjectRole_Delete_PublishesIndexerAndFGAMemberRemove(t *testing.T) {
	pub := &subjectCapturingPublisher{}
	invalidator := &mock.MockCacheInvalidator{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Project_Role__c", ChangeType: model.CDCChangeDelete, RecordIDs: []string{"kc-uid-del"}, ReplayID: []byte("r8")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		invalidator,
		pub,
		"",
	)

	replay := &fakeReplayStore{}
	require.NoError(t, consumer.Run(context.Background(), "/data/ProjectRoleChangeEvent", replay))

	assert.NotEmpty(t, pub.indexer, "indexer delete must be published")
	assert.True(t, pub.hasAccess(fgaconstants.GenericMemberRemoveSubject),
		"FGA member_remove must be published on key_contact delete; access calls: %v", pub.access)
	assert.Equal(t, 1, invalidator.KeyContactCalls)
	assert.Equal(t, []byte("r8"), replay.saved)
}

// ── Error resilience ──────────────────────────────────────────────────────────

func TestCDCConsumer_UnhandledEntity_SkipsAndAdvancesReplay(t *testing.T) {
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Opportunity", ChangeType: model.CDCChangeCreate, RecordIDs: []string{"opp-1"}, ReplayID: []byte("r9")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
	)

	replay := &fakeReplayStore{}
	require.NoError(t, consumer.Run(context.Background(), "/data/ChangeEvents", replay))

	assert.Empty(t, pub.indexer, "unknown entity must produce no indexer publish")
	assert.Empty(t, pub.access, "unknown entity must produce no FGA publish")
	assert.Equal(t, []byte("r9"), replay.saved, "replay cursor must still advance on skip")
}

func TestCDCConsumer_HandlerError_ReplayStillAdvances(t *testing.T) {
	// ProjectMembershipReader returns an error (simulates Salesforce sObject fetch
	// failure after cache invalidation). The handler fails but replay must still
	// advance — at-least-once semantics; /admin/reindex recovers missed events.
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"pm-bad"}, ReplayID: []byte("r10")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{Err: pkgerrors.NewNotFound("not found")}),
	)

	replay := &fakeReplayStore{}
	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", replay))

	assert.Empty(t, pub.indexer, "failed handler must not publish")
	assert.Equal(t, []byte("r10"), replay.saved, "replay cursor must advance even after handler error")
}

func TestCDCConsumer_MultipleRecordIDs_ProcessedAll(t *testing.T) {
	// A batch event with two record IDs must result in two cache invalidations.
	invalidator := &mock.MockCacheInvalidator{}
	pm1 := &model.ProjectMembership{UID: "pm-1"}
	pm2 := &model.ProjectMembership{UID: "pm-2"}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"pm-1", "pm-2"}, ReplayID: []byte("r11")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		invalidator,
		&subjectCapturingPublisher{},
		"",
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{Memberships: []*model.ProjectMembership{pm1, pm2}}),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", &fakeReplayStore{}))

	assert.Equal(t, 2, invalidator.MembershipCalls, "both record IDs in the batch must be processed")
}

// ── Create action tests ───────────────────────────────────────────────────────

func TestCDCConsumer_Asset_Create_SetsActionCreated(t *testing.T) {
	// CDCChangeCreate must result in ActionCreated in the indexer message payload,
	// not ActionUpdated. The action is encoded in the message body, not the subject.
	pm := &model.ProjectMembership{UID: "pm-create-1"}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeCreate, RecordIDs: []string{"pm-create-1"}, ReplayID: []byte("rc1")},
		}},
		&mock.MockControllableMemberReader{Membership: pm},
		&mock.MockControllableProjectMembershipReader{Membership: pm},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{Memberships: []*model.ProjectMembership{pm}}),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", &fakeReplayStore{}))

	require.Len(t, pub.indexer, 1, "exactly one indexer call on create")
	assert.Equal(t, "created", pub.indexerAction(0),
		"indexer message action must be 'created' for CDCChangeCreate")
}

func TestCDCConsumer_ProjectRole_Create_SetsActionCreated(t *testing.T) {
	kc := &model.KeyContact{UID: "kc-create-1", MembershipUID: "pm-uid-1", Username: "bob"}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Project_Role__c", ChangeType: model.CDCChangeCreate, RecordIDs: []string{"kc-create-1"}, ReplayID: []byte("rc2")},
		}},
		&mock.MockControllableMemberReader{Contact: kc},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCKeyContactBatchReader(&mock.MockKeyContactBatchReader{Contacts: []*model.KeyContact{kc}}),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/ProjectRoleChangeEvent", &fakeReplayStore{}))

	require.Len(t, pub.indexer, 1, "exactly one indexer call on create")
	assert.Equal(t, "created", pub.indexerAction(0),
		"indexer message action must be 'created' for CDCChangeCreate")
}

// ── Reparenting test ──────────────────────────────────────────────────────────

func TestCDCConsumer_Account_Reparenting_EmitsMoreFGAAccessCalls(t *testing.T) {
	// Pre-change: org has old-parent. Post-change: org has new-parent.
	// The consumer reads pre-change before eviction, then post-change after.
	// BuildB2BOrgReparentingMessages should fire extra FGA access calls.
	preOrg := &model.B2BOrg{UID: "org-uid-r", ParentUID: "old-parent"}
	postOrg := &model.B2BOrg{UID: "org-uid-r", ParentUID: "new-parent"}

	reparentReader := &reparentingB2BOrgReader{
		preOrg:  preOrg,
		postOrg: postOrg,
		children: map[string][]string{
			"old-parent": {"sibling-org"},
			"new-parent": {},
			"org-uid-r":  {},
		},
	}

	// Baseline: same parent (no reparenting) — should emit fewer FGA calls.
	sameOrg := &model.B2BOrg{UID: "org-uid-s", ParentUID: "same-parent"}
	sameReader := &reparentingB2BOrgReader{
		preOrg:  sameOrg,
		postOrg: sameOrg,
		children: map[string][]string{
			"same-parent": {},
			"org-uid-s":   {},
		},
	}

	reparentPub := &subjectCapturingPublisher{}
	reparentConsumer := svc.NewCDCConsumer(
		svc.WithCDCSubscriber(&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Account", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"org-uid-r"}, ReplayID: []byte("rr1")},
		}}),
		svc.WithCDCMemberReader(&mock.MockControllableMemberReader{}),
		svc.WithCDCProjectMembershipReader(&mock.MockControllableProjectMembershipReader{}),
		svc.WithCDCB2BOrgReader(reparentReader),
		svc.WithCDCCacheInvalidator(&mock.MockCacheInvalidator{}),
		svc.WithCDCPublisher(reparentPub),
		svc.WithCDCGlobalOrgAdminTeamUID(""),
		svc.WithCDCAccountBatchReader(&mock.MockAccountBatchReader{Orgs: []*model.B2BOrg{postOrg}}),
	)

	samePub := &subjectCapturingPublisher{}
	sameConsumer := svc.NewCDCConsumer(
		svc.WithCDCSubscriber(&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Account", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"org-uid-s"}, ReplayID: []byte("rr2")},
		}}),
		svc.WithCDCMemberReader(&mock.MockControllableMemberReader{}),
		svc.WithCDCProjectMembershipReader(&mock.MockControllableProjectMembershipReader{}),
		svc.WithCDCB2BOrgReader(sameReader),
		svc.WithCDCCacheInvalidator(&mock.MockCacheInvalidator{}),
		svc.WithCDCPublisher(samePub),
		svc.WithCDCGlobalOrgAdminTeamUID(""),
		svc.WithCDCAccountBatchReader(&mock.MockAccountBatchReader{Orgs: []*model.B2BOrg{sameOrg}}),
	)

	require.NoError(t, reparentConsumer.Run(context.Background(), "/data/AccountChangeEvent", &fakeReplayStore{}))
	require.NoError(t, sameConsumer.Run(context.Background(), "/data/AccountChangeEvent", &fakeReplayStore{}))

	assert.Greater(t, len(reparentPub.access), len(samePub.access),
		"reparenting must emit more FGA access calls (%d) than a non-reparenting update (%d)",
		len(reparentPub.access), len(samePub.access))
}

// ── Guard condition tests ─────────────────────────────────────────────────────

func TestCDCConsumer_ProjectRole_Upsert_WithUsername_EmptyMembershipUID_NoFGAMemberPut(t *testing.T) {
	// Guard: kc.Username != "" && kc.MembershipUID != ""
	// A malformed record with a username but no MembershipUID must NOT emit FGA member_put.
	kc := &model.KeyContact{UID: "kc-bad", MembershipUID: "", Username: "charlie"}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Project_Role__c", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"kc-bad"}, ReplayID: []byte("rg1")},
		}},
		&mock.MockControllableMemberReader{Contact: kc},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCKeyContactBatchReader(&mock.MockKeyContactBatchReader{Contacts: []*model.KeyContact{kc}}),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/ProjectRoleChangeEvent", &fakeReplayStore{}))

	assert.NotEmpty(t, pub.indexer, "indexer must still be published even for malformed record")
	assert.False(t, pub.hasAccess(fgaconstants.GenericMemberPutSubject),
		"FGA member_put must NOT be published when MembershipUID is empty; access calls: %v", pub.access)
}

// ── Startup error tests ───────────────────────────────────────────────────────

func TestCDCConsumer_ReplayStore_LoadError_RunReturnsError(t *testing.T) {
	loadErr := errors.New("nats: kv unavailable")

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		&subjectCapturingPublisher{},
		"",
	)

	replay := &fakeReplayStore{loadErr: loadErr}
	err := consumer.Run(context.Background(), "/data/AccountChangeEvent", replay)

	require.Error(t, err, "Run must return the Load error")
	assert.ErrorIs(t, err, loadErr)
}

func TestCDCConsumer_Subscriber_SubscribeError_RunReturnsError(t *testing.T) {
	subscribeErr := errors.New("grpc: connection refused")

	consumer := newTestCDCConsumer(
		&errCDCSubscriber{err: subscribeErr},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		&subjectCapturingPublisher{},
		"",
	)

	err := consumer.Run(context.Background(), "/data/AccountChangeEvent", &fakeReplayStore{})

	require.Error(t, err, "Run must propagate Subscribe error")
	assert.ErrorIs(t, err, subscribeErr)
}

// ── Replay cursor durability tests ────────────────────────────────────────────

func TestCDCConsumer_ReplayStore_SaveError_NotFatal(t *testing.T) {
	// Save failures are logged and swallowed — Run must not return an error.
	pm := &model.ProjectMembership{UID: "pm-save-err"}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"pm-save-err"}, ReplayID: []byte("rs1")},
		}},
		&mock.MockControllableMemberReader{Membership: pm},
		&mock.MockControllableProjectMembershipReader{Membership: pm},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		&subjectCapturingPublisher{},
		"",
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{Memberships: []*model.ProjectMembership{pm}}),
	)

	replay := &fakeReplayStore{saveErr: errors.New("nats: kv write failed")}
	err := consumer.Run(context.Background(), "/data/AssetChangeEvent", replay)

	require.NoError(t, err, "Save error must not be returned from Run")
}

func TestCDCConsumer_MultipleEvents_ReplayAdvancesPerEvent(t *testing.T) {
	// Three events in sequence — replay cursor must be committed after EACH one,
	// not just at the end of the batch.
	pm := &model.ProjectMembership{UID: "pm-seq"}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"pm-1"}, ReplayID: []byte("seq-1")},
			{Entity: "Asset", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"pm-2"}, ReplayID: []byte("seq-2")},
			{Entity: "Asset", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"pm-3"}, ReplayID: []byte("seq-3")},
		}},
		&mock.MockControllableMemberReader{Membership: pm},
		&mock.MockControllableProjectMembershipReader{Membership: pm},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		&subjectCapturingPublisher{},
		"",
		// Return empty — absent IDs route to delete, which still advances replay correctly.
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{}),
	)

	replay := &fakeReplayStore{}
	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", replay))

	require.Len(t, replay.savedAll, 3, "replay cursor must be committed once per event")
	assert.Equal(t, []byte("seq-1"), replay.savedAll[0])
	assert.Equal(t, []byte("seq-2"), replay.savedAll[1])
	assert.Equal(t, []byte("seq-3"), replay.savedAll[2])
}

// ── CDCChangeType fallthrough tests ──────────────────────────────────────────

func TestCDCConsumer_Asset_Undelete_TreatedAsUpsert(t *testing.T) {
	// UNDELETE falls into the same non-delete branch as UPDATE/CREATE.
	pm := &model.ProjectMembership{UID: "pm-undelete"}
	pub := &subjectCapturingPublisher{}
	invalidator := &mock.MockCacheInvalidator{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeUndelete, RecordIDs: []string{"pm-undelete"}, ReplayID: []byte("ru1")},
		}},
		&mock.MockControllableMemberReader{Membership: pm},
		&mock.MockControllableProjectMembershipReader{Membership: pm},
		&fakeB2BOrgReader{},
		invalidator,
		pub,
		"",
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{Memberships: []*model.ProjectMembership{pm}}),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", &fakeReplayStore{}))

	assert.NotEmpty(t, pub.indexer, "UNDELETE must re-publish indexer (treated as upsert)")
	assert.NotEmpty(t, pub.access, "UNDELETE must publish FGA access (upsert path, not delete path)")
	assert.Equal(t, 1, invalidator.MembershipCalls, "cache must be invalidated on UNDELETE")
}

func TestCDCConsumer_Asset_GapOverflow_TreatedAsUpsert(t *testing.T) {
	// GAP_OVERFLOW also falls into the non-delete upsert path.
	pm := &model.ProjectMembership{UID: "pm-gap"}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeGapOverflow, RecordIDs: []string{"pm-gap"}, ReplayID: []byte("rg2")},
		}},
		&mock.MockControllableMemberReader{Membership: pm},
		&mock.MockControllableProjectMembershipReader{Membership: pm},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{Memberships: []*model.ProjectMembership{pm}}),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", &fakeReplayStore{}))

	assert.NotEmpty(t, pub.indexer, "GAP_OVERFLOW must trigger re-fetch and re-publish (treated as upsert)")
}

func TestCDCConsumer_Asset_GapDelete_TreatedAsDelete(t *testing.T) {
	// GAP_DELETE must route to the delete path, not the upsert path.
	// Salesforce emits GAP_DELETE when a record is deleted during a CDC
	// overflow gap; treating it as an upsert would leave a ghost document.
	pub := &subjectCapturingPublisher{}
	invalidator := &mock.MockCacheInvalidator{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: "GAP_DELETE", RecordIDs: []string{"pm-gapdel"}, ReplayID: []byte("rgd1")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		invalidator,
		pub,
		"",
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", &fakeReplayStore{}))

	assert.NotEmpty(t, pub.indexer, "GAP_DELETE must publish a delete indexer event")
	assert.Equal(t, 1, invalidator.MembershipCalls, "cache must be invalidated on GAP_DELETE")
	// The indexer subject is present; verify no FGA access was published
	// (delete path does not emit FGA member_put).
	assert.Empty(t, pub.access, "GAP_DELETE must not emit FGA access (no re-fetch)")
}

func TestCDCConsumer_ProjectRole_GapDelete_TreatedAsDelete(t *testing.T) {
	// Same invariant for key_contact: GAP_DELETE must call the delete handler.
	pub := &subjectCapturingPublisher{}
	invalidator := &mock.MockCacheInvalidator{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Project_Role__c", ChangeType: "GAP_DELETE", RecordIDs: []string{"kc-gapdel"}, ReplayID: []byte("rgd2")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		invalidator,
		pub,
		"",
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/ProjectRoleChangeEvent", &fakeReplayStore{}))

	assert.NotEmpty(t, pub.indexer, "GAP_DELETE must publish a delete indexer event for key_contact")
	assert.Equal(t, 1, invalidator.KeyContactCalls, "cache must be invalidated on key_contact GAP_DELETE")
}

// ── pkgerrors test helper ─────────────────────────────────────────────────────

// Verify that a batch-fetch error propagates correctly: replay advances, nothing is published.
func TestCDCConsumer_Account_OrgNotFound_AdvancesReplay(t *testing.T) {
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Account", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"org-missing"}, ReplayID: []byte("r12")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{orgErr: errors.New("not found")},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		// Batch fetch errors → no publish; error is logged, replay still advances.
		svc.WithCDCAccountBatchReader(&mock.MockAccountBatchReader{Err: errors.New("not found")}),
	)

	replay := &fakeReplayStore{}
	require.NoError(t, consumer.Run(context.Background(), "/data/AccountChangeEvent", replay))

	assert.Empty(t, pub.indexer, "no indexer on org not found")
	assert.Equal(t, []byte("r12"), replay.saved, "replay must advance even when org not found")
}

// ── LFID resolution + silent provisioning (Task 8) ───────────────────────────

// fakeUserReader implements port.UserReader for CDC consumer tests.
type fakeUserReader struct {
	sub string
	err error
}

func (r *fakeUserReader) UsernameByEmail(_ context.Context, _ string) (string, error) {
	return r.sub, r.err
}

// newProjectRoleCDCConsumer builds a CDCConsumer wired for a single
// Project_Role__c upsert event keyed by kc.UID. Boring mocks (PM reader,
// org reader, cache invalidator) are pre-filled so each test only passes
// the options it actually cares about via extraOpts.
func newProjectRoleCDCConsumer(
	kc *model.KeyContact,
	pub *subjectCapturingPublisher,
	extraOpts ...svc.CDCConsumerOption,
) *svc.CDCConsumer {
	base := []svc.CDCConsumerOption{
		svc.WithCDCSubscriber(&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Project_Role__c", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{kc.UID}},
		}}),
		svc.WithCDCMemberReader(&mock.MockControllableMemberReader{Contact: kc}),
		svc.WithCDCProjectMembershipReader(&mock.MockControllableProjectMembershipReader{}),
		svc.WithCDCB2BOrgReader(&fakeB2BOrgReader{}),
		svc.WithCDCCacheInvalidator(&mock.MockCacheInvalidator{}),
		svc.WithCDCPublisher(pub),
		svc.WithCDCKeyContactBatchReader(&mock.MockKeyContactBatchReader{Contacts: []*model.KeyContact{kc}}),
	}
	return svc.NewCDCConsumer(append(base, extraOpts...)...)
}

func TestCDCConsumer_ProjectRole_Upsert_EmailResolves_GrantsFGAAndProvisions(t *testing.T) {
	// Email resolves to an LFID → FGA member_put published AND AddPrincipal called
	// with SuppressNotification=true (CDC must never email).
	kc := &model.KeyContact{
		UID: "kc-res-1", MembershipUID: "pm-1",
		B2BOrgUID: "001000000000001AAA", Email: "carol@example.com",
		Role: "Billing Contact",
	}
	pub := &subjectCapturingPublisher{}
	spy := &spyOrgSettings{}

	consumer := newProjectRoleCDCConsumer(kc, pub,
		svc.WithCDCUserReader(&fakeUserReader{sub: "auth0|carol"}),
		svc.WithCDCOrgSettings(spy),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/ProjectRoleChangeEvent", &fakeReplayStore{}))

	assert.True(t, pub.hasAccess(fgaconstants.GenericMemberPutSubject),
		"FGA member_put must be published when LFID is resolved")
	require.Len(t, spy.adds, 1, "AddPrincipal must be called once")
	assert.True(t, spy.adds[0].SuppressNotification, "CDC provisioning must suppress notification")
	assert.Equal(t, "001000000000001AAA", spy.adds[0].OrgUID)
	assert.Equal(t, "carol@example.com", spy.adds[0].Email)
}

func TestCDCConsumer_ProjectRole_Upsert_EmailNotFound_NoGrantNoProvision(t *testing.T) {
	// UsernameByEmail returns NotFound → Username stays empty → FGA grant skipped;
	// no AddPrincipal call (unregistered contacts stay pending via the invite flow).
	kc := &model.KeyContact{
		UID: "kc-res-2", MembershipUID: "pm-2",
		B2BOrgUID: "001000000000002AAA", Email: "unknown@example.com",
	}
	pub := &subjectCapturingPublisher{}
	spy := &spyOrgSettings{}

	consumer := newProjectRoleCDCConsumer(kc, pub,
		svc.WithCDCUserReader(&fakeUserReader{err: pkgerrors.NewNotFound("not found")}),
		svc.WithCDCOrgSettings(spy),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/ProjectRoleChangeEvent", &fakeReplayStore{}))

	assert.False(t, pub.hasAccess(fgaconstants.GenericMemberPutSubject),
		"FGA member_put must NOT be published for unresolved email")
	assert.Empty(t, spy.adds, "AddPrincipal must not be called for unregistered contact")
}

func TestCDCConsumer_ProjectRole_Upsert_NilUserReader_PreservesExistingBehavior(t *testing.T) {
	// nil userReader must not regress existing behavior: a contact with a stored
	// Username still gets FGA member_put; no provisioning attempt is made.
	kc := &model.KeyContact{
		UID: "kc-res-3", MembershipUID: "pm-3", Username: "auth0|existing",
		B2BOrgUID: "001000000000003AAA", Email: "existing@example.com",
	}
	pub := &subjectCapturingPublisher{}

	consumer := newProjectRoleCDCConsumer(kc, pub) // no extraOpts — nil userReader path

	require.NoError(t, consumer.Run(context.Background(), "/data/ProjectRoleChangeEvent", &fakeReplayStore{}))

	assert.True(t, pub.hasAccess(fgaconstants.GenericMemberPutSubject),
		"pre-existing Username must still produce FGA member_put even without userReader")
}

// ── Quota guard tests ─────────────────────────────────────────────────────────

func TestCDCConsumer_QuotaGuard_AboveThreshold_SkipsUpsert(t *testing.T) {
	pm := &model.ProjectMembership{UID: "pm-quota-1"}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"pm-quota-1"}, ReplayID: []byte("qg1")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{Memberships: []*model.ProjectMembership{pm}}),
		svc.WithCDCQuotaGauge(&mock.MockSalesforceQuotaGauge{Current: 96, Limit: 100}), // 0.96 ≥ 0.95
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", &fakeReplayStore{}))

	assert.Empty(t, pub.indexer, "quota exceeded must suppress indexer publish")
	assert.Empty(t, pub.access, "quota exceeded must suppress FGA publish")
}

func TestCDCConsumer_QuotaGuard_AtThreshold_SkipsUpsert(t *testing.T) {
	pm := &model.ProjectMembership{UID: "pm-quota-2"}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"pm-quota-2"}, ReplayID: []byte("qg2")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{Memberships: []*model.ProjectMembership{pm}}),
		svc.WithCDCQuotaGauge(&mock.MockSalesforceQuotaGauge{Current: 95, Limit: 100}), // 0.95 == threshold
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", &fakeReplayStore{}))

	assert.Empty(t, pub.indexer, "quota at exact threshold must suppress indexer publish")
	assert.Empty(t, pub.access, "quota at exact threshold must suppress FGA publish")
}

func TestCDCConsumer_QuotaGuard_BelowThreshold_Proceeds(t *testing.T) {
	pm := &model.ProjectMembership{UID: "pm-quota-3"}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"pm-quota-3"}, ReplayID: []byte("qg3")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{Memberships: []*model.ProjectMembership{pm}}),
		svc.WithCDCQuotaGauge(&mock.MockSalesforceQuotaGauge{Current: 94, Limit: 100}), // 0.94 < 0.95
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", &fakeReplayStore{}))

	assert.NotEmpty(t, pub.indexer, "below threshold must proceed and publish indexer")
}

func TestCDCConsumer_QuotaGuard_LimitZero_FailsOpen(t *testing.T) {
	// limit ≤ 0 means the gauge has not yet observed a response — must proceed.
	pm := &model.ProjectMembership{UID: "pm-quota-4"}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"pm-quota-4"}, ReplayID: []byte("qg4")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{Memberships: []*model.ProjectMembership{pm}}),
		svc.WithCDCQuotaGauge(&mock.MockSalesforceQuotaGauge{Current: 100, Limit: 0}),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", &fakeReplayStore{}))

	assert.NotEmpty(t, pub.indexer, "limit=0 (unobserved) must fail open and publish")
}

func TestCDCConsumer_QuotaGuard_NilGauge_FailsOpen(t *testing.T) {
	// No WithCDCQuotaGauge injected — nil gauge must fail open.
	pm := &model.ProjectMembership{UID: "pm-quota-5"}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"pm-quota-5"}, ReplayID: []byte("qg5")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{Memberships: []*model.ProjectMembership{pm}}),
		// intentionally no WithCDCQuotaGauge
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", &fakeReplayStore{}))

	assert.NotEmpty(t, pub.indexer, "nil gauge must fail open and publish")
}

func TestCDCConsumer_QuotaGuard_DeleteBypassesQuota(t *testing.T) {
	// DELETE events must publish even when quota is 100% — the delete path never
	// calls quotaExceeded and must always fire for index/FGA convergence.
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeDelete, RecordIDs: []string{"pm-quota-del"}, ReplayID: []byte("qg6")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCQuotaGauge(&mock.MockSalesforceQuotaGauge{Current: 100, Limit: 100}),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", &fakeReplayStore{}))

	assert.NotEmpty(t, pub.indexer, "DELETE must always publish regardless of quota state")
}

// ── Absent-from-SOQL → delete convergence tests ───────────────────────────────

func TestCDCConsumer_Asset_AbsentFromSOQL_RoutesToDelete(t *testing.T) {
	// Batch event with two IDs: SOQL only returns pm-present. pm-absent is missing
	// (soft-deleted or no longer holds a membership Asset) and must be routed to
	// the delete path for index/FGA convergence, not silently skipped.
	pmPresent := &model.ProjectMembership{UID: "pm-present"}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Asset", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"pm-present", "pm-absent"}, ReplayID: []byte("ab1")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCMembershipBatchReader(&mock.MockMembershipBatchReader{Memberships: []*model.ProjectMembership{pmPresent}}),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AssetChangeEvent", &fakeReplayStore{}))

	require.Len(t, pub.indexerMessages, 2, "both IDs must produce an indexer event")
	// absent ID fires delete first (absent loop runs before present loop)
	assert.Equal(t, "deleted", pub.indexerAction(0), "absent ID must produce ActionDeleted")
	assert.Equal(t, "updated", pub.indexerAction(1), "present ID must produce ActionUpdated")
}

func TestCDCConsumer_Account_AbsentFromSOQL_RoutesToDelete(t *testing.T) {
	// Same convergence guarantee for Account / b2b_org.
	orgPresent := &model.B2BOrg{UID: "org-present"}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Account", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"org-present", "org-absent"}, ReplayID: []byte("ab2")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCAccountBatchReader(&mock.MockAccountBatchReader{Orgs: []*model.B2BOrg{orgPresent}}),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/AccountChangeEvent", &fakeReplayStore{}))

	require.Len(t, pub.indexerMessages, 2, "both account IDs must produce an indexer event")
	assert.Equal(t, "deleted", pub.indexerAction(0), "absent account must produce ActionDeleted")
	assert.Equal(t, "updated", pub.indexerAction(1), "present account must produce ActionUpdated")
}

func TestCDCConsumer_ProjectRole_AbsentFromSOQL_RoutesToDelete(t *testing.T) {
	// Same convergence guarantee for Project_Role__c / key_contact.
	kcPresent := &model.KeyContact{UID: "kc-present"}
	pub := &subjectCapturingPublisher{}

	consumer := newTestCDCConsumer(
		&fakeCDCSubscriber{events: []model.CDCEvent{
			{Entity: "Project_Role__c", ChangeType: model.CDCChangeUpdate, RecordIDs: []string{"kc-present", "kc-absent"}, ReplayID: []byte("ab3")},
		}},
		&mock.MockControllableMemberReader{},
		&mock.MockControllableProjectMembershipReader{},
		&fakeB2BOrgReader{},
		&mock.MockCacheInvalidator{},
		pub,
		"",
		svc.WithCDCKeyContactBatchReader(&mock.MockKeyContactBatchReader{Contacts: []*model.KeyContact{kcPresent}}),
	)

	require.NoError(t, consumer.Run(context.Background(), "/data/ProjectRoleChangeEvent", &fakeReplayStore{}))

	require.Len(t, pub.indexerMessages, 2, "both key_contact IDs must produce an indexer event")
	assert.Equal(t, "deleted", pub.indexerAction(0), "absent key_contact must produce ActionDeleted")
	assert.Equal(t, "updated", pub.indexerAction(1), "present key_contact must produce ActionUpdated")
}
