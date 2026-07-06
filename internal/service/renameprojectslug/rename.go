// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package renameprojectslug renames a project slug across OpenSearch and NATS KV stores.
package renameprojectslug

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
	"golang.org/x/sync/errgroup"
)

var errSlugMismatch = errors.New("project_slug does not match old slug")

// DefaultNATSBuckets are the KV buckets scanned during a slug rename migration.
var DefaultNATSBuckets = []string{
	"committee-members",
	"committees",
	"committee-settings",
	"projects",
	"project-settings",
}

// bucketSlugFields maps a KV bucket name to the JSON field name(s) that hold
// the project slug.
var bucketSlugFields = map[string][]string{
	"committee-members":  {"project_slug"},
	"committees":         {"project_slug"},
	"committee-settings": {"project_slug"},
	"projects":           {"slug"},
	"project-settings":   {"project_slug"},
}

// Options configures a rename-project-slug run.
type Options struct {
	OldSlug     string
	NewSlug     string
	Target      string
	DryRun      bool
	Concurrency int
	NATSBuckets []string
}

// Summary captures aggregate counters for a migration run.
type Summary struct {
	Target    string
	DryRun    bool
	Total     int
	Updated   int
	Skipped   int
	Failed    int
	Errors    int
	Store     string
	Bucket    string
	Examined  int
	Noops     int
	Conflicts int
}

// Runner executes slug rename migrations using shared infrastructure clients.
type Runner struct {
	openSearch *opensearchgo.Client
	jetStream  jetstream.JetStream
}

// NewRunner creates a Runner backed by the provided clients.
func NewRunner(openSearch *opensearchgo.Client, jetStream jetstream.JetStream) *Runner {
	return &Runner{
		openSearch: openSearch,
		jetStream:  jetStream,
	}
}

// Run renames oldSlug to newSlug across the configured targets.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	if opts.OldSlug == "" || opts.NewSlug == "" {
		return fmt.Errorf("old slug and new slug are required")
	}
	if opts.OldSlug == opts.NewSlug {
		return fmt.Errorf("old slug and new slug must differ")
	}

	target := strings.ToLower(strings.TrimSpace(opts.Target))
	if target != "both" && target != "opensearch" && target != "nats" {
		return fmt.Errorf("target must be one of: opensearch, nats, both")
	}
	if (target == "nats" || target == "both") && opts.Concurrency < 1 {
		return fmt.Errorf("concurrency must be at least 1")
	}

	buckets := opts.NATSBuckets
	if len(buckets) == 0 {
		buckets = DefaultNATSBuckets
	}

	slog.InfoContext(ctx, "starting rename-project-slug",
		"target", target,
	)

	if target == "opensearch" || target == "both" {
		if r.openSearch == nil {
			return fmt.Errorf("OpenSearch client is required for target %q", target)
		}
		summary, err := r.runOpenSearch(ctx, opts.OldSlug, opts.NewSlug, opts.DryRun)
		logSummary(ctx, summary)
		if err != nil {
			return fmt.Errorf("opensearch migration failed: %w", err)
		}
	}

	if target == "nats" || target == "both" {
		if r.jetStream == nil {
			return fmt.Errorf("NATS JetStream client is required for target %q", target)
		}
		summary, err := r.runNATS(ctx, opts.OldSlug, opts.NewSlug, opts.DryRun, opts.Concurrency, buckets)
		logSummary(ctx, summary)
		if err != nil {
			return fmt.Errorf("nats migration failed: %w", err)
		}
	}

	return nil
}

func logSummary(ctx context.Context, summary Summary) {
	attrs := []any{
		"store", summary.Store,
		"dry_run", summary.DryRun,
		"total", summary.Total,
		"skipped", summary.Skipped,
		"failed", summary.Failed,
	}
	if summary.DryRun && summary.Store == "opensearch" {
		attrs = append(attrs, "matched", summary.Total)
	} else {
		attrs = append(attrs, "updated", summary.Updated)
	}
	if summary.Bucket != "" {
		attrs = append(attrs, "bucket", summary.Bucket)
	}
	if summary.Examined > 0 {
		attrs = append(attrs, "examined", summary.Examined)
	}
	if summary.Noops > 0 {
		attrs = append(attrs, "noops", summary.Noops)
	}
	if summary.Conflicts > 0 {
		attrs = append(attrs, "version_conflicts", summary.Conflicts)
	}
	if summary.Errors > 0 {
		attrs = append(attrs, "bucket_errors", summary.Errors)
	}
	slog.InfoContext(ctx, "rename-project-slug store complete", attrs...)
}

func (r *Runner) runOpenSearch(ctx context.Context, oldSlug, newSlug string, dryRun bool) (Summary, error) {
	summary := Summary{Store: "opensearch", DryRun: dryRun}
	query := buildOSQuery(oldSlug)

	if dryRun {
		matched, err := r.auditOpenSearch(ctx, query)
		summary.Total = matched
		summary.Updated = matched
		return summary, err
	}

	examined, updated, noops, conflicts, err := r.updateOpenSearch(ctx, query, oldSlug, newSlug)
	summary.Examined = examined
	summary.Total = examined
	summary.Updated = updated
	summary.Noops = noops
	summary.Conflicts = conflicts
	summary.Skipped = noops
	if conflicts > 0 {
		return summary, fmt.Errorf("update_by_query completed with %d version conflicts — re-run after resolving concurrent writers", conflicts)
	}
	return summary, err
}

