// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"testing"
	"time"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProjectsService_GetProjects(t *testing.T) {
	tests := []struct {
		name        string
		setupMocks  func(*domain.MockProjectRepository, *domain.MockMessageBuilder)
		expectedLen int
		wantErr     bool
		expectedErr error
	}{
		{
			name: "successful get all projects",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				now := time.Now()
				mockRepo.On("ListAllProjects", mock.Anything).Return(
					[]*models.ProjectBase{
						{
							UID:         "project-1",
							Slug:        "test-project-1",
							Name:        "Test Project 1",
							Description: "Description 1",
							Public:      true,
							CreatedAt:   &now,
							UpdatedAt:   &now,
						},
						{
							UID:         "project-2",
							Slug:        "test-project-2",
							Name:        "Test Project 2",
							Description: "Description 2",
							Public:      false,
							CreatedAt:   &now,
							UpdatedAt:   &now,
						},
					},
					[]*models.ProjectSettings{
						{
							UID:              "project-1",
							MissionStatement: "Mission 1",
							Writers: []models.UserInfo{
								{Username: "writer1", Name: "Writer One", Email: "writer1@example.com", Avatar: ""},
							},
							CreatedAt: &now,
							UpdatedAt: &now,
						},
					},
					nil,
				)
			},
			expectedLen: 2,
			wantErr:     false,
		},
		{
			name: "service not ready",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				// Don't set up repository - will make service not ready
			},
			expectedLen: 0,
			wantErr:     true,
			expectedErr: domain.ErrServiceUnavailable,
		},
		{
			name: "repository error",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("ListAllProjects", mock.Anything).Return(
					nil, nil, domain.ErrInternal,
				)
			},
			expectedLen: 0,
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name: "empty projects list",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("ListAllProjects", mock.Anything).Return(
					[]*models.ProjectBase{},
					[]*models.ProjectSettings{},
					nil,
				)
			},
			expectedLen: 0,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, mockBuilder, mockAuth := setupServiceForTesting()

			if tt.name == "service not ready" {
				service.ProjectRepository = nil
			}

			tt.setupMocks(mockRepo, mockBuilder)

			result, err := service.GetProjects(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.expectedLen)
			}

			mockRepo.AssertExpectations(t)
			mockBuilder.AssertExpectations(t)
			mockAuth.AssertExpectations(t)
		})
	}
}

func TestProjectsService_CreateProject(t *testing.T) {
	tests := []struct {
		name        string
		payload     *projsvc.CreateProjectPayload
		setupMocks  func(*domain.MockProjectRepository, *domain.MockMessageBuilder)
		wantErr     bool
		expectedErr error
		validate    func(*testing.T, *projsvc.ProjectFull)
	}{
		{
			name: "successful project creation",
			payload: &projsvc.CreateProjectPayload{
				Slug:        "test-project",
				Name:        "Test Project",
				Description: "Test Description",
				Public:      misc.BoolPtr(true),
				Stage:       misc.StringPtr("incubating"),
				Category:    misc.StringPtr("foundation"),
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("ProjectSlugExists", mock.Anything, "test-project").Return(false, nil)
				mockRepo.On("CreateProject", mock.Anything, mock.AnythingOfType("*models.ProjectBase"), mock.AnythingOfType("*models.ProjectSettings")).Return(nil)
				mockBuilder.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("types.IndexerMessageEnvelope"), mock.AnythingOfType("bool")).Return(nil).Times(2)
				mockBuilder.On("SendAccessMessage", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("models.ProjectAccessMessage"), mock.AnythingOfType("bool")).Return(nil)
			},
			wantErr: false,
			validate: func(t *testing.T, result *projsvc.ProjectFull) {
				assert.NotNil(t, result)
				assert.NotNil(t, result.UID)
				assert.Equal(t, "test-project", *result.Slug)
				assert.Equal(t, "Test Project", *result.Name)
				assert.Equal(t, "Test Description", *result.Description)
				assert.Equal(t, true, *result.Public)
			},
		},
		{
			name: "service not ready",
			payload: &projsvc.CreateProjectPayload{
				Slug: "test-project",
				Name: "Test Project",
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				// Service will not be ready
			},
			wantErr:     true,
			expectedErr: domain.ErrServiceUnavailable,
		},
		{
			name: "invalid parent UID",
			payload: &projsvc.CreateProjectPayload{
				Slug:      "test-project",
				Name:      "Test Project",
				ParentUID: "invalid-uuid",
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				// Slug check happens first, then validation
				mockRepo.On("ProjectSlugExists", mock.Anything, "test-project").Return(false, nil)
			},
			wantErr:     true,
			expectedErr: domain.ErrValidationFailed,
		},
		{
			name: "slug already exists",
			payload: &projsvc.CreateProjectPayload{
				Slug: "existing-project",
				Name: "Test Project",
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("ProjectSlugExists", mock.Anything, "existing-project").Return(true, nil)
			},
			wantErr:     true,
			expectedErr: domain.ErrProjectSlugExists,
		},
		{
			name: "repository creation error",
			payload: &projsvc.CreateProjectPayload{
				Slug: "test-project",
				Name: "Test Project",
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("ProjectSlugExists", mock.Anything, "test-project").Return(false, nil)
				mockRepo.On("CreateProject", mock.Anything, mock.AnythingOfType("*models.ProjectBase"), mock.AnythingOfType("*models.ProjectSettings")).Return(domain.ErrInternal)
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, mockBuilder, mockAuth := setupServiceForTesting()

			if tt.name == "service not ready" {
				service.ProjectRepository = nil
			}

			tt.setupMocks(mockRepo, mockBuilder)

			result, err := service.CreateProject(context.Background(), tt.payload)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}

			mockRepo.AssertExpectations(t)
			mockBuilder.AssertExpectations(t)
			mockAuth.AssertExpectations(t)
		})
	}
}

