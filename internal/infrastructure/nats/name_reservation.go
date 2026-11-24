// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
)

// nameRecoveryTimeout bounds the cleanup that follows an ambiguous write.
const nameRecoveryTimeout = 5 * time.Second

// namedCollection is a kvCollection whose records are named uniquely within their project, the name being held by a
// lookup key in the same bucket (lookup/<kind>/<sha256(projectUID|name)>). It owns the update of such a record, which
// moves the lookup key when the name changes.
type namedCollection[T any] struct {
	kvCollection[T]
	// lookupFormat formats a record's name hash into its lookup key.
	lookupFormat string
	// uid returns a record's UID.
	uid func(*T) string
	// indexKey returns the hash of a record's project and name.
	indexKey func(context.Context, *T) string
	// nameExists is returned when another record holds the new name.
	nameExists error
}

// update replaces a record when the revision is still current. A changed name moves the (project, name) uniqueness
// lookup key with it: the new name is reserved first, so a name another record holds is nameExists and changes nothing,
// and the old name is released only once the record is written. A revision that is no longer current is
// ErrRevisionMismatch before anything is reserved.
//
// Each reservation carries a token only this call knows, so a read-back can tell the key it created from one that
// merely holds the same record uid. The old name's key is read before the write and released by the revision it had
// then, so a key that was released and taken again in the meantime is left alone. The record's revision is checked
// again after that read, so the key read is known to belong to the record as the caller saw it. A write that returned
// an error but is found, byte for byte, to have landed is a success, for a rename and for an update that keeps the
// name: the caller gets the record and the index is updated as after any update.
func (c namedCollection[T]) update(ctx context.Context, rec *T, revision uint64) error {
	uid := c.uid(rec)
	data, err := json.Marshal(rec)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling "+c.noun+" into JSON", constants.ErrKey, err, c.noun+"_uid", uid)
		return domain.ErrInternal
	}
	attrs := c.attrs(rec)

	// The record as it is stored tells which name it holds the lookup key for.
	current, currentRevision, err := c.get(ctx, uid)
	if err != nil {
		return err
	}
	if currentRevision != revision {
		return domain.ErrRevisionMismatch
	}

	oldKey := fmt.Sprintf(c.lookupFormat, c.indexKey(ctx, current))
	newKey := fmt.Sprintf(c.lookupFormat, c.indexKey(ctx, rec))
	renamed := newKey != oldKey

	var oldRev, reserved uint64
	if renamed {
		// Read before anything is reserved, so a failure here changes nothing. A key that is gone, or that another
		// record holds, is not this record's to release (oldRev stays 0).
		oldEntry, errOld := c.kv.Get(ctx, oldKey)
		switch {
		case errOld == nil:
			if lookupHeldBy(string(oldEntry.Value()), uid) {
				oldRev = oldEntry.Revision()
			}
		case !errors.Is(errOld, jetstream.ErrKeyNotFound):
			slog.ErrorContext(ctx, "error reading the "+c.noun+"'s current name key", append([]any{constants.ErrKey, errOld}, attrs...)...)
			return domain.ErrInternal
		}
		// Another rename of this record may have changed the key since the record was read, and a reservation of its
		// own can hold the record's uid too. A record still at the caller's revision means the key was read while it
		// held the name the caller saw; otherwise nothing has been reserved and the caller retries.
		recheck, errRecheck := c.kv.Get(ctx, uid)
		if errRecheck != nil {
			if errors.Is(errRecheck, jetstream.ErrKeyNotFound) {
				return c.notFound
			}
			slog.ErrorContext(ctx, "error reading the "+c.noun+" before reserving its new name", append([]any{constants.ErrKey, errRecheck}, attrs...)...)
			return domain.ErrInternal
		}
		if recheck.Revision() != revision {
			return domain.ErrRevisionMismatch
		}

		reservation := fmt.Sprintf("%s#%s", uid, uuid.NewString())
		rev, errReserve := c.kv.Create(ctx, newKey, []byte(reservation))
		if errReserve != nil {
			if isJetStreamCASConflict(errReserve) {
				// A key that holds this record's own uid is not adopted: two renames of the same record to the same
				// name would share the reservation, and the one that loses would release the key the winner uses.
				return c.nameExists
			}
			// Not a refusal, so the reservation may have been stored with its reply lost: look for the key this call made.
			c.settleReservation(ctx, newKey, reservation, errReserve, attrs)
			slog.ErrorContext(ctx, "error reserving "+c.noun+" name in NATS KV store", append([]any{constants.ErrKey, errReserve}, attrs...)...)
			return domain.ErrInternal
		}
		reserved = rev
	}

	if renamed {
		// A request that has ended since the name was reserved never sends the write, so nothing can land and the
		// reservation is nobody's. (An error returned after the write is sent is ambiguous and is settled below.)
		if errCtx := ctx.Err(); errCtx != nil {
			c.releaseKey(ctx, newKey, reserved, errCtx, attrs)
			slog.WarnContext(ctx, "request ended before the "+c.noun+" write was sent; released the new name", append([]any{constants.ErrKey, errCtx}, attrs...)...)
			return domain.ErrInternal
		}
	}

	if _, errUpdate := c.kv.Update(ctx, uid, data, revision); errUpdate != nil {
		conflict := isJetStreamCASConflict(errUpdate)
		switch {
		case !renamed:
			// No key was reserved, so there is nothing to settle but the outcome itself.
			if !conflict && c.writeLanded(ctx, rec, data, errUpdate) {
				return nil
			}
		case conflict:
			// Refused outright, so the record keeps its old name and the one reserved for it is nobody's. It is
			// released by the revision it was reserved at, so a key another writer took over is left alone.
			c.releaseKey(ctx, newKey, reserved, errUpdate, attrs)
		case c.settleRename(ctx, rec, data, revision, oldKey, newKey, oldRev, reserved, errUpdate):
			return nil
		}
		if conflict {
			slog.WarnContext(ctx, "revision mismatch updating "+c.noun, append([]any{constants.ErrKey, errUpdate}, attrs...)...)
			return domain.ErrRevisionMismatch
		}
		slog.ErrorContext(ctx, "error updating "+c.noun+" in NATS KV store", append([]any{constants.ErrKey, errUpdate}, attrs...)...)
		return domain.ErrInternal
	}

	if renamed {
		c.releaseOldName(ctx, oldKey, oldRev, attrs)
	}
	return nil
}

