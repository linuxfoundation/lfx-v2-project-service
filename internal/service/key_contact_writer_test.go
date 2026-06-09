// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/mock"
	svc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testMembershipUID = "00000000-0000-0000-0000-000000000010"
	testKCUID         = "00000000-0000-0000-0000-000000000020"
)

// ── Helpers ────────────────────────────────────────────────────────────────

// trackingPublisher records (subject, call order) to verify publish sequencing.
type trackingPublisher struct {
	mu  sync.Mutex
	log []string // subject per call
}

func (p *trackingPublisher) Indexer(_ context.Context, subject string, _ any, _ bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.log = append(p.log, "indexer:"+subject)
	return nil
}

func (p *trackingPublisher) Access(_ context.Context, subject string, _ any, _ bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.log = append(p.log, "access:"+subject)
	return nil
}

func (p *trackingPublisher) calls() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.log))
	copy(out, p.log)
	return out
}

// errorPublisher returns an error from Access (FGA remove) to test error propagation.
type errorFGARemovePublisher struct{ trackingPublisher }

func (p *errorFGARemovePublisher) Access(ctx context.Context, subject string, msg any, sync bool) error {
	_ = p.trackingPublisher.Access(ctx, subject, msg, sync)
	if strings.Contains(subject, fgaconstants.GenericMemberRemoveSubject) {
		return pkgerrors.NewUnexpected("nats unavailable", nil)
	}
	return nil
}

// seededStorage is a port.MemberReader that returns a fixed key contact by UID.
type seededStorage struct {
	mock.MockMembershipRepository
	kcs map[string]*model.KeyContact
}

func newSeededStorage(kcs ...*model.KeyContact) *seededStorage {
	s := &seededStorage{kcs: make(map[string]*model.KeyContact)}
	for _, kc := range kcs {
		s.kcs[kc.UID] = kc
	}
	return s
}

func (s *seededStorage) GetKeyContact(_ context.Context, uid string) (*model.KeyContact, error) {
	if kc, ok := s.kcs[uid]; ok {
		return kc, nil
	}
	return nil, pkgerrors.NewNotFound("key contact not found")
}

func (s *seededStorage) ListKeyContactsForMembership(_ context.Context, _ string) ([]*model.KeyContact, error) {
	var out []*model.KeyContact
	for _, kc := range s.kcs {
		out = append(out, kc)
	}
	return out, nil
}

// seededPMReader returns a fixed PM for any UID.
type seededPMReader struct{ pm *model.ProjectMembership }

func (r *seededPMReader) AssembleProjectMembership(_ context.Context, _ string) (*model.ProjectMembership, time.Time, error) {
	return r.pm, time.Time{}, nil
}

// userReaderFunc implements port.UserReader with a function.
type userReaderFunc func(ctx context.Context, email string) (string, error)

func (f userReaderFunc) UsernameByEmail(ctx context.Context, email string) (string, error) {
	return f(ctx, email)
}

func newKCWriter(storage svc.MemberStorageReader, pmReader svc.PMReader, pub svc.PublisherForKC, userReader svc.UserReaderForKC) svc.KeyContactWriter {
	return svc.NewKeyContactWriter(
		svc.WithKCStorage(storage),
		svc.WithKCWriter(mock.NewMockKeyContactWriterWithOK()),
		svc.WithKCProjectMembershipReader(pmReader),
		svc.WithKCPublisher(pub),
		svc.WithKCUserReader(userReader),
	)
}

// ── Create tests ──────────────────────────────────────────────────────────

func TestKeyContactWriter_Create_NormalPath_PublishesInOrder(t *testing.T) {
	pm := &model.ProjectMembership{UID: testMembershipUID, B2BOrgUID: "org-1", ProjectUID: "proj-1"}
	pmReader := &seededPMReader{pm: pm}
	pub := &trackingPublisher{}
	storage := newSeededStorage() // empty — no self-heal

	w := newKCWriter(storage, pmReader, pub, userReaderFunc(func(_ context.Context, _ string) (string, error) {
		return "alice", nil
	}))

	in := svc.KeyContactCreateInput{
		MembershipUID: testMembershipUID,
		FirstName:     "Alice",
		LastName:      "Smith",
		Email:         "alice@example.com",
		Role:          "Technical Advisory Committee (TAC) Representative",
	}
	kc, err := w.Create(context.Background(), in)

	require.NoError(t, err)
	require.NotNil(t, kc)

	// Ordering invariant: PM FGA update_access → key_contact indexer → key_contact FGA put
	calls := pub.calls()
	require.True(t, len(calls) >= 3, "expected at least 3 publish calls, got %d: %v", len(calls), calls)
	// PM FGA is first
	assert.Contains(t, calls[0], "update_access", "first call must be PM FGA update_access")
	// indexer before FGA put
	indexerIdx := -1
	putIdx := -1
	for i, c := range calls {
		if strings.Contains(c, "indexer:") {
			indexerIdx = i
		}
		if strings.Contains(c, fgaconstants.GenericMemberPutSubject) {
			putIdx = i
		}
	}
	assert.Greater(t, putIdx, indexerIdx, "FGA put must come after indexer publish")
}

