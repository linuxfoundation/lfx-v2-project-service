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
		notReady    bool
		setupMocks  func(*domainmocks.MockProjectRepository, *domainmocks.MockLinkRepository, *domainmocks.MockFolderRepository, *domainmocks.MockMessageBuilder)
		wantErr     error
	}{
		{
			name:       "service not ready",
			projectUID: "proj-1",
			linkName:   "LFX",
			url:        "https://example.com",
			notReady:   true,
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrServiceUnavailable,
		},
		{
			name:       "empty folder uid is treated as no folder",
			projectUID: "proj-1",
			linkName:   "LFX",
			url:        "https://example.com",
			folderUID:  misc.StringPtr(""),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockLink.On("CreateLink", mock.Anything, mock.MatchedBy(func(l *models.ProjectLink) bool { return l.FolderUID == nil })).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
				mockMsg.On("SendProjectEventMessage", mock.Anything, constants.ProjectLinkCreatedSubject, mock.Anything).Return(nil).Maybe()
			},
		},
		{
			name:       "folder lookup fails with unexpected error",
			projectUID: "proj-1",
			linkName:   "LFX",
			url:        "https://example.com",
			folderUID:  misc.StringPtr("00000000-0000-0000-0000-0000000000f3"),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "00000000-0000-0000-0000-0000000000f3").Return(nil, uint64(0), errors.New("kv unavailable"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "repository create failure is returned",
			projectUID: "proj-1",
			linkName:   "LFX",
			url:        "https://example.com",
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockLink.On("CreateLink", mock.Anything, mock.AnythingOfType("*models.ProjectLink")).Return(domain.ErrInternal)
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:        "success without folder",
			projectUID:  "proj-1",
			linkName:    "LFX",
			url:         "https://lfx.linuxfoundation.org",
			description: "LFX home",
			folderUID:   nil,
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockLink.On("CreateLink", mock.Anything, mock.AnythingOfType("*models.ProjectLink")).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
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
			folderUID:  misc.StringPtr("00000000-0000-0000-0000-0000000000f1"),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				now := time.Now()
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "00000000-0000-0000-0000-0000000000f1").Return(&models.ProjectFolder{UID: "00000000-0000-0000-0000-0000000000f1", ProjectUID: "proj-1", Name: "F", CreatedAt: now, UpdatedAt: now}, uint64(1), nil)
				mockLink.On("CreateLink", mock.Anything, mock.AnythingOfType("*models.ProjectLink")).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
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
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "missing").Return(false, nil)
			},
			wantErr: domain.ErrProjectNotFound,
		},
		{
			name:       "folder not found",
			projectUID: "proj-1",
			linkName:   "LFX",
			url:        "https://example.com",
			folderUID:  misc.StringPtr("00000000-0000-0000-0000-0000000000f2"),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
				mockFolder.On("GetFolder", mock.Anything, "proj-1", "00000000-0000-0000-0000-0000000000f2").Return(nil, uint64(0), domain.ErrFolderNotFound)
			},
			wantErr: domain.ErrFolderNotFound,
		},
		{
			name:       "repository error on project check",
			projectUID: "proj-1",
			linkName:   "LFX",
			url:        "https://example.com",
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(false, domain.ErrInternal)
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "malformed folder uid rejected before lookup",
			projectUID: "proj-1",
			linkName:   "RFC",
			url:        "https://example.com",
			folderUID:  misc.StringPtr("lookup/project-folders/0123456789abcdef"),
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "proj-1").Return(true, nil)
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:       "empty name rejected",
			projectUID: "proj-1",
			linkName:   "",
			url:        "https://example.com",
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:       "non-http scheme rejected",
			projectUID: "proj-1",
			linkName:   "Malicious",
			url:        "javascript:alert(1)",
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:       "relative URL rejected",
			projectUID: "proj-1",
			linkName:   "Relative",
			url:        "/some/path",
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:       "empty URL rejected",
			projectUID: "proj-1",
			linkName:   "Empty",
			url:        "",
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockLink *domainmocks.MockLinkRepository, mockFolder *domainmocks.MockFolderRepository, mockMsg *domainmocks.MockMessageBuilder) {
			},
			wantErr: domain.ErrValidationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, mockRepo, mockMsg := setupServiceForTesting()
			mockLink := svc.LinkRepository.(*domainmocks.MockLinkRepository)
			mockFolder := svc.FolderRepository.(*domainmocks.MockFolderRepository)
			if tt.notReady {
				svc.Resolver = nil
			}
			tt.setupMocks(mockRepo, mockLink, mockFolder, mockMsg)

			result, err := svc.CreateLink(context.Background(), tt.projectUID, tt.linkName, tt.url, tt.description, tt.folderUID)

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
		notReady   bool
		setupMocks func(*domainmocks.MockLinkRepository)
		wantErr    error
	}{
		{
			name:       "service not ready",
			projectUID: "proj-1",
			linkUID:    "link-1",
			notReady:   true,
			setupMocks: func(mockLink *domainmocks.MockLinkRepository) {},
			wantErr:    domain.ErrServiceUnavailable,
		},
		{
			name:       "unexpected repository error maps to internal",
			projectUID: "proj-1",
			linkUID:    "link-1",
			setupMocks: func(mockLink *domainmocks.MockLinkRepository) {
				mockLink.On("GetLink", mock.Anything, "proj-1", "link-1").Return(nil, uint64(0), errors.New("kv unavailable"))
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "success",
			projectUID: "proj-1",
			linkUID:    "link-1",
			setupMocks: func(mockLink *domainmocks.MockLinkRepository) {
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
			setupMocks: func(mockLink *domainmocks.MockLinkRepository) {
				mockLink.On("GetLink", mock.Anything, "proj-1", "missing").Return(nil, uint64(0), domain.ErrLinkNotFound)
			},
			wantErr: domain.ErrLinkNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _ := setupServiceForTesting()
			mockLink := svc.LinkRepository.(*domainmocks.MockLinkRepository)
			if tt.notReady {
				svc.Resolver = nil
			}
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
		notReady   bool
		skipEtag   bool
		setupMocks func(*domainmocks.MockLinkRepository, *domainmocks.MockMessageBuilder)
		wantErr    error
	}{
		{
			name:       "service not ready",
			projectUID: "proj-1",
			linkUID:    "link-1",
			ifMatch:    misc.StringPtr("3"),
			notReady:   true,
			setupMocks: func(mockLink *domainmocks.MockLinkRepository, mockMsg *domainmocks.MockMessageBuilder) {},
			wantErr:    domain.ErrServiceUnavailable,
		},
		{
			name:       "skip etag validation fetches the current revision",
			projectUID: "proj-1",
			linkUID:    "link-1",
			skipEtag:   true,
			setupMocks: func(mockLink *domainmocks.MockLinkRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockLink.On("GetLink", mock.Anything, "proj-1", "link-1").Return(&models.ProjectLink{UID: "link-1", ProjectUID: "proj-1"}, uint64(7), nil)
				mockLink.On("DeleteLink", mock.Anything, "proj-1", "link-1", uint64(7)).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
			},
		},
		{
			name:       "skip etag validation returns not found from revision fetch",
			projectUID: "proj-1",
			linkUID:    "missing",
			skipEtag:   true,
			setupMocks: func(mockLink *domainmocks.MockLinkRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockLink.On("GetLink", mock.Anything, "proj-1", "missing").Return(nil, uint64(0), domain.ErrLinkNotFound)
			},
			wantErr: domain.ErrLinkNotFound,
		},
		{
			name:       "link not found on delete",
			projectUID: "proj-1",
			linkUID:    "missing",
			ifMatch:    misc.StringPtr("2"),
			setupMocks: func(mockLink *domainmocks.MockLinkRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockLink.On("DeleteLink", mock.Anything, "proj-1", "missing", uint64(2)).Return(domain.ErrLinkNotFound)
			},
			wantErr: domain.ErrLinkNotFound,
		},
		{
			name:       "success with etag",
			projectUID: "proj-1",
			linkUID:    "link-1",
			ifMatch:    misc.StringPtr("3"),
			setupMocks: func(mockLink *domainmocks.MockLinkRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockLink.On("DeleteLink", mock.Anything, "proj-1", "link-1", uint64(3)).Return(nil)
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
			},
		},
		{
			name:       "missing If-Match header",
			projectUID: "proj-1",
			linkUID:    "link-1",
			ifMatch:    nil,
			setupMocks: func(mockLink *domainmocks.MockLinkRepository, mockMsg *domainmocks.MockMessageBuilder) {},
			wantErr:    domain.ErrValidationFailed,
		},
		{
			name:       "invalid If-Match value",
			projectUID: "proj-1",
			linkUID:    "link-1",
			ifMatch:    misc.StringPtr("not-a-number"),
			setupMocks: func(mockLink *domainmocks.MockLinkRepository, mockMsg *domainmocks.MockMessageBuilder) {},
			wantErr:    domain.ErrValidationFailed,
		},
		{
			name:       "revision conflict",
			projectUID: "proj-1",
			linkUID:    "link-1",
			ifMatch:    misc.StringPtr("1"),
			setupMocks: func(mockLink *domainmocks.MockLinkRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockLink.On("DeleteLink", mock.Anything, "proj-1", "link-1", uint64(1)).Return(domain.ErrRevisionMismatch)
			},
			wantErr: domain.ErrRevisionMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, mockMsg := setupServiceForTesting()
			mockLink := svc.LinkRepository.(*domainmocks.MockLinkRepository)
			if tt.notReady {
				svc.Resolver = nil
			}
			svc.Config.SkipEtagValidation = tt.skipEtag
			tt.setupMocks(mockLink, mockMsg)

			err := svc.DeleteLink(context.Background(), tt.projectUID, tt.linkUID, tt.ifMatch)

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

func TestProjectsService_UpdateLink(t *testing.T) {
	const (
		folderUID    = "00000000-0000-0000-0000-0000000000f1"
		oldFolderUID = "00000000-0000-0000-0000-0000000000f9"
		created      = "2026-01-02T03:04:05Z"
	)
	createdAt, _ := time.Parse(time.RFC3339, created)
	existing := func() *models.ProjectLink {
		return &models.ProjectLink{
			UID:         "link-1",
			ProjectUID:  "proj-1",
			FolderUID:   misc.StringPtr(oldFolderUID),
			Name:        "Old name",
			URL:         "https://old.example.com",
			Description: "old",
			CreatedBy:   &models.UserInfo{Username: "creator"},
			CreatedAt:   createdAt,
			UpdatedAt:   createdAt,
		}
	}

	tests := []struct {
		name        string
		linkName    string
		url         string
		description string
		folderUID   *string
		ifMatch     *string
		notReady    bool
		skipEtag    bool
		setupMocks  func(*domainmocks.MockLinkRepository, *domainmocks.MockFolderRepository, *domainmocks.MockMessageBuilder, chan indexerTypes.IndexerMessageEnvelope)
		wantErr     error
		wantFolder  *string
		wantTag     string
		wantNoTag   string
	}{
		{
			name:     "service not ready",
			linkName: "New",
			url:      "https://example.com",
			ifMatch:  misc.StringPtr("3"),
			notReady: true,
			setupMocks: func(*domainmocks.MockLinkRepository, *domainmocks.MockFolderRepository, *domainmocks.MockMessageBuilder, chan indexerTypes.IndexerMessageEnvelope) {
			},
			wantErr: domain.ErrServiceUnavailable,
		},
		{
			name:     "empty name is rejected",
			linkName: "",
			url:      "https://example.com",
			ifMatch:  misc.StringPtr("3"),
			setupMocks: func(*domainmocks.MockLinkRepository, *domainmocks.MockFolderRepository, *domainmocks.MockMessageBuilder, chan indexerTypes.IndexerMessageEnvelope) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:     "non-http url is rejected",
			linkName: "New",
			url:      "ftp://example.com",
			ifMatch:  misc.StringPtr("3"),
			setupMocks: func(*domainmocks.MockLinkRepository, *domainmocks.MockFolderRepository, *domainmocks.MockMessageBuilder, chan indexerTypes.IndexerMessageEnvelope) {
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:     "link not found",
			linkName: "New",
			url:      "https://example.com",
			ifMatch:  misc.StringPtr("3"),
			setupMocks: func(l *domainmocks.MockLinkRepository, _ *domainmocks.MockFolderRepository, _ *domainmocks.MockMessageBuilder, _ chan indexerTypes.IndexerMessageEnvelope) {
				l.On("GetLink", mock.Anything, "proj-1", "link-1").Return(nil, uint64(0), domain.ErrLinkNotFound)
			},
			wantErr: domain.ErrLinkNotFound,
		},
		{
			name:     "missing If-Match header",
			linkName: "New",
			url:      "https://example.com",
			setupMocks: func(l *domainmocks.MockLinkRepository, _ *domainmocks.MockFolderRepository, _ *domainmocks.MockMessageBuilder, _ chan indexerTypes.IndexerMessageEnvelope) {
				l.On("GetLink", mock.Anything, "proj-1", "link-1").Return(existing(), uint64(3), nil)
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:      "target folder is not in the project",
			linkName:  "New",
			url:       "https://example.com",
			folderUID: misc.StringPtr(folderUID),
			ifMatch:   misc.StringPtr("3"),
			setupMocks: func(l *domainmocks.MockLinkRepository, f *domainmocks.MockFolderRepository, _ *domainmocks.MockMessageBuilder, _ chan indexerTypes.IndexerMessageEnvelope) {
				l.On("GetLink", mock.Anything, "proj-1", "link-1").Return(existing(), uint64(3), nil)
				f.On("GetFolder", mock.Anything, "proj-1", folderUID).Return(nil, uint64(0), domain.ErrFolderNotFound)
			},
			wantErr: domain.ErrFolderNotFound,
		},
		{
			name:      "malformed folder uid is rejected",
			linkName:  "New",
			url:       "https://example.com",
			folderUID: misc.StringPtr("not a uid"),
			ifMatch:   misc.StringPtr("3"),
			setupMocks: func(l *domainmocks.MockLinkRepository, _ *domainmocks.MockFolderRepository, _ *domainmocks.MockMessageBuilder, _ chan indexerTypes.IndexerMessageEnvelope) {
				l.On("GetLink", mock.Anything, "proj-1", "link-1").Return(existing(), uint64(3), nil)
			},
			wantErr: domain.ErrValidationFailed,
		},
		{
			name:     "stale revision is a conflict and publishes nothing",
			linkName: "New",
			url:      "https://example.com",
			ifMatch:  misc.StringPtr("2"),
			setupMocks: func(l *domainmocks.MockLinkRepository, _ *domainmocks.MockFolderRepository, _ *domainmocks.MockMessageBuilder, _ chan indexerTypes.IndexerMessageEnvelope) {
				l.On("GetLink", mock.Anything, "proj-1", "link-1").Return(existing(), uint64(3), nil)
				l.On("UpdateLink", mock.Anything, mock.AnythingOfType("*models.ProjectLink"), uint64(2)).Return(domain.ErrRevisionMismatch)
			},
			wantErr: domain.ErrRevisionMismatch,
		},
		{
			name:        "success moves the link into a folder and keeps its creation fields",
			linkName:    "New name",
			url:         "https://new.example.com",
			description: "new",
			folderUID:   misc.StringPtr(folderUID),
			ifMatch:     misc.StringPtr("3"),
			setupMocks: func(l *domainmocks.MockLinkRepository, f *domainmocks.MockFolderRepository, m *domainmocks.MockMessageBuilder, indexed chan indexerTypes.IndexerMessageEnvelope) {
				l.On("GetLink", mock.Anything, "proj-1", "link-1").Return(existing(), uint64(3), nil)
				f.On("GetFolder", mock.Anything, "proj-1", folderUID).Return(&models.ProjectFolder{UID: folderUID, ProjectUID: "proj-1", Name: "F"}, uint64(1), nil)
				l.On("UpdateLink", mock.Anything, mock.MatchedBy(func(got *models.ProjectLink) bool {
					return got.UID == "link-1" && got.Name == "New name" && got.URL == "https://new.example.com" &&
						got.Description == "new" && got.FolderUID != nil && *got.FolderUID == folderUID &&
						got.CreatedAt.Equal(createdAt) && got.UpdatedAt.After(createdAt)
				}), uint64(3)).Return(nil)
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectLinkSubject, mock.Anything).Run(func(a mock.Arguments) { indexed <- a.Get(2).(indexerTypes.IndexerMessageEnvelope) }).Return(nil).Once()
			},
			wantFolder: misc.StringPtr(folderUID),
			wantTag:    "folder_uid:" + folderUID,
			wantNoTag:  "folder_uid:" + oldFolderUID,
		},
		{
			name:     "omitting the folder moves the link to the project root",
			linkName: "New name",
			url:      "https://new.example.com",
			ifMatch:  misc.StringPtr("3"),
			setupMocks: func(l *domainmocks.MockLinkRepository, _ *domainmocks.MockFolderRepository, m *domainmocks.MockMessageBuilder, indexed chan indexerTypes.IndexerMessageEnvelope) {
				l.On("GetLink", mock.Anything, "proj-1", "link-1").Return(existing(), uint64(3), nil)
				l.On("UpdateLink", mock.Anything, mock.MatchedBy(func(got *models.ProjectLink) bool { return got.FolderUID == nil }), uint64(3)).Return(nil)
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectLinkSubject, mock.Anything).Run(func(a mock.Arguments) { indexed <- a.Get(2).(indexerTypes.IndexerMessageEnvelope) }).Return(nil).Once()
			},
			wantNoTag: "folder_uid:" + oldFolderUID,
		},
		{
			name:     "skip etag validation writes at the fetched revision",
			linkName: "New name",
			url:      "https://new.example.com",
			skipEtag: true,
			setupMocks: func(l *domainmocks.MockLinkRepository, _ *domainmocks.MockFolderRepository, m *domainmocks.MockMessageBuilder, indexed chan indexerTypes.IndexerMessageEnvelope) {
				l.On("GetLink", mock.Anything, "proj-1", "link-1").Return(existing(), uint64(7), nil)
				l.On("UpdateLink", mock.Anything, mock.AnythingOfType("*models.ProjectLink"), uint64(7)).Return(nil)
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectLinkSubject, mock.Anything).Run(func(a mock.Arguments) { indexed <- a.Get(2).(indexerTypes.IndexerMessageEnvelope) }).Return(nil).Once()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, mockMsg := setupServiceForTesting()
			mockLink := svc.LinkRepository.(*domainmocks.MockLinkRepository)
			mockFolder := svc.FolderRepository.(*domainmocks.MockFolderRepository)
			if tt.notReady {
				svc.Resolver = nil
			}
			svc.Config.SkipEtagValidation = tt.skipEtag
			indexed := make(chan indexerTypes.IndexerMessageEnvelope, 1)
			tt.setupMocks(mockLink, mockFolder, mockMsg, indexed)

			got, err := svc.UpdateLink(context.Background(), "proj-1", "link-1", tt.ifMatch, tt.linkName, tt.url, tt.description, tt.folderUID)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.linkName, got.Name)
				assert.Equal(t, tt.wantFolder, got.FolderUID)
				select {
				case msg := <-indexed:
					assert.Equal(t, indexerConstants.ActionUpdated, msg.Action)
					link, ok := msg.Data.(models.ProjectLink)
					if assert.True(t, ok, "indexer data should be the updated link") {
						assert.Equal(t, "link-1", link.UID)
						assert.Equal(t, tt.linkName, link.Name)
						assert.Equal(t, tt.wantFolder, link.FolderUID)
					}
					if assert.NotNil(t, msg.IndexingConfig) {
						if tt.wantTag != "" {
							assert.Contains(t, msg.IndexingConfig.Tags, tt.wantTag)
						}
						if tt.wantNoTag != "" {
							assert.NotContains(t, msg.IndexingConfig.Tags, tt.wantNoTag)
						}
					}
				case <-time.After(2 * time.Second):
					t.Fatal("expected an updated indexer message")
				}
			}

			mockLink.AssertExpectations(t)
			mockFolder.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}
}
