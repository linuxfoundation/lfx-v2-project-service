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

func TestNatsRepository_GetLink(t *testing.T) {
	const validUID = "00000000-0000-0000-0000-000000000002"

	for _, tt := range malformedResourceUIDs {
		t.Run("malformed uid returns not found without KV read: "+tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{}
			repo := &NatsRepository{Links: mockKV}

			link, rev, err := repo.GetLink(context.Background(), "proj-1", tt.uid)

			assert.Equal(t, domain.ErrLinkNotFound, err)
			assert.Nil(t, link)
			assert.Zero(t, rev)
			mockKV.AssertNotCalled(t, "Get", mock.Anything, mock.Anything)
		})
	}

	t.Run("missing well-formed uid returns not found", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(nil, jetstream.ErrKeyNotFound)
		repo := &NatsRepository{Links: mockKV}

		_, _, err := repo.GetLink(context.Background(), "proj-1", validUID)

		assert.Equal(t, domain.ErrLinkNotFound, err)
		mockKV.AssertExpectations(t)
	})

	t.Run("success returns the record with its revision", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(NewMockKeyValueEntry(mustMarshal(t, &models.ProjectLink{UID: validUID, ProjectUID: "proj-1"}), 9), nil)
		repo := &NatsRepository{Links: mockKV}

		got, rev, err := repo.GetLink(context.Background(), "proj-1", validUID)

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, validUID, got.UID)
		assert.Equal(t, uint64(9), rev)
		mockKV.AssertExpectations(t)
	})

	t.Run("key-value store error returns internal error", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(nil, errors.New("nats error"))
		repo := &NatsRepository{Links: mockKV}

		got, rev, err := repo.GetLink(context.Background(), "proj-1", validUID)

		assert.Equal(t, domain.ErrInternal, err)
		assert.Nil(t, got)
		assert.Zero(t, rev)
	})

	t.Run("invalid stored json returns unmarshal error", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(NewMockKeyValueEntry([]byte("{not json"), 1), nil)
		repo := &NatsRepository{Links: mockKV}

		got, rev, err := repo.GetLink(context.Background(), "proj-1", validUID)

		assert.Equal(t, domain.ErrUnmarshal, err)
		assert.Nil(t, got)
		assert.Zero(t, rev)
	})

	t.Run("record owned by another project returns not found", func(t *testing.T) {
		mockKV := &MockKeyValue{}
		mockKV.On("Get", mock.Anything, validUID).Return(NewMockKeyValueEntry(mustMarshal(t, &models.ProjectLink{UID: validUID, ProjectUID: "other-proj"}), 3), nil)
		repo := &NatsRepository{Links: mockKV}

		got, rev, err := repo.GetLink(context.Background(), "proj-1", validUID)

		assert.Equal(t, domain.ErrLinkNotFound, err)
		assert.Nil(t, got)
		assert.Zero(t, rev)
	})
}

