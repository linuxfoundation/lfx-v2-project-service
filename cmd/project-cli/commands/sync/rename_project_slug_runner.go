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

// errSlugIndexCollision indicates the new slug's lookup index already points
// at a different project than the one being renamed.
var errSlugIndexCollision = errors.New("new slug is already indexed to a different project")

// projectsBucket is the only bucket that carries a "slug/<slug>" -> uid
// lookup index (see GetProjectUIDFromSlug in internal/infrastructure/nats),
// which needs its own reconciliation logic distinct from the plain
// field-rename path used by every other bucket.
const projectsBucket = "projects"

// DefaultNATSBuckets are the KV buckets scanned during a slug rename migration.
var DefaultNATSBuckets = []string{
	"committee-members",
	"committees",
	"committee-settings",
	projectsBucket,
	"project-settings",
}

// bucketSlugFields maps a KV bucket name to the JSON field name(s) that hold
// the project slug.
var bucketSlugFields = map[string][]string{
	"committee-members":  {"project_slug"},
	"committees":         {"project_slug"},
	"committee-settings": {"project_slug"},
	projectsBucket:       {"slug"},
	"project-settings":   {"project_slug"},
}

// RenameSlugOptions configures a rename-project-slug run.
type RenameSlugOptions struct {
	OldSlug     string
	NewSlug     string
	DryRun      bool
	Concurrency int
	NATSBuckets []string
}

type renameSlugSummary struct {
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

// RenameSlugRunner executes slug rename migrations using shared infrastructure clients.
type RenameSlugRunner struct {
	openSearch *opensearchgo.Client
	jetStream  jetstream.JetStream
}

// NewRenameSlugRunner creates a RenameSlugRunner backed by the provided clients.
func NewRenameSlugRunner(openSearch *opensearchgo.Client, jetStream jetstream.JetStream) *RenameSlugRunner {
	return &RenameSlugRunner{
		openSearch: openSearch,
		jetStream:  jetStream,
	}
}

// Run renames oldSlug to newSlug across OpenSearch and NATS KV stores.
func (r *RenameSlugRunner) Run(ctx context.Context, opts RenameSlugOptions) error {
	if opts.OldSlug == "" || opts.NewSlug == "" {
		return fmt.Errorf("old slug and new slug are required")
	}
	if opts.OldSlug == opts.NewSlug {
		return fmt.Errorf("old slug and new slug must differ")
	}
	if opts.Concurrency < 1 {
		return fmt.Errorf("concurrency must be at least 1")
	}
	if r.openSearch == nil {
		return fmt.Errorf("OpenSearch client is required")
	}
	if r.jetStream == nil {
		return fmt.Errorf("JetStream client is required")
	}

	buckets := opts.NATSBuckets
	if len(buckets) == 0 {
		buckets = DefaultNATSBuckets
	}

	slog.InfoContext(ctx, "starting rename-project-slug")

	summary, err := r.runOpenSearch(ctx, opts.OldSlug, opts.NewSlug, opts.DryRun)
	if err != nil {
		summary.Failed = 1
	}
	logRenameSlugSummary(ctx, summary)
	if err != nil {
		return fmt.Errorf("opensearch migration failed: %w", err)
	}

	summary, err = r.runNATS(ctx, opts.OldSlug, opts.NewSlug, opts.DryRun, opts.Concurrency, buckets)
	logRenameSlugSummary(ctx, summary)
	if err != nil {
		return fmt.Errorf("nats migration failed: %w", err)
	}

	return nil
}

func logRenameSlugSummary(ctx context.Context, summary renameSlugSummary) {
	attrs := []any{
		"store", summary.Store,
		"dry_run", summary.DryRun,
		"total", summary.Total,
		"skipped", summary.Skipped,
		"failed", summary.Failed,
	}
	if summary.DryRun {
		attrs = append(attrs, "matched", summary.Updated)
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

func (r *RenameSlugRunner) runOpenSearch(ctx context.Context, oldSlug, newSlug string, dryRun bool) (renameSlugSummary, error) {
	summary := renameSlugSummary{Store: "opensearch", DryRun: dryRun}
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
	if conflicts > 0 || err != nil {
		if conflicts > 0 && err != nil {
			return summary, fmt.Errorf("update_by_query completed with %d version conflicts: %w", conflicts, err)
		}
		if err != nil {
			return summary, err
		}
		return summary, fmt.Errorf("update_by_query completed with %d version conflicts — re-run after resolving concurrent writers", conflicts)
	}
	return summary, nil
}

// buildOSQuery matches documents in the shared resources index by slug-bearing
// fields. project-service indexes object_ref/parent_refs with project UIDs, not
// slugs, so those fields are not used as query clauses here.
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
			},
			"minimum_should_match": 1,
		},
	}
}

func (r *RenameSlugRunner) auditOpenSearch(ctx context.Context, query map[string]any) (int, error) {
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

func (r *RenameSlugRunner) updateOpenSearch(ctx context.Context, query map[string]any, oldSlug, newSlug string) (examined, updated, noops, conflicts int, err error) {
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
// Legacy slug-based refs in the shared index, if any.
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

func (r *RenameSlugRunner) runNATS(ctx context.Context, oldSlug, newSlug string, dryRun bool, concurrency int, buckets []string) (renameSlugSummary, error) {
	summary := renameSlugSummary{Store: "nats", DryRun: dryRun}

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

		countKey := "updated"
		if dryRun {
			countKey = "matched"
		}
		slog.InfoContext(ctx, "bucket migration complete",
			"bucket", bucket,
			"total", bucketSummary.Total,
			countKey, bucketSummary.Updated,
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

func (r *RenameSlugRunner) migrateBucket(ctx context.Context, bucket, oldSlug, newSlug string, dryRun bool, concurrency int) (*bucketStats, error) {
	kvStore, err := r.jetStream.KeyValue(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to open KV bucket %q: %w", bucket, err)
	}

	fields := bucketFieldsFor(bucket)

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
			var err error
			if bucket == projectsBucket {
				err = processProjectRecord(gCtx, kvStore, key, fields, oldSlug, newSlug, dryRun)
			} else {
				err = processKVRecord(gCtx, kvStore, key, fields, oldSlug, newSlug, dryRun)
			}

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
	if dryRun {
		slog.InfoContext(ctx, "dry run: would delete stale slug index", "uid", uid, "key", "slug/"+oldSlug)
		return nil
	}

	existingOld, err := kv.Get(ctx, "slug/"+oldSlug)
	switch {
	case err == nil:
		if string(existingOld.Value()) != uid {
			slog.DebugContext(ctx, "stale slug index no longer owned by this project, leaving in place",
				"uid", uid, "key", "slug/"+oldSlug)
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

func bucketFieldsFor(bucket string) []string {
	if fields, ok := bucketSlugFields[bucket]; ok {
		return fields
	}
	return []string{"project_slug"}
}

func parseNATSBuckets(s string) []string {
	var out []string
	for _, b := range strings.Split(s, ",") {
		b = strings.TrimSpace(b)
		if b != "" {
			out = append(out, b)
		}
	}
	return out
}

func redactURL(raw string) string {
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
