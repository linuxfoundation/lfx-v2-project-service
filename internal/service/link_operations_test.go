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
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

func TestProjectsService_CreateLink(t *testing.T) {
	tests := []struct {
		name        string
		projectUID  string
		linkName    string
		url         string
		description string
		folderUID   *string
		setupMocks  func(*domain.MockProjectRepository, *domain.MockLinkRepository, *domain.MockFolderRepository, *domain.MockMessageBuilder)
		wantErr     error
	}{
		{
			name:        "success without folder",
			projectUID:  "proj-1",
			linkName:    "LFX",
			url:         "https://lfx.linuxfoundation.org",
			description: "LFX home",
			folderUID:   nil,
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockLink *domain.MockLinkRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockLink.On("CreateLink", mock.Anything, mock.AnythingOfType("*models.ProjectLink")).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("bool")).Return(nil).Maybe()
				mockMsg.On("SendProjectEventMessage", mock.Anything, constants.ProjectLinkCreatedSubject,
					mock.MatchedBy(func(m any) bool {
						ev, ok := m.(events.ProjectLinkCreatedMessage)
						return ok && ev.ProjectUID == "proj-1"
					})).Return(nil).Maybe()
			},
		},
		{
			name:       "success with folder",
			projectUID: "proj-1",
			linkName:   "RFC",
			url:        "https://example.com",
			folderUID:  misc.StringPtr("folder-1"),
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockLink *domain.MockLinkRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
				now := time.Now()
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "folder-1").Return(&models.ProjectFolder{UID: "folder-1", ProjectUID: "proj-1", Name: "F", CreatedAt: now, UpdatedAt: now}, uint64(1), nil)
				mockLink.On("CreateLink", mock.Anything, mock.AnythingOfType("*models.ProjectLink")).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("bool")).Return(nil).Maybe()
				mockMsg.On("SendProjectEventMessage", mock.Anything, constants.ProjectLinkCreatedSubject,
					mock.MatchedBy(func(m any) bool {
						ev, ok := m.(events.ProjectLinkCreatedMessage)
						return ok && ev.ProjectUID == "proj-1"
					})).Return(nil).Maybe()
			},
		},
		{
			name:       "project not found",
			projectUID: "missing",
			linkName:   "LFX",
			url:        "https://example.com",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockLink *domain.MockLinkRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "missing").Return(false, nil)
			},
			wantErr: domain.ErrProjectNotFound,
		},
		{
			name:       "folder not found",
			projectUID: "proj-1",
			linkName:   "LFX",
			url:        "https://example.com",
			folderUID:  misc.StringPtr("bad-folder"),
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockLink *domain.MockLinkRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "bad-folder").Return(nil, uint64(0), domain.ErrFolderNotFound)
			},
			wantErr: domain.ErrFolderNotFound,
		},
		{
			name:       "repository error on project check",
			projectUID: "proj-1",
			linkName:   "LFX",
			url:        "https://example.com",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockLink *domain.MockLinkRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(false, domain.ErrInternal)
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "empty name rejected",
			projectUID: "proj-1",
			linkName:   "",
			url:        "https://example.com",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockLink *domain.MockLinkRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:       "non-http scheme rejected",
			projectUID: "proj-1",
			linkName:   "Malicious",
			url:        "javascript:alert(1)",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockLink *domain.MockLinkRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:       "relative URL rejected",
			projectUID: "proj-1",
			linkName:   "Relative",
			url:        "/some/path",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockLink *domain.MockLinkRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:       "empty URL rejected",
			projectUID: "proj-1",
			linkName:   "Empty",
			url:        "",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockLink *domain.MockLinkRepository, mockFolder *domain.MockFolderRepository, mockMsg *domain.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, mockRepo, mockMsg, _ := setupServiceForTesting()
			mockLink := svc.LinkRepository.(*domain.MockLinkRepository)
			mockFolder := svc.FolderRepository.(*domain.MockFolderRepository)
			tt.setupMocks(mockRepo, mockLink, mockFolder, mockMsg)

			result, err := svc.CreateLink(context.Background(), tt.projectUID, tt.linkName, tt.url, tt.description, tt.folderUID, false)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.projectUID, result.ProjectUID)
				assert.Equal(t, tt.linkName, result.Name)
			}

			mockRepo.AssertExpectations(t)
			mockLink.AssertExpectations(t)
			mockFolder.AssertExpectations(t)
		})
	}
}