func TestKeyContactWriter_Create_SelfHeal_ReturnsExistingWithoutWrite(t *testing.T) {
	existing := &model.KeyContact{
		UID:           testKCUID,
		MembershipUID: testMembershipUID,
		Email:         "alice@example.com",
		Role:          "Technical Advisory Committee (TAC) Representative",
		Status:        "Active",
		UpdatedAt:     time.Now(),
	}
	storage := newSeededStorage(existing)
	pub := &trackingPublisher{}

	w := newKCWriter(storage, &seededPMReader{pm: &model.ProjectMembership{UID: testMembershipUID}}, pub, userReaderFunc(func(_ context.Context, _ string) (string, error) {
		return "", nil
	}))

	in := svc.KeyContactCreateInput{
		MembershipUID: testMembershipUID,
		FirstName:     "Alice",
		LastName:      "Smith",
		Email:         "alice@example.com", // same email + role → self-heal
		Role:          "Technical Advisory Committee (TAC) Representative",
	}
	kc, err := w.Create(context.Background(), in)

	require.NoError(t, err)
	assert.Equal(t, testKCUID, kc.UID, "self-heal must return existing record")
	assert.Empty(t, pub.calls(), "self-heal must not publish anything")
}

// ── Update tests ──────────────────────────────────────────────────────────

func TestKeyContactWriter_Update_NoOpETag_SkipsPublish(t *testing.T) {
	kc := &model.KeyContact{
		UID: testKCUID, MembershipUID: testMembershipUID,
		Email: "alice@example.com", Role: "role-a", Status: "Active",
		UpdatedAt: time.Now(),
	}
	storage := newSeededStorage(kc)
	pub := &trackingPublisher{}

	w := newKCWriter(storage, &seededPMReader{pm: &model.ProjectMembership{}}, pub, userReaderFunc(func(_ context.Context, _ string) (string, error) {
		return "alice", nil
	}))

	// UpdateKeyContact with same data → writer returns identical kc → ETag unchanged → skip publish
	in := svc.KeyContactUpdateInput{
		MembershipUID: testMembershipUID,
		UID:           testKCUID,
	}
	_, err := w.Update(context.Background(), in)

	require.NoError(t, err)
	assert.Empty(t, pub.calls(), "no-op update must not publish")
}

func TestKeyContactWriter_Update_EmailChange_PutBeforeRemove(t *testing.T) {
	oldKC := &model.KeyContact{
		UID: testKCUID, MembershipUID: testMembershipUID,
		Email: "old@example.com", Username: "old-sub",
		Role: "role-a", Status: "Active", UpdatedAt: time.Now(),
	}
	storage := newSeededStorage(oldKC)
	pub := &trackingPublisher{}
	newEmail := "new@example.com"

	w := newKCWriter(storage, &seededPMReader{pm: &model.ProjectMembership{}}, pub, userReaderFunc(func(_ context.Context, email string) (string, error) {
		if strings.EqualFold(email, "new@example.com") {
			return "new-sub", nil
		}
		return "old-sub", nil
	}))

	in := svc.KeyContactUpdateInput{
		MembershipUID: testMembershipUID,
		UID:           testKCUID,
		Email:         &newEmail,
	}
	_, err := w.Update(context.Background(), in)

	require.NoError(t, err)

	// Ordering invariant: FGA put (new sub) BEFORE FGA remove (old sub)
	calls := pub.calls()
	putIdx := -1
	removeIdx := -1
	for i, c := range calls {
		if strings.Contains(c, fgaconstants.GenericMemberPutSubject) {
			putIdx = i
		}
		if strings.Contains(c, fgaconstants.GenericMemberRemoveSubject) {
			removeIdx = i
		}
	}
	require.NotEqual(t, -1, putIdx, "FGA put must be called")
	require.NotEqual(t, -1, removeIdx, "FGA remove must be called on email change")
	assert.Less(t, putIdx, removeIdx, "FGA put must precede FGA remove")
}

