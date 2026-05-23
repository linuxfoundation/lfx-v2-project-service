// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

const keyPrefixOrgSettings = "org-settings."

// GetOrgSettings returns the settings for a b2b_org and the current KV revision.
// Returns (nil, 0, nil) when no record exists yet.
func (s *Storage) GetOrgSettings(ctx context.Context, orgUID string) (*model.OrgSettings, uint64, error) {
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

	var settings model.OrgSettings
	if err := json.Unmarshal(entry.Value(), &settings); err != nil {
		return nil, 0, errs.NewUnexpected("failed to unmarshal org settings", err)
	}

	return &settings, entry.Revision(), nil
}

// PutOrgSettings persists org settings. When revision > 0 uses optimistic-
// locking (kv.Update); when revision == 0 uses kv.Put (unconditional create).
func (s *Storage) PutOrgSettings(ctx context.Context, orgUID string, settings *model.OrgSettings, revision uint64) error {
	if orgUID == "" {
		return errs.NewValidation("orgUID cannot be empty")
	}
	if settings == nil {
		return errs.NewValidation("settings cannot be nil")
	}

	kv, ok := s.client.kvStore[constants.KVBucketNameOrgSettings]
	if !ok {
		return errs.NewUnexpected(fmt.Sprintf("KV bucket %q not initialized", constants.KVBucketNameOrgSettings))
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return errs.NewUnexpected("failed to marshal org settings", err)
	}

	key := keyPrefixOrgSettings + orgUID
	if revision > 0 {
		newRev, err := kv.Update(ctx, key, data, revision)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				return errs.NewNotFound("org settings not found for update")
			}
			return errs.NewConflict("org settings were modified concurrently, please retry")
		}
		slog.DebugContext(ctx, "updated org settings",
			"org_uid", orgUID, "old_revision", revision, "new_revision", newRev)
	} else {
		if _, err := kv.Put(ctx, key, data); err != nil {
			return errs.NewUnexpected("failed to put org settings", err)
		}
		slog.DebugContext(ctx, "created org settings", "org_uid", orgUID)
	}

	return nil
}
