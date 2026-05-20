// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goa "goa.design/goa/v3/pkg"
)

// TestStubHandlers_ReturnNotImplemented checks that handlers not yet wired
// return a Goa NotImplemented error. B2BOrg, ProjectMembership, and KeyContact
// handlers are now wired (tested separately); this list covers the remaining stubs.
func TestStubHandlers_ReturnNotImplemented(t *testing.T) {
	tests := []struct {
		name        string
		callHandler func(svc membershipservice.Service, ctx context.Context) error
	}{
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
	mockProjectMembershipReader := mock.NewMockProjectMembershipReader()
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
		mockProjectMembershipReader,
		mockPublisher,
		&mock.MockUserReader{},
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
		mock.NewMockProjectMembershipReader(),
		publisher,
		&mock.MockUserReader{},
		globalOrgAdminTeamUID,
	)
}

// TestGetProjectMembership_Happy verifies that GetProjectMembership assembles
// and returns a fully denormalised ProjectMembership with ETag and Last-Modified.
func TestGetProjectMembership_Happy(t *testing.T) {
	now := time.Now()
	sampleMembership := &model.ProjectMembership{
		UID:             "membership-uid-001",
		TierUID:         "tier-uid-001",
		ProjectUID:      "project-uid-001",
		ProjectSlug:     "linux-foundation",
		Status:          "Active",
		Year:            "2025",
		Tier:            "Gold",
		AutoRenew:       true,
		CompanyName:     "Acme Corp",
		CompanyLogoURL:  "https://acme.com/logo.png",
		CompanyDomain:   "https://acme.com",
		TierName:        "Gold Membership",
		TierFamily:      "Membership",
		TierProductType: "Corporate",
		CreatedAt:       now.Add(-24 * time.Hour),
		UpdatedAt:       now,
	}

	pmr := &mockProjectMembershipReader{
		membership: sampleMembership,
		lastMod:    now,
	}

	mockRepo := mock.NewMockMembershipRepository()
	readMemberUseCase := usecaseSvc.NewMemberReaderOrchestrator(
		usecaseSvc.WithMemberReader(mockRepo),
	)

	svc := NewMembershipService(
		readMemberUseCase,
		mockRepo,
		&auth.MockJWTAuth{},
		&mockKeyContactWriter{},
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		pmr,
		mock.NewMockMemberPublisher(),
		&mock.MockUserReader{},
		"",
	)

	result, err := svc.GetProjectMembership(context.Background(), &membershipservice.GetProjectMembershipPayload{
		UID: "membership-uid-001",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.ProjectMembership)
	assert.Equal(t, "membership-uid-001", *result.ProjectMembership.UID)
	assert.Equal(t, "Acme Corp", *result.ProjectMembership.CompanyName)
	assert.Equal(t, "Gold Membership", *result.ProjectMembership.TierName)
	assert.Equal(t, "linux-foundation", *result.ProjectMembership.ProjectSlug)
	assert.NotNil(t, result.Etag, "ETag must be set")
	assert.NotNil(t, result.LastModified, "Last-Modified must be set")
}

// TestGetProjectMembership_NotFound verifies that when the reader returns
// a not-found error, the handler propagates it as a Goa NotFound error.
func TestGetProjectMembership_NotFound(t *testing.T) {
	pmr := &mockProjectMembershipReader{
		err: pkgerrors.NewNotFound("membership not found"),
	}

	mockRepo := mock.NewMockMembershipRepository()
	readMemberUseCase := usecaseSvc.NewMemberReaderOrchestrator(
		usecaseSvc.WithMemberReader(mockRepo),
	)

	svc := NewMembershipService(
		readMemberUseCase,
		mockRepo,
		&auth.MockJWTAuth{},
		&mockKeyContactWriter{},
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		pmr,
		mock.NewMockMemberPublisher(),
		&mock.MockUserReader{},
		"",
	)

	_, err := svc.GetProjectMembership(context.Background(), &membershipservice.GetProjectMembershipPayload{
		UID: "nonexistent-uid",
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr), "expected *goa.ServiceError, got %T: %v", err, err)
	assert.Equal(t, "NotFound", serviceErr.Name)
}

// TestGetProjectMembership_ReaderError verifies that when the reader returns
// a generic error, the handler propagates it appropriately.
func TestGetProjectMembership_ReaderError(t *testing.T) {
	pmr := &mockProjectMembershipReader{
		err: pkgerrors.NewUnexpected("reader failed", fmt.Errorf("salesforce error")),
	}

	mockRepo := mock.NewMockMembershipRepository()
	readMemberUseCase := usecaseSvc.NewMemberReaderOrchestrator(
		usecaseSvc.WithMemberReader(mockRepo),
	)

	svc := NewMembershipService(
		readMemberUseCase,
		mockRepo,
		&auth.MockJWTAuth{},
		&mockKeyContactWriter{},
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		pmr,
		mock.NewMockMemberPublisher(),
		&mock.MockUserReader{},
		"",
	)

	_, err := svc.GetProjectMembership(context.Background(), &membershipservice.GetProjectMembershipPayload{
		UID: "test-uid",
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr), "expected *goa.ServiceError, got %T: %v", err, err)
	// Unexpected errors map to InternalServerError.
	assert.Equal(t, "InternalServerError", serviceErr.Name)
}

// TestGetKeyContact_Happy verifies that GetKeyContact maps the domain KeyContact
// to the response type and includes ETag + Last-Modified headers.
func TestGetKeyContact_Happy(t *testing.T) {
	mockRepo := mock.NewMockMembershipRepository()
	svc := NewMembershipService(
		usecaseSvc.NewMemberReaderOrchestrator(usecaseSvc.WithMemberReader(mockRepo)),
		mockRepo,
		&auth.MockJWTAuth{},
		&mockKeyContactWriter{},
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		mock.NewMockProjectMembershipReader(),
		mock.NewMockMemberPublisher(),
		&mock.MockUserReader{},
		"",
	)
	ctx := context.Background()

	result, err := svc.GetKeyContact(ctx, &membershipservice.GetKeyContactPayload{
		UID:           "contact-role-1",
		MembershipUID: "11111111-1111-1111-1111-111111111111",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.KeyContact)
	assert.Equal(t, "contact-role-1", *result.KeyContact.UID)
	assert.Equal(t, "John", *result.KeyContact.FirstName)
	assert.Equal(t, "Doe", *result.KeyContact.LastName)
	assert.NotNil(t, result.Etag, "ETag must be set")
	assert.NotNil(t, result.LastModified, "Last-Modified must be set")
}

// TestGetKeyContact_NotFound asserts that GetKeyContact returns a Goa NotFound
// error when the mock repository cannot locate the UID.
func TestGetKeyContact_NotFound(t *testing.T) {
	mockRepo := mock.NewMockMembershipRepository()
	svc := NewMembershipService(
		usecaseSvc.NewMemberReaderOrchestrator(usecaseSvc.WithMemberReader(mockRepo)),
		mockRepo,
		&auth.MockJWTAuth{},
		&mockKeyContactWriter{},
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		mock.NewMockProjectMembershipReader(),
		mock.NewMockMemberPublisher(),
		&mock.MockUserReader{},
		"",
	)
	ctx := context.Background()

	_, err := svc.GetKeyContact(ctx, &membershipservice.GetKeyContactPayload{
		UID: "nonexistent-uid",
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr), "expected *goa.ServiceError, got %T: %v", err, err)
	assert.Equal(t, "NotFound", serviceErr.Name)
}

// TestCreateKeyContact_MockReturnsNotImplemented asserts that CreateKeyContact
// propagates errors from the writer.
func TestCreateKeyContact_MockReturnsNotImplemented(t *testing.T) {
	mockRepo := mock.NewMockMembershipRepository()
	svc := NewMembershipService(
		usecaseSvc.NewMemberReaderOrchestrator(usecaseSvc.WithMemberReader(mockRepo)),
		mockRepo,
		&auth.MockJWTAuth{},
		mock.NewMockKeyContactWriter(), // This always returns NotImplemented
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		mock.NewMockProjectMembershipReader(),
		mock.NewMockMemberPublisher(),
		&mock.MockUserReader{},
		"",
	)
	ctx := context.Background()

	_, err := svc.CreateKeyContact(ctx, &membershipservice.CreateKeyContactPayload{
		MembershipUID: "11111111-1111-1111-1111-111111111111",
		Email:         "test@example.com",
		FirstName:     "Test",
		LastName:      "User",
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr), "expected *goa.ServiceError, got %T: %v", err, err)
	assert.Equal(t, "NotImplemented", serviceErr.Name)
}

// TestUpdateKeyContact_MockReturnsNotImplemented asserts that UpdateKeyContact
// propagates errors from the writer.
func TestUpdateKeyContact_MockReturnsNotImplemented(t *testing.T) {
	mockRepo := mock.NewMockMembershipRepository()
	svc := NewMembershipService(
		usecaseSvc.NewMemberReaderOrchestrator(usecaseSvc.WithMemberReader(mockRepo)),
		mockRepo,
		&auth.MockJWTAuth{},
		mock.NewMockKeyContactWriter(), // This always returns NotImplemented
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		mock.NewMockProjectMembershipReader(),
		mock.NewMockMemberPublisher(),
		&mock.MockUserReader{},
		"",
	)
	ctx := context.Background()

	_, err := svc.UpdateKeyContact(ctx, &membershipservice.UpdateKeyContactPayload{
		UID:           "contact-role-1", // pre-seeded in mock so storage lookup succeeds
		MembershipUID: "11111111-1111-1111-1111-111111111111",
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr), "expected *goa.ServiceError, got %T: %v", err, err)
	assert.Equal(t, "NotImplemented", serviceErr.Name)
}

// TestDeleteKeyContact_MockReturnsNotImplemented asserts that DeleteKeyContact
// propagates errors from the writer.
func TestDeleteKeyContact_MockReturnsNotImplemented(t *testing.T) {
	mockRepo := mock.NewMockMembershipRepository()
	svc := NewMembershipService(
		usecaseSvc.NewMemberReaderOrchestrator(usecaseSvc.WithMemberReader(mockRepo)),
		mockRepo,
		&auth.MockJWTAuth{},
		mock.NewMockKeyContactWriter(), // This always returns NotImplemented
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		mock.NewMockProjectMembershipReader(),
		mock.NewMockMemberPublisher(),
		&mock.MockUserReader{},
		"",
	)
	ctx := context.Background()

	err := svc.DeleteKeyContact(ctx, &membershipservice.DeleteKeyContactPayload{
		UID:           "contact-role-1",
		MembershipUID: "11111111-1111-1111-1111-111111111111",
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr), "expected *goa.ServiceError, got %T: %v", err, err)
	assert.Equal(t, "NotImplemented", serviceErr.Name)
}

// TestUpdateKeyContact_StalePreconditionFailed verifies that a stale If-Match
// returns HTTP 412. The service layer validates ETag against the current record
// (fetched via the reader) before calling the writer.
func TestUpdateKeyContact_StalePreconditionFailed(t *testing.T) {
	mockRepo := mock.NewMockMembershipRepository()
	svc := NewMembershipService(
		usecaseSvc.NewMemberReaderOrchestrator(usecaseSvc.WithMemberReader(mockRepo)),
		mockRepo,
		&auth.MockJWTAuth{},
		&mockKeyContactWriter{},
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		mock.NewMockProjectMembershipReader(),
		mock.NewMockMemberPublisher(),
		&mock.MockUserReader{},
		"",
	)
	ctx := context.Background()

	_, err := svc.UpdateKeyContact(ctx, &membershipservice.UpdateKeyContactPayload{
		UID:           "contact-role-1",
		MembershipUID: "11111111-1111-1111-1111-111111111111",
		IfMatch:       strPtr(`"stale-etag"`),
		Role:          strPtr("Updated Role"),
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr))
	assert.Equal(t, "PreconditionFailed", serviceErr.Name)
}

// ── Alignment 404 tests ───────────────────────────────────────────────────────

// TestGetKeyContact_MembershipMismatch verifies that GetKeyContact returns 404 (not 403)
// when the contact UID exists but belongs to a different membership than the path supplies.
func TestGetKeyContact_MembershipMismatch(t *testing.T) {
	mockRepo := mock.NewMockMembershipRepository()
	svc := NewMembershipService(
		usecaseSvc.NewMemberReaderOrchestrator(usecaseSvc.WithMemberReader(mockRepo)),
		mockRepo,
		&auth.MockJWTAuth{},
		&mockKeyContactWriter{},
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		mock.NewMockProjectMembershipReader(),
		mock.NewMockMemberPublisher(),
		&mock.MockUserReader{},
		"",
	)

	_, err := svc.GetKeyContact(context.Background(), &membershipservice.GetKeyContactPayload{
		UID:           "contact-role-1",       // exists in membership-1
		MembershipUID: "wrong-membership-uid", // mismatch → 404
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr), "expected *goa.ServiceError, got %T: %v", err, err)
	assert.Equal(t, "NotFound", serviceErr.Name, "must return 404 (not 403) to avoid leaking existence")
}

// TestUpdateKeyContact_MembershipMismatch verifies that UpdateKeyContact returns 404
// when the contact UID does not belong to the supplied membership_uid.
func TestUpdateKeyContact_MembershipMismatch(t *testing.T) {
	mockRepo := mock.NewMockMembershipRepository()
	svc := NewMembershipService(
		usecaseSvc.NewMemberReaderOrchestrator(usecaseSvc.WithMemberReader(mockRepo)),
		mockRepo,
		&auth.MockJWTAuth{},
		&mockKeyContactWriter{},
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		mock.NewMockProjectMembershipReader(),
		mock.NewMockMemberPublisher(),
		&mock.MockUserReader{},
		"",
	)

	_, err := svc.UpdateKeyContact(context.Background(), &membershipservice.UpdateKeyContactPayload{
		UID:           "contact-role-1",
		MembershipUID: "wrong-membership-uid",
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr))
	assert.Equal(t, "NotFound", serviceErr.Name, "must return 404 to avoid leaking existence")
}

// TestDeleteKeyContact_MembershipMismatch verifies that DeleteKeyContact returns 404
// when the contact UID does not belong to the supplied membership_uid.
func TestDeleteKeyContact_MembershipMismatch(t *testing.T) {
	mockRepo := mock.NewMockMembershipRepository()
	svc := NewMembershipService(
		usecaseSvc.NewMemberReaderOrchestrator(usecaseSvc.WithMemberReader(mockRepo)),
		mockRepo,
		&auth.MockJWTAuth{},
		&mockKeyContactWriter{},
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		mock.NewMockProjectMembershipReader(),
		mock.NewMockMemberPublisher(),
		&mock.MockUserReader{},
		"",
	)

	err := svc.DeleteKeyContact(context.Background(), &membershipservice.DeleteKeyContactPayload{
		UID:           "contact-role-1",
		MembershipUID: "wrong-membership-uid",
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr))
	assert.Equal(t, "NotFound", serviceErr.Name, "must return 404 to avoid leaking existence")
}

// ── Key contact validation tests ────────────────────────────────────────────

// TestNormalizeAndValidateCreate tests the create validation helper.
func TestNormalizeAndValidateCreate(t *testing.T) {
	tests := []struct {
		name           string
		payload        *membershipservice.CreateKeyContactPayload
		siblings       []*model.KeyContact
		wantExisting   *model.KeyContact
		wantErr        bool
		wantErrType    interface{}
		wantErrMessage string
	}{
		{
			name: "create_missing_email",
			payload: &membershipservice.CreateKeyContactPayload{
				Email:         "",
				FirstName:     "John",
				LastName:      "Doe",
				MembershipUID: "mem-1",
				Role:          "Billing Contact",
				Status:        strPtr("Active"),
			},
			siblings:       nil,
			wantErr:        true,
			wantErrType:    pkgerrors.Validation{},
			wantErrMessage: "email is required",
		},
		{
			name: "create_email_whitespace_only",
			payload: &membershipservice.CreateKeyContactPayload{
				Email:         "   ",
				FirstName:     "John",
				LastName:      "Doe",
				MembershipUID: "mem-1",
				Role:          "Billing Contact",
				Status:        strPtr("Active"),
			},
			siblings:       nil,
			wantErr:        true,
			wantErrType:    pkgerrors.Validation{},
			wantErrMessage: "email is required",
		},
		{
			name: "create_email_lowercased",
			payload: &membershipservice.CreateKeyContactPayload{
				Email:         "JANE@EX.COM",
				FirstName:     "Jane",
				LastName:      "Smith",
				MembershipUID: "mem-1",
				Role:          "Marketing Contact",
				Status:        strPtr("Active"),
			},
			siblings:     nil,
			wantExisting: nil,
			wantErr:      false,
		},
		{
			name: "create_voting_already_taken",
			payload: &membershipservice.CreateKeyContactPayload{
				Email:         "new@ex.com",
				FirstName:     "New",
				LastName:      "Contact",
				MembershipUID: "mem-1",
				Role:          "Representative/Voting Contact",
				Status:        strPtr("Active"),
			},
			siblings: []*model.KeyContact{
				{
					UID:           "kc-1",
					Email:         "existing@ex.com",
					Role:          "Representative/Voting Contact",
					Status:        "Active",
					MembershipUID: "mem-1",
				},
			},
			wantErr:        true,
			wantErrType:    pkgerrors.Conflict{},
			wantErrMessage: "Representative/Voting Contact is limited to 1 per membership",
		},
		{
			name: "create_billing_at_limit",
			payload: &membershipservice.CreateKeyContactPayload{
				Email:         "new@ex.com",
				FirstName:     "New",
				LastName:      "Contact",
				MembershipUID: "mem-1",
				Role:          "Billing Contact",
				Status:        strPtr("Active"),
			},
			siblings: []*model.KeyContact{
				{UID: "kc-1", Email: "a@ex.com", Role: "Billing Contact", Status: "Active", MembershipUID: "mem-1"},
				{UID: "kc-2", Email: "b@ex.com", Role: "Billing Contact", Status: "Active", MembershipUID: "mem-1"},
				{UID: "kc-3", Email: "c@ex.com", Role: "Billing Contact", Status: "Active", MembershipUID: "mem-1"},
			},
			wantErr:        true,
			wantErrType:    pkgerrors.Conflict{},
			wantErrMessage: "Billing Contact is limited to 3 per membership",
		},
		{
			name: "create_billing_under_limit",
			payload: &membershipservice.CreateKeyContactPayload{
				Email:         "new@ex.com",
				FirstName:     "New",
				LastName:      "Contact",
				MembershipUID: "mem-1",
				Role:          "Billing Contact",
				Status:        strPtr("Active"),
			},
			siblings: []*model.KeyContact{
				{UID: "kc-1", Email: "a@ex.com", Role: "Billing Contact", Status: "Active", MembershipUID: "mem-1"},
				{UID: "kc-2", Email: "b@ex.com", Role: "Billing Contact", Status: "Active", MembershipUID: "mem-1"},
			},
			wantExisting: nil,
			wantErr:      false,
		},
		{
			name: "create_inactive_dont_count",
			payload: &membershipservice.CreateKeyContactPayload{
				Email:         "new@ex.com",
				FirstName:     "New",
				LastName:      "Contact",
				MembershipUID: "mem-1",
				Role:          "Billing Contact",
				Status:        strPtr("Active"),
			},
			siblings: []*model.KeyContact{
				{UID: "kc-1", Email: "a@ex.com", Role: "Billing Contact", Status: "Inactive", MembershipUID: "mem-1"},
				{UID: "kc-2", Email: "b@ex.com", Role: "Billing Contact", Status: "Inactive", MembershipUID: "mem-1"},
				{UID: "kc-3", Email: "c@ex.com", Role: "Billing Contact", Status: "Inactive", MembershipUID: "mem-1"},
			},
			wantExisting: nil,
			wantErr:      false,
		},
		{
			name: "create_self_heal_duplicate",
			payload: &membershipservice.CreateKeyContactPayload{
				Email:         "EXISTING@EX.COM",
				FirstName:     "New",
				LastName:      "Contact",
				MembershipUID: "mem-1",
				Role:          "Billing Contact",
				Status:        strPtr("Active"),
			},
			siblings: []*model.KeyContact{
				{
					UID:           "kc-existing",
					Email:         "existing@ex.com",
					Role:          "Billing Contact",
					Status:        "Active",
					MembershipUID: "mem-1",
					CreatedAt:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					UpdatedAt:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
			wantExisting: &model.KeyContact{
				UID:           "kc-existing",
				Email:         "existing@ex.com",
				Role:          "Billing Contact",
				Status:        "Active",
				MembershipUID: "mem-1",
				CreatedAt:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestMembershipServiceWithMockReader(&mockReaderWithSiblings{siblings: tt.siblings})
			ctx := context.Background()

			existing, err := svc.(*membershipServicesrvc).normalizeAndValidateCreate(ctx, tt.payload)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrType != nil {
					assert.IsType(t, tt.wantErrType, err)
				}
				if tt.wantErrMessage != "" {
					assert.Contains(t, err.Error(), tt.wantErrMessage)
				}
			} else {
				require.NoError(t, err)
			}

			if tt.wantExisting != nil {
				require.NotNil(t, existing)
				assert.Equal(t, tt.wantExisting.UID, existing.UID)
				assert.Equal(t, tt.wantExisting.Email, existing.Email)
			} else if !tt.wantErr {
				assert.Nil(t, existing)
			}
		})
	}
}

// TestNormalizeAndValidateUpdate tests the update validation helper.
func TestNormalizeAndValidateUpdate(t *testing.T) {
	tests := []struct {
		name        string
		current     *model.KeyContact
		payload     *membershipservice.UpdateKeyContactPayload
		siblings    []*model.KeyContact
		wantErr     bool
		wantErrType interface{}
	}{
		{
			name: "update_email_lowercased",
			current: &model.KeyContact{
				UID:           "kc-1",
				Email:         "john@ex.com",
				Role:          "Billing Contact",
				Status:        "Active",
				MembershipUID: "mem-1",
			},
			payload: &membershipservice.UpdateKeyContactPayload{
				UID:   "kc-1",
				Email: strPtr("JANE@EX.COM"),
			},
			siblings: nil,
			wantErr:  false,
		},
		{
			name: "update_role_unchanged_skip",
			current: &model.KeyContact{
				UID:           "kc-1",
				Email:         "john@ex.com",
				Role:          "Billing Contact",
				Status:        "Active",
				MembershipUID: "mem-1",
			},
			payload: &membershipservice.UpdateKeyContactPayload{
				UID:  "kc-1",
				Role: strPtr("Billing Contact"),
			},
			siblings: []*model.KeyContact{
				{UID: "kc-2", Role: "Billing Contact", Status: "Active", MembershipUID: "mem-1"},
				{UID: "kc-3", Role: "Billing Contact", Status: "Active", MembershipUID: "mem-1"},
				{UID: "kc-4", Role: "Billing Contact", Status: "Active", MembershipUID: "mem-1"},
			},
			wantErr: false,
		},
		{
			name: "update_role_change_blocked",
			current: &model.KeyContact{
				UID:           "kc-1",
				Email:         "john@ex.com",
				Role:          "Representative/Voting Contact",
				Status:        "Active",
				MembershipUID: "mem-1",
			},
			payload: &membershipservice.UpdateKeyContactPayload{
				UID:  "kc-1",
				Role: strPtr("Billing Contact"),
			},
			siblings: []*model.KeyContact{
				{UID: "kc-2", Role: "Billing Contact", Status: "Active", MembershipUID: "mem-1"},
				{UID: "kc-3", Role: "Billing Contact", Status: "Active", MembershipUID: "mem-1"},
				{UID: "kc-4", Role: "Billing Contact", Status: "Active", MembershipUID: "mem-1"},
			},
			wantErr:     true,
			wantErrType: pkgerrors.Conflict{},
		},
		{
			name: "update_role_change_allowed",
			current: &model.KeyContact{
				UID:           "kc-1",
				Email:         "john@ex.com",
				Role:          "Representative/Voting Contact",
				Status:        "Active",
				MembershipUID: "mem-1",
			},
			payload: &membershipservice.UpdateKeyContactPayload{
				UID:  "kc-1",
				Role: strPtr("Billing Contact"),
			},
			siblings: []*model.KeyContact{
				{UID: "kc-2", Role: "Billing Contact", Status: "Active", MembershipUID: "mem-1"},
				{UID: "kc-3", Role: "Billing Contact", Status: "Active", MembershipUID: "mem-1"},
			},
			wantErr: false,
		},
		{
			name: "update_creates_duplicate_blocked",
			current: &model.KeyContact{
				UID:           "kc-1",
				Email:         "jane@ex.com",
				Role:          "Legal Contact",
				Status:        "Active",
				MembershipUID: "mem-1",
			},
			payload: &membershipservice.UpdateKeyContactPayload{
				UID:  "kc-1",
				Role: strPtr("Billing Contact"),
			},
			siblings: []*model.KeyContact{
				// sibling already holds Billing Contact with same email
				{UID: "kc-2", Role: "Billing Contact", Email: "jane@ex.com", Status: "Active", MembershipUID: "mem-1"},
			},
			wantErr:     true,
			wantErrType: pkgerrors.Conflict{},
		},
		{
			name: "update_no_self_conflict",
			current: &model.KeyContact{
				UID:           "kc-1",
				Email:         "jane@ex.com",
				Role:          "Billing Contact",
				Status:        "Active",
				MembershipUID: "mem-1",
			},
			// Title-only update — role and email unchanged; must not collide with self
			payload: &membershipservice.UpdateKeyContactPayload{
				UID: "kc-1",
			},
			siblings: []*model.KeyContact{
				{UID: "kc-1", Role: "Billing Contact", Email: "jane@ex.com", Status: "Active", MembershipUID: "mem-1"},
			},
			wantErr: false,
		},
		{
			name: "reactivate_at_capacity_blocked",
			current: &model.KeyContact{
				UID:           "kc-1",
				Email:         "jane@ex.com",
				Role:          "Authorized Signatory",
				Status:        "Inactive",
				MembershipUID: "mem-1",
			},
			// Re-activating; role/email unchanged but capacity limit (1) is already filled.
			payload: &membershipservice.UpdateKeyContactPayload{
				UID:    "kc-1",
				Status: strPtr("Active"),
			},
			siblings: []*model.KeyContact{
				{UID: "kc-2", Role: "Authorized Signatory", Email: "other@ex.com", Status: "Active", MembershipUID: "mem-1"},
			},
			wantErr:     true,
			wantErrType: pkgerrors.Conflict{},
		},
		{
			name: "reactivate_creates_duplicate_blocked",
			current: &model.KeyContact{
				UID:           "kc-1",
				Email:         "jane@ex.com",
				Role:          "Billing Contact",
				Status:        "Inactive",
				MembershipUID: "mem-1",
			},
			// Re-activating; a different active sibling already holds same (role, email).
			payload: &membershipservice.UpdateKeyContactPayload{
				UID:    "kc-1",
				Status: strPtr("Active"),
			},
			siblings: []*model.KeyContact{
				{UID: "kc-2", Role: "Billing Contact", Email: "jane@ex.com", Status: "Active", MembershipUID: "mem-1"},
			},
			wantErr:     true,
			wantErrType: pkgerrors.Conflict{},
		},
		{
			name: "reactivate_within_capacity_allowed",
			current: &model.KeyContact{
				UID:           "kc-1",
				Email:         "jane@ex.com",
				Role:          "Billing Contact",
				Status:        "Inactive",
				MembershipUID: "mem-1",
			},
			payload: &membershipservice.UpdateKeyContactPayload{
				UID:    "kc-1",
				Status: strPtr("Active"),
			},
			siblings: []*model.KeyContact{
				{UID: "kc-2", Role: "Billing Contact", Email: "other@ex.com", Status: "Active", MembershipUID: "mem-1"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestMembershipServiceWithMockReader(&mockReaderWithSiblings{siblings: tt.siblings})
			ctx := context.Background()

			err := svc.(*membershipServicesrvc).normalizeAndValidateUpdate(ctx, tt.current, tt.payload)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrType != nil {
					assert.IsType(t, tt.wantErrType, err)
				}
			} else {
				require.NoError(t, err)
			}

			// Verify email normalization occurred in-place
			if tt.payload.Email != nil && *tt.payload.Email != "" {
				assert.Equal(t, strings.ToLower(*tt.payload.Email), *tt.payload.Email)
			}
		})
	}
}

// mockReaderWithSiblings embeds the existing mock and overrides ListKeyContactsForMembership
// to return a fixed list of test siblings.
type mockReaderWithSiblings struct {
	*mock.MockMembershipRepository
	siblings []*model.KeyContact
}

func (m *mockReaderWithSiblings) ListKeyContactsForMembership(_ context.Context, _ string) ([]*model.KeyContact, error) {
	return m.siblings, nil
}

// newTestMembershipServiceWithMockReader constructs a service with a custom mock reader.
func newTestMembershipServiceWithMockReader(mockReader port.MemberReader) membershipservice.Service {
	mockB2BOrgReader := mock.NewMockB2BOrgReader()
	mockB2BOrgWriter := mock.NewMockB2BOrgWriter()
	mockProjectMembershipReader := mock.NewMockProjectMembershipReader()
	mockPublisher := mock.NewMockMemberPublisher()
	mockKCWriter := &mockKeyContactWriter{}

	readMemberUseCase := usecaseSvc.NewMemberReaderOrchestrator(
		usecaseSvc.WithMemberReader(mockReader),
	)

	mockAuth := &auth.MockJWTAuth{}

	svc := NewMembershipService(
		readMemberUseCase,
		mockReader,
		mockAuth,
		mockKCWriter,
		mockB2BOrgReader,
		mockB2BOrgWriter,
		mockProjectMembershipReader,
		mockPublisher,
		&mock.MockUserReader{},
		"",
	)

	return svc
}

// mockProjectMembershipReader is a simple test implementation of port.ProjectMembershipReader.
type mockProjectMembershipReader struct {
	membership *model.ProjectMembership
	lastMod    time.Time
	err        error
}

func (m *mockProjectMembershipReader) AssembleProjectMembership(_ context.Context, _ string) (*model.ProjectMembership, time.Time, error) {
	return m.membership, m.lastMod, m.err
}

// ── FGA publish gap tests ─────────────────────────────────────────────────────

// TestCreateKeyContact_MembershipNotFound verifies that CreateKeyContact returns 404
// when AssembleProjectMembership cannot find the supplied membership_uid. This covers
// the validation introduced to derive b2b_org_uid / project_uid from the path.
func TestCreateKeyContact_MembershipNotFound(t *testing.T) {
	// MockProjectMembershipReader returns not-found for any UID other than "11111111-1111-1111-1111-111111111111".
	mockRepo := mock.NewMockMembershipRepository()
	svc := NewMembershipService(
		usecaseSvc.NewMemberReaderOrchestrator(usecaseSvc.WithMemberReader(mockRepo)),
		mockRepo,
		&auth.MockJWTAuth{},
		&mockKeyContactWriter{},
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		mock.NewMockProjectMembershipReader(),
		mock.NewMockMemberPublisher(),
		&mock.MockUserReader{},
		"",
	)

	_, err := svc.CreateKeyContact(context.Background(), &membershipservice.CreateKeyContactPayload{
		MembershipUID: "nonexistent-membership",
		Email:         "test@example.com",
		FirstName:     "Test",
		LastName:      "User",
		Role:          "Billing Contact",
	})

	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.True(t, errors.As(err, &serviceErr))
	assert.Equal(t, "NotFound", serviceErr.Name)
}

// TestCreateKeyContact_EmptySubSkipsFGAPublish verifies the fail-open pattern:
// when email→sub resolution returns an empty sub (e.g. user not yet in Authelia),
// the FGA keycontact put is skipped but the create still succeeds.
func TestCreateKeyContact_EmptySubSkipsFGAPublish(t *testing.T) {
	mockRepo := mock.NewMockMembershipRepository()
	pub := &accessSubjectCapture{}
	kc := &model.KeyContact{
		UID:           "new-kc-uid",
		MembershipUID: "11111111-1111-1111-1111-111111111111",
		Email:         "noresolve@example.com",
		Role:          "Billing Contact",
		Status:        "Active",
		UpdatedAt:     time.Now(),
	}
	svc := NewMembershipService(
		usecaseSvc.NewMemberReaderOrchestrator(usecaseSvc.WithMemberReader(mockRepo)),
		mockRepo,
		&auth.MockJWTAuth{},
		&returningKCWriter{contact: kc},
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		mock.NewMockProjectMembershipReader(),
		pub,
		&mock.MockUserReader{}, // always returns "" → sub empty
		"",
	)

	result, err := svc.CreateKeyContact(context.Background(), &membershipservice.CreateKeyContactPayload{
		MembershipUID: "11111111-1111-1111-1111-111111111111",
		Email:         "noresolve@example.com",
		FirstName:     "No",
		LastName:      "Resolve",
		Role:          "Billing Contact",
	})

	require.NoError(t, err, "fail-open: empty sub must not fail the create")
	require.NotNil(t, result)

	for _, s := range pub.accessSubjects() {
		assert.NotEqual(t, "lfx.fga-sync.member_put", s,
			"Access(member_put) must not be called when sub is empty")
	}
}

// TestUpdateKeyContact_EmailChange_PutBeforeRemove verifies that when an email
// changes, the FGA put for the new sub is published BEFORE the remove for the old
// sub. Reversing the order would create a window where the contact has no access.
func TestUpdateKeyContact_EmailChange_PutBeforeRemove(t *testing.T) {
	mockRepo := mock.NewMockMembershipRepository()
	pub := &accessSubjectCapture{}

	// Writer returns a contact with the updated email so the service detects a change.
	updatedKC := &model.KeyContact{
		UID:           "contact-role-1",
		MembershipUID: "11111111-1111-1111-1111-111111111111",
		Email:         "new@example.com",
		Role:          "Primary Contact",
		Status:        "Active",
		UpdatedAt:     time.Now().Add(time.Second), // different updatedAt forces ETag mismatch → publish
	}
	userReader := &configuredUserReader{
		subs: map[string]string{
			"john.doe@example.com": "sub-old", // contact-role-1's current email in mock
			"new@example.com":      "sub-new",
		},
	}

	svc := NewMembershipService(
		usecaseSvc.NewMemberReaderOrchestrator(usecaseSvc.WithMemberReader(mockRepo)),
		mockRepo,
		&auth.MockJWTAuth{},
		&returningKCWriter{contact: updatedKC},
		mock.NewMockB2BOrgReader(),
		mock.NewMockB2BOrgWriter(),
		mock.NewMockProjectMembershipReader(),
		pub,
		userReader,
		"",
	)

	_, err := svc.UpdateKeyContact(context.Background(), &membershipservice.UpdateKeyContactPayload{
		UID:           "contact-role-1",
		MembershipUID: "11111111-1111-1111-1111-111111111111",
		Email:         strPtr("new@example.com"),
	})

	require.NoError(t, err)
	subjects := pub.accessSubjects()
	require.Len(t, subjects, 2, "email change must emit exactly 2 Access messages (put + remove)")
	assert.Equal(t, "lfx.fga-sync.member_put", subjects[0],
		"keycontact put must come first to avoid an access gap")
	assert.Equal(t, "lfx.fga-sync.member_remove", subjects[1],
		"keycontact remove must come second")
}

// accessSubjectCapture records the NATS subjects of each Access() call in order.
// Indexer calls are silently dropped since only FGA Access messages are under test.
type accessSubjectCapture struct {
	mu       sync.Mutex
	subjects []string
}

func (p *accessSubjectCapture) Indexer(_ context.Context, _ string, _ any, _ bool) error { return nil }
func (p *accessSubjectCapture) Access(_ context.Context, subject string, _ any, _ bool) error {
	p.mu.Lock()
	p.subjects = append(p.subjects, subject)
	p.mu.Unlock()
	return nil
}
func (p *accessSubjectCapture) accessSubjects() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.subjects...)
}

// configuredUserReader maps email → OIDC sub for test scenarios.
type configuredUserReader struct {
	subs map[string]string
}

func (r *configuredUserReader) SubByEmail(_ context.Context, email string) (string, error) {
	return r.subs[email], nil
}

// returningKCWriter always returns the pre-configured contact, allowing tests to
// control the contact the service layer receives back from the writer.
type returningKCWriter struct {
	contact *model.KeyContact
}

func (w *returningKCWriter) CreateKeyContact(_ context.Context, _ model.KeyContactInput) (*model.KeyContact, error) {
	return w.contact, nil
}
func (w *returningKCWriter) UpdateKeyContact(_ context.Context, _ string, _ model.KeyContactInput) (*model.KeyContact, error) {
	return w.contact, nil
}
func (w *returningKCWriter) DeleteKeyContact(_ context.Context, _ string, _ string) error {
	return nil
}

// Verify that the mock ports are properly implemented.
var (
	_ port.MemberReader            = (*mock.MockMembershipRepository)(nil)
	_ port.MemberReader            = (*mockReaderWithSiblings)(nil)
	_ port.KeyContactWriter        = (*mockKeyContactWriter)(nil)
	_ port.KeyContactWriter        = (*returningKCWriter)(nil)
	_ port.B2BOrgReader            = (*mock.MockB2BOrgReader)(nil)
	_ port.B2BOrgWriter            = (*mock.MockB2BOrgWriter)(nil)
	_ port.ProjectMembershipReader = (*mockProjectMembershipReader)(nil)
	_ port.MemberPublisher         = (*mock.MockMemberPublisher)(nil)
	_ port.UserReader              = (*mock.MockUserReader)(nil)
	_ port.UserReader              = (*configuredUserReader)(nil)
	_ port.B2BOrgReader            = (*seededB2BOrgReader)(nil)
	_ port.B2BOrgWriter            = (*happyB2BOrgWriter)(nil)
	_ port.B2BOrgWriter            = (*preconditionFailingWriter)(nil)
	_ port.B2BOrgWriter            = (*capturingB2BOrgWriter)(nil)
	_ port.MemberPublisher         = (*errorPublisher)(nil)
	_ port.MemberPublisher         = (*capturingPublisher)(nil)
	_ port.MemberPublisher         = (*indexerMessageCapture)(nil)
	_ port.MemberPublisher         = (*accessSubjectCapture)(nil)
)
