// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"testing"

	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/mock"
	svc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedUIDInviteSender is a test double that returns a pre-configured InviteUID from SendInvite.
type fixedUIDInviteSender struct{ uid string }

func (f *fixedUIDInviteSender) SendInvite(_ context.Context, _ inviteapi.SendInviteRequest) (port.InviteResult, error) {
	return port.InviteResult{InviteUID: f.uid}, nil
}

// noScanReader wraps MockB2BOrgSettings and fails the test if ListSettingsOrgUIDs is called.
// Used to verify that the index fast-path in InviteAcceptedService.Handle skips the full scan.
type noScanReader struct {
	t     *testing.T
	inner *mock.MockB2BOrgSettings
}

func (r *noScanReader) GetSettings(ctx context.Context, orgUID string) (*model.B2BOrgSettings, uint64, error) {
	return r.inner.GetSettings(ctx, orgUID)
}

func (r *noScanReader) ListSettingsOrgUIDs(_ context.Context) ([]string, error) {
	r.t.Helper()
	r.t.Fatal("ListSettingsOrgUIDs must NOT be called when the index fast-path resolves the org")
	return nil, nil
}

func (r *noScanReader) LookupInviteOrgUID(ctx context.Context, inviteUUID string) (string, error) {
	return r.inner.LookupInviteOrgUID(ctx, inviteUUID)
}

// newOrgSettingsWriterWithSender constructs an OrgSettingsWriter with an invite sender wired.
// Used by reconcile tests to exercise the invite-send path that stamps an InviteUUID.
func newOrgSettingsWriterWithSender(store *mock.MockB2BOrgSettings, orgReader port.B2BOrgReader, pub port.MemberPublisher, sender port.InviteSender) svc.OrgSettingsWriter {
	return svc.NewOrgSettingsWriter(
		svc.WithOrgSettingsReader(store),
		svc.WithOrgSettingsWriter(store),
		svc.WithOrgSettingsB2BOrgReader(orgReader),
		svc.WithOrgSettingsPublisher(pub),
		svc.WithOrgSettingsInviteSender(sender),
	)
}

// ── B4: Index reconciliation ───────────────────────────────────────────────

func TestOrgSettingsWriter_AddPrincipal_SeedsInviteIndex(t *testing.T) {
	// Adding a new principal (no pre-existing LFID) must PUT an InviteUUID→orgUID
	// entry into the secondary index so InviteAcceptedService can find the org O(1).
	const inviteUID = "invite-seed-test"
	store := mock.NewMockB2BOrgSettings()
	// Seed empty settings so the org-existence check is skipped.
	store.Seed(testOrgUID, &model.B2BOrgSettings{UID: testOrgUID}, 1)

	sender := &fixedUIDInviteSender{uid: inviteUID}
	writer := newOrgSettingsWriterWithSender(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher(), sender)

	_, err := writer.AddPrincipal(context.Background(), svc.B2BOrgSettingsAddPrincipal{
		OrgUID: testOrgUID, Email: "new@example.com", InvitedAs: model.B2BOrgRoleWriter, Name: "New User",
	})
	require.NoError(t, err)

	idx := store.InviteIndex()
	if assert.Contains(t, idx, inviteUID, "invite UUID must be in the secondary index after AddPrincipal") {
		assert.Equal(t, testOrgUID, idx[inviteUID])
	}
}

func TestOrgSettingsWriter_RemovePrincipal_ClearsInviteIndex(t *testing.T) {
	// Removing a pending principal must DELETE its InviteUUID entry from the secondary index.
	const inviteUID = "invite-to-remove"
	ctx := context.Background()

	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{{
			Email:        "pending@example.com",
			InviteUUID:   inviteUID,
			InvitedAs:    model.B2BOrgRoleWriter,
			InviteStatus: model.InviteStatusPending,
		}},
	}, 1)
	require.NoError(t, store.PutInviteIndex(ctx, inviteUID, testOrgUID))

	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	_, err := writer.RemovePrincipal(ctx, svc.B2BOrgSettingsRemovePrincipal{
		OrgUID: testOrgUID, Email: "pending@example.com",
	})
	require.NoError(t, err)

	assert.NotContains(t, store.InviteIndex(), inviteUID,
		"invite UUID must be deleted from the secondary index after RemovePrincipal")
}

