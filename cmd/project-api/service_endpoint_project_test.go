// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

// setupAPI creates a new ProjectsAPI with mocked dependencies.
func setupAPI() (*ProjectsAPI, *domain.MockProjectRepository, *domain.MockMessageBuilder) {
	if os.Getenv("DEBUG") == "true" {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	mockRepo := &domain.MockProjectRepository{}
	mockMessageBuilder := &domain.MockMessageBuilder{}
	mockJwtAuth := &auth.MockJWTAuth{}

	projectService := &service.ProjectsService{
		ProjectRepository: mockRepo,
		MessageBuilder:    mockMessageBuilder,
		Auth:              mockJwtAuth,
	}

	api := &ProjectsAPI{
		service: projectService,
	}

	return api, mockRepo, mockMessageBuilder
}

func TestGetProjects(t *testing.T) {
	tests := []struct {
		name           string
		payload        *projsvc.GetProjectsPayload
		setupMocks     func(*domain.MockProjectRepository, *domain.MockMessageBuilder)
		expectedError  bool
		expectedLength int
	}{
		{
			name:    "success with projects",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockMsg *domain.MockMessageBuilder) {
				// Create mock projects
				now := time.Now()
				projectsBase := []*models.ProjectBase{
					{
						UID:         "project-1",
						Slug:        "test-1",
						Name:        "Test Project 1",
						Description: "Test 1",
						Public:      true,
						CreatedAt:   &now,
						UpdatedAt:   &now,
					},
				}
				projectsSettings := []*models.ProjectSettings{
					{
						UID:      "project-1",
						Writers:  []string{"user2"},
						Auditors: []string{"user1"},
					},
				}

				mockRepo.On("ListAllProjects", mock.Anything).Return(projectsBase, projectsSettings, nil)
			},
			expectedError:  false,
			expectedLength: 1,
		},
		{
			name:    "success with no projects",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockMsg *domain.MockMessageBuilder) {
				mockRepo.On("ListAllProjects", mock.Anything).Return([]*models.ProjectBase{}, []*models.ProjectSettings{}, nil)
			},
			expectedError:  false,
			expectedLength: 0,
		},
		{
			name:    "repository error",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockMsg *domain.MockMessageBuilder) {
				mockRepo.On("ListAllProjects", mock.Anything).Return(nil, nil, domain.ErrInternal)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, mockRepo, mockMsg := setupAPI()
			tt.setupMocks(mockRepo, mockMsg)

			result, err := api.GetProjects(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Len(t, result.Projects, tt.expectedLength)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestCreateProject(t *testing.T) {
	tests := []struct {
		name          string
		payload       *projsvc.CreateProjectPayload
		setupMocks    func(*domain.MockProjectRepository, *domain.MockMessageBuilder)
		expectedError bool
	}{
		{
			name: "success",
			payload: &projsvc.CreateProjectPayload{
				Slug:        "test-project",
				Name:        "Test Project",
				Description: "Test description",
				Public:      misc.BoolPtr(true),
				ParentUID:   "787620d0-d7de-449a-b0bf-9d28b13da818",
				Writers:     []string{"user1", "user2"},
				Auditors:    []string{"user3", "user4"},
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockMsg *domain.MockMessageBuilder) {
				// Mock parent project exists (for ParentUID validation)
				mockRepo.On("ProjectExists", mock.Anything, "787620d0-d7de-449a-b0bf-9d28b13da818").Return(true, nil)
				// Mock slug doesn't exist
				mockRepo.On("ProjectSlugExists", mock.Anything, "test-project").Return(false, nil)
				// Mock successful project creation
				mockRepo.On("CreateProject", mock.Anything, mock.AnythingOfType("*models.ProjectBase"), mock.AnythingOfType("*models.ProjectSettings")).Return(nil)
				// Mock message sending
				mockMsg.On("SendIndexProject", mock.Anything, models.ActionCreated, mock.Anything).Return(nil)
				mockMsg.On("SendUpdateAccessProject", mock.Anything, mock.Anything).Return(nil)
				mockMsg.On("SendIndexProjectSettings", mock.Anything, models.ActionCreated, mock.Anything).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "slug already exists",
			payload: &projsvc.CreateProjectPayload{
				Slug:        "existing-project",
				Name:        "Test Project",
				Description: "Test description",
				Public:      misc.BoolPtr(true),
				ParentUID:   "787620d0-d7de-449a-b0bf-9d28b13da818",
				Auditors:    []string{"user1"},
				Writers:     []string{"user2"},
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockMsg *domain.MockMessageBuilder) {
				// Mock slug exists
				mockRepo.On("ProjectSlugExists", mock.Anything, "existing-project").Return(true, nil)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, mockRepo, mockMsg := setupAPI()
			tt.setupMocks(mockRepo, mockMsg)

			result, err := api.CreateProject(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.payload.Slug, *result.Slug)
				assert.Equal(t, tt.payload.Name, *result.Name)
			}

			mockRepo.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}
}
