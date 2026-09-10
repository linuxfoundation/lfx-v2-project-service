// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectFolder_BuildIndexKey(t *testing.T) {
	// Note: BuildIndexKey dereferences the receiver without a nil check; calling it
	// on a nil *ProjectFolder panics by design (same as the standard library's
	// behaviour for value-receiver methods on zero-value structs).
	ctx := context.Background()

	tests := []struct {
		name     string
		folder   *ProjectFolder
		wantKey  string // non-empty pins the exact expected digest
		notEqual *ProjectFolder
	}{
		{
			name:   "pinned SHA-256 of projectUID|name",
			folder: &ProjectFolder{ProjectUID: "proj-001", Name: "Meeting Notes"},
			// SHA-256("proj-001|Meeting Notes") pre-computed and pinned.
			wantKey: "9ea1819bc013cd079e9f90e269a26126f39dca10f36477bddca7e728982756d2",
		},
		{
			name:     "different project UIDs produce different keys",
			folder:   &ProjectFolder{ProjectUID: "proj-001", Name: "Meeting Notes"},
			notEqual: &ProjectFolder{ProjectUID: "proj-002", Name: "Meeting Notes"},
		},
		{
			name:     "different names produce different keys",
			folder:   &ProjectFolder{ProjectUID: "proj-001", Name: "Meeting Notes"},
			notEqual: &ProjectFolder{ProjectUID: "proj-001", Name: "Design Docs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := tt.folder.BuildIndexKey(ctx)
			// Determinism — same input always produces the same key.
			assert.Equal(t, key, tt.folder.BuildIndexKey(ctx), "BuildIndexKey must be deterministic")
			if tt.wantKey != "" {
				assert.Equal(t, tt.wantKey, key)
			}
			if tt.notEqual != nil {
				assert.NotEqual(t, key, tt.notEqual.BuildIndexKey(ctx))
			}
		})
	}
}

func TestProjectFolder_Tags(t *testing.T) {
	tests := []struct {
		name     string
		folder   *ProjectFolder
		wantTags []string
	}{
		{
			name:     "nil folder returns nil",
			folder:   nil,
			wantTags: nil,
		},
		{
			name:     "empty folder returns no tags",
			folder:   &ProjectFolder{},
			wantTags: nil,
		},
		{
			name:   "UID produces uid and uid-prefixed tag",
			folder: &ProjectFolder{UID: "folder-001"},
			wantTags: []string{
				"folder-001",
				"project_folder_uid:folder-001",
			},
		},
		{
			name:   "projectUID produces project_uid tag",
			folder: &ProjectFolder{ProjectUID: "proj-001"},
			wantTags: []string{
				"project_uid:proj-001",
			},
		},
		{
			name:   "createdBy username produces uploaded_by tag",
			folder: &ProjectFolder{CreatedBy: &UserInfo{Username: "alice"}},
			wantTags: []string{
				"uploaded_by:alice",
			},
		},
		{
			name: "all fields produce all expected tags",
			folder: &ProjectFolder{
				UID:        "folder-001",
				ProjectUID: "proj-001",
				CreatedBy:  &UserInfo{Username: "alice"},
			},
			wantTags: []string{
				"folder-001",
				"project_folder_uid:folder-001",
				"project_uid:proj-001",
				"uploaded_by:alice",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantTags, tt.folder.Tags())
		})
	}
}
