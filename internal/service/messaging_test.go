// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"testing"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testB2BOrg is the canonical B2BOrg fixture used in events golden tests.
var testB2BOrg = &model.B2BOrg{
	UID:           "b2b-org-uid-001",
	SFID:          "001000000000001AAA",
	Name:          "Linux Foundation",
	PrimaryDomain: "linuxfoundation.org",
	Description:   "Supporting open source ecosystems.",
	Industry:      "Technology",
	Sector:        "Non-Profit",
	DomainAliases: []string{"lf.org", "thelinuxfoundation.org"},
}

func TestBuildB2BOrgIndexingConfig(t *testing.T) {
	cfg := BuildB2BOrgIndexingConfig(testB2BOrg)

	require.NotNil(t, cfg)
	require.NotNil(t, cfg.Public, "Public must be set explicitly — nil defaults indexer to public")
	assert.False(t, *cfg.Public, "b2b_org must not be publicly accessible")
	assert.Equal(t, "b2b-org-uid-001", cfg.ObjectID)
	assert.Equal(t, "b2b_org:b2b-org-uid-001", cfg.AccessCheckObject)
	assert.Equal(t, fgaconstants.RelationAuditor, cfg.AccessCheckRelation)
	assert.Equal(t, "b2b_org:b2b-org-uid-001", cfg.HistoryCheckObject)
	assert.Equal(t, fgaconstants.RelationAuditor, cfg.HistoryCheckRelation)
	assert.Equal(t, "linux foundation", cfg.SortName)
	assert.Equal(t,
		[]string{"Linux Foundation", "linuxfoundation.org", "lf.org", "thelinuxfoundation.org"},
		cfg.NameAndAliases,
	)
	assert.Contains(t, cfg.Fulltext, "Linux Foundation")
	assert.Contains(t, cfg.Fulltext, "linuxfoundation.org")
	assert.Contains(t, cfg.Fulltext, "Supporting open source ecosystems.")
	assert.Contains(t, cfg.Fulltext, "Technology")
	assert.Contains(t, cfg.Fulltext, "Non-Profit")
	assert.Equal(t, testB2BOrg.Tags(), cfg.Tags)
}

func TestBuildB2BOrgIndexingConfig_EmptyOptionals(t *testing.T) {
	sparse := &model.B2BOrg{UID: "uid-sparse", Name: "Sparse Org"}
	cfg := BuildB2BOrgIndexingConfig(sparse)

	assert.Equal(t, []string{"Sparse Org"}, cfg.NameAndAliases)
	assert.Equal(t, "Sparse Org", cfg.Fulltext)
	assert.Empty(t, cfg.ParentRefs)
}

func TestBuildB2BOrgIndexingConfig_WithParent(t *testing.T) {
	org := &model.B2BOrg{UID: "child-org-uid", Name: "Child Org", ParentUID: "parent-org-uid"}
	cfg := BuildB2BOrgIndexingConfig(org)

	require.NotNil(t, cfg)
	assert.Equal(t, []string{"b2b_org:parent-org-uid"}, cfg.ParentRefs)
}

func TestBuildProjectMembershipIndexingConfig(t *testing.T) {
	pm := &model.ProjectMembership{
		UID:           "pm-uid-001",
		B2BOrgUID:     "b2b-org-uid-001",
		ProjectUID:    "project-uid-001",
		CompanyName:   "Acme Corp",
		CompanyDomain: "acme.com",
		TierName:      "Gold Corporate Membership",
		Status:        "Active",
		Year:          "2025",
	}
	cfg := BuildProjectMembershipIndexingConfig(pm)

	require.NotNil(t, cfg)
	require.NotNil(t, cfg.Public)
	assert.False(t, *cfg.Public)
	assert.Equal(t, "pm-uid-001", cfg.ObjectID)
	assert.Equal(t, "project_membership:pm-uid-001", cfg.AccessCheckObject)
	assert.Equal(t, fgaconstants.RelationAuditor, cfg.AccessCheckRelation)
	assert.Equal(t, "acme corp", cfg.SortName)
	assert.Equal(t, []string{"Acme Corp", "acme.com"}, cfg.NameAndAliases)
	assert.Equal(t,
		[]string{"b2b_org:b2b-org-uid-001", "project:project-uid-001"},
		cfg.ParentRefs,
	)
	assert.Contains(t, cfg.Fulltext, "Acme Corp")
	assert.Contains(t, cfg.Fulltext, "Gold Corporate Membership")
	assert.Contains(t, cfg.Fulltext, "Active")
	assert.Contains(t, cfg.Fulltext, "2025")
	assert.Equal(t, pm.Tags(), cfg.Tags)
}

