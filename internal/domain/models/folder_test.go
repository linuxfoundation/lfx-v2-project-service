// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectFolder_BuildIndexKey(t *testing.T) {
	ctx := context.Background()

	t.Run("deterministic — same input produces same key", func(t *testing.T) {
		f := &ProjectFolder{ProjectUID: "proj-001", Name: "Meeting Notes"}
		assert.Equal(t, f.BuildIndexKey(ctx), f.BuildIndexKey(ctx))
	})

	t.Run("different project UIDs produce different keys", func(t *testing.T) {
		f1 := &ProjectFolder{ProjectUID: "proj-001", Name: "Meeting Notes"}
		f2 := &ProjectFolder{ProjectUID: "proj-002", Name: "Meeting Notes"}
		assert.NotEqual(t, f1.BuildIndexKey(ctx), f2.BuildIndexKey(ctx))
	})

	t.Run("different names produce different keys", func(t *testing.T) {
		f1 := &ProjectFolder{ProjectUID: "proj-001", Name: "Meeting Notes"}
		f2 := &ProjectFolder{ProjectUID: "proj-001", Name: "Design Docs"}
		assert.NotEqual(t, f1.BuildIndexKey(ctx), f2.BuildIndexKey(ctx))
	})

	t.Run("key is a 64-character hex string", func(t *testing.T) {
		f := &ProjectFolder{ProjectUID: "proj-001", Name: "Meeting Notes"}
		key := f.BuildIndexKey(ctx)
		require.NotEmpty(t, key)
		assert.Len(t, key, 64)
	})
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
