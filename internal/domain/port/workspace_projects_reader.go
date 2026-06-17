// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// WorkspaceProjectsReader reads workspace project documents from the
// "org_workspace_projects" NATS KV bucket. Authoritative state, no MaxAge eviction.
type WorkspaceProjectsReader interface {
	// GetWorkspaceProjects returns the projects document for the given workspace UID
	// and the current KV revision (needed for optimistic-locking on UpdateWorkspaceProjects).
	// Returns (nil, 0, nil) when no record exists yet.
	GetWorkspaceProjects(ctx context.Context, workspaceUID string) (*model.WorkspaceProjects, uint64, error)
}
