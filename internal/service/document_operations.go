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

// UploadDocument validates and stores a new project document (metadata + binary file).
func (s *ProjectsService) UploadDocument(
	ctx context.Context,
	projectUID string,
	name, description, fileName, contentType string,
	folderUID *string,
	fileData []byte,
	xSync bool,
) (*models.ProjectDocument, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return nil, domain.ErrServiceUnavailable
	}

	if name == "" {
		return nil, domain.ErrValidationFailed
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", projectUID))

	if !models.AllowedDocumentContentTypes[contentType] {
		return nil, domain.ErrInvalidContentType
	}
	if int64(len(fileData)) > models.MaxDocumentFileSize {
		return nil, domain.ErrFileTooLarge
	}

	exists, err := s.ProjectRepository.ProjectExists(ctx, projectUID)
	if err != nil {
		slog.ErrorContext(ctx, "error checking if project exists", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}
	if !exists {
		return nil, domain.ErrProjectNotFound
	}

	if folderUID != nil && *folderUID != "" {
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
	doc := &models.ProjectDocument{
		UID:                uuid.NewString(),
		ProjectUID:         projectUID,
		FolderUID:          folderUID,
		Name:               name,
		Description:        description,
		FileName:           fileName,
		FileSize:           int64(len(fileData)),
		ContentType:        contentType,
		UploadedByUsername: principal,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	// Reserve the unique document name before writing any data.
	uniqueKey, err := s.DocumentRepository.UniqueDocumentName(ctx, doc)
	if err != nil {
		return nil, err
	}

	// Store the binary file first; metadata points to it.
	if err := s.DocumentRepository.PutDocumentFile(ctx, doc.UID, fileData); err != nil {
		if rbErr := s.DocumentRepository.DeleteUniqueDocumentName(ctx, uniqueKey); rbErr != nil {
			slog.ErrorContext(ctx, "error rolling back document name reservation", constants.ErrKey, rbErr)
		}
		return nil, err
	}

	if err := s.DocumentRepository.CreateDocumentMetadata(ctx, doc); err != nil {
		if rbErr := s.DocumentRepository.DeleteUniqueDocumentName(ctx, uniqueKey); rbErr != nil {
			slog.ErrorContext(ctx, "error rolling back document name reservation", constants.ErrKey, rbErr)
		}
		return nil, err
	}

	go func() {
		msg := indexerTypes.IndexerMessageEnvelope{
			Action:         indexerConstants.ActionCreated,
			Data:           *doc,
			IndexingConfig: doc.IndexingConfig(),
		}
		if err := s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectDocumentSubject, msg, xSync); err != nil {
			slog.WarnContext(ctx, "error sending document indexer message", constants.ErrKey, err)
		}
	}()

	return doc, nil
}

// GetDocumentMetadata retrieves document metadata.
func (s *ProjectsService) GetDocumentMetadata(ctx context.Context, projectUID, documentUID string) (*models.ProjectDocument, string, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return nil, "", domain.ErrServiceUnavailable
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", projectUID))
	ctx = log.AppendCtx(ctx, slog.String("document_uid", documentUID))

	doc, revision, err := s.DocumentRepository.GetDocumentMetadata(ctx, projectUID, documentUID)
	if err != nil {
		if errors.Is(err, domain.ErrDocumentNotFound) {
			return nil, "", domain.ErrDocumentNotFound
		}
		slog.ErrorContext(ctx, "error getting document metadata", constants.ErrKey, err)
		return nil, "", domain.ErrInternal
	}

	revisionStr := strconv.FormatUint(revision, 10)
	return doc, revisionStr, nil
}

// GetDocumentFile retrieves the binary file content for a document.
func (s *ProjectsService) GetDocumentFile(ctx context.Context, projectUID, documentUID string) ([]byte, *models.ProjectDocument, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return nil, nil, domain.ErrServiceUnavailable
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", projectUID))
	ctx = log.AppendCtx(ctx, slog.String("document_uid", documentUID))

	doc, _, err := s.DocumentRepository.GetDocumentMetadata(ctx, projectUID, documentUID)
	if err != nil {
		if errors.Is(err, domain.ErrDocumentNotFound) {
			return nil, nil, domain.ErrDocumentNotFound
		}
		slog.ErrorContext(ctx, "error getting document metadata", constants.ErrKey, err)
		return nil, nil, domain.ErrInternal
	}

	fileData, err := s.DocumentRepository.GetDocumentFile(ctx, documentUID)
	if err != nil {
		return nil, nil, err
	}

	return fileData, doc, nil
}

// DeleteDocument deletes document metadata and its binary file.
func (s *ProjectsService) DeleteDocument(ctx context.Context, projectUID, documentUID string, ifMatch *string, xSync bool) error {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return domain.ErrServiceUnavailable
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", projectUID))
	ctx = log.AppendCtx(ctx, slog.String("document_uid", documentUID))

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
		_, revision, err = s.DocumentRepository.GetDocumentMetadata(ctx, projectUID, documentUID)
		if err != nil {
			return err
		}
	}

	if err := s.DocumentRepository.DeleteDocumentMetadata(ctx, projectUID, documentUID, revision); err != nil {
		return err
	}

	// Delete the binary file fire-and-forget; failure is logged but not propagated.
	go func() {
		if err := s.DocumentRepository.DeleteDocumentFile(ctx, documentUID); err != nil {
			slog.WarnContext(ctx, "error deleting document file from object store", constants.ErrKey, err)
		}
	}()

	go func() {
		if err := s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectDocumentSubject, documentUID, xSync); err != nil {
			slog.WarnContext(ctx, "error sending document delete indexer message", constants.ErrKey, err)
		}
	}()

	return nil
}
