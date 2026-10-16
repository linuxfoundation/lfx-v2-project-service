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

type fakeDriftRepo struct {
	bases     []*models.ProjectBase
	links     []*models.ProjectLink
	folders   []*models.ProjectFolder
	documents []*models.ProjectDocument
	failOn    string
}

func (f fakeDriftRepo) fail(op string) error {
	if f.failOn == op {
		return errors.New("boom")
	}
	return nil
}

func (f fakeDriftRepo) ListAllProjectsBase(context.Context) ([]*models.ProjectBase, error) {
	return f.bases, f.fail("projects")
}

func (f fakeDriftRepo) ListAllLinks(context.Context, string) ([]*models.ProjectLink, error) {
	return f.links, f.fail("links")
}

func (f fakeDriftRepo) ListFolders(context.Context, string) ([]*models.ProjectFolder, error) {
	return f.folders, f.fail("folders")
}

func (f fakeDriftRepo) ListAllDocuments(context.Context, string) ([]*models.ProjectDocument, error) {
	return f.documents, f.fail("documents")
}

func allResources() map[string]bool {
	selected, _ := parseDriftResources("all")
	return selected
}

func TestRunIndexDrift(t *testing.T) {
	const (
		linkUID   = "00000000-0000-4000-8000-0000000000a1"
		folderUID = "00000000-0000-4000-8000-0000000000b1"
		docUID    = "00000000-0000-4000-8000-0000000000c1"
	)
	repo := fakeDriftRepo{
		bases: []*models.ProjectBase{
			{UID: uid1, Slug: "alpha"},
			{UID: uid2, Slug: "beta"},
			{UID: uid3, Slug: "ROOT"},
		},
		links:     []*models.ProjectLink{{UID: linkUID, ProjectUID: uid1}},
		folders:   []*models.ProjectFolder{{UID: folderUID, ProjectUID: uid1}},
		documents: []*models.ProjectDocument{{UID: docUID, ProjectUID: uid1}},
	}
	everything := []string{
		"project:" + uid1, "project_settings:" + uid1, "project:" + uid2, "project_settings:" + uid2,
		"project_link:" + linkUID, "project_folder:" + folderUID, "project_document:" + docUID,
	}

	tests := []struct {
		name         string
		repo         fakeDriftRepo
		hits         []string
		selected     map[string]bool
		wantErr      string
		wantChecked  map[string]int
		wantFindings []string // kind:uid
	}{
		{
			name:        "fully indexed is clean and ROOT is not looked up",
			repo:        repo,
			hits:        everything,
			selected:    allResources(),
			wantChecked: map[string]int{"project": 2, "project_settings": 2, "project_link": 1, "project_folder": 1, "project_document": 1},
		},
		{
			name:         "a missing settings document",
			repo:         repo,
			hits:         withoutString(everything, "project_settings:"+uid2),
			selected:     allResources(),
			wantErr:      "found 1 problem(s)",
			wantFindings: []string{"missing_project_settings:" + uid2},
		},
		{
			name:         "child documents that were never indexed",
			repo:         repo,
			hits:         everything[:4],
			selected:     allResources(),
			wantErr:      "found 3 problem(s)",
			wantFindings: []string{"missing_project_document:" + docUID, "missing_project_folder:" + folderUID, "missing_project_link:" + linkUID},
		},
		{
			name:        "a project with no documents lacks both",
			repo:        fakeDriftRepo{bases: repo.bases[:1]},
			selected:    allResources(),
			wantErr:     "found 2 problem(s)",
			wantChecked: map[string]int{"project": 1, "project_settings": 1},
			wantFindings: []string{
				"missing_project:" + uid1, "missing_project_settings:" + uid1,
			},
		},
		{
			name:        "only the selected resources are looked up",
			repo:        repo,
			selected:    map[string]bool{driftLink: true},
			wantErr:     "found 1 problem(s)",
			wantChecked: map[string]int{"project_link": 1},
			wantFindings: []string{
				"missing_project_link:" + linkUID,
			},
		},
		{
			name:        "resources that are not selected are not looked up",
			repo:        repo,
			hits:        everything[:4],
			selected:    map[string]bool{driftProject: true},
			wantChecked: map[string]int{"project": 2, "project_settings": 2},
		},
		{name: "project listing failure", repo: fakeDriftRepo{failOn: "projects"}, selected: allResources(), wantErr: "list project bases"},
		{name: "link listing failure", repo: fakeDriftRepo{failOn: "links"}, selected: allResources(), wantErr: "list links"},
		{name: "folder listing failure", repo: fakeDriftRepo{failOn: "folders"}, selected: allResources(), wantErr: "list folders"},
		{name: "document listing failure", repo: fakeDriftRepo{failOn: "documents"}, selected: allResources(), wantErr: "list documents"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			err := runIndexDrift(context.Background(), tt.repo, newOpenSearchClient(t, tt.hits...), tt.selected, &out)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if out.Len() == 0 {
				return
			}
			var report indexDriftReport
			require.NoError(t, json.Unmarshal(out.Bytes(), &report))
			assert.Equal(t, wantStatus(err), report.Status)
			if tt.wantChecked != nil {
				assert.Equal(t, tt.wantChecked, report.Checked)
			}
			var got []string
			for _, f := range report.Findings {
				got = append(got, f.Kind+":"+f.UID)
			}
			assert.ElementsMatch(t, tt.wantFindings, got)
		})
	}
}

func Test_parseDriftResources(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    []string
		wantErr string
	}{
		{name: "all", value: "all", want: driftResources},
		{name: "one", value: "link", want: []string{driftLink}},
		{name: "several with spaces", value: "project, folder", want: []string{driftProject, driftFolder}},
		{name: "unknown", value: "project,widget", wantErr: `unknown resource "widget"`},
		{name: "empty", value: "", wantErr: "unknown resource"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDriftResources(tt.value)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			names := make([]string, 0, len(got))
			for name := range got {
				names = append(names, name)
			}
			assert.ElementsMatch(t, tt.want, names)
		})
	}
}

func TestIndexDriftSubcommand_Run(t *testing.T) {
	runSubcommand(t, &indexDriftSubcommand{}, []runCase{
		{name: "rejects extra args", args: []string{"extra"}, wantErr: "unexpected argument"},
		{name: "rejects an unknown resource", args: []string{"--resource=widget"}, wantErr: "unknown resource"},
		{name: "fails at the NATS connect"},
		{name: "fails at the NATS connect for one resource", args: []string{"--resource=link"}},
		{name: "help is not an error", args: []string{"-h"}, wantErr: "-"},
	})
}

// withoutString drops one string.
func withoutString(items []string, drop string) []string {
	kept := make([]string, 0, len(items))
	for _, item := range items {
		if item != drop {
			kept = append(kept, item)
		}
	}
	return kept
}