func TestNatsRepository_ListLinks(t *testing.T) {
	const (
		projectUID = "proj-1"
		linkUID    = "00000000-0000-0000-0000-000000000001"
		staleUID   = "00000000-0000-0000-0000-000000000002"
		otherUID   = "00000000-0000-0000-0000-000000000003"
	)
	prefix := "lookup/project-links/" + projectUID + "/"

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
			name: "no keys returns an empty list",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{}), nil)
			},
			wantUIDs: []string{},
		},
		{
			name: "only keys under the project prefix are read, stale index keys are skipped",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{
					prefix + linkUID,
					"lookup/project-links/other-proj/" + otherUID,
					linkUID, // the record key itself, not an index key
					prefix + staleUID,
				}), nil)
				kv.On("Get", mock.Anything, linkUID).Return(NewMockKeyValueEntry(mustMarshal(t, &models.ProjectLink{UID: linkUID, ProjectUID: projectUID}), 1), nil)
				kv.On("Get", mock.Anything, staleUID).Return(nil, jetstream.ErrKeyNotFound)
			},
			wantUIDs: []string{linkUID},
		},
		{
			name: "error getting a link record",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{prefix + linkUID}), nil)
				kv.On("Get", mock.Anything, linkUID).Return(nil, errors.New("nats error"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name: "invalid link json",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{prefix + linkUID}), nil)
				kv.On("Get", mock.Anything, linkUID).Return(NewMockKeyValueEntry([]byte("{not json"), 1), nil)
			},
			wantErr: domain.ErrUnmarshal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{}
			tt.setupMocks(mockKV)
			repo := &NatsRepository{Links: mockKV}

			links, err := repo.ListLinks(context.Background(), projectUID)

			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				assert.Nil(t, links)
			} else {
				require.NoError(t, err)
				got := []string{}
				for _, l := range links {
					got = append(got, l.UID)
				}
				assert.Equal(t, tt.wantUIDs, got)
			}
			mockKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_CreateLink(t *testing.T) {
	link := &models.ProjectLink{UID: "00000000-0000-0000-0000-000000000001", ProjectUID: "proj-1", Name: "Docs", URL: "https://example.com"}
	lookupKey := fmt.Sprintf(constants.KVLookupLinkKey, link.ProjectUID, link.UID)

	tests := []struct {
		name       string
		setupMocks func(*MockKeyValue)
		wantErr    error
	}{
		{
			name: "success writes the record and the index key",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Put", mock.Anything, link.UID, mock.Anything).Return(uint64(1), nil)
				kv.On("Put", mock.Anything, lookupKey, []byte(link.UID)).Return(uint64(2), nil)
			},
		},
		{
			name: "record put fails",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Put", mock.Anything, link.UID, mock.Anything).Return(uint64(0), errors.New("nats error"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name: "index put fails and the record is rolled back",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Put", mock.Anything, link.UID, mock.Anything).Return(uint64(1), nil)
				kv.On("Put", mock.Anything, lookupKey, mock.Anything).Return(uint64(0), errors.New("nats error"))
				kv.On("Purge", mock.Anything, link.UID).Return(nil)
			},
			wantErr: domain.ErrInternal,
		},
		{
			name: "index put fails and the rollback also fails",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Put", mock.Anything, link.UID, mock.Anything).Return(uint64(1), nil)
				kv.On("Put", mock.Anything, lookupKey, mock.Anything).Return(uint64(0), errors.New("nats error"))
				kv.On("Purge", mock.Anything, link.UID).Return(errors.New("purge failed"))
			},
			wantErr: domain.ErrInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{}
			tt.setupMocks(mockKV)
			repo := &NatsRepository{Links: mockKV}

			err := repo.CreateLink(context.Background(), link)

			assert.Equal(t, tt.wantErr, err)
			mockKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_DeleteLink(t *testing.T) {
	const (
		projectUID = "proj-1"
		linkUID    = "00000000-0000-0000-0000-000000000001"
	)
	lookupKey := fmt.Sprintf(constants.KVLookupLinkKey, projectUID, linkUID)
	linkEntry := func() *MockKeyValueEntry {
		return NewMockKeyValueEntry(mustMarshal(t, &models.ProjectLink{UID: linkUID, ProjectUID: projectUID}), 9)
	}

	tests := []struct {
		name       string
		setupMocks func(*MockKeyValue)
		wantErr    error
	}{
		{
			name: "link not found",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, linkUID).Return(nil, jetstream.ErrKeyNotFound)
			},
			wantErr: domain.ErrLinkNotFound,
		},
		{
			name: "revision mismatch",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, linkUID).Return(linkEntry(), nil)
				kv.On("Delete", mock.Anything, linkUID, forwardedRevision(4)).Return(errors.New("wrong last sequence"))
			},
			wantErr: domain.ErrRevisionMismatch,
		},
		{
			name: "other delete error",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, linkUID).Return(linkEntry(), nil)
				kv.On("Delete", mock.Anything, linkUID, forwardedRevision(4)).Return(errors.New("nats error"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name: "success purges the index key",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, linkUID).Return(linkEntry(), nil)
				kv.On("Delete", mock.Anything, linkUID, forwardedRevision(4)).Return(nil)
				kv.On("Purge", mock.Anything, lookupKey).Return(nil)
			},
		},
		{
			name: "index key purge failure only warns",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, linkUID).Return(linkEntry(), nil)
				kv.On("Delete", mock.Anything, linkUID, forwardedRevision(4)).Return(nil)
				kv.On("Purge", mock.Anything, lookupKey).Return(errors.New("purge failed"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{}
			tt.setupMocks(mockKV)
			repo := &NatsRepository{Links: mockKV}

			err := repo.DeleteLink(context.Background(), projectUID, linkUID, 4)

			assert.Equal(t, tt.wantErr, err)
			mockKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_ListAllLinks(t *testing.T) {
	link := func(uid, projectUID string) *models.ProjectLink {
		return &models.ProjectLink{UID: uid, ProjectUID: projectUID, Name: "Link " + uid, URL: "https://example.com/" + uid}
	}
	tests := []syncListCase{
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
			kv := newSyncListKV(tt)
			repo.Links = kv

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
