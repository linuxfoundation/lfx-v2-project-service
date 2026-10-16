// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"time"
)

type fakeKVEntry struct {
	key      string
	value    []byte
	revision uint64
}

func (f fakeKVEntry) Bucket() string { return "projects" }

func (f fakeKVEntry) Key() string { return f.key }

func (f fakeKVEntry) Value() []byte { return f.value }

func (f fakeKVEntry) Revision() uint64 { return f.revision }

func (f fakeKVEntry) Created() time.Time { return time.Time{} }

func (f fakeKVEntry) Delta() uint64 { return 0 }

func (f fakeKVEntry) Operation() jetstream.KeyValueOp { return jetstream.KeyValuePut }

// fakeRecordKV implements recordKV over an in-memory map keyed by both
// record keys (e.g. project uids) and "slug/<slug>" index keys, so it can
// back processProjectRecord/processKVRecord tests without a full
// jetstream.KeyValue fake.
type fakeRecordKV struct {
	entries   map[string][]byte
	revisions map[string]uint64
}

func newFakeRecordKV(entries map[string][]byte) *fakeRecordKV {
	if entries == nil {
		entries = map[string][]byte{}
	}
	revisions := make(map[string]uint64, len(entries))
	for k := range entries {
		revisions[k] = 1
	}
	return &fakeRecordKV{entries: entries, revisions: revisions}
}

func (f *fakeRecordKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	v, ok := f.entries[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}
	return fakeKVEntry{key: key, value: v, revision: f.revisions[key]}, nil
}

func (f *fakeRecordKV) Update(_ context.Context, key string, value []byte, revision uint64) (uint64, error) {
	if f.revisions[key] != revision {
		return 0, jetstream.ErrKeyExists
	}
	f.entries[key] = value
	f.revisions[key]++
	return f.revisions[key], nil
}

func (f *fakeRecordKV) Create(_ context.Context, key string, value []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	if _, ok := f.entries[key]; ok {
		return 0, jetstream.ErrKeyExists
	}
	f.entries[key] = value
	f.revisions[key] = 1
	return 1, nil
}

func (f *fakeRecordKV) Delete(_ context.Context, key string, _ ...jetstream.KVDeleteOpt) error {
	if _, ok := f.entries[key]; !ok {
		return jetstream.ErrKeyNotFound
	}
	delete(f.entries, key)
	delete(f.revisions, key)
	return nil
}

// reconcile runs the same reserve-then-delete sequence processProjectRecord
// uses to keep the slug index in sync (reserveSlugIndex, then
// deleteOldSlugIndex).
func reconcile(ctx context.Context, kv recordKV, uid, oldSlug, newSlug string, dryRun bool) error {
	if err := reserveSlugIndex(ctx, kv, uid, newSlug, dryRun); err != nil {
		return err
	}
	return deleteOldSlugIndex(ctx, kv, uid, oldSlug, dryRun)
}

