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
