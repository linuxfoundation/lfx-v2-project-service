// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import (
	"fmt"
	"time"
)

// ProjectInfo carries the enriched project fields stored as a write-time
// snapshot on each WorkspaceProject. Populated from the ProjectResolver port
// when a project is added to a workspace.
//
// NOTE: these fields are a snapshot as of the last write that touched this
// association. Salesforce renames or re-slugs a project without re-writing
// the workspace; downstream reads may see stale name/slug/sfid until the next
// write operation on the association.
type ProjectInfo struct {
	UID  string `json:"uid"`
	SFID string `json:"sfid,omitempty"`
	Name string `json:"name,omitempty"`
	Slug string `json:"slug,omitempty"`
}

// WorkspaceProject is a project associated with a workspace. The enrichment
// fields (ProjectSFID, ProjectSlug, ProjectName) are write-time snapshots.
type WorkspaceProject struct {
	// ProjectUID is the v2 project UID (from project-service).
	ProjectUID string `json:"project_uid"`
	// ProjectSFID is the Salesforce Project__c.Id (write-time snapshot).
	ProjectSFID string `json:"project_sfid,omitempty"`
	// ProjectSlug is the project URL slug (write-time snapshot).
	ProjectSlug string `json:"project_slug,omitempty"`
	// ProjectName is the project display name (write-time snapshot).
	ProjectName string `json:"project_name,omitempty"`
	// AddedBy is the LFID username of the principal who added this project.
	AddedBy string `json:"added_by,omitempty"`
	// AddedAt is when the project was added.
	AddedAt time.Time `json:"added_at"`
}

