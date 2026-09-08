// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProjectsService_HandleMessage(t *testing.T) {

	ctx := context.Background()

	tests := []struct {
		name        string
		subject     string
		messageData []byte
		setupMocks  func(*domain.MockProjectRepository, *domain.MockMessageBuilder)
		expectCalls bool
	}{
		{
			name:        "handle project get name message",
			subject:     constants.ProjectGetNameSubject,
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				now := time.Now()
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectBase{
						UID:       "01234567-89ab-cdef-0123-456789abcdef",
						Name:      "Test Project",
						CreatedAt: &now,
						UpdatedAt: &now,
					},
					nil,
				)
			},
			expectCalls: true,
		},
		{
			name:        "handle project get slug message",
			subject:     constants.ProjectGetSlugSubject,
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				now := time.Now()
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectBase{
						UID:       "01234567-89ab-cdef-0123-456789abcdef",
						Name:      "Test Project",
						Slug:      "test-project-slug",
						CreatedAt: &now,
						UpdatedAt: &now,
					},
					nil,
				)
			},
			expectCalls: true,
		},
		{
			name:        "handle project get logo message",
			subject:     constants.ProjectGetLogoSubject,
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				now := time.Now()
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectBase{
						UID:       "01234567-89ab-cdef-0123-456789abcdef",
						Name:      "Test Project",
						LogoURL:   "https://example.com/logo.png",
						CreatedAt: &now,
						UpdatedAt: &now,
					},
					nil,
				)
			},
			expectCalls: true,
		},
		{
			name:        "handle project slug to UID message",
			subject:     constants.ProjectSlugToUIDSubject,
			messageData: []byte("test-project"),
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("GetProjectUIDFromSlug", mock.Anything, "test-project").Return("test-project-uid", nil)
			},
			expectCalls: true,
		},
		{
			name:        "handle project get parent UID message",
			subject:     constants.ProjectGetParentUIDSubject,
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				now := time.Now()
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectBase{
						UID:       "01234567-89ab-cdef-0123-456789abcdef",
						Name:      "Test Project",
						ParentUID: "parent-uid-123",
						CreatedAt: &now,
						UpdatedAt: &now,
					},
					nil,
				)
			},
			expectCalls: true,
		},
		{
			name:        "handle project get writers message",
			subject:     constants.ProjectGetWritersSubject,
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("GetProjectSettings", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectSettings{
						UID: "01234567-89ab-cdef-0123-456789abcdef",
						Writers: []models.UserInfo{
							{Username: "writer1", Email: "writer1@example.com", Name: "Writer One"},
						},
					},
					nil,
				)
			},
			expectCalls: true,
		},
		{
			name:        "handle project get settings message",
			subject:     constants.ProjectGetSettingsSubject,
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("GetProjectSettings", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectSettings{
						UID: "01234567-89ab-cdef-0123-456789abcdef",
						Auditors: []models.UserInfo{
							{Username: "auditor1", Email: "auditor1@example.com", Name: "Auditor One"},
						},
					},
					nil,
				)
			},
			expectCalls: true,
		},
		{
			name:        "handle project list projects message",
			subject:     constants.ProjectListProjectsSubject,
			messageData: []byte(`{"stages":["Formation - Engaged"]}`),
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("ListAllProjectsBase", mock.Anything).Return(
					[]*models.ProjectBase{
						{
							UID:   "01234567-89ab-cdef-0123-456789abcdef",
							Slug:  "test-project-slug",
							Stage: "Formation - Engaged",
						},
					},
					nil,
				)
			},
			expectCalls: true,
		},
		{
			name:        "unknown subject",
			subject:     "unknown.subject",
			messageData: []byte(`{}`),
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				// No mock calls expected
			},
			expectCalls: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, mockBuilder, mockAuth := setupServiceForTesting()
			tt.setupMocks(mockRepo, mockBuilder)

			// Create mock message
			mockMsg := newMockMessage(tt.subject, tt.messageData)

			if tt.expectCalls {
				mockMsg.On("Respond", mock.Anything).Return(nil)
			}

			// Call HandleMessage
			service.HandleMessage(ctx, mockMsg)

			// Verify expectations
			if tt.expectCalls {
				mockMsg.AssertExpectations(t)
			}
			mockRepo.AssertExpectations(t)
			mockBuilder.AssertExpectations(t)
			mockAuth.AssertExpectations(t)
		})
	}
}

