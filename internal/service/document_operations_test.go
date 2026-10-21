// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	domainmocks "github.com/linuxfoundation/lfx-v2-project-service/internal/domain/mocks"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

func TestProjectsService_UploadDocument(t *testing.T) {
	validFile := []byte("hello pdf")
	errPutFile := errors.New("object store down")
	errCreateMetadata := errors.New("kv down")

	tests := []struct {
		name        string
		projectUID  string
		docName     string
		contentType string
		fileData    []byte
		folderUID   *string
		fileName    *string // nil means "spec.pdf"
		notReady    bool
		setupMocks  func(*domainmocks.MockProjectRepository, *domainmocks.MockDocumentRepository, *domainmocks.MockFolderRepository, *domainmocks.MockMessageBuilder)
		wantErr     error
		wantType    string // expected stored content type on success; empty skips the check
	}{
		{
			name:        "success without folder",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockDoc.On("UniqueDocumentName", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return("lookup/project-documents/abc", nil)
				mockDoc.On("PutDocumentFile", mock.Anything, mock.AnythingOfType("string"), validFile).Return(nil)
				mockDoc.On("CreateDocumentMetadata", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
				mockMsg.On("SendProjectEventMessage", mock.Anything, constants.ProjectDocumentCreatedSubject,
					mock.MatchedBy(func(m any) bool {
						ev, ok := m.(events.ProjectDocumentCreatedMessage)
						return ok && ev.ProjectUID == "proj-1"
					})).Return(nil).Maybe()
			},
		},
		{
			name:        "success with folder",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			folderUID:   misc.StringPtr("00000000-0000-0000-0000-0000000000f1"),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				now := time.Now()
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "00000000-0000-0000-0000-0000000000f1").Return(&models.ProjectFolder{UID: "00000000-0000-0000-0000-0000000000f1", ProjectUID: "proj-1", Name: "F", CreatedAt: now, UpdatedAt: now}, uint64(1), nil)
				mockDoc.On("UniqueDocumentName", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return("lookup/project-documents/abc", nil)
				mockDoc.On("PutDocumentFile", mock.Anything, mock.AnythingOfType("string"), validFile).Return(nil)
				mockDoc.On("CreateDocumentMetadata", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
				mockMsg.On("SendProjectEventMessage", mock.Anything, constants.ProjectDocumentCreatedSubject,
					mock.MatchedBy(func(m any) bool {
						ev, ok := m.(events.ProjectDocumentCreatedMessage)
						return ok && ev.ProjectUID == "proj-1"
					})).Return(nil).Maybe()
			},
		},
		{
			name:        "malformed folder uid rejected before lookup",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			folderUID:   misc.StringPtr("lookup/project-folders/0123456789abcdef"),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:        "invalid content type",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/x-executable",
			fileData:    validFile,
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrInvalidContentType,
		},
		{
			name:        "file too large",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    []byte(strings.Repeat("x", int(models.MaxDocumentFileSize)+1)),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrFileTooLarge,
		},
		{
			name:        "project not found",
			projectUID:  "missing",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
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
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockDoc.On("UniqueDocumentName", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return("", domain.ErrDocumentNameExists)
			},
			wantErr: domain.ErrDocumentNameExists,
		},
		{
			name:        "empty file rejected",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    []byte{},
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:        "empty name rejected",
			projectUID:  "proj-1",
			docName:     "",
			contentType: "application/pdf",
			fileData:    validFile,
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:        "service not ready",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			notReady:    true,
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrServiceUnavailable,
		},
		{
			name:        "empty file name rejected",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			fileName:    misc.StringPtr(""),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:        "sniffed type outside allowlist rejected despite allowed declared type",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    []byte("<html><body>not a pdf</body></html>"),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrInvalidContentType,
		},
		{
			name:        "sniffed allowed type overrides declared type",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockDoc.On("UniqueDocumentName", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return("lookup/project-documents/abc", nil)
				mockDoc.On("PutDocumentFile", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil)
				mockDoc.On("CreateDocumentMetadata", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
				mockMsg.On("SendProjectEventMessage", mock.Anything, constants.ProjectDocumentCreatedSubject, mock.Anything).Return(nil).Maybe()
			},
			wantType: "image/png",
		},
		{
			name:        "undetectable binary keeps declared type",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/msword",
			fileData:    []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockDoc.On("UniqueDocumentName", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return("lookup/project-documents/abc", nil)
				mockDoc.On("PutDocumentFile", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil)
				mockDoc.On("CreateDocumentMetadata", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
				mockMsg.On("SendProjectEventMessage", mock.Anything, constants.ProjectDocumentCreatedSubject, mock.Anything).Return(nil).Maybe()
			},
			wantType: "application/msword",
		},
		{
			name:        "project existence check fails",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(false, errors.New("kv down"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:        "empty folder uid is treated as no folder",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			folderUID:   misc.StringPtr(""),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockDoc.On("UniqueDocumentName", mock.Anything, mock.MatchedBy(func(d *models.ProjectDocument) bool { return d.FolderUID == nil })).Return("lookup/project-documents/abc", nil)
				mockDoc.On("PutDocumentFile", mock.Anything, mock.AnythingOfType("string"), validFile).Return(nil)
				mockDoc.On("CreateDocumentMetadata", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
				mockMsg.On("SendProjectEventMessage", mock.Anything, constants.ProjectDocumentCreatedSubject, mock.Anything).Return(nil).Maybe()
			},
		},
		{
			name:        "folder not found",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			folderUID:   misc.StringPtr("00000000-0000-0000-0000-0000000000f1"),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "00000000-0000-0000-0000-0000000000f1").Return(nil, uint64(0), domain.ErrFolderNotFound)
			},
			wantErr: domain.ErrFolderNotFound,
		},
		{
			name:        "folder lookup fails",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			folderUID:   misc.StringPtr("00000000-0000-0000-0000-0000000000f1"),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "00000000-0000-0000-0000-0000000000f1").Return(nil, uint64(0), errors.New("kv down"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:        "file write fails — name reservation released",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockDoc.On("UniqueDocumentName", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return("lookup/project-documents/abc", nil)
				mockDoc.On("PutDocumentFile", mock.Anything, mock.AnythingOfType("string"), validFile).Return(errPutFile)
				mockDoc.On("DeleteUniqueDocumentName", mock.Anything, "lookup/project-documents/abc").Return(nil).Once()
			},
			wantErr: errPutFile,
		},
		{
			name:        "file write fails and name release fails — original error returned",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockDoc.On("UniqueDocumentName", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return("lookup/project-documents/abc", nil)
				mockDoc.On("PutDocumentFile", mock.Anything, mock.AnythingOfType("string"), validFile).Return(errPutFile)
				mockDoc.On("DeleteUniqueDocumentName", mock.Anything, "lookup/project-documents/abc").Return(errors.New("rollback failed")).Once()
			},
			wantErr: errPutFile,
		},
		{
			name:        "metadata write fails — file and name reservation rolled back",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockDoc.On("UniqueDocumentName", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return("lookup/project-documents/abc", nil)
				mockDoc.On("PutDocumentFile", mock.Anything, mock.AnythingOfType("string"), validFile).Return(nil)
				mockDoc.On("CreateDocumentMetadata", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return(errCreateMetadata)
				mockDoc.On("DeleteDocumentFile", mock.Anything, mock.AnythingOfType("string")).Return(nil).Once()
				mockDoc.On("DeleteUniqueDocumentName", mock.Anything, "lookup/project-documents/abc").Return(nil).Once()
			},
			wantErr: errCreateMetadata,
		},
		{
			name:        "metadata write fails and rollbacks fail — original error returned",
			projectUID:  "proj-1",
			docName:     "Spec",
			contentType: "application/pdf",
			fileData:    validFile,
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockDoc *domainmocks.MockDocumentRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockDoc.On("UniqueDocumentName", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return("lookup/project-documents/abc", nil)
				mockDoc.On("PutDocumentFile", mock.Anything, mock.AnythingOfType("string"), validFile).Return(nil)
				mockDoc.On("CreateDocumentMetadata", mock.Anything, mock.AnythingOfType("*models.ProjectDocument")).Return(errCreateMetadata)
				mockDoc.On("DeleteDocumentFile", mock.Anything, mock.AnythingOfType("string")).Return(errors.New("file rollback failed")).Once()
				mockDoc.On("DeleteUniqueDocumentName", mock.Anything, "lookup/project-documents/abc").Return(errors.New("name rollback failed")).Once()
			},
			wantErr: errCreateMetadata,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, mockRepo, mockMsg := setupServiceForTesting()
			mockDoc := svc.DocumentRepository.(*domainmocks.MockDocumentRepository)
			mockFolder := svc.FolderRepository.(*domainmocks.MockFolderRepository)
			tt.setupMocks(mockRepo, mockDoc, mockFolder, mockMsg)
			if tt.notReady {
				svc.ProjectRepository = nil
			}
			fileName := "spec.pdf"
			if tt.fileName != nil {
				fileName = *tt.fileName
			}

			result, err := svc.UploadDocument(context.Background(), tt.projectUID, tt.docName, "", fileName, tt.contentType, tt.folderUID, tt.fileData)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.projectUID, result.ProjectUID)
				assert.Equal(t, tt.docName, result.Name)
				if tt.wantType != "" {
					assert.Equal(t, tt.wantType, result.ContentType)
				}
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
		notReady    bool
		setupMocks  func(*domainmocks.MockDocumentRepository)
		wantErr     error
	}{
		{
			name:        "service not ready",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			notReady:    true,
			setupMocks:  func(mockDoc *domainmocks.MockDocumentRepository) {},
			wantErr:     domain.ErrServiceUnavailable,
		},
		{
			name:        "repository error maps to internal",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository) {
				mockDoc.On("GetDocumentMetadata", mock.Anything, "proj-1", "doc-1").Return(nil, uint64(0), errors.New("kv down"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:        "success",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository) {
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
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository) {
				mockDoc.On("GetDocumentMetadata", mock.Anything, "proj-1", "missing").Return(nil, uint64(0), domain.ErrDocumentNotFound)
			},
			wantErr: domain.ErrDocumentNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _ := setupServiceForTesting()
			mockDoc := svc.DocumentRepository.(*domainmocks.MockDocumentRepository)
			tt.setupMocks(mockDoc)
			if tt.notReady {
				svc.ProjectRepository = nil
			}

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
	errObjectStore := errors.New("object store down")

	tests := []struct {
		name        string
		projectUID  string
		documentUID string
		notReady    bool
		setupMocks  func(*domainmocks.MockDocumentRepository)
		wantErr     error
	}{
		{
			name:        "service not ready",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			notReady:    true,
			setupMocks:  func(mockDoc *domainmocks.MockDocumentRepository) {},
			wantErr:     domain.ErrServiceUnavailable,
		},
		{
			name:        "metadata repository error maps to internal",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository) {
				mockDoc.On("GetDocumentMetadata", mock.Anything, "proj-1", "doc-1").Return(nil, uint64(0), errors.New("kv down"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:        "file read error is returned",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository) {
				mockDoc.On("GetDocumentMetadata", mock.Anything, "proj-1", "doc-1").Return(
					&models.ProjectDocument{UID: "doc-1", ProjectUID: "proj-1", Name: "Spec", CreatedAt: now, UpdatedAt: now},
					uint64(1), nil,
				)
				mockDoc.On("GetDocumentFile", mock.Anything, "doc-1").Return(nil, errObjectStore)
			},
			wantErr: errObjectStore,
		},
		{
			name:        "success",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository) {
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
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository) {
				mockDoc.On("GetDocumentMetadata", mock.Anything, "proj-1", "missing").Return(nil, uint64(0), domain.ErrDocumentNotFound)
			},
			wantErr: domain.ErrDocumentNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _ := setupServiceForTesting()
			mockDoc := svc.DocumentRepository.(*domainmocks.MockDocumentRepository)
			tt.setupMocks(mockDoc)
			if tt.notReady {
				svc.ProjectRepository = nil
			}

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
		notReady    bool
		skipEtag    bool
		// fileDeleteErr, when expectFileDelete is set, is returned by the background DeleteDocumentFile call.
		expectFileDelete bool
		fileDeleteErr    error
		setupMocks       func(*domainmocks.MockDocumentRepository, *domainmocks.MockMessageBuilder)
		wantErr          error
	}{
		{
			name:        "service not ready",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			ifMatch:     misc.StringPtr("1"),
			notReady:    true,
			setupMocks:  func(mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {},
			wantErr:     domain.ErrServiceUnavailable,
		},
		{
			name:        "skip etag validation resolves revision from the store",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			skipEtag:    true,
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockDoc.On("GetDocumentMetadata", mock.Anything, "proj-1", "doc-1").Return(
					&models.ProjectDocument{UID: "doc-1", ProjectUID: "proj-1", CreatedAt: now, UpdatedAt: now}, uint64(9), nil,
				)
				mockDoc.On("DeleteDocumentMetadata", mock.Anything, "proj-1", "doc-1", uint64(9)).Return(nil)
				mockDoc.On("DeleteDocumentFile", mock.Anything, "doc-1").Return(nil).Maybe()
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
			},
		},
		{
			name:        "skip etag validation returns the store error when the revision lookup fails",
			projectUID:  "proj-1",
			documentUID: "missing",
			skipEtag:    true,
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockDoc.On("GetDocumentMetadata", mock.Anything, "proj-1", "missing").Return(nil, uint64(0), domain.ErrDocumentNotFound)
			},
			wantErr: domain.ErrDocumentNotFound,
		},
		{
			name:        "metadata delete failure is returned and no file delete happens",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			ifMatch:     misc.StringPtr("2"),
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockDoc.On("DeleteDocumentMetadata", mock.Anything, "proj-1", "doc-1", uint64(2)).Return(domain.ErrInternal)
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:        "background file delete failure does not fail the request",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			ifMatch:     misc.StringPtr("3"),
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockDoc.On("DeleteDocumentMetadata", mock.Anything, "proj-1", "doc-1", uint64(3)).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
			},
			expectFileDelete: true,
			fileDeleteErr:    errors.New("object store down"),
		},
		{
			name:        "success with etag",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			ifMatch:     misc.StringPtr("4"),
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockDoc.On("DeleteDocumentMetadata", mock.Anything, "proj-1", "doc-1", uint64(4)).Return(nil)
				mockDoc.On("DeleteDocumentFile", mock.Anything, "doc-1").Return(nil).Maybe()
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
			},
		},
		{
			name:        "missing If-Match header",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			ifMatch:     nil,
			setupMocks:  func(mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {},
			wantErr:     domain.ErrValidationFailed,
		},
		{
			name:        "invalid If-Match value",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			ifMatch:     misc.StringPtr("not-a-number"),
			setupMocks:  func(mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {},
			wantErr:     domain.ErrValidationFailed,
		},
		{
			name:        "document not found",
			projectUID:  "proj-1",
			documentUID: "missing",
			ifMatch:     misc.StringPtr("1"),
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockDoc.On("DeleteDocumentMetadata", mock.Anything, "proj-1", "missing", uint64(1)).Return(domain.ErrDocumentNotFound)
			},
			wantErr: domain.ErrDocumentNotFound,
		},
		{
			name:        "revision conflict",
			projectUID:  "proj-1",
			documentUID: "doc-1",
			ifMatch:     misc.StringPtr("1"),
			setupMocks: func(mockDoc *domainmocks.MockDocumentRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockDoc.On("DeleteDocumentMetadata", mock.Anything, "proj-1", "doc-1", uint64(1)).Return(domain.ErrRevisionMismatch)
			},
			wantErr: domain.ErrRevisionMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, mockMsg := setupServiceForTesting()
			mockDoc := svc.DocumentRepository.(*domainmocks.MockDocumentRepository)
			tt.setupMocks(mockDoc, mockMsg)
			svc.Config.SkipEtagValidation = tt.skipEtag
			if tt.notReady {
				svc.ProjectRepository = nil
			}
			fileDeleted := make(chan struct{})
			if tt.expectFileDelete {
				mockDoc.On("DeleteDocumentFile", mock.Anything, tt.documentUID).Return(tt.fileDeleteErr).
					Run(func(mock.Arguments) { close(fileDeleted) }).Once()
			}

			err := svc.DeleteDocument(context.Background(), tt.projectUID, tt.documentUID, tt.ifMatch)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			if tt.expectFileDelete {
				select {
				case <-fileDeleted:
				case <-time.After(2 * time.Second):
					t.Fatal("timed out waiting for background file delete")
				}
			}

			mockDoc.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}
}
