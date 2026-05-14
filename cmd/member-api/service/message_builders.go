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

	return &indexerTypes.IndexingConfig{
		Public:               boolPtr(false),
		ObjectID:             org.UID,
		AccessCheckObject:    "b2b_org:" + org.UID,
		AccessCheckRelation:  fgaconstants.RelationAuditor,
		HistoryCheckObject:   "b2b_org:" + org.UID,
		HistoryCheckRelation: fgaconstants.RelationAuditor,
		SortName:             strings.ToLower(org.Name),
		NameAndAliases:       nameAndAliases,
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
			UID:        org.UID,
			References: refs,
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
// document. The contacts body is populated from the key contact record.
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
		AccessCheckObject:    "key_contact:" + kc.UID,
		AccessCheckRelation:  fgaconstants.RelationAuditor,
		HistoryCheckObject:   "key_contact:" + kc.UID,
		HistoryCheckRelation: fgaconstants.RelationAuditor,
		SortName:             strings.ToLower(kc.LastName + " " + kc.FirstName),
		NameAndAliases:       nameAndAliases,
		ParentRefs:           parentRefs,
		Fulltext:             strings.Join(fulltext, " "),
		Tags:                 kc.Tags(),
		Contacts:             []indexerTypes.ContactBody{contact},
	}
}

// buildKeyContactFGAMessage constructs a GenericFGAMessage for a KeyContact
// access-control update (references only — no direct relations on key contacts).
func buildKeyContactFGAMessage(kc *model.KeyContact) fgatypes.GenericFGAMessage {
	return fgatypes.GenericFGAMessage{
		ObjectType: "key_contact",
		Operation:  "update_access",
		Data: fgatypes.GenericAccessData{
			UID: kc.UID,
		},
	}
}
