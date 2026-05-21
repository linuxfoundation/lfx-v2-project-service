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

// TestBuildB2BOrgIndexingConfig locks down the IndexingConfig shape for B2BOrg.
// Changing the output breaks the contract with lfx-v2-indexer-service consumers.
func TestBuildB2BOrgIndexingConfig(t *testing.T) {
	cfg := buildB2BOrgIndexingConfig(testB2BOrg)

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
		"NameAndAliases must be: name, primary_domain, then domain aliases in order",
	)
	assert.Contains(t, cfg.Fulltext, "Linux Foundation", "name must appear in fulltext")
	assert.Contains(t, cfg.Fulltext, "linuxfoundation.org", "primary_domain must appear in fulltext")
	assert.Contains(t, cfg.Fulltext, "Supporting open source ecosystems.")
	assert.Contains(t, cfg.Fulltext, "Technology")
	assert.Contains(t, cfg.Fulltext, "Non-Profit")
	assert.Equal(t, testB2BOrg.Tags(), cfg.Tags)
}

// TestBuildB2BOrgIndexingConfig_EmptyOptionals verifies that nil/empty optional
// fields on the org do not produce empty entries in NameAndAliases or Fulltext.
func TestBuildB2BOrgIndexingConfig_EmptyOptionals(t *testing.T) {
	sparse := &model.B2BOrg{UID: "uid-sparse", Name: "Sparse Org"}
	cfg := buildB2BOrgIndexingConfig(sparse)

	assert.Equal(t, []string{"Sparse Org"}, cfg.NameAndAliases,
		"no primary_domain or aliases means only org name in NameAndAliases")
	assert.Equal(t, "Sparse Org", cfg.Fulltext,
		"fulltext must contain at least the name even with no domain/description")
	assert.Empty(t, cfg.ParentRefs, "no parent means no parent_refs")
}

// TestBuildB2BOrgIndexingConfig_WithParent verifies that when B2BOrg.ParentUID
// is set, buildB2BOrgIndexingConfig emits a parent_refs entry so the query
// service can fetch all child orgs by filtering on parent_refs.
func TestBuildB2BOrgIndexingConfig_WithParent(t *testing.T) {
	org := &model.B2BOrg{
		UID:       "child-org-uid",
		Name:      "Child Org",
		ParentUID: "parent-org-uid",
	}

	cfg := buildB2BOrgIndexingConfig(org)

	require.NotNil(t, cfg)
	assert.Equal(t,
		[]string{"b2b_org:parent-org-uid"},
		cfg.ParentRefs,
		"parent_refs must carry b2b_org:<parent_uid> so query service can filter children",
	)
}

// TestBuildB2BOrgFGAMessage_WithGlobalAdmin locks down the FGA message shape
// when a globalOrgAdminTeamUID is provided (create path).
func TestBuildB2BOrgFGAMessage_WithGlobalAdmin(t *testing.T) {
	msg := buildB2BOrgFGAMessage(testB2BOrg, "global-admin-team-uid")

	assert.Equal(t, "b2b_org", msg.ObjectType)
	assert.Equal(t, "update_access", msg.Operation)

	data, ok := msg.Data.(fgatypes.GenericAccessData)
	require.True(t, ok, "Data must be GenericAccessData")
	assert.Equal(t, "b2b-org-uid-001", data.UID)
	require.Contains(t, data.References, "global_org_admin",
		"create must set global_org_admin reference")
	assert.Equal(t,
		[]string{"team:global-admin-team-uid"},
		data.References["global_org_admin"],
	)
}

// TestBuildB2BOrgFGAMessage_NoGlobalAdmin verifies that when no team UID is
// provided (update path), References is empty so no unintended grants are made.
func TestBuildB2BOrgFGAMessage_NoGlobalAdmin(t *testing.T) {
	msg := buildB2BOrgFGAMessage(testB2BOrg, "")

	data, ok := msg.Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Empty(t, data.References,
		"update path must not set global_org_admin when team UID is empty")
}

// TestBuildProjectMembershipIndexingConfig locks down the IndexingConfig shape
// for ProjectMembership. This golden exists before handler wiring so that PR B
// can reference it as the wire-format contract.
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
	cfg := buildProjectMembershipIndexingConfig(pm)

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
		"parent_refs must carry both b2b_org and project references",
	)
	assert.Contains(t, cfg.Fulltext, "Acme Corp")
	assert.Contains(t, cfg.Fulltext, "Gold Corporate Membership")
	assert.Contains(t, cfg.Fulltext, "Active")
	assert.Contains(t, cfg.Fulltext, "2025")
	assert.Equal(t, pm.Tags(), cfg.Tags)
}

