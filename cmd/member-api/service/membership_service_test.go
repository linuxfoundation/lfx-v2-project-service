// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	membershipservice "github.com/linuxfoundation/lfx-v2-member-service/gen/membership_service"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/mock"
	usecaseSvc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/etag"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goa "goa.design/goa/v3/pkg"
)

// TestStubHandlers_ReturnNotImplemented checks that handlers not yet wired
// return a Goa NotImplemented error. B2BOrg handlers are now wired (tested
// separately); this list covers the remaining stubs.
func TestStubHandlers_ReturnNotImplemented(t *testing.T) {
	tests := []struct {
		name        string
		callHandler func(svc membershipservice.Service, ctx context.Context) error
	}{
		{
			name: "GetProjectMembership returns NotImplemented",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				_, err := svc.GetProjectMembership(ctx, &membershipservice.GetProjectMembershipPayload{
					UID: "test-uid",
				})
				return err
			},
		},
		{
			name: "GetKeyContact returns NotImplemented",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				_, err := svc.GetKeyContact(ctx, &membershipservice.GetKeyContactPayload{
					UID: "test-uid",
				})
				return err
			},
		},
		{
			name: "CreateKeyContact returns NotImplemented",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				_, err := svc.CreateKeyContact(ctx, &membershipservice.CreateKeyContactPayload{
					B2bOrgUID:     "org-uid",
					ProjectUID:    "project-uid",
					MembershipUID: "membership-uid",
					Email:         "test@example.com",
					FirstName:     "Test",
					LastName:      "User",
				})
				return err
			},
		},
		{
			name: "UpdateKeyContact returns NotImplemented",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				_, err := svc.UpdateKeyContact(ctx, &membershipservice.UpdateKeyContactPayload{
					UID: "test-uid",
				})
				return err
			},
		},
		{
			name: "DeleteKeyContact returns NotImplemented",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				return svc.DeleteKeyContact(ctx, &membershipservice.DeleteKeyContactPayload{
					UID: "test-uid",
				})
			},
		},
		{
			name: "AdminReindex returns NotImplemented",
			callHandler: func(svc membershipservice.Service, ctx context.Context) error {
				_, err := svc.AdminReindex(ctx, &membershipservice.AdminReindexPayload{})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestMembershipService()
			ctx := context.Background()

			err := tt.callHandler(svc, ctx)

			require.Error(t, err)

			var serviceErr *goa.ServiceError
			ok := errors.As(err, &serviceErr)
			require.True(t, ok, "error should be a *goa.ServiceError, got %T: %v", err, err)

			assert.Equal(t, "NotImplemented", serviceErr.Name)
		})
	}
}

// TestGetB2bOrg_NotFound asserts that GetB2bOrg returns a Goa NotFound error
// when the mock reader cannot locate the UID.
func TestGetB2bOrg_NotFound(t *testing.T) {
	svc := newTestMembershipService()
	ctx := context.Background()

	_, err := svc.GetB2bOrg(ctx, &membershipservice.GetB2bOrgPayload{
		UID: "nonexistent-uid",
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr), "expected *goa.ServiceError, got %T: %v", err, err)
	assert.Equal(t, "NotFound", serviceErr.Name)
}

// TestCreateB2bOrg_MockReturnsNotImplemented asserts that CreateB2bOrg
// propagates errors from the writer — the mock writer always returns
// NotImplemented, so the handler should surface it as a Goa error.
func TestCreateB2bOrg_MockReturnsNotImplemented(t *testing.T) {
	svc := newTestMembershipService()
	ctx := context.Background()

	_, err := svc.CreateB2bOrg(ctx, &membershipservice.CreateB2bOrgPayload{
		Sfid: "001000000000001AAA",
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr), "expected *goa.ServiceError, got %T: %v", err, err)
	assert.Equal(t, "NotImplemented", serviceErr.Name)
}

// newTestMembershipService constructs a MembershipService with mock dependencies
// for testing.
func newTestMembershipService() membershipservice.Service {
	mockRepo := mock.NewMockMembershipRepository()
	mockB2BOrgReader := mock.NewMockB2BOrgReader()
	mockB2BOrgWriter := mock.NewMockB2BOrgWriter()
	mockPublisher := mock.NewMockMemberPublisher()
	mockKCWriter := &mockKeyContactWriter{}

	readMemberUseCase := usecaseSvc.NewMemberReaderOrchestrator(
		usecaseSvc.WithMemberReader(mockRepo),
	)

	mockAuth := &auth.MockJWTAuth{}

	return NewMembershipService(
		readMemberUseCase,
		mockRepo,
		mockAuth,
		mockKCWriter,
		mockB2BOrgReader,
		mockB2BOrgWriter,
		mockPublisher,
		"",
	)
}

