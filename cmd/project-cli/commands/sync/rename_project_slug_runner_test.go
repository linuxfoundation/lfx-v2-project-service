// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"errors"
	"fmt"
	gosync "sync"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// fakeKeyLister is a jetstream.KeyLister over a fixed key list.
type fakeKeyLister struct{ keys chan string }

func newFakeKeyLister(keys []string) *fakeKeyLister {
	ch := make(chan string, len(keys))
	for _, k := range keys {
		ch <- k
	}
	close(ch)
	return &fakeKeyLister{keys: ch}
}

func (f *fakeKeyLister) Keys() <-chan string { return f.keys }

func (f *fakeKeyLister) Stop() error { return nil }

// fakeBucketKV is a jetstream.KeyValue backed by fakeRecordKV. Only the
// methods the migration uses are implemented; the mutex makes it safe for the
// concurrent per-record workers.
type fakeBucketKV struct {
	jetstream.KeyValue
	mu      gosync.Mutex
	rec     *fakeRecordKV
	listErr error
}

func (f *fakeBucketKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rec.Get(ctx, key)
}

func (f *fakeBucketKV) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rec.Update(ctx, key, value, revision)
}

func (f *fakeBucketKV) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rec.Create(ctx, key, value, opts...)
}

func (f *fakeBucketKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rec.Delete(ctx, key, opts...)
}

func (f *fakeBucketKV) ListKeys(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	keys := make([]string, 0, len(f.rec.entries))
	for k := range f.rec.entries {
		keys = append(keys, k)
	}
	return newFakeKeyLister(keys), nil
}

// fakeJetStream hands out fakeBucketKV buckets by name.
type fakeJetStream struct {
	jetstream.JetStream
	buckets map[string]*fakeBucketKV
	errs    map[string]error
}

func (f *fakeJetStream) KeyValue(_ context.Context, bucket string) (jetstream.KeyValue, error) {
	if err := f.errs[bucket]; err != nil {
		return nil, err
	}
	kv, ok := f.buckets[bucket]
	if !ok {
		return nil, jetstream.ErrBucketNotFound
	}
	return kv, nil
}

func newFakeBucket(entries map[string][]byte) *fakeBucketKV {
	return &fakeBucketKV{rec: newFakeRecordKV(entries)}
}

func TestNewRenameSlugRunner(t *testing.T) {
	os := failingOpenSearchClient(t)
	js := &fakeJetStream{}

	r := NewRenameSlugRunner(os, js)

	assert.Same(t, os, r.openSearch)
	assert.Equal(t, js, r.jetStream)
}

