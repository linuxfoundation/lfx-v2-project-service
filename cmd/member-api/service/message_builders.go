// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package service — message_builders.go contains helpers that build indexer IndexingConfig
// and FGA GenericFGAMessage payloads for each domain object type.
//
// Publish-failure policy (must be enforced by all callers):
//   - Creates and updates: swallow publish errors, log at warn with
//     publish_failed_for_backfill_repair=true so the /admin/reindex endpoint
//     can recover the record later.
//   - Deletes: propagate publish errors to the caller; a delete without FGA/index
//     cleanup leaves dangling permissions.
package service

import (
	"strings"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// boolPtr returns a pointer to the given bool value.
// Used to set optional *bool fields such as IndexingConfig.Public.
func boolPtr(b bool) *bool { return &b }

// buildB2BOrgIndexingConfig constructs an IndexingConfig for a B2BOrg document.
// name_and_aliases carries the org name followed by primary domain and all
// domain aliases so the indexer can surface orgs by any domain variant.
// fulltext includes name, domain, description, industry and sector for
// keyword search.
func buildB2BOrgIndexingConfig(org *model.B2BOrg) *indexerTypes.IndexingConfig {
	var nameAndAliases []string
	if org.Name != "" {
		nameAndAliases = append(nameAndAliases, org.Name)
	}
	if org.PrimaryDomain != "" {
		nameAndAliases = append(nameAndAliases, org.PrimaryDomain)
	}
	nameAndAliases = append(nameAndAliases, org.DomainAliases...)

	var fulltext []string
	for _, s := range []string{org.Name, org.PrimaryDomain, org.Description, org.Industry, org.Sector} {
		if s != "" {
			fulltext = append(fulltext, s)
		}
	}

	var parentRefs []string
	if org.ParentUID != "" {
		parentRefs = append(parentRefs, "b2b_org:"+org.ParentUID)
	}

	return &indexerTypes.IndexingConfig{
		Public:               boolPtr(false),
		ObjectID:             org.UID,
		AccessCheckObject:    "b2b_org:" + org.UID,
		AccessCheckRelation:  fgaconstants.RelationAuditor,
		HistoryCheckObject:   "b2b_org:" + org.UID,
		HistoryCheckRelation: fgaconstants.RelationAuditor,
		SortName:             strings.ToLower(org.Name),
		NameAndAliases:       nameAndAliases,
		ParentRefs:           parentRefs,
		Fulltext:             strings.Join(fulltext, " "),
		Tags:                 org.Tags(),
	}
}

// buildB2BOrgFGAMessage constructs a GenericFGAMessage for a B2BOrg access-control
// update. On create, globalOrgAdminTeamUID must be non-empty so that the global org
// admin team receives references on the new record.
func buildB2BOrgFGAMessage(org *model.B2BOrg, globalOrgAdminTeamUID string) fgatypes.GenericFGAMessage {
	refs := make(map[string][]string)
	if globalOrgAdminTeamUID != "" {
		refs["global_org_admin"] = []string{"team:" + globalOrgAdminTeamUID}
	}

	return fgatypes.GenericFGAMessage{
		ObjectType: "b2b_org",
		Operation:  "update_access",
		Data: fgatypes.GenericAccessData{
			UID:              org.UID,
			References:       refs,
			ExcludeRelations: []string{"parent", "child"}, // parent and child tuples are managed by buildB2BOrgReparentingMessages
		},
	}
}

// buildProjectMembershipIndexingConfig constructs an IndexingConfig for a
// ProjectMembership document.
func buildProjectMembershipIndexingConfig(pm *model.ProjectMembership) *indexerTypes.IndexingConfig {
	var parentRefs []string
	if pm.B2BOrgUID != "" {
		parentRefs = append(parentRefs, "b2b_org:"+pm.B2BOrgUID)
	}
	if pm.ProjectUID != "" {
		parentRefs = append(parentRefs, "project:"+pm.ProjectUID)
	}

	nameAndAliases := []string{pm.CompanyName}
	if pm.CompanyDomain != "" {
		nameAndAliases = append(nameAndAliases, pm.CompanyDomain)
	}

	var fulltext []string
	for _, s := range []string{pm.CompanyName, pm.TierName, pm.Status, pm.Year} {
		if s != "" {
			fulltext = append(fulltext, s)
		}
	}

	return &indexerTypes.IndexingConfig{
		Public:               boolPtr(false),
		ObjectID:             pm.UID,
		AccessCheckObject:    "project_membership:" + pm.UID,
		AccessCheckRelation:  fgaconstants.RelationAuditor,
		HistoryCheckObject:   "project_membership:" + pm.UID,
		HistoryCheckRelation: fgaconstants.RelationAuditor,
		SortName:             strings.ToLower(pm.CompanyName),
		NameAndAliases:       nameAndAliases,
		ParentRefs:           parentRefs,
		Fulltext:             strings.Join(fulltext, " "),
		Tags:                 pm.Tags(),
	}
}

// buildKeyContactIndexingConfig constructs an IndexingConfig for a KeyContact
// document. Access is checked against the parent project_membership with the
// key_contact relation (v13 model: project_membership.key_contact: [user]).
func buildKeyContactIndexingConfig(kc *model.KeyContact) *indexerTypes.IndexingConfig {
	var parentRefs []string
	if kc.B2BOrgUID != "" {
		parentRefs = append(parentRefs, "b2b_org:"+kc.B2BOrgUID)
	}
	if kc.ProjectUID != "" {
		parentRefs = append(parentRefs, "project:"+kc.ProjectUID)
	}
	if kc.MembershipUID != "" {
		parentRefs = append(parentRefs, "project_membership:"+kc.MembershipUID)
	}

	nameAndAliases := []string{kc.Name()}
	if kc.Email != "" {
		nameAndAliases = append(nameAndAliases, kc.Email)
	}

	var fulltext []string
	for _, s := range []string{kc.FirstName, kc.LastName, kc.Email, kc.Role, kc.CompanyName} {
		if s != "" {
			fulltext = append(fulltext, s)
		}
	}

	emails := kc.Emails
	if len(emails) == 0 && kc.Email != "" {
		emails = []string{kc.Email}
	}
	contact := indexerTypes.ContactBody{
		LfxPrincipal: kc.UID,
		Name:         kc.Name(),
		Emails:       emails,
	}

	return &indexerTypes.IndexingConfig{
		Public:               boolPtr(false),
		ObjectID:             kc.UID,
		AccessCheckObject:    "project_membership:" + kc.MembershipUID,
		AccessCheckRelation:  "key_contact",
		HistoryCheckObject:   "project_membership:" + kc.MembershipUID,
		HistoryCheckRelation: fgaconstants.RelationAuditor,
		SortName:             strings.ToLower(kc.LastName + " " + kc.FirstName),
		NameAndAliases:       nameAndAliases,
		ParentRefs:           parentRefs,
		Fulltext:             strings.Join(fulltext, " "),
		Tags:                 kc.Tags(),
		Contacts:             []indexerTypes.ContactBody{contact},
	}
}

// buildKeyContactFGAPutMessage constructs a GenericFGAMessage that grants the
// given user (sub) the key_contact relation on the parent project_membership.
// Published to GenericMemberPutSubject (lfx.fga-sync.member_put).
func buildKeyContactFGAPutMessage(membershipUID, sub string) fgatypes.GenericFGAMessage {
	return fgatypes.GenericFGAMessage{
		ObjectType: "project_membership",
		Operation:  "member_put",
		Data: fgatypes.GenericMemberData{
			UID:       membershipUID,
			Username:  sub,
			Relations: []string{"key_contact"},
		},
	}
}

// buildKeyContactFGARemoveMessage constructs a GenericFGAMessage that revokes
// the key_contact relation for the given user (sub) on the parent membership.
// Published to GenericMemberRemoveSubject (lfx.fga-sync.member_remove).
func buildKeyContactFGARemoveMessage(membershipUID, sub string) fgatypes.GenericFGAMessage {
	return fgatypes.GenericFGAMessage{
		ObjectType: "project_membership",
		Operation:  "member_remove",
		Data: fgatypes.GenericMemberData{
			UID:       membershipUID,
			Username:  sub,
			Relations: []string{"key_contact"},
		},
	}
}

// b2bOrgNonParentRelations lists relations excluded when updating only an org's
// own parent reference. Prevents the update from wiping global_org_admin,
// auditor, writer, owner, membership, or child tuples set by other code paths.
var b2bOrgNonParentRelations = []string{
	"global_org_admin", "auditor", "writer", "owner", "membership", "child",
}

// b2bOrgNonChildRelations lists relations excluded when updating only a parent
// org's child list. Mirrors b2bOrgNonParentRelations but protects the parent
// relation instead of child.
var b2bOrgNonChildRelations = []string{
	"global_org_admin", "auditor", "writer", "owner", "membership", "parent",
}

// buildB2BOrgReparentingMessages returns FGA update_access messages when a
// b2b_org's ParentUID changes. Pass nil for current on create.
//
// Message 1 — org's own parent ref:
//
//	References["parent"] carries the new parent UID (empty = delete tuple).
//
// Messages 2–3 — child-list updates on affected parents:
//
//	If oldParentChildren is non-nil, OldP receives an update_access with its new
//	child list (A removed). If newParentChildren is non-nil, NewP receives an
//	update_access with its new child list (A included). Callers must ensure A
//	has been added to / removed from the slices before calling.
//
// All messages use ExcludeRelations so unrelated tuples on the target org are
// never accidentally wiped.
func buildB2BOrgReparentingMessages(current, updated *model.B2BOrg, oldParentChildren, newParentChildren []string) []fgatypes.GenericFGAMessage {
	oldParent := ""
	if current != nil {
		oldParent = current.ParentUID
	}
	newParent := updated.ParentUID

	if oldParent == newParent {
		return nil
	}

	msgs := make([]fgatypes.GenericFGAMessage, 0, 3)

	// Message 1: update the org's own parent reference.
	parentRefs := map[string][]string{}
	if newParent != "" {
		parentRefs["parent"] = []string{"b2b_org:" + newParent}
	}
	msgs = append(msgs, fgatypes.GenericFGAMessage{
		ObjectType: "b2b_org",
		Operation:  "update_access",
		Data: fgatypes.GenericAccessData{
			UID:              updated.UID,
			References:       parentRefs,
			ExcludeRelations: b2bOrgNonParentRelations,
		},
	})

	// Message 2: update OldP's child list (A has left).
	if oldParent != "" && oldParentChildren != nil {
		msgs = append(msgs, buildChildListMessage(oldParent, oldParentChildren))
	}

	// Message 3: update NewP's child list (A has joined).
	if newParent != "" && newParentChildren != nil {
		msgs = append(msgs, buildChildListMessage(newParent, newParentChildren))
	}

	return msgs
}

// buildChildListMessage constructs an update_access FGA message that replaces
// a parent org's entire child list. An empty (non-nil) children slice emits
// an update with no child refs, effectively clearing the relation.
func buildChildListMessage(parentUID string, children []string) fgatypes.GenericFGAMessage {
	childRefs := map[string][]string{}
	if len(children) > 0 {
		refs := make([]string, len(children))
		for i, uid := range children {
			refs[i] = "b2b_org:" + uid
		}
		childRefs["child"] = refs
	}
	return fgatypes.GenericFGAMessage{
		ObjectType: "b2b_org",
		Operation:  "update_access",
		Data: fgatypes.GenericAccessData{
			UID:              parentUID,
			References:       childRefs,
			ExcludeRelations: b2bOrgNonChildRelations,
		},
	}
}