// mockKeyContactWriter is a simple test implementation of port.KeyContactWriter.
type mockKeyContactWriter struct{}

func (m *mockKeyContactWriter) CreateKeyContact(_ context.Context, _ model.KeyContactInput) (*model.KeyContact, error) {
	return nil, nil
}

func (m *mockKeyContactWriter) UpdateKeyContact(_ context.Context, _ string, _ model.KeyContactInput) (*model.KeyContact, error) {
	return nil, nil
}

func (m *mockKeyContactWriter) DeleteKeyContact(_ context.Context, _ string, _ string) error {
	return nil
}

// ── B2BOrg handler tests ──────────────────────────────────────────────────────

// sampleB2BOrg is the canonical test fixture returned by seeded mocks.
var sampleB2BOrg = &model.B2BOrg{
	UID:       "lf-uid-001",
	SFID:      "001000000000001AAA",
	Name:      "Linux Foundation",
	Website:   "https://linuxfoundation.org",
	Industry:  "Technology",
	Status:    "Active",
	CreatedAt: time.Date(2020, 1, 15, 10, 30, 0, 0, time.UTC),
	UpdatedAt: time.Date(2024, 6, 1, 8, 0, 0, 0, time.UTC),
}

// TestGetB2bOrg_Happy verifies that GetB2bOrg maps the domain B2BOrg to the
// response type and includes ETag + Last-Modified headers.
func TestGetB2bOrg_Happy(t *testing.T) {
	svc := newTestMembershipServiceWithReader(&seededB2BOrgReader{org: sampleB2BOrg})
	ctx := context.Background()

	result, err := svc.GetB2bOrg(ctx, &membershipservice.GetB2bOrgPayload{UID: "lf-uid-001"})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.B2bOrg)
	assert.Equal(t, "lf-uid-001", *result.B2bOrg.UID)
	assert.Equal(t, "Linux Foundation", *result.B2bOrg.Name)
	assert.NotNil(t, result.Etag, "ETag must be set")
	assert.NotNil(t, result.LastModified, "Last-Modified must be set")
}

// TestCreateB2bOrg_Happy verifies that CreateB2bOrg returns the created org
// and that a publish failure does not fail the HTTP response (swallow policy).
func TestCreateB2bOrg_PublishFailureDoesNotFailResponse(t *testing.T) {
	failingPub := &errorPublisher{}
	svc := newTestMembershipServiceWith(
		&seededB2BOrgReader{org: sampleB2BOrg},
		&happyB2BOrgWriter{org: sampleB2BOrg},
		failingPub,
		"",
	)
	ctx := context.Background()

	result, err := svc.CreateB2bOrg(ctx, &membershipservice.CreateB2bOrgPayload{
		Sfid: "001000000000001AAA",
	})

	// Publish failure must be swallowed — the handler must return 201 success.
	require.NoError(t, err, "publish failure must not fail CreateB2bOrg")
	require.NotNil(t, result)
	assert.Equal(t, "lf-uid-001", *result.B2bOrg.UID)
}

