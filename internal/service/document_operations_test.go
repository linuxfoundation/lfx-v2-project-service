// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

func TestProjectsService_UploadDocument(t *testing.T) {
	validFile := []byte("hello pdf")

	tests := []struct {
		name        string
		projectUID  string
		docName     string
		contentType string
		fileData    []byte
		folderUID   *string
		setupMocks  func(*domain.MockProjectRepository, *domain.MockDocumentRepository, *domain.MockFolderRepository, *domain.MockMessageBuilder)
		wantErr     error
	}{
		{
			name:        "success without folder",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockDoc *domain.MockDocumentRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockDoc.On("UniqueDocumentName", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return("lookup/project-documents/abc", nil)
				mockDoc.On("PutDocumentFile", mock.Anything, mock.AnythingOfType("string"), validFile).Return(nil)
				mockDoc.On("CreateDocumentMetadata", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("bool")).Return(nil).Maybe()
			},
		},
		{
			name:        "success with folder",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			folderUID:   misc.StringPtr("folder-1"),
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockDoc *domain.MockDocumentRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
				now := time.Now()
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(&models.ProjectFolder{UID: "folder-1", ProjectUID: "proj-1", Name: "F", CreatedAt: now, UpdatedAt: now}, uint64(1), nil)
				mockDoc.On("UniqueDocumentName", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return("lookup/project-documents/abc", nil)
				mockDoc.On("PutDocumentFile", mock.Anything, mock.AnythingOfType("string"), validFile).Return(nil)
				mockDoc.On("CreateDocumentMetadata", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("bool")).Return(nil).Maybe()
			},
		},
		{
			name:        "invalid content type",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/x-executable",
			fileData:    validFile,
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockDoc *domain.MockDocumentRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
			},
			wantErr: domain.ErrInvalidContentType,
		},
		{
			name:        "file too large",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    []byte(strings.Repeat("x", int(models.MaxDocumentFileSize)+1)),
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockDoc *domain.MockDocumentRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
			},
			wantErr: domain.ErrFileTooLarge,
		},
		{
			name:        "project not found",
			projectUID:  "missing",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockDoc *domain.MockDocumentRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "missing").Return(false, nil)
			},
			wantErr: domain.ErrProjectNotFound,
		},
		{
			name:        "duplicate document name",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockDoc *domain.MockDocumentRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockDoc.On("UniqueDocumentName", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return("", domain.ErrDocumentNameExists)
			},
			wantErr: domain.ErrDocumentNameExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, mockRepo, mockMsg, _ := setupServiceForTesting()
			mockDoc := svc.DocumentRepository.(*domain.MockDocumentRepository)
			mockFolder := svc.FolderRepository.(*domain.MockFolderRepository)
			tt.setupMocks(mockRepo, mockDoc, mockFolder, mockMsg)

			result, err := svc.UploadDocument(context.Background(), tt.projectUID, tt.docName, "", "spec.pdf", tt.contentType, tt.folderUID, tt.fileData, false)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.projectUID, result.ProjectUID)
				assert.Equal(t, tt.docName, result.Name)
			}

			mockRepo.AssertExpectations(t)
			mockDoc.AssertExpectations(t)
			mockFolder.AssertExpectations(t)
		})
	}
}

func TestProjectsService_GetDocumentMetadata(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		projectUID  string
		documentUID string
		setupMocks  func(*domain.MockDocumentRepository)
		wantErr     error
	}{
		{
			name:        "success",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			setupMocks: func(mockDoc *domain.MockDocumentRepository) {
				mockDoc.On("GetDocumentMetadata", mock.Anything, "proj-1", "doc-1").Return(
					&models.ProjectDocument{UID: "doc-1", ProjectUID: "proj-1", Name: "Spec", CreatedAt: now, UpdatedAt: now},
					uint64(7), nil,
				)
			},
		},
		{
			name:        "not found",
			projectUID:  "proj-1",
			documentUID: "missing",
			setupMocks: func(mockDoc *domain.MockDocumentRepository) {
				mockDoc.On("GetDocumentMetadata", mock.Anything, "proj-1", "missing").Return(nil, uint64(0), domain.ErrDocumentNotFound)
			},
			wantErr: domain.ErrDocumentNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _, _ := setupServiceForTesting()
			mockDoc := svc.DocumentRepository.(*domain.MockDocumentRepository)
			tt.setupMocks(mockDoc)

			doc, etag, err := svc.GetDocumentMetadata(context.Background(), tt.projectUID, tt.documentUID)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, doc)
				assert.Empty(t, etag)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, doc)
				assert.Equal(t, "7", etag)
			}

			mockDoc.AssertExpectations(t)
		})
	}
}

