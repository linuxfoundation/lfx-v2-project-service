// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

var errSlugMismatch = errors.New("project_slug does not match old slug")

// errSlugIndexCollision indicates the new slug's lookup index already points
// at a different project than the one being renamed.
var errSlugIndexCollision = errors.New("new slug is already indexed to a different project")

// recordKV is the subset of jetstream.KeyValue needed by processKVRecord and
// processProjectRecord to read/update a record and reconcile the projects
// bucket's slug index. Narrowing it lets tests exercise the wiring with a
// small fake instead of a full jetstream.KeyValue implementation.
type recordKV interface {
	Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error)
	Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error)
	Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error)
	Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error
}

func processKVRecord(ctx context.Context, kvStore recordKV, key string, fields []string, oldSlug, newSlug string, dryRun bool) error {
	entry, err := kvStore.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	raw := make(map[string]json.RawMessage)
	if err := json.Unmarshal(entry.Value(), &raw); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return updateSlugField(ctx, kvStore, key, fields, oldSlug, newSlug, dryRun, entry, raw)
}

// updateSlugField renames oldSlug to newSlug on an already-fetched record.
// Callers that already hold the record's entry/raw JSON (e.g.
// processProjectRecord, which fetches it to inspect the current slug) pass
// them in directly to avoid a redundant re-fetch.
func updateSlugField(ctx context.Context, kvStore recordKV, key string, fields []string, oldSlug, newSlug string, dryRun bool, entry jetstream.KeyValueEntry, raw map[string]json.RawMessage) error {
	matched := false
	for _, field := range fields {
		val, ok := raw[field]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			continue
		}
		if s == oldSlug {
			matched = true
			break
		}
	}

	if !matched {
		slog.DebugContext(ctx, "no matching slug field, skipping", "key", key)
		return errSlugMismatch
	}

	slog.DebugContext(ctx, "updating record slug fields",
		"key", key,
		"fields", fields,
		"old_slug", oldSlug,
		"new_slug", newSlug,
		"dry_run", dryRun,
	)

	if dryRun {
		return nil
	}

	maxRetries := 3
	var updateErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			var err error
			entry, err = kvStore.Get(ctx, key)
			if err != nil {
				return fmt.Errorf("failed to re-fetch entry: %w", err)
			}
			raw = make(map[string]json.RawMessage)
			if err := json.Unmarshal(entry.Value(), &raw); err != nil {
				return fmt.Errorf("failed to unmarshal re-fetched entry: %w", err)
			}
			anyMatch := false
			for _, field := range fields {
				if val, ok := raw[field]; ok {
					var s string
					if json.Unmarshal(val, &s) == nil && s == oldSlug {
						anyMatch = true
						break
					}
				}
			}
			if !anyMatch {
				slog.DebugContext(ctx, "slug changed by concurrent process, skipping", "key", key)
				return errSlugMismatch
			}
		}

		newSlugJSON, _ := json.Marshal(newSlug)
		for _, field := range fields {
			if val, ok := raw[field]; ok {
				var s string
				if json.Unmarshal(val, &s) == nil && s == oldSlug {
					raw[field] = newSlugJSON
				}
			}
		}

		if _, ok := raw["updated_at"]; ok {
			ts, _ := json.Marshal(time.Now().UTC().Format(time.RFC3339Nano))
			raw["updated_at"] = ts
		}

		updated, marshalErr := json.Marshal(raw)
		if marshalErr != nil {
			return fmt.Errorf("failed to marshal updated record: %w", marshalErr)
		}

		_, updateErr = kvStore.Update(ctx, key, updated, entry.Revision())
		if updateErr == nil {
			break
		}

		if attempt < maxRetries {
			slog.WarnContext(ctx, "optimistic lock failed, retrying",
				"key", key, "attempt", attempt, "error", updateErr)
			if err := sleepWithContext(ctx, time.Duration(attempt*100)*time.Millisecond); err != nil {
				return err
			}
		}
	}

	if updateErr != nil {
		return fmt.Errorf("failed to update after %d attempts: %w", maxRetries, updateErr)
	}

	return nil
}

