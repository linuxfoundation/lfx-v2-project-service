// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// ── Input types ───────────────────────────────────────────────────────────────

// WorkspaceCreate carries the validated parameters for creating a new workspace.
type WorkspaceCreate struct {
	OrgUID    string
	Name      string
	CreatedBy string
	IfMatch   string // optional ETag pre-check on the parent OrgWorkspaces doc
}

// WorkspaceUpdate carries the validated parameters for renaming a workspace.
type WorkspaceUpdate struct {
	OrgUID       string
	WorkspaceUID string
	Name         string
	UpdatedBy    string
	IfMatch      string
}

// WorkspaceDelete carries the validated parameters for deleting a workspace.
type WorkspaceDelete struct {
	OrgUID       string
	WorkspaceUID string
	IfMatch      string
}

// WorkspaceProjectAdd carries the validated parameters for adding a single
// project to a workspace.
type WorkspaceProjectAdd struct {
	OrgUID       string
	WorkspaceUID string
	ProjectID    string // UID or slug
	AddedBy      string
	IfMatch      string
}

// WorkspaceProjectsBulkAdd carries the validated parameters for adding multiple
// projects to a workspace in one operation.
type WorkspaceProjectsBulkAdd struct {
	OrgUID       string
	WorkspaceUID string
	ProjectIDs   []string // UIDs or slugs
	AddedBy      string
	IfMatch      string
}

// WorkspaceProjectRemove carries the validated parameters for removing a project
// from a workspace.
type WorkspaceProjectRemove struct {
	OrgUID       string
	WorkspaceUID string
	ProjectUID   string
	IfMatch      string
}

// WorkspaceBulkResult is returned by AddProjectsBulk.
type WorkspaceBulkResult struct {
	// Workspace is the updated workspace after all successful additions.
	Workspace *model.Workspace
	// Succeeded holds the ProjectInfo for each successfully added project.
	Succeeded []model.ProjectInfo
	// Failed holds per-item errors aligned to the input ProjectIDs slice.
	// Indices where addition succeeded carry a nil error.
	Failed []error
}

// ── Interface ─────────────────────────────────────────────────────────────────

// WorkspaceWriter orchestrates workspace mutation use cases.
type WorkspaceWriter interface {
	CreateWorkspace(ctx context.Context, in WorkspaceCreate) (*model.Workspace, error)
	UpdateWorkspace(ctx context.Context, in WorkspaceUpdate) (*model.Workspace, error)
	DeleteWorkspace(ctx context.Context, in WorkspaceDelete) error
	AddProject(ctx context.Context, in WorkspaceProjectAdd) (*model.Workspace, error)
	AddProjectsBulk(ctx context.Context, in WorkspaceProjectsBulkAdd) (*WorkspaceBulkResult, error)
	RemoveProject(ctx context.Context, in WorkspaceProjectRemove) (*model.Workspace, error)
}

// ── Orchestrator ──────────────────────────────────────────────────────────────

type workspaceWriterOrchestrator struct {
	workspacesReader port.OrgWorkspacesReader
	workspacesWriter port.OrgWorkspacesWriter
	b2bOrgReader     port.B2BOrgReader
	projectResolver  port.ProjectResolver
	publisher        port.MemberPublisher
}

// WorkspaceWriterOption configures a workspaceWriterOrchestrator.
type WorkspaceWriterOption func(*workspaceWriterOrchestrator)

func WithWorkspacesReader(r port.OrgWorkspacesReader) WorkspaceWriterOption {
	return func(o *workspaceWriterOrchestrator) { o.workspacesReader = r }
}

func WithWorkspacesWriter(w port.OrgWorkspacesWriter) WorkspaceWriterOption {
	return func(o *workspaceWriterOrchestrator) { o.workspacesWriter = w }
}

func WithWorkspacesB2BOrgReader(r port.B2BOrgReader) WorkspaceWriterOption {
	return func(o *workspaceWriterOrchestrator) { o.b2bOrgReader = r }
}

func WithWorkspacesProjectResolver(r port.ProjectResolver) WorkspaceWriterOption {
	return func(o *workspaceWriterOrchestrator) { o.projectResolver = r }
}

