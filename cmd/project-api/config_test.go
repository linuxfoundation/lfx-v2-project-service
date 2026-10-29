// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want environment
	}{
		{
			name: "defaults when nothing is set",
			env:  map[string]string{},
			want: environment{
				NatsURL:             "nats://localhost:4222",
				Port:                "8080",
				LFXSelfServeBaseURL: "https://app.lfx.dev",
			},
		},
		{
			name: "all values are read from the environment",
			env: map[string]string{
				"PORT":                    "9090",
				"NATS_URL":                "nats://nats.example.com:4222",
				"SKIP_ETAG_VALIDATION":    "true",
				"LFX_SELF_SERVE_BASE_URL": "https://custom.example.com",
				"EMAILS_ENABLED":          "true",
				"INVITES_ENABLED":         "true",
				"FGA_ENABLED":             "true",

				"ARCHIVED_DISSOLUTION_DATE_REQUIRED_FROM": "2026-12-01",
			},
			want: environment{
				NatsURL:             "nats://nats.example.com:4222",
				Port:                "9090",
				SkipEtagValidation:  true,
				LFXSelfServeBaseURL: "https://custom.example.com",
				EmailsEnabled:       true,
				InvitesEnabled:      true,
				FGAEnabled:          true,

				ArchivedDissolutionDateRequiredFrom: "2026-12-01",
			},
		},
		{
			name: "archived dissolution waiver date is trimmed",
			env:  map[string]string{"ARCHIVED_DISSOLUTION_DATE_REQUIRED_FROM": "  2026-12-01\t"},
			want: environment{
				NatsURL:             "nats://localhost:4222",
				Port:                "8080",
				LFXSelfServeBaseURL: "https://app.lfx.dev",

				ArchivedDissolutionDateRequiredFrom: "2026-12-01",
			},
		},
		{
			name: "boolean flags are enabled only by the exact string true",
			env: map[string]string{
				"SKIP_ETAG_VALIDATION": "1",
				"EMAILS_ENABLED":       "TRUE",
				"INVITES_ENABLED":      "yes",
				"FGA_ENABLED":          "false",
			},
			want: environment{
				NatsURL:             "nats://localhost:4222",
				Port:                "8080",
				LFXSelfServeBaseURL: "https://app.lfx.dev",
			},
		},
		{
			name: "base URL is derived from LFX_ENVIRONMENT",
			env:  map[string]string{"LFX_ENVIRONMENT": "staging"},
			want: environment{
				NatsURL:             "nats://localhost:4222",
				Port:                "8080",
				LFXSelfServeBaseURL: "https://app.staging.lfx.dev",
			},
		},
	}

	keys := []string{"PORT", "NATS_URL", "SKIP_ETAG_VALIDATION", "LFX_SELF_SERVE_BASE_URL", "LFX_ENVIRONMENT", "EMAILS_ENABLED", "INVITES_ENABLED", "FGA_ENABLED", "ARCHIVED_DISSOLUTION_DATE_REQUIRED_FROM"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start from a clean slate so ambient variables cannot leak in.
			for _, k := range keys {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			assert.Equal(t, tt.want, parseEnv())
		})
	}
}

func TestArchivedDissolutionDateRequiredFrom(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		raw  string
		want time.Time
	}{
		{name: "empty value means no waiver", raw: "", want: time.Time{}},
		{name: "whitespace-only value means no waiver", raw: "   ", want: time.Time{}},
		{name: "future date activates waiver", raw: "2026-12-01", want: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)},
		{name: "surrounding whitespace is trimmed", raw: " 2026-12-01 ", want: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)},
		{name: "past date is parsed but already expired", raw: "2026-01-01", want: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{name: "invalid format fails closed", raw: "12/01/2026", want: time.Time{}},
		{name: "timestamp instead of date fails closed", raw: "2026-12-01T00:00:00Z", want: time.Time{}},
		{name: "impossible date fails closed", raw: "2026-02-30", want: time.Time{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := archivedDissolutionDateRequiredFrom(tt.raw, now)

			assert.True(t, tt.want.Equal(got), "want %v, got %v", tt.want, got)
			assert.Equal(t, time.UTC, got.Location())
		})
	}
}

func TestLFXSelfServeBaseURL(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		environment string
		want        string
	}{
		{
			name:    "explicit base URL takes precedence",
			baseURL: "https://custom.example.com",
			want:    "https://custom.example.com",
		},
		{
			name:        "prod environment",
			environment: "prod",
			want:        "https://app.lfx.dev",
		},
		{
			name:        "production alias",
			environment: "production",
			want:        "https://app.lfx.dev",
		},
		{
			name:        "staging environment",
			environment: "staging",
			want:        "https://app.staging.lfx.dev",
		},
		{
			name:        "stg alias",
			environment: "stg",
			want:        "https://app.staging.lfx.dev",
		},
		{
			name:        "stage alias",
			environment: "stage",
			want:        "https://app.staging.lfx.dev",
		},
		{
			name:        "dev environment",
			environment: "dev",
			want:        "https://app.dev.lfx.dev",
		},
		{
			name:        "development alias",
			environment: "development",
			want:        "https://app.dev.lfx.dev",
		},
		{
			name: "unset environment defaults to prod",
			want: "https://app.lfx.dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LFX_SELF_SERVE_BASE_URL", tt.baseURL)
			t.Setenv("LFX_ENVIRONMENT", tt.environment)

			assert.Equal(t, tt.want, LFXSelfServeBaseURL())
		})
	}
}

func TestParseFlagSet(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    cmdFlags
		wantErr bool
	}{
		{name: "defaults", args: nil, want: cmdFlags{Port: "8080", Bind: "*"}},
		{name: "debug flag", args: []string{"-d"}, want: cmdFlags{Debug: true, Port: "8080", Bind: "*"}},
		{name: "port flag", args: []string{"-p", "9090"}, want: cmdFlags{Port: "9090", Bind: "*"}},
		{name: "bind flag", args: []string{"-bind", "127.0.0.1"}, want: cmdFlags{Port: "8080", Bind: "127.0.0.1"}},
		{name: "all flags", args: []string{"-d", "-p", "9090", "-bind", "127.0.0.1"}, want: cmdFlags{Debug: true, Port: "9090", Bind: "127.0.0.1"}},
		{name: "unknown flag is an error", args: []string{"-unknown"}, wantErr: true},
		{name: "missing flag value is an error", args: []string{"-p"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			fs.SetOutput(io.Discard)

			got, err := parseFlagSet(fs, tt.args, "8080")

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("default port is taken from the argument", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		got, err := parseFlagSet(fs, nil, "1234")
		require.NoError(t, err)
		assert.Equal(t, "1234", got.Port)
	})
}
