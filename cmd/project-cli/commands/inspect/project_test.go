// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/internal/fgaread"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

// fakeProjectRepo serves one project; failOn names the call that errors.
type fakeProjectRepo struct {
	base        *models.ProjectBase
	settings    *models.ProjectSettings
	slugToUID   map[string]string
	links       int
	folders     int
	documents   int
	failOn      string
	settingsErr error
}

func (f *fakeProjectRepo) fail(op string) error {
	if f.failOn == op {
		return errors.New("boom")
	}
	return nil
}

func (f *fakeProjectRepo) GetProjectBase(_ context.Context, uid string) (*models.ProjectBase, error) {
	if err := f.fail("base"); err != nil {
		return nil, err
	}
	if f.base == nil || f.base.UID != uid {
		return nil, domain.ErrProjectNotFound
	}
	return f.base, nil
}

func (f *fakeProjectRepo) GetProjectSettings(context.Context, string) (*models.ProjectSettings, error) {
	if f.settingsErr != nil {
		return nil, f.settingsErr
	}
	if f.settings == nil {
		return nil, domain.ErrProjectNotFound
	}
	return f.settings, nil
}

func (f *fakeProjectRepo) GetProjectUIDFromSlug(_ context.Context, slug string) (string, error) {
	if err := f.fail("slug"); err != nil {
		return "", err
	}
	if uid, ok := f.slugToUID[slug]; ok {
		return uid, nil
	}
	return "", domain.ErrProjectNotFound
}

func (f *fakeProjectRepo) ListAllLinks(context.Context, string) ([]*models.ProjectLink, error) {
	links := make([]*models.ProjectLink, 0, f.links+1)
	for i := 0; i < f.links; i++ {
		links = append(links, &models.ProjectLink{ProjectUID: uid1})
	}
	// A link of another project must not be counted.
	links = append(links, &models.ProjectLink{ProjectUID: uid2})
	return links, f.fail("links")
}

func (f *fakeProjectRepo) ListFolders(context.Context, string) ([]*models.ProjectFolder, error) {
	return make([]*models.ProjectFolder, f.folders), f.fail("folders")
}

func (f *fakeProjectRepo) ListAllDocuments(context.Context, string) ([]*models.ProjectDocument, error) {
	return make([]*models.ProjectDocument, f.documents), f.fail("documents")
}

type fakeGrants struct {
	tuples []fgaread.Tuple
	err    error
}

func (f fakeGrants) ReadProjectGlobalTuples(context.Context, string) ([]fgaread.Tuple, error) {
	return f.tuples, f.err
}

// healthyRepo is a project whose records and slug index agree.
func healthyRepo() *fakeProjectRepo {
	return &fakeProjectRepo{
		base:      &models.ProjectBase{UID: uid1, Slug: "alpha", Name: "Alpha", Stage: "Active"},
		settings:  &models.ProjectSettings{UID: uid1},
		slugToUID: map[string]string{"alpha": uid1},
		links:     2, folders: 1, documents: 3,
	}
}

func failingOpenSearchClient(t *testing.T) *opensearchgo.Client {
	t.Helper()
	client, err := opensearchgo.NewClient(opensearchgo.Config{
		Addresses: []string{"http://opensearch.invalid"},
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(bytes.NewBufferString("down")), Header: make(http.Header)}, nil
		}),
		DisableRetry: true,
	})
	require.NoError(t, err)
	return client
}

