// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfigFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		natsURL string
		wantURL string
	}{
		{name: "unset NATS_URL falls back to the default", natsURL: "", wantURL: "nats://localhost:4222"},
		{name: "NATS_URL overrides the default", natsURL: "nats://nats.example.com:4222", wantURL: "nats://nats.example.com:4222"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NATS_URL", tt.natsURL)

			cfg := ConfigFromEnv()

			assert.Equal(t, tt.wantURL, cfg.URL)
			assert.Equal(t, 10*time.Second, cfg.Timeout)
			assert.Equal(t, 3, cfg.MaxReconnect)
			assert.Equal(t, 2*time.Second, cfg.ReconnectWait)
		})
	}
}

func TestApplyConfigDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   Config
		want Config
	}{
		{
			name: "zero config gets every default",
			in:   Config{},
			want: Config{URL: "nats://localhost:4222", Timeout: 10 * time.Second, MaxReconnect: 3, ReconnectWait: 2 * time.Second},
		},
		{
			name: "explicit values are preserved",
			in:   Config{URL: "nats://nats.example.com:4222", Timeout: time.Second, MaxReconnect: 7, ReconnectWait: 5 * time.Second},
			want: Config{URL: "nats://nats.example.com:4222", Timeout: time.Second, MaxReconnect: 7, ReconnectWait: 5 * time.Second},
		},
		{
			name: "only unset fields are defaulted",
			in:   Config{URL: "nats://nats.example.com:4222", MaxReconnect: 1},
			want: Config{URL: "nats://nats.example.com:4222", Timeout: 10 * time.Second, MaxReconnect: 1, ReconnectWait: 2 * time.Second},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, applyConfigDefaults(tt.in))
		})
	}
}