func TestKeyContactWriter_Update_EmailChange_RemoveError_NotPropagated(t *testing.T) {
	oldKC := &model.KeyContact{
		UID: testKCUID, MembershipUID: testMembershipUID,
		Email: "old@example.com", Username: "old-sub",
		Role: "role-a", Status: "Active", UpdatedAt: time.Now(),
	}
	storage := newSeededStorage(oldKC)
	pub := &errorFGARemovePublisher{}
	newEmail := "new@example.com"

	w := newKCWriter(storage, &seededPMReader{pm: &model.ProjectMembership{}}, pub, userReaderFunc(func(_ context.Context, email string) (string, error) {
		if strings.EqualFold(email, "new@example.com") {
			return "new-sub", nil
		}
		return "old-sub", nil
	}))

	in := svc.KeyContactUpdateInput{UID: testKCUID, MembershipUID: testMembershipUID, Email: &newEmail}
	_, err := w.Update(context.Background(), in)

	assert.NoError(t, err, "FGA remove error on email change must NOT be propagated")
}

func TestKeyContactWriter_Update_IfMatch_Mismatch_PreconditionFailed(t *testing.T) {
	kc := &model.KeyContact{UID: testKCUID, MembershipUID: testMembershipUID, UpdatedAt: time.Now()}
	storage := newSeededStorage(kc)

	w := newKCWriter(storage, &seededPMReader{pm: &model.ProjectMembership{}}, &trackingPublisher{}, userReaderFunc(func(_ context.Context, _ string) (string, error) {
		return "", nil
	}))

	in := svc.KeyContactUpdateInput{UID: testKCUID, MembershipUID: testMembershipUID, IfMatch: "\"stale\""}
	_, err := w.Update(context.Background(), in)

	require.Error(t, err)
	assert.True(t, pkgerrors.IsPreconditionFailed(err))
}

// ── Delete tests ──────────────────────────────────────────────────────────

func TestKeyContactWriter_Delete_OrderingInvariant_DeleteThenIndexerThenFGARemove(t *testing.T) {
	kc := &model.KeyContact{
		UID: testKCUID, MembershipUID: testMembershipUID,
		Email: "alice@example.com", Username: "alice",
	}
	storage := newSeededStorage(kc)
	pub := &trackingPublisher{}

	w := newKCWriter(storage, &seededPMReader{pm: &model.ProjectMembership{}}, pub, userReaderFunc(func(_ context.Context, _ string) (string, error) {
		return "alice", nil
	}))

	in := svc.KeyContactDeleteInput{MembershipUID: testMembershipUID, UID: testKCUID}
	err := w.Delete(context.Background(), in)

	require.NoError(t, err)
	calls := pub.calls()
	indexerIdx := -1
	removeIdx := -1
	for i, c := range calls {
		if strings.Contains(c, "indexer:") {
			indexerIdx = i
		}
		if strings.Contains(c, fgaconstants.GenericMemberRemoveSubject) {
			removeIdx = i
		}
	}
	require.NotEqual(t, -1, indexerIdx, "indexer must be called on delete")
	require.NotEqual(t, -1, removeIdx, "FGA remove must be called on delete")
	assert.Less(t, indexerIdx, removeIdx, "indexer must precede FGA remove")
}

func TestKeyContactWriter_Delete_FGARemoveError_Propagated(t *testing.T) {
	kc := &model.KeyContact{
		UID: testKCUID, MembershipUID: testMembershipUID,
		Email: "alice@example.com", Username: "alice",
	}
	storage := newSeededStorage(kc)
	pub := &errorFGARemovePublisher{}

	w := newKCWriter(storage, &seededPMReader{pm: &model.ProjectMembership{}}, pub, userReaderFunc(func(_ context.Context, _ string) (string, error) {
		return "alice", nil
	}))

	in := svc.KeyContactDeleteInput{MembershipUID: testMembershipUID, UID: testKCUID}
	err := w.Delete(context.Background(), in)

	require.Error(t, err, "FGA remove failure on delete must be propagated")
}

func TestKeyContactWriter_Delete_IfMatch_Mismatch_PreconditionFailed(t *testing.T) {
	kc := &model.KeyContact{UID: testKCUID, MembershipUID: testMembershipUID, UpdatedAt: time.Now()}
	storage := newSeededStorage(kc)

	w := newKCWriter(storage, &seededPMReader{pm: &model.ProjectMembership{}}, &trackingPublisher{}, userReaderFunc(func(_ context.Context, _ string) (string, error) {
		return "", nil
	}))

	in := svc.KeyContactDeleteInput{MembershipUID: testMembershipUID, UID: testKCUID, IfMatch: "\"stale\""}
	err := w.Delete(context.Background(), in)

	require.Error(t, err)
	assert.True(t, pkgerrors.IsPreconditionFailed(err))
}
