// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

func TestRunParents(t *testing.T) {
	const uid4 = "00000000-0000-4000-8000-000000000004"
	project := func(uid, slug, parent string) *models.ProjectBase {
		return &models.ProjectBase{UID: uid, Slug: slug, ParentUID: parent}
	}

	tests := []struct {
		name         string
		bases        []*models.ProjectBase
		err          error
		wantErr      string
		wantFindings []finding
	}{
		{
			name:  "a clean hierarchy",
			bases: []*models.ProjectBase{project(uid1, "a", ""), project(uid2, "b", uid1), project(uid3, "c", uid2)},
		},
		{
			name:         "a parent that does not exist",
			bases:        []*models.ProjectBase{project(uid1, "a", uid2)},
			wantErr:      "found 1 problem(s)",
			wantFindings: []finding{{Kind: kindMissingParent, Slug: "a", UID: uid1, Detail: `parent "` + uid2 + `" does not exist`}},
		},
		{
			name:         "a project that is its own parent",
			bases:        []*models.ProjectBase{project(uid1, "a", uid1)},
			wantErr:      "found 1 problem(s)",
			wantFindings: []finding{{Kind: kindSelfParent, Slug: "a", UID: uid1, Detail: "parent_uid is the project itself"}},
		},
		{
			name:         "a two-project cycle is reported once at its lowest UID",
			bases:        []*models.ProjectBase{project(uid2, "b", uid1), project(uid1, "a", uid2)},
			wantErr:      "found 1 problem(s)",
			wantFindings: []finding{{Kind: kindParentCycle, Slug: "a", UID: uid1, Detail: "parent chain loops through 2 projects"}},
		},
		{
			name: "a descendant of a cycle does not add a second finding",
			bases: []*models.ProjectBase{
				project(uid1, "a", uid2), project(uid2, "b", uid3), project(uid3, "c", uid1), project(uid4, "d", uid3),
			},
			wantErr:      "found 1 problem(s)",
			wantFindings: []finding{{Kind: kindParentCycle, Slug: "a", UID: uid1, Detail: "parent chain loops through 3 projects"}},
		},
		{name: "listing failure", err: errors.New("boom"), wantErr: "list project bases"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runParents(context.Background(), fakeProjectLister{bases: tt.bases, err: tt.err}, &out)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if out.Len() == 0 {
				return
			}
			var report parentsReport
			require.NoError(t, json.Unmarshal(out.Bytes(), &report))
			assert.Equal(t, len(tt.bases), report.Projects)
			if tt.wantFindings == nil {
				assert.Empty(t, report.Findings)
			} else {
				assert.Equal(t, tt.wantFindings, report.Findings)
			}
		})
	}
}

func TestIndexOf(t *testing.T) {
	assert.Equal(t, 1, indexOf([]string{"a", "b"}, "b"))
	assert.Equal(t, -1, indexOf([]string{"a"}, "z"))
}
