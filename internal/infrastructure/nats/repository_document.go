// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
)

func (s *NatsRepository) documents() kvCollection[models.ProjectDocument] {
	return kvCollection[models.ProjectDocument]{
		kv:         s.Documents,
		noun:       "document",
		notFound:   domain.ErrDocumentNotFound,
		projectUID: func(d *models.ProjectDocument) string { return d.ProjectUID },
	}
}

// GetDocumentMetadata gets document metadata, verifying it belongs to the given project.
func (s *NatsRepository) GetDocumentMetadata(ctx context.Context, projectUID, documentUID string) (*models.ProjectDocument, uint64, error) {
	return s.documents().getForProject(ctx, projectUID, documentUID)
}

// GetDocumentFile retrieves the binary file content from the NATS Object Store.
func (s *NatsRepository) GetDocumentFile(ctx context.Context, documentUID string) ([]byte, error) {
	result, err := s.DocumentFiles.Get(ctx, documentUID)
	if err != nil {
		if errors.Is(err, jetstream.ErrObjectNotFound) {
			return nil, domain.ErrDocumentNotFound
		}
		slog.ErrorContext(ctx, "error getting document file from NATS object store", constants.ErrKey, err, "document_uid", documentUID)
		return nil, domain.ErrInternal
	}
	defer func() { _ = result.Close() }()

	data, err := io.ReadAll(result)
	if err != nil {
		slog.ErrorContext(ctx, "error reading document file from NATS object store", constants.ErrKey, err, "document_uid", documentUID)
		return nil, domain.ErrInternal
	}

	return data, nil
}

// ListDocuments lists all document metadata for a given project.
func (s *NatsRepository) ListDocuments(ctx context.Context, projectUID string) ([]*models.ProjectDocument, error) {
	return s.documents().list(ctx, func(doc *models.ProjectDocument) bool { return doc.ProjectUID == projectUID })
}

// CreateDocumentMetadata stores document metadata. The caller must first reserve the unique name
// via UniqueDocumentName and roll back via DeleteUniqueDocumentName on failure.
func (s *NatsRepository) CreateDocumentMetadata(ctx context.Context, doc *models.ProjectDocument) error {
	return s.documents().put(ctx, doc.UID, doc)
}

// PutDocumentFile stores binary file content in the NATS Object Store.
func (s *NatsRepository) PutDocumentFile(ctx context.Context, documentUID string, fileData []byte) error {
	meta := jetstream.ObjectMeta{Name: documentUID}
	if _, err := s.DocumentFiles.Put(ctx, meta, bytes.NewReader(fileData)); err != nil {
		slog.ErrorContext(ctx, "error putting document file into NATS object store", constants.ErrKey, err, "document_uid", documentUID)
		return domain.ErrInternal
	}
	return nil
}

// DeleteDocumentMetadata deletes document metadata with optimistic concurrency.
func (s *NatsRepository) DeleteDocumentMetadata(ctx context.Context, projectUID, documentUID string, revision uint64) error {
	doc, _, err := s.GetDocumentMetadata(ctx, projectUID, documentUID)
	if err != nil {
		return err
	}

	if err = s.documents().deleteAt(ctx, documentUID, revision); err != nil {
		return err
	}

	// Clean up the unique name lookup key (fire-and-forget; log on failure)
	uniqueKey := fmt.Sprintf(constants.KVLookupDocumentPrefix, doc.BuildIndexKey(ctx))
	if err = s.Documents.Purge(ctx, uniqueKey); err != nil {
		slog.WarnContext(ctx, "error purging document lookup key from NATS KV store", constants.ErrKey, err, "key", uniqueKey)
	}

	return nil
}

// DeleteDocumentFile removes the binary file from the NATS Object Store.
func (s *NatsRepository) DeleteDocumentFile(ctx context.Context, documentUID string) error {
	if err := s.DocumentFiles.Delete(ctx, documentUID); err != nil {
		slog.ErrorContext(ctx, "error deleting document file from NATS object store", constants.ErrKey, err, "document_uid", documentUID)
		return domain.ErrInternal
	}
	return nil
}

// UniqueDocumentName atomically reserves a per-project document name.
// Returns the lookup key on success, ErrDocumentNameExists if already taken.
func (s *NatsRepository) UniqueDocumentName(ctx context.Context, doc *models.ProjectDocument) (string, error) {
	uniqueKey := fmt.Sprintf(constants.KVLookupDocumentPrefix, doc.BuildIndexKey(ctx))
	if _, err := s.Documents.Create(ctx, uniqueKey, []byte(doc.UID)); err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return "", domain.ErrDocumentNameExists
		}
		slog.ErrorContext(ctx, "error reserving document name in NATS KV store", constants.ErrKey, err)
		return "", domain.ErrInternal
	}
	return uniqueKey, nil
}

// DeleteUniqueDocumentName releases a previously reserved document name lookup key.
func (s *NatsRepository) DeleteUniqueDocumentName(ctx context.Context, uniqueKey string) error {
	if err := s.Documents.Purge(ctx, uniqueKey); err != nil {
		slog.ErrorContext(ctx, "error purging document lookup key from NATS KV store", constants.ErrKey, err, "key", uniqueKey)
		return domain.ErrInternal
	}
	return nil
}

// ListAllDocuments lists project document metadata. When projectUID is non-empty, only that project's documents are returned.
func (s *NatsRepository) ListAllDocuments(ctx context.Context, projectUID string) ([]*models.ProjectDocument, error) {
	return s.documents().list(ctx, func(doc *models.ProjectDocument) bool {
		return projectUID == "" || doc.ProjectUID == projectUID
	})
}

// UpdateDocumentMetadata persists document metadata with optimistic concurrency.
func (s *NatsRepository) UpdateDocumentMetadata(ctx context.Context, doc *models.ProjectDocument, revision uint64) error {
	return s.documents().update(ctx, doc.UID, doc, revision)
}