// attrs are the log attributes that identify a record.
func (c namedCollection[T]) attrs(rec *T) []any {
	return []any{c.noun + "_uid", c.uid(rec), "project_uid", c.projectUID(rec)}
}

// lookupHeldBy reports whether a lookup key's value says uid holds it: the bare uid an upload or create writes, or the
// uid followed by the per-attempt token a rename adds to its reservation.
func lookupHeldBy(value, uid string) bool {
	return value == uid || strings.HasPrefix(value, uid+"#")
}

// releaseOldName releases the name a record has left, by the revision its key had before the write. A key that was not
// the record's (oldRev 0) or that changed hands since is left alone.
func (c namedCollection[T]) releaseOldName(ctx context.Context, oldKey string, oldRev uint64, attrs []any) {
	if oldRev == 0 {
		return
	}
	c.releaseKey(ctx, oldKey, oldRev, nil, attrs)
}

// releaseKey purges a lookup key by the revision it had, on a context detached from the request so a cancelled request
// still releases it. A key that changed hands since is left alone, and a failed purge is logged.
func (c namedCollection[T]) releaseKey(ctx context.Context, key string, revision uint64, cause error, attrs []any) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), nameRecoveryTimeout)
	defer cancel()
	c.purgeKey(ctx, key, cause, attrs, jetstream.LastRevision(revision))
}

// purgeKey purges a lookup key and logs a failure. A purge guarded by a revision that no longer matches means the key
// is not the caller's any more, which is left as it is.
func (c namedCollection[T]) purgeKey(ctx context.Context, key string, cause error, attrs []any, opts ...jetstream.KVDeleteOpt) {
	errPurge := c.kv.Purge(ctx, key, opts...)
	switch {
	case errPurge == nil, errors.Is(errPurge, jetstream.ErrKeyNotFound):
	case isJetStreamCASConflict(errPurge):
		slog.WarnContext(ctx, c.noun+" name key was taken over before it could be released; leaving it",
			append([]any{"key", key, "cause", cause}, attrs...)...)
	default:
		slog.ErrorContext(ctx, "failed to release "+c.noun+" name key; the name stays locked until it is removed",
			append([]any{constants.ErrKey, errPurge, "key", key, "cause", cause}, attrs...)...)
	}
}

// settleReservation looks for the reservation a rename's Create may have stored before its reply was lost. The key is
// released only when it holds this call's token, by the revision it has, so a key that merely holds the same record uid
// (another rename's) is left alone. A key that is not there is nothing to release. When the key cannot be read the
// state is not known, so it is kept, which is logged.
func (c namedCollection[T]) settleReservation(ctx context.Context, newKey, reservation string, createErr error, attrs []any) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), nameRecoveryTimeout)
	defer cancel()

	entry, errGet := c.kv.Get(ctx, newKey)
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
	c.purgeKey(ctx, newKey, createErr, attrs, jetstream.LastRevision(entry.Revision()))
}

