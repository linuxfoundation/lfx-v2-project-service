// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

func toServiceDocument(d *models.ProjectDocument) *projsvc.ProjectDocument {
	if d == nil {
		return nil
	}
	doc := &projsvc.ProjectDocument{
		UID:       &d.UID,
		FolderUID: d.FolderUID,
		Name:      &d.Name,
		FileName:  &d.FileName,
		FileSize:  &d.FileSize,
		CreatedAt: misc.StringPtr(d.CreatedAt.Format("2006-01-02T15:04:05Z07:00")),
		UpdatedAt: misc.StringPtr(d.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")),
	}
	if d.Description != "" {
		doc.Description = &d.Description
	}
	if d.ContentType != "" {
		doc.ContentType = &d.ContentType
	}
	if d.UploadedByUsername != "" {
		doc.UploadedByUsername = &d.UploadedByUsername
	}
	return doc
}

// UploadProjectDocument handles multipart document upload.
func (s *ProjectsAPI) UploadProjectDocument(ctx context.Context, payload *projsvc.UploadProjectDocumentPayload) (*projsvc.ProjectDocument, error) {
	xSync := false
	if payload.XSync != nil {
		xSync = *payload.XSync
	}

	description := ""
	if payload.Description != nil {
		description = *payload.Description
	}

	doc, err := s.service.UploadDocument(
		ctx,
		payload.UID,
		payload.Name,
		description,
		payload.FileName,
		payload.ContentType,
		payload.FolderUID,
		payload.File,
		xSync,
	)
	if err != nil {
		return nil, handleError(err)
	}

	return toServiceDocument(doc), nil
}

// GetProjectDocument gets project document metadata.
func (s *ProjectsAPI) GetProjectDocument(ctx context.Context, payload *projsvc.GetProjectDocumentPayload) (*projsvc.GetProjectDocumentResult, error) {
	doc, etag, err := s.service.GetDocumentMetadata(ctx, payload.UID, payload.DocumentUID)
	if err != nil {
		return nil, handleError(err)
	}

	return &projsvc.GetProjectDocumentResult{
		Document: toServiceDocument(doc),
		Etag:     &etag,
	}, nil
}

// DownloadProjectDocument streams the document binary.
func (s *ProjectsAPI) DownloadProjectDocument(ctx context.Context, payload *projsvc.DownloadProjectDocumentPayload) (*projsvc.DownloadProjectDocumentResult, error) {
	fileData, doc, err := s.service.GetDocumentFile(ctx, payload.UID, payload.DocumentUID)
	if err != nil {
		return nil, handleError(err)
	}

	disposition := fmt.Sprintf("attachment; filename=%q", doc.FileName)
	return &projsvc.DownloadProjectDocumentResult{
		Content:            fileData,
		ContentType:        &doc.ContentType,
		ContentDisposition: &disposition,
	}, nil
}

// DeleteProjectDocument deletes a project document.
func (s *ProjectsAPI) DeleteProjectDocument(ctx context.Context, payload *projsvc.DeleteProjectDocumentPayload) error {
	xSync := false
	if payload.XSync != nil {
		xSync = *payload.XSync
	}

	if err := s.service.DeleteDocument(ctx, payload.UID, payload.DocumentUID, payload.IfMatch, xSync); err != nil {
		return handleError(err)
	}

	return nil
}