// processProjectRecord handles the projects bucket's per-key dispatch. Unlike
// processKVRecord (used by the other four buckets), it reconciles the slug
// index off the record's observed current slug rather than off whether this
// run performed the field write — so a record left inconsistent by a prior
// partial failure (field already renamed, index still stale) is repaired on
// rerun instead of becoming permanently invisible to the migration.
func processProjectRecord(ctx context.Context, kvStore recordKV, key string, fields []string, oldSlug, newSlug string, dryRun bool) error {
	entry, err := kvStore.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	raw := make(map[string]json.RawMessage)
	if err := json.Unmarshal(entry.Value(), &raw); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	currentSlug := ""
	for _, field := range fields {
		val, ok := raw[field]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(val, &s); err == nil {
			currentSlug = s
			break
		}
	}

	switch currentSlug {
	case oldSlug, newSlug:
		// Reserve the new index key before touching the record's field, so a
		// losing race (another writer claims "slug/<newSlug>" first) is
		// caught before the field is renamed, instead of after — which would
		// otherwise leave the field pointing at a slug now legitimately
		// owned by another project.
		if err := reserveSlugIndex(ctx, kvStore, key, newSlug, dryRun); err != nil {
			return err
		}
	default:
		slog.DebugContext(ctx, "no matching slug field, skipping", "key", key)
		return errSlugMismatch
	}

	if currentSlug == oldSlug {
		if err := updateSlugField(ctx, kvStore, key, fields, oldSlug, newSlug, dryRun, entry, raw); err != nil {
			return err
		}
	} else {
		slog.DebugContext(ctx, "record already renamed, repairing slug index", "key", key)
	}

	return deleteOldSlugIndex(ctx, kvStore, key, oldSlug, dryRun)
}

// slugIndexKV is the subset of jetstream.KeyValue used by reserveSlugIndex and deleteOldSlugIndex.
type slugIndexKV interface {
	Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error)
	Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error)
	Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error
}

// reserveSlugIndex is the authoritative check-and-claim for "slug/<newSlug>".
// In dry-run mode it performs a non-mutating Get-based check, since Create
// would actually write. In apply mode it atomically reserves the key with
// Create: if a concurrent writer won the race and created the key first,
// Create fails with ErrKeyExists and the owning uid is re-checked instead of
// blindly overwriting it.
func reserveSlugIndex(ctx context.Context, kv slugIndexKV, uid, newSlug string, dryRun bool) error {
	if dryRun {
		existing, err := kv.Get(ctx, "slug/"+newSlug)
		switch {
		case err == nil:
			if string(existing.Value()) != uid {
				return fmt.Errorf("%w: slug/%s is indexed to %q, not %q", errSlugIndexCollision, newSlug, existing.Value(), uid)
			}
			return nil
		case errors.Is(err, jetstream.ErrKeyNotFound):
			return nil
		default:
			return fmt.Errorf("failed to check slug index %q for collision: %w", newSlug, err)
		}
	}

	if _, err := kv.Create(ctx, "slug/"+newSlug, []byte(uid)); err != nil {
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("failed to create slug index %q: %w", newSlug, err)
		}
		existing, getErr := kv.Get(ctx, "slug/"+newSlug)
		if getErr != nil {
			return fmt.Errorf("failed to verify slug index %q after create conflict: %w", newSlug, getErr)
		}
		if string(existing.Value()) != uid {
			return fmt.Errorf("%w: slug/%s is indexed to %q, not %q", errSlugIndexCollision, newSlug, existing.Value(), uid)
		}
	}
	return nil
}

// deleteOldSlugIndex removes "slug/<oldSlug>" only if it still maps to uid,
// using a revision-conditional delete so a writer that reassigns the key
// between this check and the delete causes a conflict error instead of
// losing that writer's mapping.
func deleteOldSlugIndex(ctx context.Context, kv slugIndexKV, uid, oldSlug string, dryRun bool) error {
	existingOld, err := kv.Get(ctx, "slug/"+oldSlug)
	switch {
	case err == nil:
		if string(existingOld.Value()) != uid {
			slog.DebugContext(ctx, "stale slug index no longer owned by this project, leaving in place",
				"uid", uid, "key", "slug/"+oldSlug)
			return nil
		}
		if dryRun {
			slog.InfoContext(ctx, "dry run: would delete stale slug index", "uid", uid, "key", "slug/"+oldSlug)
			return nil
		}
		if err := kv.Delete(ctx, "slug/"+oldSlug, jetstream.LastRevision(existingOld.Revision())); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return fmt.Errorf("failed to delete stale slug index %q: %w", oldSlug, err)
		}
	case errors.Is(err, jetstream.ErrKeyNotFound):
		return nil
	default:
		return fmt.Errorf("failed to check slug index %q before delete: %w", oldSlug, err)
	}
	return nil
}
