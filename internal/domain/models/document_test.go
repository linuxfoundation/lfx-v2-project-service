// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectDocument_BuildIndexKey(t *testing.T) {
	ctx := context.Background()

	t.Run("deterministic — same input produces same key", func(t *testing.T) {
		d := &ProjectDocument{ProjectUID: "proj-001", Name: "report.pdf"}
		assert.Equal(t, d.BuildIndexKey(ctx), d.BuildIndexKey(ctx))
	})

	t.Run("different project UIDs produce different keys", func(t *testing.T) {
		d1 := &ProjectDocument{ProjectUID: "proj-001", Name: "report.pdf"}
		d2 := &ProjectDocument{ProjectUID: "proj-002", Name: "report.pdf"}
		assert.NotEqual(t, d1.BuildIndexKey(ctx), d2.BuildIndexKey(ctx))
	})

	t.Run("different names produce different keys", func(t *testing.T) {
		d1 := &ProjectDocument{ProjectUID: "proj-001", Name: "report.pdf"}
		d2 := &ProjectDocument{ProjectUID: "proj-001", Name: "slides.pdf"}
		assert.NotEqual(t, d1.BuildIndexKey(ctx), d2.BuildIndexKey(ctx))
	})

	t.Run("key matches expected SHA-256 of projectUID|name", func(t *testing.T) {
		d := &ProjectDocument{ProjectUID: "proj-001", Name: "report.pdf"}
		// SHA-256("proj-001|report.pdf") pre-computed and pinned.
		const want = "022479a1edb6f4684b70355c7df592da5977fd9d3837331960978d2a5b970d86"
		assert.Equal(t, want, d.BuildIndexKey(ctx))
	})
}

func TestProjectDocument_Tags(t *testing.T) {
	tests := []struct {
		name     string
		doc      *ProjectDocument
		wantTags []string
	}{
		{
			name:     "nil document returns nil",
			doc:      nil,
			wantTags: nil,
		},
		{
			name:     "empty document returns no tags",
			doc:      &ProjectDocument{},
			wantTags: nil,
		},
		{
			name: "UID produces uid and uid-prefixed tag",
			doc:  &ProjectDocument{UID: "doc-001"},
			wantTags: []string{
				"doc-001",
				"project_document_uid:doc-001",
			},
		},
		{
			name: "projectUID produces project_uid tag",
			doc:  &ProjectDocument{ProjectUID: "proj-001"},
			wantTags: []string{
				"project_uid:proj-001",
			},
		},
		{
			name: "folderUID produces folder_uid tag",
			doc: func() *ProjectDocument {
				f := "folder-001"
				return &ProjectDocument{FolderUID: &f}
			}(),
			wantTags: []string{
				"folder_uid:folder-001",
			},
		},
		{
			name: "contentType produces content_type tag",
			doc:  &ProjectDocument{ContentType: "application/pdf"},
			wantTags: []string{
				"content_type:application/pdf",
			},
		},
		{
			name: "createdBy username produces uploaded_by tag",
			doc:  &ProjectDocument{CreatedBy: &UserInfo{Username: "alice"}},
			wantTags: []string{
				"uploaded_by:alice",
			},
		},
		{
			name: "all fields produce all expected tags",
			doc: func() *ProjectDocument {
				f := "folder-001"
				return &ProjectDocument{
					UID:         "doc-001",
					ProjectUID:  "proj-001",
					FolderUID:   &f,
					ContentType: "application/pdf",
					CreatedBy:   &UserInfo{Username: "alice"},
				}
			}(),
			wantTags: []string{
				"doc-001",
				"project_document_uid:doc-001",
				"project_uid:proj-001",
				"folder_uid:folder-001",
				"content_type:application/pdf",
				"uploaded_by:alice",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantTags, tt.doc.Tags())
		})
	}
}
