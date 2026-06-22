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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testOrgUID2 = "00000000-0000-0000-0000-000000000002"

// ── Multi-org promotion ───────────────────────────────────────────────────

func TestInviteAcceptedService_MultiOrg_PromotesBothOrgs(t *testing.T) {
	// The same email is pending in two orgs. One acceptance must promote both.
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{{
			Email: "multi@example.com", InviteStatus: model.InviteStatusPending,
		}},
	}, 1)
	store.Seed(testOrgUID2, &model.B2BOrgSettings{
		UID: testOrgUID2,
		Writers: []model.B2BOrgUser{{
			Email: "multi@example.com", InviteStatus: model.InviteStatusPending,
		}},
	}, 1)

	inner := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())
	invSvc := svc.NewInviteAcceptedService(
		svc.WithInviteAcceptedSettingsReader(store),
		svc.WithInviteAcceptedOrgSettingsWriter(inner),
	)

	err := invSvc.Handle(context.Background(), orgAcceptedEvent("multi@example.com", "auth0|multi"))
	require.NoError(t, err)

	for _, uid := range []string{testOrgUID, testOrgUID2} {
		saved, _, _ := store.GetSettings(context.Background(), uid)
		require.Len(t, saved.Writers, 1, "org %s must still have one writer", uid)
		assert.Equal(t, "multi", saved.Writers[0].Username, "org %s: auth0| prefix must be stripped on promotion", uid)
		assert.Equal(t, model.InviteStatusAccepted, saved.Writers[0].InviteStatus, "org %s", uid)
	}
}

// ── Role tie-break ────────────────────────────────────────────────────────

func TestInviteAcceptedService_RoleTieBreak_ManagePromotesWriters(t *testing.T) {
	// Email in both lists; ev.Role=Manage → only writers promoted.
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{{
			Email: "tie@example.com", InviteStatus: model.InviteStatusPending,
		}},
		Auditors: []model.B2BOrgUser{{
			Email: "tie@example.com", InviteStatus: model.InviteStatusPending,
		}},
	}, 1)

	inner := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())
	invSvc := newInviteAcceptedService(store, inner)

	err := invSvc.Handle(context.Background(), orgAcceptedEventWithRole("tie@example.com", "auth0|tie", string(inviteapi.InviteRoleManage)))
	require.NoError(t, err)

	saved, _, _ := store.GetSettings(context.Background(), testOrgUID)
	assert.Equal(t, "tie", saved.Writers[0].Username, "auth0| prefix must be stripped on promotion")
	assert.Equal(t, model.InviteStatusAccepted, saved.Writers[0].InviteStatus)
	assert.Empty(t, saved.Auditors[0].Username, "auditor must NOT be promoted on Manage role")
	assert.Equal(t, model.InviteStatusPending, saved.Auditors[0].InviteStatus)
}

func TestInviteAcceptedService_RoleTieBreak_ViewPromotesAuditors(t *testing.T) {
	// Email in both lists; ev.Role=View → only auditors promoted.
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{{
			Email: "view@example.com", InviteStatus: model.InviteStatusPending,
		}},
		Auditors: []model.B2BOrgUser{{
			Email: "view@example.com", InviteStatus: model.InviteStatusPending,
		}},
	}, 1)

	inner := newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())
	invSvc := newInviteAcceptedService(store, inner)

	err := invSvc.Handle(context.Background(), orgAcceptedEventWithRole("view@example.com", "auth0|view", string(inviteapi.InviteRoleView)))
	require.NoError(t, err)

	saved, _, _ := store.GetSettings(context.Background(), testOrgUID)
	assert.Empty(t, saved.Writers[0].Username, "writer must NOT be promoted on View role")
	assert.Equal(t, model.InviteStatusPending, saved.Writers[0].InviteStatus)
	assert.Equal(t, "view", saved.Auditors[0].Username, "auth0| prefix must be stripped on promotion")
	assert.Equal(t, model.InviteStatusAccepted, saved.Auditors[0].InviteStatus)
}

func TestInviteAcceptedService_RoleTieBreak_UnknownRole_SkipsOrg(t *testing.T) {
	// Email in both lists; ev.Role is unknown → skip, no over-grant.
	store := mock.NewMockB2BOrgSettings()
	store.Seed(testOrgUID, &model.B2BOrgSettings{
		UID: testOrgUID,
		Writers: []model.B2BOrgUser{{
			Email: "ambig@example.com", InviteStatus: model.InviteStatusPending,
		}},
		Auditors: []model.B2BOrgUser{{
			Email: "ambig@example.com", InviteStatus: model.InviteStatusPending,
		}},
	}, 1)

	inner := &countingWriter{inner: newOrgSettingsWriter(store, mock.NewMockB2BOrgReader(), mock.NewMockMemberPublisher())}
	invSvc := newInviteAcceptedService(store, inner)

	err := invSvc.Handle(context.Background(), orgAcceptedEventWithRole("ambig@example.com", "auth0|ambig", ""))
	require.NoError(t, err)
	assert.Equal(t, 0, inner.calls, "Update must not be called when role cannot resolve tie-break")
}
