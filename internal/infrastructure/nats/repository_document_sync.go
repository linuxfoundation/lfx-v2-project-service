// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

// ListFolders lists project folders. When projectUID is empty, all folders are returned.
func (s *NatsRepository) ListFolders(ctx context.Context, projectUID string) ([]*models.ProjectFolder, error) {
	return s.folders().list(ctx, func(folder *models.ProjectFolder) bool {
		return projectUID == "" || folder.ProjectUID == projectUID
	})
}

// ListAllLinks lists project links. When projectUID is non-empty, only that project's links are returned.
func (s *NatsRepository) ListAllLinks(ctx context.Context, projectUID string) ([]*models.ProjectLink, error) {
	if projectUID != "" {
		return s.ListLinks(ctx, projectUID)
	}
	return s.links().list(ctx, func(*models.ProjectLink) bool { return true })
}

// ListAllDocuments lists project document metadata. When projectUID is non-empty, only that project's documents are returned.
func (s *NatsRepository) ListAllDocuments(ctx context.Context, projectUID string) ([]*models.ProjectDocument, error) {
	return s.documents().list(ctx, func(doc *models.ProjectDocument) bool {
		return projectUID == "" || doc.ProjectUID == projectUID
	})
}

// UpdateFolder persists folder metadata with optimistic concurrency.
func (s *NatsRepository) UpdateFolder(ctx context.Context, folder *models.ProjectFolder, revision uint64) error {
	return s.folders().update(ctx, folder.UID, folder, revision)
}

// UpdateLink persists link metadata with optimistic concurrency.
func (s *NatsRepository) UpdateLink(ctx context.Context, link *models.ProjectLink, revision uint64) error {
	return s.links().update(ctx, link.UID, link, revision)
}

// UpdateDocumentMetadata persists document metadata with optimistic concurrency.
func (s *NatsRepository) UpdateDocumentMetadata(ctx context.Context, doc *models.ProjectDocument, revision uint64) error {
	return s.documents().update(ctx, doc.UID, doc, revision)
}