func TestProjectsService_HandleProjectGetName(t *testing.T) {

	ctx := context.Background()

	tests := []struct {
		name        string
		messageData []byte
		setupMocks  func(*domain.MockProjectRepository)
		expectedErr bool
		validate    func(*testing.T, []byte)
	}{
		{
			name:        "successful get project name",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				now := time.Now()
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectBase{
						UID:       "01234567-89ab-cdef-0123-456789abcdef",
						Name:      "Test Project Name",
						Slug:      "test-project",
						CreatedAt: &now,
						UpdatedAt: &now,
					},
					nil,
				)
			},
			expectedErr: false,
			validate: func(t *testing.T, response []byte) {
				assert.Equal(t, "Test Project Name", string(response))
			},
		},
		{
			name:        "project not found",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcd00"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcd00").Return(
					nil, domain.ErrProjectNotFound,
				)
			},
			expectedErr: true,
		},
		{
			name:        "invalid JSON",
			messageData: []byte(`invalid-json`),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				// No repo calls expected
			},
			expectedErr: true,
		},
		{
			name:        "missing UID",
			messageData: []byte(`{}`),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				// No repo calls expected
			},
			expectedErr: true,
		},
		{
			name:        "empty UID",
			messageData: []byte(`{"uid": ""}`),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				// No repo calls expected
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, _, _ := setupServiceForTesting()
			tt.setupMocks(mockRepo)

			mockMsg := newMockMessage(constants.ProjectGetNameSubject, tt.messageData)

			response, err := service.HandleProjectGetName(ctx, mockMsg)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if tt.validate != nil {
					tt.validate(t, response)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestProjectsService_HandleProjectGetSlug(t *testing.T) {

	ctx := context.Background()

	tests := []struct {
		name        string
		messageData []byte
		setupMocks  func(*domain.MockProjectRepository)
		expectedErr bool
		validate    func(*testing.T, []byte)
	}{
		{
			name:        "successful get project slug",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				now := time.Now()
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectBase{
						UID:       "01234567-89ab-cdef-0123-456789abcdef",
						Name:      "Test Project Name",
						Slug:      "test-project-slug",
						CreatedAt: &now,
						UpdatedAt: &now,
					},
					nil,
				)
			},
			expectedErr: false,
			validate: func(t *testing.T, response []byte) {
				assert.Equal(t, "test-project-slug", string(response))
			},
		},
		{
			name:        "project not found",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcd00"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcd00").Return(
					nil, domain.ErrProjectNotFound,
				)
			},
			expectedErr: true,
		},
		{
			name:        "invalid UUID format",
			messageData: []byte("invalid-uuid-format"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				// No repo calls expected for invalid UUID
			},
			expectedErr: true,
		},
		{
			name:        "empty project UID",
			messageData: []byte(""),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				// No repo calls expected for empty UID
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, _, _ := setupServiceForTesting()
			tt.setupMocks(mockRepo)

			mockMsg := newMockMessage(constants.ProjectGetSlugSubject, tt.messageData)

			response, err := service.HandleProjectGetSlug(ctx, mockMsg)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if tt.validate != nil {
					tt.validate(t, response)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestProjectsService_HandleProjectGetLogo(t *testing.T) {

	ctx := context.Background()

	tests := []struct {
		name        string
		messageData []byte
		setupMocks  func(*domain.MockProjectRepository)
		expectedErr bool
		validate    func(*testing.T, []byte)
	}{
		{
			name:        "successful get project logo",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				now := time.Now()
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectBase{
						UID:       "01234567-89ab-cdef-0123-456789abcdef",
						Name:      "Test Project Name",
						LogoURL:   "https://example.com/logo.png",
						CreatedAt: &now,
						UpdatedAt: &now,
					},
					nil,
				)
			},
			expectedErr: false,
			validate: func(t *testing.T, response []byte) {
				assert.Equal(t, "https://example.com/logo.png", string(response))
			},
		},
		{
			name:        "project not found",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcd00"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcd00").Return(
					nil, domain.ErrProjectNotFound,
				)
			},
			expectedErr: true,
		},
		{
			name:        "invalid UUID format",
			messageData: []byte("invalid-uuid-format"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				// No repo calls expected for invalid UUID
			},
			expectedErr: true,
		},
		{
			name:        "empty project UID",
			messageData: []byte(""),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				// No repo calls expected for empty UID
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, _, _ := setupServiceForTesting()
			tt.setupMocks(mockRepo)

			mockMsg := newMockMessage(constants.ProjectGetLogoSubject, tt.messageData)

			response, err := service.HandleProjectGetLogo(ctx, mockMsg)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if tt.validate != nil {
					tt.validate(t, response)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestProjectsService_HandleProjectSlugToUID(t *testing.T) {

	ctx := context.Background()

	tests := []struct {
		name        string
		messageData []byte
		setupMocks  func(*domain.MockProjectRepository)
		expectedErr bool
		validate    func(*testing.T, []byte)
	}{
		{
			name:        "successful slug to UID conversion",
			messageData: []byte("test-project"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectUIDFromSlug", mock.Anything, "test-project").Return(
					"test-project-uid", nil,
				)
			},
			expectedErr: false,
			validate: func(t *testing.T, response []byte) {
				assert.Equal(t, "test-project-uid", string(response))
			},
		},
		{
			name:        "project not found by slug",
			messageData: []byte("non-existent-slug"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectUIDFromSlug", mock.Anything, "non-existent-slug").Return(
					"", domain.ErrProjectNotFound,
				)
			},
			expectedErr: true,
		},
		{
			name:        "project not found with strange slug",
			messageData: []byte("invalid-json"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectUIDFromSlug", mock.Anything, "invalid-json").Return(
					"", domain.ErrProjectNotFound,
				)
			},
			expectedErr: true,
		},
		{
			name:        "empty slug",
			messageData: []byte(""),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectUIDFromSlug", mock.Anything, "").Return(
					"", domain.ErrProjectNotFound,
				)
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, _, _ := setupServiceForTesting()
			tt.setupMocks(mockRepo)

			mockMsg := newMockMessage(constants.ProjectSlugToUIDSubject, tt.messageData)

			response, err := service.HandleProjectSlugToUID(ctx, mockMsg)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if tt.validate != nil {
					tt.validate(t, response)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestProjectsService_HandleProjectGetParentUID(t *testing.T) {

	ctx := context.Background()

	tests := []struct {
		name        string
		messageData []byte
		setupMocks  func(*domain.MockProjectRepository)
		expectedErr bool
		validate    func(*testing.T, []byte)
	}{
		{
			name:        "successful get parent UID",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				now := time.Now()
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectBase{
						UID:       "01234567-89ab-cdef-0123-456789abcdef",
						Name:      "Test Project Name",
						ParentUID: "parent-project-uid-123",
						CreatedAt: &now,
						UpdatedAt: &now,
					},
					nil,
				)
			},
			expectedErr: false,
			validate: func(t *testing.T, response []byte) {
				assert.Equal(t, "parent-project-uid-123", string(response))
			},
		},
		{
			name:        "project with empty parent UID",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				now := time.Now()
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectBase{
						UID:       "01234567-89ab-cdef-0123-456789abcdef",
						Name:      "Root Project",
						ParentUID: "",
						CreatedAt: &now,
						UpdatedAt: &now,
					},
					nil,
				)
			},
			expectedErr: false,
			validate: func(t *testing.T, response []byte) {
				assert.Equal(t, "", string(response))
			},
		},
		{
			name:        "project not found",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcd00"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcd00").Return(
					nil, domain.ErrProjectNotFound,
				)
			},
			expectedErr: true,
		},
		{
			name:        "invalid UUID format",
			messageData: []byte("invalid-uuid-format"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				// No repo calls expected for invalid UUID
			},
			expectedErr: true,
		},
		{
			name:        "empty project UID",
			messageData: []byte(""),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				// No repo calls expected for empty UID
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, _, _ := setupServiceForTesting()
			tt.setupMocks(mockRepo)

			mockMsg := newMockMessage(constants.ProjectGetParentUIDSubject, tt.messageData)

			response, err := service.HandleProjectGetParentUID(ctx, mockMsg)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if tt.validate != nil {
					tt.validate(t, response)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestProjectsService_HandleProjectGetWriters(t *testing.T) {

	ctx := context.Background()

	tests := []struct {
		name        string
		messageData []byte
		setupMocks  func(*domain.MockProjectRepository)
		expectedErr bool
		validate    func(*testing.T, []byte)
	}{
		{
			name:        "successful get project writers",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectSettings", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectSettings{
						UID: "01234567-89ab-cdef-0123-456789abcdef",
						Writers: []models.UserInfo{
							{Username: "writer1", Email: "writer1@example.com", Name: "Writer One"},
							{Username: "writer2", Email: "writer2@example.com", Name: "Writer Two"},
						},
					},
					nil,
				)
			},
			expectedErr: false,
			validate: func(t *testing.T, response []byte) {
				var writers []models.UserInfo
				assert.NoError(t, json.Unmarshal(response, &writers))
				assert.Len(t, writers, 2)
				assert.Equal(t, "writer1", writers[0].Username)
				assert.Equal(t, "writer1@example.com", writers[0].Email)
				assert.Equal(t, "writer2", writers[1].Username)
			},
		},
		{
			name:        "returns empty array when no writers configured",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectSettings", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectSettings{
						UID:     "01234567-89ab-cdef-0123-456789abcdef",
						Writers: nil,
					},
					nil,
				)
			},
			expectedErr: false,
			validate: func(t *testing.T, response []byte) {
				var writers []models.UserInfo
				assert.NoError(t, json.Unmarshal(response, &writers))
				assert.Empty(t, writers)
			},
		},
		{
			name:        "project settings not found",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcd00"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectSettings", mock.Anything, "01234567-89ab-cdef-0123-456789abcd00").Return(
					nil, domain.ErrProjectNotFound,
				)
			},
			expectedErr: true,
		},
		{
			name:        "invalid UUID format",
			messageData: []byte("not-a-uuid"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				// No repo calls expected
			},
			expectedErr: true,
		},
		{
			name:        "empty project UID",
			messageData: []byte(""),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				// No repo calls expected
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, _, _ := setupServiceForTesting()
			tt.setupMocks(mockRepo)

			mockMsg := newMockMessage(constants.ProjectGetWritersSubject, tt.messageData)

			response, err := service.HandleProjectGetWriters(ctx, mockMsg)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if tt.validate != nil {
					tt.validate(t, response)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestProjectsService_HandleProjectGetSettings(t *testing.T) {

	ctx := context.Background()

	announcementDate := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		messageData []byte
		setupMocks  func(*domain.MockProjectRepository)
		expectedErr bool
		validate    func(*testing.T, []byte)
	}{
		{
			name:        "returns writers, auditors and announcement date together",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectSettings", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectSettings{
						UID:              "01234567-89ab-cdef-0123-456789abcdef",
						AnnouncementDate: &announcementDate,
						Writers: []models.UserInfo{
							{Username: "writer1", Email: "writer1@example.com", Name: "Writer One"},
						},
						Auditors: []models.UserInfo{
							{Username: "auditor1", Email: "auditor1@example.com", Name: "Auditor One"},
							{Username: "auditor2", Email: "auditor2@example.com", Name: "Auditor Two"},
						},
					},
					nil,
				)
			},
			validate: func(t *testing.T, response []byte) {
				var summary events.ProjectSettingsSummary
				assert.NoError(t, json.Unmarshal(response, &summary))
				assert.Equal(t, "01234567-89ab-cdef-0123-456789abcdef", summary.UID)
				assert.Equal(t, &announcementDate, summary.AnnouncementDate)
				assert.Len(t, summary.Writers, 1)
				assert.Equal(t, "writer1", summary.Writers[0].Username)
				assert.Len(t, summary.Auditors, 2)
				assert.Equal(t, "auditor1", summary.Auditors[0].Username)
				assert.Equal(t, "auditor2", summary.Auditors[1].Username)
			},
		},
		{
			name:        "omits the settings fields that are not part of the roster",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectSettings", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectSettings{
						UID:                 "01234567-89ab-cdef-0123-456789abcdef",
						MissionStatement:    "a mission statement no caller of this subject asked for",
						MeetingCoordinators: []models.UserInfo{{Username: "coordinator1"}},
						ExecutiveDirector:   &models.UserInfo{Username: "director1"},
					},
					nil,
				)
			},
			validate: func(t *testing.T, response []byte) {
				assert.NotContains(t, string(response), "mission_statement")
				assert.NotContains(t, string(response), "meeting_coordinators")
				assert.NotContains(t, string(response), "executive_director")
			},
		},
		{
			name:        "replies with empty rosters rather than null when none are configured",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectSettings", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					&models.ProjectSettings{UID: "01234567-89ab-cdef-0123-456789abcdef"},
					nil,
				)
			},
			validate: func(t *testing.T, response []byte) {
				assert.Contains(t, string(response), `"writers":[]`)
				assert.Contains(t, string(response), `"auditors":[]`)
			},
		},
		{
			name:        "project settings not found",
			messageData: []byte("01234567-89ab-cdef-0123-456789abcd00"),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectSettings", mock.Anything, "01234567-89ab-cdef-0123-456789abcd00").Return(
					nil, domain.ErrProjectNotFound,
				)
			},
			expectedErr: true,
		},
		{
			name:        "invalid UUID format",
			messageData: []byte("not-a-uuid"),
			setupMocks:  func(mockRepo *domain.MockProjectRepository) {},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, _, _ := setupServiceForTesting()
			tt.setupMocks(mockRepo)

			mockMsg := newMockMessage(constants.ProjectGetSettingsSubject, tt.messageData)

			response, err := service.HandleProjectGetSettings(ctx, mockMsg)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if tt.validate != nil {
					tt.validate(t, response)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestProjectsService_HandleProjectListProjects(t *testing.T) {

	ctx := context.Background()

	const (
		formingUID   = "11111111-1111-1111-1111-111111111111"
		activeUID    = "22222222-2222-2222-2222-222222222222"
		prospectUID  = "33333333-3333-3333-3333-333333333333"
		deletedUID   = "44444444-4444-4444-4444-444444444444"
		parentUID    = "99999999-9999-9999-9999-999999999999"
		formingStage = "Formation - Engaged"
	)

	allProjects := []*models.ProjectBase{
		{UID: activeUID, Slug: "active-project", Stage: "Active", ParentUID: parentUID},
		{UID: formingUID, Slug: "forming-project", Stage: formingStage, IsFoundation: true, ParentUID: parentUID},
		{UID: prospectUID, Slug: "prospect-project", Stage: "Prospect", ParentUID: parentUID},
	}

	// One past the cap, each a valid UUID so the request is refused for its size rather
	// than for a malformed entry.
	overCapUIDs := make([]string, maxListProjectsUIDs+1)
	for i := range overCapUIDs {
		overCapUIDs[i] = uuid.NewString()
	}
	overCapRequest, err := json.Marshal(events.ProjectListRequest{UIDs: overCapUIDs})
	require.NoError(t, err)

	tests := []struct {
		name        string
		messageData []byte
		setupMocks  func(*domain.MockProjectRepository)
		expectedErr bool
		validate    func(*testing.T, *domain.MockProjectRepository, []events.ProjectRef)
		validateErr func(*testing.T, *domain.MockProjectRepository)
	}{
		{
			name:        "stage filter returns only the projects at those stages",
			messageData: []byte(`{"stages":["Formation - Engaged"]}`),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("ListAllProjectsBase", mock.Anything).Return(allProjects, nil)
			},
			validate: func(t *testing.T, mockRepo *domain.MockProjectRepository, refs []events.ProjectRef) {
				assert.Len(t, refs, 1)
				assert.Equal(t, formingUID, refs[0].UID)
				assert.Equal(t, "forming-project", refs[0].Slug)
				assert.Equal(t, formingStage, refs[0].Stage)
				assert.True(t, refs[0].IsFoundation)
				assert.Equal(t, parentUID, refs[0].ParentUID)
			},
		},
		{
			name:        "uid filter answers without scanning the store",
			messageData: []byte(`{"uids":["` + activeUID + `"]}`),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectBase", mock.Anything, activeUID).Return(allProjects[0], nil)
			},
			validate: func(t *testing.T, mockRepo *domain.MockProjectRepository, refs []events.ProjectRef) {
				mockRepo.AssertNotCalled(t, "ListAllProjectsBase", mock.Anything)
				assert.Len(t, refs, 1)
				assert.Equal(t, activeUID, refs[0].UID)
				assert.Equal(t, "Active", refs[0].Stage)
			},
		},
		{
			name:        "both filters union, and a project matching both appears once",
			messageData: []byte(`{"stages":["Formation - Engaged"],"uids":["` + formingUID + `","` + activeUID + `"]}`),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("ListAllProjectsBase", mock.Anything).Return(allProjects, nil)
			},
			validate: func(t *testing.T, mockRepo *domain.MockProjectRepository, refs []events.ProjectRef) {
				// Neither UID is read again: the stage-matched one is already in the
				// reply, and the other was decoded by the same scan.
				mockRepo.AssertNotCalled(t, "GetProjectBase", mock.Anything, formingUID)
				mockRepo.AssertNotCalled(t, "GetProjectBase", mock.Anything, activeUID)
				assert.Len(t, refs, 2)
				assert.Equal(t, formingUID, refs[0].UID)
				assert.Equal(t, activeUID, refs[1].UID)
			},
		},
		{
			name:        "a uid at an unwanted stage is served from the scan, not read again",
			messageData: []byte(`{"stages":["Formation - Engaged"],"uids":["` + prospectUID + `"]}`),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("ListAllProjectsBase", mock.Anything).Return(allProjects, nil)
			},
			validate: func(t *testing.T, mockRepo *domain.MockProjectRepository, refs []events.ProjectRef) {
				mockRepo.AssertNotCalled(t, "GetProjectBase", mock.Anything, prospectUID)
				assert.Len(t, refs, 2)
				assert.Equal(t, formingUID, refs[0].UID)
				assert.Equal(t, prospectUID, refs[1].UID)
				assert.Equal(t, "Prospect", refs[1].Stage)
			},
		},
		{
			name:        "a uid naming no project is skipped rather than failing the request",
			messageData: []byte(`{"uids":["` + deletedUID + `","` + activeUID + `"]}`),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectBase", mock.Anything, deletedUID).Return(nil, domain.ErrProjectNotFound)
				mockRepo.On("GetProjectBase", mock.Anything, activeUID).Return(allProjects[0], nil)
			},
			validate: func(t *testing.T, mockRepo *domain.MockProjectRepository, refs []events.ProjectRef) {
				assert.Len(t, refs, 1)
				assert.Equal(t, activeUID, refs[0].UID)
			},
		},
		{
			name:        "a stage nothing is at replies with an empty list, not null",
			messageData: []byte(`{"stages":["Archived"]}`),
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("ListAllProjectsBase", mock.Anything).Return(allProjects, nil)
			},
			validate: func(t *testing.T, mockRepo *domain.MockProjectRepository, refs []events.ProjectRef) {
				assert.Empty(t, refs)
			},
		},
		{
			name:        "a request with no filter is refused rather than answered with everything",
			messageData: []byte(`{}`),
			setupMocks:  func(mockRepo *domain.MockProjectRepository) {},
			expectedErr: true,
		},
		{
			name:        "malformed request body",
			messageData: []byte(`not json`),
			setupMocks:  func(mockRepo *domain.MockProjectRepository) {},
			expectedErr: true,
		},
		{
			name:        "malformed uid fails the request",
			messageData: []byte(`{"uids":["not-a-uuid"]}`),
			setupMocks:  func(mockRepo *domain.MockProjectRepository) {},
			expectedErr: true,
		},
		{
			name:        "malformed uid fails before the stage filter scans the store",
			messageData: []byte(`{"stages":["Formation - Engaged"],"uids":["not-a-uuid"]}`),
			setupMocks:  func(mockRepo *domain.MockProjectRepository) {},
			expectedErr: true,
			validateErr: func(t *testing.T, mockRepo *domain.MockProjectRepository) {
				mockRepo.AssertNotCalled(t, "ListAllProjectsBase", mock.Anything)
			},
		},
		{
			name:        "a uid list over the cap is refused before any read",
			messageData: overCapRequest,
			setupMocks:  func(mockRepo *domain.MockProjectRepository) {},
			expectedErr: true,
			validateErr: func(t *testing.T, mockRepo *domain.MockProjectRepository) {
				mockRepo.AssertNotCalled(t, "GetProjectBase", mock.Anything, mock.Anything)
				mockRepo.AssertNotCalled(t, "ListAllProjectsBase", mock.Anything)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, _, _ := setupServiceForTesting()
			tt.setupMocks(mockRepo)

			mockMsg := newMockMessage(constants.ProjectListProjectsSubject, tt.messageData)

			response, err := service.HandleProjectListProjects(ctx, mockMsg)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, response)
				if tt.validateErr != nil {
					tt.validateErr(t, mockRepo)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)

				var refs []events.ProjectRef
				assert.NoError(t, json.Unmarshal(response, &refs))
				assert.NotEqual(t, "null", string(response))

				if tt.validate != nil {
					tt.validate(t, mockRepo, refs)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestProjectsService_MessageHandling_ErrorCases(t *testing.T) {

	ctx := context.Background()

	tests := []struct {
		name         string
		setupService func() *ProjectsService
		subject      string
		messageData  []byte
		description  string
	}{
		{
			name: "service not ready",
			setupService: func() *ProjectsService {
				return &ProjectsService{
					ProjectRepository: nil,
					MessageBuilder:    nil,
					Auth:              &auth.MockJWTAuth{},
				}
			},
			subject:     constants.ProjectGetNameSubject,
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			description: "should handle service not ready gracefully",
		},
		{
			name: "repository error",
			setupService: func() *ProjectsService {
				mockRepo := &domain.MockProjectRepository{}
				mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
					nil, domain.ErrInternal,
				)
				mockBuilder := &domain.MockMessageBuilder{}
				resolver := NewUserResolver(&domain.MockUserReader{})
				return &ProjectsService{
					ProjectRepository:  mockRepo,
					DocumentRepository: &domain.MockDocumentRepository{},
					LinkRepository:     &domain.MockLinkRepository{},
					FolderRepository:   &domain.MockFolderRepository{},
					MessageBuilder:     mockBuilder,
					UserReader:         &domain.MockUserReader{},
					Resolver:           resolver,
					Dispatcher:         NewNotificationDispatcher(mockBuilder, resolver, false, false),
					Auth:               &auth.MockJWTAuth{},
				}
			},
			subject:     constants.ProjectGetNameSubject,
			messageData: []byte("01234567-89ab-cdef-0123-456789abcdef"),
			description: "should handle repository errors gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := tt.setupService()

			mockMsg := newMockMessage(tt.subject, tt.messageData)
			mockMsg.On("Respond", mock.Anything).Return(nil)

			// Should not panic
			assert.NotPanics(t, func() {
				service.HandleMessage(ctx, mockMsg)
			})

			if mockRepo, ok := service.ProjectRepository.(*domain.MockProjectRepository); ok {
				mockRepo.AssertExpectations(t)
			}
		})
	}
}

