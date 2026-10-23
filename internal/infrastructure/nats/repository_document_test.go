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
