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

type fakeOrphanRepo struct {
	bases     []*models.ProjectBase
	links     []*models.ProjectLink
	folders   []*models.ProjectFolder
	documents []*models.ProjectDocument
	failOn    string
}

func (f *fakeOrphanRepo) fail(op string) error {
	if f.failOn == op {
		return errors.New("boom")
	}
	return nil
}

func (f *fakeOrphanRepo) ListAllProjectsBase(context.Context) ([]*models.ProjectBase, error) {
	return f.bases, f.fail("projects")
}

func (f *fakeOrphanRepo) ListFolders(context.Context, string) ([]*models.ProjectFolder, error) {
	return f.folders, f.fail("folders")
}

func (f *fakeOrphanRepo) ListAllLinks(context.Context, string) ([]*models.ProjectLink, error) {
	return f.links, f.fail("links")
}

func (f *fakeOrphanRepo) ListAllDocuments(context.Context, string) ([]*models.ProjectDocument, error) {
	return f.documents, f.fail("documents")
}

func TestRunOrphans(t *testing.T) {
	live := []*models.ProjectBase{{UID: uid1, Slug: "alpha"}}

	tests := []struct {
		name       string
		repo       *fakeOrphanRepo
		wantErr    string
		wantCounts map[string]int
	}{
		{
			name: "children of live projects are not orphans",
			repo: &fakeOrphanRepo{
				bases:     live,
				links:     []*models.ProjectLink{{UID: "l1", ProjectUID: uid1}},
				folders:   []*models.ProjectFolder{{UID: "f1", ProjectUID: uid1}},
				documents: []*models.ProjectDocument{{UID: "d1", ProjectUID: uid1}},
			},
		},
		{
			name: "children of a deleted project are reported by kind",
			repo: &fakeOrphanRepo{
				bases:     live,
				links:     []*models.ProjectLink{{UID: "l1", ProjectUID: uid1}, {UID: "l2", ProjectUID: uid2, Name: "docs"}},
				folders:   []*models.ProjectFolder{{UID: "f1", ProjectUID: uid2}},
				documents: []*models.ProjectDocument{{UID: "d1", ProjectUID: uid2}, {UID: "d2", ProjectUID: uid3}},
			},
			wantErr:    "found 4 problem(s)",
			wantCounts: map[string]int{kindOrphanLink: 1, kindOrphanFolder: 1, kindOrphanDocument: 2},
		},
		{name: "project listing failure", repo: &fakeOrphanRepo{failOn: "projects"}, wantErr: "list project bases"},
		{name: "link listing failure", repo: &fakeOrphanRepo{failOn: "links"}, wantErr: "list links"},
		{name: "folder listing failure", repo: &fakeOrphanRepo{failOn: "folders"}, wantErr: "list folders"},
		{name: "document listing failure", repo: &fakeOrphanRepo{failOn: "documents"}, wantErr: "list documents"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runOrphans(context.Background(), tt.repo, &out)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if out.Len() == 0 {
				return
			}

			var report orphansReport
			require.NoError(t, json.Unmarshal(out.Bytes(), &report))
			if tt.wantCounts == nil {
				assert.Empty(t, report.Findings)
				return
			}
			assert.Equal(t, tt.wantCounts, report.Counts)
			assert.Len(t, report.Findings, 4)
		})
	}
}

func TestOrphansSubcommand_Run(t *testing.T) {
	runSubcommand(t, &orphansSubcommand{}, []runCase{
		{name: "rejects extra args", args: []string{"extra"}, wantErr: "unexpected argument"},
		{name: "fails at the NATS connect"},
		{name: "help is not an error", args: []string{"-h"}, wantErr: "-"},
	})
}
