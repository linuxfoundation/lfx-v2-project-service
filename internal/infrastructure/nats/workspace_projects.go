// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// keyPrefixWorkspaceProjects is the NATS KV key prefix for workspace project records.
// workspaceUID must be a valid UUID — callers are responsible for sanitising input.
const keyPrefixWorkspaceProjects = "org_workspace_projects."

// GetWorkspaceProjects returns the projects document for a workspace and the current
// KV revision. Returns (nil, 0, nil) when no record exists yet.
func (s *Storage) GetWorkspaceProjects(ctx context.Context, workspaceUID string) (*model.WorkspaceProjects, uint64, error) {
	if workspaceUID == "" {
		return nil, 0, errs.NewValidation("workspaceUID cannot be empty")
	}
	return getDocWithRevision[model.WorkspaceProjects](ctx, s, constants.KVBucketNameWorkspaceProjects, keyPrefixWorkspaceProjects+workspaceUID)
}

// UpdateWorkspaceProjects persists workspace project associations. The workspace UID
// is carried in projects.WorkspaceUID. When revision > 0 uses optimistic-locking
// (kv.Update); when revision == 0 uses kv.Create (exclusive create — fails on
// concurrent first-write, returns Conflict).
func (s *Storage) UpdateWorkspaceProjects(ctx context.Context, projects *model.WorkspaceProjects, revision uint64) error {
	if projects == nil {
		return errs.NewValidation("projects cannot be nil")
	}
	if projects.WorkspaceUID == "" {
		return errs.NewValidation("projects.WorkspaceUID cannot be empty")
	}
	return updateDocWithRevision(ctx, s, constants.KVBucketNameWorkspaceProjects, keyPrefixWorkspaceProjects+projects.WorkspaceUID, "workspace projects", projects, revision)
}

// DeleteWorkspaceProjects removes the projects document for a workspace.
// Safe to call when the document does not exist.
func (s *Storage) DeleteWorkspaceProjects(ctx context.Context, workspaceUID string) error {
	if workspaceUID == "" {
		return errs.NewValidation("workspaceUID cannot be empty")
	}
	return deleteDoc(ctx, s, constants.KVBucketNameWorkspaceProjects, keyPrefixWorkspaceProjects+workspaceUID)
}
