// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
package constants

// NATS subjects for the project service.
const (
	// IndexProjectSubject is the subject for the project indexing.
	IndexProjectSubject = "dev.lfx.index.project"
	// UpdateAccessProjectSubject is the subject for the project access control updates.
	UpdateAccessProjectSubject = "dev.lfx.update_access.project"
	// DeleteAllAccessSubject is the subject for the project access control deletion.
	DeleteAllAccessSubject = "dev.lfx.delete_all_access.project"
)

// NATS queue names for the project service.
const (
	// ProjectsAPIQueue is the queue name for the projects API.
	ProjectsAPIQueue = "dev.lfx.projects-api.queue"
)
