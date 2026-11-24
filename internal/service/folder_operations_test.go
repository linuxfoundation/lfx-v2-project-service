// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	domainmocks "github.com/linuxfoundation/lfx-v2-project-service/internal/domain/mocks"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

func TestProjectsService_CreateFolder(t *testing.T) {
	tests := []struct {
		name       string
		projectUID string
		folderName string
		notReady   bool
		setupMocks func(*domainmocks.MockProjectRepository, *domainmocks.MockFolderRepository, *domainmocks.MockMessageBuilder)
		wantErr    error
	}{
		{
			name:       "service not ready",
			projectUID: "proj-1",
			folderName: "Governance",
			notReady:   true,
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrServiceUnavailable,
		},
		{
			name:       "project existence check fails",
			projectUID: "proj-1",
			folderName: "Governance",
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(false, errors.New("kv unavailable"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "create folder fails and rollback also fails",
			projectUID: "proj-1",
			folderName: "Governance",
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("UniqueFolderName", mock.Anything, mock.AnythingOfType("*models.ProjectFolder")).Return("lookup/project-folders/abc", nil)
				mockFolder.On("CreateFolder", mock.Anything, mock.AnythingOfType("*models.ProjectFolder")).Return(domain.ErrInternal)
				mockFolder.On("DeleteUniqueFolderName", mock.Anything, "lookup/project-folders/abc").Return(errors.New("rollback failed"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "success",
			projectUID: "proj-1",
			folderName: "Governance",
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("UniqueFolderName", mock.Anything, mock.AnythingOfType("*models.ProjectFolder")).Return("lookup/project-folders/abc", nil)
				mockFolder.On("CreateFolder", mock.Anything, mock.AnythingOfType("*models.ProjectFolder")).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
			},
		},
		{
			name:       "duplicate folder name",
			projectUID: "proj-1",
			folderName: "Governance",
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("UniqueFolderName", mock.Anything, mock.AnythingOfType("*models.ProjectFolder")).Return("", domain.ErrFolderNameExists)
			},
			wantErr: domain.ErrFolderNameExists,
		},
		{
			name:       "project not found",
			projectUID: "missing",
			folderName: "Governance",
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "missing").Return(false, nil)
			},
			wantErr: domain.ErrProjectNotFound,
		},
		{
			name:       "create folder fails with rollback",
			projectUID: "proj-1",
			folderName: "Governance",
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("UniqueFolderName", mock.Anything, mock.AnythingOfType("*models.ProjectFolder")).Return("lookup/project-folders/abc", nil)
				mockFolder.On("CreateFolder", mock.Anything, mock.AnythingOfType("*models.ProjectFolder")).Return(domain.ErrInternal)
				mockFolder.On("DeleteUniqueFolderName", mock.Anything, "lookup/project-folders/abc").Return(nil)
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "empty name rejected",
			projectUID: "proj-1",
			folderName: "",
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, mockRepo, mockMsg := setupServiceForTesting()
			mockFolder := svc.FolderRepository.(*domainmocks.MockFolderRepository)
			if tt.notReady {
				svc.Resolver = nil
			}
			tt.setupMocks(mockRepo, mockFolder, mockMsg)

			result, err := svc.CreateFolder(context.Background(), tt.projectUID, tt.folderName)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.projectUID, result.ProjectUID)
				assert.Equal(t, tt.folderName, result.Name)
			}

			mockRepo.AssertExpectations(t)
			mockFolder.AssertExpectations(t)
		})
	}
}

