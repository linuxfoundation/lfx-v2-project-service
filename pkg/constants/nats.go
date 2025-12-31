// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package constants

// NATS Key-Value store bucket names.
const (
	// KVStoreNameProjects is the name of the KV store for projects.
	KVStoreNameProjects = "projects"

	// KVStoreNameProjectSettings is the name of the KV store for project settings.
	KVStoreNameProjectSettings = "project-settings"
)

// NATS subjects that the project service sends messages about.
const (
	// IndexProjectSubject is the subject for the project indexing.
	// The subject is of the form: lfx.index.project
	IndexProjectSubject = "lfx.index.project"

	// IndexProjectSettingsSubject is the subject for the project settings indexing.
	// The subject is of the form: lfx.index.project_settings
	IndexProjectSettingsSubject = "lfx.index.project_settings"

	// UpdateAccessProjectSubject is the subject for the project access control updates.
	// The subject is of the form: lfx.update_access.project
	UpdateAccessProjectSubject = "lfx.update_access.project"

	// UpdateAccessProjectSettingsSubject is the subject for the project settings access control updates.
	// The subject is of the form: lfx.update_access.project_settings
	UpdateAccessProjectSettingsSubject = "lfx.update_access.project_settings"

	// DeleteAllAccessSubject is the subject for the project access control deletion.
	// The subject is of the form: lfx.delete_all_access.project
	DeleteAllAccessSubject = "lfx.delete_all_access.project"

	// DeleteAllAccessProjectSettingsSubject is the subject for the project settings access control deletion.
	// The subject is of the form: lfx.delete_all_access.project_settings
	DeleteAllAccessProjectSettingsSubject = "lfx.delete_all_access.project_settings"

	// ProjectSettingsUpdatedSubject is the subject for project settings change events.
	// This event is published when project settings are updated, containing both before and after states.
	// The subject is of the form: lfx.projects-api.project_settings.updated
	ProjectSettingsUpdatedSubject = "lfx.projects-api.project_settings.updated"
)

// NATS wildcard subjects that the project service handles messages about.
const (
	// ProjectsAPIQueue is the subject name for the projects API.
	// The subject is of the form: lfx.projects-api.queue
	ProjectsAPIQueue = "lfx.projects-api.queue"
)

// NATS specific subjects that the project service handles messages about.
const (
	// ProjectGetNameSubject is the subject for the project get name.
	// The subject is of the form: lfx.projects-api.get_name
	ProjectGetNameSubject = "lfx.projects-api.get_name"
	// ProjectGetSlugSubject is the subject for the project get slug.
	// The subject is of the form: lfx.projects-api.get_slug
	ProjectGetSlugSubject = "lfx.projects-api.get_slug"
	// ProjectGetLogoSubject is the subject for the project get logo.
	// The subject is of the form: lfx.projects-api.get_logo
	ProjectGetLogoSubject = "lfx.projects-api.get_logo"
	// ProjectSlugToUIDSubject is the subject for the project slug to UID.
	// The subject is of the form: lfx.projects-api.slug_to_uid
	ProjectSlugToUIDSubject = "lfx.projects-api.slug_to_uid"
	// ProjectGetParentUIDSubject is the subject for getting the parent project UID.
	// The subject is of the form: lfx.projects-api.get_parent_uid
	ProjectGetParentUIDSubject = "lfx.projects-api.get_parent_uid"
)