// Workspace is a named container of project associations within a b2b_org.
type Workspace struct {
	// UID is the workspace's v2 UUID.
	UID string `json:"uid"`
	// Name is the workspace display name; unique within the org.
	Name string `json:"name"`
	// Projects holds the project associations for this workspace.
	Projects []WorkspaceProject `json:"projects,omitempty"`
	// CreatedBy is the LFID username of the principal who created the workspace.
	CreatedBy string `json:"created_by,omitempty"`
	// UpdatedBy is the LFID username of the principal who last mutated the workspace.
	UpdatedBy string    `json:"updated_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrgWorkspaces is the root document stored in the "org-workspaces" NATS KV
// bucket. One document per b2b_org; keyed "org-workspaces.{orgUID}".
type OrgWorkspaces struct {
	// OrgUID is the parent b2b_org UID this document belongs to.
	OrgUID string `json:"org_uid"`
	// Workspaces lists all workspaces owned by the org.
	Workspaces []Workspace `json:"workspaces,omitempty"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// Tag prefix consts for workspace indexer tags — mirror the TagPrefix* style
// used by b2b_org_settings but scoped to workspaces.
const (
	// TagPrefixB2BOrgWorkspaceUID is the per-workspace type-prefixed UID tag.
	// Inverse query: tags=b2b_org_workspace_uid:<uid>
	TagPrefixB2BOrgWorkspaceUID = "b2b_org_workspace_uid:"
	// TagPrefixB2BOrgUID is the parent org UID tag (shared with other member-service
	// model tags so a single b2b_org_uid: query surfaces related docs).
	// Inverse query: tags=b2b_org_uid:<orgUID>
	TagPrefixB2BOrgUID = "b2b_org_uid:"
	// TagPrefixProjectUID is the per-project UID tag.
	// Inverse query: tags=project_uid:<projectUID>
	TagPrefixProjectUID = "project_uid:"
	// TagPrefixProjectSFID is the per-project Salesforce ID tag.
	// Inverse query: tags=project_sfid:<sfid>
	TagPrefixProjectSFID = "project_sfid:"
)

// Tags returns the indexer tags for a single Workspace within its parent org.
// Mirrors the tag style of model.ProjectMembership.Tags() and
// model.KeyContact.Tags(): bare UID first, then prefixed UID, then relation refs.
// Returns nil for a nil receiver (nil-receiver-safe).
func (w *Workspace) Tags(orgUID string) []string {
	if w == nil {
		return nil
	}
	var tags []string
	if w.UID != "" {
		tags = append(tags, w.UID)
		tags = append(tags, TagPrefixB2BOrgWorkspaceUID+w.UID)
	}
	if orgUID != "" {
		tags = append(tags, TagPrefixB2BOrgUID+orgUID)
	}
	for _, p := range w.Projects {
		if p.ProjectUID != "" {
			tags = append(tags, TagPrefixProjectUID+p.ProjectUID)
		}
		if p.ProjectSFID != "" {
			tags = append(tags, TagPrefixProjectSFID+p.ProjectSFID)
		}
	}
	return tags
}

// FulltextTokens returns the free-text search tokens for a workspace:
// the workspace name and each associated project name.
// Returns nil for a nil receiver.
func (w *Workspace) FulltextTokens() []string {
	if w == nil {
		return nil
	}
	var tokens []string
	if w.Name != "" {
		tokens = append(tokens, w.Name)
	}
	for _, p := range w.Projects {
		if p.ProjectName != "" {
			tokens = append(tokens, p.ProjectName)
		}
	}
	return tokens
}

// FindWorkspace returns the workspace with the given UID, or nil if not found.
func (o *OrgWorkspaces) FindWorkspace(uid string) *Workspace {
	if o == nil {
		return nil
	}
	for i := range o.Workspaces {
		if o.Workspaces[i].UID == uid {
			return &o.Workspaces[i]
		}
	}
	return nil
}

// FindWorkspaceByName returns the first workspace whose Name exactly matches
// name (case-sensitive), or nil if not found.
func (o *OrgWorkspaces) FindWorkspaceByName(name string) *Workspace {
	if o == nil {
		return nil
	}
	for i := range o.Workspaces {
		if o.Workspaces[i].Name == name {
			return &o.Workspaces[i]
		}
	}
	return nil
}

// Clone returns a shallow copy of the OrgWorkspaces document with Workspaces
// and each workspace's Projects slice duplicated so callers can mutate the
// copy without touching the reader's cached value.
// A nil source yields a fresh document for orgUID.
func CloneOrgWorkspaces(o *OrgWorkspaces, orgUID string, now time.Time) *OrgWorkspaces {
	if o == nil {
		return &OrgWorkspaces{OrgUID: orgUID, UpdatedAt: now}
	}
	clone := &OrgWorkspaces{
		OrgUID:    orgUID,
		UpdatedAt: o.UpdatedAt,
	}
	if len(o.Workspaces) > 0 {
		clone.Workspaces = make([]Workspace, len(o.Workspaces))
		for i, w := range o.Workspaces {
			wc := Workspace{
				UID:       w.UID,
				Name:      w.Name,
				CreatedBy: w.CreatedBy,
				UpdatedBy: w.UpdatedBy,
				CreatedAt: w.CreatedAt,
				UpdatedAt: w.UpdatedAt,
			}
			if len(w.Projects) > 0 {
				wc.Projects = make([]WorkspaceProject, len(w.Projects))
				copy(wc.Projects, w.Projects)
			}
			clone.Workspaces[i] = wc
		}
	}
	return clone
}

// workspaceProjectByUID returns a pointer to the WorkspaceProject with the
// given projectUID in w, or nil if not present.
func (w *Workspace) WorkspaceProjectByUID(projectUID string) *WorkspaceProject {
	if w == nil {
		return nil
	}
	for i := range w.Projects {
		if w.Projects[i].ProjectUID == projectUID {
			return &w.Projects[i]
		}
	}
	return nil
}

// RemoveProject removes the project with the given projectUID from w.Projects.
// Returns true if a project was removed.
func (w *Workspace) RemoveProject(projectUID string) bool {
	if w == nil {
		return false
	}
	for i, p := range w.Projects {
		if p.ProjectUID == projectUID {
			w.Projects = append(w.Projects[:i], w.Projects[i+1:]...)
			return true
		}
	}
	return false
}

// WorkspaceTagsAll returns the combined tags for all workspaces in the document.
// Used for OrgWorkspaces-level indexer messages (not per-workspace).
func (o *OrgWorkspaces) WorkspaceTagsAll() []string {
	if o == nil {
		return nil
	}
	var tags []string
	for i := range o.Workspaces {
		tags = append(tags, o.Workspaces[i].Tags(o.OrgUID)...)
	}
	return tags
}

// workspaceTagKey returns a formatted tag; exposed as a package-level helper
// so tests and callers outside the model can construct expected tag values
// without importing format strings.
func WorkspaceTagKey(prefix, value string) string {
	return fmt.Sprintf("%s%s", prefix, value)
}
