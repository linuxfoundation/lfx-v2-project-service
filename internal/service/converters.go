// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
)

// extractUsername extracts the username from a single UserInfo pointer. Returns empty string if nil.
func extractUsername(user *models.UserInfo) string {
	if user == nil {
		return ""
	}
	return user.Username
}

// extractUsernames extracts non-empty usernames from UserInfo slice for access control.
// Empty usernames (pending invite / unregistered email) are excluded to prevent invalid FGA tuples.
// When every username in a non-empty slice is empty, the caller omits the relation key — that
// is safe only because mergeUsers keeps a stored LFID after a lookup miss.
func extractUsernames(users []models.UserInfo) []string {
	if len(users) == 0 {
		return nil
	}

	usernames := make([]string, 0, len(users))
	for _, user := range users {
		if user.Username != "" {
			usernames = append(usernames, user.Username)
		}
	}
	if len(usernames) == 0 {
		return nil
	}
	return usernames
}

// buildFGAUpdateAccessMessage builds a GenericFGAMessage for update_access operations.
// It constructs the relations map from project settings, and the references map from the
// project's parent and the global team grants its stage allows.
func buildFGAUpdateAccessMessage(projectDB *models.ProjectBase, projectSettingsDB *models.ProjectSettings, excludeRelations ...string) fgatypes.GenericFGAMessage {
	// Build relations map for FGA sync
	relations := make(map[string][]string)
	if writers := extractUsernames(projectSettingsDB.Writers); len(writers) > 0 {
		relations[fgaconstants.RelationWriter] = writers
	}
	if auditors := extractUsernames(projectSettingsDB.Auditors); len(auditors) > 0 {
		relations[fgaconstants.RelationAuditor] = auditors
	}
	if coordinators := extractUsernames(projectSettingsDB.MeetingCoordinators); len(coordinators) > 0 {
		relations[fgaconstants.RelationMeetingCoordinator] = coordinators
	}
	if ed := extractUsername(projectSettingsDB.ExecutiveDirector); ed != "" {
		relations["executive_director"] = []string{ed}
	}
	if admins := extractUsernames(projectSettingsDB.MentorshipProgramAdmins); len(admins) > 0 {
		relations["mentorship_program_admin"] = admins
	}

	// Build references map for the global team grants and the parent relationship
	references := globalTeamGrants(projectDB.Stage)
	if projectDB.ParentUID != "" {
		references[fgaconstants.RelationParent] = []string{fgaconstants.ObjectTypeProject + projectDB.ParentUID}
	}

	return fgatypes.GenericFGAMessage{
		ObjectType: "project",
		Operation:  "update_access",
		Data: fgatypes.GenericAccessData{
			UID:        projectDB.UID,
			Public:     projectDB.Public,
			Relations:  relations,
			References: references,
			// nil unless the caller asks, so ordinary publishes keep the full sync.
			ExcludeRelations: excludeRelations,
		},
	}
}

// DomainSettingsToEvent converts an internal ProjectSettings domain model to
// its event wire type for publishing on NATS.
func DomainSettingsToEvent(s *models.ProjectSettings) events.ProjectSettings {
	if s == nil {
		return events.ProjectSettings{}
	}
	return events.ProjectSettings{
		UID:                     s.UID,
		MissionStatement:        s.MissionStatement,
		AnnouncementDate:        s.AnnouncementDate,
		Auditors:                domainUsersToEvent(s.Auditors),
		Writers:                 domainUsersToEvent(s.Writers),
		MeetingCoordinators:     domainUsersToEvent(s.MeetingCoordinators),
		ExecutiveDirector:       domainUserPtrToEvent(s.ExecutiveDirector),
		MentorshipProgramAdmins: domainUsersToEvent(s.MentorshipProgramAdmins),
		ProgramManager:          domainUserPtrToEvent(s.ProgramManager),
		OpportunityOwner:        domainUserPtrToEvent(s.OpportunityOwner),
		CreatedAt:               s.CreatedAt,
		UpdatedAt:               s.UpdatedAt,
	}
}

// DomainSettingsToSummary converts an internal ProjectSettings domain model to
// the reply of the get_settings lookup, which carries the grant roster and the
// announcement date only.
//
// The two rosters are emptied rather than left nil, so that a project with none
// configured replies with [] instead of null. That is the same guarantee
// HandleProjectGetWriters already makes, and callers of both should not have to
// decode them differently.
func DomainSettingsToSummary(s *models.ProjectSettings) events.ProjectSettingsSummary {
	if s == nil {
		return events.ProjectSettingsSummary{
			Writers:  []events.UserInfo{},
			Auditors: []events.UserInfo{},
		}
	}

	summary := events.ProjectSettingsSummary{
		UID:              s.UID,
		AnnouncementDate: s.AnnouncementDate,
		Writers:          domainUsersToEvent(s.Writers),
		Auditors:         domainUsersToEvent(s.Auditors),
	}
	if summary.Writers == nil {
		summary.Writers = []events.UserInfo{}
	}
	if summary.Auditors == nil {
		summary.Auditors = []events.UserInfo{}
	}
	return summary
}

