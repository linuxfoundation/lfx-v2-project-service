// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

// ProjectAccessData is the schema for the data in the message sent to the fga-sync service.
// These are the fields that the fga-sync service needs in order to update the OpenFGA permissions.
type ProjectAccessData struct {
	UID                 string   `json:"uid"`
	Public              bool     `json:"public"`
	ParentUID           string   `json:"parent_uid"`
	Writers             []string `json:"writers"`
	Auditors            []string `json:"auditors"`
	MeetingCoordinators []string `json:"meeting_coordinators"`
}

// ProjectAccessMessage is a type-safe NATS message for project access control operations.
type ProjectAccessMessage struct {
	Data ProjectAccessData `json:"data"`
}

// ProjectSettingsUpdatedMessage is a NATS message published when project settings are updated.
// It contains both the before and after states to allow downstream services to react to changes.
type ProjectSettingsUpdatedMessage struct {
	ProjectUID  string          `json:"project_uid"`
	OldSettings ProjectSettings `json:"old_settings"`
	NewSettings ProjectSettings `json:"new_settings"`
}
