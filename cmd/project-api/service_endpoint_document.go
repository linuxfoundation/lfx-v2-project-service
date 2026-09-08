// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

// maxTextPartSize caps the bytes read for text multipart fields (name, description, folder_uid,
// file_name, content_type).
const maxTextPartSize = 4096

// uploadDocumentDecoder is the multipart decoder for the upload-project-document endpoint.
// It reads each multipart part, filling in the payload fields for name, description,
// folder_uid, and the binary file content. File parts are capped at MaxDocumentFileSize+1
// to detect oversized uploads at read time. Text fields are capped at maxTextPartSize.
func uploadDocumentDecoder(mr *multipart.Reader, p **projsvc.UploadProjectDocumentPayload) error {
	payload := *p
	if payload == nil {
		payload = &projsvc.UploadProjectDocumentPayload{}
		*p = payload
	}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		fieldName := part.FormName()
		switch fieldName {
		case "name":
			data, err := io.ReadAll(io.LimitReader(part, maxTextPartSize+1))
			if err != nil {
				return err
			}
			if int64(len(data)) > maxTextPartSize {
				slog.Warn("multipart field exceeds max size", "field", "name", "max_bytes", maxTextPartSize)
				return createResponse(http.StatusBadRequest, domain.ErrValidationFailed)
			}
			payload.Name = string(data)

		case "description":
			data, err := io.ReadAll(io.LimitReader(part, maxTextPartSize+1))
			if err != nil {
				return err
			}
			if int64(len(data)) > maxTextPartSize {
				slog.Warn("multipart field exceeds max size", "field", "description", "max_bytes", maxTextPartSize)
				return createResponse(http.StatusBadRequest, domain.ErrValidationFailed)
			}
			s := string(data)
			payload.Description = &s

		case "folder_uid":
			data, err := io.ReadAll(io.LimitReader(part, maxTextPartSize+1))
			if err != nil {
				return err
			}
			if int64(len(data)) > maxTextPartSize {
				slog.Warn("multipart field exceeds max size", "field", "folder_uid", "max_bytes", maxTextPartSize)
				return createResponse(http.StatusBadRequest, domain.ErrValidationFailed)
			}
			if s := string(data); s != "" {
				payload.FolderUID = &s
			}

		case "file_name":
			data, err := io.ReadAll(io.LimitReader(part, maxTextPartSize+1))
			if err != nil {
				return err
			}
			if int64(len(data)) > maxTextPartSize {
				slog.Warn("multipart field exceeds max size", "field", "file_name", "max_bytes", maxTextPartSize)
				return createResponse(http.StatusBadRequest, domain.ErrValidationFailed)
			}
			payload.FileName = string(data)

		case "content_type":
			data, err := io.ReadAll(io.LimitReader(part, maxTextPartSize+1))
			if err != nil {
				return err
			}
			if int64(len(data)) > maxTextPartSize {
				slog.Warn("multipart field exceeds max size", "field", "content_type", "max_bytes", maxTextPartSize)
				return createResponse(http.StatusBadRequest, domain.ErrValidationFailed)
			}
			if ct, _, err := mime.ParseMediaType(string(data)); err == nil {
				payload.ContentType = ct
			} else {
				payload.ContentType = string(data)
			}

		case "file":
			// FileName and ContentType from explicit form fields take precedence; only
			// fall back to the file part's headers if they haven't been set already.
			if payload.FileName == "" {
				payload.FileName = part.FileName()
			}
			if payload.ContentType == "" {
				contentType := part.Header.Get("Content-Type")
				if ct, _, err := mime.ParseMediaType(contentType); err == nil {
					contentType = ct
				}
				payload.ContentType = contentType
			}

			// Limit reads to MaxDocumentFileSize+1 so the service can detect oversized files.
			data, err := io.ReadAll(io.LimitReader(part, models.MaxDocumentFileSize+1))
			if err != nil {
				return err
			}
			payload.File = data
		}

		_ = part.Close()
	}

	return nil
}

func toServiceDocument(d *models.ProjectDocument) *projsvc.ProjectDocument {
	if d == nil {
		return nil
	}
	doc := &projsvc.ProjectDocument{
		UID:        &d.UID,
		ProjectUID: &d.ProjectUID,
		FolderUID:  d.FolderUID,
		Name:       &d.Name,
		FileName:   &d.FileName,
		FileSize:   &d.FileSize,
		CreatedBy:  service.ConvertUserToAPI(d.CreatedBy),
		UpdatedBy:  service.ConvertUserToAPI(d.UpdatedBy),
		CreatedAt:  misc.StringPtr(d.CreatedAt.Format("2006-01-02T15:04:05Z07:00")),
		UpdatedAt:  misc.StringPtr(d.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")),
	}
	if d.Description != "" {
		doc.Description = &d.Description
	}
	if d.ContentType != "" {
		doc.ContentType = &d.ContentType
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
		return nil, handleError(ctx, err)
	}

	return toServiceDocument(doc), nil
}

// GetProjectDocument gets project document metadata.
func (s *ProjectsAPI) GetProjectDocument(ctx context.Context, payload *projsvc.GetProjectDocumentPayload) (*projsvc.GetProjectDocumentResult, error) {
	doc, etag, err := s.service.GetDocumentMetadata(ctx, payload.UID, payload.DocumentUID)
	if err != nil {
		return nil, handleError(ctx, err)
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
		return nil, handleError(ctx, err)
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

// WriteTo is called by Goa's transport layer (server.go NewDownloadProjectDocumentHandler) after the
// no-op EncodeDownloadProjectDocumentResponse returns. Because that encoder never calls WriteHeader,
// the headers set here are guaranteed to arrive before the implicit WriteHeader(200) that Go's
// http.ResponseWriter triggers on the first w.Write call.
func (b *documentDownloadBody) WriteTo(w io.Writer) (int64, error) {
	if hw, ok := w.(http.ResponseWriter); ok {
		if b.contentType != "" {
			hw.Header().Set("Content-Type", b.contentType)
		}
		if b.fileName != "" {
			// ASCII fallback: strip non-ASCII and chars illegal inside quoted-string.
			asciiFallback := strings.Map(func(r rune) rune {
				if r > 127 || r == '"' || r == '\\' || r == '\n' || r == '\r' {
					return '_'
				}
				return r
			}, b.fileName)
			// RFC 6266 dual form: legacy agents use filename="…", modern agents prefer filename*=UTF-8''…
			hw.Header().Set("Content-Disposition", fmt.Sprintf(
				`attachment; filename="%s"; filename*=UTF-8''%s`,
				asciiFallback,
				url.PathEscape(b.fileName),
			))
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
		return handleError(ctx, err)
	}

	return nil
}