func TestProjectsService_GetOneProjectBase(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		payload     *projsvc.GetOneProjectBasePayload
		setupMocks  func(*domain.MockProjectRepository, *domain.MockMessageBuilder)
		wantErr     bool
		expectedErr error
		validate    func(*testing.T, *projsvc.GetOneProjectBaseResult)
	}{
		{
			name: "successful get project base",
			payload: &projsvc.GetOneProjectBasePayload{
				UID: misc.StringPtr("test-project-uid"),
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("GetProjectBaseWithRevision", mock.Anything, "test-project-uid").Return(
					&models.ProjectBase{
						UID:         "test-project-uid",
						Slug:        "test-project",
						Name:        "Test Project",
						Description: "Test Description",
						Public:      true,
						CreatedAt:   &now,
						UpdatedAt:   &now,
					},
					uint64(123),
					nil,
				)
			},
			wantErr: false,
			validate: func(t *testing.T, result *projsvc.GetOneProjectBaseResult) {
				assert.NotNil(t, result)
				assert.NotNil(t, result.Project)
				assert.Equal(t, "test-project-uid", *result.Project.UID)
				assert.Equal(t, "test-project", *result.Project.Slug)
				assert.NotNil(t, result.Etag)
				assert.Equal(t, "123", *result.Etag)
			},
		},
		{
			name: "project not found",
			payload: &projsvc.GetOneProjectBasePayload{
				UID: misc.StringPtr("non-existent-uid"),
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("GetProjectBaseWithRevision", mock.Anything, "non-existent-uid").Return(
					nil, uint64(0), domain.ErrProjectNotFound,
				)
			},
			wantErr:     true,
			expectedErr: domain.ErrProjectNotFound,
		},
		{
			name:    "nil payload",
			payload: nil,
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				// No repo calls expected
			},
			wantErr:     true,
			expectedErr: domain.ErrValidationFailed,
		},
		{
			name: "empty UID",
			payload: &projsvc.GetOneProjectBasePayload{
				UID: misc.StringPtr(""),
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				// Repository is called even with empty UID, but returns error
				mockRepo.On("GetProjectBaseWithRevision", mock.Anything, "").Return(
					nil, uint64(0), domain.ErrValidationFailed,
				)
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, mockBuilder, mockAuth := setupServiceForTesting()
			tt.setupMocks(mockRepo, mockBuilder)

			result, err := service.GetOneProjectBase(context.Background(), tt.payload)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}

			mockRepo.AssertExpectations(t)
			mockBuilder.AssertExpectations(t)
			mockAuth.AssertExpectations(t)
		})
	}
}

