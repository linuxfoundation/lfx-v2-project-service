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

// DocumentUploadedMessage is published on lfx.projects-api.document.uploaded whenever a
// document (file) or link is added to a project. Subscribers fan out notification emails
// to LFID writers and auditors.
type DocumentUploadedMessage struct {
	ProjectUID   string `json:"project_uid"`
	DocumentName string `json:"document_name"`
	DocumentType string `json:"document_type"` // "file" | "link"
	FileName     string `json:"file_name,omitempty"`
	URL          string `json:"url,omitempty"`
	Actor        Actor  `json:"actor"`
}

// InviteAccepted is the NATS event payload published by the LFX self-serve web app
// on lfx.invite.accepted when a user completes LFID account creation and accepts
// their invite. Resource services subscribe to this subject to promote the user from
// email-only to LFID and clean up pending invite state.
type InviteAccepted struct {
	InviteUID string `json:"invite_uid"`
	Username  string `json:"username"`
}
