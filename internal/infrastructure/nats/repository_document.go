// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
)

// documentRecoveryTimeout bounds the cleanup that follows an ambiguous write.
const documentRecoveryTimeout = 5 * time.Second

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

// UpdateDocumentMetadata replaces a document's metadata when the revision is still current. A changed name moves the
// (project, name) uniqueness lookup key with it: the new name is reserved first, so a name another document holds is
// ErrDocumentNameExists and changes nothing, and the old name is released only once the record is written. A revision
// that is no longer current is ErrRevisionMismatch before anything is reserved.
//
// Each reservation carries a token only this call knows, so a read-back can tell the key it created from one that
// merely holds the same document uid. The old name's key is read before the write and released by the revision it had
// then, so a key that was released and taken again in the meantime is left alone. The record's revision is checked
// again after that read, so the key read is known to belong to the record as the caller saw it. A write that returned
// an error but is found, byte for byte, to have landed is a success, for a rename and for an update that keeps the
// name: the caller gets the record and the index is updated as after any update.
func (s *NatsRepository) UpdateDocumentMetadata(ctx context.Context, doc *models.ProjectDocument, revision uint64) error {
	data, err := json.Marshal(doc)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling document into JSON", constants.ErrKey, err, "document_uid", doc.UID)
		return domain.ErrInternal
	}
	attrs := []any{"document_uid", doc.UID, "project_uid", doc.ProjectUID}

	// The record as it is stored tells which name the document holds the lookup key for.
	current, currentRevision, err := s.documents().get(ctx, doc.UID)
	if err != nil {
		return err
	}
	if currentRevision != revision {
		return domain.ErrRevisionMismatch
	}

	oldKey := fmt.Sprintf(constants.KVLookupDocumentPrefix, current.BuildIndexKey(ctx))
	newKey := fmt.Sprintf(constants.KVLookupDocumentPrefix, doc.BuildIndexKey(ctx))
	renamed := newKey != oldKey

	var oldRev, reserved uint64
	if renamed {
		// Read before anything is reserved, so a failure here changes nothing. A key that is gone, or that another
		// document holds, is not this document's to release (oldRev stays 0).
		oldEntry, errOld := s.Documents.Get(ctx, oldKey)
		switch {
		case errOld == nil:
			if lookupHeldBy(string(oldEntry.Value()), doc.UID) {
				oldRev = oldEntry.Revision()
			}
		case !errors.Is(errOld, jetstream.ErrKeyNotFound):
			slog.ErrorContext(ctx, "error reading the document's current name key", append([]any{constants.ErrKey, errOld}, attrs...)...)
			return domain.ErrInternal
		}
		// Another rename of this document may have changed the key since the record was read, and a reservation of
		// its own can hold the document's uid too. A record still at the caller's revision means the key was read
		// while it held the name the caller saw; otherwise nothing has been reserved and the caller retries.
		recheck, errRecheck := s.Documents.Get(ctx, doc.UID)
		if errRecheck != nil {
			if errors.Is(errRecheck, jetstream.ErrKeyNotFound) {
				return domain.ErrDocumentNotFound
			}
			slog.ErrorContext(ctx, "error reading the document before reserving its new name", append([]any{constants.ErrKey, errRecheck}, attrs...)...)
			return domain.ErrInternal
		}
		if recheck.Revision() != revision {
			return domain.ErrRevisionMismatch
		}

		reservation := fmt.Sprintf("%s#%s", doc.UID, uuid.NewString())
		rev, errReserve := s.Documents.Create(ctx, newKey, []byte(reservation))
		if errReserve != nil {
			if isJetStreamCASConflict(errReserve) {
				// A key that holds this document's own uid is not adopted: two renames of the same document to the
				// same name would share the reservation, and the one that loses would release the key the winner uses.
				return domain.ErrDocumentNameExists
			}
			// Not a refusal, so the reservation may have been stored with its reply lost: look for the key this call made.
			s.settleDocumentReservation(ctx, newKey, reservation, errReserve, attrs)
			slog.ErrorContext(ctx, "error reserving document name in NATS KV store", append([]any{constants.ErrKey, errReserve}, attrs...)...)
			return domain.ErrInternal
		}
		reserved = rev
	}

	if renamed {
		// A request that has ended since the name was reserved never sends the write, so nothing can land and the
		// reservation is nobody's. (An error returned after the write is sent is ambiguous and is settled below.)
		if errCtx := ctx.Err(); errCtx != nil {
			s.releaseDocumentKey(ctx, newKey, reserved, errCtx, attrs)
			slog.WarnContext(ctx, "request ended before the document write was sent; released the new name", append([]any{constants.ErrKey, errCtx}, attrs...)...)
			return domain.ErrInternal
		}
	}

	if _, errUpdate := s.Documents.Update(ctx, doc.UID, data, revision); errUpdate != nil {
		conflict := isJetStreamCASConflict(errUpdate)
		switch {
		case !renamed:
			// No key was reserved, so there is nothing to settle but the outcome itself.
			if !conflict && s.documentWriteLanded(ctx, doc, data, errUpdate) {
				return nil
			}
		case conflict:
			// Refused outright, so the record keeps its old name and the one reserved for it is nobody's. It is
			// released by the revision it was reserved at, so a key another writer took over is left alone.
			s.releaseDocumentKey(ctx, newKey, reserved, errUpdate, attrs)
		case s.settleDocumentRename(ctx, doc, data, revision, oldKey, newKey, oldRev, reserved, errUpdate):
			return nil
		}
		if conflict {
			slog.WarnContext(ctx, "revision mismatch updating document", append([]any{constants.ErrKey, errUpdate}, attrs...)...)
			return domain.ErrRevisionMismatch
		}
		slog.ErrorContext(ctx, "error updating document in NATS KV store", append([]any{constants.ErrKey, errUpdate}, attrs...)...)
		return domain.ErrInternal
	}

	if renamed {
		s.releaseOldDocumentName(ctx, oldKey, oldRev, attrs)
	}
	return nil
}