func TestBuildKeyContactIndexingConfig(t *testing.T) {
	kc := &model.KeyContact{
		UID:           "kc-uid-001",
		B2BOrgUID:     "b2b-org-uid-001",
		ProjectUID:    "project-uid-001",
		MembershipUID: "pm-uid-001",
		FirstName:     "Ada",
		LastName:      "Lovelace",
		Email:         "ada@example.com",
		Emails:        []string{"ada@example.com", "alovelace@example.com"},
		Role:          "Voting Representative",
		CompanyName:   "Acme Corp",
	}
	cfg := BuildKeyContactIndexingConfig(kc)

	require.NotNil(t, cfg)
	require.NotNil(t, cfg.Public)
	assert.False(t, *cfg.Public)
	assert.Equal(t, "kc-uid-001", cfg.ObjectID)
	assert.Equal(t, "project_membership:pm-uid-001", cfg.AccessCheckObject)
	assert.Equal(t, "key_contact", cfg.AccessCheckRelation)
	assert.Equal(t, "project_membership:pm-uid-001", cfg.HistoryCheckObject)
	assert.Equal(t, fgaconstants.RelationAuditor, cfg.HistoryCheckRelation)
	assert.Equal(t, "lovelace ada", cfg.SortName)
	assert.Equal(t, []string{"Ada Lovelace", "ada@example.com"}, cfg.NameAndAliases)
	assert.Equal(t,
		[]string{"b2b_org:b2b-org-uid-001", "project:project-uid-001", "project_membership:pm-uid-001"},
		cfg.ParentRefs,
	)
	assert.Contains(t, cfg.Fulltext, "Ada")
	assert.Contains(t, cfg.Fulltext, "Lovelace")
	assert.Contains(t, cfg.Fulltext, "ada@example.com")
	assert.Contains(t, cfg.Fulltext, "Voting Representative")
	assert.Contains(t, cfg.Fulltext, "Acme Corp")
	assert.Equal(t, kc.Tags(), cfg.Tags)
	require.Len(t, cfg.Contacts, 1)
	assert.Equal(t, "kc-uid-001", cfg.Contacts[0].LfxPrincipal)
	assert.Equal(t, kc.Name(), cfg.Contacts[0].Name)
	assert.Equal(t, []string{"ada@example.com", "alovelace@example.com"}, cfg.Contacts[0].Emails)
}

func TestBuildKeyContactFGAPutMessage(t *testing.T) {
	msg := BuildKeyContactFGAPutMessage("pm-uid-001", "user-sub-abc")

	assert.Equal(t, "project_membership", msg.ObjectType)
	assert.Equal(t, "member_put", msg.Operation)
	data, ok := msg.Data.(fgatypes.GenericMemberData)
	require.True(t, ok)
	assert.Equal(t, "pm-uid-001", data.UID)
	assert.Equal(t, "user-sub-abc", data.Username)
	assert.Equal(t, []string{"key_contact"}, data.Relations)
}

func TestBuildKeyContactFGARemoveMessage(t *testing.T) {
	msg := BuildKeyContactFGARemoveMessage("pm-uid-001", "user-sub-abc")

	assert.Equal(t, "project_membership", msg.ObjectType)
	assert.Equal(t, "member_remove", msg.Operation)
	data, ok := msg.Data.(fgatypes.GenericMemberData)
	require.True(t, ok)
	assert.Equal(t, "pm-uid-001", data.UID)
	assert.Equal(t, "user-sub-abc", data.Username)
	assert.Equal(t, []string{"key_contact"}, data.Relations)
}

