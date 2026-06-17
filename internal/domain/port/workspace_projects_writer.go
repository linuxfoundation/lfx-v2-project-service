// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// WorkspaceProjectsWriter persists workspace project documents to the
// "org_workspace_projects" NATS KV bucket. Authoritative state, no MaxAge eviction.
type WorkspaceProjectsWriter interface {
	// UpdateWorkspaceProjects persists the projects document for a workspace. The workspace UID
	// is carried in projects.WorkspaceUID. When revision > 0, uses optimistic-locking
	// (kv.Update); when revision == 0, uses an exclusive create (fails with Conflict
	// if a concurrent first-write already landed). Returns Conflict on any revision
	// mismatch.
	UpdateWorkspaceProjects(ctx context.Context, projects *model.WorkspaceProjects, revision uint64) error

	// DeleteWorkspaceProjects removes the projects document for a workspace.
	// Safe to call even if the document does not exist.
	DeleteWorkspaceProjects(ctx context.Context, workspaceUID string) error
}
