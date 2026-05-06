// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

func TestProjectsService_CreateFolder(t *testing.T) {
	tests := []struct {
		name       string
		projectUID string
		folderName string
		setupMocks func(*domain.MockProjectRepository, *domain.MockFolderRepository, *domain.MockMessageBuilder)
		wantErr    error
	}{
		{
			name:       "success",
			projectUID: "proj-1",
			folderName: "Governance",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("UniqueFolderName", mock.Anything, mock.AnythingOfType("*models.ProjectFolder")).Return("lookup/project-folders/abc", nil)
				mockFolder.On("CreateFolder", mock.Anything, mock.AnythingOfType("*models.ProjectFolder")).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("bool")).Return(nil).Maybe()
			},
		},
		{
			name:       "duplicate folder name",
			projectUID: "proj-1",
			folderName: "Governance",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("UniqueFolderName", mock.Anything, mock.AnythingOfType("*models.ProjectFolder")).Return("", domain.ErrFolderNameExists)
			},
			wantErr: domain.ErrFolderNameExists,
		},
		{
			name:       "project not found",
			projectUID: "missing",
			folderName: "Governance",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "missing").Return(false, nil)
			},
			wantErr: domain.ErrProjectNotFound,
		},
		{
			name:       "create folder fails with rollback",
			projectUID: "proj-1",
			folderName: "Governance",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
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
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, mockRepo, mockMsg, _ := setupServiceForTesting()
			mockFolder := svc.FolderRepository.(*domain.MockFolderRepository)
			tt.setupMocks(mockRepo, mockFolder, mockMsg)

			result, err := svc.CreateFolder(context.Background(), tt.projectUID, tt.folderName, false)

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
		setupMocks func(*domain.MockFolderRepository)
		wantErr    error
	}{
		{
			name:       "success",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			setupMocks: func(mockFolder *domain.MockFolderRepository) {
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(
					&models.ProjectFolder{UID: "folder-1", ProjectUID: "proj-1", Name: "Governance", CreatedAt: now, UpdatedAt: now},
					uint64(2), nil,
				)
			},
		},
		{
			name:       "not found",
			projectUID: "proj-1",
			folderUID:  "missing",
			setupMocks: func(mockFolder *domain.MockFolderRepository) {
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "missing").Return(nil, uint64(0), domain.ErrFolderNotFound)
			},
			wantErr: domain.ErrFolderNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _, _ := setupServiceForTesting()
			mockFolder := svc.FolderRepository.(*domain.MockFolderRepository)
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
			}

			mockFolder.AssertExpectations(t)
		})
	}
}

func TestProjectsService_ListFolders(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		projectUID     string
		setupMocks     func(*domain.MockProjectRepository, *domain.MockFolderRepository)
		expectedLength int
		wantErr        error
	}{
		{
			name:       "success",
			projectUID: "proj-1",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockFolder *domain.MockFolderRepository) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("ListFolders", mock.Anything, "proj-1").Return([]*models.ProjectFolder{
					{UID: "folder-1", ProjectUID: "proj-1", Name: "Governance", CreatedAt: now, UpdatedAt: now},
				}, nil)
			},
			expectedLength: 1,
		},
		{
			name:       "project not found",
			projectUID: "missing",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockFolder *domain.MockFolderRepository) {
				mockRepo.On("ProjectExists", mock.Anything, "missing").Return(false, nil)
			},
			wantErr: domain.ErrProjectNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, mockRepo, _, _ := setupServiceForTesting()
			mockFolder := svc.FolderRepository.(*domain.MockFolderRepository)
			tt.setupMocks(mockRepo, mockFolder)

			folders, err := svc.ListFolders(context.Background(), tt.projectUID)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, folders)
			} else {
				assert.NoError(t, err)
				assert.Len(t, folders, tt.expectedLength)
			}

			mockRepo.AssertExpectations(t)
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
		setupMocks func(*domain.MockFolderRepository, *domain.MockLinkRepository, *domain.MockDocumentRepository, *domain.MockMessageBuilder)
		wantErr    error
	}{
		{
			name:       "success with no links or documents",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    misc.StringPtr("2"),
			setupMocks: func(mockFolder *domain.MockFolderRepository, mockLink *domain.MockLinkRepository, mockDoc *domain.MockDocumentRepository, mockMsg *domain.MockMessageBuilder) {
				mockLink.On("ListLinks", mock.Anything, "proj-1").Return([]*models.ProjectLink{}, nil)
				mockDoc.On("ListDocuments", mock.Anything, "proj-1").Return([]*models.ProjectDocument{}, nil)
				mockFolder.On("DeleteFolder", mock.Anything, "proj-1", "folder-1", uint64(2)).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("bool")).Return(nil).Maybe()
			},
		},
		{
			name:       "folder not empty due to link",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    misc.StringPtr("2"),
			setupMocks: func(mockFolder *domain.MockFolderRepository, mockLink *domain.MockLinkRepository, mockDoc *domain.MockDocumentRepository, mockMsg *domain.MockMessageBuilder) {
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
			setupMocks: func(mockFolder *domain.MockFolderRepository, mockLink *domain.MockLinkRepository, mockDoc *domain.MockDocumentRepository, mockMsg *domain.MockMessageBuilder) {
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
			setupMocks: func(mockFolder *domain.MockFolderRepository, mockLink *domain.MockLinkRepository, mockDoc *domain.MockDocumentRepository, mockMsg *domain.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:       "revision conflict",
			projectUID: "proj-1",
			folderUID:  "folder-1",
			ifMatch:    misc.StringPtr("1"),
			setupMocks: func(mockFolder *domain.MockFolderRepository, mockLink *domain.MockLinkRepository, mockDoc *domain.MockDocumentRepository, mockMsg *domain.MockMessageBuilder) {
				mockLink.On("ListLinks", mock.Anything, "proj-1").Return([]*models.ProjectLink{}, nil)
				mockDoc.On("ListDocuments", mock.Anything, "proj-1").Return([]*models.ProjectDocument{}, nil)
				mockFolder.On("DeleteFolder", mock.Anything, "proj-1", "folder-1", uint64(1)).Return(domain.ErrRevisionMismatch)
			},
			wantErr: domain.ErrRevisionMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, mockMsg, _ := setupServiceForTesting()
			mockFolder := svc.FolderRepository.(*domain.MockFolderRepository)
			mockLink := svc.LinkRepository.(*domain.MockLinkRepository)
			mockDoc := svc.DocumentRepository.(*domain.MockDocumentRepository)
			tt.setupMocks(mockFolder, mockLink, mockDoc, mockMsg)

			err := svc.DeleteFolder(context.Background(), tt.projectUID, tt.folderUID, tt.ifMatch, false)

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
