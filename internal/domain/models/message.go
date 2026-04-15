// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

// ProjectSettingsUpdatedMessage is a NATS message published when project settings are updated.
// It contains both the before and after states to allow downstream services to react to changes.
type ProjectSettingsUpdatedMessage struct {
	ProjectUID  string          `json:"project_uid"`
	OldSettings ProjectSettings `json:"old_settings"`
	NewSettings ProjectSettings `json:"new_settings"`
}
