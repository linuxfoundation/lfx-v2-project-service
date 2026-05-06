// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type NatsRepository struct {
	Projects        INatsKeyValue
	ProjectSettings INatsKeyValue
	Links           INatsKeyValue
	Folders         INatsKeyValue
	Documents       INatsKeyValue
	DocumentFiles   INatsObjectStore
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
		slog.ErrorContext(ctx, "error getting project from NATS KV store", constants.ErrKey, err, "project_uid", projectUID)
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
		slog.ErrorContext(ctx, "error getting project from NATS KV store", constants.ErrKey, err, "project_uid", projectUID)
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

// ─── Links ───────────────────────────────────────────────────────────────────

func (s *NatsRepository) getLink(ctx context.Context, linkUID string) (jetstream.KeyValueEntry, error) {
	return s.Links.Get(ctx, linkUID)
}

func (s *NatsRepository) getLinkUnmarshal(ctx context.Context, entry jetstream.KeyValueEntry) (*models.ProjectLink, error) {
	link := &models.ProjectLink{}
	if err := json.Unmarshal(entry.Value(), link); err != nil {
		slog.ErrorContext(ctx, "error unmarshalling link from NATS KV store", constants.ErrKey, err)
		return nil, err
	}
	return link, nil
}

// GetLink gets a project link, verifying it belongs to the given project.
func (s *NatsRepository) GetLink(ctx context.Context, projectUID, linkUID string) (*models.ProjectLink, uint64, error) {
	entry, err := s.getLink(ctx, linkUID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, 0, domain.ErrLinkNotFound
		}
		slog.ErrorContext(ctx, "error getting link from NATS KV store", constants.ErrKey, err, "link_uid", linkUID)
		return nil, 0, domain.ErrInternal
	}

	link, err := s.getLinkUnmarshal(ctx, entry)
	if err != nil {
		return nil, 0, domain.ErrUnmarshal
	}

	if link.ProjectUID != projectUID {
		return nil, 0, domain.ErrLinkNotFound
	}

	return link, entry.Revision(), nil
}

// ListLinks lists all links for a given project using the per-project index prefix.
func (s *NatsRepository) ListLinks(ctx context.Context, projectUID string) ([]*models.ProjectLink, error) {
	prefix := fmt.Sprintf("lookup/project-links/%s/", projectUID)
	keysLister, err := s.Links.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing link keys from NATS KV store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	links := []*models.ProjectLink{}
	for key := range keysLister.Keys() {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		linkUID := strings.TrimPrefix(key, prefix)
		entry, err := s.getLink(ctx, linkUID)
		if err != nil {
			slog.ErrorContext(ctx, "error getting link from NATS KV store", constants.ErrKey, err, "link_uid", linkUID)
			return nil, domain.ErrInternal
		}

		link, err := s.getLinkUnmarshal(ctx, entry)
		if err != nil {
			return nil, domain.ErrUnmarshal
		}

		links = append(links, link)
	}

	return links, nil
}

// CreateLink stores a new project link and writes a per-project index key.
func (s *NatsRepository) CreateLink(ctx context.Context, link *models.ProjectLink) error {
	data, err := json.Marshal(link)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling link into JSON", constants.ErrKey, err)
		return domain.ErrInternal
	}

	if _, err = s.Links.Put(ctx, link.UID, data); err != nil {
		slog.ErrorContext(ctx, "error putting link into NATS KV store", constants.ErrKey, err)
		return domain.ErrInternal
	}

	// Write the per-project index key so ListLinks can filter without a full scan.
	lookupKey := fmt.Sprintf(constants.KVLookupLinkKey, link.ProjectUID, link.UID)
	if _, err = s.Links.Put(ctx, lookupKey, []byte(link.UID)); err != nil {
		slog.WarnContext(ctx, "error writing link index key to NATS KV store", constants.ErrKey, err, "key", lookupKey)
	}

	return nil
}

