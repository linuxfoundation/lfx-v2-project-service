// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/mock"
	svc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/etag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isConflict reports whether err is (or wraps) a pkgerrors.Conflict.
func isConflict(err error) bool {
	var c pkgerrors.Conflict
	return errors.As(err, &c)
}

// isValidation reports whether err is (or wraps) a pkgerrors.Validation.
func isValidation(err error) bool {
	var v pkgerrors.Validation
	return errors.As(err, &v)
}

// seedTwoAdmins seeds an org with two accepted admin writers (each with a username),
// the canonical shape we must protect: per-principal mutations must never disturb the
// other accepted member (the root cause of the prod incident was dropping username).
func seedTwoAdmins(store *mock.MockB2BOrgSettings) {
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{
			{Email: "alice@example.com", Name: "Alice", Username: "auth0|alice", InvitedAs: "writer", InviteStatus: model.InviteStatusAccepted},
			{Email: "bob@example.com", Name: "Bob", Username: "auth0|bob", InvitedAs: "writer", InviteStatus: model.InviteStatusAccepted},
		},
	}, 1)
}

func findUser(users []model.B2BOrgUser, email string) (model.B2BOrgUser, bool) {
	for _, u := range users {
		if u.Email == email {
			return u, true
		}
	}
	return model.B2BOrgUser{}, false
}

// ── AddPrincipal ──────────────────────────────────────────────────────────────

func TestOrgSettingsWriter_AddPrincipal_PreservesExistingMembers(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	seedTwoAdmins(store)
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	result, err := writer.AddPrincipal(context.Background(), svc.B2BOrgSettingsAddPrincipal{
		OrgUID: testOrgUID, Email: "Carol@Example.com", InvitedAs: "auditor", Name: "Carol",
	})

	require.NoError(t, err)
	// Both original admins must remain accepted WITH their usernames (regression guard).
	alice, ok := findUser(result.Writers, "alice@example.com")
	require.True(t, ok)
	assert.Equal(t, model.InviteStatusAccepted, alice.EffectiveStatus())
	assert.Equal(t, "auth0|alice", alice.Username)
	bob, ok := findUser(result.Writers, "bob@example.com")
	require.True(t, ok)
	assert.Equal(t, "auth0|bob", bob.Username)
	// New invitee lands as a pending auditor (email lowercased, no username).
	carol, ok := findUser(result.Auditors, "carol@example.com")
	require.True(t, ok)
	assert.Equal(t, model.InviteStatusPending, carol.EffectiveStatus())
	assert.Empty(t, carol.Username)
}

func TestOrgSettingsWriter_AddPrincipal_DuplicateIsConflict(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	seedTwoAdmins(store)
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	_, err := writer.AddPrincipal(context.Background(), svc.B2BOrgSettingsAddPrincipal{
		OrgUID: testOrgUID, Email: "alice@example.com", InvitedAs: "auditor",
	})
	require.Error(t, err)
	assert.True(t, isConflict(err), "re-inviting an existing member must be a Conflict, got %T", err)
}

func TestOrgSettingsWriter_AddPrincipal_RejectsBadRole(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	seedTwoAdmins(store)
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	_, err := writer.AddPrincipal(context.Background(), svc.B2BOrgSettingsAddPrincipal{
		OrgUID: testOrgUID, Email: "x@example.com", InvitedAs: "owner",
	})
	require.Error(t, err)
}

// TestOrgSettingsWriter_AddPrincipal_EnforcesMaxPrincipals verifies the per-principal
// add rejects a write that would push a relation list past the maxPrincipals bound,
// matching the cap enforced by the bulk Update path.
func TestOrgSettingsWriter_AddPrincipal_EnforcesMaxPrincipals(t *testing.T) {
	const maxPrincipals = 700
	auditors := make([]model.B2BOrgUser, 0, maxPrincipals)
	for i := 0; i < maxPrincipals; i++ {
		auditors = append(auditors, model.B2BOrgUser{
			Email:        fmt.Sprintf("user%d@example.com", i),
			InvitedAs:    "auditor",
			InviteStatus: model.InviteStatusPending,
		})
	}
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{
			{Email: "alice@example.com", Username: "auth0|alice", InvitedAs: "writer", InviteStatus: model.InviteStatusAccepted},
		},
		Auditors: auditors,
	}, 1)
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	_, err := writer.AddPrincipal(context.Background(), svc.B2BOrgSettingsAddPrincipal{
		OrgUID: testOrgUID, Email: "overflow@example.com", InvitedAs: "auditor",
	})
	require.Error(t, err)
	assert.True(t, isValidation(err), "exceeding maxPrincipals must be a Validation error, got %T", err)
}

// ── ChangePrincipalRole ───────────────────────────────────────────────────────