func TestOrgSettingsWriter_Resend_UpdatesInviteIndex(t *testing.T) {
	// Resending an invite (same email + role, pending entry) must DELETE the old UUID and
	// PUT the new one. The net result is the index reflects only the freshly issued UUID.
	const oldUID = "invite-old"
	const newUID = "invite-new"
	ctx := context.Background()

	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{{
			Email:        "resend@example.com",
			InviteUUID:   oldUID,
			InvitedAs:    model.B2BOrgRoleWriter,
			InviteStatus: model.InviteStatusPending,
		}},
	}, 1)
	require.NoError(t, store.PutInviteIndex(ctx, oldUID, testOrgUID))

	sender := &fixedUIDInviteSender{uid: newUID}
	writer := newOrgSettingsWriterWithSender(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher(), sender)

	// AddPrincipal for the same email + same role triggers the resend-in-place path.
	_, err := writer.AddPrincipal(ctx, svc.B2BOrgSettingsAddPrincipal{
		OrgUID: testOrgUID, Email: "resend@example.com", InvitedAs: model.B2BOrgRoleWriter,
	})
	require.NoError(t, err)

	idx := store.InviteIndex()
	assert.NotContains(t, idx, oldUID, "old invite UUID must be deleted from the index after resend")
	if assert.Contains(t, idx, newUID, "new invite UUID must be added to the index after resend") {
		assert.Equal(t, testOrgUID, idx[newUID])
	}
}

// ── B5: Index fast-path and fallback ──────────────────────────────────────

func TestInviteAcceptedService_Handle_IndexFastPath_SkipsScan(t *testing.T) {
	// When the secondary index has an entry for the invite UID, Handle must resolve
	// the org O(1) and MUST NOT call ListSettingsOrgUIDs (full scan).
	const inviteUID = "invite-fast-path"
	ctx := context.Background()

	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{{
			Email:        "fp@example.com",
			InviteUUID:   inviteUID,
			InvitedAs:    model.B2BOrgRoleWriter,
			InviteStatus: model.InviteStatusPending,
		}},
	}, 1)
	require.NoError(t, store.PutInviteIndex(ctx, inviteUID, testOrgUID))

	// noScanReader intercepts the reader used by InviteAcceptedService.
	// Handle's index lookup (LookupInviteOrgUID) and the nested GetSettings call both
	// go through this wrapper, but ListSettingsOrgUIDs must never be reached.
	reader := &noScanReader{t: t, inner: store}
	inner := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())
	invSvc := svc.NewInviteAcceptedService(
		svc.WithInviteAcceptedSettingsReader(reader),
		svc.WithInviteAcceptedOrgSettingsWriter(inner),
	)

	err := invSvc.Handle(ctx, acceptedEvent(inviteUID, "fp@example.com", "auth0|fp"))
	require.NoError(t, err)

	saved, _, _ := store.GetSettings(ctx, testOrgUID)
	require.Len(t, saved.Writers, 1)
	assert.Equal(t, "auth0|fp", saved.Writers[0].Username, "entry must be promoted to accepted")
	assert.Equal(t, model.InviteStatusAccepted, saved.Writers[0].InviteStatus)
}

func TestInviteAcceptedService_Handle_IndexFallback_OnMiss_StillPromotes(t *testing.T) {
	// When no index entry exists for the invite UID (legacy entry written before the
	// index was introduced), Handle must fall through to the full scan and still
	// promote the matching pending entry.
	const inviteUID = "invite-legacy-no-index"
	ctx := context.Background()

	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{{
			Email:        "legacy@example.com",
			InviteUUID:   inviteUID,
			InvitedAs:    model.B2BOrgRoleWriter,
			InviteStatus: model.InviteStatusPending,
		}},
	}, 1)
	// No PutInviteIndex — simulating an entry that predates the secondary index.

	inner := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())
	invSvc := newInviteAcceptedService(store, inner)

	err := invSvc.Handle(ctx, acceptedEvent(inviteUID, "legacy@example.com", "auth0|legacy"))
	require.NoError(t, err)

	saved, _, _ := store.GetSettings(ctx, testOrgUID)
	require.Len(t, saved.Writers, 1)
	assert.Equal(t, "auth0|legacy", saved.Writers[0].Username, "entry must be promoted even on index miss")
}

func TestInviteAcceptedService_Handle_AcceptanceClearsInviteIndex(t *testing.T) {
	// Accepting an invite must remove the InviteUUID→orgUID entry from the secondary index
	// (the UUID is cleared on the entry; reconcileInviteIndex detects the removal and deletes it).
	const inviteUID = "invite-to-accept"
	ctx := context.Background()

	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{{
			Email:        "accept@example.com",
			InviteUUID:   inviteUID,
			InvitedAs:    model.B2BOrgRoleWriter,
			InviteStatus: model.InviteStatusPending,
		}},
	}, 1)
	require.NoError(t, store.PutInviteIndex(ctx, inviteUID, testOrgUID))

	inner := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())
	invSvc := newInviteAcceptedService(store, inner)

	err := invSvc.Handle(ctx, acceptedEvent(inviteUID, "accept@example.com", "auth0|accept"))
	require.NoError(t, err)

	assert.NotContains(t, store.InviteIndex(), inviteUID,
		"InviteUUID must be removed from the secondary index after acceptance")
}
