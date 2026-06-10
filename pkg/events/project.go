// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package events contains the wire types for NATS events published by the
// project service. Any struct that appears in a NATS message payload lives
// here so other services can import the canonical definitions.
//
// Convention: if a type is used in a NATS message payload it belongs in
// pkg/events, not internal/. Internal domain types may differ from event
// types; explicit converters in internal/service map between them.
package events

import "time"

// InviteInfo holds the pending invite metadata for a user without an LFID.
type InviteInfo struct {
	UID       string     `json:"uid"`
	Email     string     `json:"email"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// UserInfo is the user representation used in project event payloads.
type UserInfo struct {
	Name     string      `json:"name"`
	Email    string      `json:"email"`
	Username string      `json:"username"`
	Avatar   string      `json:"avatar"`
	Invite   *InviteInfo `json:"invite,omitempty"`
}

// ProjectSettings is the project-settings representation used in event payloads.
type ProjectSettings struct {
	UID                 string     `json:"uid"`
	MissionStatement    string     `json:"mission_statement"`
	AnnouncementDate    *time.Time `json:"announcement_date"`
	Auditors            []UserInfo `json:"auditors"`
	Writers             []UserInfo `json:"writers"`
	MeetingCoordinators []UserInfo `json:"meeting_coordinators"`
	ExecutiveDirector   *UserInfo  `json:"executive_director,omitempty"`
	ProgramManager      *UserInfo  `json:"program_manager,omitempty"`
	OpportunityOwner    *UserInfo  `json:"opportunity_owner,omitempty"`
	CreatedAt           *time.Time `json:"created_at"`
	UpdatedAt           *time.Time `json:"updated_at"`
}

// Actor represents the user who triggered a settings change.
type Actor struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
}

// ProjectSettingsUpdatedMessage is published on
// lfx.projects-api.project_settings.updated whenever project settings change.
// It carries both the before and after states so subscribers can diff them.
type ProjectSettingsUpdatedMessage struct {
	ProjectUID  string          `json:"project_uid"`
	OldSettings ProjectSettings `json:"old_settings"`
	NewSettings ProjectSettings `json:"new_settings"`
	Actor       Actor           `json:"actor"`
}

// ProjectDocumentCreatedMessage is published on lfx.projects-api.project_document.created
// when a file document is uploaded to a project.
type ProjectDocumentCreatedMessage struct {
	ProjectUID  string `json:"project_uid"`
	DocumentUID string `json:"document_uid"`
	Name        string `json:"name"`
	FileName    string `json:"file_name"`
	FolderUID   string `json:"folder_uid,omitempty"`
	CreatedBy   string `json:"created_by"`
}

// ProjectLinkCreatedMessage is published on lfx.projects-api.project_link.created
// when a link is added to a project.
type ProjectLinkCreatedMessage struct {
	ProjectUID string `json:"project_uid"`
	LinkUID    string `json:"link_uid"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	FolderUID  string `json:"folder_uid,omitempty"`
	CreatedBy  string `json:"created_by"`
}

// InviteAccepted is the NATS event payload published by the LFX self-serve web app
// on lfx.invite.accepted when a user completes LFID account creation and accepts
// their invite. Resource services subscribe to this subject to promote the user from
// email-only to LFID and clean up pending invite state.
type InviteAccepted struct {
	InviteUID string `json:"invite_uid"`
	Username  string `json:"username"`
}