// DeleteLink deletes a project link with optimistic concurrency and removes its index key.
func (s *NatsRepository) DeleteLink(ctx context.Context, projectUID, linkUID string, revision uint64) error {
	link, _, err := s.GetLink(ctx, projectUID, linkUID)
	if err != nil {
		return err
	}

	if err = s.Links.Delete(ctx, linkUID, jetstream.LastRevision(revision)); err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "revision mismatch deleting link", constants.ErrKey, err)
			return domain.ErrRevisionMismatch
		}
		slog.ErrorContext(ctx, "error deleting link from NATS KV store", constants.ErrKey, err)
		return domain.ErrInternal
	}

	// Clean up the per-project index key (fire-and-forget; log on failure)
	lookupKey := fmt.Sprintf(constants.KVLookupLinkKey, link.ProjectUID, link.UID)
	if err = s.Links.Purge(ctx, lookupKey); err != nil {
		slog.WarnContext(ctx, "error purging link index key from NATS KV store", constants.ErrKey, err, "key", lookupKey)
	}

	return nil
}

// ─── Folders ─────────────────────────────────────────────────────────────────

func (s *NatsRepository) getFolder(ctx context.Context, folderUID string) (jetstream.KeyValueEntry, error) {
	return s.Folders.Get(ctx, folderUID)
}

func (s *NatsRepository) getFolderUnmarshal(ctx context.Context, entry jetstream.KeyValueEntry) (*models.ProjectFolder, error) {
	folder := &models.ProjectFolder{}
	if err := json.Unmarshal(entry.Value(), folder); err != nil {
		slog.ErrorContext(ctx, "error unmarshalling folder from NATS KV store", constants.ErrKey, err)
		return nil, err
	}
	return folder, nil
}

// GetFolder gets a project folder, verifying it belongs to the given project.
func (s *NatsRepository) GetFolder(ctx context.Context, projectUID, folderUID string) (*models.ProjectFolder, uint64, error) {
	entry, err := s.getFolder(ctx, folderUID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, 0, domain.ErrFolderNotFound
		}
		slog.ErrorContext(ctx, "error getting folder from NATS KV store", constants.ErrKey, err, "folder_uid", folderUID)
		return nil, 0, domain.ErrInternal
	}

	folder, err := s.getFolderUnmarshal(ctx, entry)
	if err != nil {
		return nil, 0, domain.ErrUnmarshal
	}

	if folder.ProjectUID != projectUID {
		return nil, 0, domain.ErrFolderNotFound
	}

	return folder, entry.Revision(), nil
}

// CreateFolder stores a new project folder. The caller must first reserve the unique name
// via UniqueFolderName and roll back via DeleteUniqueFolderName on failure.
func (s *NatsRepository) CreateFolder(ctx context.Context, folder *models.ProjectFolder) error {
	data, err := json.Marshal(folder)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling folder into JSON", constants.ErrKey, err)
		return domain.ErrInternal
	}

	if _, err = s.Folders.Put(ctx, folder.UID, data); err != nil {
		slog.ErrorContext(ctx, "error putting folder into NATS KV store", constants.ErrKey, err)
		return domain.ErrInternal
	}

	return nil
}

// DeleteFolder deletes a project folder with optimistic concurrency.
func (s *NatsRepository) DeleteFolder(ctx context.Context, projectUID, folderUID string, revision uint64) error {
	folder, _, err := s.GetFolder(ctx, projectUID, folderUID)
	if err != nil {
		return err
	}

	if err = s.Folders.Delete(ctx, folderUID, jetstream.LastRevision(revision)); err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "revision mismatch deleting folder", constants.ErrKey, err)
			return domain.ErrRevisionMismatch
		}
		slog.ErrorContext(ctx, "error deleting folder from NATS KV store", constants.ErrKey, err)
		return domain.ErrInternal
	}

	// Clean up the unique name lookup key (fire-and-forget; log on failure)
	uniqueKey := fmt.Sprintf(constants.KVLookupFolderPrefix, folder.BuildIndexKey(ctx))
	if err = s.Folders.Purge(ctx, uniqueKey); err != nil {
		slog.WarnContext(ctx, "error purging folder lookup key from NATS KV store", constants.ErrKey, err, "key", uniqueKey)
	}

	return nil
}

