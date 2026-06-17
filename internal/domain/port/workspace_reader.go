// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// OrgWorkspacesReader reads b2b_org workspace documents from the
// "org-workspaces" NATS KV bucket. Authoritative state, no MaxAge eviction.
type OrgWorkspacesReader interface {
	// GetWorkspaces returns the workspace document for the given b2b_org UID
	// and the current KV revision (needed for optimistic-locking on UpdateWorkspaces).
	// Returns (nil, 0, nil) when no record exists yet.
	GetWorkspaces(ctx context.Context, orgUID string) (*model.OrgWorkspaces, uint64, error)
}