func buildOSQuery(oldSlug string) map[string]any {
	return map[string]any{
		"bool": map[string]any{
			"filter": []any{
				map[string]any{"term": map[string]any{"latest": true}},
			},
			"should": []any{
				map[string]any{"term": map[string]any{"data.project_slug": oldSlug}},
				map[string]any{"term": map[string]any{"data.slug": oldSlug}},
				map[string]any{"term": map[string]any{"tags": "project_slug:" + oldSlug}},
				map[string]any{"term": map[string]any{"object_ref": "project:" + oldSlug}},
				map[string]any{"term": map[string]any{"parent_refs": "project:" + oldSlug}},
			},
			"minimum_should_match": 1,
		},
	}
}

func (r *Runner) auditOpenSearch(ctx context.Context, query map[string]any) (int, error) {
	body, err := jsonBody(map[string]any{
		"size":             0,
		"track_total_hits": true,
		"query":            query,
	})
	if err != nil {
		return 0, err
	}

	res, err := r.openSearch.Search(
		r.openSearch.Search.WithContext(ctx),
		r.openSearch.Search.WithIndex("resources"),
		r.openSearch.Search.WithBody(body),
	)
	if err != nil {
		return 0, fmt.Errorf("search request failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.IsError() {
		raw, _ := io.ReadAll(res.Body)
		return 0, fmt.Errorf("search error %s: %s", res.Status(), raw)
	}

	var result struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode search response: %w", err)
	}

	return result.Hits.Total.Value, nil
}