func TestProjectsService_GetFolder(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		projectUID string
		folderUID  string
		notReady   bool
		setupMocks func(*domainmocks.MockFolderRepository)
		wantErr    error
	}{
		{
			name:       "service not ready",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			notReady:   true,
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository) {},
			wantErr:    domain.ErrServiceUnavailable,
		},
		{
			name:       "unexpected repository error maps to internal",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository) {
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(nil, uint64(0), errors.New("kv unavailable"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "success",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository) {
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(
					&models.ProjectFolder{UID: "folder-1", ProjectUID: "proj-1", Name: "Governance", CreatedAt: now, UpdatedAt: now},
					uint64(2), nil,
				)
			},
		},
		{
			name:       "enriches legacy username on read",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository) {
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(
					&models.ProjectFolder{
						UID:        "folder-1",
						ProjectUID: "proj-1",
						Name:       "Governance",
						CreatedBy:  &models.UserInfo{Username: "alice"},
						CreatedAt:  now,
						UpdatedAt:  now,
					},
					uint64(2), nil,
				)
			},
		},
		{
			name:       "not found",
			projectUID: "proj-1",
			folderUID:  "missing",
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository) {
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "missing").Return(nil, uint64(0), domain.ErrFolderNotFound)
			},
			wantErr: domain.ErrFolderNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _ := setupServiceForTesting()
			mockFolder := svc.FolderRepository.(*domainmocks.MockFolderRepository)
			if tt.name == "enriches legacy username on read" {
				mockUser := svc.UserReader.(*domainmocks.MockUserReader)
				mockUser.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(&domain.UserMetadata{
					Name: "Alice Example",
				}, nil)
				t.Cleanup(func() { mockUser.AssertExpectations(t) })
			}
			if tt.notReady {
				svc.Resolver = nil
			}
			tt.setupMocks(mockFolder)

			folder, etag, err := svc.GetFolder(context.Background(), tt.projectUID, tt.folderUID)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, folder)
				assert.Empty(t, etag)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, folder)
				assert.Equal(t, "2", etag)
				if tt.name == "enriches legacy username on read" {
					assert.Equal(t, "Alice Example", folder.CreatedBy.Name)
					assert.NotNil(t, folder.UpdatedBy)
					assert.Equal(t, folder.CreatedBy.Name, folder.UpdatedBy.Name)
				}
			}

			mockFolder.AssertExpectations(t)
		})
	}
}

