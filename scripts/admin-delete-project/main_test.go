// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	pnats "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	natsio "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// stubKeyValue satisfies jetstream.KeyValue for tests; only ListKeys is implemented.
// Calls to any other method will panic — intentional, since tests should not reach them.
type stubKeyValue struct {
	jetstream.KeyValue
	listKeysFn func(context.Context, ...jetstream.WatchOpt) (jetstream.KeyLister, error)
}

// deleteKeyValue exposes only the delete operation used during cleanup. Any
// unexpected reads or writes panic rather than being silently accepted.
type deleteKeyValue struct {
	jetstream.KeyValue
	deletes *pnats.MockKeyValue
}

func (s *deleteKeyValue) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	return s.deletes.Delete(ctx, key, opts...)
}

func deleteRevision(want uint64) any {
	return mock.MatchedBy(func(opt jetstream.KVDeleteOpt) bool {
		fn := reflect.ValueOf(opt)
		target := reflect.New(fn.Type().In(0).Elem())
		fn.Call([]reflect.Value{target})
		return target.Elem().FieldByName("revision").Uint() == want
	})
}

func TestExecuteDelete(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"
	const canonicalUID = "00000000-0000-0000-0000-000000000002"
	fgaFailure := errors.New("FGA publish failed")
	settingsFailure := errors.New("settings cleanup failed")
	tests := []struct {
		name        string
		indexedUID  string
		slugFound   bool
		slugErr     error
		fgaErr      error
		settingsErr error
		wantErrs    []error
	}{
		{name: "owned slug is deleted with audited revision", indexedUID: uid, slugFound: true},
		{name: "deleting a duplicate preserves the canonical slug index", indexedUID: canonicalUID, slugFound: true},
		{name: "a slug reassigned after audit is protected by CAS but settings cleanup continues", indexedUID: uid, slugFound: true, slugErr: jetstream.ErrKeyExists, wantErrs: []error{jetstream.ErrKeyExists}},
		{name: "already absent slug is idempotent", indexedUID: uid, slugFound: true, slugErr: jetstream.ErrKeyNotFound},
		{name: "missing slug in audit is not deleted"},
		{name: "FGA and slug failures are both returned after settings cleanup", indexedUID: uid, slugFound: true, slugErr: jetstream.ErrKeyExists, fgaErr: fgaFailure, wantErrs: []error{jetstream.ErrKeyExists, fgaFailure}},
		{name: "FGA and settings failures are both returned", indexedUID: uid, slugFound: true, fgaErr: fgaFailure, settingsErr: settingsFailure, wantErrs: []error{fgaFailure, settingsFailure}},
		{name: "all cleanup failures are preserved", indexedUID: uid, slugFound: true, slugErr: jetstream.ErrKeyExists, fgaErr: fgaFailure, settingsErr: settingsFailure, wantErrs: []error{jetstream.ErrKeyExists, fgaFailure, settingsFailure}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projects := &pnats.MockKeyValue{}
			settings := &pnats.MockKeyValue{}
			conn := &pnats.MockNATSConn{}
			projects.On("Delete", mock.Anything, uid, deleteRevision(11)).Return(nil).Once()
			if tt.slugFound && tt.indexedUID == uid {
				projects.On("Delete", mock.Anything, "slug/example-project", deleteRevision(12)).Return(tt.slugErr).Once()
			}
			settings.On("Delete", mock.Anything, uid).Return(tt.settingsErr).Once()
			for _, subject := range []string{constants.IndexProjectSubject, constants.IndexProjectSettingsSubject, fgaconstants.GenericDeleteAccessSubject} {
				var publishErr error
				if subject == fgaconstants.GenericDeleteAccessSubject {
					publishErr = tt.fgaErr
				}
				conn.On("PublishMsg", mock.MatchedBy(func(msg *natsio.Msg) bool {
					if msg.Subject != subject {
						return false
					}
					var envelope struct {
						Data json.RawMessage `json:"data"`
					}
					if json.Unmarshal(msg.Data, &envelope) != nil {
						return false
					}
					var deletedUID string
					if json.Unmarshal(envelope.Data, &deletedUID) == nil {
						return deletedUID == uid
					}
					var access struct {
						UID string `json:"uid"`
					}
					return json.Unmarshal(envelope.Data, &access) == nil && access.UID == uid
				})).Return(publishErr).Once()
			}

			rec := projectAudit{
				UID: uid, Slug: "example-project",
				Base:     auditedKVEntry{Found: true, Revision: 11},
				SlugKV:   auditedKVEntry{Key: "slug/example-project", Found: tt.slugFound, Revision: 12, Value: []byte(tt.indexedUID)},
				Settings: auditedKVEntry{Found: true},
			}
			err := executeDelete(t.Context(), kvBuckets{
				Projects: &deleteKeyValue{deletes: projects}, ProjectSettings: &deleteKeyValue{deletes: settings},
			}, &pnats.MessageBuilder{NatsConn: conn}, rec, false)
			if len(tt.wantErrs) != 0 {
				for _, wantErr := range tt.wantErrs {
					require.ErrorIs(t, err, wantErr)
				}
			} else {
				require.NoError(t, err)
			}
			projects.AssertExpectations(t)
			settings.AssertExpectations(t)
			conn.AssertExpectations(t)
		})
	}
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

func TestSanitizeNATSURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "URL with user and password is redacted",
			raw:  "nats://user:secret@localhost:4222",
			want: "nats://user@localhost:4222",
		},
		{
			name: "URL with only username is unchanged",
			raw:  "nats://user@localhost:4222",
			want: "nats://user@localhost:4222",
		},
		{
			name: "URL without credentials is unchanged",
			raw:  "nats://localhost:4222",
			want: "nats://localhost:4222",
		},
		{
			name: "invalid URL is returned as-is",
			raw:  "://bad-url",
			want: "://bad-url",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeNATSURL(tt.raw))
		})
	}
}

func TestStringSliceFlagSet(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:  "valid UUID is accepted",
			input: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:  "valid UUID uppercase is accepted",
			input: "550E8400-E29B-41D4-A716-446655440000",
		},
		{
			name:    "empty string is rejected",
			input:   "",
			wantErr: "--uid cannot be empty",
		},
		{
			name:    "whitespace-only is rejected",
			input:   "   ",
			wantErr: "--uid cannot be empty",
		},
		{
			name:    "non-UUID string is rejected",
			input:   "not-a-uuid",
			wantErr: `--uid "not-a-uuid" is not a valid UUID`,
		},
		{
			name:    "short hex string is rejected",
			input:   "abc123",
			wantErr: `--uid "abc123" is not a valid UUID`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f stringSliceFlag
			err := f.Set(tt.input)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Empty(t, f)
				return
			}
			require.NoError(t, err)
			assert.Len(t, f, 1)
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
