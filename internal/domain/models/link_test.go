// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectLink_BuildIndexKey(t *testing.T) {
	ctx := context.Background()

	t.Run("nil returns empty string", func(t *testing.T) {
		var l *ProjectLink
		assert.Empty(t, l.BuildIndexKey(ctx))
	})

	t.Run("deterministic — same input produces same key", func(t *testing.T) {
		l := &ProjectLink{ProjectUID: "proj-001", UID: "link-001"}
		assert.Equal(t, l.BuildIndexKey(ctx), l.BuildIndexKey(ctx))
	})

	t.Run("different project UIDs produce different keys", func(t *testing.T) {
		l1 := &ProjectLink{ProjectUID: "proj-001", UID: "link-001"}
		l2 := &ProjectLink{ProjectUID: "proj-002", UID: "link-001"}
		assert.NotEqual(t, l1.BuildIndexKey(ctx), l2.BuildIndexKey(ctx))
	})

	t.Run("different link UIDs produce different keys", func(t *testing.T) {
		l1 := &ProjectLink{ProjectUID: "proj-001", UID: "link-001"}
		l2 := &ProjectLink{ProjectUID: "proj-001", UID: "link-002"}
		assert.NotEqual(t, l1.BuildIndexKey(ctx), l2.BuildIndexKey(ctx))
	})

	t.Run("key matches expected SHA-256 of projectUID|uid", func(t *testing.T) {
		l := &ProjectLink{ProjectUID: "proj-001", UID: "link-001"}
		// SHA-256("proj-001|link-001") pre-computed and pinned.
		const want = "13a23c5093d49babb8402cf4c82ff8213b1922f26f6e55ba15451c95cb0ae383"
		assert.Equal(t, want, l.BuildIndexKey(ctx))
	})
}

func TestProjectLink_Tags(t *testing.T) {
	tests := []struct {
		name     string
		link     *ProjectLink
		wantTags []string
	}{
		{
			name:     "nil link returns nil",
			link:     nil,
			wantTags: nil,
		},
		{
			name:     "empty link returns no tags",
			link:     &ProjectLink{},
			wantTags: nil,
		},
		{
			name: "UID produces uid and uid-prefixed tag",
			link: &ProjectLink{UID: "link-001"},
			wantTags: []string{
				"link-001",
				"project_link_uid:link-001",
			},
		},
		{
			name: "projectUID produces project_uid tag",
			link: &ProjectLink{ProjectUID: "proj-001"},
			wantTags: []string{
				"project_uid:proj-001",
			},
		},
		{
			name: "folderUID produces folder_uid tag",
			link: func() *ProjectLink {
				f := "folder-001"
				return &ProjectLink{FolderUID: &f}
			}(),
			wantTags: []string{
				"folder_uid:folder-001",
			},
		},
		{
			name: "createdBy username produces uploaded_by tag",
			link: &ProjectLink{CreatedBy: &UserInfo{Username: "alice"}},
			wantTags: []string{
				"uploaded_by:alice",
			},
		},
		{
			name: "all fields produce all expected tags",
			link: func() *ProjectLink {
				f := "folder-001"
				return &ProjectLink{
					UID:        "link-001",
					ProjectUID: "proj-001",
					FolderUID:  &f,
					CreatedBy:  &UserInfo{Username: "alice"},
				}
			}(),
			wantTags: []string{
				"link-001",
				"project_link_uid:link-001",
				"project_uid:proj-001",
				"folder_uid:folder-001",
				"uploaded_by:alice",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantTags, tt.link.Tags())
		})
	}
}