// lookupHeldBy reports whether a lookup key's value says uid holds it: the bare uid an upload writes, or the uid
// followed by the per-attempt token a rename adds to its reservation.
func lookupHeldBy(value, uid string) bool {
	return value == uid || strings.HasPrefix(value, uid+"#")
}

// releaseOldDocumentName releases the name a record has left, by the revision its key had before the write. A key that
// was not the document's (oldRev 0) or that changed hands since is left alone.
func (s *NatsRepository) releaseOldDocumentName(ctx context.Context, oldKey string, oldRev uint64, attrs []any) {
	if oldRev == 0 {
		return
	}
	s.releaseDocumentKey(ctx, oldKey, oldRev, nil, attrs)
}

// releaseDocumentKey purges a lookup key by the revision it had, on a context detached from the request so a cancelled
// request still releases it. A key that changed hands since is left alone, and a failed purge is logged.
func (s *NatsRepository) releaseDocumentKey(ctx context.Context, key string, revision uint64, cause error, attrs []any) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), documentRecoveryTimeout)
	defer cancel()
	s.purgeDocumentKey(ctx, key, cause, attrs, jetstream.LastRevision(revision))
}

// purgeDocumentKey purges a lookup key and logs a failure. A purge guarded by a revision that no longer matches means
// the key is not the caller's any more, which is left as it is.
func (s *NatsRepository) purgeDocumentKey(ctx context.Context, key string, cause error, attrs []any, opts ...jetstream.KVDeleteOpt) {
	errPurge := s.Documents.Purge(ctx, key, opts...)
	switch {
	case errPurge == nil, errors.Is(errPurge, jetstream.ErrKeyNotFound):
	case isJetStreamCASConflict(errPurge):
		slog.WarnContext(ctx, "document name key was taken over before it could be released; leaving it",
			append([]any{"key", key, "cause", cause}, attrs...)...)
	default:
		slog.ErrorContext(ctx, "failed to release document name key; the name stays locked until it is removed",
			append([]any{constants.ErrKey, errPurge, "key", key, "cause", cause}, attrs...)...)
	}
}

// settleDocumentReservation looks for the reservation a rename's Create may have stored before its reply was lost. The
// key is released only when it holds this call's token, by the revision it has, so a key that merely holds the same
// document uid (another rename's) is left alone. A key that is not there is nothing to release. When the key cannot be
// read the state is not known, so it is kept, which is logged.
func (s *NatsRepository) settleDocumentReservation(ctx context.Context, newKey, reservation string, createErr error, attrs []any) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), documentRecoveryTimeout)
	defer cancel()

	entry, errGet := s.Documents.Get(ctx, newKey)
	if errGet != nil {
		if !errors.Is(errGet, jetstream.ErrKeyNotFound) {
			slog.ErrorContext(ctx, "could not read the new name's key after a reservation error; if it was stored the name stays locked until it is removed",
				append([]any{constants.ErrKey, errGet, "create_error", createErr, "key", newKey}, attrs...)...)
		}
		return
	}
	if string(entry.Value()) != reservation {
		return
	}
	s.purgeDocumentKey(ctx, newKey, createErr, attrs, jetstream.LastRevision(entry.Revision()))
}

