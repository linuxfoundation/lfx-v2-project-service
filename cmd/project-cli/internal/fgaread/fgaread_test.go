// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fgaread

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpectedTuples(t *testing.T) {
	t.Run("ordinary project expects all six tuples", func(t *testing.T) {
		assert.ElementsMatch(t, []Tuple{
			{User: "team:formation#member", Relation: "global_owner"},
			{User: "team:product-support#member", Relation: "global_owner"},
			{User: "team:global-project-writers#member", Relation: "global_writer"},
			{User: "team:lf-staff#member", Relation: "global_auditor"},
			{User: "team:global-project-auditors#member", Relation: "global_auditor"},
			{User: "team:marketing-ops#member", Relation: "global_marketing_ops"},
		}, ExpectedTuples("Active"))
	})

	for _, stage := range []string{"Prospect", "Formation - Confidential"} {
		t.Run(stage+" expects owner only", func(t *testing.T) {
			tuples := ExpectedTuples(stage)
			assert.Equal(t, []Tuple{
				{User: "team:formation#member", Relation: "global_owner"},
				{User: "team:product-support#member", Relation: "global_owner"},
			}, tuples)
		})
	}
}

func TestClient_ReadProjectGlobalTuples(t *testing.T) {
	const storeID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	newClient := func(server *httptest.Server) *Client {
		return &Client{baseURL: server.URL, storeID: storeID, http: server.Client()}
	}

	t.Run("paginates and keeps only global relations", func(t *testing.T) {
		var requests int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			require.Equal(t, http.MethodPost, r.Method)
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "project:project-test", body["tuple_key"].(map[string]any)["object"])
			assert.Equal(t, "HIGHER_CONSISTENCY", body["consistency"])

			w.Header().Set("Content-Type", "application/json")
			if requests == 1 {
				_, _ = w.Write([]byte(`{
					"tuples": [
						{"key": {"user": "team:formation#member", "relation": "global_owner", "object": "project:project-test"}},
						{"key": {"user": "user:test", "relation": "writer", "object": "project:project-test"}}
					],
					"continuation_token": "next"
				}`))
				return
			}
			assert.Equal(t, "next", body["continuation_token"])
			_, _ = w.Write([]byte(`{
				"tuples": [{
					"key": {
						"user": "team:product-support#member",
						"relation": "global_owner",
						"object": "project:project-test",
						"condition": {"name": "temporary"}
					}
				}]
			}`))
		}))
		defer server.Close()

		tuples, err := newClient(server).ReadProjectGlobalTuples(context.Background(), "project-test")
		require.NoError(t, err)
		require.Len(t, tuples, 2)
		assert.False(t, tuples[0].Conditioned)
		assert.True(t, tuples[1].Conditioned)
		assert.Equal(t, 2, requests)
	})

	t.Run("rejects a repeated continuation token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tuples":[],"continuation_token":"same"}`))
		}))
		defer server.Close()

		_, err := newClient(server).ReadProjectGlobalTuples(context.Background(), "project-test")
		assert.ErrorIs(t, err, ErrRepeatedContinuationToken)
	})

	t.Run("categorizes an HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "not authorized", http.StatusForbidden)
		}))
		defer server.Close()

		_, err := newClient(server).ReadProjectGlobalTuples(context.Background(), "project-test")
		require.Error(t, err)
		var httpErr HTTPError
		require.ErrorAs(t, err, &httpErr)
		assert.Equal(t, http.StatusForbidden, httpErr.Status)
	})

	t.Run("transport error omits store ID", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		server.Close()

		_, err := newClient(server).ReadProjectGlobalTuples(context.Background(), "project-test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OpenFGA read failed")
		assert.NotContains(t, err.Error(), storeID)
		var httpErr HTTPError
		assert.False(t, errors.As(err, &httpErr), "a transport failure is not an HTTP status error")
	})
}

func TestClient_CheckStore(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		wantErr string
	}{
		{name: "existing store", status: http.StatusOK},
		{name: "absent store", status: http.StatusNotFound, wantErr: "does not name an existing OpenFGA store"},
		{name: "server error", status: http.StatusInternalServerError, wantErr: "HTTP 500"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/stores/01ARZ3NDEKTSV4RRFFQ69G5FAV", r.URL.Path)
				w.WriteHeader(tc.status)
			}))
			defer server.Close()
			client := &Client{
				baseURL: server.URL,
				storeID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
				http:    server.Client(),
			}

			err := client.CheckStore(context.Background())
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.NotContains(t, err.Error(), "01ARZ3NDEKTSV4RRFFQ69G5FAV")
		})
	}

	t.Run("transport error omits store ID", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		server.Close()
		client := &Client{
			baseURL: server.URL,
			storeID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
			http:    server.Client(),
		}

		err := client.CheckStore(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OpenFGA store check failed")
		assert.NotContains(t, err.Error(), "01ARZ3NDEKTSV4RRFFQ69G5FAV")
	})
}

func TestClass(t *testing.T) {
	assert.Equal(t, ClassSystemRoot, Class("ROOT", ""))
	assert.Equal(t, ClassProspect, Class("example", "Prospect"))
	assert.Equal(t, ClassConfidential, Class("example", "Formation - Confidential"))
	assert.Equal(t, ClassOrdinary, Class("example", "Active"))
}

func TestHasTupleCondition(t *testing.T) {
	assert.False(t, hasTupleCondition(nil))
	assert.False(t, hasTupleCondition(json.RawMessage(`null`)))
	assert.False(t, hasTupleCondition(json.RawMessage(`{}`)))
	assert.True(t, hasTupleCondition(json.RawMessage(`{"name":"temporary"}`)))
}

func TestNewClientFromEnv(t *testing.T) {
	const storeID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

	tests := []struct {
		name    string
		url     string
		store   string
		wantErr string
		wantURL string
	}{
		{name: "valid config trims the trailing slash", url: " https://openfga.example.com/ ", store: storeID, wantURL: "https://openfga.example.com"},
		{name: "missing URL", store: storeID, wantErr: "OPENFGA_API_URL and OPENFGA_STORE_ID are required"},
		{name: "missing store", url: "https://openfga.example.com", wantErr: "OPENFGA_API_URL and OPENFGA_STORE_ID are required"},
		{name: "non-HTTP scheme", url: "ftp://openfga.example.com", store: storeID, wantErr: "absolute HTTP(S) URL"},
		{name: "relative URL", url: "openfga.example.com", store: storeID, wantErr: "absolute HTTP(S) URL"},
		{name: "store ID that is not a ULID", url: "https://openfga.example.com", store: "not-a-ulid", wantErr: "must be a ULID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OPENFGA_API_URL", tt.url)
			t.Setenv("OPENFGA_STORE_ID", tt.store)

			client, err := NewClientFromEnv()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				if tt.store != "" {
					assert.NotContains(t, err.Error(), tt.store, "store IDs are not echoed")
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantURL, client.baseURL)
			assert.Equal(t, tt.store, client.storeID)
			assert.NotZero(t, client.http.Timeout)
		})
	}
}

func TestHTTPError_Error(t *testing.T) {
	assert.Equal(t, "OpenFGA read returned HTTP 503", HTTPError{Status: 503}.Error())
}

func TestClient_ReadProjectTuples(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tuples": [
			{"key": {"user": "user:test", "relation": "writer"}},
			{"key": {"user": "user:other", "relation": "auditor"}},
			{"key": {"user": "team:formation#member", "relation": "global_owner"}}
		]}`))
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL, storeID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", http: server.Client()}

	tuples, err := client.ReadProjectTuples(context.Background(), "project-test", func(relation string) bool {
		return relation == "writer" || relation == "auditor"
	})

	require.NoError(t, err)
	assert.Equal(t, []Tuple{
		{User: "user:test", Relation: "writer"},
		{User: "user:other", Relation: "auditor"},
	}, tuples)
}
