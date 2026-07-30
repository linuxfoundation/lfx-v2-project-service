// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type fakeKVEntry struct {
	key      string
	value    []byte
	revision uint64
}

func (f fakeKVEntry) Bucket() string                  { return "projects" }
func (f fakeKVEntry) Key() string                     { return f.key }
func (f fakeKVEntry) Value() []byte                   { return f.value }
func (f fakeKVEntry) Revision() uint64                { return f.revision }
func (f fakeKVEntry) Created() time.Time              { return time.Time{} }
func (f fakeKVEntry) Delta() uint64                   { return 0 }
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

func (f *fakeRecordKV) Put(_ context.Context, key string, value []byte) (uint64, error) {
	f.entries[key] = value
	f.revisions[key]++
	return f.revisions[key], nil
}

func (f *fakeRecordKV) Delete(_ context.Context, key string, _ ...jetstream.KVDeleteOpt) error {
	if _, ok := f.entries[key]; !ok {
		return jetstream.ErrKeyNotFound
	}
	delete(f.entries, key)
	delete(f.revisions, key)
	return nil
}

func TestResolveSlugs_fromFlags(t *testing.T) {
	old, new, err := resolveSlugs("old-slug", "new-slug", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if old != "old-slug" || new != "new-slug" {
		t.Fatalf("got %q %q", old, new)
	}
}

func TestResolveSlugs_fromPositionals(t *testing.T) {
	old, new, err := resolveSlugs("", "", []string{"old-slug", "new-slug"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if old != "old-slug" || new != "new-slug" {
		t.Fatalf("got %q %q", old, new)
	}
}

func TestResolveSlugs_rejectsMixedInput(t *testing.T) {
	_, _, err := resolveSlugs("old-slug", "", []string{"new-slug"})
	if err == nil {
		t.Fatal("expected error when mixing flags and positional args")
	}
}

func TestResolveSlugs_requiresBothSlugs(t *testing.T) {
	_, _, err := resolveSlugs("old-slug", "", nil)
	if err == nil {
		t.Fatal("expected error when new slug is missing")
	}
}

func TestBucketFieldsFor_knownBuckets(t *testing.T) {
	cases := []struct {
		bucket string
		field  string
	}{
		{"committee-members", "project_slug"},
		{"committees", "project_slug"},
		{"committee-settings", "project_slug"},
		{"projects", "slug"},
		{"project-settings", "project_slug"},
	}
	for _, c := range cases {
		fields := bucketFieldsFor(c.bucket)
		found := false
		for _, f := range fields {
			if f == c.field {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("bucketFieldsFor(%q): expected field %q, got %v", c.bucket, c.field, fields)
		}
	}
}

func TestBucketFieldsFor_unknownBucket(t *testing.T) {
	fields := bucketFieldsFor("some-unknown-bucket")
	if len(fields) != 1 || fields[0] != "project_slug" {
		t.Errorf("expected [project_slug] for unknown bucket, got %v", fields)
	}
}

func TestParseNATSBuckets(t *testing.T) {
	got := parseNATSBuckets("committee-members, committees , committee-settings")
	want := []string{"committee-members", "committees", "committee-settings"}
	assertEqual(t, want, got)
}

func TestBuildOSQuery_containsOldSlug(t *testing.T) {
	const slug = "old-slug"
	q := buildOSQuery(slug)
	b, ok := q["bool"].(map[string]any)
	if !ok {
		t.Fatal("expected bool key in query")
	}
	should, ok := b["should"].([]any)
	if !ok {
		t.Fatal("expected should key in bool query")
	}
	if len(should) == 0 {
		t.Fatal("expected non-empty should clauses")
	}

	fields := map[string]bool{}
	for _, clause := range should {
		termClause, ok := clause.(map[string]any)
		if !ok {
			continue
		}
		term, ok := termClause["term"].(map[string]any)
		if !ok {
			continue
		}
		for k, v := range term {
			fields[k] = true
			if str, ok := v.(string); ok {
				if str != slug && str != "project_slug:"+slug {
					t.Errorf("unexpected term value for field %q: %q", k, str)
				}
			}
		}
	}

	for _, required := range []string{"data.project_slug", "data.slug", "tags"} {
		if !fields[required] {
			t.Errorf("expected should clause for field %q, but it was missing", required)
		}
	}
}

// reconcile runs the same collision-then-write sequence processProjectRecord
// uses to keep the slug index in sync (checkSlugIndexCollision, then
// writeSlugIndex).
func reconcile(ctx context.Context, kv recordKV, uid, oldSlug, newSlug string, dryRun bool) error {
	if err := checkSlugIndexCollision(ctx, kv, uid, newSlug); err != nil {
		return err
	}
	return writeSlugIndex(ctx, kv, uid, oldSlug, newSlug, dryRun)
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

func assertEqual[T comparable](t *testing.T, want, got []T) {
	t.Helper()
	if len(want) != len(got) {
		t.Errorf("length mismatch: want %v, got %v", want, got)
		return
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("index %d: want %v, got %v", i, want[i], got[i])
		}
	}
}
