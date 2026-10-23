// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testRecord struct {
	UID        string `json:"uid"`
	ProjectUID string `json:"project_uid"`
}

var errTestRecordNotFound = errors.New("test record not found")

func newTestCollection(kv INatsKeyValue) kvCollection[testRecord] {
	return kvCollection[testRecord]{
		kv:         kv,
		noun:       "record",
		notFound:   errTestRecordNotFound,
		projectUID: func(r *testRecord) string { return r.ProjectUID },
	}
}

func TestKVCollection_get(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name         string
		setupMocks   func(*MockKeyValue)
		wantErr      error
		wantRevision uint64
	}{
		{
			name: "returns the decoded record and its revision",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, uid).Return(NewMockKeyValueEntry(mustMarshal(t, &testRecord{UID: uid, ProjectUID: "p1"}), 7), nil)
			},
			wantRevision: 7,
		},
		{
			name: "missing key maps to the collection's not-found error",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, uid).Return(nil, jetstream.ErrKeyNotFound)
			},
			wantErr: errTestRecordNotFound,
		},
		{
			name: "store failure maps to internal error",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, uid).Return(nil, errors.New("boom"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name: "invalid json maps to unmarshal error",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, uid).Return(NewMockKeyValueEntry([]byte("{not json"), 1), nil)
			},
			wantErr: domain.ErrUnmarshal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := &MockKeyValue{}
			tt.setupMocks(kv)

			got, revision, err := newTestCollection(kv).get(context.Background(), uid)

			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, uid, got.UID)
				assert.Equal(t, tt.wantRevision, revision)
			}
			kv.AssertExpectations(t)
		})
	}
}

func TestKVCollection_getForProject(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name       string
		uid        string
		setupMocks func(*MockKeyValue)
		wantErr    error
	}{
		{
			name: "record owned by the project is returned",
			uid:  uid,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, uid).Return(NewMockKeyValueEntry(mustMarshal(t, &testRecord{UID: uid, ProjectUID: "p1"}), 1), nil)
			},
		},
		{
			name:       "uid that is not shaped like a UID is not found without reading the store",
			uid:        "lookup/project-links/p1/x",
			setupMocks: func(*MockKeyValue) {},
			wantErr:    errTestRecordNotFound,
		},
		{
			name: "record owned by another project is not found",
			uid:  uid,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, uid).Return(NewMockKeyValueEntry(mustMarshal(t, &testRecord{UID: uid, ProjectUID: "other"}), 1), nil)
			},
			wantErr: errTestRecordNotFound,
		},
		{
			name: "store failure is returned as internal error",
			uid:  uid,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, uid).Return(nil, errors.New("boom"))
			},
			wantErr: domain.ErrInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := &MockKeyValue{}
			tt.setupMocks(kv)

			got, _, err := newTestCollection(kv).getForProject(context.Background(), "p1", tt.uid)

			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, uid, got.UID)
			}
			kv.AssertExpectations(t)
		})
	}
}

func TestKVCollection_list(t *testing.T) {
	const (
		uid1 = "00000000-0000-0000-0000-000000000001"
		uid2 = "00000000-0000-0000-0000-000000000002"
	)
	keepP1 := func(r *testRecord) bool { return r.ProjectUID == "p1" }

	tests := []struct {
		name       string
		setupMocks func(*MockKeyValue)
		wantUIDs   []string
		wantErr    error
	}{
		{
			name: "skips lookup keys, deleted records and records the filter rejects",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{"lookup/x", "gone", uid1, uid2}), nil)
				kv.On("Get", mock.Anything, "gone").Return(nil, jetstream.ErrKeyNotFound)
				kv.On("Get", mock.Anything, uid1).Return(NewMockKeyValueEntry(mustMarshal(t, &testRecord{UID: uid1, ProjectUID: "p1"}), 1), nil)
				kv.On("Get", mock.Anything, uid2).Return(NewMockKeyValueEntry(mustMarshal(t, &testRecord{UID: uid2, ProjectUID: "p2"}), 1), nil)
			},
			wantUIDs: []string{uid1},
		},
		{
			name: "empty bucket returns an empty non-nil slice",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(NewMockKeyLister(nil), nil)
			},
			wantUIDs: []string{},
		},
		{
			name: "listing failure maps to internal error",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(nil, errors.New("boom"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name: "read failure aborts the list",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{uid1}), nil)
				kv.On("Get", mock.Anything, uid1).Return(nil, errors.New("boom"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name: "undecodable record aborts the list",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{uid1}), nil)
				kv.On("Get", mock.Anything, uid1).Return(NewMockKeyValueEntry([]byte("{not json"), 1), nil)
			},
			wantErr: domain.ErrUnmarshal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := &MockKeyValue{}
			tt.setupMocks(kv)

			got, err := newTestCollection(kv).list(context.Background(), keepP1)

			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				uids := []string{}
				for _, r := range got {
					uids = append(uids, r.UID)
				}
				assert.Equal(t, tt.wantUIDs, uids)
			}
			kv.AssertExpectations(t)
		})
	}
}

func TestKVCollection_put(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name    string
		kvErr   error
		wantErr error
	}{
		{name: "stores the record"},
		{name: "store failure maps to internal error", kvErr: errors.New("boom"), wantErr: domain.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := &MockKeyValue{}
			kv.On("Put", mock.Anything, uid, mock.Anything).Return(uint64(1), tt.kvErr)

			err := newTestCollection(kv).put(context.Background(), uid, &testRecord{UID: uid, ProjectUID: "p1"})

			assert.Equal(t, tt.wantErr, err)
			kv.AssertExpectations(t)
		})
	}
}

func TestKVCollection_update(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name    string
		kvErr   error
		wantErr error
	}{
		{name: "replaces the record at the revision"},
		{name: "stale revision maps to revision mismatch", kvErr: errors.New("nats: wrong last sequence: 5"), wantErr: domain.ErrRevisionMismatch},
		{name: "store failure maps to internal error", kvErr: errors.New("boom"), wantErr: domain.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := &MockKeyValue{}
			kv.On("Update", mock.Anything, uid, mock.Anything, uint64(4)).Return(uint64(5), tt.kvErr)

			err := newTestCollection(kv).update(context.Background(), uid, &testRecord{UID: uid, ProjectUID: "p1"}, 4)

			assert.Equal(t, tt.wantErr, err)
			kv.AssertExpectations(t)
		})
	}
}

func TestKVCollection_deleteAt(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name    string
		kvErr   error
		wantErr error
	}{
		{name: "removes the record at the revision"},
		{name: "stale revision maps to revision mismatch", kvErr: jetstream.ErrKeyExists, wantErr: domain.ErrRevisionMismatch},
		{name: "store failure maps to internal error", kvErr: errors.New("boom"), wantErr: domain.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := &MockKeyValue{}
			kv.On("Delete", mock.Anything, uid, forwardedRevision(4)).Return(tt.kvErr)

			err := newTestCollection(kv).deleteAt(context.Background(), uid, 4)

			assert.Equal(t, tt.wantErr, err)
			kv.AssertExpectations(t)
		})
	}
}
