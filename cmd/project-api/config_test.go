// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
			},
			want: environment{
				NatsURL:             "nats://nats.example.com:4222",
				Port:                "9090",
				SkipEtagValidation:  true,
				LFXSelfServeBaseURL: "https://custom.example.com",
				EmailsEnabled:       true,
				InvitesEnabled:      true,
				FGAEnabled:          true,
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

	keys := []string{"PORT", "NATS_URL", "SKIP_ETAG_VALIDATION", "LFX_SELF_SERVE_BASE_URL", "LFX_ENVIRONMENT", "EMAILS_ENABLED", "INVITES_ENABLED", "FGA_ENABLED"}

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