func TestReconcileProjectSlugIndex(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"
	const otherUID = "00000000-0000-0000-0000-000000000002"

	t.Run("renames the index key", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{
			"slug/old-slug": []byte(uid),
		})
		if err := reconcile(context.Background(), kv, uid, "old-slug", "new-slug", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, ok := kv.entries["slug/new-slug"]; !ok || string(got) != uid {
			t.Errorf("expected slug/new-slug to map to %q, got %q (present=%v)", uid, got, ok)
		}
		if _, ok := kv.entries["slug/old-slug"]; ok {
			t.Errorf("expected stale slug/old-slug to be deleted")
		}
	})

	t.Run("is idempotent when the old index key is already gone", func(t *testing.T) {
		kv := newFakeRecordKV(nil)
		if err := reconcile(context.Background(), kv, uid, "old-slug", "new-slug", false); err != nil {
			t.Fatalf("unexpected error re-running on an already-migrated index: %v", err)
		}
		if got, ok := kv.entries["slug/new-slug"]; !ok || string(got) != uid {
			t.Errorf("expected slug/new-slug to map to %q, got %q (present=%v)", uid, got, ok)
		}
	})

	t.Run("fails on collision with another project's index", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{
			"slug/old-slug": []byte(uid),
			"slug/new-slug": []byte(otherUID),
		})
		err := reconcile(context.Background(), kv, uid, "old-slug", "new-slug", false)
		if !errors.Is(err, errSlugIndexCollision) {
			t.Fatalf("expected errSlugIndexCollision, got %v", err)
		}
		if got := kv.entries["slug/new-slug"]; string(got) != otherUID {
			t.Errorf("expected slug/new-slug to remain untouched at %q, got %q", otherUID, got)
		}
		if got, ok := kv.entries["slug/old-slug"]; !ok || string(got) != uid {
			t.Errorf("expected slug/old-slug to remain untouched, got %q (present=%v)", got, ok)
		}
	})

	t.Run("dry run makes no writes", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{
			"slug/old-slug": []byte(uid),
		})
		if err := reconcile(context.Background(), kv, uid, "old-slug", "new-slug", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := kv.entries["slug/new-slug"]; ok {
			t.Errorf("expected no write to slug/new-slug during dry run")
		}
		if got, ok := kv.entries["slug/old-slug"]; !ok || string(got) != uid {
			t.Errorf("expected slug/old-slug to remain untouched during dry run, got %q (present=%v)", got, ok)
		}
	})

	t.Run("reserveSlugIndex catches a collision from a concurrent writer", func(t *testing.T) {
		// Simulates a concurrent writer creating slug/new-slug for another
		// project just before reserveSlugIndex's atomic Create runs.
		kv := newFakeRecordKV(map[string][]byte{
			"slug/old-slug": []byte(uid),
			"slug/new-slug": []byte(otherUID),
		})
		err := reserveSlugIndex(context.Background(), kv, uid, "new-slug", false)
		if !errors.Is(err, errSlugIndexCollision) {
			t.Fatalf("expected errSlugIndexCollision, got %v", err)
		}
		if got := kv.entries["slug/new-slug"]; string(got) != otherUID {
			t.Errorf("expected slug/new-slug to remain owned by the other project, got %q", got)
		}
	})

	t.Run("deleteOldSlugIndex does not delete the old index key if it was reassigned to another project", func(t *testing.T) {
		// Simulates a concurrent writer reusing old-slug for another project
		// after this record's field was renamed but before the index was
		// reconciled; the stale-looking slug/old-slug entry must be left in
		// place rather than deleted out from under its new owner.
		kv := newFakeRecordKV(map[string][]byte{
			"slug/old-slug": []byte(otherUID),
		})
		if err := deleteOldSlugIndex(context.Background(), kv, uid, "old-slug", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, ok := kv.entries["slug/old-slug"]; !ok || string(got) != otherUID {
			t.Errorf("expected slug/old-slug to remain owned by the other project, got %q (present=%v)", got, ok)
		}
	})
}

