// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// syncListCase describes one scenario for the ListFolders/ListAllLinks/ListAllDocuments
// tests. The keys and entries are wired into a MockKeyValue by newSyncListKV.
type syncListCase struct {
	name       string
	nilStore   bool
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

func TestNatsRepository_ListFolders(t *testing.T) {
	folder := func(uid, projectUID string) *models.ProjectFolder {
		return &models.ProjectFolder{UID: uid, ProjectUID: projectUID, Name: "Folder " + uid}
	}
	tests := []syncListCase{
		{name: "nil store returns internal error", nilStore: true, wantErr: domain.ErrInternal},
		{name: "list keys error returns internal error", listErr: errors.New("boom"), wantErr: domain.ErrInternal},
		{
			name: "empty project uid returns all folders and skips lookup keys",
			keys: []string{"lookup/project-folders/abc", "f1", "f2"},
			entries: map[string]jetstream.KeyValueEntry{
				"f1": nil, "f2": nil, // replaced below
			},
			wantUIDs: []string{"f1", "f2"},
		},
		{
			name:       "project uid filters folders from other projects",
			projectUID: "p1",
			keys:       []string{"f1", "f2"},
			entries:    map[string]jetstream.KeyValueEntry{"f1": nil, "f2": nil},
			wantUIDs:   []string{"f1"},
		},
		{
			name:     "key not found is skipped",
			keys:     []string{"gone", "f1"},
			getErrs:  map[string]error{"gone": jetstream.ErrKeyNotFound},
			entries:  map[string]jetstream.KeyValueEntry{"f1": nil},
			wantUIDs: []string{"f1"},
		},
		{
			name:    "get error returns internal error",
			keys:    []string{"f1"},
			getErrs: map[string]error{"f1": errors.New("boom")},
			wantErr: domain.ErrInternal,
		},
		{
			name:    "unmarshal error returns unmarshal error",
			keys:    []string{"f1"},
			entries: map[string]jetstream.KeyValueEntry{"f1": NewMockKeyValueEntry([]byte("not-json"), 1)},
			wantErr: domain.ErrUnmarshal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Populate placeholder entries with real folder records: f1 belongs to p1, f2 to p2.
			for key := range tt.entries {
				if tt.entries[key] == nil {
					owner := "p1"
					if key == "f2" {
						owner = "p2"
					}
					tt.entries[key] = mustEntry(t, folder(key, owner))
				}
			}
			repo := &NatsRepository{}
			var kv *MockKeyValue
			if !tt.nilStore {
				kv = newSyncListKV(tt)
				repo.Folders = kv
			}

			got, err := repo.ListFolders(context.Background(), tt.projectUID)

			// Assert before the error branch returns so error cases also prove the expected store calls happened.
			if kv != nil {
				kv.AssertExpectations(t)
			}
			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			uids := []string{}
			for _, f := range got {
				uids = append(uids, f.UID)
			}
			assert.Equal(t, tt.wantUIDs, uids)
		})
	}
}

func TestNatsRepository_ListAllLinks(t *testing.T) {
	link := func(uid, projectUID string) *models.ProjectLink {
		return &models.ProjectLink{UID: uid, ProjectUID: projectUID, Name: "Link " + uid, URL: "https://example.com/" + uid}
	}
	tests := []syncListCase{
		{name: "nil store returns internal error", nilStore: true, wantErr: domain.ErrInternal},
		{name: "list keys error returns internal error", listErr: errors.New("boom"), wantErr: domain.ErrInternal},
		{
			name:     "empty project uid returns all links and skips lookup keys",
			keys:     []string{"lookup/project-links/p1/l1", "l1", "l2"},
			entries:  map[string]jetstream.KeyValueEntry{"l1": nil, "l2": nil},
			wantUIDs: []string{"l1", "l2"},
		},
		{
			name:     "key not found is skipped",
			keys:     []string{"gone", "l1"},
			getErrs:  map[string]error{"gone": jetstream.ErrKeyNotFound},
			entries:  map[string]jetstream.KeyValueEntry{"l1": nil},
			wantUIDs: []string{"l1"},
		},
		{
			name:    "get error returns internal error",
			keys:    []string{"l1"},
			getErrs: map[string]error{"l1": errors.New("boom")},
			wantErr: domain.ErrInternal,
		},
		{
			name:    "unmarshal error returns unmarshal error",
			keys:    []string{"l1"},
			entries: map[string]jetstream.KeyValueEntry{"l1": NewMockKeyValueEntry([]byte("not-json"), 1)},
			wantErr: domain.ErrUnmarshal,
		},
		{
			name:       "project uid delegates to the per-project index scan",
			projectUID: "p1",
			keys:       []string{"lookup/project-links/p1/l1", "lookup/project-links/p2/l2"},
			entries:    map[string]jetstream.KeyValueEntry{"l1": nil},
			wantUIDs:   []string{"l1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key := range tt.entries {
				if tt.entries[key] == nil {
					tt.entries[key] = mustEntry(t, link(key, "p1"))
				}
			}
			repo := &NatsRepository{}
			var kv *MockKeyValue
			if !tt.nilStore {
				kv = newSyncListKV(tt)
				repo.Links = kv
			}

			got, err := repo.ListAllLinks(context.Background(), tt.projectUID)

			// Assert before the error branch returns so error cases also prove the expected store calls happened.
			if kv != nil {
				kv.AssertExpectations(t)
			}
			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			uids := []string{}
			for _, l := range got {
				uids = append(uids, l.UID)
			}
			assert.Equal(t, tt.wantUIDs, uids)
		})
	}
}