func TestProjectsService_MessageHandling_Integration(t *testing.T) {

	ctx := context.Background()

	t.Run("end to end message handling", func(t *testing.T) {
		service, mockRepo, mockBuilder, mockAuth := setupServiceForTesting()

		// Setup expectations for a complete flow
		now := time.Now()
		mockRepo.On("GetProjectBase", mock.Anything, "01234567-89ab-cdef-0123-456789abcdef").Return(
			&models.ProjectBase{
				UID:       "integration-test-uid",
				Name:      "Integration Test Project",
				Slug:      "integration-test",
				CreatedAt: &now,
				UpdatedAt: &now,
			},
			nil,
		)

		// Create message and set up response expectation
		messageData := []byte("01234567-89ab-cdef-0123-456789abcdef")
		mockMsg := newMockMessage(constants.ProjectGetNameSubject, messageData)

		// Expect a response with the project name
		mockMsg.On("Respond", mock.MatchedBy(func(data []byte) bool {
			return string(data) == "Integration Test Project"
		})).Return(nil)

		// Execute
		service.HandleMessage(ctx, mockMsg)

		// Verify all expectations
		mockRepo.AssertExpectations(t)
		mockBuilder.AssertExpectations(t)
		mockAuth.AssertExpectations(t)
		mockMsg.AssertExpectations(t)
	})
}
