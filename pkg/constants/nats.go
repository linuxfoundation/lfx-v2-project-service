// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package constants

// NATS Key-Value store bucket names.
const (
	// KVBucketNameProjects is the name of the KV bucket for projects.
	KVBucketNameProjects = "projects"
)

// NATS subjects that the project service sends messages about.
const (
	// IndexProjectSubject is the subject for the project indexing.
	// The subject is of the form: <lfx_environment>.lfx.index.project
	IndexProjectSubject = ".lfx.index.project"

	// UpdateAccessProjectSubject is the subject for the project access control updates.
	// The subject is of the form: <lfx_environment>.lfx.update_access.project
	UpdateAccessProjectSubject = ".lfx.update_access.project"

	// DeleteAllAccessSubject is the subject for the project access control deletion.
	// The subject is of the form: <lfx_environment>.lfx.delete_all_access.project
	DeleteAllAccessSubject = ".lfx.delete_all_access.project"
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
