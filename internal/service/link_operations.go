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

// isValidLinkURL reports whether url is an absolute http(s) URL with a host.
func isValidLinkURL(url string) bool {
	parsed, err := neturl.Parse(url)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// resolveLinkFolder normalises an optional folder UID (empty means the project root) and checks
// that a given folder exists in the project. It returns ErrValidationFailed for a malformed UID,
// ErrFolderNotFound for a folder that is not in the project, and ErrInternal on a lookup failure.
func (s *ProjectsService) resolveLinkFolder(ctx context.Context, projectUID string, folderUID *string) (*string, error) {
	if folderUID == nil || *folderUID == "" {
		return nil, nil
	}
	if !domain.IsValidUID(*folderUID) {
		return nil, domain.ErrValidationFailed
	}
	if _, _, err := s.FolderRepository.GetFolder(ctx, projectUID, *folderUID); err != nil {
		if errors.Is(err, domain.ErrFolderNotFound) {
			return nil, domain.ErrFolderNotFound
		}
		slog.ErrorContext(ctx, "error getting folder", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}
	return folderUID, nil
}

// CreateLink creates a new project link.
func (s *ProjectsService) CreateLink(ctx context.Context, projectUID string, name, url, description string, folderUID *string) (*models.ProjectLink, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return nil, domain.ErrServiceUnavailable
	}

	if name == "" {
		return nil, domain.ErrValidationFailed
	}

	if !isValidLinkURL(url) {
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

	folderUID, err = s.resolveLinkFolder(ctx, projectUID, folderUID)
	if err != nil {
		return nil, err
	}

	createdBy, updatedBy := s.stampAuditUsers(ctx)
	now := time.Now().UTC()
	link := &models.ProjectLink{
		UID:         uuid.NewString(),
		ProjectUID:  projectUID,
		FolderUID:   folderUID,
		Name:        name,
		URL:         url,
		Description: description,
		CreatedBy:   createdBy,
		UpdatedBy:   updatedBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.LinkRepository.CreateLink(ctx, link); err != nil {
		return nil, err
	}

	msg := indexerTypes.IndexerMessageEnvelope{
		Action:         indexerConstants.ActionCreated,
		Data:           *link,
		IndexingConfig: link.IndexingConfig(),
	}
	s.publishIndexer(ctx, constants.IndexProjectLinkSubject, msg)

	bgCtx := context.WithoutCancel(ctx)
	go func() {
		sendCtx, cancel := context.WithTimeout(bgCtx, notificationTimeout)
		defer cancel()
		if err := s.Publisher.SendProjectEventMessage(sendCtx, constants.ProjectLinkCreatedSubject, DomainLinkToEvent(link)); err != nil {
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

	link.CreatedBy, link.UpdatedBy = s.normalizeAuditUsers(ctx, link.CreatedBy, link.UpdatedBy, "", "")

	revisionStr := strconv.FormatUint(revision, 10)
	return link, revisionStr, nil
}

// UpdateLink replaces a link's name, URL, description and folder with optimistic concurrency.
// A nil or empty folderUID moves the link to the project root. Audit creation fields are kept.
func (s *ProjectsService) UpdateLink(ctx context.Context, projectUID, linkUID string, ifMatch *string, name, url, description string, folderUID *string) (*models.ProjectLink, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return nil, domain.ErrServiceUnavailable
	}

	if name == "" || !isValidLinkURL(url) {
		return nil, domain.ErrValidationFailed
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", projectUID))
	ctx = log.AppendCtx(ctx, slog.String("link_uid", linkUID))

	link, currentRevision, err := s.LinkRepository.GetLink(ctx, projectUID, linkUID)
	if err != nil {
		if errors.Is(err, domain.ErrLinkNotFound) {
			return nil, domain.ErrLinkNotFound
		}
		slog.ErrorContext(ctx, "error getting link", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	revision, err := s.resolveRevision(ctx, ifMatch, func() (uint64, error) { return currentRevision, nil })
	if err != nil {
		return nil, err
	}

	folderUID, err = s.resolveLinkFolder(ctx, projectUID, folderUID)
	if err != nil {
		return nil, err
	}

	if _, updatedBy := s.stampAuditUsers(ctx); updatedBy != nil {
		link.UpdatedBy = updatedBy
	}
	link.Name = name
	link.URL = url
	link.Description = description
	link.FolderUID = folderUID
	link.UpdatedAt = time.Now().UTC()

	if err := s.LinkRepository.UpdateLink(ctx, link, revision); err != nil {
		if errors.Is(err, domain.ErrRevisionMismatch) {
			return nil, domain.ErrRevisionMismatch
		}
		return nil, err
	}

	msg := indexerTypes.IndexerMessageEnvelope{
		Action:         indexerConstants.ActionUpdated,
		Data:           *link,
		IndexingConfig: link.IndexingConfig(),
	}
	s.publishIndexer(ctx, constants.IndexProjectLinkSubject, msg)

	return link, nil
}

// DeleteLink deletes a project link with optimistic concurrency.
func (s *ProjectsService) DeleteLink(ctx context.Context, projectUID, linkUID string, ifMatch *string) error {
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
	s.publishIndexer(ctx, constants.IndexProjectLinkSubject, deleteMsg)
	return nil
}