func TestProcessProjectRecord(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"
	const otherUID = "00000000-0000-0000-0000-000000000002"
	fields := []string{"slug"}

	t.Run("forward match renames the field and the index", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{
			uid:             []byte(`{"slug":"old-slug"}`),
			"slug/old-slug": []byte(uid),
		})
		if err := processProjectRecord(context.Background(), kv, uid, fields, "old-slug", "new-slug", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := string(kv.entries[uid]); got != `{"slug":"new-slug"}` {
			t.Errorf("expected record slug field renamed, got %s", got)
		}
		if got, ok := kv.entries["slug/new-slug"]; !ok || string(got) != uid {
			t.Errorf("expected slug/new-slug to map to %q, got %q (present=%v)", uid, got, ok)
		}
		if _, ok := kv.entries["slug/old-slug"]; ok {
			t.Errorf("expected stale slug/old-slug to be deleted")
		}
	})

	t.Run("repairs a stale index when the field is already renamed", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{
			uid:             []byte(`{"slug":"new-slug"}`),
			"slug/old-slug": []byte(uid),
		})
		if err := processProjectRecord(context.Background(), kv, uid, fields, "old-slug", "new-slug", false); err != nil {
			t.Fatalf("unexpected error repairing an already-migrated record: %v", err)
		}
		if got, ok := kv.entries["slug/new-slug"]; !ok || string(got) != uid {
			t.Errorf("expected slug/new-slug to map to %q, got %q (present=%v)", uid, got, ok)
		}
		if _, ok := kv.entries["slug/old-slug"]; ok {
			t.Errorf("expected stale slug/old-slug to be deleted")
		}
	})

	t.Run("skips a record whose slug matches neither old nor new", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{
			uid: []byte(`{"slug":"unrelated-slug"}`),
		})
		err := processProjectRecord(context.Background(), kv, uid, fields, "old-slug", "new-slug", false)
		if !errors.Is(err, errSlugMismatch) {
			t.Fatalf("expected errSlugMismatch, got %v", err)
		}
	})

	t.Run("fails without renaming the field when the index has a collision", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{
			uid:             []byte(`{"slug":"old-slug"}`),
			"slug/old-slug": []byte(uid),
			"slug/new-slug": []byte(otherUID),
		})
		err := processProjectRecord(context.Background(), kv, uid, fields, "old-slug", "new-slug", false)
		if !errors.Is(err, errSlugIndexCollision) {
			t.Fatalf("expected errSlugIndexCollision, got %v", err)
		}
		if got := string(kv.entries[uid]); got != `{"slug":"old-slug"}` {
			t.Errorf("expected record field left untouched after collision, got %s", got)
		}
		if got := kv.entries["slug/new-slug"]; string(got) != otherUID {
			t.Errorf("expected slug/new-slug to remain untouched at %q, got %q", otherUID, got)
		}
	})

	t.Run("dry run makes no writes to the record or the index", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{
			uid:             []byte(`{"slug":"old-slug"}`),
			"slug/old-slug": []byte(uid),
		})
		if err := processProjectRecord(context.Background(), kv, uid, fields, "old-slug", "new-slug", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := string(kv.entries[uid]); got != `{"slug":"old-slug"}` {
			t.Errorf("expected no write to record during dry run, got %s", got)
		}
		if _, ok := kv.entries["slug/new-slug"]; ok {
			t.Errorf("expected no write to slug/new-slug during dry run")
		}
		if _, ok := kv.entries["slug/old-slug"]; !ok {
			t.Errorf("expected slug/old-slug to remain untouched during dry run")
		}
	})
}

// conflictingKV fails the first failUpdates Update calls with a revision
// conflict, optionally rewriting the record just before the retry re-reads it.
type conflictingKV struct {
	*fakeRecordKV
	failUpdates int
	updates     int
	onConflict  func(*fakeRecordKV)
}

func (c *conflictingKV) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	c.updates++
	if c.updates <= c.failUpdates {
		if c.onConflict != nil {
			c.onConflict(c.fakeRecordKV)
		}
		return 0, jetstream.ErrKeyExists
	}
	return c.fakeRecordKV.Update(ctx, key, value, revision)
}

