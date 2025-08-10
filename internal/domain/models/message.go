// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

// MessageAction is a type for the action of a project message.
type MessageAction string

// MessageAction constants for the action of a project message.
const (
	// ActionCreated is the action for a resource creation message.
	ActionCreated MessageAction = "created"
	// ActionUpdated is the action for a resource update message.
	ActionUpdated MessageAction = "updated"
	// ActionDeleted is the action for a resource deletion message.
	ActionDeleted MessageAction = "deleted"
)

// ProjectIndexerMessage is a type-safe NATS message for project indexing operations.
type ProjectIndexerMessage struct {
	Action MessageAction `json:"action"`
	Data   ProjectBase   `json:"data"`
	Tags   []string      `json:"tags"`
}

// ProjectSettingsIndexerMessage is a type-safe NATS message for project settings indexing operations.
type ProjectSettingsIndexerMessage struct {
	Action MessageAction   `json:"action"`
	Data   ProjectSettings `json:"data"`
	Tags   []string        `json:"tags"`
}

// IndexerMessageEnvelope is the actual message format sent to NATS for indexing operations.
type IndexerMessageEnvelope struct {
	Action  MessageAction     `json:"action"`
	Headers map[string]string `json:"headers"`
	Data    any               `json:"data"`
	Tags    []string          `json:"tags"`
}

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
