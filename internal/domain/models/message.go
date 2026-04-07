// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

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
