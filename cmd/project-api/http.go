// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

// maxTextPartSize caps the bytes read for text multipart fields (name, description, folder_uid).
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
				return fmt.Errorf("field 'name' exceeds maximum length of %d bytes", maxTextPartSize)
			}
			payload.Name = string(data)

		case "description":
			data, err := io.ReadAll(io.LimitReader(part, maxTextPartSize+1))
			if err != nil {
				return err
			}
			if int64(len(data)) > maxTextPartSize {
				return fmt.Errorf("field 'description' exceeds maximum length of %d bytes", maxTextPartSize)
			}
			s := string(data)
			payload.Description = &s

		case "folder_uid":
			data, err := io.ReadAll(io.LimitReader(part, maxTextPartSize+1))
			if err != nil {
				return err
			}
			if int64(len(data)) > maxTextPartSize {
				return fmt.Errorf("field 'folder_uid' exceeds maximum length of %d bytes", maxTextPartSize)
			}
			if s := string(data); s != "" {
				payload.FolderUID = &s
			}

		case "file":
			payload.FileName = part.FileName()

			contentType := part.Header.Get("Content-Type")
			if ct, _, err := mime.ParseMediaType(contentType); err == nil {
				contentType = ct
			}
			payload.ContentType = contentType

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
