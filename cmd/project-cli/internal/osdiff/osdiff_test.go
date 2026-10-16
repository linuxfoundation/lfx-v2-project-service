// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package osdiff

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

const (
	uid1 = "00000000-0000-4000-8000-000000000001"
	uid2 = "00000000-0000-4000-8000-000000000002"
	uid3 = "00000000-0000-4000-8000-000000000003"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func newFakeOpenSearchClientWithBody(t *testing.T, body string) *opensearchgo.Client {
	t.Helper()

	client, err := opensearchgo.NewClient(opensearchgo.Config{
		Addresses: []string{"http://opensearch.invalid"},
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     make(http.Header),
			}, nil
		}),
		DisableRetry: true,
	})
	require.NoError(t, err)
	return client
}

func newFakeOpenSearchClient(t *testing.T, hitIDs []string) *opensearchgo.Client {
	t.Helper()

	body := `{"hits":{"hits":[`
	for i, id := range hitIDs {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"_id":%q}`, id)
	}
	body += `]}}`
	return newFakeOpenSearchClientWithBody(t, body)
}

func Test_chunkStrings(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		size  int
		want  [][]string
	}{
		{
			name:  "empty input",
			items: nil,
			size:  2,
			want:  nil,
		},
		{
			name:  "single chunk under size",
			items: []string{"a", "b"},
			size:  5,
			want:  [][]string{{"a", "b"}},
		},
		{
			name:  "exact multiple of size",
			items: []string{"a", "b", "c", "d"},
			size:  2,
			want:  [][]string{{"a", "b"}, {"c", "d"}},
		},
		{
			name:  "boundary plus remainder",
			items: []string{"a", "b", "c"},
			size:  2,
			want:  [][]string{{"a", "b"}, {"c"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkStrings(tt.items, tt.size)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_existingIDs(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantIDs map[string]struct{}
		wantErr bool
	}{
		{
			name:    "clean result",
			body:    `{"timed_out":false,"_shards":{"failed":0},"hits":{"hits":[{"_id":"project:1"}]}}`,
			wantIDs: map[string]struct{}{"project:1": {}},
		},
		{
			name:    "timed out",
			body:    `{"timed_out":true,"_shards":{"failed":0},"hits":{"hits":[]}}`,
			wantErr: true,
		},
		{
			name:    "shard failure",
			body:    `{"timed_out":false,"_shards":{"failed":1},"hits":{"hits":[]}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := existingIDs(context.Background(), newFakeOpenSearchClientWithBody(t, tt.body), []string{"project:1"})
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantIDs, got)
		})
	}
}

func TestDiff(t *testing.T) {
	tests := []struct {
		name   string
		bases  []*models.ProjectBase
		hitIDs []string
		want   map[string]Missing
	}{
		{
			name: "both docs present",
			bases: []*models.ProjectBase{
				{UID: "00000000-0000-0000-0000-000000000001"},
			},
			hitIDs: []string{
				"project:00000000-0000-0000-0000-000000000001",
				"project_settings:00000000-0000-0000-0000-000000000001",
			},
			want: map[string]Missing{
				"00000000-0000-0000-0000-000000000001": {Project: false, Settings: false},
			},
		},
		{
			name: "both docs missing",
			bases: []*models.ProjectBase{
				{UID: "00000000-0000-0000-0000-000000000002"},
			},
			hitIDs: nil,
			want: map[string]Missing{
				"00000000-0000-0000-0000-000000000002": {Project: true, Settings: true},
			},
		},
		{
			name: "only settings doc missing",
			bases: []*models.ProjectBase{
				{UID: "00000000-0000-0000-0000-000000000003"},
			},
			hitIDs: []string{
				"project:00000000-0000-0000-0000-000000000003",
			},
			want: map[string]Missing{
				"00000000-0000-0000-0000-000000000003": {Project: false, Settings: true},
			},
		},
		{
			name: "only project doc missing across multiple projects",
			bases: []*models.ProjectBase{
				{UID: "00000000-0000-0000-0000-000000000004"},
				{UID: "00000000-0000-0000-0000-000000000005"},
			},
			hitIDs: []string{
				"project_settings:00000000-0000-0000-0000-000000000004",
				"project:00000000-0000-0000-0000-000000000005",
				"project_settings:00000000-0000-0000-0000-000000000005",
			},
			want: map[string]Missing{
				"00000000-0000-0000-0000-000000000004": {Project: true, Settings: false},
				"00000000-0000-0000-0000-000000000005": {Project: false, Settings: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Diff(context.Background(), newFakeOpenSearchClient(t, tt.hitIDs), tt.bases)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMissingDocs(t *testing.T) {
	docs := []Doc{
		{TypeProject, uid1}, {TypeProjectLink, uid2}, {TypeProjectFolder, uid3}, {TypeProjectDocument, uid3},
	}

	t.Run("returns the documents the index lacks, in order", func(t *testing.T) {
		client := newFakeOpenSearchClient(t, []string{"project:" + uid1, "project_folder:" + uid3})

		missing, err := MissingDocs(context.Background(), client, docs)

		require.NoError(t, err)
		assert.Equal(t, []Doc{{TypeProjectLink, uid2}, {TypeProjectDocument, uid3}}, missing)
	})

	t.Run("a type prefix alone does not match another type's document", func(t *testing.T) {
		client := newFakeOpenSearchClient(t, []string{"project_link:" + uid1})

		missing, err := MissingDocs(context.Background(), client, []Doc{{TypeProject, uid1}})

		require.NoError(t, err)
		assert.Equal(t, []Doc{{TypeProject, uid1}}, missing)
	})

	t.Run("nothing is missing when everything is indexed", func(t *testing.T) {
		client := newFakeOpenSearchClient(t, []string{"project:" + uid1})
		missing, err := MissingDocs(context.Background(), client, docs[:1])
		require.NoError(t, err)
		assert.Empty(t, missing)
	})

	t.Run("no documents asks nothing", func(t *testing.T) {
		missing, err := MissingDocs(context.Background(), newFakeOpenSearchClient(t, nil), nil)
		require.NoError(t, err)
		assert.Empty(t, missing)
	})

	t.Run("a search failure is returned", func(t *testing.T) {
		client := newFakeOpenSearchClientWithBody(t, `{"timed_out":true,"hits":{"hits":[]}}`)
		_, err := MissingDocs(context.Background(), client, docs)
		assert.Error(t, err)
	})
}

func TestDoc_ID(t *testing.T) {
	assert.Equal(t, "project_link:"+uid1, Doc{TypeProjectLink, uid1}.ID())
}

func TestDocumentTypes(t *testing.T) {
	// The type is the indexer subject without its prefix; a drift here would make every
	// document of that type look missing.
	for subject, docType := range map[string]string{
		constants.IndexProjectSubject:         TypeProject,
		constants.IndexProjectSettingsSubject: TypeProjectSettings,
		constants.IndexProjectLinkSubject:     TypeProjectLink,
		constants.IndexProjectFolderSubject:   TypeProjectFolder,
		constants.IndexProjectDocumentSubject: TypeProjectDocument,
	} {
		assert.Equal(t, strings.TrimPrefix(subject, "lfx.index."), docType)
	}
}