// UniqueFolderName atomically reserves a per-project folder name.
// Returns the lookup key on success, ErrFolderNameExists if already taken.
func (s *NatsRepository) UniqueFolderName(ctx context.Context, folder *models.ProjectFolder) (string, error) {
	uniqueKey := fmt.Sprintf(constants.KVLookupFolderPrefix, folder.BuildIndexKey(ctx))
	if _, err := s.Folders.Create(ctx, uniqueKey, []byte(folder.UID)); err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return "", domain.ErrFolderNameExists
		}
		slog.ErrorContext(ctx, "error reserving folder name in NATS KV store", constants.ErrKey, err)
		return "", domain.ErrInternal
	}
	return uniqueKey, nil
}

// DeleteUniqueFolderName releases a previously reserved folder name lookup key.
func (s *NatsRepository) DeleteUniqueFolderName(ctx context.Context, uniqueKey string) error {
	if err := s.Folders.Purge(ctx, uniqueKey); err != nil {
		slog.ErrorContext(ctx, "error purging folder lookup key from NATS KV store", constants.ErrKey, err, "key", uniqueKey)
		return domain.ErrInternal
	}
	return nil
}

// ─── Documents ────────────────────────────────────────────────────────────────

func (s *NatsRepository) getDocumentMetadata(ctx context.Context, documentUID string) (jetstream.KeyValueEntry, error) {
	return s.Documents.Get(ctx, documentUID)
}

func (s *NatsRepository) getDocumentMetadataUnmarshal(ctx context.Context, entry jetstream.KeyValueEntry) (*models.ProjectDocument, error) {
	doc := &models.ProjectDocument{}
	if err := json.Unmarshal(entry.Value(), doc); err != nil {
		slog.ErrorContext(ctx, "error unmarshalling document from NATS KV store", constants.ErrKey, err)
		return nil, err
	}
	return doc, nil
}

// GetDocumentMetadata gets document metadata, verifying it belongs to the given project.
func (s *NatsRepository) GetDocumentMetadata(ctx context.Context, projectUID, documentUID string) (*models.ProjectDocument, uint64, error) {
	entry, err := s.getDocumentMetadata(ctx, documentUID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, 0, domain.ErrDocumentNotFound
		}
		slog.ErrorContext(ctx, "error getting document from NATS KV store", constants.ErrKey, err, "document_uid", documentUID)
		return nil, 0, domain.ErrInternal
	}

	doc, err := s.getDocumentMetadataUnmarshal(ctx, entry)
	if err != nil {
		return nil, 0, domain.ErrUnmarshal
	}

	if doc.ProjectUID != projectUID {
		return nil, 0, domain.ErrDocumentNotFound
	}

	return doc, entry.Revision(), nil
}

// GetDocumentFile retrieves the binary file content from the NATS Object Store.
func (s *NatsRepository) GetDocumentFile(ctx context.Context, documentUID string) ([]byte, error) {
	result, err := s.DocumentFiles.Get(ctx, documentUID)
	if err != nil {
		if errors.Is(err, nats.ErrObjectNotFound) {
			return nil, domain.ErrDocumentNotFound
		}
		slog.ErrorContext(ctx, "error getting document file from NATS object store", constants.ErrKey, err, "document_uid", documentUID)
		return nil, domain.ErrInternal
	}
	defer func() { _ = result.Close() }()

	data, err := io.ReadAll(result)
	if err != nil {
		slog.ErrorContext(ctx, "error reading document file from NATS object store", constants.ErrKey, err, "document_uid", documentUID)
		return nil, domain.ErrInternal
	}

	return data, nil
}

// ListDocuments lists all document metadata for a given project.
func (s *NatsRepository) ListDocuments(ctx context.Context, projectUID string) ([]*models.ProjectDocument, error) {
	keysLister, err := s.Documents.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing document keys from NATS KV store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	docs := []*models.ProjectDocument{}
	for key := range keysLister.Keys() {
		if strings.HasPrefix(key, "lookup/") {
			continue
		}

		entry, err := s.getDocumentMetadata(ctx, key)
		if err != nil {
			slog.ErrorContext(ctx, "error getting document from NATS KV store", constants.ErrKey, err, "document_uid", key)
			return nil, domain.ErrInternal
		}

		doc, err := s.getDocumentMetadataUnmarshal(ctx, entry)
		if err != nil {
			return nil, domain.ErrUnmarshal
		}

		if doc.ProjectUID == projectUID {
			docs = append(docs, doc)
		}
	}

	return docs, nil
}