func TestRenameSlugRunner_Run(t *testing.T) {
	const (
		searchHit = `{"hits":{"total":{"value":2}}}`
		updateOK  = `{"total":2,"updated":2,"version_conflicts":0,"noops":0,"failures":[]}`
	)

	newJS := func() *fakeJetStream {
		return &fakeJetStream{buckets: map[string]*fakeBucketKV{
			"projects": newFakeBucket(map[string][]byte{
				"p1":            []byte(`{"slug":"old-slug"}`),
				"slug/old-slug": []byte("p1"),
			}),
			"project-settings": newFakeBucket(map[string][]byte{
				"p1": []byte(`{"project_slug":"old-slug"}`),
			}),
		}}
	}
	buckets := []string{"projects", "project-settings"}

	tests := []struct {
		name       string
		runner     func(t *testing.T) *RenameSlugRunner
		opts       RenameSlugOptions
		wantErr    string
		wantOSPath string
		check      func(t *testing.T, r *RenameSlugRunner)
	}{
		{
			name:    "missing slugs are rejected",
			runner:  func(t *testing.T) *RenameSlugRunner { return &RenameSlugRunner{} },
			opts:    RenameSlugOptions{OldSlug: "old-slug", Concurrency: 1},
			wantErr: "old slug and new slug are required",
		},
		{
			name:    "identical slugs are rejected",
			runner:  func(t *testing.T) *RenameSlugRunner { return &RenameSlugRunner{} },
			opts:    RenameSlugOptions{OldSlug: "same", NewSlug: "same", Concurrency: 1},
			wantErr: "must differ",
		},
		{
			name:    "zero concurrency is rejected",
			runner:  func(t *testing.T) *RenameSlugRunner { return &RenameSlugRunner{} },
			opts:    RenameSlugOptions{OldSlug: "old-slug", NewSlug: "new-slug"},
			wantErr: "concurrency must be at least 1",
		},
		{
			name:    "a missing OpenSearch client is rejected",
			runner:  func(t *testing.T) *RenameSlugRunner { return &RenameSlugRunner{jetStream: newJS()} },
			opts:    RenameSlugOptions{OldSlug: "old-slug", NewSlug: "new-slug", Concurrency: 1},
			wantErr: "OpenSearch client is required",
		},
		{
			name: "a missing JetStream client is rejected",
			runner: func(t *testing.T) *RenameSlugRunner {
				return &RenameSlugRunner{openSearch: newRoutedOpenSearchClient(t, 200, searchHit, nil)}
			},
			opts:    RenameSlugOptions{OldSlug: "old-slug", NewSlug: "new-slug", Concurrency: 1},
			wantErr: "JetStream client is required",
		},
		{
			name: "dry run searches OpenSearch and leaves every store untouched",
			runner: func(t *testing.T) *RenameSlugRunner {
				return NewRenameSlugRunner(newRoutedOpenSearchClient(t, 200, searchHit, nil), newJS())
			},
			opts: RenameSlugOptions{OldSlug: "old-slug", NewSlug: "new-slug", DryRun: true, Concurrency: 2, NATSBuckets: buckets},
			check: func(t *testing.T, r *RenameSlugRunner) {
				js := r.jetStream.(*fakeJetStream)
				assert.JSONEq(t, `{"slug":"old-slug"}`, string(js.buckets["projects"].rec.entries["p1"]))
				assert.Contains(t, js.buckets["projects"].rec.entries, "slug/old-slug")
				assert.NotContains(t, js.buckets["projects"].rec.entries, "slug/new-slug")
			},
		},
		{
			name: "apply renames the slug in OpenSearch and every bucket",
			runner: func(t *testing.T) *RenameSlugRunner {
				return NewRenameSlugRunner(newRoutedOpenSearchClient(t, 200, updateOK, nil), newJS())
			},
			opts: RenameSlugOptions{OldSlug: "old-slug", NewSlug: "new-slug", Concurrency: 2, NATSBuckets: buckets},
			check: func(t *testing.T, r *RenameSlugRunner) {
				js := r.jetStream.(*fakeJetStream)
				assert.JSONEq(t, `{"slug":"new-slug"}`, string(js.buckets["projects"].rec.entries["p1"]))
				assert.Equal(t, "p1", string(js.buckets["projects"].rec.entries["slug/new-slug"]))
				assert.NotContains(t, js.buckets["projects"].rec.entries, "slug/old-slug")
				assert.JSONEq(t, `{"project_slug":"new-slug"}`, string(js.buckets["project-settings"].rec.entries["p1"]))
			},
		},
		{
			name: "an OpenSearch failure stops before NATS is touched",
			runner: func(t *testing.T) *RenameSlugRunner {
				return NewRenameSlugRunner(newRoutedOpenSearchClient(t, 500, `{"error":"boom"}`, nil), newJS())
			},
			opts:    RenameSlugOptions{OldSlug: "old-slug", NewSlug: "new-slug", Concurrency: 1, NATSBuckets: buckets},
			wantErr: "opensearch migration failed",
			check: func(t *testing.T, r *RenameSlugRunner) {
				js := r.jetStream.(*fakeJetStream)
				assert.JSONEq(t, `{"slug":"old-slug"}`, string(js.buckets["projects"].rec.entries["p1"]))
			},
		},
		{
			name: "a NATS failure is reported after OpenSearch succeeds",
			runner: func(t *testing.T) *RenameSlugRunner {
				js := newJS()
				js.errs = map[string]error{"projects": errors.New("kv unavailable")}
				return NewRenameSlugRunner(newRoutedOpenSearchClient(t, 200, updateOK, nil), js)
			},
			opts:    RenameSlugOptions{OldSlug: "old-slug", NewSlug: "new-slug", Concurrency: 1, NATSBuckets: buckets},
			wantErr: "nats migration failed",
		},
		{
			name: "default buckets are used when none are given, skipping ones that do not exist",
			runner: func(t *testing.T) *RenameSlugRunner {
				return NewRenameSlugRunner(newRoutedOpenSearchClient(t, 200, updateOK, nil), newJS())
			},
			opts: RenameSlugOptions{OldSlug: "old-slug", NewSlug: "new-slug", Concurrency: 1},
			check: func(t *testing.T, r *RenameSlugRunner) {
				js := r.jetStream.(*fakeJetStream)
				assert.JSONEq(t, `{"slug":"new-slug"}`, string(js.buckets["projects"].rec.entries["p1"]))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.runner(t)
			err := r.Run(context.Background(), tt.opts)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tt.check != nil {
				tt.check(t, r)
			}
		})
	}
}