// DomainProjectToRef converts an internal ProjectBase domain model to one entry
// of the list_projects reply.
func DomainProjectToRef(p *models.ProjectBase) events.ProjectRef {
	if p == nil {
		return events.ProjectRef{}
	}
	return events.ProjectRef{
		UID:          p.UID,
		Slug:         p.Slug,
		IsFoundation: p.IsFoundation,
		ParentUID:    p.ParentUID,
		Stage:        p.Stage,
	}
}

func domainUsersToEvent(users []models.UserInfo) []events.UserInfo {
	if users == nil {
		return nil
	}
	result := make([]events.UserInfo, len(users))
	for i, u := range users {
		result[i] = domainUserToEvent(u)
	}
	return result
}

func domainUserToEvent(u models.UserInfo) events.UserInfo {
	ev := events.UserInfo{
		Name:     u.Name,
		Email:    u.Email,
		Username: u.Username,
		Avatar:   u.Avatar,
	}
	if u.Invite != nil {
		ev.Invite = &events.InviteInfo{
			UID:       u.Invite.UID,
			Email:     u.Invite.Email,
			ExpiresAt: u.Invite.ExpiresAt,
		}
	}
	return ev
}

func domainUserPtrToEvent(u *models.UserInfo) *events.UserInfo {
	if u == nil {
		return nil
	}
	ev := domainUserToEvent(*u)
	return &ev
}

// ProjectProjection wraps a fully-loaded project's domain models and provides
// typed conversion methods to the event and FGA type universes. Callers that need
// multiple output shapes from the same (base, settings) pair build one projection
// and call the relevant methods, rather than forwarding the pair to each
// standalone converter independently. The Goa API shapes are built by the
// presentation layer from the same pair (see cmd/project-api/converters.go).
type ProjectProjection struct {
	Base     *models.ProjectBase
	Settings *models.ProjectSettings
}

// NewProjectProjection wraps base and settings into a projection. For most
// methods both fields should be non-nil; nil-safety matches the behaviour of
// the underlying converter helpers (ToEventSettings is nil-safe; ToFGAMessage
// requires non-nil fields).
func NewProjectProjection(base *models.ProjectBase, settings *models.ProjectSettings) ProjectProjection {
	return ProjectProjection{Base: base, Settings: settings}
}

// ToEventSettings converts the settings model to a NATS event wire type.
func (p ProjectProjection) ToEventSettings() events.ProjectSettings {
	return DomainSettingsToEvent(p.Settings)
}

// ToFGAMessage builds the FGA update-access message for this project.
func (p ProjectProjection) ToFGAMessage() fgatypes.GenericFGAMessage {
	return buildFGAUpdateAccessMessage(p.Base, p.Settings)
}

// directUserRelations are the relations the access message fills from project settings.
var directUserRelations = []string{
	fgaconstants.RelationWriter,
	fgaconstants.RelationAuditor,
	fgaconstants.RelationMeetingCoordinator,
	"executive_director",
	"mentorship_program_admin",
}

// ToFGAMessageKeepingDirectUsers builds the same message as ToFGAMessage but marks the
// settings-derived user relations as excluded. fga-sync still writes every tuple the message
// carries, but no longer deletes a direct user tuple that settings do not list. It is for
// backfills only: ordinary edits keep the full sync, so removing a user in settings still
// removes the tuple.
func (p ProjectProjection) ToFGAMessageKeepingDirectUsers() fgatypes.GenericFGAMessage {
	return buildFGAUpdateAccessMessage(p.Base, p.Settings, directUserRelations...)
}

// DomainDocumentToEvent converts an internal ProjectDocument to the NATS wire type.
func DomainDocumentToEvent(doc *models.ProjectDocument) events.ProjectDocumentCreatedMessage {
	folderUID := ""
	if doc.FolderUID != nil {
		folderUID = *doc.FolderUID
	}
	return events.ProjectDocumentCreatedMessage{
		ProjectUID:  doc.ProjectUID,
		DocumentUID: doc.UID,
		Name:        doc.Name,
		FileName:    doc.FileName,
		FolderUID:   folderUID,
		CreatedBy:   models.AuditCreatorUsername(doc.CreatedBy),
	}
}

// DomainLinkToEvent converts an internal ProjectLink to the NATS wire type.
func DomainLinkToEvent(link *models.ProjectLink) events.ProjectLinkCreatedMessage {
	folderUID := ""
	if link.FolderUID != nil {
		folderUID = *link.FolderUID
	}
	return events.ProjectLinkCreatedMessage{
		ProjectUID: link.ProjectUID,
		LinkUID:    link.UID,
		Name:       link.Name,
		URL:        link.URL,
		FolderUID:  folderUID,
		CreatedBy:  models.AuditCreatorUsername(link.CreatedBy),
	}
}