func WithWorkspacesPublisher(p port.MemberPublisher) WorkspaceWriterOption {
	return func(o *workspaceWriterOrchestrator) { o.publisher = p }
}

// NewWorkspaceWriter constructs a WorkspaceWriter.
func NewWorkspaceWriter(opts ...WorkspaceWriterOption) WorkspaceWriter {
	o := &workspaceWriterOrchestrator{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// ── Use-case methods ──────────────────────────────────────────────────────────

// CreateWorkspace creates a new workspace in the org's KV document.
// Name must be unique within the org (case-sensitive). Returns Conflict on
// duplicate name and NotFound if the parent org does not exist.
func (o *workspaceWriterOrchestrator) CreateWorkspace(ctx context.Context, in WorkspaceCreate) (*model.Workspace, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, pkgerrors.NewValidation("workspace name is required")
	}

	existing, revision, err := o.workspacesReader.GetWorkspaces(ctx, in.OrgUID)
	if err != nil {
		return nil, err
	}
	if err := checkWorkspacesIfMatch(existing, in.IfMatch); err != nil {
		return nil, err
	}

	// Guard: verify parent org exists on first-write.
	if existing == nil && o.b2bOrgReader != nil {
		if _, err := o.guardOrg(ctx, in.OrgUID); err != nil {
			return nil, err
		}
	}

	updated := model.CloneOrgWorkspaces(existing, in.OrgUID, time.Now().UTC())

	if updated.FindWorkspaceByName(name) != nil {
		return nil, pkgerrors.NewConflict("a workspace with that name already exists in this organization")
	}

	now := time.Now().UTC()
	ws := model.Workspace{
		UID:       uuid.New().String(),
		Name:      name,
		CreatedBy: in.CreatedBy,
		UpdatedBy: in.CreatedBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	updated.Workspaces = append(updated.Workspaces, ws)
	updated.UpdatedAt = now

	if err := o.persistAndPublish(ctx, existing, updated, revision, &ws, indexerConstants.ActionCreated); err != nil {
		return nil, err
	}
	return &ws, nil
}

// UpdateWorkspace renames an existing workspace. Name must be unique within the org.
func (o *workspaceWriterOrchestrator) UpdateWorkspace(ctx context.Context, in WorkspaceUpdate) (*model.Workspace, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, pkgerrors.NewValidation("workspace name is required")
	}

	existing, revision, err := o.workspacesReader.GetWorkspaces(ctx, in.OrgUID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, pkgerrors.NewNotFound("workspace not found")
	}
	if err := checkWorkspacesIfMatch(existing, in.IfMatch); err != nil {
		return nil, err
	}

	updated := model.CloneOrgWorkspaces(existing, in.OrgUID, time.Now().UTC())

	ws := updated.FindWorkspace(in.WorkspaceUID)
	if ws == nil {
		return nil, pkgerrors.NewNotFound("workspace not found")
	}

	// Disallow rename to an existing name (unless it's the same workspace).
	if conflict := updated.FindWorkspaceByName(name); conflict != nil && conflict.UID != in.WorkspaceUID {
		return nil, pkgerrors.NewConflict("a workspace with that name already exists in this organization")
	}

	now := time.Now().UTC()
	ws.Name = name
	ws.UpdatedBy = in.UpdatedBy
	ws.UpdatedAt = now
	updated.UpdatedAt = now

	if err := o.persistAndPublish(ctx, existing, updated, revision, ws, indexerConstants.ActionUpdated); err != nil {
		return nil, err
	}
	// Return a copy of the workspace (the pointer into updated is valid after persist).
	wsCopy := *ws
	return &wsCopy, nil
}

// DeleteWorkspace removes a workspace and all its project associations (cascade).
// Returns 204 (nil, nil) on success.
func (o *workspaceWriterOrchestrator) DeleteWorkspace(ctx context.Context, in WorkspaceDelete) error {
	existing, revision, err := o.workspacesReader.GetWorkspaces(ctx, in.OrgUID)
	if err != nil {
		return err
	}
	if existing == nil {
		return pkgerrors.NewNotFound("workspace not found")
	}
	if err := checkWorkspacesIfMatch(existing, in.IfMatch); err != nil {
		return err
	}

	// Verify the workspace exists before publishing a delete.
	found := existing.FindWorkspace(in.WorkspaceUID)
	if found == nil {
		return pkgerrors.NewNotFound("workspace not found")
	}

	updated := model.CloneOrgWorkspaces(existing, in.OrgUID, time.Now().UTC())
	// Remove the workspace by UID.
	filtered := updated.Workspaces[:0]
	for _, w := range updated.Workspaces {
		if w.UID != in.WorkspaceUID {
			filtered = append(filtered, w)
		}
	}
	updated.Workspaces = filtered
	updated.UpdatedAt = time.Now().UTC()

	if err := o.workspacesWriter.UpdateWorkspaces(ctx, updated, revision); err != nil {
		return err
	}

	// Fire-and-forget indexer delete.
	if o.b2bOrgReader != nil && o.publisher != nil {
		org, orgErr := o.b2bOrgReader.GetB2BOrg(ctx, in.OrgUID)
		if orgErr == nil && org != nil {
			PublishWorkspaceIndexer(ctx, o.publisher, org, found, indexerConstants.ActionDeleted)
		}
	}
	return nil
}

// AddProject enriches and appends a single project to a workspace.
// Idempotent: if the project is already associated, returns the current workspace
// without error (BR-6). Returns NotFound if the workspace does not exist and
// Validation (→ HTTP 400) if the project cannot be resolved.
func (o *workspaceWriterOrchestrator) AddProject(ctx context.Context, in WorkspaceProjectAdd) (*model.Workspace, error) {
	existing, revision, err := o.workspacesReader.GetWorkspaces(ctx, in.OrgUID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, pkgerrors.NewNotFound("workspace not found")
	}
	if err := checkWorkspacesIfMatch(existing, in.IfMatch); err != nil {
		return nil, err
	}

	updated := model.CloneOrgWorkspaces(existing, in.OrgUID, time.Now().UTC())
	ws := updated.FindWorkspace(in.WorkspaceUID)
	if ws == nil {
		return nil, pkgerrors.NewNotFound("workspace not found")
	}

	// Enrich project (write-time snapshot).
	info, resolveErr := o.projectResolver.ResolveProject(ctx, in.ProjectID)
	if resolveErr != nil {
		return nil, resolveErr // already a Validation error → HTTP 400
	}

	// Idempotency: already associated → return current workspace.
	if ws.WorkspaceProjectByUID(info.UID) != nil {
		wsCopy := *ws
		return &wsCopy, nil
	}

	now := time.Now().UTC()
	ws.Projects = append(ws.Projects, model.WorkspaceProject{
		ProjectUID:  info.UID,
		ProjectSFID: info.SFID,
		ProjectSlug: info.Slug,
		ProjectName: info.Name,
		AddedBy:     in.AddedBy,
		AddedAt:     now,
	})
	ws.UpdatedAt = now
	updated.UpdatedAt = now

	if err := o.persistAndPublish(ctx, existing, updated, revision, ws, indexerConstants.ActionUpdated); err != nil {
		return nil, err
	}
	wsCopy := *ws
	return &wsCopy, nil
}

// AddProjectsBulk adds multiple projects to a workspace using a batch SOQL enrichment.
// Per-item resolution failures are collected; only successfully enriched projects are
// persisted in a single CAS write.
func (o *workspaceWriterOrchestrator) AddProjectsBulk(ctx context.Context, in WorkspaceProjectsBulkAdd) (*WorkspaceBulkResult, error) {
	existing, revision, err := o.workspacesReader.GetWorkspaces(ctx, in.OrgUID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, pkgerrors.NewNotFound("workspace not found")
	}
	if err := checkWorkspacesIfMatch(existing, in.IfMatch); err != nil {
		return nil, err
	}

	updated := model.CloneOrgWorkspaces(existing, in.OrgUID, time.Now().UTC())
	ws := updated.FindWorkspace(in.WorkspaceUID)
	if ws == nil {
		return nil, pkgerrors.NewNotFound("workspace not found")
	}

	// Batch-resolve all project identifiers in one round-trip.
	infos, resolveErrs := o.projectResolver.ResolveProjectsBatch(ctx, in.ProjectIDs)

	var succeeded []model.ProjectInfo
	failedOut := make([]error, len(in.ProjectIDs))

	now := time.Now().UTC()
	for i, info := range infos {
		if resolveErrs[i] != nil {
			failedOut[i] = resolveErrs[i]
			continue
		}
		// Idempotency: skip already-associated projects.
		if ws.WorkspaceProjectByUID(info.UID) != nil {
			succeeded = append(succeeded, info)
			continue
		}
		ws.Projects = append(ws.Projects, model.WorkspaceProject{
			ProjectUID:  info.UID,
			ProjectSFID: info.SFID,
			ProjectSlug: info.Slug,
			ProjectName: info.Name,
			AddedBy:     in.AddedBy,
			AddedAt:     now,
		})
		succeeded = append(succeeded, info)
	}

	if len(succeeded) > 0 {
		ws.UpdatedAt = now
		updated.UpdatedAt = now
		if err := o.persistAndPublish(ctx, existing, updated, revision, ws, indexerConstants.ActionUpdated); err != nil {
			return nil, err
		}
	}

	wsCopy := *ws
	return &WorkspaceBulkResult{
		Workspace: &wsCopy,
		Succeeded: succeeded,
		Failed:    failedOut,
	}, nil
}

// RemoveProject removes a project association from a workspace. Returns NotFound if
// either the workspace or the project association does not exist.
func (o *workspaceWriterOrchestrator) RemoveProject(ctx context.Context, in WorkspaceProjectRemove) (*model.Workspace, error) {
	existing, revision, err := o.workspacesReader.GetWorkspaces(ctx, in.OrgUID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, pkgerrors.NewNotFound("workspace not found")
	}
	if err := checkWorkspacesIfMatch(existing, in.IfMatch); err != nil {
		return nil, err
	}

	updated := model.CloneOrgWorkspaces(existing, in.OrgUID, time.Now().UTC())
	ws := updated.FindWorkspace(in.WorkspaceUID)
	if ws == nil {
		return nil, pkgerrors.NewNotFound("workspace not found")
	}

	if !ws.RemoveProject(in.ProjectUID) {
		return nil, pkgerrors.NewNotFound("project not found in workspace")
	}

	now := time.Now().UTC()
	ws.UpdatedAt = now
	updated.UpdatedAt = now

	if err := o.persistAndPublish(ctx, existing, updated, revision, ws, indexerConstants.ActionUpdated); err != nil {
		return nil, err
	}
	wsCopy := *ws
	return &wsCopy, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// persistAndPublish writes the updated document (CAS) and fires the indexer publish.
// The action describes the operation performed on the specific workspace (created/
// updated/deleted). The indexer always publishes the per-workspace view.
func (o *workspaceWriterOrchestrator) persistAndPublish(
	ctx context.Context,
	existing *model.OrgWorkspaces,
	updated *model.OrgWorkspaces,
	revision uint64,
	ws *model.Workspace,
	action indexerConstants.MessageAction,
) error {
	if err := o.workspacesWriter.UpdateWorkspaces(ctx, updated, revision); err != nil {
		return err
	}
	if o.b2bOrgReader == nil || o.publisher == nil {
		return nil
	}
	_ = existing // available for future create-vs-update action derivation
	org, err := o.b2bOrgReader.GetB2BOrg(ctx, updated.OrgUID)
	if err != nil || org == nil {
		return nil // fire-and-forget: swallow publish errors
	}
	PublishWorkspaceIndexer(ctx, o.publisher, org, ws, action)
	return nil
}

// guardOrg verifies the parent b2b_org exists, returning NotFound if not.
// Returns the org name so callers can use it without a second fetch.
func (o *workspaceWriterOrchestrator) guardOrg(ctx context.Context, orgUID string) (string, error) {
	if o.b2bOrgReader == nil {
		return "", nil
	}
	org, err := o.b2bOrgReader.GetB2BOrg(ctx, orgUID)
	if err != nil {
		return "", err
	}
	if org == nil {
		return "", pkgerrors.NewNotFound("organization not found")
	}
	return org.Name, nil
}

// checkWorkspacesIfMatch validates the optional If-Match precondition against
// the current workspace document ETag.
func checkWorkspacesIfMatch(existing *model.OrgWorkspaces, ifMatch string) error {
	return checkIfMatch(existing, ifMatch, "workspace record")
}
