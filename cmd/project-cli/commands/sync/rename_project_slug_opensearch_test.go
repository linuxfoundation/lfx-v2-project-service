// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildOSQuery_containsOldSlug(t *testing.T) {
	const slug = "old-slug"
	q := buildOSQuery(slug)
	b, ok := q["bool"].(map[string]any)
	if !ok {
		t.Fatal("expected bool key in query")
	}
	should, ok := b["should"].([]any)
	if !ok {
		t.Fatal("expected should key in bool query")
	}
	if len(should) == 0 {
		t.Fatal("expected non-empty should clauses")
	}

	fields := map[string]bool{}
	for _, clause := range should {
		termClause, ok := clause.(map[string]any)
		if !ok {
			continue
		}
		term, ok := termClause["term"].(map[string]any)
		if !ok {
			continue
		}
		for k, v := range term {
			fields[k] = true
			if str, ok := v.(string); ok {
				if str != slug && str != "project_slug:"+slug {
					t.Errorf("unexpected term value for field %q: %q", k, str)
				}
			}
		}
	}

	for _, required := range []string{"data.project_slug", "data.slug", "tags"} {
		if !fields[required] {
			t.Errorf("expected should clause for field %q, but it was missing", required)
		}
	}
}

// osCall is one request the fake OpenSearch transport received.
type osCall struct {
	path  string
	query string
	body  string
}

// newRoutedOpenSearchClient returns a client whose every response is the given
// status and body, recording each request so a test can assert what was sent.
func newRoutedOpenSearchClient(t *testing.T, status int, body string, calls *[]osCall) *opensearchgo.Client {
	t.Helper()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		if calls != nil {
			*calls = append(*calls, osCall{path: req.URL.Path, query: req.URL.RawQuery, body: string(raw)})
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	})
	client, err := opensearchgo.NewClient(opensearchgo.Config{
		Addresses:    []string{"http://opensearch.invalid"},
		DisableRetry: true,
		Transport:    transport,
	})
	require.NoError(t, err)
	return client
}

// failingOpenSearchClient returns a client whose transport always fails.
func failingOpenSearchClient(t *testing.T) *opensearchgo.Client {
	t.Helper()
	client, err := opensearchgo.NewClient(opensearchgo.Config{
		Addresses:    []string{"http://opensearch.invalid"},
		DisableRetry: true,
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		}),
	})
	require.NoError(t, err)
	return client
}

func TestRenameSlugRunner_runOpenSearch(t *testing.T) {
	tests := []struct {
		name        string
		dryRun      bool
		status      int
		body        string
		wantErr     string
		wantTotal   int
		wantUpdated int
		wantNoops   int
		wantSkipped int
		wantPath    string
	}{
		{
			name: "dry run reports the matched count", dryRun: true, status: 200,
			body:      `{"hits":{"total":{"value":5}}}`,
			wantTotal: 5, wantUpdated: 5, wantPath: "/resources/_search",
		},
		{
			name: "apply reports updated and noop counts", status: 200,
			body:      `{"total":6,"updated":4,"noops":2,"version_conflicts":0}`,
			wantTotal: 6, wantUpdated: 4, wantNoops: 2, wantSkipped: 2, wantPath: "/resources/_update_by_query",
		},
		{
			name: "version conflicts fail the run", status: 200,
			body:    `{"total":3,"updated":2,"version_conflicts":1}`,
			wantErr: "1 version conflicts",
		},
		{
			name: "document failures fail the run", status: 200,
			body:    `{"total":3,"updated":2,"failures":[{"cause":"x"}]}`,
			wantErr: "1 document failures",
		},
		{
			name: "document failures alongside conflicts report both", status: 200,
			body:    `{"total":3,"updated":1,"version_conflicts":2,"failures":[{"cause":"x"}]}`,
			wantErr: "2 version conflicts",
		},
		{
			name: "an HTTP error fails the run", status: 500, body: `{"error":"boom"}`,
			wantErr: "update_by_query error",
		},
		{
			name: "dry run HTTP error fails the run", dryRun: true, status: 503, body: `{"error":"boom"}`,
			wantErr: "search error",
		},
		{
			name: "an undecodable response fails the run", status: 200, body: `not json`,
			wantErr: "failed to decode update_by_query response",
		},
		{
			name: "dry run undecodable response fails the run", dryRun: true, status: 200, body: `not json`,
			wantErr: "failed to decode search response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []osCall
			r := &RenameSlugRunner{openSearch: newRoutedOpenSearchClient(t, tt.status, tt.body, &calls)}

			summary, err := r.runOpenSearch(context.Background(), "old-slug", "new-slug", tt.dryRun)

			require.Len(t, calls, 1)
			if tt.wantPath != "" {
				assert.Equal(t, tt.wantPath, calls[0].path)
			}
			assert.Equal(t, "opensearch", summary.Store)
			assert.Equal(t, tt.dryRun, summary.DryRun)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTotal, summary.Total)
			assert.Equal(t, tt.wantUpdated, summary.Updated)
			assert.Equal(t, tt.wantNoops, summary.Noops)
			assert.Equal(t, tt.wantSkipped, summary.Skipped)
		})
	}

	t.Run("apply sends the slugs as script params and proceeds on conflicts", func(t *testing.T) {
		var calls []osCall
		r := &RenameSlugRunner{openSearch: newRoutedOpenSearchClient(t, 200, `{"total":1,"updated":1}`, &calls)}

		_, err := r.runOpenSearch(context.Background(), "old-slug", "new-slug", false)

		require.NoError(t, err)
		require.Len(t, calls, 1)
		assert.Contains(t, calls[0].query, "conflicts=proceed")
		assert.Contains(t, calls[0].body, `"oldSlug":"old-slug"`)
		assert.Contains(t, calls[0].body, `"newSlug":"new-slug"`)
		assert.Contains(t, calls[0].body, `"lang":"painless"`)
	})

	t.Run("dry run only counts: no hits are fetched and nothing is updated", func(t *testing.T) {
		var calls []osCall
		r := &RenameSlugRunner{openSearch: newRoutedOpenSearchClient(t, 200, `{"hits":{"total":{"value":0}}}`, &calls)}

		_, err := r.runOpenSearch(context.Background(), "old-slug", "new-slug", true)

		require.NoError(t, err)
		require.Len(t, calls, 1)
		assert.Contains(t, calls[0].body, `"size":0`)
		assert.NotContains(t, calls[0].path, "_update_by_query")
	})

	t.Run("a transport failure is wrapped", func(t *testing.T) {
		r := &RenameSlugRunner{openSearch: failingOpenSearchClient(t)}

		_, err := r.runOpenSearch(context.Background(), "old-slug", "new-slug", true)
		require.ErrorContains(t, err, "search request failed")

		_, err = r.runOpenSearch(context.Background(), "old-slug", "new-slug", false)
		require.ErrorContains(t, err, "update_by_query request failed")
	})
}
