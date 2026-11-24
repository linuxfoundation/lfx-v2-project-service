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

const updateFolderUID = "00000000-0000-0000-0000-0000000000f1"

// folderKeyOf is the name lookup key a folder with this name holds in the bucket.
func folderKeyOf(name string) string {
	folder := &models.ProjectFolder{ProjectUID: updateProjectUID, Name: name}
	return fmt.Sprintf(constants.KVLookupFolderPrefix, folder.BuildIndexKey(context.Background()))
}

// seedFolder stores the folder "Reports" the way a create does: its name reserved, then its record.
func seedFolder(t *testing.T, kv INatsKeyValue) (*NatsRepository, *models.ProjectFolder, uint64) {
	t.Helper()
	ctx := context.Background()
	repo := &NatsRepository{Folders: kv}
	folder := &models.ProjectFolder{
		UID: updateFolderUID, ProjectUID: updateProjectUID, Name: "Reports",
		CreatedBy: &models.UserInfo{Username: "creator"}, UpdatedBy: &models.UserInfo{Username: "creator"},
	}
	_, err := repo.UniqueFolderName(ctx, folder)
	require.NoError(t, err)
	require.NoError(t, repo.CreateFolder(ctx, folder))
	got, revision, err := repo.GetFolder(ctx, updateProjectUID, updateFolderUID)
	require.NoError(t, err)
	return repo, got, revision
}

