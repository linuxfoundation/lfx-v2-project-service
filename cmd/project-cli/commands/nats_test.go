// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natsinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

func TestWithServiceAuthorization(t *testing.T) {
	ctx := WithServiceAuthorization(context.Background())

	assert.Equal(t, "Bearer lfx-v2-project-service", ctx.Value(constants.AuthorizationContextID))
}

func TestConnectRepository(t *testing.T) {
	conn, repo, err := ConnectRepository(context.Background(), natsinfra.Config{URL: "nats://127.0.0.1:1", Timeout: 200 * time.Millisecond})

	require.ErrorContains(t, err, "failed to connect to NATS")
	assert.Nil(t, conn)
	assert.Nil(t, repo)
}

type stubFlusher struct {
	err     error
	timeout time.Duration
}

func (s *stubFlusher) FlushTimeout(timeout time.Duration) error {
	s.timeout = timeout
	return s.err
}

func TestFinish(t *testing.T) {
	tests := []struct {
		name     string
		failed   int
		flushErr error
		wantErr  string
	}{
		{name: "clean run"},
		{name: "failed records become an error", failed: 3, wantErr: "3 project(s) failed to reindex"},
		{name: "a flush error is reported", flushErr: errors.New("timeout"), wantErr: "flush NATS connection: timeout"},
		{name: "a flush error takes precedence over failed records", failed: 2, flushErr: errors.New("timeout"), wantErr: "flush NATS connection"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := NewStats()
			stats.Failed = tt.failed
			conn := &stubFlusher{err: tt.flushErr}

			err := Finish(context.Background(), "sync demo", stats, conn, "project(s) failed to reindex")

			assert.Equal(t, flushTimeout, conn.timeout, "the connection is always flushed, even when records failed")
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
