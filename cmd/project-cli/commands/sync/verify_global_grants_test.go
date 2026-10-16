// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/internal/fgaread"
	natsinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
)

func TestGlobalGrantVerificationReport_add(t *testing.T) {
	t.Run("counts missing, unexpected and conditioned tuples", func(t *testing.T) {
		expected := []fgaread.Tuple{
			{User: "team:formation#member", Relation: "global_owner"},
			{User: "team:product-support#member", Relation: "global_owner"},
		}
		actual := []fgaread.Tuple{
			{User: "team:formation#member", Relation: "global_owner"},
			{User: "team:unexpected#member", Relation: "global_writer"},
			{User: "team:product-support#member", Relation: "global_owner", Conditioned: true},
		}

		report := newGlobalGrantVerificationReport()
		report.add(fgaread.ClassProspect, expected, actual, nil)

		assert.Equal(t, 1, report.Projects)
		assert.Equal(t, 1, report.Missing)
		assert.Equal(t, 2, report.Unexpected)
		assert.Equal(t, 1, report.Conditioned)
		assert.Equal(t, 2, report.Classes[fgaread.ClassProspect].Relations["global_owner"].Expected)
		assert.Equal(t, 1, report.Classes[fgaread.ClassProspect].Relations["global_owner"].Missing)
		assert.Equal(t, 1, report.Classes[fgaread.ClassProspect].Relations["global_owner"].Unexpected)
		assert.Equal(t, 1, report.Classes[fgaread.ClassProspect].Relations["global_writer"].Unexpected)
	})

	t.Run("counts expected tuples when the read fails", func(t *testing.T) {
		report := newGlobalGrantVerificationReport()
		report.add(fgaread.ClassOrdinary, fgaread.ExpectedTuples("Active"), nil, errors.New("read failed"))

		assert.Equal(t, 1, report.ReadErrors)
		assert.Equal(t, 1, report.Classes[fgaread.ClassOrdinary].ReadErrors)
		assert.Equal(t, 2, report.Classes[fgaread.ClassOrdinary].Relations["global_owner"].Expected)
		assert.Equal(t, 1, report.Classes[fgaread.ClassOrdinary].Relations["global_writer"].Expected)
		assert.Equal(t, 2, report.Classes[fgaread.ClassOrdinary].Relations["global_auditor"].Expected)
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

func TestGlobalGrantReadErrorKind(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "http status", err: fgaread.HTTPError{Status: 500}, want: "http_500"},
		{name: "wrapped http status", err: fmt.Errorf("read: %w", fgaread.HTTPError{Status: 429}), want: "http_429"},
		{name: "timeout", err: context.DeadlineExceeded, want: "timeout"},
		{name: "canceled", err: context.Canceled, want: "canceled"},
		{name: "repeated token", err: fgaread.ErrRepeatedContinuationToken, want: "repeated_continuation_token"},
		{name: "page limit", err: fgaread.ErrTooManyReadPages, want: "page_limit"},
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
