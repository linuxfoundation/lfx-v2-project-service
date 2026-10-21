// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectDocument_BuildIndexKey(t *testing.T) {
	// Note: BuildIndexKey dereferences the receiver without a nil check; calling it
	// on a nil *ProjectDocument panics by design (same as the standard library's
	// behaviour for value-receiver methods on zero-value structs).
	ctx := context.Background()

	tests := []struct {
		name       string
		doc        *ProjectDocument
		wantDigest string // non-empty pins the exact expected digest
		notEqual   *ProjectDocument
	}{
		{
			name: "pinned SHA-256 of projectUID|name",
			doc:  &ProjectDocument{ProjectUID: "proj-001", Name: "report.pdf"},
			// SHA-256("proj-001|report.pdf") pre-computed and pinned.
			wantDigest: "022479a1edb6f4684b70355c7df592da5977fd9d3837331960978d2a5b970d86",
		},
		{
			name:     "different project UIDs produce different keys",
			doc:      &ProjectDocument{ProjectUID: "proj-001", Name: "report.pdf"},
			notEqual: &ProjectDocument{ProjectUID: "proj-002", Name: "report.pdf"},
		},
		{
			name:     "different names produce different keys",
			doc:      &ProjectDocument{ProjectUID: "proj-001", Name: "report.pdf"},
			notEqual: &ProjectDocument{ProjectUID: "proj-001", Name: "slides.pdf"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := tt.doc.BuildIndexKey(ctx)
			// Determinism — same input always produces the same key.
			assert.Equal(t, key, tt.doc.BuildIndexKey(ctx), "BuildIndexKey must be deterministic")
			if tt.wantDigest != "" {
				assert.Equal(t, tt.wantDigest, key)
			}
			if tt.notEqual != nil {
				assert.NotEqual(t, key, tt.notEqual.BuildIndexKey(ctx))
			}
		})
	}
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
			name: "empty folderUID pointer produces no folder_uid tag",
			doc: func() *ProjectDocument {
				f := ""
				return &ProjectDocument{FolderUID: &f}
			}(),
			wantTags: nil,
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

func TestProjectDocument_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ProjectDocument
		wantErr bool
	}{
		{
			name:  "current format keeps audit users",
			input: `{"uid":"d1","project_uid":"p1","name":"Charter","file_name":"charter.pdf","file_size":10,"content_type":"application/pdf","created_by":{"username":"alice"},"updated_by":{"username":"bob"}}`,
			want: ProjectDocument{
				UID: "d1", ProjectUID: "p1", Name: "Charter", FileName: "charter.pdf", FileSize: 10, ContentType: "application/pdf",
				CreatedBy: &UserInfo{Username: "alice"}, UpdatedBy: &UserInfo{Username: "bob"},
			},
		},
		{
			name:  "legacy uploaded_by_username migrates to created_by and updated_by",
			input: `{"uid":"d1","project_uid":"p1","name":"Charter","uploaded_by_username":"alice"}`,
			want: ProjectDocument{
				UID: "d1", ProjectUID: "p1", Name: "Charter",
				CreatedBy: &UserInfo{Username: "alice"}, UpdatedBy: &UserInfo{Username: "alice"},
			},
		},
		{
			name:  "folder uid and description are preserved",
			input: `{"uid":"d1","project_uid":"p1","folder_uid":"f1","name":"Charter","description":"d"}`,
			want: ProjectDocument{
				UID: "d1", ProjectUID: "p1", FolderUID: func() *string { s := "f1"; return &s }(),
				Name: "Charter", Description: "d",
			},
		},
		{
			name:    "invalid json returns error",
			input:   `{"uid":`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ProjectDocument
			err := json.Unmarshal([]byte(tt.input), &got)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProjectDocument_MarshalJSON(t *testing.T) {
	t.Run("nil document marshals to null", func(t *testing.T) {
		var d *ProjectDocument
		out, err := json.Marshal(d)
		require.NoError(t, err)
		assert.Equal(t, "null", string(out))
	})

	t.Run("round trip preserves fields and drops legacy key", func(t *testing.T) {
		folder := "f1"
		in := &ProjectDocument{
			UID: "d1", ProjectUID: "p1", FolderUID: &folder, Name: "Charter", Description: "d",
			FileName: "charter.pdf", FileSize: 10, ContentType: "application/pdf",
			CreatedBy: &UserInfo{Username: "alice"}, UpdatedBy: &UserInfo{Username: "bob"},
			CreatedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		}
		out, err := json.Marshal(in)
		require.NoError(t, err)
		assert.NotContains(t, string(out), "uploaded_by_username")

		var got ProjectDocument
		require.NoError(t, json.Unmarshal(out, &got))
		assert.Equal(t, *in, got)
	})
}

func TestProjectDocument_IndexingConfig(t *testing.T) {
	t.Run("nil document returns nil", func(t *testing.T) {
		var d *ProjectDocument
		assert.Nil(t, d.IndexingConfig())
	})

	t.Run("populates access, parent and tags from the document", func(t *testing.T) {
		d := &ProjectDocument{UID: "d1", ProjectUID: "p1", Name: "Charter"}
		cfg := d.IndexingConfig()
		require.NotNil(t, cfg)
		assert.Equal(t, "d1", cfg.ObjectID)
		assert.Equal(t, "project:p1", cfg.AccessCheckObject)
		assert.Equal(t, "viewer", cfg.AccessCheckRelation)
		assert.Equal(t, "project:p1", cfg.HistoryCheckObject)
		assert.Equal(t, "auditor_guard", cfg.HistoryCheckRelation)
		assert.Equal(t, "Charter", cfg.SortName)
		assert.Equal(t, []string{"project:p1"}, cfg.ParentRefs)
		assert.Equal(t, d.Tags(), cfg.Tags)
	})
}
