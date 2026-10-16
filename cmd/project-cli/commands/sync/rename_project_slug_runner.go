// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/nats-io/nats.go/jetstream"
	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
	"golang.org/x/sync/errgroup"

	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// projectsBucket is the only bucket that carries a "slug/<slug>" -> uid
// lookup index (see GetProjectUIDFromSlug in internal/infrastructure/nats),
// which needs its own reconciliation logic distinct from the plain
// field-rename path used by every other bucket.
const projectsBucket = constants.KVStoreNameProjects

// defaultNATSBuckets are the KV buckets scanned during a slug rename. Besides
// the project buckets it includes the committee buckets, whose records
// denormalise the owning project's slug in a project_slug field (see
// bucketSlugFields) and would otherwise point at a slug that no longer exists.
var defaultNATSBuckets = []string{
	"committee-members",
	"committees",
	"committee-settings",
	projectsBucket,
	constants.KVStoreNameProjectSettings,
}

// bucketSlugFields maps a KV bucket name to the JSON field name(s) that hold
// the project slug.
var bucketSlugFields = map[string][]string{
	"committee-members":                  {"project_slug"},
	"committees":                         {"project_slug"},
	"committee-settings":                 {"project_slug"},
	projectsBucket:                       {"slug"},
	constants.KVStoreNameProjectSettings: {"project_slug"},
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
//
// The projects bucket's slug index reservation (reserveSlugIndex) only
// guards against concurrent runs of this migration; it cannot protect
// against project-api's own live writes, which use an unconditional Put for
// slug mappings (see putProjectSlugMapping in internal/infrastructure/nats).
// Run this migration only during a maintenance window with project-api
// write traffic stopped.
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
		buckets = defaultNATSBuckets
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
