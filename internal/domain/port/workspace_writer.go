// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// OrgWorkspacesWriter persists b2b_org workspace documents to the
// "org-workspaces" NATS KV bucket. Authoritative state, no MaxAge eviction.
type OrgWorkspacesWriter interface {
	// UpdateWorkspaces persists the workspace document for a b2b_org. The org UID
	// is carried in workspaces.OrgUID. When revision > 0, uses optimistic-locking
	// (kv.Update); when revision == 0, uses an exclusive create (fails with Conflict
	// if a concurrent first-write already landed). Returns Conflict on any revision
	// mismatch.
	UpdateWorkspaces(ctx context.Context, workspaces *model.OrgWorkspaces, revision uint64) error
}