func TestProjectsService_DeleteFolder(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		projectUID string
		folderUID  string
		ifMatch    *string
		notReady   bool
		skipEtag   bool
		setupMocks func(*domainmocks.MockFolderRepository, *domainmocks.MockLinkRepository, *domainmocks.MockDocumentRepository, *domainmocks.MockMessageBuilder)
		wantErr    error
	}{
		{
			name:       "service not ready",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    misc.StringPtr("2"),
			notReady:   true,
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository, mockLink *domainmocks.MockLinkRepository, mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrServiceUnavailable,
		},
		{
			name:       "invalid If-Match value",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    misc.StringPtr("not-a-number"),
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository, mockLink *domainmocks.MockLinkRepository, mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:       "skip etag validation fetches the current revision",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			skipEtag:   true,
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository, mockLink *domainmocks.MockLinkRepository, mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(&models.ProjectFolder{UID: "folder-1", ProjectUID: "proj-1"}, uint64(9), nil)
				mockLink.On("ListLinks", mock.Anything, "proj-1").Return([]*models.ProjectLink{}, nil)
				mockDoc.On("ListDocuments", mock.Anything, "proj-1").Return([]*models.ProjectDocument{}, nil)
				mockFolder.On("DeleteFolder", mock.Anything, "proj-1", "folder-1", uint64(9)).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
			},
		},
		{
			name:       "skip etag validation returns not found from revision fetch",
			projectUID: "proj-1",
			folderUID:  "missing",
			skipEtag:   true,
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository, mockLink *domainmocks.MockLinkRepository, mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "missing").Return(nil, uint64(0), domain.ErrFolderNotFound)
			},
			wantErr: domain.ErrFolderNotFound,
		},
		{
			name:       "links and documents in other folders do not block deletion",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    misc.StringPtr("2"),
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository, mockLink *domainmocks.MockLinkRepository, mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockLink.On("ListLinks", mock.Anything, "proj-1").Return([]*models.ProjectLink{
					{UID: "link-1", ProjectUID: "proj-1", FolderUID: misc.StringPtr("folder-2"), CreatedAt: now, UpdatedAt: now},
					{UID: "link-2", ProjectUID: "proj-1", CreatedAt: now, UpdatedAt: now},
				}, nil)
				mockDoc.On("ListDocuments", mock.Anything, "proj-1").Return([]*models.ProjectDocument{
					{UID: "doc-1", ProjectUID: "proj-1", FolderUID: misc.StringPtr("folder-2"), CreatedAt: now, UpdatedAt: now},
					{UID: "doc-2", ProjectUID: "proj-1", CreatedAt: now, UpdatedAt: now},
				}, nil)
				mockFolder.On("DeleteFolder", mock.Anything, "proj-1", "folder-1", uint64(2)).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
			},
		},
		{
			name:       "listing links fails",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    misc.StringPtr("2"),
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository, mockLink *domainmocks.MockLinkRepository, mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockLink.On("ListLinks", mock.Anything, "proj-1").Return(nil, errors.New("kv unavailable"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "listing documents fails",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    misc.StringPtr("2"),
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository, mockLink *domainmocks.MockLinkRepository, mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockLink.On("ListLinks", mock.Anything, "proj-1").Return([]*models.ProjectLink{}, nil)
				mockDoc.On("ListDocuments", mock.Anything, "proj-1").Return(nil, errors.New("kv unavailable"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "repository delete failure is returned",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    misc.StringPtr("2"),
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository, mockLink *domainmocks.MockLinkRepository, mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockLink.On("ListLinks", mock.Anything, "proj-1").Return([]*models.ProjectLink{}, nil)
				mockDoc.On("ListDocuments", mock.Anything, "proj-1").Return([]*models.ProjectDocument{}, nil)
				mockFolder.On("DeleteFolder", mock.Anything, "proj-1", "folder-1", uint64(2)).Return(domain.ErrFolderNotFound)
			},
			wantErr: domain.ErrFolderNotFound,
		},
		{
			name:       "success with no links or documents",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    misc.StringPtr("2"),
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository, mockLink *domainmocks.MockLinkRepository, mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockLink.On("ListLinks", mock.Anything, "proj-1").Return([]*models.ProjectLink{}, nil)
				mockDoc.On("ListDocuments", mock.Anything, "proj-1").Return([]*models.ProjectDocument{}, nil)
				mockFolder.On("DeleteFolder", mock.Anything, "proj-1", "folder-1", uint64(2)).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
			},
		},
		{
			name:       "folder not empty due to link",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    misc.StringPtr("2"),
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository, mockLink *domainmocks.MockLinkRepository, mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockLink.On("ListLinks", mock.Anything, "proj-1").Return([]*models.ProjectLink{
					{UID: "link-1", ProjectUID: "proj-1", FolderUID: misc.StringPtr("folder-1"), Name: "L", URL: "https://example.com", CreatedAt: now, UpdatedAt: now},
				}, nil)
			},
			wantErr: domain.ErrFolderNotEmpty,
		},
		{
			name:       "folder not empty due to document",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    misc.StringPtr("2"),
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository, mockLink *domainmocks.MockLinkRepository, mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockLink.On("ListLinks", mock.Anything, "proj-1").Return([]*models.ProjectLink{}, nil)
				mockDoc.On("ListDocuments", mock.Anything, "proj-1").Return([]*models.ProjectDocument{
					{UID: "doc-1", ProjectUID: "proj-1", FolderUID: misc.StringPtr("folder-1"), Name: "D", CreatedAt: now, UpdatedAt: now},
				}, nil)
			},
			wantErr: domain.ErrFolderNotEmpty,
		},
		{
			name:       "missing If-Match header",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    nil,
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository, mockLink *domainmocks.MockLinkRepository, mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:       "revision conflict",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    misc.StringPtr("1"),
			setupMocks: func(mockFolder *domainmocks.MockFolderRepository, mockLink *domainmocks.MockLinkRepository, mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockLink.On("ListLinks", mock.Anything, "proj-1").Return([]*models.ProjectLink{}, nil)
				mockDoc.On("ListDocuments", mock.Anything, "proj-1").Return([]*models.ProjectDocument{}, nil)
				mockFolder.On("DeleteFolder", mock.Anything, "proj-1", "folder-1", uint64(1)).Return(domain.ErrRevisionMismatch)
			},
			wantErr: domain.ErrRevisionMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, mockMsg := setupServiceForTesting()
			mockFolder := svc.FolderRepository.(*domainmocks.MockFolderRepository)
			mockLink := svc.LinkRepository.(*domainmocks.MockLinkRepository)
			mockDoc := svc.DocumentRepository.(*domainmocks.MockDocumentRepository)
			if tt.notReady {
				svc.Resolver = nil
			}
			svc.Config.SkipEtagValidation = tt.skipEtag
			tt.setupMocks(mockFolder, mockLink, mockDoc, mockMsg)

			err := svc.DeleteFolder(context.Background(), tt.projectUID, tt.folderUID, tt.ifMatch)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			mockFolder.AssertExpectations(t)
			mockLink.AssertExpectations(t)
			mockDoc.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}
}

