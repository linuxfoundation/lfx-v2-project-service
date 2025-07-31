// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
)

type NatsRepository struct {
	Projects        INatsKeyValue
	ProjectSettings INatsKeyValue
}

func NewNatsRepository(projects INatsKeyValue, projectSettings INatsKeyValue) *NatsRepository {
	return &NatsRepository{
		Projects:        projects,
		ProjectSettings: projectSettings,
	}
}

func (s *NatsRepository) getProjectBase(ctx context.Context, projectUID string) (jetstream.KeyValueEntry, error) {
	entry, err := s.Projects.Get(ctx, projectUID)
	if err != nil {
		return nil, err
	}

	return entry, nil
}

func (s *NatsRepository) getProjectBaseUnmarshal(ctx context.Context, entry jetstream.KeyValueEntry) (*models.ProjectBase, error) {
	projectDB := models.ProjectBase{}
	err := json.Unmarshal(entry.Value(), &projectDB)
	if err != nil {
		slog.ErrorContext(ctx, "error unmarshalling project from NATS KV store", constants.ErrKey, err)
		return nil, err
	}

	return &projectDB, nil
}

// GetProjectBase gets the project base from the NATS KV store.
func (s *NatsRepository) GetProjectBase(ctx context.Context, projectUID string) (*models.ProjectBase, error) {
	entry, err := s.getProjectBase(ctx, projectUID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, domain.ErrProjectNotFound
		}
		return nil, domain.ErrInternal
	}

	projectDB, err := s.getProjectBaseUnmarshal(ctx, entry)
	if err != nil {
		return nil, domain.ErrUnmarshal
	}

	return projectDB, nil
}

// GetProjectBaseWithRevision gets the project base from the NATS KV store along with its revision.
func (s *NatsRepository) GetProjectBaseWithRevision(ctx context.Context, projectUID string) (*models.ProjectBase, uint64, error) {
	entry, err := s.getProjectBase(ctx, projectUID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, 0, domain.ErrProjectNotFound
		}
		return nil, 0, domain.ErrInternal
	}

	projectDB, err := s.getProjectBaseUnmarshal(ctx, entry)
	if err != nil {
		return nil, 0, domain.ErrUnmarshal
	}

	return projectDB, entry.Revision(), nil
}

// ProjectExists checks if a project exists in the NATS KV store.
func (s *NatsRepository) ProjectExists(ctx context.Context, projectUID string) (bool, error) {
	_, err := s.getProjectBase(ctx, projectUID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil
		}
		return false, domain.ErrInternal
	}

	return true, nil
}

// GetProjectUIDFromSlug gets the project UID from the project slug.
func (s *NatsRepository) GetProjectUIDFromSlug(ctx context.Context, projectSlug string) (projectUID string, err error) {
	var entry jetstream.KeyValueEntry
	entry, err = s.Projects.Get(ctx, fmt.Sprintf("slug/%s", projectSlug))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "project not found", constants.ErrKey, err)
			return "", domain.ErrProjectNotFound
		}
		slog.ErrorContext(ctx, "error getting project from NATS KV store", constants.ErrKey, err)
		return "", domain.ErrInternal
	}

	return string(entry.Value()), nil
}