func TestBuildB2BOrgReparentingMessages_SetParent(t *testing.T) {
	updated := &model.B2BOrg{UID: "child-uid", ParentUID: "parent-uid"}
	newChildren := []string{"sibling-uid", "child-uid"}
	msgs := BuildB2BOrgReparentingMessages(nil, updated, nil, newChildren)

	require.Len(t, msgs, 2)

	data0, ok := msgs[0].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "child-uid", data0.UID)
	assert.Equal(t, []string{"b2b_org:parent-uid"}, data0.References["parent"])
	assert.Equal(t, b2bOrgNonParentRelations, data0.ExcludeRelations)

	data1, ok := msgs[1].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "parent-uid", data1.UID)
	assert.Equal(t, []string{"b2b_org:sibling-uid", "b2b_org:child-uid"}, data1.References["child"])
	assert.Equal(t, b2bOrgNonChildRelations, data1.ExcludeRelations)
}

func TestBuildB2BOrgReparentingMessages_Reparent(t *testing.T) {
	current := &model.B2BOrg{UID: "child-uid", ParentUID: "old-parent-uid"}
	updated := &model.B2BOrg{UID: "child-uid", ParentUID: "new-parent-uid"}
	oldChildren := []string{"sibling-uid"}
	newChildren := []string{"other-uid", "child-uid"}

	msgs := BuildB2BOrgReparentingMessages(current, updated, oldChildren, newChildren)

	require.Len(t, msgs, 3)

	data0, ok := msgs[0].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "child-uid", data0.UID)
	assert.Equal(t, []string{"b2b_org:new-parent-uid"}, data0.References["parent"])

	data1, ok := msgs[1].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "old-parent-uid", data1.UID)
	assert.Equal(t, []string{"b2b_org:sibling-uid"}, data1.References["child"])
	assert.Equal(t, b2bOrgNonChildRelations, data1.ExcludeRelations)

	data2, ok := msgs[2].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "new-parent-uid", data2.UID)
	assert.Equal(t, []string{"b2b_org:other-uid", "b2b_org:child-uid"}, data2.References["child"])
	assert.Equal(t, b2bOrgNonChildRelations, data2.ExcludeRelations)
}

func TestBuildB2BOrgReparentingMessages_ClearParent(t *testing.T) {
	current := &model.B2BOrg{UID: "child-uid", ParentUID: "old-parent-uid"}
	updated := &model.B2BOrg{UID: "child-uid", ParentUID: ""}
	msgs := BuildB2BOrgReparentingMessages(current, updated, nil, nil)

	require.Len(t, msgs, 1)
	data, ok := msgs[0].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Empty(t, data.References)
}

func TestBuildB2BOrgReparentingMessages_ClearParent_WithOldChildren(t *testing.T) {
	current := &model.B2BOrg{UID: "child-uid", ParentUID: "old-parent-uid"}
	updated := &model.B2BOrg{UID: "child-uid", ParentUID: ""}
	oldChildren := []string{"sibling-uid"}

	msgs := BuildB2BOrgReparentingMessages(current, updated, oldChildren, nil)

	require.Len(t, msgs, 2)
	data1, ok := msgs[1].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "old-parent-uid", data1.UID)
	assert.Equal(t, []string{"b2b_org:sibling-uid"}, data1.References["child"])
}

func TestBuildB2BOrgReparentingMessages_NoChange(t *testing.T) {
	current := &model.B2BOrg{UID: "child-uid", ParentUID: "same-parent"}
	updated := &model.B2BOrg{UID: "child-uid", ParentUID: "same-parent"}
	msgs := BuildB2BOrgReparentingMessages(current, updated, nil, nil)

	assert.Nil(t, msgs)
}

func TestBuildB2BOrgFGAMessage_ExcludesParentChildAndMembership(t *testing.T) {
	msg := BuildB2BOrgFGAMessage(testB2BOrg, "", nil, nil, nil)

	data, ok := msg.Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Contains(t, data.ExcludeRelations, "parent")
	assert.Contains(t, data.ExcludeRelations, "child")
	assert.Contains(t, data.ExcludeRelations, "membership")
}

