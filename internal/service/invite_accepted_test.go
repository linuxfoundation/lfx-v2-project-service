// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"testing"

	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/mock"
	svc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingWriter wraps a real OrgSettingsWriter and records how many times Update is called.
// It can be configured to return a fixed error on every Update call.
type countingWriter struct {
	inner     svc.OrgSettingsWriter
	updateErr error
	calls     int
}

func (c *countingWriter) Update(ctx context.Context, in svc.B2BOrgSettingsUpdate) (*model.B2BOrgSettings, error) {
	c.calls++
	if c.updateErr != nil {
		return nil, c.updateErr
	}
	return c.inner.Update(ctx, in)
}

func (c *countingWriter) AddPrincipal(ctx context.Context, in svc.B2BOrgSettingsAddPrincipal) (*model.B2BOrgSettings, error) {
	return c.inner.AddPrincipal(ctx, in)
}

func (c *countingWriter) ChangePrincipalRole(ctx context.Context, in svc.B2BOrgSettingsChangeRole) (*model.B2BOrgSettings, error) {
	return c.inner.ChangePrincipalRole(ctx, in)
}

func (c *countingWriter) RemovePrincipal(ctx context.Context, in svc.B2BOrgSettingsRemovePrincipal) (*model.B2BOrgSettings, error) {
	return c.inner.RemovePrincipal(ctx, in)
}

func newInviteAcceptedService(store *mock.MockB2BOrgSettings, writer svc.OrgSettingsWriter) *svc.InviteAcceptedService {
	return svc.NewInviteAcceptedService(
		svc.WithInviteAcceptedSettingsReader(store),
		svc.WithInviteAcceptedOrgSettingsWriter(writer),
	)
}

func acceptedEvent(inviteUID, email, acceptedBy string) inviteapi.InviteServiceAcceptedEvent {
	return inviteapi.InviteServiceAcceptedEvent{
		Invite: inviteapi.Invite{
			UID:        inviteUID,
			AcceptedBy: acceptedBy,
			Recipient:  inviteapi.Recipient{Email: email},
		},
	}
}

// ── Handle ────────────────────────────────────────────────────────────────

func TestInviteAcceptedService_Handle_PromotesPendingWriter(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{{
			Email:        "alice@example.com",
			InviteUUID:   "invite-writer-1",
			InvitedAs:    "writer",
			InviteStatus: model.InviteStatusPending,
		}},
	}, 1)

	inner := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())
	svc := newInviteAcceptedService(store, inner)

	err := svc.Handle(context.Background(), acceptedEvent("invite-writer-1", "alice@example.com", "auth0|alice"))

	require.NoError(t, err)
	saved, _, _ := store.GetSettings(context.Background(), testOrgUID)
	require.Len(t, saved.Writers, 1)
	w := saved.Writers[0]
	assert.Equal(t, "auth0|alice", w.Username, "Username must be stamped from AcceptedBy")
	assert.Equal(t, model.InviteStatusAccepted, w.InviteStatus)
	assert.NotNil(t, w.AcceptedAt)
	assert.Empty(t, w.InviteUUID, "InviteUUID must be cleared on acceptance")
}

func TestInviteAcceptedService_Handle_PromotesPendingAuditor(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Auditors: []model.B2BOrgUser{{
			Email:        "bob@example.com",
			InviteUUID:   "invite-auditor-1",
			InvitedAs:    "auditor",
			InviteStatus: model.InviteStatusPending,
		}},
	}, 1)

	inner := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())
	svc := newInviteAcceptedService(store, inner)

	err := svc.Handle(context.Background(), acceptedEvent("invite-auditor-1", "bob@example.com", "auth0|bob"))

	require.NoError(t, err)
	saved, _, _ := store.GetSettings(context.Background(), testOrgUID)
	require.Len(t, saved.Auditors, 1)
	a := saved.Auditors[0]
	assert.Equal(t, "auth0|bob", a.Username)
	assert.Equal(t, model.InviteStatusAccepted, a.InviteStatus)
	assert.Empty(t, a.InviteUUID)
}

func TestInviteAcceptedService_Handle_EmailFallback_PromotesPendingEntry(t *testing.T) {
	// No InviteUUID on the entry — fallback to email match.
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{{
			Email:        "carol@example.com",
			InvitedAs:    "writer",
			InviteStatus: model.InviteStatusPending,
		}},
	}, 1)

	inner := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())
	svc := newInviteAcceptedService(store, inner)

	// ev.UID is empty — relies solely on email fallback.
	err := svc.Handle(context.Background(), acceptedEvent("", "carol@example.com", "auth0|carol"))

	require.NoError(t, err)
	saved, _, _ := store.GetSettings(context.Background(), testOrgUID)
	require.Len(t, saved.Writers, 1)
	assert.Equal(t, "auth0|carol", saved.Writers[0].Username)
}

func TestInviteAcceptedService_Handle_NoMatch_IsNoOp(t *testing.T) {
	// Event UID does not match any entry — should not error and must not update the store.
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{{
			Email:        "dave@example.com",
			InviteUUID:   "other-uuid",
			InviteStatus: model.InviteStatusPending,
		}},
	}, 1)

	inner := &countingWriter{inner: newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())}
	svc := newInviteAcceptedService(store, inner)

	err := svc.Handle(context.Background(), acceptedEvent("unknown-uuid", "", "auth0|nobody"))

	require.NoError(t, err)
	assert.Equal(t, 0, inner.calls, "Update must not be called when there is no matching entry")
}

func TestInviteAcceptedService_Handle_RevisionConflict_RetriesThreeTimes(t *testing.T) {
	// When Update always returns Conflict, the retry loop must exhaust 3 attempts and give up
	// without propagating an error.
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{{
			Email:        "eve@example.com",
			InviteUUID:   "invite-eve",
			InviteStatus: model.InviteStatusPending,
		}},
	}, 1)

	inner := &countingWriter{
		inner:     newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher()),
		updateErr: pkgerrors.NewConflict("concurrent write"),
	}
	svc := newInviteAcceptedService(store, inner)

	err := svc.Handle(context.Background(), acceptedEvent("invite-eve", "eve@example.com", "auth0|eve"))

	require.NoError(t, err, "revision conflict must not propagate to caller")
	assert.Equal(t, 3, inner.calls, "must retry exactly 3 times before giving up")
}

func TestInviteAcceptedService_Handle_MalformedEvent_BothUIDAndEmailEmpty_DropsEvent(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{UID: testOrgUID}, 1)

	inner := &countingWriter{inner: newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())}
	svc := newInviteAcceptedService(store, inner)

	err := svc.Handle(context.Background(), acceptedEvent("", "", "auth0|someone"))

	require.NoError(t, err)
	assert.Equal(t, 0, inner.calls, "malformed event must be dropped without scanning")
}

func TestInviteAcceptedService_Handle_MalformedEvent_AcceptedByEmpty_DropsEvent(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{UID: testOrgUID}, 1)

	inner := &countingWriter{inner: newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())}
	svc := newInviteAcceptedService(store, inner)

	err := svc.Handle(context.Background(), acceptedEvent("invite-1", "user@example.com", ""))

	require.NoError(t, err)
	assert.Equal(t, 0, inner.calls, "event missing AcceptedBy must be dropped")
}
