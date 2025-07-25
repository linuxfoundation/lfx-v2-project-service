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
	// The subject is of the form: <lfx_environment>.lfx.index.project
	// TODO: remove the <lfx_environment>. since it isn't needed. The subjects should just be lfx.* for all subjects.
	IndexProjectSubject = ".lfx.index.project"

	// IndexProjectSettingsSubject is the subject for the project settings indexing.
	// The subject is of the form: <lfx_environment>.lfx.index.project_settings
	IndexProjectSettingsSubject = ".lfx.index.project_settings"

	// UpdateAccessProjectSubject is the subject for the project access control updates.
	// The subject is of the form: <lfx_environment>.lfx.update_access.project
	UpdateAccessProjectSubject = ".lfx.update_access.project"

	// UpdateAccessProjectSettingsSubject is the subject for the project settings access control updates.
	// The subject is of the form: <lfx_environment>.lfx.update_access.project_settings
	UpdateAccessProjectSettingsSubject = ".lfx.update_access.project_settings"

	// DeleteAllAccessSubject is the subject for the project access control deletion.
	// The subject is of the form: <lfx_environment>.lfx.delete_all_access.project
	DeleteAllAccessSubject = ".lfx.delete_all_access.project"

	// DeleteAllAccessProjectSettingsSubject is the subject for the project settings access control deletion.
	// The subject is of the form: <lfx_environment>.lfx.delete_all_access.project_settings
	DeleteAllAccessProjectSettingsSubject = ".lfx.delete_all_access.project_settings"
)

// NATS wildcard subjects that the project service handles messages about.
const (
	// ProjectsAPIQueue is the subject name for the projects API.
	// The subject is of the form: <lfx_environment>.lfx.projects-api.queue
	ProjectsAPIQueue = ".lfx.projects-api.queue"
)

// NATS specific subjects that the project service handles messages about.
const (
	// ProjectGetNameSubject is the subject for the project get name.
	// The subject is of the form: <lfx_environment>.lfx.projects-api.get_name
	ProjectGetNameSubject = ".lfx.projects-api.get_name"
	// ProjectSlugToUIDSubject is the subject for the project slug to UID.
	// The subject is of the form: <lfx_environment>.lfx.projects-api.slug_to_uid
	ProjectSlugToUIDSubject = ".lfx.projects-api.slug_to_uid"
)
