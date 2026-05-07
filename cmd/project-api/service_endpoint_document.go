// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

func toServiceDocument(d *models.ProjectDocument) *projsvc.ProjectDocument {
	if d == nil {
		return nil
	}
	doc := &projsvc.ProjectDocument{
		UID:        &d.UID,
		ProjectUID: &d.ProjectUID,
		FolderUID:  d.FolderUID,
		Name:       &d.Name,
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
func (s *ProjectsAPI) DownloadProjectDocument(ctx context.Context, payload *projsvc.DownloadProjectDocumentPayload) (io.ReadCloser, error) {
	fileData, doc, err := s.service.GetDocumentFile(ctx, payload.UID, payload.DocumentUID)
	if err != nil {
		return nil, handleError(err)
	}

	return &documentDownloadBody{
		data:        fileData,
		contentType: doc.ContentType,
		fileName:    doc.FileName,
	}, nil
}

// documentDownloadBody is an io.ReadCloser that also implements io.WriterTo.
// Goa calls WriteTo(w) with the http.ResponseWriter when SkipResponseBodyEncodeDecode
// is set, so headers are written before the body without touching generated code.
type documentDownloadBody struct {
	data        []byte
	contentType string
	fileName    string
	offset      int
}

func (b *documentDownloadBody) Read(p []byte) (n int, err error) {
	if b.offset >= len(b.data) {
		return 0, io.EOF
	}
	n = copy(p, b.data[b.offset:])
	b.offset += n
	return n, nil
}

func (b *documentDownloadBody) Close() error { return nil }

func (b *documentDownloadBody) WriteTo(w io.Writer) (int64, error) {
	if hw, ok := w.(http.ResponseWriter); ok {
		if b.contentType != "" {
			hw.Header().Set("Content-Type", b.contentType)
		}
		if b.fileName != "" {
			safeName := strings.Map(func(r rune) rune {
				if r == '"' || r == '\\' || r == '\n' || r == '\r' {
					return '_'
				}
				return r
			}, b.fileName)
			hw.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
		}
	}
	n, err := w.Write(b.data)
	return int64(n), err
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
