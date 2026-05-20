// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package constants

// NATS Key-Value store bucket names.
const (
	// KVStoreNameProjects is the name of the KV store for projects.
	KVStoreNameProjects = "projects"

	// KVStoreNameProjectSettings is the name of the KV store for project settings.
	KVStoreNameProjectSettings = "project-settings"

	// KVStoreNameProjectLinks is the name of the KV store for project links.
	KVStoreNameProjectLinks = "project-links"

	// KVStoreNameProjectFolders is the name of the KV store for project folders.
	KVStoreNameProjectFolders = "project-folders"

	// KVStoreNameProjectDocuments is the name of the KV store for project document metadata.
	KVStoreNameProjectDocuments = "project-documents-metadata"

	// ObjectStoreNameProjectDocuments is the name of the object store for project document files.
	ObjectStoreNameProjectDocuments = "project-documents"

	// KVLookupFolderPrefix is the lookup key prefix for project folder name uniqueness.
	KVLookupFolderPrefix = "lookup/project-folders/%s"

	// KVLookupDocumentPrefix is the lookup key prefix for project document name uniqueness.
	KVLookupDocumentPrefix = "lookup/project-documents/%s"

	// KVLookupLinkKey is the per-project link index key.
	// Format: lookup/project-links/<projectUID>/<linkUID>
	KVLookupLinkKey = "lookup/project-links/%s/%s"

	// KVLookupInviteMappingPrefix is the mapping key prefix that resolves an invite UID to the
	// project UID whose settings contain that invite.
	// Key: "lookup/project-settings-invite/<invite_uid>", Value: <project_uid>
	KVLookupInviteMappingPrefix = "lookup/project-settings-invite/%s"
)

// NATS subjects that the project service sends messages about.
const (
	// IndexProjectSubject is the subject for the project indexing.
	// The subject is of the form: lfx.index.project
	IndexProjectSubject = "lfx.index.project"

	// IndexProjectSettingsSubject is the subject for the project settings indexing.
	// The subject is of the form: lfx.index.project_settings
	IndexProjectSettingsSubject = "lfx.index.project_settings"

	// ProjectSettingsUpdatedSubject is the subject for project settings change events.
	// This event is published when project settings are updated, containing both before and after states.
	// The subject is of the form: lfx.projects-api.project_settings.updated
	ProjectSettingsUpdatedSubject = "lfx.projects-api.project_settings.updated"

	// IndexProjectLinkSubject is the subject for project link indexing.
	IndexProjectLinkSubject = "lfx.index.project_link"

	// IndexProjectFolderSubject is the subject for project folder indexing.
	IndexProjectFolderSubject = "lfx.index.project_folder"

	// IndexProjectDocumentSubject is the subject for project document indexing.
	IndexProjectDocumentSubject = "lfx.index.project_document"
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

// NATS subjects for external service lookups.
const (
	// AuthUserMetadataReadSubject is the subject for looking up a user's profile metadata by principal.
	// The subject is of the form: lfx.auth-service.user_metadata.read
	AuthUserMetadataReadSubject = "lfx.auth-service.user_metadata.read"
)
