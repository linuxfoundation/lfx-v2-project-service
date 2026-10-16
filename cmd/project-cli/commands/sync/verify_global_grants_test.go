// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	natsinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
)

func TestExpectedGlobalGrantTuples(t *testing.T) {
	t.Run("ordinary project expects all six tuples", func(t *testing.T) {
		assert.ElementsMatch(t, []globalGrantTuple{
			{User: "team:formation#member", Relation: "global_owner"},
			{User: "team:product-support#member", Relation: "global_owner"},
			{User: "team:global-project-writers#member", Relation: "global_writer"},
			{User: "team:lf-staff#member", Relation: "global_auditor"},
			{User: "team:global-project-auditors#member", Relation: "global_auditor"},
			{User: "team:marketing-ops#member", Relation: "global_marketing_ops"},
		}, expectedGlobalGrantTuples("Active"))
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

func TestGlobalGrantVerificationReport_add(t *testing.T) {
	t.Run("counts missing, unexpected and conditioned tuples", func(t *testing.T) {
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
	})

	t.Run("counts expected tuples when the read fails", func(t *testing.T) {
		report := newGlobalGrantVerificationReport()
		report.add(classOrdinary, expectedGlobalGrantTuples("Active"), nil, errors.New("read failed"))

		assert.Equal(t, 1, report.ReadErrors)
		assert.Equal(t, 1, report.Classes[classOrdinary].ReadErrors)
		assert.Equal(t, 2, report.Classes[classOrdinary].Relations["global_owner"].Expected)
		assert.Equal(t, 1, report.Classes[classOrdinary].Relations["global_writer"].Expected)
		assert.Equal(t, 2, report.Classes[classOrdinary].Relations["global_auditor"].Expected)
		assert.Equal(t, 0, report.Missing)
		assert.Equal(t, 0, report.Unexpected)
	})
}

func TestGlobalGrantVerificationReport_failed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		report globalGrantVerificationReport
		want   bool
	}{
		{name: "clean", report: globalGrantVerificationReport{Projects: 3}, want: false},
		{name: "read error", report: globalGrantVerificationReport{ReadErrors: 1}, want: true},
		{name: "missing tuple", report: globalGrantVerificationReport{Missing: 1}, want: true},
		{name: "unexpected tuple", report: globalGrantVerificationReport{Unexpected: 1}, want: true},
		{name: "conditioned tuple", report: globalGrantVerificationReport{Conditioned: 1}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.report.failed())
		})
	}
}

func TestGlobalGrantFGAClient_readProjectGlobalTuples(t *testing.T) {
	const storeID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	newClient := func(server *httptest.Server) *globalGrantFGAClient {
		return &globalGrantFGAClient{baseURL: server.URL, storeID: storeID, http: server.Client()}
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

		tuples, err := newClient(server).readProjectGlobalTuples(context.Background(), "project-test")
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

		_, err := newClient(server).readProjectGlobalTuples(context.Background(), "project-test")
		assert.ErrorIs(t, err, errRepeatedContinuationToken)
	})

	t.Run("categorizes an HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "not authorized", http.StatusForbidden)
		}))
		defer server.Close()

		_, err := newClient(server).readProjectGlobalTuples(context.Background(), "project-test")
		require.Error(t, err)
		assert.Equal(t, "http_403", globalGrantReadErrorKind(err))
	})

	t.Run("transport error omits store ID", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		server.Close()

		_, err := newClient(server).readProjectGlobalTuples(context.Background(), "project-test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OpenFGA read failed")
		assert.NotContains(t, err.Error(), storeID)
		assert.Equal(t, "transport_or_decode", globalGrantReadErrorKind(err))
	})
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

func TestNewGlobalGrantFGAClientFromEnv(t *testing.T) {
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

			client, err := newGlobalGrantFGAClientFromEnv()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.NotContains(t, err.Error(), tt.store+"x", "store IDs are not echoed")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantURL, client.baseURL)
			assert.Equal(t, tt.store, client.storeID)
			assert.NotZero(t, client.http.Timeout)
		})
	}
}

