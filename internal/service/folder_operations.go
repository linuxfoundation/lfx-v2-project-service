// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/log"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// CreateFolder creates a new project folder, enforcing per-project name uniqueness.
func (s *ProjectsService) CreateFolder(ctx context.Context, projectUID, name string, xSync bool) (*models.ProjectFolder, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return nil, domain.ErrServiceUnavailable
	}

	if name == "" {
		return nil, domain.ErrValidationFailed
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", projectUID))

	exists, err := s.ProjectRepository.ProjectExists(ctx, projectUID)
	if err != nil {
		slog.ErrorContext(ctx, "error checking if project exists", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}
	if !exists {
		return nil, domain.ErrProjectNotFound
	}

	principal, _ := ctx.Value(constants.PrincipalContextID).(string)
	now := time.Now().UTC()
	folder := &models.ProjectFolder{
		UID:               uuid.NewString(),
		ProjectUID:        projectUID,
		Name:              name,
		CreatedByUsername: principal,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	// Reserve the unique name before writing the record so concurrent creates fail atomically.
	uniqueKey, err := s.FolderRepository.UniqueFolderName(ctx, folder)
	if err != nil {
		return nil, err
	}

	if err := s.FolderRepository.CreateFolder(ctx, folder); err != nil {
		// Roll back the name reservation on failure.
		if rbErr := s.FolderRepository.DeleteUniqueFolderName(ctx, uniqueKey); rbErr != nil {
			slog.ErrorContext(ctx, "error rolling back folder name reservation", constants.ErrKey, rbErr)
		}
		return nil, err
	}

	msg := indexerTypes.IndexerMessageEnvelope{
		Action:         indexerConstants.ActionCreated,
		Data:           *folder,
		IndexingConfig: folder.IndexingConfig(),
	}
	if xSync {
		if err := s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectLinkFolderSubject, msg, true); err != nil {
			slog.WarnContext(ctx, "error sending folder indexer message", constants.ErrKey, err)
		}
	} else {
		bgCtx := context.WithoutCancel(ctx)
		go func() {
			if err := s.MessageBuilder.SendIndexerMessage(bgCtx, constants.IndexProjectLinkFolderSubject, msg, false); err != nil {
				slog.WarnContext(bgCtx, "error sending folder indexer message", constants.ErrKey, err)
			}
		}()
	}

	return folder, nil
}

// GetFolder retrieves a project folder.
func (s *ProjectsService) GetFolder(ctx context.Context, projectUID, folderUID string) (*models.ProjectFolder, string, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return nil, "", domain.ErrServiceUnavailable
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", projectUID))
	ctx = log.AppendCtx(ctx, slog.String("folder_uid", folderUID))

	folder, revision, err := s.FolderRepository.GetFolder(ctx, projectUID, folderUID)
	if err != nil {
		if errors.Is(err, domain.ErrFolderNotFound) {
			return nil, "", domain.ErrFolderNotFound
		}
		slog.ErrorContext(ctx, "error getting folder", constants.ErrKey, err)
		return nil, "", domain.ErrInternal
	}

	revisionStr := strconv.FormatUint(revision, 10)
	return folder, revisionStr, nil
}

// ListFolders lists all folders for a project.
func (s *ProjectsService) ListFolders(ctx context.Context, projectUID string) ([]*models.ProjectFolder, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return nil, domain.ErrServiceUnavailable
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", projectUID))

	exists, err := s.ProjectRepository.ProjectExists(ctx, projectUID)
	if err != nil {
		slog.ErrorContext(ctx, "error checking if project exists", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}
	if !exists {
		return nil, domain.ErrProjectNotFound
	}

	folders, err := s.FolderRepository.ListFolders(ctx, projectUID)
	if err != nil {
		return nil, err
	}

	return folders, nil
}

// DeleteFolder deletes a project folder with optimistic concurrency.
// Returns ErrFolderNotEmpty if the folder still has links.
func (s *ProjectsService) DeleteFolder(ctx context.Context, projectUID, folderUID string, ifMatch *string, xSync bool) error {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return domain.ErrServiceUnavailable
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", projectUID))
	ctx = log.AppendCtx(ctx, slog.String("folder_uid", folderUID))

	var revision uint64
	var err error

	if !s.Config.SkipEtagValidation {
		if ifMatch == nil {
			slog.WarnContext(ctx, "If-Match header is missing")
			return domain.ErrValidationFailed
		}
		revision, err = strconv.ParseUint(*ifMatch, 10, 64)
		if err != nil {
			slog.ErrorContext(ctx, "error parsing If-Match header", constants.ErrKey, err)
			return domain.ErrValidationFailed
		}
	} else {
		_, revision, err = s.FolderRepository.GetFolder(ctx, projectUID, folderUID)
		if err != nil {
			return err
		}
	}

	// Block deletion if the folder still has links.
	links, err := s.LinkRepository.ListLinks(ctx, projectUID)
	if err != nil {
		return domain.ErrInternal
	}
	for _, l := range links {
		if l.FolderUID != nil && *l.FolderUID == folderUID {
			return domain.ErrFolderNotEmpty
		}
	}

	// Block deletion if the folder still has documents.
	docs, err := s.DocumentRepository.ListDocuments(ctx, projectUID)
	if err != nil {
		return domain.ErrInternal
	}
	for _, d := range docs {
		if d.FolderUID != nil && *d.FolderUID == folderUID {
			return domain.ErrFolderNotEmpty
		}
	}

	if err := s.FolderRepository.DeleteFolder(ctx, projectUID, folderUID, revision); err != nil {
		return err
	}

	if xSync {
		if err := s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectLinkFolderSubject, folderUID, true); err != nil {
			slog.WarnContext(ctx, "error sending folder delete indexer message", constants.ErrKey, err)
		}
	} else {
		bgCtx := context.WithoutCancel(ctx)
		go func() {
			if err := s.MessageBuilder.SendIndexerMessage(bgCtx, constants.IndexProjectLinkFolderSubject, folderUID, false); err != nil {
				slog.WarnContext(bgCtx, "error sending folder delete indexer message", constants.ErrKey, err)
			}
		}()
	}

	return nil
}
