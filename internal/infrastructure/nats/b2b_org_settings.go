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

// keyPrefixOrgSettings is the NATS KV key prefix for org settings records.
// orgUID must be a valid UUID — callers are responsible for sanitising input
// before reaching this layer. HTTP callers are safe because Goa validates path
// params as UUIDs; non-HTTP callers (RPC, admin tools) must do the same.
const keyPrefixOrgSettings = "org-settings."

// keyFmtInviteIndex is the key format for the InviteUUID→orgUID secondary index.
// Index entries live in the same org-settings KV bucket under a "lookup/" prefix
// so they never collide with primary "org-settings.{uid}" keys.
const keyFmtInviteIndex = "lookup/org-settings-invite/%s"

// GetSettings returns the settings for a b2b_org and the current KV revision.
// Returns (nil, 0, nil) when no record exists yet.
func (s *Storage) GetSettings(ctx context.Context, orgUID string) (*model.B2BOrgSettings, uint64, error) {
	if orgUID == "" {
		return nil, 0, errs.NewValidation("orgUID cannot be empty")
	}

	kv, ok := s.client.kvStore[constants.KVBucketNameOrgSettings]
	if !ok {
		return nil, 0, errs.NewUnexpected(fmt.Sprintf("KV bucket %q not initialized", constants.KVBucketNameOrgSettings))
	}

	entry, err := kv.Get(ctx, keyPrefixOrgSettings+orgUID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, 0, nil
		}
		return nil, 0, errs.NewUnexpected("failed to get org settings", err)
	}

	var settings model.B2BOrgSettings
	if err := json.Unmarshal(entry.Value(), &settings); err != nil {
		return nil, 0, errs.NewUnexpected("failed to unmarshal org settings", err)
	}

	return &settings, entry.Revision(), nil
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

// orgSettingsKV returns the org-settings KV store, or an error if it has not been initialised.
func (s *Storage) orgSettingsKV() (jetstream.KeyValue, error) {
	kv, ok := s.client.kvStore[constants.KVBucketNameOrgSettings]
	if !ok {
		return nil, errs.NewUnexpected(fmt.Sprintf("KV bucket %q not initialized", constants.KVBucketNameOrgSettings))
	}
	return kv, nil
}

// PutInviteIndex writes (or overwrites) the InviteUUID→orgUID secondary-index entry
// in the org-settings KV bucket. Last-write-wins: a resend that issues a new InviteUUID
// must call this again with the new UUID; the old key is cleaned up by reconcileInviteIndex.
func (s *Storage) PutInviteIndex(ctx context.Context, inviteUUID, orgUID string) error {
	kv, err := s.orgSettingsKV()
	if err != nil {
		return err
	}
	key := fmt.Sprintf(keyFmtInviteIndex, inviteUUID)
	if _, err := kv.Put(ctx, key, []byte(orgUID)); err != nil {
		return errs.NewUnexpected("failed to put invite index entry", err)
	}
	slog.DebugContext(ctx, "invite index: put", "invite_uuid", inviteUUID, "org_uid", orgUID)
	return nil
}

// LookupInviteOrgUID returns the orgUID stored for the given InviteUUID.
// Returns a NotFound error when no index entry exists (index miss or not yet written).
func (s *Storage) LookupInviteOrgUID(ctx context.Context, inviteUUID string) (string, error) {
	kv, err := s.orgSettingsKV()
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf(keyFmtInviteIndex, inviteUUID)
	entry, err := kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return "", errs.NewNotFound("invite index entry not found")
		}
		return "", errs.NewUnexpected("failed to look up invite index entry", err)
	}
	return string(entry.Value()), nil
}

// DeleteInviteIndex removes the InviteUUID→orgUID secondary-index entry.
// Not-found is tolerated so callers can issue a best-effort delete without
// checking whether the key was ever written.
func (s *Storage) DeleteInviteIndex(ctx context.Context, inviteUUID string) error {
	kv, err := s.orgSettingsKV()
	if err != nil {
		return err
	}
	key := fmt.Sprintf(keyFmtInviteIndex, inviteUUID)
	if err := kv.Delete(ctx, key); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil // idempotent
		}
		return errs.NewUnexpected("failed to delete invite index entry", err)
	}
	slog.DebugContext(ctx, "invite index: delete", "invite_uuid", inviteUUID)
	return nil
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

	kv, ok := s.client.kvStore[constants.KVBucketNameOrgSettings]
	if !ok {
		return errs.NewUnexpected(fmt.Sprintf("KV bucket %q not initialized", constants.KVBucketNameOrgSettings))
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return errs.NewUnexpected("failed to marshal org settings", err)
	}

	key := keyPrefixOrgSettings + settings.UID
	if revision > 0 {
		newRev, err := kv.Update(ctx, key, data, revision)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				return errs.NewNotFound("org settings not found for update")
			}
			if errors.Is(err, jetstream.ErrKeyExists) {
				return errs.NewConflict("org settings were modified concurrently, please retry")
			}
			return errs.NewUnexpected("failed to update org settings", err)
		}
		slog.DebugContext(ctx, "updated org settings",
			"org_uid", settings.UID, "old_revision", revision, "new_revision", newRev)
	} else {
		newRev, err := kv.Create(ctx, key, data)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				return errs.NewConflict("org settings were created concurrently, please retry")
			}
			return errs.NewUnexpected("failed to create org settings", err)
		}
		slog.DebugContext(ctx, "created org settings", "org_uid", settings.UID, "revision", newRev)
	}

	return nil
}