func TestRenameSlugRunner_runNATS(t *testing.T) {
	record := func(slug string) []byte { return []byte(fmt.Sprintf(`{"project_slug":%q}`, slug)) }

	t.Run("totals every bucket and counts mismatched records as skipped", func(t *testing.T) {
		js := &fakeJetStream{buckets: map[string]*fakeBucketKV{
			"committees": newFakeBucket(map[string][]byte{
				"c1":            record("old-slug"),
				"c2":            record("other-slug"),
				"lookup/c1":     []byte("ignored"),
				"slug/old-slug": []byte("ignored"),
			}),
			"committee-members": newFakeBucket(map[string][]byte{"m1": record("old-slug")}),
		}}
		r := &RenameSlugRunner{jetStream: js}

		summary, err := r.runNATS(context.Background(), "old-slug", "new-slug", false, 4, []string{"committees", "committee-members"})

		require.NoError(t, err)
		assert.Equal(t, "nats", summary.Store)
		assert.Equal(t, 3, summary.Total, "lookup/ and slug/ keys are not records")
		assert.Equal(t, 2, summary.Updated)
		assert.Equal(t, 1, summary.Skipped)
		assert.JSONEq(t, `{"project_slug":"new-slug"}`, string(js.buckets["committees"].rec.entries["c1"]))
		assert.JSONEq(t, `{"project_slug":"other-slug"}`, string(js.buckets["committees"].rec.entries["c2"]))
	})

	t.Run("dry run counts matches without writing", func(t *testing.T) {
		js := &fakeJetStream{buckets: map[string]*fakeBucketKV{
			"committees": newFakeBucket(map[string][]byte{"c1": record("old-slug")}),
		}}
		r := &RenameSlugRunner{jetStream: js}

		summary, err := r.runNATS(context.Background(), "old-slug", "new-slug", true, 1, []string{"committees"})

		require.NoError(t, err)
		assert.Equal(t, 1, summary.Updated)
		assert.JSONEq(t, `{"project_slug":"old-slug"}`, string(js.buckets["committees"].rec.entries["c1"]))
	})

	t.Run("a missing bucket is skipped, not an error", func(t *testing.T) {
		r := &RenameSlugRunner{jetStream: &fakeJetStream{}}

		summary, err := r.runNATS(context.Background(), "old-slug", "new-slug", false, 1, []string{"committees"})

		require.NoError(t, err)
		assert.Zero(t, summary.Errors)
	})

	t.Run("a bucket that cannot be opened makes the migration incomplete", func(t *testing.T) {
		js := &fakeJetStream{
			buckets: map[string]*fakeBucketKV{"committees": newFakeBucket(map[string][]byte{"c1": record("old-slug")})},
			errs:    map[string]error{"projects": errors.New("kv unavailable")},
		}
		r := &RenameSlugRunner{jetStream: js}

		summary, err := r.runNATS(context.Background(), "old-slug", "new-slug", false, 1, []string{"projects", "committees"})

		require.ErrorContains(t, err, "1 bucket(s) failed")
		assert.Equal(t, 1, summary.Errors)
		assert.Equal(t, 1, summary.Updated, "later buckets are still migrated")
	})

	t.Run("a bucket whose keys cannot be listed makes the migration incomplete", func(t *testing.T) {
		bucket := newFakeBucket(nil)
		bucket.listErr = errors.New("list failed")
		r := &RenameSlugRunner{jetStream: &fakeJetStream{buckets: map[string]*fakeBucketKV{"committees": bucket}}}

		_, err := r.runNATS(context.Background(), "old-slug", "new-slug", false, 1, []string{"committees"})

		require.ErrorContains(t, err, "1 bucket(s) failed")
	})

	t.Run("records that fail to update fail the run", func(t *testing.T) {
		js := &fakeJetStream{buckets: map[string]*fakeBucketKV{
			"committees": newFakeBucket(map[string][]byte{"c1": []byte(`not json`)}),
		}}
		r := &RenameSlugRunner{jetStream: js}

		summary, err := r.runNATS(context.Background(), "old-slug", "new-slug", false, 1, []string{"committees"})

		require.ErrorContains(t, err, "1 records failed")
		assert.Equal(t, 1, summary.Failed)
	})

	t.Run("a project collision fails that record and leaves it unchanged", func(t *testing.T) {
		js := &fakeJetStream{buckets: map[string]*fakeBucketKV{
			"projects": newFakeBucket(map[string][]byte{
				"p1":            []byte(`{"slug":"old-slug"}`),
				"slug/old-slug": []byte("p1"),
				"slug/new-slug": []byte("p2"),
			}),
		}}
		r := &RenameSlugRunner{jetStream: js}

		summary, err := r.runNATS(context.Background(), "old-slug", "new-slug", false, 1, []string{"projects"})

		require.Error(t, err)
		assert.Equal(t, 1, summary.Failed)
		assert.JSONEq(t, `{"slug":"old-slug"}`, string(js.buckets["projects"].rec.entries["p1"]))
	})
}