// The rename sequence itself (reserve, write, release, and settling an ambiguous write error) is shared with documents
// and is exercised in depth by the document tests; these cover what is specific to folders and the main failure cases.
func TestNatsRepository_UpdateFolder(t *testing.T) {
	ctx := context.Background()
	nameHeld := func(kv *memKV, name string) bool {
		_, err := kv.Get(ctx, folderKeyOf(name))
		return err == nil
	}
	stored := func(t *testing.T, kv *memKV) *models.ProjectFolder {
		t.Helper()
		got, _, err := (&NatsRepository{Folders: kv}).GetFolder(ctx, updateProjectUID, updateFolderUID)
		require.NoError(t, err)
		return got
	}

	t.Run("an update that keeps the name replaces the record and touches no key", func(t *testing.T) {
		kv := newMemKV()
		repo, folder, revision := seedFolder(t, kv)

		folder.UpdatedBy = &models.UserInfo{Username: "editor"}
		require.NoError(t, repo.UpdateFolder(ctx, folder, revision))

		got, newRevision, err := repo.GetFolder(ctx, updateProjectUID, updateFolderUID)
		require.NoError(t, err)
		assert.Equal(t, "editor", got.UpdatedBy.Username)
		assert.Greater(t, newRevision, revision)
		assert.True(t, nameHeld(kv, "Reports"))
	})

	t.Run("a rename reserves the new name, writes the record and releases the old name", func(t *testing.T) {
		kv := newMemKV()
		repo, folder, revision := seedFolder(t, kv)

		folder.Name = "Reports 2026"
		require.NoError(t, repo.UpdateFolder(ctx, folder, revision))

		assert.Equal(t, "Reports 2026", stored(t, kv).Name)
		assert.True(t, nameHeld(kv, "Reports 2026"))
		assert.False(t, nameHeld(kv, "Reports"), "the old name is free again")
		entry, err := kv.Get(ctx, folderKeyOf("Reports 2026"))
		require.NoError(t, err)
		assert.True(t, lookupHeldBy(string(entry.Value()), updateFolderUID), "the reservation names the folder that holds it")
	})

	t.Run("a name another folder holds is a conflict that changes nothing", func(t *testing.T) {
		kv := newMemKV()
		repo, folder, revision := seedFolder(t, kv)
		_, err := repo.UniqueFolderName(ctx, &models.ProjectFolder{UID: "00000000-0000-0000-0000-0000000000f2", ProjectUID: updateProjectUID, Name: "Archive"})
		require.NoError(t, err)

		folder.Name = "Archive"
		err = repo.UpdateFolder(ctx, folder, revision)

		assert.Equal(t, domain.ErrFolderNameExists, err)
		assert.Equal(t, "Reports", stored(t, kv).Name)
		assert.True(t, nameHeld(kv, "Reports"))
	})

	t.Run("a stale revision is a mismatch before anything is reserved", func(t *testing.T) {
		kv := newMemKV()
		repo, folder, revision := seedFolder(t, kv)

		folder.Name = "Reports 2026"
		assert.Equal(t, domain.ErrRevisionMismatch, repo.UpdateFolder(ctx, folder, revision-1))
		assert.False(t, nameHeld(kv, "Reports 2026"), "nothing is reserved for a stale revision")
	})

	t.Run("a folder that is not there is not found", func(t *testing.T) {
		repo := &NatsRepository{Folders: newMemKV()}

		err := repo.UpdateFolder(ctx, &models.ProjectFolder{UID: updateFolderUID, ProjectUID: updateProjectUID, Name: "Reports"}, 1)

		assert.Equal(t, domain.ErrFolderNotFound, err)
	})

	t.Run("a store error on the update is an internal error", func(t *testing.T) {
		kv := newMemKV()
		_, folder, revision := seedFolder(t, kv)
		repo := &NatsRepository{Folders: &renameWriteKV{INatsKeyValue: kv, recordKey: updateFolderUID, err: errors.New("boom")}}

		folder.UpdatedBy = &models.UserInfo{Username: "editor"}
		assert.Equal(t, domain.ErrInternal, repo.UpdateFolder(ctx, folder, revision))
	})

	t.Run("a write error that is not a refusal", func(t *testing.T) {
		setup := func(t *testing.T, wrap func(*renameWriteKV)) (*NatsRepository, *memKV, *models.ProjectFolder, uint64, *renameWriteKV) {
			t.Helper()
			kv := newMemKV()
			_, folder, revision := seedFolder(t, kv)
			failing := &renameWriteKV{INatsKeyValue: kv, recordKey: updateFolderUID}
			wrap(failing)
			return &NatsRepository{Folders: failing}, kv, folder, revision, failing
		}

		t.Run("a write that landed but was reported as failed is a success that releases the old name", func(t *testing.T) {
			repo, kv, folder, revision, _ := setup(t, func(k *renameWriteKV) { k.applied, k.err = true, context.DeadlineExceeded })

			folder.Name = "Reports 2026"
			require.NoError(t, repo.UpdateFolder(ctx, folder, revision), "a landed write is reported as done, so the update is indexed")

			assert.Equal(t, "Reports 2026", stored(t, kv).Name)
			assert.True(t, nameHeld(kv, "Reports 2026"))
			assert.False(t, nameHeld(kv, "Reports"))
		})

		t.Run("a write that may still land keeps the name reserved for it", func(t *testing.T) {
			repo, kv, folder, revision, _ := setup(t, func(k *renameWriteKV) { k.err = context.DeadlineExceeded })

			folder.Name = "Reports 2026"
			require.Error(t, repo.UpdateFolder(ctx, folder, revision))

			assert.Equal(t, "Reports", stored(t, kv).Name)
			assert.True(t, nameHeld(kv, "Reports"))
			assert.True(t, nameHeld(kv, "Reports 2026"), "the name stays reserved while the write may still land")
		})

		t.Run("a write the server refuses outright releases the reserved name", func(t *testing.T) {
			repo, kv, folder, revision, _ := setup(t, func(k *renameWriteKV) { k.err = fmt.Errorf("update: %w", jetstream.ErrKeyExists) })

			folder.Name = "Reports 2026"
			assert.Equal(t, domain.ErrRevisionMismatch, repo.UpdateFolder(ctx, folder, revision))

			assert.False(t, nameHeld(kv, "Reports 2026"), "a refused rename must not leave its name locked")
			assert.True(t, nameHeld(kv, "Reports"))
		})

		t.Run("a rename is not reported as done when only the audit users changed afterwards", func(t *testing.T) {
			repo, kv, folder, revision, failing := setup(t, func(k *renameWriteKV) { k.applied, k.err = true, context.DeadlineExceeded })
			other := &NatsRepository{Folders: kv}
			failing.beforeReadBack = func() {
				latest, rev, err := other.GetFolder(ctx, updateProjectUID, updateFolderUID)
				require.NoError(t, err)
				latest.UpdatedBy = &models.UserInfo{Username: "backfilled"}
				require.NoError(t, other.UpdateFolder(ctx, latest, rev))
			}

			folder.Name = "Reports 2026"
			require.Error(t, repo.UpdateFolder(ctx, folder, revision), "reporting this write as the outcome would index the older audit users")

			assert.True(t, nameHeld(kv, "Reports 2026"))
			assert.False(t, nameHeld(kv, "Reports"), "the old name is released, as the write did land")
		})

		for _, applied := range []bool{true, false} {
			t.Run(fmt.Sprintf("a folder deleted before the read-back leaves no name locked (write applied: %t)", applied), func(t *testing.T) {
				repo, kv, folder, revision, failing := setup(t, func(k *renameWriteKV) { k.applied, k.err = applied, context.DeadlineExceeded })
				other := &NatsRepository{Folders: kv}
				failing.beforeReadBack = func() {
					_, rev, err := other.GetFolder(ctx, updateProjectUID, updateFolderUID)
					require.NoError(t, err)
					require.NoError(t, other.DeleteFolder(ctx, updateProjectUID, updateFolderUID, rev))
				}

				folder.Name = "Reports 2026"
				require.Error(t, repo.UpdateFolder(ctx, folder, revision))

				assert.False(t, nameHeld(kv, "Reports"))
				assert.False(t, nameHeld(kv, "Reports 2026"))
			})
		}
	})

	t.Run("a lost reservation reply releases the key this attempt stored, so the same rename can be retried", func(t *testing.T) {
		kv := newMemKV()
		_, folder, revision := seedFolder(t, kv)
		repo := &NatsRepository{Folders: &lostReservationKV{INatsKeyValue: kv, lostKey: folderKeyOf("Reports 2026"), err: context.DeadlineExceeded}}

		folder.Name = "Reports 2026"
		require.Error(t, repo.UpdateFolder(ctx, folder, revision))

		assert.False(t, nameHeld(kv, "Reports 2026"), "the reservation this call stored is released")
		require.NoError(t, (&NatsRepository{Folders: kv}).UpdateFolder(ctx, folder, revision), "the retry is not blocked by the lost reply")
	})

	t.Run("a request that ended after the new name was reserved releases it without writing", func(t *testing.T) {
		kv := newMemKV()
		_, folder, revision := seedFolder(t, kv)
		cancelCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		repo := &NatsRepository{Folders: &cancelAfterCreateKV{INatsKeyValue: kv, cancel: cancel}}

		folder.Name = "Reports 2026"
		require.Error(t, repo.UpdateFolder(cancelCtx, folder, revision))

		assert.Equal(t, "Reports", stored(t, kv).Name, "no write was sent")
		assert.False(t, nameHeld(kv, "Reports 2026"), "the reservation is nobody's, so it is released")
	})

	t.Run("a renamed folder's name is released along with the folder", func(t *testing.T) {
		kv := newMemKV()
		repo, folder, revision := seedFolder(t, kv)
		folder.Name = "Reports 2026"
		require.NoError(t, repo.UpdateFolder(ctx, folder, revision))
		_, revision, err := repo.GetFolder(ctx, updateProjectUID, updateFolderUID)
		require.NoError(t, err)

		require.NoError(t, repo.DeleteFolder(ctx, updateProjectUID, updateFolderUID, revision))

		assert.False(t, nameHeld(kv, "Reports 2026"), "a deleted folder must not leave its renamed name locked")
	})
}