func TestGlobalGrantHTTPError_Error(t *testing.T) {
	assert.Equal(t, "OpenFGA read returned HTTP 503", globalGrantHTTPError{status: 503}.Error())
}

func TestGlobalGrantReadErrorKind(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "http status", err: globalGrantHTTPError{status: 500}, want: "http_500"},
		{name: "wrapped http status", err: fmt.Errorf("read: %w", globalGrantHTTPError{status: 429}), want: "http_429"},
		{name: "timeout", err: context.DeadlineExceeded, want: "timeout"},
		{name: "canceled", err: context.Canceled, want: "canceled"},
		{name: "repeated token", err: errRepeatedContinuationToken, want: "repeated_continuation_token"},
		{name: "page limit", err: errTooManyReadPages, want: "page_limit"},
		{name: "anything else", err: errors.New("connection reset"), want: "transport_or_decode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, globalGrantReadErrorKind(tt.err))
		})
	}
}

func TestRelationCounts(t *testing.T) {
	report := newGlobalGrantClassReport()

	existing := relationCounts(report, "global_owner")
	existing.Missing = 2
	assert.Same(t, existing, relationCounts(report, "global_owner"), "an existing relation is returned, not replaced")

	created := relationCounts(report, "legacy_relation")
	require.NotNil(t, created)
	assert.Same(t, created, report.Relations["legacy_relation"], "an unknown relation is registered on first use")
}

func TestVerifyGlobalGrantsSubcommand_Run(t *testing.T) {
	const storeID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	s := &verifyGlobalGrantsSubcommand{}
	assert.Equal(t, "verify-global-grants", s.Name())
	assert.NotEmpty(t, s.Help())

	t.Run("flag and argument validation", func(t *testing.T) {
		tests := []struct {
			name    string
			args    []string
			wantErr string
		}{
			{name: "help is not an error", args: []string{"-h"}},
			{name: "unexpected positional argument", args: []string{"extra"}, wantErr: "unexpected arguments: extra"},
			{name: "concurrency below range", args: []string{"--concurrency=0"}, wantErr: "concurrency must be between 1 and 100"},
			{name: "concurrency above range", args: []string{"--concurrency=101"}, wantErr: "concurrency must be between 1 and 100"},
			{name: "unknown flag", args: []string{"--nope"}, wantErr: "flag provided but not defined"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := s.Run(context.Background(), commands.RunContext{Args: tt.args})
				if tt.wantErr == "" {
					require.NoError(t, err)
					return
				}
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			})
		}
	})

	t.Run("missing OpenFGA configuration fails before any connection", func(t *testing.T) {
		t.Setenv("OPENFGA_API_URL", "")
		t.Setenv("OPENFGA_STORE_ID", "")

		err := s.Run(context.Background(), commands.RunContext{})

		require.ErrorContains(t, err, "OPENFGA_API_URL and OPENFGA_STORE_ID are required")
	})

	t.Run("an unknown store fails before NATS is contacted", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()
		t.Setenv("OPENFGA_API_URL", server.URL)
		t.Setenv("OPENFGA_STORE_ID", storeID)

		err := s.Run(context.Background(), commands.RunContext{NATSConfig: unreachableNATS()})

		require.ErrorContains(t, err, "does not name an existing OpenFGA store")
	})

	t.Run("a NATS connection failure is reported once the store exists", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		t.Setenv("OPENFGA_API_URL", server.URL)
		t.Setenv("OPENFGA_STORE_ID", storeID)

		err := s.Run(context.Background(), commands.RunContext{NATSConfig: unreachableNATS()})

		require.ErrorContains(t, err, "failed to connect to NATS")
	})
}

// unreachableNATS points at a closed local port so a subcommand's Connect
// fails immediately, letting tests cover everything that runs before it.
func unreachableNATS() natsinfra.Config {
	return natsinfra.Config{URL: "nats://127.0.0.1:1", Timeout: 200 * time.Millisecond}
}
