// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The URL is assembled from parts so no credential-shaped literal is committed
// for secret scanners to flag.
const (
	fakeUser     = "admin"
	fakePassword = "not-a-real-password"
)

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no credentials is unchanged", in: "http://localhost:9200", want: "http://localhost:9200"},
		{name: "user and password are redacted", in: "https://" + fakeUser + ":" + fakePassword + "@search.example.com:9200/x", want: "https://REDACTED@search.example.com:9200/x"},
		{name: "a bare user is redacted", in: "nats://token@nats.example.com:4222", want: "nats://REDACTED@nats.example.com:4222"},
		{name: "an unparseable URL is not echoed", in: "http://[::1", want: "<invalid>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactURL(tt.in)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, fakePassword)
		})
	}
}

func TestJSONBody(t *testing.T) {
	t.Run("encodes the value", func(t *testing.T) {
		r, err := jsonBody(map[string]any{"a": 1})
		require.NoError(t, err)
		raw, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.JSONEq(t, `{"a":1}`, string(raw))
	})

	t.Run("an unencodable value is an error", func(t *testing.T) {
		_, err := jsonBody(map[string]any{"c": make(chan int)})
		require.ErrorContains(t, err, "failed to marshal request body")
	})
}

func TestSleepWithContext(t *testing.T) {
	t.Run("a non-positive duration returns immediately", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.NoError(t, sleepWithContext(ctx, 0))
		require.NoError(t, sleepWithContext(ctx, -time.Second))
	})

	t.Run("waits the duration", func(t *testing.T) {
		start := time.Now()
		require.NoError(t, sleepWithContext(context.Background(), 10*time.Millisecond))
		assert.GreaterOrEqual(t, time.Since(start), 10*time.Millisecond)
	})

	t.Run("a cancelled context interrupts the wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.ErrorIs(t, sleepWithContext(ctx, time.Minute), context.Canceled)
	})
}