func TestLogRenameSlugSummary(t *testing.T) {
	t.Run("apply logs updated and only the non-zero optional counters", func(t *testing.T) {
		buf := captureSlog(t)

		logRenameSlugSummary(context.Background(), renameSlugSummary{Store: "nats", Total: 4, Updated: 3, Skipped: 1})

		out := buf.String()
		assert.Contains(t, out, `"store":"nats"`)
		assert.Contains(t, out, `"updated":3`)
		assert.NotContains(t, out, `"matched"`)
		assert.NotContains(t, out, `"bucket"`)
		assert.NotContains(t, out, `"noops"`)
		assert.NotContains(t, out, `"version_conflicts"`)
	})

	t.Run("dry run logs matched and every populated counter", func(t *testing.T) {
		buf := captureSlog(t)

		logRenameSlugSummary(context.Background(), renameSlugSummary{
			Store: "opensearch", DryRun: true, Updated: 2, Bucket: "projects", Examined: 9, Noops: 1, Conflicts: 3, Errors: 4,
		})

		out := buf.String()
		assert.Contains(t, out, `"matched":2`)
		assert.NotContains(t, out, `"updated"`)
		assert.Contains(t, out, `"bucket":"projects"`)
		assert.Contains(t, out, `"examined":9`)
		assert.Contains(t, out, `"noops":1`)
		assert.Contains(t, out, `"version_conflicts":3`)
		assert.Contains(t, out, `"bucket_errors":4`)
	})
}