// documentWriteLanded reports whether the record holds exactly the bytes a write sent, which is the case only when that
// write landed: the stored value is what was sent, so a record changed since (an audit backfill changes users without
// touching updated_at) is not mistaken for it. A record that cannot be read is not reported as landed.
func (s *NatsRepository) documentWriteLanded(ctx context.Context, doc *models.ProjectDocument, data []byte, writeErr error) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), documentRecoveryTimeout)
	defer cancel()

	entry, errGet := s.Documents.Get(ctx, doc.UID)
	if errGet != nil {
		slog.WarnContext(ctx, "could not read the document back after an update write error; the write is reported as failed",
			constants.ErrKey, errGet, "write_error", writeErr, "document_uid", doc.UID, "project_uid", doc.ProjectUID)
		return false
	}
	return bytes.Equal(entry.Value(), data)
}

// settleDocumentRename resolves a rename whose write returned an error that is not a refusal, so it is not known
// whether the record now holds the new name. The record is read back on a context detached from the request, and the
// lookup keys follow what it holds:
//   - the new name: the write landed, so the old name is released (a record that is byte for byte what was written is a
//     success, which is returned so the caller completes the update; one changed since is not);
//   - deleted: the write can no longer apply, and the delete released only the name the record held when it read it, so
//     the old name's key and the new name's key are both released;
//   - neither name, or the old name at a later revision: the write can no longer land, since it needs the revision it
//     was sent at, so the new name's key is released by its revision, and the old name's key too when the record has
//     left it;
//   - the old name at the revision it was sent at: the write may still be on its way, and a key released now could be
//     taken by another document before it lands, so both keys are kept and the condition is logged;
//   - unreadable: the state is not known, so both keys are kept.
//
// Every release is guarded by the revision the key had, so a key that changed hands is left alone. A crash between
// reserving the new name and writing the record leaves the same lock, and nothing here can settle it.
func (s *NatsRepository) settleDocumentRename(ctx context.Context, doc *models.ProjectDocument, data []byte, revision uint64, oldKey, newKey string, oldRev, reserved uint64, writeErr error) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), documentRecoveryTimeout)
	defer cancel()
	attrs := []any{"document_uid", doc.UID, "project_uid", doc.ProjectUID}

	entry, errGet := s.Documents.Get(ctx, doc.UID)
	if errors.Is(errGet, jetstream.ErrKeyNotFound) {
		// A deleted record can never take the write: it needs the revision it was sent at, and the delete moved the key
		// past it. The delete released only the name the record held when it read it, so both captured keys are released.
		s.releaseOldDocumentName(ctx, oldKey, oldRev, attrs)
		s.releaseDocumentKey(ctx, newKey, reserved, writeErr, attrs)
		return false
	}
	if errGet != nil {
		slog.ErrorContext(ctx, "could not read the document back after a rename write error; keeping both name keys, and the one the record does not use stays locked until it is removed",
			append([]any{constants.ErrKey, errGet, "write_error", writeErr}, attrs...)...)
		return false
	}
	stored := &models.ProjectDocument{}
	if errDecode := json.Unmarshal(entry.Value(), stored); errDecode != nil {
		slog.ErrorContext(ctx, "could not decode the document after a rename write error; keeping both name keys, and the one the record does not use stays locked until it is removed",
			append([]any{constants.ErrKey, errDecode, "write_error", writeErr}, attrs...)...)
		return false
	}

	storedKey := fmt.Sprintf(constants.KVLookupDocumentPrefix, stored.BuildIndexKey(ctx))
	switch {
	case storedKey == newKey:
		s.releaseOldDocumentName(ctx, oldKey, oldRev, attrs)
		return bytes.Equal(entry.Value(), data)
	case storedKey == oldKey && entry.Revision() == revision:
		slog.ErrorContext(ctx, "a rename write errored and the record still holds the old name; keeping the new name's key because the write may still land, and it stays locked until it is removed",
			append([]any{"write_error", writeErr, "new_key", newKey}, attrs...)...)
		return false
	default:
		if storedKey != oldKey {
			s.releaseOldDocumentName(ctx, oldKey, oldRev, attrs)
		}
		s.releaseDocumentKey(ctx, newKey, reserved, writeErr, attrs)
		return false
	}
}