// TestBuildKeyContactIndexingConfig locks down the IndexingConfig shape for
// KeyContact, including the Contacts body and parent_refs. This golden exists
// before PR C wires the handler so the wire-format is proven before the handler lands.
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
	cfg := buildKeyContactIndexingConfig(kc)

	require.NotNil(t, cfg)
	require.NotNil(t, cfg.Public)
	assert.False(t, *cfg.Public)
	assert.Equal(t, "kc-uid-001", cfg.ObjectID)
	assert.Equal(t, "project_membership:pm-uid-001", cfg.AccessCheckObject)
	assert.Equal(t, "key_contact", cfg.AccessCheckRelation)
	assert.Equal(t, "project_membership:pm-uid-001", cfg.HistoryCheckObject)
	assert.Equal(t, fgaconstants.RelationAuditor, cfg.HistoryCheckRelation)
	assert.Equal(t, "lovelace ada", cfg.SortName, "sort_name must be last_name+first_name")
	assert.Equal(t, []string{"Ada Lovelace", "ada@example.com"}, cfg.NameAndAliases)
	assert.Equal(t,
		[]string{
			"b2b_org:b2b-org-uid-001",
			"project:project-uid-001",
			"project_membership:pm-uid-001",
		},
		cfg.ParentRefs,
		"parent_refs must carry b2b_org, project, and project_membership",
	)
	assert.Contains(t, cfg.Fulltext, "Ada")
	assert.Contains(t, cfg.Fulltext, "Lovelace")
	assert.Contains(t, cfg.Fulltext, "ada@example.com")
	assert.Contains(t, cfg.Fulltext, "Voting Representative")
	assert.Contains(t, cfg.Fulltext, "Acme Corp")
	assert.Equal(t, kc.Tags(), cfg.Tags)
	require.Len(t, cfg.Contacts, 1, "must have exactly one ContactBody")
	assert.Equal(t, "kc-uid-001", cfg.Contacts[0].LfxPrincipal)
	assert.Equal(t, kc.Name(), cfg.Contacts[0].Name)
	assert.Equal(t, []string{"ada@example.com", "alovelace@example.com"}, cfg.Contacts[0].Emails)
}

// TestBuildKeyContactFGAPutMessage locks down the FGA member_put shape for KeyContact.
// The generic fga-sync handler uses project_membership + key_contact relation (v13 model).
func TestBuildKeyContactFGAPutMessage(t *testing.T) {
	msg := buildKeyContactFGAPutMessage("pm-uid-001", "user-sub-abc")

	assert.Equal(t, "project_membership", msg.ObjectType)
	assert.Equal(t, "member_put", msg.Operation)
	data, ok := msg.Data.(fgatypes.GenericMemberData)
	require.True(t, ok, "Data must be GenericMemberData")
	assert.Equal(t, "pm-uid-001", data.UID)
	assert.Equal(t, "user-sub-abc", data.Username)
	assert.Equal(t, []string{"key_contact"}, data.Relations)
}

// TestBuildKeyContactFGARemoveMessage locks down the FGA member_remove shape for KeyContact.
func TestBuildKeyContactFGARemoveMessage(t *testing.T) {
	msg := buildKeyContactFGARemoveMessage("pm-uid-001", "user-sub-abc")

	assert.Equal(t, "project_membership", msg.ObjectType)
	assert.Equal(t, "member_remove", msg.Operation)
	data, ok := msg.Data.(fgatypes.GenericMemberData)
	require.True(t, ok, "Data must be GenericMemberData")
	assert.Equal(t, "pm-uid-001", data.UID)
	assert.Equal(t, "user-sub-abc", data.Username)
	assert.Equal(t, []string{"key_contact"}, data.Relations)
}

// TestBuildB2BOrgReparentingMessages_SetParent verifies that setting a new parent
// with child lists emits 2 messages: one for the org's parent ref and one for
// NewP's updated child list. OldP message is skipped when oldParentChildren is nil
// (create path — no OldP).
func TestBuildB2BOrgReparentingMessages_SetParent(t *testing.T) {
	updated := &model.B2BOrg{UID: "child-uid", ParentUID: "parent-uid"}
	newChildren := []string{"sibling-uid", "child-uid"}
	msgs := buildB2BOrgReparentingMessages(nil, updated, nil, newChildren)

	require.Len(t, msgs, 2, "create-with-parent: org parent msg + NewP child-list msg")

	// msg[0]: org's own parent reference.
	assert.Equal(t, "b2b_org", msgs[0].ObjectType)
	assert.Equal(t, "update_access", msgs[0].Operation)
	data0, ok := msgs[0].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "child-uid", data0.UID)
	assert.Equal(t, []string{"b2b_org:parent-uid"}, data0.References["parent"])
	assert.Equal(t, b2bOrgNonParentRelations, data0.ExcludeRelations,
		"non-parent relations must be excluded to avoid wiping global_org_admin etc.")

	// msg[1]: NewP's child list.
	data1, ok := msgs[1].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "parent-uid", data1.UID)
	assert.Equal(t, []string{"b2b_org:sibling-uid", "b2b_org:child-uid"}, data1.References["child"])
	assert.Equal(t, b2bOrgNonChildRelations, data1.ExcludeRelations)
}

