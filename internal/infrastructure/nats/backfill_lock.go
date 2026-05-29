// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
)

const (
	backfillLockKeyPrefix = "backfill-lock/full/"
	backfillLockStaleTTL  = 2 * time.Hour
)

// AcquireFullRunLock attempts to acquire a per-type, cluster-wide lock for a
// full backfill reindex run. It uses an atomic NATS KV Create (revision 0) so
// only one pod can hold the lock per type at a time.
//
// If the lock is already held and not stale, it returns an error immediately —
// the caller should log and skip that type. If the held lock is older than
// backfillLockStaleTTL (e.g. the pod that acquired it crashed), the lock is
// force-acquired by deleting and re-creating.
//
// The returned release func deletes the lock key. Always defer it.
func AcquireFullRunLock(ctx context.Context, client *NATSClient, runID, sfType string) (release func(), err error) {
	kv, ok := client.kvStore[constants.KVBucketNameCache]
	if !ok {
		return nil, fmt.Errorf("KV bucket %q not initialised", constants.KVBucketNameCache)
	}

	key := backfillLockKeyPrefix + sfType
	value := []byte(runID + "|" + time.Now().UTC().Format(time.RFC3339))

	_, err = kv.Create(ctx, key, value)
	if err == nil {
		return lockRelease(ctx, kv, key), nil
	}

	if !errors.Is(err, jetstream.ErrKeyExists) {
		return nil, fmt.Errorf("acquiring backfill lock for %q: %w", sfType, err)
	}

	// Key exists — check whether the holder is stale.
	entry, getErr := kv.Get(ctx, key)
	if getErr != nil {
		return nil, fmt.Errorf("lock held for %q (could not inspect: %w)", sfType, getErr)
	}

	if !isStaleBackfillLock(entry.Value()) {
		return nil, fmt.Errorf("full reindex of %q already in progress (run_id prefix: %.8s)", sfType, entry.Value())
	}

	slog.WarnContext(ctx, "force-acquiring stale backfill lock",
		"type", sfType,
		"stale_value", string(entry.Value()))

	if delErr := kv.Delete(ctx, key); delErr != nil {
		return nil, fmt.Errorf("could not remove stale lock for %q: %w", sfType, delErr)
	}
	if _, createErr := kv.Create(ctx, key, value); createErr != nil {
		return nil, fmt.Errorf("re-acquiring lock for %q after stale delete: %w", sfType, createErr)
	}
	return lockRelease(ctx, kv, key), nil
}

func lockRelease(ctx context.Context, kv jetstream.KeyValue, key string) func() {
	return func() {
		// Use a fresh context for deletion — the caller's ctx may have expired
		// (e.g. the HTTP request context deadline firing after the goroutine finishes).
		deleteCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := kv.Delete(deleteCtx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "failed to release backfill lock", "key", key, "error", err)
		}
	}
}

// isStaleBackfillLock returns true when the lock value's embedded RFC3339
// timestamp is older than backfillLockStaleTTL.
func isStaleBackfillLock(value []byte) bool {
	parts := strings.SplitN(string(value), "|", 2)
	if len(parts) != 2 {
		return true // malformed — treat as stale so we can recover
	}
	t, err := time.Parse(time.RFC3339, parts[1])
	if err != nil {
		return true
	}
	return time.Since(t) > backfillLockStaleTTL
}