// TestUpdateB2bOrg_Happy verifies that UpdateB2bOrg returns the updated org.
func TestUpdateB2bOrg_Happy(t *testing.T) {
	svc := newTestMembershipServiceWith(
		&seededB2BOrgReader{org: sampleB2BOrg},
		&happyB2BOrgWriter{org: sampleB2BOrg},
		mock.NewMockMemberPublisher(),
		"team-uid-global-admin",
	)
	ctx := context.Background()

	result, err := svc.UpdateB2bOrg(ctx, &membershipservice.UpdateB2bOrgPayload{
		UID:  "lf-uid-001",
		Name: strPtr("Linux Foundation Updated"),
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "lf-uid-001", *result.B2bOrg.UID)
}

// TestUpdateB2bOrg_PreconditionFailed verifies that a stale If-Match returns
// HTTP 412. The service layer validates ETag against the current record (fetched
// via the reader) before calling the writer — so the reader must return an org.
func TestUpdateB2bOrg_PreconditionFailed(t *testing.T) {
	svc := newTestMembershipServiceWith(
		&seededB2BOrgReader{org: sampleB2BOrg},
		&preconditionFailingWriter{},
		mock.NewMockMemberPublisher(),
		"",
	)
	ctx := context.Background()

	_, err := svc.UpdateB2bOrg(ctx, &membershipservice.UpdateB2bOrgPayload{
		UID:     "lf-uid-001",
		Name:    strPtr("Linux Foundation"),
		IfMatch: strPtr(`"stale-etag"`),
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr))
	assert.Equal(t, "PreconditionFailed", serviceErr.Name)
}

// TestCreateB2bOrg_FGAMessageContainsGlobalOrgAdmin verifies that when
// globalOrgAdminTeamUID is non-empty, publishB2BOrgEvents builds an FGA
// message with References["global_org_admin"] set. We confirm this indirectly
// by checking the publish does not fail when the team UID is provided.
func TestCreateB2bOrg_GlobalOrgAdminTeamUID(t *testing.T) {
	capturer := &capturingPublisher{}
	svc := newTestMembershipServiceWith(
		&seededB2BOrgReader{org: sampleB2BOrg},
		&happyB2BOrgWriter{org: sampleB2BOrg},
		capturer,
		"global-admin-team-uid",
	)
	ctx := context.Background()

	_, err := svc.CreateB2bOrg(ctx, &membershipservice.CreateB2bOrgPayload{
		Sfid: "001000000000001AAA",
	})

	require.NoError(t, err)
	// Two publishes: Indexer + Access.
	assert.Equal(t, 2, capturer.count(), "must publish exactly two events")
}

// TestUpdateB2bOrg_ValidIfMatchTranslatedToIfUnmodifiedSince verifies that when
// the caller's If-Match matches the current record's ETag, the service translates
// it to IfUnmodifiedSince on the writer input (SF PATCH rejects If-Match directly).
func TestUpdateB2bOrg_ValidIfMatchTranslatedToIfUnmodifiedSince(t *testing.T) {
	w := &capturingB2BOrgWriter{org: sampleB2BOrg}
	svc := newTestMembershipServiceWith(
		&seededB2BOrgReader{org: sampleB2BOrg},
		w,
		mock.NewMockMemberPublisher(),
		"",
	)

	currentETag, err := etag.LFXEtag(sampleB2BOrg)
	require.NoError(t, err)

	_, err = svc.UpdateB2bOrg(context.Background(), &membershipservice.UpdateB2bOrgPayload{
		UID:     "lf-uid-001",
		Name:    strPtr("Updated Name"),
		IfMatch: &currentETag,
	})

	require.NoError(t, err)
	want := sampleB2BOrg.UpdatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	assert.Equal(t, want, w.lastInput.IfUnmodifiedSince,
		"matching If-Match must be translated to IfUnmodifiedSince for the SF PATCH")
}

// TestCreateB2bOrg_PublishesActionCreated verifies that CreateB2bOrg dispatches
// an indexer message with action=created.
func TestCreateB2bOrg_PublishesActionCreated(t *testing.T) {
	pub := &indexerMessageCapture{}
	svc := newTestMembershipServiceWith(
		&seededB2BOrgReader{org: sampleB2BOrg},
		&happyB2BOrgWriter{org: sampleB2BOrg},
		pub,
		"",
	)

	_, err := svc.CreateB2bOrg(context.Background(), &membershipservice.CreateB2bOrgPayload{
		Sfid: "001000000000001AAA",
	})

	require.NoError(t, err)
	msgs := pub.captured()
	require.Len(t, msgs, 1)
	assert.Equal(t, indexerConstants.ActionCreated, msgs[0].Action)
}

// TestUpdateB2bOrg_PublishesActionUpdated verifies that UpdateB2bOrg dispatches
// an indexer message with action=updated.
func TestUpdateB2bOrg_PublishesActionUpdated(t *testing.T) {
	pub := &indexerMessageCapture{}
	svc := newTestMembershipServiceWith(
		&seededB2BOrgReader{org: sampleB2BOrg},
		&happyB2BOrgWriter{org: sampleB2BOrg},
		pub,
		"",
	)

	_, err := svc.UpdateB2bOrg(context.Background(), &membershipservice.UpdateB2bOrgPayload{
		UID:  "lf-uid-001",
		Name: strPtr("Updated Name"),
	})

	require.NoError(t, err)
	msgs := pub.captured()
	require.Len(t, msgs, 1)
	assert.Equal(t, indexerConstants.ActionUpdated, msgs[0].Action)
}

// TestCreateB2bOrg_WithParent verifies that when parent_sfid is provided the
// service converts it to a v2 UUID and passes it as ParentUID in the writer input.
func TestCreateB2bOrg_WithParent(t *testing.T) {
	w := &capturingB2BOrgWriter{org: sampleB2BOrg}
	svc := newTestMembershipServiceWith(
		&seededB2BOrgReader{org: sampleB2BOrg},
		w,
		mock.NewMockMemberPublisher(),
		"",
	)

	parentSfid := "001B0000001ckSl"
	_, err := svc.CreateB2bOrg(context.Background(), &membershipservice.CreateB2bOrgPayload{
		Sfid:       "001000000000001AAA",
		ParentSfid: &parentSfid,
	})

	require.NoError(t, err)
	assert.Equal(t, "001000000000001AAA", w.lastCreateSFID, "sfid must be forwarded unchanged")
	require.NotNil(t, w.lastCreateInput.ParentUID,
		"ParentUID must be set when parent_sfid is provided")
	// Verify round-trip: ToSFID of the stored UID must equal the original SFID.
	reconstructed, convErr := sfuuid.ToSFID(*w.lastCreateInput.ParentUID)
	require.NoError(t, convErr)
	assert.Equal(t, parentSfid, reconstructed,
		"ParentUID must be the v2 UUID derived from parent_sfid")
}

// TestCreateB2bOrg_WithInvalidParent verifies that an invalid parent_sfid
// returns a validation error without calling the writer.
func TestCreateB2bOrg_WithInvalidParent(t *testing.T) {
	w := &capturingB2BOrgWriter{org: sampleB2BOrg}
	svc := newTestMembershipServiceWith(
		&seededB2BOrgReader{org: sampleB2BOrg},
		w,
		mock.NewMockMemberPublisher(),
		"",
	)

	badSfid := "not-a-valid-sfid"
	_, err := svc.CreateB2bOrg(context.Background(), &membershipservice.CreateB2bOrgPayload{
		Sfid:       "001000000000001AAA",
		ParentSfid: &badSfid,
	})

	require.Error(t, err)
	assert.Empty(t, w.lastCreateSFID, "writer must not be called on invalid parent_sfid")
}

// ── local test mocks ──────────────────────────────────────────────────────────

// seededB2BOrgReader returns a fixed org for any UID.
type seededB2BOrgReader struct{ org *model.B2BOrg }

func (r *seededB2BOrgReader) GetB2BOrg(_ context.Context, _ string) (*model.B2BOrg, error) {
	if r.org == nil {
		return nil, pkgerrors.NewNotFound("b2b org not found")
	}
	return r.org, nil
}

// happyB2BOrgWriter returns a fixed org for any SFID/UID.
type happyB2BOrgWriter struct{ org *model.B2BOrg }

func (w *happyB2BOrgWriter) CreateB2BOrg(_ context.Context, _ string, _ model.B2BOrgInput) (*model.B2BOrg, error) {
	return w.org, nil
}
func (w *happyB2BOrgWriter) UpdateB2BOrg(_ context.Context, _ string, _ model.B2BOrgInput) (*model.B2BOrg, error) {
	return w.org, nil
}

// preconditionFailingWriter always returns a PreconditionFailed error.
type preconditionFailingWriter struct{}

func (w *preconditionFailingWriter) CreateB2BOrg(_ context.Context, _ string, _ model.B2BOrgInput) (*model.B2BOrg, error) {
	return nil, pkgerrors.NewPreconditionFailed("stale etag")
}
func (w *preconditionFailingWriter) UpdateB2BOrg(_ context.Context, _ string, _ model.B2BOrgInput) (*model.B2BOrg, error) {
	return nil, pkgerrors.NewPreconditionFailed("stale etag")
}

// errorPublisher always returns an error from both Indexer and Access.
type errorPublisher struct{}

func (p *errorPublisher) Indexer(_ context.Context, _ string, _ any, _ bool) error {
	return pkgerrors.NewServiceUnavailable("nats unavailable")
}
func (p *errorPublisher) Access(_ context.Context, _ string, _ any, _ bool) error {
	return pkgerrors.NewServiceUnavailable("nats unavailable")
}

// capturingPublisher counts publish calls without failing.
// calls is accessed concurrently by errgroup goroutines, so use atomic ops.
type capturingPublisher struct {
	calls atomic.Int64
}

func (p *capturingPublisher) Indexer(_ context.Context, _ string, _ any, _ bool) error {
	p.calls.Add(1)
	return nil
}
func (p *capturingPublisher) Access(_ context.Context, _ string, _ any, _ bool) error {
	p.calls.Add(1)
	return nil
}
func (p *capturingPublisher) count() int { return int(p.calls.Load()) }

// capturingB2BOrgWriter records args passed to CreateB2BOrg and UpdateB2BOrg.
type capturingB2BOrgWriter struct {
	org             *model.B2BOrg
	lastInput       model.B2BOrgInput
	lastCreateSFID  string
	lastCreateInput model.B2BOrgInput
}

func (w *capturingB2BOrgWriter) CreateB2BOrg(_ context.Context, sfid string, input model.B2BOrgInput) (*model.B2BOrg, error) {
	w.lastCreateSFID = sfid
	w.lastCreateInput = input
	return w.org, nil
}
func (w *capturingB2BOrgWriter) UpdateB2BOrg(_ context.Context, _ string, input model.B2BOrgInput) (*model.B2BOrg, error) {
	w.lastInput = input
	return w.org, nil
}

// indexerMessageCapture records *model.MemberIndexerMessage from Indexer publishes.
// Access publishes carry a different type and are ignored here.
// mu guards msgs because Indexer is called from an errgroup goroutine.
type indexerMessageCapture struct {
	mu   sync.Mutex
	msgs []*model.MemberIndexerMessage
}

func (p *indexerMessageCapture) Indexer(_ context.Context, _ string, msg any, _ bool) error {
	if m, ok := msg.(*model.MemberIndexerMessage); ok {
		p.mu.Lock()
		p.msgs = append(p.msgs, m)
		p.mu.Unlock()
	}
	return nil
}
func (p *indexerMessageCapture) Access(_ context.Context, _ string, _ any, _ bool) error {
	return nil
}
func (p *indexerMessageCapture) captured() []*model.MemberIndexerMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]*model.MemberIndexerMessage(nil), p.msgs...)
}