func TestProjectsService_UpdateFolder(t *testing.T) {
	const created = "2026-01-02T03:04:05Z"
	createdAt, _ := time.Parse(time.RFC3339, created)
	existing := func() *models.ProjectFolder {
		return &models.ProjectFolder{
			UID:        "folder-1",
			ProjectUID: "proj-1",
			Name:       "Old name",
			CreatedBy:  &models.UserInfo{Username: "creator"},
			UpdatedBy:  &models.UserInfo{Username: "creator"},
			CreatedAt:  createdAt,
			UpdatedAt:  createdAt,
		}
	}
	type mocks struct {
		folders *domainmocks.MockFolderRepository
		msg     *domainmocks.MockMessageBuilder
		indexed chan indexerTypes.IndexerMessageEnvelope
	}
	expectIndexed := func(m mocks) {
		m.msg.On("SendIndexerMessage", mock.Anything, constants.IndexProjectFolderSubject, mock.Anything).
			Run(func(a mock.Arguments) { m.indexed <- a.Get(2).(indexerTypes.IndexerMessageEnvelope) }).Return(nil).Once()
	}

	tests := []struct {
		name       string
		newName    string
		ifMatch    *string
		notReady   bool
		skipEtag   bool
		principal  string
		setupMocks func(mocks)
		wantErr    error
		wantEditor string
	}{
		{
			name:       "service not ready",
			newName:    "New",
			ifMatch:    misc.StringPtr("3"),
			notReady:   true,
			setupMocks: func(mocks) {},
			wantErr:    domain.ErrServiceUnavailable,
		},
		{
			name:       "empty name is rejected",
			newName:    "",
			ifMatch:    misc.StringPtr("3"),
			setupMocks: func(mocks) {},
			wantErr:    domain.ErrValidationFailed,
		},
		{
			name:    "folder not found",
			newName: "New",
			ifMatch: misc.StringPtr("3"),
			setupMocks: func(m mocks) {
				m.folders.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(nil, uint64(0), domain.ErrFolderNotFound)
			},
			wantErr: domain.ErrFolderNotFound,
		},
		{
			name:    "a failing read is an internal error",
			newName: "New",
			ifMatch: misc.StringPtr("3"),
			setupMocks: func(m mocks) {
				m.folders.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(nil, uint64(0), errors.New("kv down"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:    "missing If-Match header",
			newName: "New",
			setupMocks: func(m mocks) {
				m.folders.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(existing(), uint64(3), nil)
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:    "stale revision is a conflict and publishes nothing",
			newName: "New",
			ifMatch: misc.StringPtr("2"),
			setupMocks: func(m mocks) {
				m.folders.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(existing(), uint64(3), nil)
				m.folders.On("UpdateFolder", mock.Anything, mock.AnythingOfType("*models.ProjectFolder"), uint64(2)).Return(domain.ErrRevisionMismatch)
			},
			wantErr: domain.ErrRevisionMismatch,
		},
		{
			name:    "a name another folder holds is a conflict and publishes nothing",
			newName: "Taken",
			ifMatch: misc.StringPtr("3"),
			setupMocks: func(m mocks) {
				m.folders.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(existing(), uint64(3), nil)
				m.folders.On("UpdateFolder", mock.Anything, mock.AnythingOfType("*models.ProjectFolder"), uint64(3)).Return(domain.ErrFolderNameExists)
			},
			wantErr: domain.ErrFolderNameExists,
		},
		{
			name:    "a storage failure on the write is returned and publishes nothing",
			newName: "New",
			ifMatch: misc.StringPtr("3"),
			setupMocks: func(m mocks) {
				m.folders.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(existing(), uint64(3), nil)
				m.folders.On("UpdateFolder", mock.Anything, mock.AnythingOfType("*models.ProjectFolder"), uint64(3)).Return(domain.ErrInternal)
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:    "success renames the folder and keeps its creation fields",
			newName: "New name",
			ifMatch: misc.StringPtr("3"),
			setupMocks: func(m mocks) {
				m.folders.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(existing(), uint64(3), nil)
				m.folders.On("UpdateFolder", mock.Anything, mock.MatchedBy(func(got *models.ProjectFolder) bool {
					return got.UID == "folder-1" && got.ProjectUID == "proj-1" && got.Name == "New name" &&
						got.CreatedAt.Equal(createdAt) && got.UpdatedAt.After(createdAt)
				}), uint64(3)).Return(nil)
				expectIndexed(m)
			},
		},
		{
			name:      "success keeps the creator and stamps the editor",
			newName:   "New name",
			ifMatch:   misc.StringPtr("3"),
			principal: "editor",
			setupMocks: func(m mocks) {
				m.folders.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(existing(), uint64(3), nil)
				m.folders.On("UpdateFolder", mock.Anything, mock.MatchedBy(func(got *models.ProjectFolder) bool {
					return got.CreatedBy != nil && got.CreatedBy.Username == "creator" &&
						got.UpdatedBy != nil && got.UpdatedBy.Username == "editor"
				}), uint64(3)).Return(nil)
				expectIndexed(m)
			},
			wantEditor: "editor",
		},
		{
			name:     "skip etag validation writes at the fetched revision",
			newName:  "New name",
			skipEtag: true,
			setupMocks: func(m mocks) {
				m.folders.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(existing(), uint64(7), nil)
				m.folders.On("UpdateFolder", mock.Anything, mock.AnythingOfType("*models.ProjectFolder"), uint64(7)).Return(nil)
				expectIndexed(m)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, mockMsg := setupServiceForTesting()
			m := mocks{
				folders: svc.FolderRepository.(*domainmocks.MockFolderRepository),
				msg:     mockMsg,
				indexed: make(chan indexerTypes.IndexerMessageEnvelope, 1),
			}
			if tt.notReady {
				svc.Resolver = nil
			}
			svc.Config.SkipEtagValidation = tt.skipEtag
			tt.setupMocks(m)
			ctx := context.Background()
			if tt.principal != "" {
				ctx = context.WithValue(ctx, constants.PrincipalContextID, tt.principal)
				userReader := svc.UserReader.(*domainmocks.MockUserReader)
				userReader.On("UserMetadataByPrincipal", mock.Anything, tt.principal).Return((*domain.UserMetadata)(nil), nil)
				userReader.On("PrimaryEmailByUsername", mock.Anything, tt.principal).Return("", nil)
			}

			got, err := svc.UpdateFolder(ctx, "proj-1", "folder-1", tt.ifMatch, tt.newName)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
				assert.Empty(t, m.indexed, "a refused update publishes nothing")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.newName, got.Name)
				if tt.wantEditor != "" {
					if assert.NotNil(t, got.CreatedBy) {
						assert.Equal(t, "creator", got.CreatedBy.Username)
					}
					if assert.NotNil(t, got.UpdatedBy) {
						assert.Equal(t, tt.wantEditor, got.UpdatedBy.Username)
					}
				}
				select {
				case msg := <-m.indexed:
					assert.Equal(t, indexerConstants.ActionUpdated, msg.Action)
					folder, ok := msg.Data.(models.ProjectFolder)
					if assert.True(t, ok, "indexer data should be the updated folder") {
						assert.Equal(t, "folder-1", folder.UID)
						assert.Equal(t, tt.newName, folder.Name)
					}
					if assert.NotNil(t, msg.IndexingConfig) {
						assert.Equal(t, tt.newName, msg.IndexingConfig.SortName)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("expected an updated indexer message")
				}
			}

			m.folders.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}
}