// CreateDocumentMetadata stores document metadata. The caller must first reserve the unique name
// via UniqueDocumentName and roll back via DeleteUniqueDocumentName on failure.
func (s *NatsRepository) CreateDocumentMetadata(ctx context.Context, doc *models.ProjectDocument) error {
	data, err := json.Marshal(doc)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling document into JSON", constants.ErrKey, err)
		return domain.ErrInternal
	}

	if _, err = s.Documents.Put(ctx, doc.UID, data); err != nil {
		slog.ErrorContext(ctx, "error putting document into NATS KV store", constants.ErrKey, err)
		return domain.ErrInternal
	}

	return nil
}

// PutDocumentFile stores binary file content in the NATS Object Store.
func (s *NatsRepository) PutDocumentFile(ctx context.Context, documentUID string, fileData []byte) error {
	meta := jetstream.ObjectMeta{Name: documentUID}
	if _, err := s.DocumentFiles.Put(ctx, meta, bytes.NewReader(fileData)); err != nil {
		slog.ErrorContext(ctx, "error putting document file into NATS object store", constants.ErrKey, err, "document_uid", documentUID)
		return domain.ErrInternal
	}
	return nil
}

// DeleteDocumentMetadata deletes document metadata with optimistic concurrency.
func (s *NatsRepository) DeleteDocumentMetadata(ctx context.Context, projectUID, documentUID string, revision uint64) error {
	doc, _, err := s.GetDocumentMetadata(ctx, projectUID, documentUID)
	if err != nil {
		return err
	}

	if err = s.Documents.Delete(ctx, documentUID, jetstream.LastRevision(revision)); err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "revision mismatch deleting document", constants.ErrKey, err)
			return domain.ErrRevisionMismatch
		}
		slog.ErrorContext(ctx, "error deleting document from NATS KV store", constants.ErrKey, err)
		return domain.ErrInternal
	}

	// Clean up the unique name lookup key (fire-and-forget; log on failure)
	uniqueKey := fmt.Sprintf(constants.KVLookupDocumentPrefix, doc.BuildIndexKey(ctx))
	if err = s.Documents.Purge(ctx, uniqueKey); err != nil {
		slog.WarnContext(ctx, "error purging document lookup key from NATS KV store", constants.ErrKey, err, "key", uniqueKey)
	}

	return nil
}

// DeleteDocumentFile removes the binary file from the NATS Object Store.
func (s *NatsRepository) DeleteDocumentFile(ctx context.Context, documentUID string) error {
	if err := s.DocumentFiles.Delete(ctx, documentUID); err != nil {
		slog.ErrorContext(ctx, "error deleting document file from NATS object store", constants.ErrKey, err, "document_uid", documentUID)
		return domain.ErrInternal
	}
	return nil
}

// UniqueDocumentName atomically reserves a per-project document name.
// Returns the lookup key on success, ErrDocumentNameExists if already taken.
func (s *NatsRepository) UniqueDocumentName(ctx context.Context, doc *models.ProjectDocument) (string, error) {
	uniqueKey := fmt.Sprintf(constants.KVLookupDocumentPrefix, doc.BuildIndexKey(ctx))
	if _, err := s.Documents.Create(ctx, uniqueKey, []byte(doc.UID)); err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return "", domain.ErrDocumentNameExists
		}
		slog.ErrorContext(ctx, "error reserving document name in NATS KV store", constants.ErrKey, err)
		return "", domain.ErrInternal
	}
	return uniqueKey, nil
}

// DeleteUniqueDocumentName releases a previously reserved document name lookup key.
func (s *NatsRepository) DeleteUniqueDocumentName(ctx context.Context, uniqueKey string) error {
	if err := s.Documents.Purge(ctx, uniqueKey); err != nil {
		slog.ErrorContext(ctx, "error purging document lookup key from NATS KV store", constants.ErrKey, err, "key", uniqueKey)
		return domain.ErrInternal
	}
	return nil
}
