// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
)

// UploadProjectDocument handles multipart document upload.
func (s *ProjectsAPI) UploadProjectDocument(ctx context.Context, payload *projsvc.UploadProjectDocumentPayload) (*projsvc.ProjectDocument, error) {
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
	)
	if err != nil {
		return nil, handleError(ctx, err)
	}

	return service.ConvertToServiceDocument(doc), nil
}

// GetProjectDocument gets project document metadata.
func (s *ProjectsAPI) GetProjectDocument(ctx context.Context, payload *projsvc.GetProjectDocumentPayload) (*projsvc.GetProjectDocumentResult, error) {
	doc, etag, err := s.service.GetDocumentMetadata(ctx, payload.UID, payload.DocumentUID)
	if err != nil {
		return nil, handleError(ctx, err)
	}

	return &projsvc.GetProjectDocumentResult{
		Document: service.ConvertToServiceDocument(doc),
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
			hw.Header().Set("Content-Disposition", contentDispositionHeader(b.fileName))
		}
	}
	n, err := w.Write(b.data)
	return int64(n), err
}

// contentDispositionHeader builds the attachment Content-Disposition value for
// fileName using the RFC 6266 dual form: legacy agents use the quoted filename
// parameter, modern agents prefer the UTF-8 percent-encoded filename* parameter.
func contentDispositionHeader(fileName string) string {
	// ASCII fallback: strip non-ASCII and chars illegal inside quoted-string.
	asciiFallback := strings.Map(func(r rune) rune {
		if r > 127 || r == '"' || r == '\\' || r == '\n' || r == '\r' {
			return '_'
		}
		return r
	}, fileName)
	return fmt.Sprintf(
		`attachment; filename="%s"; filename*=UTF-8''%s`,
		asciiFallback,
		url.PathEscape(fileName),
	)
}

// DeleteProjectDocument deletes a project document.
func (s *ProjectsAPI) DeleteProjectDocument(ctx context.Context, payload *projsvc.DeleteProjectDocumentPayload) error {
	if err := s.service.DeleteDocument(ctx, payload.UID, payload.DocumentUID, payload.IfMatch); err != nil {
		return handleError(ctx, err)
	}

	return nil
}
