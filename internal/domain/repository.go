// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

// ProjectRepository defines the interface for project storage operations.
// This interface can be implemented by different storage backends (NATS, PostgreSQL, etc.)
type ProjectRepository interface {
	// Project full operations
	CreateProject(ctx context.Context, projectBase *models.ProjectBase, projectSettings *models.ProjectSettings) error
	ProjectExists(ctx context.Context, projectUID string) (bool, error)
	DeleteProject(ctx context.Context, projectUID string, revision uint64) error

	// Project base operations
	GetProjectBase(ctx context.Context, projectUID string) (*models.ProjectBase, error)
	GetProjectBaseWithRevision(ctx context.Context, projectUID string) (*models.ProjectBase, uint64, error)
	UpdateProjectBase(ctx context.Context, projectBase *models.ProjectBase, revision uint64) error

	// Project settings operations
	GetProjectSettings(ctx context.Context, projectUID string) (*models.ProjectSettings, error)
	GetProjectSettingsWithRevision(ctx context.Context, projectUID string) (*models.ProjectSettings, uint64, error)
	UpdateProjectSettings(ctx context.Context, projectSettings *models.ProjectSettings, revision uint64) error

	// Slug operations
	GetProjectUIDFromSlug(ctx context.Context, projectSlug string) (string, error)
	ProjectSlugExists(ctx context.Context, projectSlug string) (bool, error)

	// Bulk operations
	ListAllProjects(ctx context.Context) ([]*models.ProjectBase, []*models.ProjectSettings, error)
	ListAllProjectsBase(ctx context.Context) ([]*models.ProjectBase, error)
	ListAllProjectsSettings(ctx context.Context) ([]*models.ProjectSettings, error)
}

// DocumentRepository defines the interface for project document storage operations.
type DocumentRepository interface {
	GetDocumentMetadata(ctx context.Context, projectUID, documentUID string) (*models.ProjectDocument, uint64, error)
	GetDocumentFile(ctx context.Context, documentUID string) ([]byte, error)
	CreateDocumentMetadata(ctx context.Context, doc *models.ProjectDocument) error
	PutDocumentFile(ctx context.Context, documentUID string, fileData []byte) error
	DeleteDocumentMetadata(ctx context.Context, projectUID, documentUID string, revision uint64) error
	DeleteDocumentFile(ctx context.Context, documentUID string) error
	UniqueDocumentName(ctx context.Context, doc *models.ProjectDocument) (string, error)
	DeleteUniqueDocumentName(ctx context.Context, uniqueKey string) error
}

// LinkRepository defines the interface for project link storage operations.
type LinkRepository interface {
	GetLink(ctx context.Context, projectUID, linkUID string) (*models.ProjectLink, uint64, error)
	ListLinks(ctx context.Context, projectUID string) ([]*models.ProjectLink, error)
	CreateLink(ctx context.Context, link *models.ProjectLink) error
	DeleteLink(ctx context.Context, projectUID, linkUID string, revision uint64) error
}

// FolderRepository defines the interface for project folder storage operations.
type FolderRepository interface {
	GetFolder(ctx context.Context, projectUID, folderUID string) (*models.ProjectFolder, uint64, error)
	ListFolders(ctx context.Context, projectUID string) ([]*models.ProjectFolder, error)
	CreateFolder(ctx context.Context, folder *models.ProjectFolder) error
	DeleteFolder(ctx context.Context, projectUID, folderUID string, revision uint64) error
	UniqueFolderName(ctx context.Context, folder *models.ProjectFolder) (string, error)
	DeleteUniqueFolderName(ctx context.Context, uniqueKey string) error
}