func TestNatsRepository_ListAllDocuments(t *testing.T) {
	doc := func(uid, projectUID string) *models.ProjectDocument {
		return &models.ProjectDocument{UID: uid, ProjectUID: projectUID, Name: "Doc " + uid, FileName: uid + ".pdf"}
	}
	tests := []syncListCase{
		{name: "nil store returns internal error", nilStore: true, wantErr: domain.ErrInternal},
		{name: "list keys error returns internal error", listErr: errors.New("boom"), wantErr: domain.ErrInternal},
		{
			name:     "empty project uid returns all documents and skips lookup keys",
			keys:     []string{"lookup/project-documents/abc", "d1", "d2"},
			entries:  map[string]jetstream.KeyValueEntry{"d1": nil, "d2": nil},
			wantUIDs: []string{"d1", "d2"},
		},
		{
			name:       "project uid filters documents from other projects",
			projectUID: "p1",
			keys:       []string{"d1", "d2"},
			entries:    map[string]jetstream.KeyValueEntry{"d1": nil, "d2": nil},
			wantUIDs:   []string{"d1"},
		},
		{
			name:     "key not found is skipped",
			keys:     []string{"gone", "d1"},
			getErrs:  map[string]error{"gone": jetstream.ErrKeyNotFound},
			entries:  map[string]jetstream.KeyValueEntry{"d1": nil},
			wantUIDs: []string{"d1"},
		},
		{
			name:    "get error returns internal error",
			keys:    []string{"d1"},
			getErrs: map[string]error{"d1": errors.New("boom")},
			wantErr: domain.ErrInternal,
		},
		{
			name:    "unmarshal error returns unmarshal error",
			keys:    []string{"d1"},
			entries: map[string]jetstream.KeyValueEntry{"d1": NewMockKeyValueEntry([]byte("not-json"), 1)},
			wantErr: domain.ErrUnmarshal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key := range tt.entries {
				if tt.entries[key] == nil {
					owner := "p1"
					if key == "d2" {
						owner = "p2"
					}
					tt.entries[key] = mustEntry(t, doc(key, owner))
				}
			}
			repo := &NatsRepository{}
			var kv *MockKeyValue
			if !tt.nilStore {
				kv = newSyncListKV(tt)
				repo.Documents = kv
			}

			got, err := repo.ListAllDocuments(context.Background(), tt.projectUID)

			// Assert before the error branch returns so error cases also prove the expected store calls happened.
			if kv != nil {
				kv.AssertExpectations(t)
			}
			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			uids := []string{}
			for _, d := range got {
				uids = append(uids, d.UID)
			}
			assert.Equal(t, tt.wantUIDs, uids)
		})
	}
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

func TestNatsRepository_UpdateFolder(t *testing.T) {
	for _, tt := range updateCases {
		t.Run(tt.name, func(t *testing.T) {
			folder := models.ProjectFolder{UID: "f1", ProjectUID: "p1", Name: "Folder"}
			kv := newUpdateKV("f1", 4, payloadEquals(folder), tt.updateErr)
			repo := &NatsRepository{Folders: kv}

			err := repo.UpdateFolder(context.Background(), &folder, 4)

			assert.Equal(t, tt.wantErr, err)
			kv.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_UpdateLink(t *testing.T) {
	for _, tt := range updateCases {
		t.Run(tt.name, func(t *testing.T) {
			link := models.ProjectLink{UID: "l1", ProjectUID: "p1", Name: "Link", URL: "https://example.com"}
			kv := newUpdateKV("l1", 4, payloadEquals(link), tt.updateErr)
			repo := &NatsRepository{Links: kv}

			err := repo.UpdateLink(context.Background(), &link, 4)

			assert.Equal(t, tt.wantErr, err)
			kv.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_UpdateDocumentMetadata(t *testing.T) {
	for _, tt := range updateCases {
		t.Run(tt.name, func(t *testing.T) {
			doc := models.ProjectDocument{UID: "d1", ProjectUID: "p1", Name: "Doc"}
			kv := newUpdateKV("d1", 4, payloadEquals(doc), tt.updateErr)
			repo := &NatsRepository{Documents: kv}

			err := repo.UpdateDocumentMetadata(context.Background(), &doc, 4)

			assert.Equal(t, tt.wantErr, err)
			kv.AssertExpectations(t)
		})
	}
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
