// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// messaging.go contains pure transforms from domain types to NATS wire format,
// plus thin Publish* wrappers. Functions here take *model.X inputs and produce
// ready-to-publish messages (or invoke port.MemberPublisher). No port reads,
// no orchestration, no state. Keep this file dependency-free of orchestrator
// types so the builders stay safe to call from any layer.

package service

import (
	"context"
	"log/slog"
	"strings"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
)

// boolPtr returns a pointer to the given bool value.
func boolPtr(b bool) *bool { return &b }

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

// BuildB2BOrgIndexingConfig constructs an IndexingConfig for a B2BOrg document.
func BuildB2BOrgIndexingConfig(org *model.B2BOrg) *indexerTypes.IndexingConfig {
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

// BuildB2BOrgSettingsIndexingConfig constructs an IndexingConfig for a B2BOrgSettings document.
// ObjectID equals the parent org UID so a single point-lookup retrieves both org and settings docs
// (callers filter by object_type). Access-check resolves against the parent b2b_org — settings
// do not have a separate FGA type.
// Public is explicitly false — settings docs are never world-readable. Spelled out here so future
// readers don't adopt committee-service's &parent.Public pattern by mistake.
// HistoryCheckRelation is writer (not auditor) — history audits are a write-side concern;
// matches project-service precedent.
func BuildB2BOrgSettingsIndexingConfig(org *model.B2BOrg, settings *model.B2BOrgSettings) *indexerTypes.IndexingConfig {
	var nameAndAliases []string
	if org.Name != "" {
		nameAndAliases = append(nameAndAliases, org.Name)
	}
	if org.PrimaryDomain != "" {
		nameAndAliases = append(nameAndAliases, org.PrimaryDomain)
	}
	nameAndAliases = append(nameAndAliases, org.DomainAliases...)

	parentRefs := []string{"b2b_org:" + org.UID}
	if org.ParentUID != "" {
		parentRefs = append(parentRefs, "b2b_org:"+org.ParentUID)
	}

	return &indexerTypes.IndexingConfig{
		Public:               boolPtr(false),
		ObjectID:             org.UID,
		AccessCheckObject:    "b2b_org:" + org.UID,
		AccessCheckRelation:  fgaconstants.RelationAuditor,
		HistoryCheckObject:   "b2b_org:" + org.UID,
		HistoryCheckRelation: fgaconstants.RelationWriter,
		SortName:             strings.ToLower(org.Name),
		NameAndAliases:       nameAndAliases,
		ParentRefs:           parentRefs,
		Fulltext:             strings.Join(settings.FulltextTokens(), " "),
		Tags:                 settings.Tags(),
	}
}

// BuildB2BOrgFGAMessage constructs a GenericFGAMessage for a B2BOrg access-control
// update.
//
//   - globalOrgAdminTeamUID: set on create to grant the LF global-admin team; empty on updates.
//   - writers, auditors: LFID usernames of accepted principals from OrgSettings.
//   - membershipUIDs: UIDs of project_memberships owned by this org. When non-empty,
//     References["membership"] is populated. When empty or nil, "membership" is added
//     to ExcludeRelations so existing membership tuples are not accidentally wiped.
//
// parent and child tuples are always excluded — managed by BuildB2BOrgReparentingMessages.
func BuildB2BOrgFGAMessage(org *model.B2BOrg, globalOrgAdminTeamUID string, writers, auditors, membershipUIDs []string) fgatypes.GenericFGAMessage {
	refs := make(map[string][]string)
	if globalOrgAdminTeamUID != "" {
		refs["global_org_admin"] = []string{"team:" + globalOrgAdminTeamUID}
	}
	if len(membershipUIDs) > 0 {
		mRefs := make([]string, len(membershipUIDs))
		for i, uid := range membershipUIDs {
			mRefs[i] = "project_membership:" + uid
		}
		refs["membership"] = mRefs
	}

	relations := make(map[string][]string)
	if len(writers) > 0 {
		relations["writer"] = writers
	}
	if len(auditors) > 0 {
		relations["auditor"] = auditors
	}

	excludes := []string{"parent", "child"}
	if len(membershipUIDs) == 0 {
		excludes = append(excludes, "membership")
	}
	// nil = caller is not managing this relation → preserve existing tuples.
	// non-nil (even empty) = caller explicitly replaces → let full-sync run.
	if writers == nil {
		excludes = append(excludes, "writer")
	}
	if auditors == nil {
		excludes = append(excludes, "auditor")
	}

	return fgatypes.GenericFGAMessage{
		ObjectType: "b2b_org",
		Operation:  "update_access",
		Data: fgatypes.GenericAccessData{
			UID:              org.UID,
			Relations:        relations,
			References:       refs,
			ExcludeRelations: excludes,
		},
	}
}

// BuildProjectMembershipFGAMessage constructs a GenericFGAMessage for a
// ProjectMembership access-control update.
func BuildProjectMembershipFGAMessage(pm *model.ProjectMembership) fgatypes.GenericFGAMessage {
	refs := make(map[string][]string)
	if pm.B2BOrgUID != "" {
		refs["b2b_org"] = []string{"b2b_org:" + pm.B2BOrgUID}
	}
	if pm.ProjectUID != "" {
		refs["project"] = []string{"project:" + pm.ProjectUID}
	}

	return fgatypes.GenericFGAMessage{
		ObjectType: "project_membership",
		Operation:  "update_access",
		Data: fgatypes.GenericAccessData{
			UID:              pm.UID,
			References:       refs,
			ExcludeRelations: []string{"key_contact"},
		},
	}
}

// BuildProjectMembershipIndexingConfig constructs an IndexingConfig for a
// ProjectMembership document.
func BuildProjectMembershipIndexingConfig(pm *model.ProjectMembership) *indexerTypes.IndexingConfig {
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

// BuildKeyContactIndexingConfig constructs an IndexingConfig for a KeyContact document.
func BuildKeyContactIndexingConfig(kc *model.KeyContact) *indexerTypes.IndexingConfig {
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

// BuildKeyContactFGAPutMessage constructs a GenericFGAMessage that grants the
// given user (sub) the key_contact relation on the parent project_membership.
func BuildKeyContactFGAPutMessage(membershipUID, sub string) fgatypes.GenericFGAMessage {
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

// BuildKeyContactFGARemoveMessage constructs a GenericFGAMessage that revokes
// the key_contact relation for the given user (sub) on the parent membership.
func BuildKeyContactFGARemoveMessage(membershipUID, sub string) fgatypes.GenericFGAMessage {
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

// BuildB2BOrgReparentingMessages returns FGA update_access messages when a
// b2b_org's ParentUID changes. Pass nil for current on create.
func BuildB2BOrgReparentingMessages(current, updated *model.B2BOrg, oldParentChildren, newParentChildren []string) []fgatypes.GenericFGAMessage {
	oldParent := ""
	if current != nil {
		oldParent = current.ParentUID
	}
	newParent := updated.ParentUID

	if oldParent == newParent {
		return nil
	}

	msgs := make([]fgatypes.GenericFGAMessage, 0, 3)

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

	if oldParent != "" && oldParentChildren != nil {
		msgs = append(msgs, BuildChildListMessage(oldParent, oldParentChildren))
	}

	if newParent != "" && newParentChildren != nil {
		msgs = append(msgs, BuildChildListMessage(newParent, newParentChildren))
	}

	return msgs
}

// BuildChildListMessage constructs an update_access FGA message that replaces
// a parent org's entire child list.
func BuildChildListMessage(parentUID string, children []string) fgatypes.GenericFGAMessage {
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

// PublishB2BOrgIndexer builds and publishes a MemberIndexerMessage for a B2BOrg.
// Errors are swallowed and logged — /admin/reindex recovers missed records.
func PublishB2BOrgIndexer(ctx context.Context, p port.MemberPublisher, org *model.B2BOrg, action indexerConstants.MessageAction) {
	indexMsg := &model.MemberIndexerMessage{
		Action:         action,
		Tags:           org.Tags(),
		IndexingConfig: BuildB2BOrgIndexingConfig(org),
	}
	builtMsg, err := indexMsg.Build(ctx, org)
	if err != nil {
		slog.WarnContext(ctx, "failed to build b2b org indexer message",
			"uid", org.UID,
			"error", err,
			"publish_failed_for_backfill_repair", true)
		return
	}
	if pubErr := p.Indexer(ctx, constants.IndexB2BOrgSubject, builtMsg, false); pubErr != nil {
		slog.WarnContext(ctx, "b2b org indexer publish failed",
			"uid", org.UID,
			"error", pubErr,
			"publish_failed_for_backfill_repair", true)
	}
}

// PublishB2BOrgSettingsIndexer builds and publishes a MemberIndexerMessage for B2BOrgSettings.
// Errors are swallowed and logged — /admin/reindex recovers missed records.
func PublishB2BOrgSettingsIndexer(ctx context.Context, p port.MemberPublisher, org *model.B2BOrg, settings *model.B2BOrgSettings, action indexerConstants.MessageAction) {
	indexMsg := &model.MemberIndexerMessage{
		Action:         action,
		Tags:           settings.Tags(),
		IndexingConfig: BuildB2BOrgSettingsIndexingConfig(org, settings),
	}
	builtMsg, err := indexMsg.Build(ctx, settings)
	if err != nil {
		slog.WarnContext(ctx, "failed to build b2b org settings indexer message",
			"uid", org.UID,
			"error", err,
			"publish_failed_for_backfill_repair", true)
		return
	}
	if pubErr := p.Indexer(ctx, constants.IndexB2BOrgSettingsSubject, builtMsg, false); pubErr != nil {
		slog.WarnContext(ctx, "b2b org settings indexer publish failed",
			"uid", org.UID,
			"error", pubErr,
			"publish_failed_for_backfill_repair", true)
	}
}

// PublishProjectMembershipIndexer builds and publishes a MemberIndexerMessage for a ProjectMembership.
// Errors are swallowed and logged — /admin/reindex recovers missed records.
func PublishProjectMembershipIndexer(ctx context.Context, p port.MemberPublisher, pm *model.ProjectMembership, action indexerConstants.MessageAction) {
	indexMsg := &model.MemberIndexerMessage{
		Action:         action,
		Tags:           pm.Tags(),
		IndexingConfig: BuildProjectMembershipIndexingConfig(pm),
	}
	builtMsg, err := indexMsg.Build(ctx, pm)
	if err != nil {
		slog.WarnContext(ctx, "failed to build project membership indexer message",
			"uid", pm.UID,
			"error", err,
			"publish_failed_for_backfill_repair", true)
		return
	}
	if pubErr := p.Indexer(ctx, constants.IndexProjectMembershipSubject, builtMsg, false); pubErr != nil {
		slog.WarnContext(ctx, "project membership indexer publish failed",
			"uid", pm.UID,
			"error", pubErr,
			"publish_failed_for_backfill_repair", true)
	}
}

// PublishProjectMembershipFGA builds and publishes a GenericFGAMessage for a ProjectMembership,
// writing the structural b2b_org and project reference tuples that enable the auditor cascade.
// Errors are swallowed and logged — /admin/reindex recovers missed records.
func PublishProjectMembershipFGA(ctx context.Context, p port.MemberPublisher, pm *model.ProjectMembership) {
	msg := BuildProjectMembershipFGAMessage(pm)
	if pubErr := p.Access(ctx, constants.FGASyncUpdateAccessSubject, msg, false); pubErr != nil {
		slog.WarnContext(ctx, "project membership fga publish failed",
			"uid", pm.UID,
			"error", pubErr,
			"publish_failed_for_backfill_repair", true)
	}
}

// PublishKeyContactIndexer builds and publishes a MemberIndexerMessage for a KeyContact.
// Errors are swallowed and logged — /admin/reindex recovers missed records.
func PublishKeyContactIndexer(ctx context.Context, p port.MemberPublisher, kc *model.KeyContact, action indexerConstants.MessageAction) {
	indexMsg := &model.MemberIndexerMessage{
		Action:         action,
		Tags:           kc.Tags(),
		IndexingConfig: BuildKeyContactIndexingConfig(kc),
	}
	builtMsg, err := indexMsg.Build(ctx, kc)
	if err != nil {
		slog.WarnContext(ctx, "failed to build key contact indexer message",
			"uid", kc.UID,
			"error", err,
			"publish_failed_for_backfill_repair", true)
		return
	}
	if pubErr := p.Indexer(ctx, constants.IndexKeyContactSubject, builtMsg, false); pubErr != nil {
		slog.WarnContext(ctx, "key contact indexer publish failed",
			"uid", kc.UID,
			"error", pubErr,
			"publish_failed_for_backfill_repair", true)
	}
}
