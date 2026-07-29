// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
)

// ListFolders lists project folders. When projectUID is empty, all folders are returned.
func (s *NatsRepository) ListFolders(ctx context.Context, projectUID string) ([]*models.ProjectFolder, error) {
	if s.Folders == nil {
		return nil, domain.ErrInternal
	}
	keysLister, err := s.Folders.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing folder keys from NATS KV store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	folders := make([]*models.ProjectFolder, 0)
	for key := range keysLister.Keys() {
		if strings.HasPrefix(key, "lookup/") {
			continue
		}
		entry, err := s.getFolder(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			slog.ErrorContext(ctx, "error getting folder from NATS KV store", constants.ErrKey, err, "folder_uid", key)
			return nil, domain.ErrInternal
		}
		folder, err := s.getFolderUnmarshal(ctx, entry)
		if err != nil {
			return nil, domain.ErrUnmarshal
		}
		if projectUID != "" && folder.ProjectUID != projectUID {
			continue
		}
		folders = append(folders, folder)
	}
	return folders, nil
}

// ListAllLinks lists project links. When projectUID is non-empty, only that project's links are returned.
func (s *NatsRepository) ListAllLinks(ctx context.Context, projectUID string) ([]*models.ProjectLink, error) {
	if projectUID != "" {
		return s.ListLinks(ctx, projectUID)
	}
	if s.Links == nil {
		return nil, domain.ErrInternal
	}
	keysLister, err := s.Links.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing link keys from NATS KV store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	links := make([]*models.ProjectLink, 0)
	for key := range keysLister.Keys() {
		if strings.HasPrefix(key, "lookup/") {
			continue
		}
		entry, err := s.getLink(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			slog.ErrorContext(ctx, "error getting link from NATS KV store", constants.ErrKey, err, "link_uid", key)
			return nil, domain.ErrInternal
		}
		link, err := s.getLinkUnmarshal(ctx, entry)
		if err != nil {
			return nil, domain.ErrUnmarshal
		}
		links = append(links, link)
	}
	return links, nil
}

// ListAllDocuments lists project document metadata. When projectUID is non-empty, only that project's documents are returned.
func (s *NatsRepository) ListAllDocuments(ctx context.Context, projectUID string) ([]*models.ProjectDocument, error) {
	if s.Documents == nil {
		return nil, domain.ErrInternal
	}
	keysLister, err := s.Documents.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing document keys from NATS KV store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	docs := make([]*models.ProjectDocument, 0)
	for key := range keysLister.Keys() {
		if strings.HasPrefix(key, "lookup/") {
			continue
		}
		entry, err := s.getDocumentMetadata(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			slog.ErrorContext(ctx, "error getting document from NATS KV store", constants.ErrKey, err, "document_uid", key)
			return nil, domain.ErrInternal
		}
		doc, err := s.getDocumentMetadataUnmarshal(ctx, entry)
		if err != nil {
			return nil, domain.ErrUnmarshal
		}
		if projectUID != "" && doc.ProjectUID != projectUID {
			continue
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// UpdateFolder persists folder metadata with optimistic concurrency.
func (s *NatsRepository) UpdateFolder(ctx context.Context, folder *models.ProjectFolder, revision uint64) error {
	data, err := json.Marshal(folder)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling folder into JSON", constants.ErrKey, err)
		return domain.ErrInternal
	}
	if _, err = s.Folders.Update(ctx, folder.UID, data, revision); err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "revision mismatch updating folder", constants.ErrKey, err)
			return domain.ErrRevisionMismatch
		}
		slog.ErrorContext(ctx, "error updating folder in NATS KV store", constants.ErrKey, err)
		return domain.ErrInternal
	}
	return nil
}

// UpdateLink persists link metadata with optimistic concurrency.
func (s *NatsRepository) UpdateLink(ctx context.Context, link *models.ProjectLink, revision uint64) error {
	data, err := json.Marshal(link)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling link into JSON", constants.ErrKey, err)
		return domain.ErrInternal
	}
	if _, err = s.Links.Update(ctx, link.UID, data, revision); err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "revision mismatch updating link", constants.ErrKey, err)
			return domain.ErrRevisionMismatch
		}
		slog.ErrorContext(ctx, "error updating link in NATS KV store", constants.ErrKey, err)
		return domain.ErrInternal
	}
	return nil
}

// UpdateDocumentMetadata persists document metadata with optimistic concurrency.
func (s *NatsRepository) UpdateDocumentMetadata(ctx context.Context, doc *models.ProjectDocument, revision uint64) error {
	data, err := json.Marshal(doc)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling document into JSON", constants.ErrKey, err)
		return domain.ErrInternal
	}
	if _, err = s.Documents.Update(ctx, doc.UID, data, revision); err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "revision mismatch updating document", constants.ErrKey, err)
			return domain.ErrRevisionMismatch
		}
		slog.ErrorContext(ctx, "error updating document in NATS KV store", constants.ErrKey, err)
		return domain.ErrInternal
	}
	return nil
}
