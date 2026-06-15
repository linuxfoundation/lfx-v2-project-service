// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// keyPrefixOrgWorkspaces is the NATS KV key prefix for org workspace records.
// orgUID must be a valid UUID — callers are responsible for sanitising input
// before reaching this layer. HTTP callers are safe because Goa validates path
// params as UUIDs; non-HTTP callers (RPC, admin tools) must do the same.
const keyPrefixOrgWorkspaces = "org-workspaces."

// GetWorkspaces returns the workspace document for a b2b_org and the current
// KV revision. Returns (nil, 0, nil) when no record exists yet.
func (s *Storage) GetWorkspaces(ctx context.Context, orgUID string) (*model.OrgWorkspaces, uint64, error) {
	if orgUID == "" {
		return nil, 0, errs.NewValidation("orgUID cannot be empty")
	}
	return getDocWithRevision[model.OrgWorkspaces](ctx, s, constants.KVBucketNameOrgWorkspaces, keyPrefixOrgWorkspaces+orgUID)
}

// UpdateWorkspaces persists org workspaces. The org UID is carried in workspaces.OrgUID.
// When revision > 0 uses optimistic-locking (kv.Update); when revision == 0 uses
// kv.Create (exclusive create — fails on concurrent first-write, returns Conflict).
func (s *Storage) UpdateWorkspaces(ctx context.Context, workspaces *model.OrgWorkspaces, revision uint64) error {
	if workspaces == nil {
		return errs.NewValidation("workspaces cannot be nil")
	}
	if workspaces.OrgUID == "" {
		return errs.NewValidation("workspaces.OrgUID cannot be empty")
	}
	return updateDocWithRevision(ctx, s, constants.KVBucketNameOrgWorkspaces, keyPrefixOrgWorkspaces+workspaces.OrgUID, "org workspaces", workspaces, revision)
}
