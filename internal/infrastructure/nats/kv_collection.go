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
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
)

// kvCollection is a typed view over a KV bucket that holds one JSON record per UID
// next to lookup/ index keys (links, folders, document metadata). It owns the read,
// list, write and delete mechanics and maps store errors to domain errors, so the
// per-resource methods on NatsRepository only add what is specific to them: index
// keys and name reservations.
type kvCollection[T any] struct {
	kv INatsKeyValue
	// noun names the resource in log messages and in the "<noun>_uid" log attribute.
	noun string
	// notFound is the domain error returned when a record is missing.
	notFound error
	// projectUID returns the owning project of a record.
	projectUID func(*T) string
}

// get reads and decodes the record stored under uid along with its revision.
func (c kvCollection[T]) get(ctx context.Context, uid string) (*T, uint64, error) {
	entry, err := c.kv.Get(ctx, uid)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, 0, c.notFound
		}
		slog.ErrorContext(ctx, "error getting "+c.noun+" from NATS KV store", constants.ErrKey, err, c.noun+"_uid", uid)
		return nil, 0, domain.ErrInternal
	}

	record := new(T)
	if err := json.Unmarshal(entry.Value(), record); err != nil {
		slog.ErrorContext(ctx, "error unmarshalling "+c.noun+" from NATS KV store", constants.ErrKey, err, c.noun+"_uid", uid)
		return nil, 0, domain.ErrUnmarshal
	}

	return record, entry.Revision(), nil
}

// getForProject is get for a caller-supplied uid: the uid must have the shape of a
// UID and the record must belong to projectUID, otherwise it is reported as not found.
func (c kvCollection[T]) getForProject(ctx context.Context, projectUID, uid string) (*T, uint64, error) {
	// UIDs are used as raw KV keys; reject anything else so index and
	// reservation keys in the shared bucket cannot be probed.
	if !domain.IsValidUID(uid) {
		return nil, 0, c.notFound
	}

	record, revision, err := c.get(ctx, uid)
	if err != nil {
		return nil, 0, err
	}
	if c.projectUID(record) != projectUID {
		return nil, 0, c.notFound
	}

	return record, revision, nil
}

// list returns every record for which keep reports true. NATS KV ListKeys has no
// server-side prefix filtering, so this scans the whole bucket. Lookup keys are
// skipped, and a record deleted between the key listing and its read is skipped.
func (c kvCollection[T]) list(ctx context.Context, keep func(*T) bool) ([]*T, error) {
	keysLister, err := c.kv.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing "+c.noun+" keys from NATS KV store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	records := []*T{}
	for key := range keysLister.Keys() {
		if strings.HasPrefix(key, constants.KVLookupPrefix) {
			continue
		}

		record, _, err := c.get(ctx, key)
		if err != nil {
			if errors.Is(err, c.notFound) {
				continue
			}
			return nil, err
		}

		if keep(record) {
			records = append(records, record)
		}
	}

	return records, nil
}

// put stores a new record under uid.
func (c kvCollection[T]) put(ctx context.Context, uid string, record *T) error {
	data, err := json.Marshal(record)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling "+c.noun+" into JSON", constants.ErrKey, err, c.noun+"_uid", uid)
		return domain.ErrInternal
	}

	if _, err = c.kv.Put(ctx, uid, data); err != nil {
		slog.ErrorContext(ctx, "error putting "+c.noun+" into NATS KV store", constants.ErrKey, err, c.noun+"_uid", uid)
		return domain.ErrInternal
	}

	return nil
}

// update replaces the record stored under uid, failing with ErrRevisionMismatch when
// revision is no longer current.
func (c kvCollection[T]) update(ctx context.Context, uid string, record *T, revision uint64) error {
	data, err := json.Marshal(record)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling "+c.noun+" into JSON", constants.ErrKey, err, c.noun+"_uid", uid)
		return domain.ErrInternal
	}

	if _, err = c.kv.Update(ctx, uid, data, revision); err != nil {
		if isJetStreamCASConflict(err) {
			slog.WarnContext(ctx, "revision mismatch updating "+c.noun, constants.ErrKey, err, c.noun+"_uid", uid)
			return domain.ErrRevisionMismatch
		}
		slog.ErrorContext(ctx, "error updating "+c.noun+" in NATS KV store", constants.ErrKey, err, c.noun+"_uid", uid)
		return domain.ErrInternal
	}

	return nil
}

// deleteAt removes the record stored under uid, failing with ErrRevisionMismatch when
// revision is no longer current.
func (c kvCollection[T]) deleteAt(ctx context.Context, uid string, revision uint64) error {
	if err := c.kv.Delete(ctx, uid, jetstream.LastRevision(revision)); err != nil {
		if isJetStreamCASConflict(err) {
			slog.WarnContext(ctx, "revision mismatch deleting "+c.noun, constants.ErrKey, err, c.noun+"_uid", uid)
			return domain.ErrRevisionMismatch
		}
		slog.ErrorContext(ctx, "error deleting "+c.noun+" from NATS KV store", constants.ErrKey, err, c.noun+"_uid", uid)
		return domain.ErrInternal
	}

	return nil
}