func TestProjectsService_DeleteProject(t *testing.T) {
	tests := []struct {
		name        string
		payload     *projsvc.DeleteProjectPayload
		setupMocks  func(*domain.MockProjectRepository, *domain.MockMessageBuilder)
		wantErr     bool
		expectedErr error
	}{
		{
			name: "successful deletion - project with Crowdfunding funding model",
			payload: &projsvc.DeleteProjectPayload{
				UID:     misc.StringPtr("test-project-uid"),
				IfMatch: misc.StringPtr("123"),
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				// Project has Crowdfunding in funding model - deletion allowed
				mockRepo.On("GetProjectBase", mock.Anything, "test-project-uid").Return(
					&models.ProjectBase{
						UID:          "test-project-uid",
						Slug:         "test-project",
						Name:         "Test Project",
						FundingModel: []string{"Crowdfunding"},
					},
					nil,
				)
				mockRepo.On("DeleteProject", mock.Anything, "test-project-uid", uint64(123)).Return(nil)
				mockBuilder.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), "test-project-uid", mock.AnythingOfType("bool")).Return(nil).Times(2)
				mockBuilder.On("SendAccessMessage", mock.Anything, mock.AnythingOfType("string"), "test-project-uid", mock.AnythingOfType("bool")).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "deletion rejected - project with Crowdfunding and other funding models",
			payload: &projsvc.DeleteProjectPayload{
				UID:     misc.StringPtr("test-project-uid"),
				IfMatch: misc.StringPtr("123"),
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				// Project has Crowdfunding plus other models - deletion NOT allowed (must be ONLY Crowdfunding)
				mockRepo.On("GetProjectBase", mock.Anything, "test-project-uid").Return(
					&models.ProjectBase{
						UID:          "test-project-uid",
						Slug:         "test-project",
						Name:         "Test Project",
						FundingModel: []string{"Membership", "Crowdfunding", "Alternate Funding"},
					},
					nil,
				)
			},
			wantErr:     true,
			expectedErr: domain.ErrCannotDeleteNonCrowdfundingProject,
		},
		{
			name: "deletion rejected - project without Crowdfunding funding model",
			payload: &projsvc.DeleteProjectPayload{
				UID:     misc.StringPtr("test-project-uid"),
				IfMatch: misc.StringPtr("123"),
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				// Project has only Membership - deletion not allowed
				mockRepo.On("GetProjectBase", mock.Anything, "test-project-uid").Return(
					&models.ProjectBase{
						UID:          "test-project-uid",
						Slug:         "test-project",
						Name:         "Test Project",
						FundingModel: []string{"Membership"},
					},
					nil,
				)
			},
			wantErr:     true,
			expectedErr: domain.ErrCannotDeleteNonCrowdfundingProject,
		},
		{
			name: "deletion rejected - project with empty funding model",
			payload: &projsvc.DeleteProjectPayload{
				UID:     misc.StringPtr("test-project-uid"),
				IfMatch: misc.StringPtr("123"),
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				// Project has empty funding model - deletion not allowed
				mockRepo.On("GetProjectBase", mock.Anything, "test-project-uid").Return(
					&models.ProjectBase{
						UID:          "test-project-uid",
						Slug:         "test-project",
						Name:         "Test Project",
						FundingModel: []string{},
					},
					nil,
				)
			},
			wantErr:     true,
			expectedErr: domain.ErrCannotDeleteNonCrowdfundingProject,
		},
		{
			name: "deletion rejected - project with nil funding model",
			payload: &projsvc.DeleteProjectPayload{
				UID:     misc.StringPtr("test-project-uid"),
				IfMatch: misc.StringPtr("123"),
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				// Project has nil funding model - deletion not allowed
				mockRepo.On("GetProjectBase", mock.Anything, "test-project-uid").Return(
					&models.ProjectBase{
						UID:          "test-project-uid",
						Slug:         "test-project",
						Name:         "Test Project",
						FundingModel: nil,
					},
					nil,
				)
			},
			wantErr:     true,
			expectedErr: domain.ErrCannotDeleteNonCrowdfundingProject,
		},
		{
			name: "project not found",
			payload: &projsvc.DeleteProjectPayload{
				UID:     misc.StringPtr("non-existent-uid"),
				IfMatch: misc.StringPtr("123"),
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("GetProjectBase", mock.Anything, "non-existent-uid").Return(
					nil, domain.ErrProjectNotFound,
				)
			},
			wantErr:     true,
			expectedErr: domain.ErrProjectNotFound,
		},
		{
			name: "service not ready",
			payload: &projsvc.DeleteProjectPayload{
				UID:     misc.StringPtr("test-project-uid"),
				IfMatch: misc.StringPtr("123"),
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				// Service will not be ready
			},
			wantErr:     true,
			expectedErr: domain.ErrServiceUnavailable,
		},
		{
			name:    "nil payload",
			payload: nil,
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				// No repo calls expected
			},
			wantErr:     true,
			expectedErr: domain.ErrValidationFailed,
		},
		{
			name: "revision mismatch",
			payload: &projsvc.DeleteProjectPayload{
				UID:     misc.StringPtr("test-project-uid"),
				IfMatch: misc.StringPtr("123"),
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("GetProjectBase", mock.Anything, "test-project-uid").Return(
					&models.ProjectBase{
						UID:          "test-project-uid",
						Slug:         "test-project",
						Name:         "Test Project",
						FundingModel: []string{"Crowdfunding"},
					},
					nil,
				)
				mockRepo.On("DeleteProject", mock.Anything, "test-project-uid", uint64(123)).Return(domain.ErrRevisionMismatch)
			},
			wantErr:     true,
			expectedErr: domain.ErrRevisionMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, mockBuilder, mockAuth := setupServiceForTesting()

			if tt.name == "service not ready" {
				service.ProjectRepository = nil
			}

			tt.setupMocks(mockRepo, mockBuilder)

			err := service.DeleteProject(context.Background(), tt.payload)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err)
				}
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
			mockBuilder.AssertExpectations(t)
			mockAuth.AssertExpectations(t)
		})
	}
}