func (r *Runner) updateOpenSearch(ctx context.Context, query map[string]any, oldSlug, newSlug string) (examined, updated, noops, conflicts int, err error) {
	painlessSource := `
def oldSlug=params.oldSlug;
def newSlug=params.newSlug;
boolean changed=false;
def data=ctx._source.get('data');
if (data instanceof Map) {
  if (oldSlug.equals(data.get('project_slug'))) { data.put('project_slug', newSlug); changed=true; }
  if (oldSlug.equals(data.get('slug'))) { data.put('slug', newSlug); changed=true; }
}
def tags=ctx._source.get('tags');
if (tags instanceof List) {
  for (int i=0; i<tags.size(); i++) {
    String tag=(String) tags.get(i);
    if (('project_slug:'+oldSlug).equals(tag)) { tags.set(i, 'project_slug:'+newSlug); changed=true; }
  }
}
String objectRef=(String) ctx._source.get('object_ref');
if (objectRef!=null && ('project:'+oldSlug).equals(objectRef)) { ctx._source.put('object_ref', 'project:'+newSlug); changed=true; }
def parentRefs=ctx._source.get('parent_refs');
if (parentRefs instanceof List) {
  for (int i=0; i<parentRefs.size(); i++) {
    String ref=(String) parentRefs.get(i);
    if (('project:'+oldSlug).equals(ref)) { parentRefs.set(i, 'project:'+newSlug); changed=true; }
  }
}
String ft=(String) ctx._source.get('fulltext');
if (ft!=null && ft.contains(oldSlug)) { ctx._source.put('fulltext', ft.replace(oldSlug, newSlug)); changed=true; }
def aliases=ctx._source.get('name_and_aliases');
if (aliases instanceof List) {
  for (int i=0; i<aliases.size(); i++) {
    String alias=(String) aliases.get(i);
    if (oldSlug.equals(alias)) { aliases.set(i, newSlug); changed=true; }
  }
}
if (!changed) { ctx.op='noop'; }
`

	body, err := jsonBody(map[string]any{
		"query": query,
		"script": map[string]any{
			"lang":   "painless",
			"source": strings.TrimSpace(painlessSource),
			"params": map[string]any{
				"oldSlug": oldSlug,
				"newSlug": newSlug,
			},
		},
	})
	if err != nil {
		return 0, 0, 0, 0, err
	}

	res, err := r.openSearch.UpdateByQuery(
		[]string{"resources"},
		r.openSearch.UpdateByQuery.WithContext(ctx),
		r.openSearch.UpdateByQuery.WithBody(body),
		r.openSearch.UpdateByQuery.WithConflicts("proceed"),
	)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("update_by_query request failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.IsError() {
		raw, _ := io.ReadAll(res.Body)
		return 0, 0, 0, 0, fmt.Errorf("update_by_query error %s: %s", res.Status(), raw)
	}

	var result struct {
		Total            int               `json:"total"`
		Updated          int               `json:"updated"`
		VersionConflicts int               `json:"version_conflicts"`
		Noops            int               `json:"noops"`
		Failures         []json.RawMessage `json:"failures"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to decode update_by_query response: %w", err)
	}
	if len(result.Failures) > 0 {
		return result.Total, result.Updated, result.Noops, result.VersionConflicts,
			fmt.Errorf("update_by_query completed with %d document failures", len(result.Failures))
	}

	return result.Total, result.Updated, result.Noops, result.VersionConflicts, nil
}

func (r *Runner) runNATS(ctx context.Context, oldSlug, newSlug string, dryRun bool, concurrency int, buckets []string) (Summary, error) {
	summary := Summary{Store: "nats", DryRun: dryRun}

	for _, bucket := range buckets {
		bucketSummary, err := r.migrateBucket(ctx, bucket, oldSlug, newSlug, dryRun, concurrency)
		if err != nil {
			if errors.Is(err, jetstream.ErrBucketNotFound) {
				slog.WarnContext(ctx, "bucket not found, skipping", "bucket", bucket)
				continue
			}
			slog.ErrorContext(ctx, "bucket migration failed", "bucket", bucket, "error", err)
			summary.Errors++
			continue
		}

		summary.Total += bucketSummary.Total
		summary.Updated += bucketSummary.Updated
		summary.Skipped += bucketSummary.Skipped
		summary.Failed += bucketSummary.Failed

		slog.InfoContext(ctx, "bucket migration complete",
			"bucket", bucket,
			"total", bucketSummary.Total,
			"updated", bucketSummary.Updated,
			"skipped", bucketSummary.Skipped,
			"failed", bucketSummary.Failed,
			"dry_run", dryRun,
		)
	}

	if summary.Errors > 0 {
		return summary, fmt.Errorf("%d bucket(s) failed to open or list — migration incomplete", summary.Errors)
	}
	if summary.Failed > 0 {
		return summary, fmt.Errorf("%d records failed to update across all buckets", summary.Failed)
	}
	return summary, nil
}

type bucketStats struct {
	Total   int
	Updated int
	Skipped int
	Failed  int
}

func (r *Runner) migrateBucket(ctx context.Context, bucket, oldSlug, newSlug string, dryRun bool, concurrency int) (*bucketStats, error) {
	kvStore, err := r.jetStream.KeyValue(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to open KV bucket %q: %w", bucket, err)
	}

	fields := BucketFieldsFor(bucket)

	slog.InfoContext(ctx, "scanning bucket", "bucket", bucket, "slug_fields", fields)

	keys, err := kvStore.ListKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys in bucket %q: %w", bucket, err)
	}
	defer keys.Stop() //nolint:errcheck

	var recordKeys []string
	for key := range keys.Keys() {
		if strings.HasPrefix(key, "lookup/") || strings.HasPrefix(key, "slug/") {
			continue
		}
		recordKeys = append(recordKeys, key)
	}

	slog.InfoContext(ctx, "found records in bucket", "bucket", bucket, "count", len(recordKeys))
	if dryRun {
		slog.InfoContext(ctx, "dry run mode - no writes will be made", "bucket", bucket)
	}

	stats := &bucketStats{Total: len(recordKeys)}
	var statsMu sync.Mutex
	var processed atomic.Int64

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for _, key := range recordKeys {
		key := key
		g.Go(func() error {
			err := processKVRecord(gCtx, kvStore, key, fields, oldSlug, newSlug, dryRun)

			statsMu.Lock()
			if err != nil {
				if errors.Is(err, errSlugMismatch) {
					stats.Skipped++
				} else {
					slog.ErrorContext(gCtx, "failed to process record",
						"bucket", bucket, "key", key, "error", err)
					stats.Failed++
				}
			} else {
				stats.Updated++
			}
			statsMu.Unlock()

			if n := processed.Add(1); n%1000 == 0 || int(n) == stats.Total {
				statsMu.Lock()
				u, sk, f := stats.Updated, stats.Skipped, stats.Failed
				statsMu.Unlock()
				slog.InfoContext(gCtx, "progress",
					"bucket", bucket,
					"processed", n, "total", stats.Total,
					"updated", u, "skipped", sk, "failed", f,
				)
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return stats, err
	}

	return stats, nil
}

func processKVRecord(ctx context.Context, kvStore jetstream.KeyValue, key string, fields []string, oldSlug, newSlug string, dryRun bool) error {
	entry, err := kvStore.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	raw := make(map[string]json.RawMessage)
	if err := json.Unmarshal(entry.Value(), &raw); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

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

// BucketFieldsFor returns the JSON field names that hold a project slug for the
// given bucket name.
func BucketFieldsFor(bucket string) []string {
	if fields, ok := bucketSlugFields[bucket]; ok {
		return fields
	}
	return []string{"project_slug"}
}

// ParseBuckets splits a comma-separated bucket list.
func ParseBuckets(s string) []string {
	var out []string
	for _, b := range strings.Split(s, ",") {
		b = strings.TrimSpace(b)
		if b != "" {
			out = append(out, b)
		}
	}
	return out
}

// RedactURL removes credentials from a URL for safe logging.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid>"
	}
	if u.User != nil {
		u.User = url.User("REDACTED")
	}
	return u.String()
}

func jsonBody(v any) (io.Reader, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}
	return bytes.NewReader(b), nil
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
