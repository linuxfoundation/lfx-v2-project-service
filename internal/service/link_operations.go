// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"log/slog"
	neturl "net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/log"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// CreateLink creates a new project link.
func (s *ProjectsService) CreateLink(ctx context.Context, projectUID string, name, url, description string, folderUID *string, xSync bool) (*models.ProjectLink, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return nil, domain.ErrServiceUnavailable
	}

	if name == "" {
		return nil, domain.ErrValidationFailed
	}

	parsed, parseErr := neturl.Parse(url)
	if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
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

	if folderUID != nil && *folderUID == "" {
		folderUID = nil
	}
	if folderUID != nil {
		_, _, err := s.FolderRepository.GetFolder(ctx, projectUID, *folderUID)
		if err != nil {
			if errors.Is(err, domain.ErrFolderNotFound) {
				return nil, domain.ErrFolderNotFound
			}
			return nil, domain.ErrInternal
		}
	}

	principal, _ := ctx.Value(constants.PrincipalContextID).(string)
	now := time.Now().UTC()
	link := &models.ProjectLink{
		UID:               uuid.NewString(),
		ProjectUID:        projectUID,
		FolderUID:         folderUID,
		Name:              name,
		URL:               url,
		Description:       description,
		CreatedByUsername: principal,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.LinkRepository.CreateLink(ctx, link); err != nil {
		return nil, err
	}

	msg := indexerTypes.IndexerMessageEnvelope{
		Action:         indexerConstants.ActionCreated,
		Data:           *link,
		IndexingConfig: link.IndexingConfig(),
	}
	if xSync {
		if err := s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectLinkSubject, msg, true); err != nil {
			slog.WarnContext(ctx, "error sending link indexer message", constants.ErrKey, err)
			return nil, err
		}
	} else {
		bgCtx := context.WithoutCancel(ctx)
		go func() {
			if err := s.MessageBuilder.SendIndexerMessage(bgCtx, constants.IndexProjectLinkSubject, msg, false); err != nil {
				slog.WarnContext(bgCtx, "error sending link indexer message", constants.ErrKey, err)
			}
		}()
	}

	bgCtx := context.WithoutCancel(ctx)
	go func() {
		sendCtx, cancel := context.WithTimeout(bgCtx, notificationTimeout)
		defer cancel()
		if err := s.MessageBuilder.SendProjectEventMessage(sendCtx, constants.ProjectLinkCreatedSubject, DomainLinkToEvent(link)); err != nil {
			slog.WarnContext(sendCtx, "error sending link created event", constants.ErrKey, err)
		}
	}()

	return link, nil
}

// GetLink retrieves a project link.
func (s *ProjectsService) GetLink(ctx context.Context, projectUID, linkUID string) (*models.ProjectLink, string, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return nil, "", domain.ErrServiceUnavailable
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", projectUID))
	ctx = log.AppendCtx(ctx, slog.String("link_uid", linkUID))

	link, revision, err := s.LinkRepository.GetLink(ctx, projectUID, linkUID)
	if err != nil {
		if errors.Is(err, domain.ErrLinkNotFound) {
			return nil, "", domain.ErrLinkNotFound
		}
		slog.ErrorContext(ctx, "error getting link", constants.ErrKey, err)
		return nil, "", domain.ErrInternal
	}

	revisionStr := strconv.FormatUint(revision, 10)
	return link, revisionStr, nil
}

// DeleteLink deletes a project link with optimistic concurrency.
func (s *ProjectsService) DeleteLink(ctx context.Context, projectUID, linkUID string, ifMatch *string, xSync bool) error {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return domain.ErrServiceUnavailable
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", projectUID))
	ctx = log.AppendCtx(ctx, slog.String("link_uid", linkUID))

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
		_, revision, err = s.LinkRepository.GetLink(ctx, projectUID, linkUID)
		if err != nil {
			return err
		}
	}

	if err := s.LinkRepository.DeleteLink(ctx, projectUID, linkUID, revision); err != nil {
		return err
	}

	deleteMsg := indexerTypes.IndexerMessageEnvelope{
		Action: indexerConstants.ActionDeleted,
		Data:   linkUID,
		IndexingConfig: (&models.ProjectLink{
			UID:        linkUID,
			ProjectUID: projectUID,
		}).IndexingConfig(),
	}
	if xSync {
		if err := s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectLinkSubject, deleteMsg, true); err != nil {
			slog.WarnContext(ctx, "error sending link delete indexer message", constants.ErrKey, err)
			return err
		}
	} else {
		bgCtx := context.WithoutCancel(ctx)
		go func() {
			if err := s.MessageBuilder.SendIndexerMessage(bgCtx, constants.IndexProjectLinkSubject, deleteMsg, false); err != nil {
				slog.WarnContext(bgCtx, "error sending link delete indexer message", constants.ErrKey, err)
			}
		}()
	}

	return nil
}