func TestOrgSettingsWriter_ChangeRole_PreservesUsernameAndOtherMembers(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	seedTwoAdmins(store)
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	result, err := writer.ChangePrincipalRole(context.Background(), svc.B2BOrgSettingsChangeRole{
		OrgUID: testOrgUID, Email: "bob@example.com", InvitedAs: "auditor",
	})

	require.NoError(t, err)
	// Bob moved to auditors, still accepted, username intact.
	bob, ok := findUser(result.Auditors, "bob@example.com")
	require.True(t, ok)
	assert.Equal(t, model.InviteStatusAccepted, bob.EffectiveStatus())
	assert.Equal(t, "auth0|bob", bob.Username)
	assert.Equal(t, "auditor", bob.InvitedAs)
	// Alice untouched and still an accepted writer.
	alice, ok := findUser(result.Writers, "alice@example.com")
	require.True(t, ok)
	assert.Equal(t, "auth0|alice", alice.Username)
	_, stillWriter := findUser(result.Writers, "bob@example.com")
	assert.False(t, stillWriter, "bob must no longer be a writer")
}

func TestOrgSettingsWriter_ChangeRole_LastAdminDemotionBlocked(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{
			{Email: "alice@example.com", Username: "auth0|alice", InvitedAs: "writer", InviteStatus: model.InviteStatusAccepted},
		},
	}, 1)
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	_, err := writer.ChangePrincipalRole(context.Background(), svc.B2BOrgSettingsChangeRole{
		OrgUID: testOrgUID, Email: "alice@example.com", InvitedAs: "auditor",
	})
	require.Error(t, err)
	assert.True(t, isConflict(err), "demoting the only Admin must be a Conflict, got %T", err)
}

func TestOrgSettingsWriter_ChangeRole_NotFound(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	seedTwoAdmins(store)
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	_, err := writer.ChangePrincipalRole(context.Background(), svc.B2BOrgSettingsChangeRole{
		OrgUID: testOrgUID, Email: "ghost@example.com", InvitedAs: "auditor",
	})
	require.Error(t, err)
}

func TestOrgSettingsWriter_ChangeRole_IfMatchMismatch(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	seedTwoAdmins(store)
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	_, err := writer.ChangePrincipalRole(context.Background(), svc.B2BOrgSettingsChangeRole{
		OrgUID: testOrgUID, Email: "bob@example.com", InvitedAs: "auditor", IfMatch: "stale-etag",
	})
	require.Error(t, err)
}

func TestOrgSettingsWriter_ChangeRole_IfMatchMatchSucceeds(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	seedTwoAdmins(store)
	seeded, _, err := store.GetSettings(context.Background(), testOrgUID)
	require.NoError(t, err)
	etagVal, err := etag.LFXEtag(seeded)
	require.NoError(t, err)

	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())
	_, err = writer.ChangePrincipalRole(context.Background(), svc.B2BOrgSettingsChangeRole{
		OrgUID: testOrgUID, Email: "bob@example.com", InvitedAs: "auditor", IfMatch: etagVal,
	})
	assert.NoError(t, err)
}

// ── RemovePrincipal ───────────────────────────────────────────────────────────

func TestOrgSettingsWriter_RemovePrincipal_DropsOnlyTarget(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{
			{Email: "alice@example.com", Username: "auth0|alice", InvitedAs: "writer", InviteStatus: model.InviteStatusAccepted},
		},
		Auditors: []model.B2BOrgUser{
			{Email: "carol@example.com", InvitedAs: "auditor", InviteStatus: model.InviteStatusPending},
		},
	}, 1)
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	result, err := writer.RemovePrincipal(context.Background(), svc.B2BOrgSettingsRemovePrincipal{
		OrgUID: testOrgUID, Email: "carol@example.com",
	})
	require.NoError(t, err)
	_, gone := findUser(result.Auditors, "carol@example.com")
	assert.False(t, gone, "carol must be removed")
	alice, ok := findUser(result.Writers, "alice@example.com")
	require.True(t, ok, "alice must remain")
	assert.Equal(t, "auth0|alice", alice.Username)
}

func TestOrgSettingsWriter_RemovePrincipal_LastAdminBlocked(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{
			{Email: "alice@example.com", Username: "auth0|alice", InvitedAs: "writer", InviteStatus: model.InviteStatusAccepted},
		},
	}, 1)
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	_, err := writer.RemovePrincipal(context.Background(), svc.B2BOrgSettingsRemovePrincipal{
		OrgUID: testOrgUID, Email: "alice@example.com",
	})
	require.Error(t, err)
	assert.True(t, isConflict(err), "removing the only Admin must be a Conflict, got %T", err)
}

func TestOrgSettingsWriter_RemovePrincipal_NotFound(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	seedTwoAdmins(store)
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	_, err := writer.RemovePrincipal(context.Background(), svc.B2BOrgSettingsRemovePrincipal{
		OrgUID: testOrgUID, Email: "ghost@example.com",
	})
	require.Error(t, err)
}

func TestOrgSettingsWriter_RemovePrincipal_PutConflictPropagates(t *testing.T) {
	store := mock.NewMockB2BOrgSettings()
	seedTwoAdmins(store)
	store.SetPutError(assert.AnError)
	writer := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())

	_, err := writer.RemovePrincipal(context.Background(), svc.B2BOrgSettingsRemovePrincipal{
		OrgUID: testOrgUID, Email: "bob@example.com",
	})
	require.Error(t, err)
}