func TestProcessKVRecord(t *testing.T) {
	fields := []string{"project_slug"}

	t.Run("renames the field and bumps updated_at", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{
			"k": []byte(`{"project_slug":"old-slug","updated_at":"2020-01-01T00:00:00Z","other":1}`),
		})

		require.NoError(t, processKVRecord(context.Background(), kv, "k", fields, "old-slug", "new-slug", false))

		got := string(kv.entries["k"])
		assert.Contains(t, got, `"project_slug":"new-slug"`)
		assert.Contains(t, got, `"other":1`)
		assert.NotContains(t, got, "2020-01-01", "updated_at is refreshed")
	})

	t.Run("does not add updated_at to a record that has none", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{"k": []byte(`{"project_slug":"old-slug"}`)})

		require.NoError(t, processKVRecord(context.Background(), kv, "k", fields, "old-slug", "new-slug", false))

		assert.JSONEq(t, `{"project_slug":"new-slug"}`, string(kv.entries["k"]))
	})

	t.Run("a missing record is an error", func(t *testing.T) {
		err := processKVRecord(context.Background(), newFakeRecordKV(nil), "k", fields, "old-slug", "new-slug", false)
		require.ErrorContains(t, err, "failed to get entry")
	})

	t.Run("an invalid JSON record is an error", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{"k": []byte(`not json`)})
		err := processKVRecord(context.Background(), kv, "k", fields, "old-slug", "new-slug", false)
		require.ErrorContains(t, err, "failed to unmarshal JSON")
	})

	t.Run("a non-string slug field is ignored", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{"k": []byte(`{"project_slug":7}`)})
		err := processKVRecord(context.Background(), kv, "k", fields, "old-slug", "new-slug", false)
		require.ErrorIs(t, err, errSlugMismatch)
	})

	t.Run("a different slug is a mismatch and is left alone", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{"k": []byte(`{"project_slug":"other-slug"}`)})

		err := processKVRecord(context.Background(), kv, "k", fields, "old-slug", "new-slug", false)

		require.ErrorIs(t, err, errSlugMismatch)
		assert.JSONEq(t, `{"project_slug":"other-slug"}`, string(kv.entries["k"]))
	})

	t.Run("retries after an optimistic lock conflict", func(t *testing.T) {
		kv := &conflictingKV{fakeRecordKV: newFakeRecordKV(map[string][]byte{"k": []byte(`{"project_slug":"old-slug"}`)}), failUpdates: 1}

		require.NoError(t, processKVRecord(context.Background(), kv, "k", fields, "old-slug", "new-slug", false))

		assert.Equal(t, 2, kv.updates)
		assert.JSONEq(t, `{"project_slug":"new-slug"}`, string(kv.entries["k"]))
	})

	t.Run("gives up after three conflicts", func(t *testing.T) {
		kv := &conflictingKV{fakeRecordKV: newFakeRecordKV(map[string][]byte{"k": []byte(`{"project_slug":"old-slug"}`)}), failUpdates: 10}

		err := processKVRecord(context.Background(), kv, "k", fields, "old-slug", "new-slug", false)

		require.ErrorContains(t, err, "failed to update after 3 attempts")
		assert.Equal(t, 3, kv.updates)
	})

	t.Run("a concurrent rename during a retry becomes a mismatch", func(t *testing.T) {
		kv := &conflictingKV{
			fakeRecordKV: newFakeRecordKV(map[string][]byte{"k": []byte(`{"project_slug":"old-slug"}`)}),
			failUpdates:  1,
			onConflict: func(rec *fakeRecordKV) {
				rec.entries["k"] = []byte(`{"project_slug":"someone-elses-slug"}`)
				rec.revisions["k"]++
			},
		}

		err := processKVRecord(context.Background(), kv, "k", fields, "old-slug", "new-slug", false)

		require.ErrorIs(t, err, errSlugMismatch)
		assert.JSONEq(t, `{"project_slug":"someone-elses-slug"}`, string(kv.entries["k"]))
	})

	t.Run("a record deleted during a retry is an error", func(t *testing.T) {
		kv := &conflictingKV{
			fakeRecordKV: newFakeRecordKV(map[string][]byte{"k": []byte(`{"project_slug":"old-slug"}`)}),
			failUpdates:  1,
			onConflict:   func(rec *fakeRecordKV) { delete(rec.entries, "k") },
		}

		err := processKVRecord(context.Background(), kv, "k", fields, "old-slug", "new-slug", false)

		require.ErrorContains(t, err, "failed to re-fetch entry")
	})

	t.Run("a record corrupted during a retry is an error", func(t *testing.T) {
		kv := &conflictingKV{
			fakeRecordKV: newFakeRecordKV(map[string][]byte{"k": []byte(`{"project_slug":"old-slug"}`)}),
			failUpdates:  1,
			onConflict:   func(rec *fakeRecordKV) { rec.entries["k"] = []byte(`not json`) },
		}

		err := processKVRecord(context.Background(), kv, "k", fields, "old-slug", "new-slug", false)

		require.ErrorContains(t, err, "failed to unmarshal re-fetched entry")
	})

	t.Run("a cancelled context stops the retry backoff", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		kv := &conflictingKV{fakeRecordKV: newFakeRecordKV(map[string][]byte{"k": []byte(`{"project_slug":"old-slug"}`)}), failUpdates: 10}

		err := processKVRecord(ctx, kv, "k", fields, "old-slug", "new-slug", false)

		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, kv.updates)
	})
}

