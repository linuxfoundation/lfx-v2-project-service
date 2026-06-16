// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// ── Authoritative KV helpers ─────────────────────────────────────────────────
//
// getDocWithRevision and updateDocWithRevision are package-level generics shared
// by all authoritative (no-TTL) KV buckets in this package (org-settings,
// org-workspaces). They are intentionally co-located with the first authoritative
// bucket (org-settings) rather than in a separate file.

// getDocWithRevision fetches and JSON-decodes a document from the named authoritative
// KV bucket. Returns (nil, 0, nil) when the key does not exist (no-error miss).
// Unlike getCached, there is no TTL envelope — the raw document is stored directly.
func getDocWithRevision[T any](ctx context.Context, s *Storage, bucket, key string) (*T, uint64, error) {
	kv, ok := s.client.kvStore[bucket]
	if !ok {
		return nil, 0, errs.NewUnexpected(fmt.Sprintf("KV bucket %q not initialized", bucket))
	}
	entry, err := kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, 0, nil
		}
		return nil, 0, errs.NewUnexpected(fmt.Sprintf("failed to get key %q from bucket %q", key, bucket), err)
	}
	var doc T
	if unmarshalErr := json.Unmarshal(entry.Value(), &doc); unmarshalErr != nil {
		return nil, 0, errs.NewUnexpected(fmt.Sprintf("failed to unmarshal key %q from bucket %q", key, bucket), unmarshalErr)
	}
	return &doc, entry.Revision(), nil
}

// updateDocWithRevision marshals doc and writes it to key in the named authoritative
// KV bucket. docLabel is used in log and error messages (e.g. "org settings").
//
// revision == 0 → exclusive create (returns Conflict on concurrent first-write).
// revision > 0  → optimistic update (returns Conflict on revision mismatch).
func updateDocWithRevision[T any](ctx context.Context, s *Storage, bucket, key, docLabel string, doc *T, revision uint64) error {
	kv, ok := s.client.kvStore[bucket]
	if !ok {
		return errs.NewUnexpected(fmt.Sprintf("KV bucket %q not initialized", bucket))
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return errs.NewUnexpected("failed to marshal "+docLabel, err)
	}
	if revision > 0 {
		newRev, err := kv.Update(ctx, key, data, revision)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				return errs.NewNotFound(docLabel + " not found for update")
			}
			if errors.Is(err, jetstream.ErrKeyExists) {
				return errs.NewConflict(docLabel + " were modified concurrently, please retry")
			}
			return errs.NewUnexpected("failed to update "+docLabel, err)
		}
		slog.DebugContext(ctx, "updated "+docLabel, "key", key, "old_revision", revision, "new_revision", newRev)
	} else {
		newRev, err := kv.Create(ctx, key, data)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				return errs.NewConflict(docLabel + " were created concurrently, please retry")
			}
			return errs.NewUnexpected("failed to create "+docLabel, err)
		}
		slog.DebugContext(ctx, "created "+docLabel, "key", key, "revision", newRev)
	}
	return nil
}

// deleteDoc removes a key from the named authoritative KV bucket.
// Safe to call when the key does not exist (ErrKeyNotFound is treated as success).
func deleteDoc(ctx context.Context, s *Storage, bucket, key string) error {
	kv, ok := s.client.kvStore[bucket]
	if !ok {
		return errs.NewUnexpected(fmt.Sprintf("KV bucket %q not initialized", bucket))
	}
	if err := kv.Delete(ctx, key); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return errs.NewUnexpected(fmt.Sprintf("failed to delete key %q from bucket %q", key, bucket), err)
	}
	return nil
}

// ── Org settings ─────────────────────────────────────────────────────────────

// keyPrefixOrgSettings is the NATS KV key prefix for org settings records.
// orgUID must be a valid UUID — callers are responsible for sanitising input
// before reaching this layer. HTTP callers are safe because Goa validates path
// params as UUIDs; non-HTTP callers (RPC, admin tools) must do the same.
const keyPrefixOrgSettings = "org-settings."

// GetSettings returns the settings for a b2b_org and the current KV revision.
// Returns (nil, 0, nil) when no record exists yet.
func (s *Storage) GetSettings(ctx context.Context, orgUID string) (*model.B2BOrgSettings, uint64, error) {
	if orgUID == "" {
		return nil, 0, errs.NewValidation("orgUID cannot be empty")
	}
	return getDocWithRevision[model.B2BOrgSettings](ctx, s, constants.KVBucketNameOrgSettings, keyPrefixOrgSettings+orgUID)
}

// ListSettingsOrgUIDs returns the org UIDs for all active keys in the org-settings KV
// bucket. Returns an empty slice when the bucket is empty (jetstream.ErrNoKeysFound).
func (s *Storage) ListSettingsOrgUIDs(ctx context.Context) ([]string, error) {
	kv, ok := s.client.kvStore[constants.KVBucketNameOrgSettings]
	if !ok {
		return nil, errs.NewUnexpected(fmt.Sprintf("KV bucket %q not initialized", constants.KVBucketNameOrgSettings))
	}

	keys, err := kv.Keys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return []string{}, nil
		}
		return nil, errs.NewUnexpected("failed to list org-settings keys", err)
	}

	uids := make([]string, 0, len(keys))
	for _, k := range keys {
		// Skip secondary-index keys explicitly; they share the same bucket but
		// must never appear in the org-UID list used for scanning.
		if strings.HasPrefix(k, "lookup/") {
			continue
		}
		uid := strings.TrimPrefix(k, keyPrefixOrgSettings)
		if uid != k { // prefix was present
			uids = append(uids, uid)
		}
	}
	return uids, nil
}

// UpdateSettings persists org settings. The org UID is carried in settings.UID.
// When revision > 0 uses optimistic-locking (kv.Update); when revision == 0 uses
// kv.Create (exclusive create — fails on concurrent first-write, returns Conflict).
func (s *Storage) UpdateSettings(ctx context.Context, settings *model.B2BOrgSettings, revision uint64) error {
	if settings == nil {
		return errs.NewValidation("settings cannot be nil")
	}
	if settings.UID == "" {
		return errs.NewValidation("settings.UID cannot be empty")
	}
	return updateDocWithRevision(ctx, s, constants.KVBucketNameOrgSettings, keyPrefixOrgSettings+settings.UID, "org settings", settings, revision)
}
