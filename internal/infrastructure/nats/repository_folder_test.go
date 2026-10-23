// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNatsRepository_GetFolder(t *testing.T) {
	const validUID = "00000000-0000-0000-0000-000000000001"

	for _, tt := range malformedResourceUIDs {
		t.Run("malformed uid returns not found without KV read: "+tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{} // no expectations: any Get call fails the test
			repo := &NatsRepository{Folders: mockKV}

			folder, rev, err := repo.GetFolder(context.Background(), "proj-1", tt.uid)

			assert.Equal(t, domain.ErrFolderNotFound, err)
			assert.Nil(t, folder)
			assert.Zero(t, rev)
			mockKV.AssertNotCalled(t, "Get", mock.Anything, mock.Anything)
		})
	}

	t.Run("missing well-formed uid returns not found", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(nil, jetstream.ErrKeyNotFound)
		repo := &NatsRepository{Folders: mockKV}

		_, _, err := repo.GetFolder(context.Background(), "proj-1", validUID)

		assert.Equal(t, domain.ErrFolderNotFound, err)
		mockKV.AssertExpectations(t)
	})

	t.Run("success returns the record with its revision", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(NewMockKeyValueEntry(mustMarshal(t, &models.ProjectFolder{UID: validUID, ProjectUID: "proj-1"}), 9), nil)
		repo := &NatsRepository{Folders: mockKV}

		got, rev, err := repo.GetFolder(context.Background(), "proj-1", validUID)

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, validUID, got.UID)
		assert.Equal(t, uint64(9), rev)
		mockKV.AssertExpectations(t)
	})

	t.Run("key-value store error returns internal error", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(nil, errors.New("nats error"))
		repo := &NatsRepository{Folders: mockKV}

		got, rev, err := repo.GetFolder(context.Background(), "proj-1", validUID)

		assert.Equal(t, domain.ErrInternal, err)
		assert.Nil(t, got)
		assert.Zero(t, rev)
	})

	t.Run("invalid stored json returns unmarshal error", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(NewMockKeyValueEntry([]byte("{not json"), 1), nil)
		repo := &NatsRepository{Folders: mockKV}

		got, rev, err := repo.GetFolder(context.Background(), "proj-1", validUID)

		assert.Equal(t, domain.ErrUnmarshal, err)
		assert.Nil(t, got)
		assert.Zero(t, rev)
	})

	t.Run("record owned by another project returns not found", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(NewMockKeyValueEntry(mustMarshal(t, &models.ProjectFolder{UID: validUID, ProjectUID: "other-proj"}), 3), nil)
		repo := &NatsRepository{Folders: mockKV}

		got, rev, err := repo.GetFolder(context.Background(), "proj-1", validUID)

		assert.Equal(t, domain.ErrFolderNotFound, err)
		assert.Nil(t, got)
		assert.Zero(t, rev)
	})
}

func TestNatsRepository_CreateFolder(t *testing.T) {
	folder := &models.ProjectFolder{UID: "00000000-0000-0000-0000-000000000001", ProjectUID: "proj-1", Name: "Reports"}

	tests := []struct {
		name    string
		putErr  error
		wantErr error
	}{
		{name: "success"},
		{name: "put fails", putErr: errors.New("nats error"), wantErr: domain.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{}
			mockKV.On("Put", mock.Anything, folder.UID, mock.Anything).Return(uint64(1), tt.putErr)
			repo := &NatsRepository{Folders: mockKV}

			err := repo.CreateFolder(context.Background(), folder)

			assert.Equal(t, tt.wantErr, err)
			mockKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_DeleteFolder(t *testing.T) {
	const (
		projectUID = "proj-1"
		folderUID  = "00000000-0000-0000-0000-000000000001"
	)
	folder := &models.ProjectFolder{UID: folderUID, ProjectUID: projectUID, Name: "Reports"}
	uniqueKey := fmt.Sprintf(constants.KVLookupFolderPrefix, folder.BuildIndexKey(context.Background()))
	folderEntry := func() *MockKeyValueEntry { return NewMockKeyValueEntry(mustMarshal(t, folder), 9) }

	tests := []struct {
		name       string
		setupMocks func(*MockKeyValue)
		wantErr    error
	}{
		{
			name: "folder not found",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, folderUID).Return(nil, jetstream.ErrKeyNotFound)
			},
			wantErr: domain.ErrFolderNotFound,
		},
		{
			name: "revision mismatch",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, folderUID).Return(folderEntry(), nil)
				kv.On("Delete", mock.Anything, folderUID, forwardedRevision(4)).Return(errors.New("wrong last sequence"))
			},
			wantErr: domain.ErrRevisionMismatch,
		},
		{
			name: "other delete error",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, folderUID).Return(folderEntry(), nil)
				kv.On("Delete", mock.Anything, folderUID, forwardedRevision(4)).Return(errors.New("nats error"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name: "success purges the name reservation",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, folderUID).Return(folderEntry(), nil)
				kv.On("Delete", mock.Anything, folderUID, forwardedRevision(4)).Return(nil)
				kv.On("Purge", mock.Anything, uniqueKey).Return(nil)
			},
		},
		{
			name: "name reservation purge failure only warns",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, folderUID).Return(folderEntry(), nil)
				kv.On("Delete", mock.Anything, folderUID, forwardedRevision(4)).Return(nil)
				kv.On("Purge", mock.Anything, uniqueKey).Return(errors.New("purge failed"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{}
			tt.setupMocks(mockKV)
			repo := &NatsRepository{Folders: mockKV}

			err := repo.DeleteFolder(context.Background(), projectUID, folderUID, 4)

			assert.Equal(t, tt.wantErr, err)
			mockKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_UniqueFolderName(t *testing.T) {
	folder := &models.ProjectFolder{UID: "00000000-0000-0000-0000-000000000001", ProjectUID: "proj-1", Name: "Reports"}
	wantKey := fmt.Sprintf(constants.KVLookupFolderPrefix, folder.BuildIndexKey(context.Background()))

	tests := []struct {
		name      string
		createErr error
		wantKey   string
		wantErr   error
	}{
		{name: "reserves the name", wantKey: wantKey},
		{name: "name already taken", createErr: jetstream.ErrKeyExists, wantErr: domain.ErrFolderNameExists},
		{name: "other store error", createErr: errors.New("nats error"), wantErr: domain.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{}
			mockKV.On("Create", mock.Anything, wantKey, []byte(folder.UID)).Return(uint64(1), tt.createErr)
			repo := &NatsRepository{Folders: mockKV}

			key, err := repo.UniqueFolderName(context.Background(), folder)

			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantKey, key)
			mockKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_DeleteUniqueFolderName(t *testing.T) {
	tests := []struct {
		name     string
		purgeErr error
		wantErr  error
	}{
		{name: "releases the reservation"},
		{name: "purge fails", purgeErr: errors.New("nats error"), wantErr: domain.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{}
			mockKV.On("Purge", mock.Anything, "lookup/project-folders/key").Return(tt.purgeErr)
			repo := &NatsRepository{Folders: mockKV}

			err := repo.DeleteUniqueFolderName(context.Background(), "lookup/project-folders/key")

			assert.Equal(t, tt.wantErr, err)
			mockKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_ListFolders(t *testing.T) {
	folder := func(uid, projectUID string) *models.ProjectFolder {
		return &models.ProjectFolder{UID: uid, ProjectUID: projectUID, Name: "Folder " + uid}
	}
	tests := []syncListCase{
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
			kv := newSyncListKV(tt)
			repo.Folders = kv

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