func TestReserveSlugIndex(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"

	t.Run("dry run accepts a free key and one already owned by the project", func(t *testing.T) {
		require.NoError(t, reserveSlugIndex(context.Background(), newFakeRecordKV(nil), uid, "new-slug", true))
		owned := newFakeRecordKV(map[string][]byte{"slug/new-slug": []byte(uid)})
		require.NoError(t, reserveSlugIndex(context.Background(), owned, uid, "new-slug", true))
	})

	t.Run("dry run reports a collision with another project", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{"slug/new-slug": []byte("other")})
		err := reserveSlugIndex(context.Background(), kv, uid, "new-slug", true)
		require.ErrorIs(t, err, errSlugIndexCollision)
	})

	t.Run("apply accepts a key already owned by the project", func(t *testing.T) {
		kv := newFakeRecordKV(map[string][]byte{"slug/new-slug": []byte(uid)})
		require.NoError(t, reserveSlugIndex(context.Background(), kv, uid, "new-slug", false))
	})

	t.Run("a lookup error other than not-found is surfaced", func(t *testing.T) {
		err := reserveSlugIndex(context.Background(), erroringSlugKV{getErr: errors.New("kv down")}, uid, "new-slug", true)
		require.ErrorContains(t, err, "failed to check slug index")
	})

	t.Run("a create error other than a conflict is surfaced", func(t *testing.T) {
		err := reserveSlugIndex(context.Background(), erroringSlugKV{createErr: errors.New("kv down")}, uid, "new-slug", false)
		require.ErrorContains(t, err, "failed to create slug index")
	})

	t.Run("a conflict whose owner cannot be re-read is surfaced", func(t *testing.T) {
		kv := erroringSlugKV{createErr: jetstream.ErrKeyExists, getErr: errors.New("kv down")}
		err := reserveSlugIndex(context.Background(), kv, uid, "new-slug", false)
		require.ErrorContains(t, err, "failed to verify slug index")
	})
}

func TestDeleteOldSlugIndex(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"

	t.Run("a lookup error other than not-found is surfaced", func(t *testing.T) {
		err := deleteOldSlugIndex(context.Background(), erroringSlugKV{getErr: errors.New("kv down")}, uid, "old-slug", false)
		require.ErrorContains(t, err, "failed to check slug index")
	})

	t.Run("a delete error is surfaced", func(t *testing.T) {
		kv := erroringSlugKV{entry: fakeKVEntry{key: "slug/old-slug", value: []byte(uid), revision: 3}, deleteErr: errors.New("kv down")}
		err := deleteOldSlugIndex(context.Background(), kv, uid, "old-slug", false)
		require.ErrorContains(t, err, "failed to delete stale slug index")
	})

	t.Run("a delete that races with another delete is not an error", func(t *testing.T) {
		kv := erroringSlugKV{entry: fakeKVEntry{key: "slug/old-slug", value: []byte(uid), revision: 3}, deleteErr: jetstream.ErrKeyNotFound}
		require.NoError(t, deleteOldSlugIndex(context.Background(), kv, uid, "old-slug", false))
	})
}

// erroringSlugKV is a slugIndexKV whose operations return canned results.
type erroringSlugKV struct {
	entry     jetstream.KeyValueEntry
	getErr    error
	createErr error
	deleteErr error
}

func (e erroringSlugKV) Get(context.Context, string) (jetstream.KeyValueEntry, error) {
	if e.getErr != nil {
		return nil, e.getErr
	}
	if e.entry == nil {
		return nil, jetstream.ErrKeyNotFound
	}
	return e.entry, nil
}

func (e erroringSlugKV) Create(context.Context, string, []byte, ...jetstream.KVCreateOpt) (uint64, error) {
	return 1, e.createErr
}

func (e erroringSlugKV) Delete(context.Context, string, ...jetstream.KVDeleteOpt) error {
	return e.deleteErr
}
