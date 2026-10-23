// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// malformedResourceUIDs are client-controlled strings that must never reach a KV Get.
var malformedResourceUIDs = []struct {
	name string
	uid  string
}{
	{"reservation key", "lookup/project-folders/0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
	{"non-uuid string", "folder-1"},
	{"empty", ""},
}

// mustMarshal marshals v to JSON, failing the test on error.
// forwardedRevision matches a jetstream.LastRevision delete option carrying the
// given revision. The option is opaque, so apply it to a fresh options struct
// via reflection and read the revision back.
func forwardedRevision(want uint64) any {
	return mock.MatchedBy(func(opt jetstream.KVDeleteOpt) bool {
		fn := reflect.ValueOf(opt)
		if fn.Kind() != reflect.Func {
			return false
		}
		target := reflect.New(fn.Type().In(0).Elem())
		fn.Call([]reflect.Value{target})
		return target.Elem().FieldByName("revision").Uint() == want
	})
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

func Test_isJetStreamCASConflict(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "jetstream key exists", err: jetstream.ErrKeyExists, want: true},
		{name: "wrapped key exists", err: fmt.Errorf("wrapped: %w", jetstream.ErrKeyExists), want: true},
		{name: "wrong last sequence message", err: errors.New("nats: wrong last sequence: 3"), want: true},
		{name: "unrelated error", err: errors.New("boom"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isJetStreamCASConflict(tt.err))
		})
	}
}

// syncListCase describes one scenario for the ListFolders/ListAllLinks/ListAllDocuments
// tests. The keys and entries are wired into a MockKeyValue by newSyncListKV.
type syncListCase struct {
	name       string
	projectUID string
	listErr    error
	keys       []string
	entries    map[string]jetstream.KeyValueEntry // key -> entry returned by Get
	getErrs    map[string]error                   // key -> error returned by Get
	wantUIDs   []string
	wantErr    error
}

func mustEntry(t *testing.T, v any) jetstream.KeyValueEntry {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return NewMockKeyValueEntry(data, 1)
}

func newSyncListKV(tt syncListCase) *MockKeyValue {
	kv := &MockKeyValue{}
	if tt.listErr != nil {
		kv.On("ListKeys", mock.Anything).Return(nil, tt.listErr)
		return kv
	}
	kv.On("ListKeys", mock.Anything).Return(NewMockKeyLister(tt.keys), nil)
	for key, entry := range tt.entries {
		kv.On("Get", mock.Anything, key).Return(entry, nil)
	}
	for key, err := range tt.getErrs {
		kv.On("Get", mock.Anything, key).Return(nil, err)
	}
	return kv
}

// updateCase describes one scenario for the Update* tests; updateErr is what the KV Update returns.
type updateCase struct {
	name      string
	updateErr error
	wantErr   error
}

var updateCases = []updateCase{
	{name: "success"},
	{name: "key exists conflict maps to revision mismatch", updateErr: jetstream.ErrKeyExists, wantErr: domain.ErrRevisionMismatch},
	{name: "wrong last sequence maps to revision mismatch", updateErr: errors.New("nats: wrong last sequence: 7"), wantErr: domain.ErrRevisionMismatch},
	{name: "other update error maps to internal error", updateErr: errors.New("boom"), wantErr: domain.ErrInternal},
}

// payloadEquals matches a KV write whose JSON body decodes to the wanted record.
func payloadEquals[T any](want T) any {
	return mock.MatchedBy(func(data []byte) bool {
		var got T
		return json.Unmarshal(data, &got) == nil && reflect.DeepEqual(got, want)
	})
}

func newUpdateKV(uid string, revision uint64, payload any, updateErr error) *MockKeyValue {
	kv := &MockKeyValue{}
	kv.On("Update", mock.Anything, uid, payload, revision).Return(uint64(revision+1), updateErr)
	return kv
}