// ProjectSlugExists checks if a project slug exists in the NATS KV store.
func (s *NatsRepository) ProjectSlugExists(ctx context.Context, projectSlug string) (bool, error) {
	_, err := s.GetProjectUIDFromSlug(ctx, projectSlug)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			// We only care about knowing the existence of the slug, so this is not an error
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// ListAllProjectsBase lists all project base data from the NATS KV stores.
func (s *NatsRepository) ListAllProjectsBase(ctx context.Context) ([]*models.ProjectBase, error) {
	keysLister, err := s.Projects.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing project keys from NATS KV store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	projectsBase := []*models.ProjectBase{}
	for key := range keysLister.Keys() {
		// Skip slug mappings
		if strings.HasPrefix(key, "slug/") {
			continue
		}

		entry, err := s.getProjectBase(ctx, key)
		if err != nil {
			slog.ErrorContext(ctx, "error getting project from NATS KV store", constants.ErrKey, err, "project_uid", key)
			return nil, domain.ErrInternal
		}

		projectDB, err := s.getProjectBaseUnmarshal(ctx, entry)
		if err != nil {
			slog.ErrorContext(ctx, "error unmarshalling project from NATS KV store", constants.ErrKey, err, "project_uid", key)
			return nil, domain.ErrUnmarshal
		}

		projectsBase = append(projectsBase, projectDB)
	}

	return projectsBase, nil
}

// ListAllProjectsSettings lists all project settings data from the NATS KV stores.
func (s *NatsRepository) ListAllProjectsSettings(ctx context.Context) ([]*models.ProjectSettings, error) {
	keysLister, err := s.ProjectSettings.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing project settings keys from NATS KV store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	projectsSettings := []*models.ProjectSettings{}
	for key := range keysLister.Keys() {
		entry, err := s.ProjectSettings.Get(ctx, key)
		if err != nil {
			slog.ErrorContext(ctx, "error getting project settings from NATS KV store", constants.ErrKey, err, "project_uid", key)
			return nil, domain.ErrInternal
		}

		projectSettingsDB := &models.ProjectSettings{}
		err = json.Unmarshal(entry.Value(), projectSettingsDB)
		if err != nil {
			slog.ErrorContext(ctx, "error unmarshalling project settings from NATS KV store", constants.ErrKey, err, "project_uid", key)
			return nil, domain.ErrUnmarshal
		}

		projectsSettings = append(projectsSettings, projectSettingsDB)
	}

	return projectsSettings, nil
}

// ListAllProjects lists all projects from the NATS KV stores.
func (s *NatsRepository) ListAllProjects(ctx context.Context) ([]*models.ProjectBase, []*models.ProjectSettings, error) {
	projectsBase, err := s.ListAllProjectsBase(ctx)
	if err != nil {
		return nil, nil, err
	}

	projectsSettings, err := s.ListAllProjectsSettings(ctx)
	if err != nil {
		return nil, nil, err
	}

	return projectsBase, projectsSettings, nil
}

func (s *NatsRepository) putProjectSlugMapping(ctx context.Context, projectBase *models.ProjectBase) (uint64, error) {
	revision, err := s.Projects.Put(ctx, fmt.Sprintf("slug/%s", projectBase.Slug), []byte(projectBase.UID))
	if err != nil {
		return 0, err
	}

	return revision, nil
}

func (s *NatsRepository) putProjectBase(ctx context.Context, projectBase *models.ProjectBase) (uint64, error) {
	projectBaseBytes, err := json.Marshal(projectBase)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project into JSON", constants.ErrKey, err)
		return 0, err
	}

	revision, err := s.Projects.Put(ctx, projectBase.UID, projectBaseBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error putting project into NATS KV store", constants.ErrKey, err)
		return 0, err
	}

	return revision, nil
}

func (s *NatsRepository) putProjectSettings(ctx context.Context, projectSettings *models.ProjectSettings) (uint64, error) {
	projectSettingsBytes, err := json.Marshal(projectSettings)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project settings into JSON", constants.ErrKey, err)
		return 0, err
	}

	revision, err := s.ProjectSettings.Put(ctx, projectSettings.UID, projectSettingsBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error putting project settings into NATS KV store", constants.ErrKey, err)
		return 0, err
	}

	return revision, nil
}

// CreateProject creates a new project in the NATS KV stores.
func (s *NatsRepository) CreateProject(ctx context.Context, projectBase *models.ProjectBase, projectSettings *models.ProjectSettings) error {

	// Create slug mapping first
	_, err := s.putProjectSlugMapping(ctx, projectBase)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			slog.WarnContext(ctx, "project slug already exists", constants.ErrKey, err)
			return domain.ErrProjectSlugExists
		}
		slog.ErrorContext(ctx, "error putting project UID mapping into NATS KV store", constants.ErrKey, err)
		return err
	}

	// Store the project base data
	_, err = s.putProjectBase(ctx, projectBase)
	if err != nil {
		return domain.ErrInternal
	}

	// Store the project settings if provided
	if projectSettings != nil {
		_, err = s.putProjectSettings(ctx, projectSettings)
		if err != nil {
			return domain.ErrInternal
		}
	}

	return nil
}

func (s *NatsRepository) updateProjectBase(ctx context.Context, projectBase *models.ProjectBase, revision uint64) error {
	projectBaseBytes, err := json.Marshal(projectBase)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project into JSON", constants.ErrKey, err)
		return err
	}

	_, err = s.Projects.Update(ctx, projectBase.UID, projectBaseBytes, revision)
	if err != nil {
		slog.ErrorContext(ctx, "error updating project in NATS KV store", constants.ErrKey, err)
		return err
	}

	return nil
}

// UpdateProjectBase updates a project's base information in the NATS KV store.
func (s *NatsRepository) UpdateProjectBase(ctx context.Context, projectBase *models.ProjectBase, revision uint64) error {
	// Get the existing project to check if slug is changing
	existingProject, err := s.GetProjectBase(ctx, projectBase.UID)
	if err != nil {
		return err
	}

	// If the slug is changing, update the slug mapping
	if existingProject.Slug != projectBase.Slug {
		// Delete the old slug mapping
		err = s.deleteProjectSlugMapping(ctx, existingProject.Slug)
		if err != nil {
			return domain.ErrInternal
		}

		// Create the new slug mapping
		_, err = s.putProjectSlugMapping(ctx, projectBase)
		if err != nil {
			return domain.ErrInternal
		}
	}

	// Update the project base data
	err = s.updateProjectBase(ctx, projectBase, revision)
	if err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "revision mismatch", constants.ErrKey, err)
			return domain.ErrRevisionMismatch
		}
		return domain.ErrInternal
	}

	return nil
}

