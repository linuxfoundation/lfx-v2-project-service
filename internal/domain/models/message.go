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

// GenericFGAMessage is the envelope for all FGA sync operations.
// It uses the generic, resource-agnostic FGA Sync handlers.
type GenericFGAMessage struct {
	ObjectType string `json:"object_type"` // Resource type (e.g., "project", "committee", "meeting")
	Operation  string `json:"operation"`   // Operation name (e.g., "update_access", "delete_access")
	Data       any    `json:"data"`        // Operation-specific payload
}

// UpdateAccessData is the data payload for update_access operations.
// This is a full sync operation - any relations not included will be removed.
type UpdateAccessData struct {
	UID              string              `json:"uid"`                         // Unique identifier for the resource
	Public           bool                `json:"public"`                      // If true, adds user:* as viewer
	Relations        map[string][]string `json:"relations,omitempty"`         // Map of relation names to arrays of usernames
	References       map[string][]string `json:"references,omitempty"`        // Map of relation names to arrays of object UIDs
	ExcludeRelations []string            `json:"exclude_relations,omitempty"` // Relations managed elsewhere
}

// DeleteAccessData is the data payload for delete_access operations.
// Deletes all access control tuples for a resource.
type DeleteAccessData struct {
	UID string `json:"uid"` // Unique identifier for the resource to delete
}

// ProjectSettingsUpdatedMessage is a NATS message published when project settings are updated.
// It contains both the before and after states to allow downstream services to react to changes.
type ProjectSettingsUpdatedMessage struct {
	ProjectUID  string          `json:"project_uid"`
	OldSettings ProjectSettings `json:"old_settings"`
	NewSettings ProjectSettings `json:"new_settings"`
}