// TestBuildB2BOrgReparentingMessages_Reparent verifies that moving an org from
// OldP to NewP emits 3 messages: the org's parent ref, OldP's pruned child list,
// and NewP's extended child list.
func TestBuildB2BOrgReparentingMessages_Reparent(t *testing.T) {
	current := &model.B2BOrg{UID: "child-uid", ParentUID: "old-parent-uid"}
	updated := &model.B2BOrg{UID: "child-uid", ParentUID: "new-parent-uid"}
	oldChildren := []string{"sibling-uid"} // child-uid already removed by caller
	newChildren := []string{"other-uid", "child-uid"}

	msgs := buildB2BOrgReparentingMessages(current, updated, oldChildren, newChildren)

	require.Len(t, msgs, 3, "reparent: org msg + OldP child-list msg + NewP child-list msg")

	// msg[0]: org's new parent ref.
	data0, ok := msgs[0].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "child-uid", data0.UID)
	assert.Equal(t, []string{"b2b_org:new-parent-uid"}, data0.References["parent"])

	// msg[1]: OldP's remaining children.
	data1, ok := msgs[1].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "old-parent-uid", data1.UID)
	assert.Equal(t, []string{"b2b_org:sibling-uid"}, data1.References["child"])
	assert.Equal(t, b2bOrgNonChildRelations, data1.ExcludeRelations)

	// msg[2]: NewP's full child list including the moved org.
	data2, ok := msgs[2].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "new-parent-uid", data2.UID)
	assert.Equal(t, []string{"b2b_org:other-uid", "b2b_org:child-uid"}, data2.References["child"])
	assert.Equal(t, b2bOrgNonChildRelations, data2.ExcludeRelations)
}

// TestBuildB2BOrgReparentingMessages_ClearParent verifies that clearing the parent
// emits an update_access with an empty parent ref (which removes the existing tuple).
// Child-list messages are skipped when child-slice args are nil.
func TestBuildB2BOrgReparentingMessages_ClearParent(t *testing.T) {
	current := &model.B2BOrg{UID: "child-uid", ParentUID: "old-parent-uid"}
	updated := &model.B2BOrg{UID: "child-uid", ParentUID: ""}
	msgs := buildB2BOrgReparentingMessages(current, updated, nil, nil)

	require.Len(t, msgs, 1, "clear-parent with nil child lists: only the org's parent-ref message")
	data, ok := msgs[0].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Empty(t, data.References, "clearing parent sends empty refs to delete the tuple")
}

// TestBuildB2BOrgReparentingMessages_ClearParent_WithOldChildren verifies that
// when oldParentChildren is provided on a parent clear, OldP also gets a child-list
// update (A removed from OldP's list).
func TestBuildB2BOrgReparentingMessages_ClearParent_WithOldChildren(t *testing.T) {
	current := &model.B2BOrg{UID: "child-uid", ParentUID: "old-parent-uid"}
	updated := &model.B2BOrg{UID: "child-uid", ParentUID: ""}
	oldChildren := []string{"sibling-uid"} // child-uid already removed by caller

	msgs := buildB2BOrgReparentingMessages(current, updated, oldChildren, nil)

	require.Len(t, msgs, 2, "clear-parent with oldChildren: org msg + OldP child-list msg")
	data1, ok := msgs[1].Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Equal(t, "old-parent-uid", data1.UID)
	assert.Equal(t, []string{"b2b_org:sibling-uid"}, data1.References["child"])
}

// TestBuildB2BOrgReparentingMessages_NoChange verifies that when the parent is
// unchanged (or both nil), no messages are emitted.
func TestBuildB2BOrgReparentingMessages_NoChange(t *testing.T) {
	current := &model.B2BOrg{UID: "child-uid", ParentUID: "same-parent"}
	updated := &model.B2BOrg{UID: "child-uid", ParentUID: "same-parent"}
	msgs := buildB2BOrgReparentingMessages(current, updated, nil, nil)

	assert.Nil(t, msgs, "no change in parent must produce no messages")
}

// TestBuildB2BOrgFGAMessage_ExcludesParentAndChild verifies that the main b2b_org
// FGA message excludes both parent and child relations so reparenting messages
// from buildB2BOrgReparentingMessages are never overwritten.
func TestBuildB2BOrgFGAMessage_ExcludesParentAndChild(t *testing.T) {
	msg := buildB2BOrgFGAMessage(testB2BOrg, "")

	data, ok := msg.Data.(fgatypes.GenericAccessData)
	require.True(t, ok)
	assert.Contains(t, data.ExcludeRelations, "parent",
		"parent must be excluded so the main message does not wipe parent tuples")
	assert.Contains(t, data.ExcludeRelations, "child",
		"child must be excluded so the main message does not wipe child-list tuples")
}