func TestProjectInspector_run(t *testing.T) {
	allDocs := []string{"project:" + uid1, "project_settings:" + uid1}

	tests := []struct {
		name         string
		repo         *fakeProjectRepo
		target       string
		openSearch   *opensearchgo.Client
		grants       grantReader
		wantErr      string
		wantProblems []string
		check        func(t *testing.T, r projectReport)
	}{
		{
			name:       "healthy project by slug",
			repo:       healthyRepo(),
			target:     "alpha",
			openSearch: newOpenSearchClient(t, allDocs...),
			grants:     fakeGrants{tuples: expectedFor("Active")},
			check: func(t *testing.T, r projectReport) {
				assert.Equal(t, uid1, r.UID)
				assert.Equal(t, "Alpha", r.Name)
				assert.True(t, r.SettingsPresent)
				assert.Equal(t, slugIndexCheck{Present: true, PointsTo: uid1}, r.SlugIndex)
				assert.Equal(t, &openSearchCheck{ProjectDocument: true, SettingsDocument: true}, r.OpenSearch)
				require.NotNil(t, r.Grants)
				assert.Equal(t, fgaread.ClassOrdinary, r.Grants.Class)
				assert.Equal(t, childCounts{Links: 2, Folders: 1, Documents: 3}, r.Children)
				assert.Empty(t, r.Problems)
			},
		},
		{
			name:   "healthy project by uid with checks skipped",
			repo:   healthyRepo(),
			target: uid1,
			check: func(t *testing.T, r projectReport) {
				assert.Nil(t, r.OpenSearch)
				assert.Nil(t, r.Grants)
				assert.Equal(t, []string{"openfga: skipped"}, r.Skipped)
			},
		},
		{
			name: "missing settings and slug index",
			repo: func() *fakeProjectRepo {
				r := healthyRepo()
				r.settings = nil
				r.slugToUID = map[string]string{}
				return r
			}(),
			target:       uid1,
			wantErr:      "found 2 problem(s)",
			wantProblems: []string{"project settings record is missing", `slug index entry "alpha" is missing`},
		},
		{
			name: "slug index points at another project",
			repo: func() *fakeProjectRepo {
				r := healthyRepo()
				r.slugToUID["alpha"] = uid2
				return r
			}(),
			target:       uid1,
			wantErr:      "found 1 problem(s)",
			wantProblems: []string{`slug index entry "alpha" points at "` + uid2 + `", not this project`},
		},
		{
			name:         "missing opensearch documents",
			repo:         healthyRepo(),
			target:       uid1,
			openSearch:   newOpenSearchClient(t),
			wantErr:      "found 2 problem(s)",
			wantProblems: []string{"OpenSearch project document is missing", "OpenSearch project_settings document is missing"},
			check: func(t *testing.T, r projectReport) {
				assert.Equal(t, &openSearchCheck{}, r.OpenSearch)
			},
		},
		{
			name: "ROOT is never checked against opensearch",
			repo: func() *fakeProjectRepo {
				r := healthyRepo()
				r.base.Slug = "ROOT"
				r.slugToUID = map[string]string{"ROOT": uid1}
				return r
			}(),
			target:     uid1,
			openSearch: newOpenSearchClient(t),
			check: func(t *testing.T, r projectReport) {
				assert.Nil(t, r.OpenSearch)
				assert.Contains(t, r.Skipped, "opensearch: the hidden ROOT project is never indexed")
				assert.Empty(t, r.Problems)
			},
		},
		{
			// A conditioned grant that is expected is reported once, as conditioned, not
			// also as missing: verify-global-grants fails the run on either.
			name:   "grant drift: missing, unexpected and conditioned",
			repo:   healthyRepo(),
			target: uid1,
			grants: fakeGrants{tuples: append(without(expectedFor("Active"), "global_owner"),
				fgaread.Tuple{User: "team:formation#member", Relation: "global_owner", Conditioned: true},
				fgaread.Tuple{User: "team:stray#member", Relation: "global_writer"})},
			wantErr: "found 3 problem(s)",
			wantProblems: []string{
				"1 expected global grant(s) are missing",
				"1 unexpected global grant(s) are present",
				"1 global grant(s) carry a condition",
			},
		},
		{name: "unknown slug", repo: healthyRepo(), target: "nope", wantErr: `no slug index entry for "nope"`},
		{name: "unknown uid", repo: healthyRepo(), target: uid2, wantErr: "not found"},
		{name: "slug lookup failure", repo: &fakeProjectRepo{failOn: "slug"}, target: "alpha", wantErr: "resolve slug"},
		{name: "base read failure", repo: &fakeProjectRepo{failOn: "base"}, target: uid1, wantErr: "get project base"},
		{
			name: "settings read failure",
			repo: func() *fakeProjectRepo {
				r := healthyRepo()
				r.settingsErr = errors.New("boom")
				return r
			}(),
			target:  uid1,
			wantErr: "get project settings",
		},
		{
			name: "slug index read failure",
			repo: func() *fakeProjectRepo {
				r := healthyRepo()
				r.failOn = "slug"
				return r
			}(),
			target:  uid1,
			wantErr: "read slug index",
		},
		{name: "opensearch failure", repo: healthyRepo(), target: uid1, openSearch: failingOpenSearchClient(t), wantErr: "diff opensearch"},
		{name: "grant read failure", repo: healthyRepo(), target: uid1, grants: fakeGrants{err: errors.New("boom")}, wantErr: "read OpenFGA grants"},
		{name: "link listing failure", repo: func() *fakeProjectRepo { r := healthyRepo(); r.failOn = "links"; return r }(), target: uid1, wantErr: "list links"},
		{name: "folder listing failure", repo: func() *fakeProjectRepo { r := healthyRepo(); r.failOn = "folders"; return r }(), target: uid1, wantErr: "list folders"},
		{name: "document listing failure", repo: func() *fakeProjectRepo { r := healthyRepo(); r.failOn = "documents"; return r }(), target: uid1, wantErr: "list documents"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inspector := &projectInspector{repo: tt.repo, openSearch: tt.openSearch, grants: tt.grants}
			var skipped []string
			if tt.name == "healthy project by uid with checks skipped" {
				skipped = []string{"openfga: skipped"}
			}
			var out bytes.Buffer

			err := inspector.run(context.Background(), tt.target, skipped, &out)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if out.Len() == 0 {
				return
			}
			var report projectReport
			require.NoError(t, json.Unmarshal(out.Bytes(), &report))
			if tt.wantProblems != nil {
				assert.Equal(t, tt.wantProblems, report.Problems)
			}
			if tt.check != nil {
				tt.check(t, report)
			}
		})
	}
}

