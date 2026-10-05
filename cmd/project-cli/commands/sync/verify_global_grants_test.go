// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpectedGlobalGrantTuples(t *testing.T) {
	t.Run("ordinary project expects all six tuples", func(t *testing.T) {
		tuples := expectedGlobalGrantTuples("Active")
		assert.Len(t, tuples, 6)
		assert.Contains(t, tuples, globalGrantTuple{
			User:     "team:global-project-writers#member",
			Relation: "global_writer",
		})
	})

	for _, stage := range []string{"Prospect", "Formation - Confidential"} {
		t.Run(stage+" expects owner only", func(t *testing.T) {
			tuples := expectedGlobalGrantTuples(stage)
			assert.Equal(t, []globalGrantTuple{
				{User: "team:formation#member", Relation: "global_owner"},
				{User: "team:product-support#member", Relation: "global_owner"},
			}, tuples)
		})
	}
}

func TestGlobalGrantVerificationReport(t *testing.T) {
	expected := []globalGrantTuple{
		{User: "team:formation#member", Relation: "global_owner"},
		{User: "team:product-support#member", Relation: "global_owner"},
	}
	actual := []globalGrantTuple{
		{User: "team:formation#member", Relation: "global_owner"},
		{User: "team:unexpected#member", Relation: "global_writer"},
		{User: "team:product-support#member", Relation: "global_owner", Conditioned: true},
	}

	report := newGlobalGrantVerificationReport()
	report.add(classProspect, expected, actual, nil)

	assert.Equal(t, 1, report.Projects)
	assert.Equal(t, 1, report.Missing)
	assert.Equal(t, 2, report.Unexpected)
	assert.Equal(t, 1, report.Conditioned)
	assert.Equal(t, 2, report.Classes[classProspect].Relations["global_owner"].Expected)
	assert.Equal(t, 1, report.Classes[classProspect].Relations["global_owner"].Missing)
	assert.Equal(t, 1, report.Classes[classProspect].Relations["global_owner"].Unexpected)
	assert.Equal(t, 1, report.Classes[classProspect].Relations["global_writer"].Unexpected)
}

func TestReadProjectGlobalTuplesPaginatesAndFilters(t *testing.T) {
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

	client := &globalGrantFGAClient{
		baseURL: server.URL,
		storeID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		http:    server.Client(),
	}
	tuples, err := client.readProjectGlobalTuples(context.Background(), "project-test")
	require.NoError(t, err)
	require.Len(t, tuples, 2)
	assert.False(t, tuples[0].Conditioned)
	assert.True(t, tuples[1].Conditioned)
	assert.Equal(t, 2, requests)
}

func TestReadProjectGlobalTuplesRejectsRepeatedContinuationToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tuples":[],"continuation_token":"same"}`))
	}))
	defer server.Close()
	client := &globalGrantFGAClient{
		baseURL: server.URL,
		storeID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		http:    server.Client(),
	}

	_, err := client.readProjectGlobalTuples(context.Background(), "project-test")
	assert.ErrorIs(t, err, errRepeatedContinuationToken)
}

func TestReadProjectGlobalTuplesCategorizesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not authorized", http.StatusForbidden)
	}))
	defer server.Close()
	client := &globalGrantFGAClient{
		baseURL: server.URL,
		storeID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		http:    server.Client(),
	}

	_, err := client.readProjectGlobalTuples(context.Background(), "project-test")
	require.Error(t, err)
	assert.Equal(t, "http_403", globalGrantReadErrorKind(err))
}

func TestCheckStore(t *testing.T) {
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
			client := &globalGrantFGAClient{
				baseURL: server.URL,
				storeID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
				http:    server.Client(),
			}

			err := client.checkStore(context.Background())
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
		client := &globalGrantFGAClient{
			baseURL: server.URL,
			storeID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
			http:    server.Client(),
		}

		err := client.checkStore(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OpenFGA store check failed")
		assert.NotContains(t, err.Error(), "01ARZ3NDEKTSV4RRFFQ69G5FAV")
	})
}

func TestGlobalGrantClass(t *testing.T) {
	assert.Equal(t, classSystemRoot, globalGrantClass("ROOT", ""))
	assert.Equal(t, classProspect, globalGrantClass("example", "Prospect"))
	assert.Equal(t, classConfidential, globalGrantClass("example", "Formation - Confidential"))
	assert.Equal(t, classOrdinary, globalGrantClass("example", "Active"))
}

func TestHasTupleCondition(t *testing.T) {
	assert.False(t, hasTupleCondition(nil))
	assert.False(t, hasTupleCondition(json.RawMessage(`null`)))
	assert.False(t, hasTupleCondition(json.RawMessage(`{}`)))
	assert.True(t, hasTupleCondition(json.RawMessage(`{"name":"temporary"}`)))
}