func (s *NatsRepository) getProjectSettings(ctx context.Context, projectUID string) (jetstream.KeyValueEntry, error) {
	entry, err := s.ProjectSettings.Get(ctx, projectUID)
	if err != nil {
		slog.ErrorContext(ctx, "error getting project settings from NATS KV store", constants.ErrKey, err)
		return nil, err
	}

	return entry, nil
}

func (s *NatsRepository) getProjectSettingsUnmarshal(ctx context.Context, entry jetstream.KeyValueEntry) (*models.ProjectSettings, error) {
	projectSettingsDB := models.ProjectSettings{}
	err := json.Unmarshal(entry.Value(), &projectSettingsDB)
	if err != nil {
		slog.ErrorContext(ctx, "error unmarshalling project settings from NATS KV store", constants.ErrKey, err)
		return nil, err
	}

	return &projectSettingsDB, nil
}

// GetProjectSettings gets the project settings from the NATS KV store.
func (s *NatsRepository) GetProjectSettings(ctx context.Context, projectUID string) (*models.ProjectSettings, error) {
	entry, err := s.getProjectSettings(ctx, projectUID)
	if err != nil {
		return nil, err
	}

	return s.getProjectSettingsUnmarshal(ctx, entry)
}

// GetProjectSettingsWithRevision gets the project settings from the NATS KV store along with its revision.
func (s *NatsRepository) GetProjectSettingsWithRevision(ctx context.Context, projectUID string) (*models.ProjectSettings, uint64, error) {
	entry, err := s.getProjectSettings(ctx, projectUID)
	if err != nil {
		return nil, 0, err
	}
	slog.InfoContext(ctx, "GetProjectSettingsWithRevision", "revision", entry.Revision())

	projectSettingsDB, err := s.getProjectSettingsUnmarshal(ctx, entry)
	if err != nil {
		return nil, 0, err
	}

	return projectSettingsDB, entry.Revision(), nil
}

func (s *NatsRepository) updateProjectSettings(ctx context.Context, projectSettings *models.ProjectSettings, revision uint64) error {
	projectSettingsBytes, err := json.Marshal(projectSettings)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project settings into JSON", constants.ErrKey, err)
		return err
	}

	slog.InfoContext(ctx, "updateProjectSettings", "revision", revision)

	_, err = s.ProjectSettings.Update(ctx, projectSettings.UID, projectSettingsBytes, revision)
	if err != nil {
		return err
	}

	return nil
}

// UpdateProjectSettings updates a project's settings information in the NATS KV store.
func (s *NatsRepository) UpdateProjectSettings(ctx context.Context, projectSettings *models.ProjectSettings, revision uint64) error {
	err := s.updateProjectSettings(ctx, projectSettings, revision)
	if err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "revision mismatch", constants.ErrKey, err)
			return domain.ErrRevisionMismatch
		}
		return domain.ErrInternal
	}

	return nil
}

func (s *NatsRepository) deleteProjectSlugMapping(ctx context.Context, projectSlug string) error {
	err := s.Projects.Delete(ctx, fmt.Sprintf("slug/%s", projectSlug))
	if err != nil {
		slog.ErrorContext(ctx, "error deleting slug mapping from NATS KV store", constants.ErrKey, err)
		return err
	}

	return nil
}

func (s *NatsRepository) deleteProjectBase(ctx context.Context, projectUID string, revision uint64) error {
	err := s.Projects.Delete(ctx, projectUID, jetstream.LastRevision(revision))
	if err != nil {
		slog.ErrorContext(ctx, "error deleting project from NATS KV store", constants.ErrKey, err)
		return err
	}

	return nil
}

func (s *NatsRepository) deleteProjectSettings(ctx context.Context, projectUID string) error {
	err := s.ProjectSettings.Delete(ctx, projectUID)
	if err != nil {
		slog.ErrorContext(ctx, "error deleting project settings from NATS KV store", constants.ErrKey, err)
		return err
	}

	return nil
}

// DeleteProject deletes a project from the NATS KV stores.
func (s *NatsRepository) DeleteProject(ctx context.Context, projectUID string, revision uint64) error {
	// Get the project to find the slug
	project, err := s.GetProjectBase(ctx, projectUID)
	if err != nil {
		return err
	}

	// Delete the project with revision check
	err = s.deleteProjectBase(ctx, projectUID, revision)
	if err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "revision mismatch", constants.ErrKey, err)
			return domain.ErrRevisionMismatch
		}
		return domain.ErrInternal
	}

	// Delete the slug mapping
	err = s.deleteProjectSlugMapping(ctx, project.Slug)
	if err != nil {
		return domain.ErrInternal
	}

	// Delete the project settings (if they exist)
	err = s.deleteProjectSettings(ctx, projectUID)
	if err != nil {
		return domain.ErrInternal
	}

	return nil
}