func TestProjectInspector_resolveProjectUID(t *testing.T) {
	inspector := &projectInspector{repo: healthyRepo()}

	uid, err := inspector.resolveProjectUID(context.Background(), uid2)
	require.NoError(t, err)
	assert.Equal(t, uid2, uid, "a UUID is taken as a UID without a lookup")

	uid, err = inspector.resolveProjectUID(context.Background(), "alpha")
	require.NoError(t, err)
	assert.Equal(t, uid1, uid)
}

func TestDescribeTuple(t *testing.T) {
	assert.Equal(t, "global_owner team:formation#member",
		describeTuple(fgaread.Tuple{User: "team:formation#member", Relation: "global_owner"}))
}

// expectedFor is the tuples a project at the stage must carry.
func expectedFor(stage string) []fgaread.Tuple { return fgaread.ExpectedTuples(stage) }

// without drops every tuple of the given relation.
func without(tuples []fgaread.Tuple, relation string) []fgaread.Tuple {
	kept := make([]fgaread.Tuple, 0, len(tuples))
	for _, tuple := range tuples {
		if tuple.Relation != relation {
			kept = append(kept, tuple)
		}
	}
	return kept
}

func TestProjectSubcommand_Run(t *testing.T) {
	const storeID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	runSubcommand(t, &projectSubcommand{}, []runCase{
		{name: "needs one argument", wantErr: "exactly one project UID or slug"},
		{name: "rejects two arguments", args: []string{"a", "b"}, wantErr: "exactly one project UID or slug"},
		{name: "rejects a blank target", args: []string{"  "}, wantErr: "must not be empty"},
		{name: "fails at the NATS connect", args: []string{"--skip-fga", "--skip-opensearch", "example"}},
		{name: "skips OpenFGA when it is not configured", args: []string{"--skip-opensearch", "example"}, env: map[string]string{"OPENFGA_API_URL": "", "OPENFGA_STORE_ID": ""}},
		{name: "a half-configured client is a config error", args: []string{"--skip-opensearch", "example"}, env: map[string]string{"OPENFGA_API_URL": "https://openfga.example.com", "OPENFGA_STORE_ID": ""}, wantErr: "OPENFGA_API_URL and OPENFGA_STORE_ID are required"},
		{name: "a bad store ID is a config error", args: []string{"--skip-opensearch", "example"}, env: map[string]string{"OPENFGA_API_URL": "https://openfga.example.com", "OPENFGA_STORE_ID": "nope"}, wantErr: "must be a ULID"},
		{name: "an unreachable store fails the store check", args: []string{"--skip-opensearch", "example"}, env: map[string]string{"OPENFGA_API_URL": "http://127.0.0.1:1", "OPENFGA_STORE_ID": storeID}, wantErr: "OpenFGA store check failed"},
		{name: "help is not an error", args: []string{"-h"}, wantErr: "-"},
	})
}
