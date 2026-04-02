// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/nats"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

// soqlB2BOrgsTemplateRef is the stable identifier used as the first segment of
// SOQL B2BOrg batch cache keys for SearchB2BOrgs queries.
const soqlB2BOrgsTemplateRef = "b2b-orgs"

// B2BOrgReader implements port.B2BOrgReader using Salesforce SOQL as the
// source of truth and NATS KV as a per-record TTL cache.
type B2BOrgReader struct {
	accounts *AccountRepo
	cache    *nats.Storage
}

// NewB2BOrgReader creates a B2BOrgReader backed by the given AccountRepo and
// NATS KV cache.
func NewB2BOrgReader(accounts *AccountRepo, cache *nats.Storage) *B2BOrgReader {
	return &B2BOrgReader{accounts: accounts, cache: cache}
}

// Ensure B2BOrgReader satisfies the port at compile time.
var _ port.B2BOrgReader = (*B2BOrgReader)(nil)

// ─── GetB2BOrg ───────────────────────────────────────────────────────────────

// GetB2BOrg returns the B2BOrg identified by its v2 UUID. Returns an error
// wrapping ErrNotFound if no record exists.
func (r *B2BOrgReader) GetB2BOrg(ctx context.Context, uid string) (*model.B2BOrg, error) {
	sfid, err := sfuuid.ToSFID(uid)
	if err != nil {
		return nil, fmt.Errorf("decoding b2b org UID %s: %w", uid, err)
	}

	org, err := r.accounts.FetchAccountBySFID(ctx, sfid)
	if err != nil {
		return nil, fmt.Errorf("fetching b2b org %s from Salesforce: %w", uid, err)
	}
	if org == nil {
		return nil, errs.NewNotFound("b2b org not found", fmt.Errorf("uid: %s", uid))
	}

	return org, nil
}

// ─── SearchB2BOrgs ───────────────────────────────────────────────────────────

// b2bOrgBatchKeyParams returns the stable SOQL key parameters for the given
// B2BOrg search filters. These are the same for all batches in a sequence.
func b2bOrgBatchKeyParams(filters model.B2BOrgFilters) []string {
	params := []string{string(filters.EffectiveSortOrder())}
	if filters.NameSearch != "" {
		// NameSearch is already lowercased by contract so "Google" and "google"
		// always produce the same cache key.
		params = append(params, "search:"+filters.NameSearch)
	}
	return params
}