func TestProjectsService_ProjectValidation(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T, *ProjectsService)
	}{
		{
			name: "service validates project creation",
			testFunc: func(t *testing.T, service *ProjectsService) {
				// Test that the service properly validates inputs
				// This is a placeholder for validation logic tests
				assert.NotNil(t, service)
			},
		},
		{
			name: "service handles concurrent operations",
			testFunc: func(t *testing.T, service *ProjectsService) {
				// Test that the service can handle multiple concurrent operations
				// This is a placeholder for concurrency tests
				assert.NotNil(t, service)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, mockBuilder, _ := setupServiceForTesting()
			// Setup basic mocks to ensure service is ready
			_ = mockRepo
			_ = mockBuilder

			tt.testFunc(t, service)
		})
	}
}

// TestIsCrowdfundingOnly tests the helper function for strict Crowdfunding-only validation.
func TestIsCrowdfundingOnly(t *testing.T) {
	tests := []struct {
		name          string
		fundingModels []string
		want          bool
	}{
		{
			name:          "exactly Crowdfunding only - valid for deletion",
			fundingModels: []string{"Crowdfunding"},
			want:          true,
		},
		{
			name:          "Crowdfunding with other models - invalid",
			fundingModels: []string{"Membership", "Crowdfunding", "Alternate Funding"},
			want:          false,
		},
		{
			name:          "Crowdfunding with one other model - invalid",
			fundingModels: []string{"Crowdfunding", "Membership"},
			want:          false,
		},
		{
			name:          "only Membership - invalid",
			fundingModels: []string{"Membership"},
			want:          false,
		},
		{
			name:          "multiple models without Crowdfunding - invalid",
			fundingModels: []string{"Membership", "Alternate Funding"},
			want:          false,
		},
		{
			name:          "empty array - invalid",
			fundingModels: []string{},
			want:          false,
		},
		{
			name:          "nil array - invalid",
			fundingModels: nil,
			want:          false,
		},
		{
			name:          "case sensitive - lowercase crowdfunding invalid",
			fundingModels: []string{"crowdfunding"},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCrowdfundingOnly(tt.fundingModels)
			assert.Equal(t, tt.want, got, "isCrowdfundingOnly(%v) = %v, want %v", tt.fundingModels, got, tt.want)
		})
	}
}
