// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
)

const (
	// MaxDocumentFileSize is the maximum allowed file size for document uploads (10MB).
	MaxDocumentFileSize = 10 * 1024 * 1024
)

// AllowedDocumentContentTypes is the set of MIME types permitted for document uploads.
var AllowedDocumentContentTypes = map[string]bool{
	"application/pdf": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true, // .docx
	"application/msword": true, // .doc
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true, // .xlsx
	"application/vnd.ms-excel": true, // .xls
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true, // .pptx
	"application/vnd.ms-powerpoint":                                             true, // .ppt
	"text/plain":                                                                true,
	"text/csv":                                                                  true,
	"image/png":                                                                 true,
	"image/jpeg":                                                                true,
	"image/gif":                                                                 true,
	"application/zip":                                                           true,
}

// ProjectDocument represents a file attachment associated with a project.
// Metadata is stored in NATS KV; file data is stored in NATS Object Store.
type ProjectDocument struct {
	UID                string    `json:"uid"`
	ProjectUID         string    `json:"project_uid"`
	FolderUID          *string   `json:"folder_uid,omitempty"`
	Name               string    `json:"name"`
	Description        string    `json:"description,omitempty"`
	FileName           string    `json:"file_name"`
	FileSize           int64     `json:"file_size"`
	ContentType        string    `json:"content_type"`
	UploadedByUsername string    `json:"uploaded_by_username,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// BuildIndexKey returns a SHA-256 hash of projectUID|name for document name uniqueness enforcement.
func (d *ProjectDocument) BuildIndexKey(_ context.Context) string {
	data := fmt.Sprintf("%s|%s", d.ProjectUID, d.Name)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// IndexingConfig returns indexing configuration for the project document.
func (d *ProjectDocument) IndexingConfig() *indexerTypes.IndexingConfig {
	if d == nil {
		return nil
	}
	return &indexerTypes.IndexingConfig{
		ObjectID:             d.UID,
		AccessCheckObject:    fmt.Sprintf("project:%s", d.ProjectUID),
		AccessCheckRelation:  "viewer",
		HistoryCheckObject:   fmt.Sprintf("project:%s", d.ProjectUID),
		HistoryCheckRelation: "auditor",
		SortName:             d.Name,
		ParentRefs:           []string{fmt.Sprintf("project:%s", d.ProjectUID)},
		Tags:                 d.Tags(),
	}
}

// Tags generates a consistent set of tags for the project document.
func (d *ProjectDocument) Tags() []string {
	if d == nil {
		return nil
	}

	var tags []string

	if d.UID != "" {
		tags = append(tags, d.UID)
		tags = append(tags, fmt.Sprintf("project_document_uid:%s", d.UID))
	}

	if d.ProjectUID != "" {
		tags = append(tags, fmt.Sprintf("project_uid:%s", d.ProjectUID))
	}

	if d.FolderUID != nil && *d.FolderUID != "" {
		tags = append(tags, fmt.Sprintf("folder_uid:%s", *d.FolderUID))
	}

	if d.ContentType != "" {
		tags = append(tags, fmt.Sprintf("content_type:%s", d.ContentType))
	}

	if d.UploadedByUsername != "" {
		tags = append(tags, fmt.Sprintf("uploaded_by:%s", d.UploadedByUsername))
	}

	return tags
}