func TestProjectsService_GetDocumentFile(t *testing.T) {
	now := time.Now()
	fileContent := []byte("pdf content")

	tests := []struct {
		name        string
		projectUID  string
		documentUID string
		setupMocks  func(*domain.MockDocumentRepository)
		wantErr     error
	}{
		{
			name:        "success",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			setupMocks: func(mockDoc *domain.MockDocumentRepository) {
				mockDoc.On("GetDocumentMetadata", mock.Anything, "proj-1", "doc-1").Return(
					&models.ProjectDocument{UID: "doc-1", ProjectUID: "proj-1", Name: "Spec", FileName: "spec.pdf", ContentType: "application/pdf", CreatedAt: now, UpdatedAt: now},
					uint64(1), nil,
				)
				mockDoc.On("GetDocumentFile", mock.Anything, "doc-1").Return(fileContent, nil)
			},
		},
		{
			name:        "metadata not found",
			projectUID:  "proj-1",
			documentUID: "missing",
			setupMocks: func(mockDoc *domain.MockDocumentRepository) {
				mockDoc.On("GetDocumentMetadata", mock.Anything, "proj-1", "missing").Return(nil, uint64(0), domain.ErrDocumentNotFound)
			},
			wantErr: domain.ErrDocumentNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _, _ := setupServiceForTesting()
			mockDoc := svc.DocumentRepository.(*domain.MockDocumentRepository)
			tt.setupMocks(mockDoc)

			data, doc, err := svc.GetDocumentFile(context.Background(), tt.projectUID, tt.documentUID)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, data)
				assert.Nil(t, doc)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, fileContent, data)
				assert.NotNil(t, doc)
			}

			mockDoc.AssertExpectations(t)
		})
	}
}

func TestProjectsService_DeleteDocument(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		projectUID  string
		documentUID string
		ifMatch     *string
		setupMocks  func(*domain.MockDocumentRepository, *domain.MockMessageBuilder)
		wantErr     error
	}{
		{
			name:        "success with etag",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			ifMatch:     misc.StringPtr("4"),
			setupMocks: func(mockDoc *domain.MockDocumentRepository, mockMsg *domain.MockMessageBuilder) {
				mockDoc.On("DeleteDocumentMetadata", mock.Anything, "proj-1", "doc-1", uint64(4)).Return(nil)
				mockDoc.On("DeleteDocumentFile", mock.Anything, "doc-1").Return(nil).Maybe()
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("bool")).Return(nil).Maybe()
			},
		},
		{
			name:        "missing If-Match header",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			ifMatch:     nil,
			setupMocks:  func(mockDoc *domain.MockDocumentRepository, mockMsg *domain.MockMessageBuilder) {},
			wantErr:     domain.ErrValidationFailed,
		},
		{
			name:        "invalid If-Match value",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			ifMatch:     misc.StringPtr("not-a-number"),
			setupMocks:  func(mockDoc *domain.MockDocumentRepository, mockMsg *domain.MockMessageBuilder) {},
			wantErr:     domain.ErrValidationFailed,
		},
		{
			name:        "document not found",
			projectUID:  "proj-1",
			documentUID: "missing",
			ifMatch:     misc.StringPtr("1"),
			setupMocks: func(mockDoc *domain.MockDocumentRepository, mockMsg *domain.MockMessageBuilder) {
				mockDoc.On("DeleteDocumentMetadata", mock.Anything, "proj-1", "missing", uint64(1)).Return(domain.ErrDocumentNotFound)
			},
			wantErr: domain.ErrDocumentNotFound,
		},
		{
			name:        "revision conflict",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			ifMatch:     misc.StringPtr("1"),
			setupMocks: func(mockDoc *domain.MockDocumentRepository, mockMsg *domain.MockMessageBuilder) {
				mockDoc.On("DeleteDocumentMetadata", mock.Anything, "proj-1", "doc-1", uint64(1)).Return(domain.ErrRevisionMismatch)
			},
			wantErr: domain.ErrRevisionMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, mockMsg, _ := setupServiceForTesting()
			mockDoc := svc.DocumentRepository.(*domain.MockDocumentRepository)
			// suppress unused variable warning
			_ = now
			tt.setupMocks(mockDoc, mockMsg)

			err := svc.DeleteDocument(context.Background(), tt.projectUID, tt.documentUID, tt.ifMatch, false)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			mockDoc.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}
}