func TestProjectsService_GetLink(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		projectUID string
		linkUID    string
		setupMocks func(*domain.MockLinkRepository)
		wantErr    error
	}{
		{
			name:       "success",
			projectUID: "proj-1",
			linkUID:    "link-1",
			setupMocks: func(mockLink *domain.MockLinkRepository) {
				mockLink.On("GetLink", mock.Anything, "proj-1", "link-1").Return(
					&models.ProjectLink{UID: "link-1", ProjectUID: "proj-1", Name: "L", URL: "https://example.com", CreatedAt: now, UpdatedAt: now},
					uint64(5), nil,
				)
			},
		},
		{
			name:       "not found",
			projectUID: "proj-1",
			linkUID:    "missing",
			setupMocks: func(mockLink *domain.MockLinkRepository) {
				mockLink.On("GetLink", mock.Anything, "proj-1", "missing").Return(nil, uint64(0), domain.ErrLinkNotFound)
			},
			wantErr: domain.ErrLinkNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _, _ := setupServiceForTesting()
			mockLink := svc.LinkRepository.(*domain.MockLinkRepository)
			tt.setupMocks(mockLink)

			link, etag, err := svc.GetLink(context.Background(), tt.projectUID, tt.linkUID)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, link)
				assert.Empty(t, etag)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, link)
				assert.Equal(t, "5", etag)
			}

			mockLink.AssertExpectations(t)
		})
	}
}

func TestProjectsService_DeleteLink(t *testing.T) {
	tests := []struct {
		name       string
		projectUID string
		linkUID    string
		ifMatch    *string
		setupMocks func(*domain.MockLinkRepository, *domain.MockMessageBuilder)
		wantErr    error
	}{
		{
			name:       "success with etag",
			projectUID: "proj-1",
			linkUID:    "link-1",
			ifMatch:    misc.StringPtr("3"),
			setupMocks: func(mockLink *domain.MockLinkRepository, mockMsg *domain.MockMessageBuilder) {
				mockLink.On("DeleteLink", mock.Anything, "proj-1", "link-1", uint64(3)).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("bool")).Return(nil).Maybe()
			},
		},
		{
			name:       "missing If-Match header",
			projectUID: "proj-1",
			linkUID:    "link-1",
			ifMatch:    nil,
			setupMocks: func(mockLink *domain.MockLinkRepository, mockMsg *domain.MockMessageBuilder) {},
			wantErr:    domain.ErrValidationFailed,
		},
		{
			name:       "invalid If-Match value",
			projectUID: "proj-1",
			linkUID:    "link-1",
			ifMatch:    misc.StringPtr("not-a-number"),
			setupMocks: func(mockLink *domain.MockLinkRepository, mockMsg *domain.MockMessageBuilder) {},
			wantErr:    domain.ErrValidationFailed,
		},
		{
			name:       "revision conflict",
			projectUID: "proj-1",
			linkUID:    "link-1",
			ifMatch:    misc.StringPtr("1"),
			setupMocks: func(mockLink *domain.MockLinkRepository, mockMsg *domain.MockMessageBuilder) {
				mockLink.On("DeleteLink", mock.Anything, "proj-1", "link-1", uint64(1)).Return(domain.ErrRevisionMismatch)
			},
			wantErr: domain.ErrRevisionMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, mockMsg, _ := setupServiceForTesting()
			mockLink := svc.LinkRepository.(*domain.MockLinkRepository)
			tt.setupMocks(mockLink, mockMsg)

			err := svc.DeleteLink(context.Background(), tt.projectUID, tt.linkUID, tt.ifMatch, false)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			mockLink.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}
}
