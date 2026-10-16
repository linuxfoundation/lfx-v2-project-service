// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/log"
)

// osCall is one request the fake OpenSearch transport received.
type osCall struct {
	path  string
	query string
	body  string
}

// newRoutedOpenSearchClient returns a client whose every response is the given
// status and body, recording each request so a test can assert what was sent.
func newRoutedOpenSearchClient(t *testing.T, status int, body string, calls *[]osCall) *opensearchgo.Client {
	t.Helper()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		if calls != nil {
			*calls = append(*calls, osCall{path: req.URL.Path, query: req.URL.RawQuery, body: string(raw)})
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	})
	client, err := opensearchgo.NewClient(opensearchgo.Config{
		Addresses:    []string{"http://opensearch.invalid"},
		DisableRetry: true,
		Transport:    transport,
	})
	require.NoError(t, err)
	return client
}

// failingOpenSearchClient returns a client whose transport always fails.
func failingOpenSearchClient(t *testing.T) *opensearchgo.Client {
	t.Helper()
	client, err := opensearchgo.NewClient(opensearchgo.Config{
		Addresses:    []string{"http://opensearch.invalid"},
		DisableRetry: true,
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		}),
	})
	require.NoError(t, err)
	return client
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
func (f *fakeKeyLister) Stop() error         { return nil }

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

func TestRenameSlugRunner_runOpenSearch(t *testing.T) {
	tests := []struct {
		name        string
		dryRun      bool
		status      int
		body        string
		wantErr     string
		wantTotal   int
		wantUpdated int
		wantNoops   int
		wantSkipped int
		wantPath    string
	}{
		{
			name: "dry run reports the matched count", dryRun: true, status: 200,
			body:      `{"hits":{"total":{"value":5}}}`,
			wantTotal: 5, wantUpdated: 5, wantPath: "/resources/_search",
		},
		{
			name: "apply reports updated and noop counts", status: 200,
			body:      `{"total":6,"updated":4,"noops":2,"version_conflicts":0}`,
			wantTotal: 6, wantUpdated: 4, wantNoops: 2, wantSkipped: 2, wantPath: "/resources/_update_by_query",
		},
		{
			name: "version conflicts fail the run", status: 200,
			body:    `{"total":3,"updated":2,"version_conflicts":1}`,
			wantErr: "1 version conflicts",
		},
		{
			name: "document failures fail the run", status: 200,
			body:    `{"total":3,"updated":2,"failures":[{"cause":"x"}]}`,
			wantErr: "1 document failures",
		},
		{
			name: "document failures alongside conflicts report both", status: 200,
			body:    `{"total":3,"updated":1,"version_conflicts":2,"failures":[{"cause":"x"}]}`,
			wantErr: "2 version conflicts",
		},
		{
			name: "an HTTP error fails the run", status: 500, body: `{"error":"boom"}`,
			wantErr: "update_by_query error",
		},
		{
			name: "dry run HTTP error fails the run", dryRun: true, status: 503, body: `{"error":"boom"}`,
			wantErr: "search error",
		},
		{
			name: "an undecodable response fails the run", status: 200, body: `not json`,
			wantErr: "failed to decode update_by_query response",
		},
		{
			name: "dry run undecodable response fails the run", dryRun: true, status: 200, body: `not json`,
			wantErr: "failed to decode search response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []osCall
			r := &RenameSlugRunner{openSearch: newRoutedOpenSearchClient(t, tt.status, tt.body, &calls)}

			summary, err := r.runOpenSearch(context.Background(), "old-slug", "new-slug", tt.dryRun)

			require.Len(t, calls, 1)
			if tt.wantPath != "" {
				assert.Equal(t, tt.wantPath, calls[0].path)
			}
			assert.Equal(t, "opensearch", summary.Store)
			assert.Equal(t, tt.dryRun, summary.DryRun)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTotal, summary.Total)
			assert.Equal(t, tt.wantUpdated, summary.Updated)
			assert.Equal(t, tt.wantNoops, summary.Noops)
			assert.Equal(t, tt.wantSkipped, summary.Skipped)
		})
	}

	t.Run("apply sends the slugs as script params and proceeds on conflicts", func(t *testing.T) {
		var calls []osCall
		r := &RenameSlugRunner{openSearch: newRoutedOpenSearchClient(t, 200, `{"total":1,"updated":1}`, &calls)}

		_, err := r.runOpenSearch(context.Background(), "old-slug", "new-slug", false)

		require.NoError(t, err)
		require.Len(t, calls, 1)
		assert.Contains(t, calls[0].query, "conflicts=proceed")
		assert.Contains(t, calls[0].body, `"oldSlug":"old-slug"`)
		assert.Contains(t, calls[0].body, `"newSlug":"new-slug"`)
		assert.Contains(t, calls[0].body, `"lang":"painless"`)
	})

	t.Run("dry run only counts: no hits are fetched and nothing is updated", func(t *testing.T) {
		var calls []osCall
		r := &RenameSlugRunner{openSearch: newRoutedOpenSearchClient(t, 200, `{"hits":{"total":{"value":0}}}`, &calls)}

		_, err := r.runOpenSearch(context.Background(), "old-slug", "new-slug", true)

		require.NoError(t, err)
		require.Len(t, calls, 1)
		assert.Contains(t, calls[0].body, `"size":0`)
		assert.NotContains(t, calls[0].path, "_update_by_query")
	})

	t.Run("a transport failure is wrapped", func(t *testing.T) {
		r := &RenameSlugRunner{openSearch: failingOpenSearchClient(t)}

		_, err := r.runOpenSearch(context.Background(), "old-slug", "new-slug", true)
		require.ErrorContains(t, err, "search request failed")

		_, err = r.runOpenSearch(context.Background(), "old-slug", "new-slug", false)
		require.ErrorContains(t, err, "update_by_query request failed")
	})
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

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no credentials is unchanged", in: "http://localhost:9200", want: "http://localhost:9200"},
		{name: "user and password are redacted", in: "https://admin:s3cret@search.example.com:9200/x", want: "https://REDACTED@search.example.com:9200/x"},
		{name: "a bare user is redacted", in: "nats://token@nats.example.com:4222", want: "nats://REDACTED@nats.example.com:4222"},
		{name: "an unparseable URL is not echoed", in: "http://[::1", want: "<invalid>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactURL(tt.in)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "s3cret")
		})
	}
}