// SearchB2BOrgs returns a single logical page of B2BOrg records filtered by
// the supplied predicates. Results are served from the NATS KV batch cache when
// available; on a miss the first SF batch is fetched synchronously and the
// remainder is swept in the background.
func (r *B2BOrgReader) SearchB2BOrgs(ctx context.Context, filters model.B2BOrgFilters, pageSize int) (model.B2BOrgPage, error) {
	// Decode the inbound cursor. An empty PageToken means first page of batch 0.
	cursor, err := DecodeCursor(filters.PageToken)
	if err != nil {
		return model.B2BOrgPage{}, fmt.Errorf("invalid page token: %w", err)
	}
	// Honour the page size agreed when the sequence was started.
	if cursor.PageSize > 0 {
		pageSize = cursor.PageSize
	}
	pageSize = NormalizePageSize(pageSize)

	batchParams := b2bOrgBatchKeyParams(filters)
	batchKey := nats.MembershipBatchCacheKey(soqlB2BOrgsTemplateRef, batchParams, cursor.BatchIndex, cursor.NextBatchIterator)

	// ── Cache read ───────────────────────────────────────────────────────────
	cacheResult, cacheErr := r.cache.GetB2BOrgBatch(ctx, batchKey)
	if cacheErr != nil {
		slog.WarnContext(ctx, "b2b org batch cache read error; falling through to Salesforce",
			"cache_key", batchKey,
			"error", cacheErr,
		)
	} else {
		switch cacheResult.Status {
		case nats.CacheStatusFresh:
			page, decodeErr := sliceCachedB2BOrgBatch(cacheResult.Value, cursor, pageSize, cursor.BatchIndex, batchParams)
			if decodeErr == nil {
				return page, nil
			}
			slog.WarnContext(ctx, "failed to decode fresh cached b2b org batch; re-fetching",
				"cache_key", batchKey, "error", decodeErr,
			)
		case nats.CacheStatusStale:
			page, decodeErr := sliceCachedB2BOrgBatch(cacheResult.Value, cursor, pageSize, cursor.BatchIndex, batchParams)
			if decodeErr == nil {
				if cursor.BatchIndex == 0 {
					r.refreshInBackground(func(bgCtx context.Context) error {
						return r.fetchAndCacheAllB2BOrgBatches(bgCtx, filters, batchParams)
					})
				}
				return page, nil
			}
			slog.WarnContext(ctx, "failed to decode stale cached b2b org batch; re-fetching",
				"cache_key", batchKey, "error", decodeErr,
			)
		}
		// CacheStatusExpired and CacheStatusMiss fall through to Salesforce.
	}

	// ── Cache miss / expired: fetch batch 0 from Salesforce ──────────────────
	if err := r.fetchAndCacheAllB2BOrgBatches(ctx, filters, batchParams); err != nil {
		return model.B2BOrgPage{}, err
	}

	freshResult, readErr := r.cache.GetB2BOrgBatch(ctx, nats.MembershipBatchCacheKey(soqlB2BOrgsTemplateRef, batchParams, 0, ""))
	if readErr != nil || freshResult.Status == nats.CacheStatusMiss {
		return model.B2BOrgPage{}, fmt.Errorf("failed to read back b2b org batch 0 after Salesforce fetch")
	}

	resetCursor := PageCursor{PageSize: pageSize}
	page, decodeErr := sliceCachedB2BOrgBatch(freshResult.Value, resetCursor, pageSize, 0, batchParams)
	if decodeErr != nil {
		return model.B2BOrgPage{}, fmt.Errorf("decoding freshly-fetched b2b org batch 0: %w", decodeErr)
	}
	return page, nil
}

// fetchAndCacheAllB2BOrgBatches fetches the first SF Account batch
// synchronously, then follows any remaining locators in a background goroutine.
// Batches are written tail-first so each entry's NextBatchIterator is valid
// before the preceding entry references it. Batch 0 is written last
// (synchronously) so the caller can immediately read it back.
func (r *B2BOrgReader) fetchAndCacheAllB2BOrgBatches(
	ctx context.Context,
	filters model.B2BOrgFilters,
	batchParams []string,
) error {
	firstBatch, err := r.accounts.FetchFirstAccountBatch(ctx, filters)
	if err != nil {
		return fmt.Errorf("fetching first b2b org batch from Salesforce: %w", err)
	}

	if firstBatch.SFLocator == "" {
		key0 := nats.MembershipBatchCacheKey(soqlB2BOrgsTemplateRef, batchParams, 0, "")
		return r.cache.PutB2BOrgBatch(ctx, key0, firstBatch.Records, "", firstBatch.TotalSize)
	}

	batch0Records := firstBatch.Records
	locator := firstBatch.SFLocator
	totalSize := firstBatch.TotalSize

	writeBatch0 := func(bgCtx context.Context, nextIter string) error {
		key0 := nats.MembershipBatchCacheKey(soqlB2BOrgsTemplateRef, batchParams, 0, "")
		return r.cache.PutB2BOrgBatch(bgCtx, key0, batch0Records, nextIter, totalSize)
	}

	sweepRemaining := func(bgCtx context.Context) error {
		remaining, _, fetchErr := QueryAllPages[soqlAccount](bgCtx, r.accounts.client, "", locator)
		if fetchErr != nil {
			return fmt.Errorf("sweeping remaining b2b org batches: %w", fetchErr)
		}

		converted := make([]*model.B2BOrg, 0, len(remaining))
		for _, acc := range remaining {
			org, convErr := convertSOQLToB2BOrg(bgCtx, acc)
			if convErr != nil {
				slog.WarnContext(bgCtx, "skipping account with invalid SFID during sweep",
					"sfid", acc.ID, "error", convErr,
				)
				continue
			}
			converted = append(converted, org)
		}

		chunks := splitB2BOrgBatches(converted, sfQueryBatchSize)
		iterators := make([]string, len(chunks))
		for i := range iterators {
			iterators[i] = newBatchIterator()
		}

		nextIter := ""
		for i := len(chunks) - 1; i >= 0; i-- {
			key := nats.MembershipBatchCacheKey(soqlB2BOrgsTemplateRef, batchParams, i+1, iterators[i])
			if putErr := r.cache.PutB2BOrgBatch(bgCtx, key, chunks[i], nextIter, 0); putErr != nil {
				slog.WarnContext(bgCtx, "failed to write b2b org batch to cache",
					"batch_index", i+1, "cache_key", key, "error", putErr,
				)
				return nil
			}
			nextIter = iterators[i]
		}

		return writeBatch0(bgCtx, nextIter)
	}

	key0 := nats.MembershipBatchCacheKey(soqlB2BOrgsTemplateRef, batchParams, 0, "")
	if putErr := r.cache.PutB2BOrgBatch(ctx, key0, batch0Records, "", totalSize); putErr != nil {
		return fmt.Errorf("writing b2b org batch 0: %w", putErr)
	}

	r.refreshInBackground(sweepRemaining)
	return nil
}

