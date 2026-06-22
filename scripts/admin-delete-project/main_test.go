// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubKeyValue satisfies jetstream.KeyValue for tests; only ListKeys is implemented.
// Calls to any other method will panic — intentional, since tests should not reach them.
type stubKeyValue struct {
	jetstream.KeyValue
	listKeysFn func(context.Context, ...jetstream.WatchOpt) (jetstream.KeyLister, error)
}

func (s *stubKeyValue) ListKeys(ctx context.Context, opts ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	return s.listKeysFn(ctx, opts...)
}

// stubKeyLister satisfies jetstream.KeyLister. Channel lifecycle is managed by
// the test — Stop() is intentionally a no-op.
type stubKeyLister struct {
	ch chan string
}

func (s *stubKeyLister) Keys() <-chan string { return s.ch }
func (s *stubKeyLister) Stop() error         { return nil }

func TestExtractSlugFromBase(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		wantSlug string
		wantErr  bool
	}{
		{
			name:     "valid JSON with slug",
			input:    []byte(`{"slug":"my-project","name":"My Project"}`),
			wantSlug: "my-project",
		},
		{
			name:  "slug field absent",
			input: []byte(`{"name":"My Project"}`),
		},
		{
			name:  "slug is non-string value",
			input: []byte(`{"slug":123,"name":"My Project"}`),
		},
		{
			name:    "invalid JSON returns error",
			input:   []byte(`not-json`),
			wantErr: true,
		},
		{
			name:    "empty bytes returns error",
			input:   []byte{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug, obj, err := extractSlugFromBase(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, slug)
				assert.Nil(t, obj)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSlug, slug)
			assert.NotNil(t, obj)
		})
	}
}

func TestListAllKeys(t *testing.T) {
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name       string
		ctx        context.Context
		listKeysFn func(context.Context, ...jetstream.WatchOpt) (jetstream.KeyLister, error)
		wantKeys   []string
		wantErr    error
		wantErrStr string
	}{
		{
			name: "returns all keys when channel closes normally",
			ctx:  context.Background(),
			listKeysFn: func(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
				ch := make(chan string, 3)
				ch <- "key1"
				ch <- "key2"
				ch <- "key3"
				close(ch)
				return &stubKeyLister{ch: ch}, nil
			},
			wantKeys: []string{"key1", "key2", "key3"},
		},
		{
			name: "empty bucket returns empty slice",
			ctx:  context.Background(),
			listKeysFn: func(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
				ch := make(chan string)
				close(ch)
				return &stubKeyLister{ch: ch}, nil
			},
			wantKeys: nil,
		},
		{
			name: "context cancellation returns context error",
			ctx:  canceledCtx,
			listKeysFn: func(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
				return &stubKeyLister{ch: make(chan string)}, nil // never sends
			},
			wantErr: context.Canceled,
		},
		{
			name: "ListKeys error is propagated",
			ctx:  context.Background(),
			listKeysFn: func(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
				return nil, errors.New("NATS unavailable")
			},
			wantErrStr: "NATS unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := &stubKeyValue{listKeysFn: tt.listKeysFn}
			got, err := listAllKeys(tt.ctx, kv)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			if tt.wantErrStr != "" {
				require.EqualError(t, err, tt.wantErrStr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantKeys, got)
		})
	}
}