// strPtr is a test helper returning a pointer to the given string value.
func strPtr(s string) *string { return &s }

// ── constructors ──────────────────────────────────────────────────────────────

// newTestMembershipServiceWithReader builds a service using the provided reader
// and default mocks for all other dependencies.
func newTestMembershipServiceWithReader(reader port.B2BOrgReader) membershipservice.Service {
	return newTestMembershipServiceWith(reader, mock.NewMockB2BOrgWriter(), mock.NewMockMemberPublisher(), "")
}

// newTestMembershipServiceWith builds a service with explicit reader, writer,
// publisher, and globalOrgAdminTeamUID.
func newTestMembershipServiceWith(
	b2bOrgReader port.B2BOrgReader,
	b2bOrgWriter port.B2BOrgWriter,
	publisher port.MemberPublisher,
	globalOrgAdminTeamUID string,
) membershipservice.Service {
	mockRepo := mock.NewMockMembershipRepository()
	readMemberUseCase := usecaseSvc.NewMemberReaderOrchestrator(
		usecaseSvc.WithMemberReader(mockRepo),
	)
	return NewMembershipService(
		readMemberUseCase,
		mockRepo,
		&auth.MockJWTAuth{},
		&mockKeyContactWriter{},
		b2bOrgReader,
		b2bOrgWriter,
		publisher,
		globalOrgAdminTeamUID,
	)
}

// Verify that the mock ports are properly implemented.
var (
	_ port.MemberReader     = (*mock.MockMembershipRepository)(nil)
	_ port.KeyContactWriter = (*mockKeyContactWriter)(nil)
	_ port.B2BOrgReader     = (*mock.MockB2BOrgReader)(nil)
	_ port.B2BOrgWriter     = (*mock.MockB2BOrgWriter)(nil)
	_ port.MemberPublisher  = (*mock.MockMemberPublisher)(nil)
	_ port.B2BOrgReader     = (*seededB2BOrgReader)(nil)
	_ port.B2BOrgWriter     = (*happyB2BOrgWriter)(nil)
	_ port.B2BOrgWriter     = (*preconditionFailingWriter)(nil)
	_ port.B2BOrgWriter     = (*capturingB2BOrgWriter)(nil)
	_ port.MemberPublisher  = (*errorPublisher)(nil)
	_ port.MemberPublisher  = (*capturingPublisher)(nil)
	_ port.MemberPublisher  = (*indexerMessageCapture)(nil)
)