// refreshInBackground spawns a best-effort goroutine to re-fetch data from
// Salesforce and update the KV cache. The goroutine runs with a detached
// context so it is not cancelled when the HTTP response is sent. Errors are
// logged at debug level only and do not affect the caller.
func (r *B2BOrgReader) refreshInBackground(fetchFn func(ctx context.Context) error) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
		defer cancel()
		if err := fetchFn(ctx); err != nil {
			slog.Debug("background b2b org cache refresh failed", "error", err)
		}
	}()
}

// sliceCachedB2BOrgBatch decodes a B2BOrgBatchCacheEntry, slices the logical
// page at [cursor.BatchOffset : cursor.BatchOffset+pageSize], and builds the
// next-page cursor. batchIndex is the 0-based index of this batch in the
// sequence; batchParams is used to construct the next-batch cache key segment
// embedded in the cursor.
func sliceCachedB2BOrgBatch(
	entry *nats.B2BOrgBatchCacheEntry,
	cursor PageCursor,
	pageSize int,
	batchIndex int,
	batchParams []string,
) (model.B2BOrgPage, error) {
	if entry == nil {
		return model.B2BOrgPage{}, fmt.Errorf("nil cache entry")
	}

	records, err := entry.DecodeBatchRecords()
	if err != nil {
		return model.B2BOrgPage{}, fmt.Errorf("decoding b2b org batch records: %w", err)
	}

	start := cursor.BatchOffset
	if start > len(records) {
		start = len(records)
	}
	end := start + pageSize
	if end > len(records) {
		end = len(records)
	}
	page := records[start:end]

	nextOffset := end
	var nextCursor *PageCursor

	if nextOffset < len(records) {
		nextCursor = &PageCursor{
			BatchIndex:        batchIndex,
			BatchOffset:       nextOffset,
			PageSize:          pageSize,
			NextBatchIterator: cursor.NextBatchIterator,
		}
	} else if entry.NextBatchIterator != "" {
		nextCursor = &PageCursor{
			BatchIndex:        batchIndex + 1,
			BatchOffset:       0,
			PageSize:          pageSize,
			NextBatchIterator: entry.NextBatchIterator,
		}
	}

	var nextPageToken string
	if nextCursor != nil {
		nextPageToken = EncodeCursor(*nextCursor)
	}

	return model.B2BOrgPage{
		Orgs:          page,
		NextPageToken: nextPageToken,
		TotalSize:     entry.TotalSize,
	}, nil
}

// splitB2BOrgBatches partitions B2BOrg records into chunks of at most
// chunkSize. The last chunk may be smaller than chunkSize.
func splitB2BOrgBatches(records []*model.B2BOrg, chunkSize int) [][]*model.B2BOrg {
	if chunkSize <= 0 || len(records) == 0 {
		return nil
	}
	var chunks [][]*model.B2BOrg
	for i := 0; i < len(records); i += chunkSize {
		end := i + chunkSize
		if end > len(records) {
			end = len(records)
		}
		chunks = append(chunks, records[i:end])
	}
	return chunks
}