func TestBuildB2BOrgFGAMessage_NilWritersAuditorsExcluded(t *testing.T) {
	// nil writers/auditors means "don't touch these relations" (e.g. b2b_org field
	// update that must not wipe settings-driven ACL tuples set by a prior PUT /settings).
	msg := BuildB2BOrgFGAMessage(testB2BOrg, "", nil, nil, nil)

	data, ok := msg.Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Contains(t, data.ExcludeRelations, "writer",
		"nil writers must be excluded so update_access does not wipe existing writer tuples")
	assert.Contains(t, data.ExcludeRelations, "auditor",
		"nil auditors must be excluded so update_access does not wipe existing auditor tuples")
}

func TestBuildB2BOrgFGAMessage_ExplicitWritersAuditorsNotExcluded(t *testing.T) {
	// Non-nil (even empty) slices mean "replace this relation" — must NOT be excluded.
	msg := BuildB2BOrgFGAMessage(testB2BOrg, "", []string{}, []string{}, nil)

	data, ok := msg.Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.NotContains(t, data.ExcludeRelations, "writer",
		"explicit (empty) writers must not be excluded — caller intends to clear all writers")
	assert.NotContains(t, data.ExcludeRelations, "auditor",
		"explicit (empty) auditors must not be excluded — caller intends to clear all auditors")
}

func TestBuildB2BOrgFGAMessage_WithWritersAndAuditors(t *testing.T) {
	writers := []string{"alice", "bob"}
	auditors := []string{"viewer1"}
	msg := BuildB2BOrgFGAMessage(testB2BOrg, "global-admin-uid", writers, auditors, nil)

	data, ok := msg.Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, []string{"alice", "bob"}, data.Relations["writer"])
	assert.Equal(t, []string{"viewer1"}, data.Relations["auditor"])
	assert.Equal(t, []string{"team:global-admin-uid"}, data.References["global_org_admin"])
}

func TestBuildB2BOrgFGAMessage_WithMembershipUIDs(t *testing.T) {
	membershipUIDs := []string{"pm-uid-001", "pm-uid-002"}
	msg := BuildB2BOrgFGAMessage(testB2BOrg, "", nil, nil, membershipUIDs)

	data, ok := msg.Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t,
		[]string{"project_membership:pm-uid-001", "project_membership:pm-uid-002"},
		data.References["membership"],
	)
	assert.NotContains(t, data.ExcludeRelations, "membership")
}

func TestBuildB2BOrgFGAMessage_WithGlobalAdmin(t *testing.T) {
	msg := BuildB2BOrgFGAMessage(testB2BOrg, "global-admin-team-uid", nil, nil, nil)

	assert.Equal(t, "b2b_org", msg.ObjectType)
	assert.Equal(t, "update_access", msg.Operation)

	data, ok := msg.Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "b2b-org-uid-001", data.UID)
	require.Contains(t, data.References, "global_org_admin")
	assert.Equal(t, []string{"team:global-admin-team-uid"}, data.References["global_org_admin"])
}

func TestBuildB2BOrgFGAMessage_NoGlobalAdmin(t *testing.T) {
	msg := BuildB2BOrgFGAMessage(testB2BOrg, "", nil, nil, nil)

	data, ok := msg.Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.NotContains(t, data.References, "global_org_admin")
}

func TestBuildProjectMembershipFGAMessage(t *testing.T) {
	pm := &model.ProjectMembership{
		UID:        "pm-uid-001",
		B2BOrgUID:  "b2b-org-uid-001",
		ProjectUID: "project-uid-001",
	}
	msg := BuildProjectMembershipFGAMessage(pm)

	assert.Equal(t, "project_membership", msg.ObjectType)
	assert.Equal(t, "update_access", msg.Operation)

	data, ok := msg.Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "pm-uid-001", data.UID)
	assert.Equal(t, []string{"b2b_org:b2b-org-uid-001"}, data.References["b2b_org"])
	assert.Equal(t, []string{"project:project-uid-001"}, data.References["project"])
	assert.Contains(t, data.ExcludeRelations, "key_contact")
}

func TestBuildProjectMembershipFGAMessage_MissingParents(t *testing.T) {
	pm := &model.ProjectMembership{UID: "pm-uid-sparse"}
	msg := BuildProjectMembershipFGAMessage(pm)

	data, ok := msg.Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Empty(t, data.References)
	assert.Contains(t, data.ExcludeRelations, "key_contact")
}