// writeLanded reports whether the record holds exactly the bytes a write sent, which is the case only when that write
// landed: the stored value is what was sent, so a record changed since (an audit backfill changes users without
// touching updated_at) is not mistaken for it. A record that cannot be read is not reported as landed.
func (c namedCollection[T]) writeLanded(ctx context.Context, rec *T, data []byte, writeErr error) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), nameRecoveryTimeout)
	defer cancel()

	entry, errGet := c.kv.Get(ctx, c.uid(rec))
	if errGet != nil {
		slog.WarnContext(ctx, "could not read the "+c.noun+" back after an update write error; the write is reported as failed",
			append([]any{constants.ErrKey, errGet, "write_error", writeErr}, c.attrs(rec)...)...)
		return false
	}
	return bytes.Equal(entry.Value(), data)
}

// settleRename resolves a rename whose write returned an error that is not a refusal, so it is not known whether the
// record now holds the new name. The record is read back on a context detached from the request, and the lookup keys
// follow what it holds:
//   - the new name: the write landed, so the old name is released (a record that is byte for byte what was written is a
//     success, which is returned so the caller completes the update; one changed since is not);
//   - deleted: the write can no longer apply, and the delete released only the name the record held when it read it, so
//     the old name's key and the new name's key are both released;
//   - neither name, or the old name at a later revision: the write can no longer land, since it needs the revision it
//     was sent at, so the new name's key is released by its revision, and the old name's key too when the record has
//     left it;
//   - the old name at the revision it was sent at: the write may still be on its way, and a key released now could be
//     taken by another record before it lands, so both keys are kept and the condition is logged;
//   - unreadable, or older than the revision the write was sent at (a stale read from a lagging replica): the state is
//     not known, so both keys are kept.
//
// Every release is guarded by the revision the key had, so a key that changed hands is left alone. A crash between
// reserving the new name and writing the record leaves the same lock, and nothing here can settle it.
func (c namedCollection[T]) settleRename(ctx context.Context, rec *T, data []byte, revision uint64, oldKey, newKey string, oldRev, reserved uint64, writeErr error) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), nameRecoveryTimeout)
	defer cancel()
	attrs := c.attrs(rec)

	entry, errGet := c.kv.Get(ctx, c.uid(rec))
	if errors.Is(errGet, jetstream.ErrKeyNotFound) {
		// A deleted record can never take the write: it needs the revision it was sent at, and the delete moved the key
		// past it. The delete released only the name the record held when it read it, so both captured keys are released.
		c.releaseOldName(ctx, oldKey, oldRev, attrs)
		c.releaseKey(ctx, newKey, reserved, writeErr, attrs)
		return false
	}
	if errGet != nil {
		slog.ErrorContext(ctx, "could not read the "+c.noun+" back after a rename write error; keeping both name keys, and the one the record does not use stays locked until it is removed",
			append([]any{constants.ErrKey, errGet, "write_error", writeErr}, attrs...)...)
		return false
	}
	if entry.Revision() < revision {
		// The record was read at this revision before the write was sent, and revisions only grow, so an older one is
		// a stale read (KV reads may be served by a replica that lags the leader). It says nothing about whether the
		// pending write can still apply, so both keys are kept, as for a record that cannot be read.
		slog.ErrorContext(ctx, "the "+c.noun+" read back after a rename write error is older than the revision the write was sent at; keeping both name keys, and the one the record does not use stays locked until it is removed",
			append([]any{"write_error", writeErr, "read_revision", entry.Revision(), "write_revision", revision}, attrs...)...)
		return false
	}
	stored := new(T)
	if errDecode := json.Unmarshal(entry.Value(), stored); errDecode != nil {
		slog.ErrorContext(ctx, "could not decode the "+c.noun+" after a rename write error; keeping both name keys, and the one the record does not use stays locked until it is removed",
			append([]any{constants.ErrKey, errDecode, "write_error", writeErr}, attrs...)...)
		return false
	}

	storedKey := fmt.Sprintf(c.lookupFormat, c.indexKey(ctx, stored))
	switch {
	case storedKey == newKey:
		c.releaseOldName(ctx, oldKey, oldRev, attrs)
		return bytes.Equal(entry.Value(), data)
	case storedKey == oldKey && entry.Revision() == revision:
		slog.ErrorContext(ctx, "a rename write errored and the record still holds the old name; keeping the new name's key because the write may still land, and it stays locked until it is removed",
			append([]any{"write_error", writeErr, "new_key", newKey}, attrs...)...)
		return false
	default:
		if storedKey != oldKey {
			c.releaseOldName(ctx, oldKey, oldRev, attrs)
		}
		c.releaseKey(ctx, newKey, reserved, writeErr, attrs)
		return false
	}
}
