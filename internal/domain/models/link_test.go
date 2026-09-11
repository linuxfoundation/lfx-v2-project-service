// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectLink_BuildIndexKey(t *testing.T) {
	// Note: ProjectLink.BuildIndexKey nil-guards the receiver and returns "" for nil,
	// unlike ProjectDocument and ProjectFolder which dereference without a nil check.
	ctx := context.Background()

	tests := []struct {
		name       string
		link       *ProjectLink
		wantDigest string // non-empty pins the exact expected digest; "" used for nil case
		nilInput   bool   // true → expect an empty string (nil-guard branch)
		notEqual   *ProjectLink
	}{
		{
			name:     "nil returns empty string",
			link:     nil,
			nilInput: true,
		},
		{
			name: "pinned SHA-256 of projectUID|uid",
			link: &ProjectLink{ProjectUID: "proj-001", UID: "link-001"},
			// SHA-256("proj-001|link-001") pre-computed and pinned.
			wantDigest: "13a23c5093d49babb8402cf4c82ff8213b1922f26f6e55ba15451c95cb0ae383",
		},
		{
			name:     "different project UIDs produce different keys",
			link:     &ProjectLink{ProjectUID: "proj-001", UID: "link-001"},
			notEqual: &ProjectLink{ProjectUID: "proj-002", UID: "link-001"},
		},
		{
			name:     "different link UIDs produce different keys",
			link:     &ProjectLink{ProjectUID: "proj-001", UID: "link-001"},
			notEqual: &ProjectLink{ProjectUID: "proj-001", UID: "link-002"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := tt.link.BuildIndexKey(ctx)
			if tt.nilInput {
				assert.Empty(t, key)
				return
			}
			// Determinism — same input always produces the same key.
			assert.Equal(t, key, tt.link.BuildIndexKey(ctx), "BuildIndexKey must be deterministic")
			if tt.wantDigest != "" {
				assert.Equal(t, tt.wantDigest, key)
			}
			if tt.notEqual != nil {
				assert.NotEqual(t, key, tt.notEqual.BuildIndexKey(ctx))
			}
		})
	}
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
			name: "empty folderUID pointer produces no folder_uid tag",
			link: func() *ProjectLink {
				f := ""
				return &ProjectLink{FolderUID: &f}
			}(),
			wantTags: nil,
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
