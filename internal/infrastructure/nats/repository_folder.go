// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
)

func (s *NatsRepository) folders() kvCollection[models.ProjectFolder] {
	return kvCollection[models.ProjectFolder]{
		kv:         s.Folders,
		noun:       "folder",
		notFound:   domain.ErrFolderNotFound,
		projectUID: func(f *models.ProjectFolder) string { return f.ProjectUID },
	}
}

// GetFolder gets a project folder, verifying it belongs to the given project.
func (s *NatsRepository) GetFolder(ctx context.Context, projectUID, folderUID string) (*models.ProjectFolder, uint64, error) {
	return s.folders().getForProject(ctx, projectUID, folderUID)
}

// CreateFolder stores a new project folder. The caller must first reserve the unique name
// via UniqueFolderName and roll back via DeleteUniqueFolderName on failure.
func (s *NatsRepository) CreateFolder(ctx context.Context, folder *models.ProjectFolder) error {
	return s.folders().put(ctx, folder.UID, folder)
}

// DeleteFolder deletes a project folder with optimistic concurrency.
func (s *NatsRepository) DeleteFolder(ctx context.Context, projectUID, folderUID string, revision uint64) error {
	folder, _, err := s.GetFolder(ctx, projectUID, folderUID)
	if err != nil {
		return err
	}

	if err = s.folders().deleteAt(ctx, folderUID, revision); err != nil {
		return err
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

// ListFolders lists project folders. When projectUID is empty, all folders are returned.
func (s *NatsRepository) ListFolders(ctx context.Context, projectUID string) ([]*models.ProjectFolder, error) {
	return s.folders().list(ctx, func(folder *models.ProjectFolder) bool {
		return projectUID == "" || folder.ProjectUID == projectUID
	})
}

// UpdateFolder persists folder metadata with optimistic concurrency.
func (s *NatsRepository) UpdateFolder(ctx context.Context, folder *models.ProjectFolder, revision uint64) error {
	return s.folders().update(ctx, folder.UID, folder, revision)
}
