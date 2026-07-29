// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
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
