// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNatsRepository_GetDocumentMetadata(t *testing.T) {
	const validUID = "00000000-0000-0000-0000-000000000003"

	for _, tt := range malformedResourceUIDs {
		t.Run("malformed uid returns not found without KV read: "+tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{}
			repo := &NatsRepository{Documents: mockKV}

			doc, rev, err := repo.GetDocumentMetadata(context.Background(), "proj-1", tt.uid)

			assert.Equal(t, domain.ErrDocumentNotFound, err)
			assert.Nil(t, doc)
			assert.Zero(t, rev)
			mockKV.AssertNotCalled(t, "Get", mock.Anything, mock.Anything)
		})
	}

	t.Run("missing well-formed uid returns not found", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(nil, jetstream.ErrKeyNotFound)
		repo := &NatsRepository{Documents: mockKV}

		_, _, err := repo.GetDocumentMetadata(context.Background(), "proj-1", validUID)

		assert.Equal(t, domain.ErrDocumentNotFound, err)
		mockKV.AssertExpectations(t)
	})

	t.Run("success returns the record with its revision", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(NewMockKeyValueEntry(mustMarshal(t, &models.ProjectDocument{UID: validUID, ProjectUID: "proj-1"}), 9), nil)
		repo := &NatsRepository{Documents: mockKV}

		got, rev, err := repo.GetDocumentMetadata(context.Background(), "proj-1", validUID)

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, validUID, got.UID)
		assert.Equal(t, uint64(9), rev)
		mockKV.AssertExpectations(t)
	})

	t.Run("key-value store error returns internal error", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(nil, errors.New("nats error"))
		repo := &NatsRepository{Documents: mockKV}

		got, rev, err := repo.GetDocumentMetadata(context.Background(), "proj-1", validUID)

		assert.Equal(t, domain.ErrInternal, err)
		assert.Nil(t, got)
		assert.Zero(t, rev)
	})

	t.Run("invalid stored json returns unmarshal error", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(NewMockKeyValueEntry([]byte("{not json"), 1), nil)
		repo := &NatsRepository{Documents: mockKV}

		got, rev, err := repo.GetDocumentMetadata(context.Background(), "proj-1", validUID)

		assert.Equal(t, domain.ErrUnmarshal, err)
		assert.Nil(t, got)
		assert.Zero(t, rev)
	})

	t.Run("record owned by another project returns not found", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(NewMockKeyValueEntry(mustMarshal(t, &models.ProjectDocument{UID: validUID, ProjectUID: "other-proj"}), 3), nil)
		repo := &NatsRepository{Documents: mockKV}

		got, rev, err := repo.GetDocumentMetadata(context.Background(), "proj-1", validUID)

		assert.Equal(t, domain.ErrDocumentNotFound, err)
		assert.Nil(t, got)
		assert.Zero(t, rev)
	})
}

// fakeObjectResult is a minimal jetstream.ObjectResult backed by a reader.
type fakeObjectResult struct {
	reader io.Reader
}

func (f *fakeObjectResult) Read(p []byte) (int, error) { return f.reader.Read(p) }

func (f *fakeObjectResult) Close() error { return nil }

func (f *fakeObjectResult) Info() (*jetstream.ObjectInfo, error) { return &jetstream.ObjectInfo{}, nil }

func (f *fakeObjectResult) Error() error { return nil }