func TestJSONBody(t *testing.T) {
	t.Run("encodes the value", func(t *testing.T) {
		r, err := jsonBody(map[string]any{"a": 1})
		require.NoError(t, err)
		raw, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.JSONEq(t, `{"a":1}`, string(raw))
	})

	t.Run("an unencodable value is an error", func(t *testing.T) {
		_, err := jsonBody(map[string]any{"c": make(chan int)})
		require.ErrorContains(t, err, "failed to marshal request body")
	})
}

func TestSleepWithContext(t *testing.T) {
	t.Run("a non-positive duration returns immediately", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.NoError(t, sleepWithContext(ctx, 0))
		require.NoError(t, sleepWithContext(ctx, -time.Second))
	})

	t.Run("waits the duration", func(t *testing.T) {
		start := time.Now()
		require.NoError(t, sleepWithContext(context.Background(), 10*time.Millisecond))
		assert.GreaterOrEqual(t, time.Since(start), 10*time.Millisecond)
	})

	t.Run("a cancelled context interrupts the wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.ErrorIs(t, sleepWithContext(ctx, time.Minute), context.Canceled)
	})
}

func TestWithSlugContext(t *testing.T) {
	// The attributes ride on the context and only surface through the handler
	// installed by log.InitStructureLogConfig, which writes to os.Stdout.
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	prevStdout, prevDefault := os.Stdout, slog.Default()
	os.Stdout = writer
	log.InitStructureLogConfig()
	t.Cleanup(func() {
		os.Stdout = prevStdout
		slog.SetDefault(prevDefault)
	})

	ctx := withSlugContext(context.Background(), "old-slug", "new-slug", true)
	slog.InfoContext(ctx, "probe")

	require.NoError(t, writer.Close())
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &entry), string(raw))
	assert.Equal(t, "old-slug", entry["old_slug"])
	assert.Equal(t, "new-slug", entry["new_slug"])
	assert.Equal(t, true, entry["dry_run"])
}