// failingReader always fails to read.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestNatsRepository_ListDocuments(t *testing.T) {
	const (
		projectUID = "proj-1"
		docUID     = "00000000-0000-0000-0000-000000000001"
		otherUID   = "00000000-0000-0000-0000-000000000002"
	)

	tests := []struct {
		name       string
		setupMocks func(*MockKeyValue)
		wantUIDs   []string
		wantErr    error
	}{
		{
			name: "error listing keys",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(nil, errors.New("nats error"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name: "lookup keys are skipped and other projects' documents are filtered out",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{"lookup/project-documents/hash", docUID, otherUID}), nil)
				kv.On("Get", mock.Anything, docUID).Return(NewMockKeyValueEntry(mustMarshal(t, &models.ProjectDocument{UID: docUID, ProjectUID: projectUID}), 1), nil)
				kv.On("Get", mock.Anything, otherUID).Return(NewMockKeyValueEntry(mustMarshal(t, &models.ProjectDocument{UID: otherUID, ProjectUID: "other-proj"}), 1), nil)
			},
			wantUIDs: []string{docUID},
		},
		{
			name: "error getting a document",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{docUID}), nil)
				kv.On("Get", mock.Anything, docUID).Return(nil, errors.New("nats error"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name: "document deleted between list and get is skipped",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{docUID}), nil)
				kv.On("Get", mock.Anything, docUID).Return(nil, jetstream.ErrKeyNotFound)
			},
			wantUIDs: []string{},
		},
		{
			name: "invalid document json",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{docUID}), nil)
				kv.On("Get", mock.Anything, docUID).Return(NewMockKeyValueEntry([]byte("{not json"), 1), nil)
			},
			wantErr: domain.ErrUnmarshal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{}
			tt.setupMocks(mockKV)
			repo := &NatsRepository{Documents: mockKV}

			docs, err := repo.ListDocuments(context.Background(), projectUID)

			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				assert.Nil(t, docs)
			} else {
				require.NoError(t, err)
				got := []string{}
				for _, d := range docs {
					got = append(got, d.UID)
				}
				assert.Equal(t, tt.wantUIDs, got)
			}
			mockKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_CreateDocumentMetadata(t *testing.T) {
	doc := &models.ProjectDocument{UID: "00000000-0000-0000-0000-000000000001", ProjectUID: "proj-1", Name: "Charter"}

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
			mockKV.On("Put", mock.Anything, doc.UID, mock.Anything).Return(uint64(1), tt.putErr)
			repo := &NatsRepository{Documents: mockKV}

			err := repo.CreateDocumentMetadata(context.Background(), doc)

			assert.Equal(t, tt.wantErr, err)
			mockKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_DeleteDocumentMetadata(t *testing.T) {
	const (
		projectUID = "proj-1"
		docUID     = "00000000-0000-0000-0000-000000000001"
	)
	doc := &models.ProjectDocument{UID: docUID, ProjectUID: projectUID, Name: "Charter"}
	uniqueKey := fmt.Sprintf(constants.KVLookupDocumentPrefix, doc.BuildIndexKey(context.Background()))
	docEntry := func() *MockKeyValueEntry { return NewMockKeyValueEntry(mustMarshal(t, doc), 9) }

	tests := []struct {
		name       string
		setupMocks func(*MockKeyValue)
		wantErr    error
	}{
		{
			name: "document not found",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, docUID).Return(nil, jetstream.ErrKeyNotFound)
			},
			wantErr: domain.ErrDocumentNotFound,
		},
		{
			name: "revision mismatch",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, docUID).Return(docEntry(), nil)
				kv.On("Delete", mock.Anything, docUID, forwardedRevision(4)).Return(errors.New("wrong last sequence"))
			},
			wantErr: domain.ErrRevisionMismatch,
		},
		{
			name: "other delete error",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, docUID).Return(docEntry(), nil)
				kv.On("Delete", mock.Anything, docUID, forwardedRevision(4)).Return(errors.New("nats error"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name: "success purges the name reservation",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, docUID).Return(docEntry(), nil)
				kv.On("Delete", mock.Anything, docUID, forwardedRevision(4)).Return(nil)
				kv.On("Purge", mock.Anything, uniqueKey).Return(nil)
			},
		},
		{
			name: "name reservation purge failure only warns",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, docUID).Return(docEntry(), nil)
				kv.On("Delete", mock.Anything, docUID, forwardedRevision(4)).Return(nil)
				kv.On("Purge", mock.Anything, uniqueKey).Return(errors.New("purge failed"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{}
			tt.setupMocks(mockKV)
			repo := &NatsRepository{Documents: mockKV}

			err := repo.DeleteDocumentMetadata(context.Background(), projectUID, docUID, 4)

			assert.Equal(t, tt.wantErr, err)
			mockKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_UniqueDocumentName(t *testing.T) {
	doc := &models.ProjectDocument{UID: "00000000-0000-0000-0000-000000000001", ProjectUID: "proj-1", Name: "Charter"}
	wantKey := fmt.Sprintf(constants.KVLookupDocumentPrefix, doc.BuildIndexKey(context.Background()))

	tests := []struct {
		name      string
		createErr error
		wantKey   string
		wantErr   error
	}{
		{name: "reserves the name", wantKey: wantKey},
		{name: "name already taken", createErr: jetstream.ErrKeyExists, wantErr: domain.ErrDocumentNameExists},
		{name: "other store error", createErr: errors.New("nats error"), wantErr: domain.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{}
			mockKV.On("Create", mock.Anything, wantKey, []byte(doc.UID)).Return(uint64(1), tt.createErr)
			repo := &NatsRepository{Documents: mockKV}

			key, err := repo.UniqueDocumentName(context.Background(), doc)

			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantKey, key)
			mockKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_DeleteUniqueDocumentName(t *testing.T) {
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
			mockKV.On("Purge", mock.Anything, "lookup/project-documents/key").Return(tt.purgeErr)
			repo := &NatsRepository{Documents: mockKV}

			err := repo.DeleteUniqueDocumentName(context.Background(), "lookup/project-documents/key")

			assert.Equal(t, tt.wantErr, err)
			mockKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_GetDocumentFile(t *testing.T) {
	const docUID = "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name       string
		setupMocks func(*MockObjectStore)
		want       []byte
		wantErr    error
	}{
		{
			name: "returns the stored bytes",
			setupMocks: func(os *MockObjectStore) {
				os.On("Get", mock.Anything, docUID).Return(&fakeObjectResult{reader: strings.NewReader("file contents")}, nil)
			},
			want: []byte("file contents"),
		},
		{
			name: "object not found",
			setupMocks: func(os *MockObjectStore) {
				os.On("Get", mock.Anything, docUID).Return(nil, jetstream.ErrObjectNotFound)
			},
			wantErr: domain.ErrDocumentNotFound,
		},
		{
			name: "other store error",
			setupMocks: func(os *MockObjectStore) {
				os.On("Get", mock.Anything, docUID).Return(nil, errors.New("nats error"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name: "read error",
			setupMocks: func(os *MockObjectStore) {
				os.On("Get", mock.Anything, docUID).Return(&fakeObjectResult{reader: failingReader{}}, nil)
			},
			wantErr: domain.ErrInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockOS := &MockObjectStore{}
			tt.setupMocks(mockOS)
			repo := &NatsRepository{DocumentFiles: mockOS}

			got, err := repo.GetDocumentFile(context.Background(), docUID)

			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.want, got)
			mockOS.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_PutDocumentFile(t *testing.T) {
	const docUID = "00000000-0000-0000-0000-000000000001"

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
			mockOS := &MockObjectStore{}
			wantContents := mock.MatchedBy(func(r io.Reader) bool {
				data, err := io.ReadAll(r)
				return err == nil && string(data) == "file contents"
			})
			if tt.putErr != nil {
				mockOS.On("Put", mock.Anything, jetstream.ObjectMeta{Name: docUID}, wantContents).Return(nil, tt.putErr)
			} else {
				mockOS.On("Put", mock.Anything, jetstream.ObjectMeta{Name: docUID}, wantContents).Return(&jetstream.ObjectInfo{}, nil)
			}
			repo := &NatsRepository{DocumentFiles: mockOS}

			err := repo.PutDocumentFile(context.Background(), docUID, []byte("file contents"))

			assert.Equal(t, tt.wantErr, err)
			mockOS.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_DeleteDocumentFile(t *testing.T) {
	const docUID = "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name      string
		deleteErr error
		wantErr   error
	}{
		{name: "success"},
		{name: "delete fails", deleteErr: errors.New("nats error"), wantErr: domain.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockOS := &MockObjectStore{}
			mockOS.On("Delete", mock.Anything, docUID).Return(tt.deleteErr)
			repo := &NatsRepository{DocumentFiles: mockOS}

			err := repo.DeleteDocumentFile(context.Background(), docUID)

			assert.Equal(t, tt.wantErr, err)
			mockOS.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_ListAllDocuments(t *testing.T) {
	doc := func(uid, projectUID string) *models.ProjectDocument {
		return &models.ProjectDocument{UID: uid, ProjectUID: projectUID, Name: "Doc " + uid, FileName: uid + ".pdf"}
	}
	tests := []syncListCase{
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
			kv := newSyncListKV(tt)
			repo.Documents = kv

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

const (
	updateProjectUID = "proj-1"
	updateDocUID     = "00000000-0000-0000-0000-0000000000d1"
)

// documentKeyOf is the name lookup key a document with this name holds in the bucket.
func documentKeyOf(name string) string {
	doc := &models.ProjectDocument{ProjectUID: updateProjectUID, Name: name}
	return fmt.Sprintf(constants.KVLookupDocumentPrefix, doc.BuildIndexKey(context.Background()))
}

// seedDocument stores the document "Minutes" the way an upload does: its name reserved, then its record.
func seedDocument(t *testing.T, kv INatsKeyValue) (*NatsRepository, *models.ProjectDocument, uint64) {
	t.Helper()
	ctx := context.Background()
	repo := &NatsRepository{Documents: kv}
	doc := &models.ProjectDocument{
		UID: updateDocUID, ProjectUID: updateProjectUID, Name: "Minutes", Description: "Meeting minutes",
		FileName: "minutes.pdf", ContentType: "application/pdf", FileSize: 2048,
		CreatedBy: &models.UserInfo{Username: "creator"}, UpdatedBy: &models.UserInfo{Username: "creator"},
	}
	_, err := repo.UniqueDocumentName(ctx, doc)
	require.NoError(t, err)
	require.NoError(t, repo.CreateDocumentMetadata(ctx, doc))
	got, revision, err := repo.GetDocumentMetadata(ctx, updateProjectUID, updateDocUID)
	require.NoError(t, err)
	return repo, got, revision
}

func TestNatsRepository_UpdateDocumentMetadata(t *testing.T) {
	ctx := context.Background()
	nameHeld := func(kv *memKV, name string) bool {
		_, err := kv.Get(ctx, documentKeyOf(name))
		return err == nil
	}

	t.Run("an update that keeps the name replaces the record and touches no key", func(t *testing.T) {
		kv := newMemKV()
		repo, doc, revision := seedDocument(t, kv)

		doc.Description = "edited"
		require.NoError(t, repo.UpdateDocumentMetadata(ctx, doc, revision))

		got, newRevision, err := repo.GetDocumentMetadata(ctx, updateProjectUID, updateDocUID)
		require.NoError(t, err)
		assert.Equal(t, "edited", got.Description)
		assert.Greater(t, newRevision, revision)
		assert.True(t, nameHeld(kv, "Minutes"))
	})

	t.Run("a rename reserves the new name, writes the record and releases the old name", func(t *testing.T) {
		kv := newMemKV()
		repo, doc, revision := seedDocument(t, kv)

		doc.Name = "Minutes 2026"
		require.NoError(t, repo.UpdateDocumentMetadata(ctx, doc, revision))

		got, _, err := repo.GetDocumentMetadata(ctx, updateProjectUID, updateDocUID)
		require.NoError(t, err)
		assert.Equal(t, "Minutes 2026", got.Name)
		assert.True(t, nameHeld(kv, "Minutes 2026"))
		assert.False(t, nameHeld(kv, "Minutes"), "the old name is free again")
		entry, err := kv.Get(ctx, documentKeyOf("Minutes 2026"))
		require.NoError(t, err)
		assert.True(t, lookupHeldBy(string(entry.Value()), updateDocUID), "the reservation names the document that holds it")
	})

	t.Run("a name another document holds is a conflict that changes nothing", func(t *testing.T) {
		kv := newMemKV()
		repo, doc, revision := seedDocument(t, kv)
		_, err := repo.UniqueDocumentName(ctx, &models.ProjectDocument{UID: "00000000-0000-0000-0000-0000000000d2", ProjectUID: updateProjectUID, Name: "Charter"})
		require.NoError(t, err)

		doc.Name = "Charter"
		err = repo.UpdateDocumentMetadata(ctx, doc, revision)

		assert.Equal(t, domain.ErrDocumentNameExists, err)
		got, gotRevision, getErr := repo.GetDocumentMetadata(ctx, updateProjectUID, updateDocUID)
		require.NoError(t, getErr)
		assert.Equal(t, "Minutes", got.Name)
		assert.Equal(t, revision, gotRevision)
		assert.True(t, nameHeld(kv, "Minutes"))
	})

	t.Run("a stale revision is a mismatch before anything is reserved", func(t *testing.T) {
		kv := newMemKV()
		repo, doc, revision := seedDocument(t, kv)

		doc.Name = "Minutes 2026"
		err := repo.UpdateDocumentMetadata(ctx, doc, revision-1)

		assert.Equal(t, domain.ErrRevisionMismatch, err)
		assert.False(t, nameHeld(kv, "Minutes 2026"), "nothing is reserved for a stale revision")
	})

	t.Run("a document that is not there is not found", func(t *testing.T) {
		repo := &NatsRepository{Documents: newMemKV()}

		err := repo.UpdateDocumentMetadata(ctx, &models.ProjectDocument{UID: updateDocUID, ProjectUID: updateProjectUID, Name: "Minutes"}, 1)

		assert.Equal(t, domain.ErrDocumentNotFound, err)
	})

	t.Run("a store error on the update is an internal error", func(t *testing.T) {
		kv := newMemKV()
		_, doc, revision := seedDocument(t, kv)
		failing := &renameWriteKV{INatsKeyValue: kv, recordKey: updateDocUID, err: errors.New("boom")}
		repo := &NatsRepository{Documents: failing}

		doc.Description = "edited"
		assert.Equal(t, domain.ErrInternal, repo.UpdateDocumentMetadata(ctx, doc, revision))
	})
}

// renameWriteKV makes the update of one record, the document's own, come back with an error, either after the update
// ran (a write whose reply was lost) or without running it, the two outcomes a caller cannot tell apart. The lookup
// keys go straight to the store. Once the update has been attempted, reading the record back can be made to fail too.
type renameWriteKV struct {
	INatsKeyValue
	recordKey string
	err       error
	applied   bool
	getErr    error
	attempted bool
	// beforeReadBack runs once, when the record is read back after the update was attempted: what another request does
	// between the failed write and the recovery.
	beforeReadBack func()
}

func (k *renameWriteKV) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	if key != k.recordKey {
		return k.INatsKeyValue.Update(ctx, key, value, revision)
	}
	k.attempted = true
	if k.applied {
		if _, err := k.INatsKeyValue.Update(ctx, key, value, revision); err != nil {
			return 0, err
		}
	}
	return 0, k.err
}

func (k *renameWriteKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if key == k.recordKey && k.attempted && k.beforeReadBack != nil {
		hook := k.beforeReadBack
		k.beforeReadBack = nil
		hook()
	}
	if key == k.recordKey && k.attempted && k.getErr != nil {
		return nil, k.getErr
	}
	return k.INatsKeyValue.Get(ctx, key)
}

// getHookKV runs a hook once, just before one key is read: what another request does while this one is paused there.
type getHookKV struct {
	INatsKeyValue
	key  string
	hook func()
}

func (k *getHookKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if key == k.key && k.hook != nil {
		hook := k.hook
		k.hook = nil
		hook()
	}
	return k.INatsKeyValue.Get(ctx, key)
}

// An update whose write comes back with an error that is not a refusal (a client that disconnects mid-write is the usual
// cause) must not leave a name locked or a landed write unreported: the record is read back, and the keys follow what it
// holds.
func TestNatsRepository_UpdateDocumentMetadata_WriteError(t *testing.T) {
	ctx := context.Background()

	// setup creates the document "Minutes" and wraps the bucket so that the document's next update fails as the case says.
	setup := func(t *testing.T, wrap func(*renameWriteKV)) (*NatsRepository, *memKV, *models.ProjectDocument, uint64, *renameWriteKV) {
		t.Helper()
		kv := newMemKV()
		_, doc, revision := seedDocument(t, kv)
		failing := &renameWriteKV{INatsKeyValue: kv, recordKey: updateDocUID}
		wrap(failing)
		return &NatsRepository{Documents: failing}, kv, doc, revision, failing
	}
	nameHeld := func(kv *memKV, name string) bool {
		_, err := kv.Get(ctx, documentKeyOf(name))
		return err == nil
	}
	stored := func(t *testing.T, kv *memKV) *models.ProjectDocument {
		t.Helper()
		got, _, err := (&NatsRepository{Documents: kv}).GetDocumentMetadata(ctx, updateProjectUID, updateDocUID)
		require.NoError(t, err)
		return got
	}

	t.Run("a write that landed but was reported as failed is a success that releases the old name and keeps the new one", func(t *testing.T) {
		repo, kv, doc, revision, _ := setup(t, func(k *renameWriteKV) { k.applied, k.err = true, context.DeadlineExceeded })

		doc.Name = "Minutes 2026"
		require.NoError(t, repo.UpdateDocumentMetadata(ctx, doc, revision), "a write found to have landed is reported as done, so the update completes and is indexed")

		assert.Equal(t, "Minutes 2026", stored(t, kv).Name, "the write did land")
		assert.True(t, nameHeld(kv, "Minutes 2026"), "the record holds the new name")
		assert.False(t, nameHeld(kv, "Minutes"), "the old name must not stay locked by a record that left it")
	})

	t.Run("a write that may still land keeps the name reserved for it", func(t *testing.T) {
		repo, kv, doc, revision, _ := setup(t, func(k *renameWriteKV) { k.err = context.DeadlineExceeded })

		doc.Name = "Minutes 2026"
		require.Error(t, repo.UpdateDocumentMetadata(ctx, doc, revision))

		assert.Equal(t, "Minutes", stored(t, kv).Name, "the write did not land")
		assert.True(t, nameHeld(kv, "Minutes"), "the record keeps its name")
		// The record is still at the revision the write was sent at, so the write could land later; releasing the
		// name now would let another document take it first.
		assert.True(t, nameHeld(kv, "Minutes 2026"), "the name stays reserved while the write may still land")
	})

	t.Run("a write that can no longer land because the record moved on releases the reserved name", func(t *testing.T) {
		repo, kv, doc, revision, failing := setup(t, func(k *renameWriteKV) { k.err = context.Canceled })
		other := &NatsRepository{Documents: kv}
		failing.beforeReadBack = func() {
			// Another request changes only the description: the record is at a later revision, so the earlier write,
			// which needs the revision it was sent at, can never apply.
			latest, rev, err := other.GetDocumentMetadata(ctx, updateProjectUID, updateDocUID)
			require.NoError(t, err)
			latest.Description = "edited meanwhile"
			require.NoError(t, other.UpdateDocumentMetadata(ctx, latest, rev))
		}

		doc.Name = "Minutes 2026"
		require.Error(t, repo.UpdateDocumentMetadata(ctx, doc, revision))

		assert.True(t, nameHeld(kv, "Minutes"))
		assert.False(t, nameHeld(kv, "Minutes 2026"), "the write cannot land any more, so its name is free")
	})

	t.Run("overlapping renames leave no name locked: A to B lands, B to C lands, then A to B is settled", func(t *testing.T) {
		repo, kv, doc, revision, failing := setup(t, func(k *renameWriteKV) { k.applied, k.err = true, context.DeadlineExceeded })
		other := &NatsRepository{Documents: kv}
		failing.beforeReadBack = func() {
			latest, rev, err := other.GetDocumentMetadata(ctx, updateProjectUID, updateDocUID)
			require.NoError(t, err)
			latest.Name = "Minutes Final"
			require.NoError(t, other.UpdateDocumentMetadata(ctx, latest, rev))
		}

		doc.Name = "Minutes 2026"
		require.Error(t, repo.UpdateDocumentMetadata(ctx, doc, revision), "the record was changed again, so this write is not reported as the outcome")

		assert.Equal(t, "Minutes Final", stored(t, kv).Name)
		assert.True(t, nameHeld(kv, "Minutes Final"))
		assert.False(t, nameHeld(kv, "Minutes 2026"), "the intermediate name is free")
		assert.False(t, nameHeld(kv, "Minutes"), "the original name must not stay locked by a record that left it")
	})

	t.Run("recovery does not free a name the record took back in the meantime", func(t *testing.T) {
		repo, kv, doc, revision, failing := setup(t, func(k *renameWriteKV) { k.applied, k.err = true, context.DeadlineExceeded })
		other := &NatsRepository{Documents: kv}
		failing.beforeReadBack = func() {
			// B to C, then C back to B: the key for B is released and reserved again by another request.
			for _, name := range []string{"Minutes Final", "Minutes 2026"} {
				latest, rev, err := other.GetDocumentMetadata(ctx, updateProjectUID, updateDocUID)
				require.NoError(t, err)
				latest.Name = name
				require.NoError(t, other.UpdateDocumentMetadata(ctx, latest, rev))
			}
		}

		doc.Name = "Minutes 2026"
		_ = repo.UpdateDocumentMetadata(ctx, doc, revision)

		assert.True(t, nameHeld(kv, "Minutes 2026"), "the record holds this name, so its key must survive the recovery")
		assert.False(t, nameHeld(kv, "Minutes Final"))
		assert.False(t, nameHeld(kv, "Minutes"))
		_, err := other.UniqueDocumentName(ctx, &models.ProjectDocument{UID: "00000000-0000-0000-0000-0000000000d2", ProjectUID: updateProjectUID, Name: "Minutes 2026"})
		assert.Equal(t, domain.ErrDocumentNameExists, err, "another document still cannot take the name")
	})

	t.Run("recovery does not release an old name that another document has taken since", func(t *testing.T) {
		repo, kv, doc, revision, failing := setup(t, func(k *renameWriteKV) { k.applied, k.err = true, context.DeadlineExceeded })
		failing.beforeReadBack = func() {
			// The old name's key is freed and another document takes it before the recovery gets to release it.
			require.NoError(t, kv.Purge(ctx, documentKeyOf("Minutes")))
			_, err := kv.Create(ctx, documentKeyOf("Minutes"), []byte("00000000-0000-0000-0000-0000000000d2"))
			require.NoError(t, err)
		}

		doc.Name = "Minutes 2026"
		require.NoError(t, repo.UpdateDocumentMetadata(ctx, doc, revision), "the write landed")

		entry, err := kv.Get(ctx, documentKeyOf("Minutes"))
		require.NoError(t, err, "the other document's key must survive the recovery")
		assert.Equal(t, "00000000-0000-0000-0000-0000000000d2", string(entry.Value()))
	})

	t.Run("a write the server refuses outright releases the reserved name and changes nothing", func(t *testing.T) {
		repo, kv, doc, revision, _ := setup(t, func(k *renameWriteKV) { k.err = fmt.Errorf("update: %w", jetstream.ErrKeyExists) })

		doc.Name = "Minutes 2026"
		err := repo.UpdateDocumentMetadata(ctx, doc, revision)

		assert.Equal(t, domain.ErrRevisionMismatch, err)
		assert.False(t, nameHeld(kv, "Minutes 2026"), "a refused rename must not leave its name locked")
		assert.True(t, nameHeld(kv, "Minutes"))
	})

	t.Run("when the record cannot be read back both names are kept rather than guessed at", func(t *testing.T) {
		repo, kv, doc, revision, _ := setup(t, func(k *renameWriteKV) {
			k.err, k.getErr = context.DeadlineExceeded, errors.New("kv unavailable")
		})

		doc.Name = "Minutes 2026"
		require.Error(t, repo.UpdateDocumentMetadata(ctx, doc, revision))

		// Either name may be the record's, so neither key can be freed without risking a name that is in use.
		assert.True(t, nameHeld(kv, "Minutes"))
		assert.True(t, nameHeld(kv, "Minutes 2026"))
	})

	t.Run("an update that keeps the name and landed despite the error is a success", func(t *testing.T) {
		repo, kv, doc, revision, _ := setup(t, func(k *renameWriteKV) { k.applied, k.err = true, context.DeadlineExceeded })

		doc.Description = "edited"
		require.NoError(t, repo.UpdateDocumentMetadata(ctx, doc, revision), "a landed write is reported as done, so the update is indexed")

		assert.Equal(t, "edited", stored(t, kv).Description)
		assert.True(t, nameHeld(kv, "Minutes"), "no name key moves when the name is kept")
	})

	t.Run("an update that keeps the name and did not land is an error", func(t *testing.T) {
		repo, kv, doc, revision, _ := setup(t, func(k *renameWriteKV) { k.err = context.DeadlineExceeded })

		doc.Description = "edited"
		require.Error(t, repo.UpdateDocumentMetadata(ctx, doc, revision))

		assert.Equal(t, "Meeting minutes", stored(t, kv).Description)
	})

	t.Run("an update that keeps the name is not reported as done when the record was changed afterwards", func(t *testing.T) {
		repo, kv, doc, revision, failing := setup(t, func(k *renameWriteKV) { k.applied, k.err = true, context.DeadlineExceeded })
		other := &NatsRepository{Documents: kv}
		failing.beforeReadBack = func() {
			latest, rev, err := other.GetDocumentMetadata(ctx, updateProjectUID, updateDocUID)
			require.NoError(t, err)
			latest.Description = "someone else's edit"
			require.NoError(t, other.UpdateDocumentMetadata(ctx, latest, rev))
		}

		doc.Description = "edited"
		require.Error(t, repo.UpdateDocumentMetadata(ctx, doc, revision), "the record is no longer what this write stored")
	})

	t.Run("a rename is not reported as done when only the audit users changed afterwards", func(t *testing.T) {
		repo, kv, doc, revision, failing := setup(t, func(k *renameWriteKV) { k.applied, k.err = true, context.DeadlineExceeded })
		other := &NatsRepository{Documents: kv}
		failing.beforeReadBack = func() {
			// The audit backfill rewrites the users and leaves updated_at as it was.
			latest, rev, err := other.GetDocumentMetadata(ctx, updateProjectUID, updateDocUID)
			require.NoError(t, err)
			latest.UpdatedBy = &models.UserInfo{Username: "backfilled"}
			require.NoError(t, other.UpdateDocumentMetadata(ctx, latest, rev))
		}

		doc.Name = "Minutes 2026"
		require.Error(t, repo.UpdateDocumentMetadata(ctx, doc, revision), "reporting this write as the outcome would index the older audit users")

		assert.True(t, nameHeld(kv, "Minutes 2026"), "the record holds the new name")
		assert.False(t, nameHeld(kv, "Minutes"), "the old name is released, as the write did land")
	})

	for _, applied := range []bool{true, false} {
		t.Run(fmt.Sprintf("a document deleted before the read-back leaves no name locked (write applied: %t)", applied), func(t *testing.T) {
			repo, kv, doc, revision, failing := setup(t, func(k *renameWriteKV) { k.applied, k.err = applied, context.DeadlineExceeded })
			other := &NatsRepository{Documents: kv}
			failing.beforeReadBack = func() {
				_, rev, err := other.GetDocumentMetadata(ctx, updateProjectUID, updateDocUID)
				require.NoError(t, err)
				require.NoError(t, other.DeleteDocumentMetadata(ctx, updateProjectUID, updateDocUID, rev))
			}

			doc.Name = "Minutes 2026"
			require.Error(t, repo.UpdateDocumentMetadata(ctx, doc, revision))

			// The delete released only the name the record held when it read it; the other key is settled here.
			assert.False(t, nameHeld(kv, "Minutes"))
			assert.False(t, nameHeld(kv, "Minutes 2026"))
		})
	}

	t.Run("a rename does not release a name another rename of the same document is reserving", func(t *testing.T) {
		kv := newMemKV()
		_, doc, revision := seedDocument(t, kv)
		failing := &renameWriteKV{INatsKeyValue: kv, recordKey: updateDocUID, err: context.Canceled}
		other := &NatsRepository{Documents: kv}
		oldKey := documentKeyOf("Minutes")
		hooked := &getHookKV{INatsKeyValue: failing, key: oldKey, hook: func() {
			// While this request is paused before reading the old name's key, the document is renamed away, and a
			// rename back to the old name has reserved it without having written the record yet.
			latest, rev, err := other.GetDocumentMetadata(ctx, updateProjectUID, updateDocUID)
			require.NoError(t, err)
			latest.Name = "Minutes Final"
			require.NoError(t, other.UpdateDocumentMetadata(ctx, latest, rev))
			_, err = kv.Create(ctx, oldKey, []byte(updateDocUID+"#another-attempt"))
			require.NoError(t, err)
		}}
		repo := &NatsRepository{Documents: hooked}

		doc.Name = "Minutes 2026"
		err := repo.UpdateDocumentMetadata(ctx, doc, revision)

		assert.Equal(t, domain.ErrRevisionMismatch, err, "the record changed after it was read, so nothing is reserved and the caller retries")
		assert.True(t, nameHeld(kv, "Minutes"), "the other rename's reservation must survive")
		assert.False(t, nameHeld(kv, "Minutes 2026"), "nothing was reserved for the stale rename")
	})
}

// lostReservationKV stores the reservation for one lookup key and then reports an error, as a Create whose reply was
// lost does. value, when set, is stored instead of what was asked for: another attempt's reservation.
type lostReservationKV struct {
	INatsKeyValue
	lostKey string
	err     error
	value   string
}

func (k *lostReservationKV) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	if key != k.lostKey {
		return k.INatsKeyValue.Create(ctx, key, value, opts...)
	}
	if k.value != "" {
		value = []byte(k.value)
	}
	if _, err := k.INatsKeyValue.Create(ctx, key, value, opts...); err != nil {
		return 0, err
	}
	return 0, k.err
}

func TestNatsRepository_UpdateDocumentMetadata_LostReservationReply(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) (*NatsRepository, *memKV, *models.ProjectDocument, uint64, *lostReservationKV) {
		t.Helper()
		kv := newMemKV()
		_, doc, revision := seedDocument(t, kv)
		lost := &lostReservationKV{INatsKeyValue: kv, lostKey: documentKeyOf("Minutes 2026"), err: context.DeadlineExceeded}
		return &NatsRepository{Documents: lost}, kv, doc, revision, lost
	}

	t.Run("the key this attempt stored is released, so the same rename can be retried", func(t *testing.T) {
		repo, kv, doc, revision, _ := setup(t)

		doc.Name = "Minutes 2026"
		require.Error(t, repo.UpdateDocumentMetadata(ctx, doc, revision))

		_, err := kv.Get(ctx, documentKeyOf("Minutes 2026"))
		assert.ErrorIs(t, err, jetstream.ErrKeyNotFound, "the reservation this call stored is released")
		retry := &NatsRepository{Documents: kv}
		require.NoError(t, retry.UpdateDocumentMetadata(ctx, doc, revision), "the retry is not blocked by the lost reply")
	})

	t.Run("a key that holds the document's uid but not this attempt's token is left alone", func(t *testing.T) {
		repo, kv, doc, revision, lost := setup(t)
		lost.value = updateDocUID + "#another-attempt"

		doc.Name = "Minutes 2026"
		require.Error(t, repo.UpdateDocumentMetadata(ctx, doc, revision))

		_, err := kv.Get(ctx, documentKeyOf("Minutes 2026"))
		assert.NoError(t, err, "another attempt's reservation is not this one's to release")
	})
}

// The reservation a rename makes carries a token, which must not stop the document's delete from releasing the name.
func TestNatsRepository_DeleteDocumentMetadata_ReleasesRenamedName(t *testing.T) {
	ctx := context.Background()
	kv := newMemKV()
	repo, doc, revision := seedDocument(t, kv)
	doc.Name = "Minutes 2026"
	require.NoError(t, repo.UpdateDocumentMetadata(ctx, doc, revision))
	_, revision, err := repo.GetDocumentMetadata(ctx, updateProjectUID, updateDocUID)
	require.NoError(t, err)

	require.NoError(t, repo.DeleteDocumentMetadata(ctx, updateProjectUID, updateDocUID, revision))

	_, err = kv.Get(ctx, documentKeyOf("Minutes 2026"))
	assert.ErrorIs(t, err, jetstream.ErrKeyNotFound, "a deleted document must not leave its renamed name locked")
}
