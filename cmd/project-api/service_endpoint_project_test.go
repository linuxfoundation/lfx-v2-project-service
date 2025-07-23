// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// setupService creates a new ProjectsService with mocked external service APIs.
func setupService() *ProjectsService {
	if os.Getenv("DEBUG") == "true" {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	service := &ProjectsService{
		lfxEnvironment: constants.LFXEnvironmentDev,
		natsConn:       &nats.MockNATSConn{},
		projectsKV:     &nats.MockKeyValue{},
		auth:           &MockJwtAuth{},
	}

	return service
}

func TestGetProjects(t *testing.T) {
	tests := []struct {
		name           string
		payload        *projsvc.GetProjectsPayload
		setupMocks     func(*nats.MockKeyValue)
		expectedError  bool
		expectedResult *projsvc.GetProjectsResult
	}{
		{
			name:    "success with projects",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockKV *nats.MockKeyValue) {
				// Create mock key lister with project keys
				mockLister := nats.NewMockKeyLister([]string{"project-1", "project-2"})
				mockKV.On("ListKeys", mock.Anything).Return(mockLister, nil)

				// Mock project entries
				project1Data := `{"uid":"project-1","slug":"test-1","name":"Test Project 1","description":"Test 1","public":true,"parent_uid":"","auditors":["user1"],"writers":["user2"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				project2Data := `{"uid":"project-2","slug":"test-2","name":"Test Project 2","description":"Test 2","public":false,"parent_uid":"parent-uid","auditors":["user2"],"writers":["user3"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`

				mockKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte(project1Data), 123), nil)
				mockKV.On("Get", mock.Anything, "project-2").Return(nats.NewMockKeyValueEntry([]byte(project2Data), 123), nil)
			},
			expectedError: false,
			expectedResult: &projsvc.GetProjectsResult{
				Projects: []*projsvc.Project{
					{
						ID:          stringPtr("project-1"),
						Slug:        stringPtr("test-1"),
						Name:        stringPtr("Test Project 1"),
						Description: stringPtr("Test 1"),
						Public:      boolPtr(true),
						ParentUID:   stringPtr(""),
						Auditors:    []string{"user1"},
						Writers:     []string{"user2"},
					},
					{
						ID:          stringPtr("project-2"),
						Slug:        stringPtr("test-2"),
						Name:        stringPtr("Test Project 2"),
						Description: stringPtr("Test 2"),
						Public:      boolPtr(false),
						ParentUID:   stringPtr("parent-uid"),
						Auditors:    []string{"user2"},
						Writers:     []string{"user3"},
					},
				},
			},
		},
		{
			name:    "success with no projects",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockKV *nats.MockKeyValue) {
				mockLister := nats.NewMockKeyLister([]string{})
				mockKV.On("ListKeys", mock.Anything).Return(mockLister, nil)
			},
			expectedError: false,
			expectedResult: &projsvc.GetProjectsResult{
				Projects: []*projsvc.Project{},
			},
		},
		{
			name:    "error listing keys",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockKV *nats.MockKeyValue) {
				mockKV.On("ListKeys", mock.Anything).Return(&nats.MockKeyLister{}, assert.AnError)
			},
			expectedError: true,
		},
		{
			name:    "error getting project",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockKV *nats.MockKeyValue) {
				mockLister := nats.NewMockKeyLister([]string{"project-1"})
				mockKV.On("ListKeys", mock.Anything).Return(mockLister, nil)
				mockKV.On("Get", mock.Anything, "project-1").Return(&nats.MockKeyValueEntry{}, assert.AnError)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			tt.setupMocks(service.projectsKV.(*nats.MockKeyValue))

			result, err := service.GetProjects(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, len(tt.expectedResult.Projects), len(result.Projects))

				// Compare project details
				for i, expectedProject := range tt.expectedResult.Projects {
					if i < len(result.Projects) {
						actualProject := result.Projects[i]
						assert.Equal(t, *expectedProject.ID, *actualProject.ID)
						assert.Equal(t, *expectedProject.Slug, *actualProject.Slug)
						assert.Equal(t, *expectedProject.Name, *actualProject.Name)
						if expectedProject.Description != nil {
							assert.Equal(t, *expectedProject.Description, *actualProject.Description)
						}
						assert.Equal(t, expectedProject.Public, actualProject.Public)
						assert.Equal(t, expectedProject.ParentUID, actualProject.ParentUID)
						assert.Equal(t, expectedProject.Auditors, actualProject.Auditors)
						assert.Equal(t, expectedProject.Writers, actualProject.Writers)
					}
				}
			}
		})
	}
}

func TestCreateProject(t *testing.T) {
	tests := []struct {
		name          string
		payload       *projsvc.CreateProjectPayload
		setupMocks    func(*ProjectsService)
		expectedError bool
	}{
		{
			name: "success",
			payload: &projsvc.CreateProjectPayload{
				Slug:        "test-project",
				Name:        "Test Project",
				Description: "Test description",
				Public:      boolPtr(true),
				ParentUID:   stringPtr(""),
				Auditors:    []string{"user1", "user2"},
				Writers:     []string{"user3", "user4"},
			},
			setupMocks: func(service *ProjectsService) {
				mockNats := service.natsConn.(*nats.MockNATSConn)
				mockKV := service.projectsKV.(*nats.MockKeyValue)
				// Mock successful slug mapping creation
				mockKV.On("Put", mock.Anything, "slug/test-project", mock.Anything).Return(uint64(1), nil)
				// Mock successful project creation
				mockKV.On("Put", mock.Anything, mock.Anything, mock.Anything).Return(uint64(1), nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.IndexProjectSubject), mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.UpdateAccessProjectSubject), mock.Anything).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "invalid parent UID",
			payload: &projsvc.CreateProjectPayload{
				Slug:        "test-project",
				Name:        "Test Project",
				Description: "Test description",
				Public:      boolPtr(true),
				ParentUID:   stringPtr("invalid-parent-uid"),
				Auditors:    []string{"user1"},
				Writers:     []string{"user2"},
			},
			setupMocks:    func(_ *ProjectsService) {},
			expectedError: true,
		},
		{
			name: "parent project not found",
			payload: &projsvc.CreateProjectPayload{
				Slug:        "test-project",
				Name:        "Test Project",
				Description: "Test description",
				Public:      boolPtr(true),
				ParentUID:   stringPtr("787620d0-d7de-449a-b0bf-9d28b13da818"),
				Auditors:    []string{"user1"},
				Writers:     []string{"user2"},
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.projectsKV.(*nats.MockKeyValue)
				mockKV.On("Get", mock.Anything, "787620d0-d7de-449a-b0bf-9d28b13da818").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
			},
			expectedError: true,
		},
		{
			name: "slug already exists",
			payload: &projsvc.CreateProjectPayload{
				Slug:        "existing-project",
				Name:        "Test Project",
				Description: "Test description",
				Public:      boolPtr(true),
				ParentUID:   stringPtr(""),
				Auditors:    []string{"user1"},
				Writers:     []string{"user2"},
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.projectsKV.(*nats.MockKeyValue)
				mockKV.On("Put", mock.Anything, "slug/existing-project", mock.Anything).Return(uint64(1), jetstream.ErrKeyExists)
			},
			expectedError: true,
		},
		{
			name: "error creating slug mapping",
			payload: &projsvc.CreateProjectPayload{
				Slug:        "test-project",
				Name:        "Test Project",
				Description: "Test description",
				Public:      boolPtr(true),
				ParentUID:   stringPtr(""),
				Auditors:    []string{"user1"},
				Writers:     []string{"user2"},
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.projectsKV.(*nats.MockKeyValue)
				mockKV.On("Put", mock.Anything, "slug/test-project", mock.Anything).Return(uint64(1), assert.AnError)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			tt.setupMocks(service)

			result, err := service.CreateProject(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.payload.Slug, *result.Slug)
				assert.Equal(t, tt.payload.Name, *result.Name)
				assert.Equal(t, tt.payload.Description, *result.Description)
				assert.Equal(t, tt.payload.Public, result.Public)
				assert.Equal(t, tt.payload.ParentUID, result.ParentUID)
				assert.Equal(t, tt.payload.Auditors, result.Auditors)
				assert.Equal(t, tt.payload.Writers, result.Writers)
				assert.NotEmpty(t, *result.ID)
			}
		})
	}
}

func TestGetOneProject(t *testing.T) {
	tests := []struct {
		name          string
		payload       *projsvc.GetOneProjectPayload
		setupMocks    func(*nats.MockKeyValue)
		expectedError bool
		expectedID    string
	}{
		{
			name: "success",
			payload: &projsvc.GetOneProjectPayload{
				ID: stringPtr("project-1"),
			},
			setupMocks: func(mockKV *nats.MockKeyValue) {
				projectData := `{"uid":"project-1","slug":"test-1","name":"Test Project","description":"Test description","public":true,"parent_uid":"","auditors":["user1"],"writers":["user2"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				mockKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte(projectData), 123), nil)
			},
			expectedError: false,
			expectedID:    "project-1",
		},
		{
			name: "project not found",
			payload: &projsvc.GetOneProjectPayload{
				ID: stringPtr("nonexistent"),
			},
			setupMocks: func(mockKV *nats.MockKeyValue) {
				mockKV.On("Get", mock.Anything, "nonexistent").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
			},
			expectedError: true,
		},
		{
			name: "error getting project",
			payload: &projsvc.GetOneProjectPayload{
				ID: stringPtr("project-1"),
			},
			setupMocks: func(mockKV *nats.MockKeyValue) {
				mockKV.On("Get", mock.Anything, "project-1").Return(&nats.MockKeyValueEntry{}, assert.AnError)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			tt.setupMocks(service.projectsKV.(*nats.MockKeyValue))

			result, err := service.GetOneProject(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if assert.NotNil(t, result) {
					if assert.NotNil(t, result.Project) {
						assert.Equal(t, tt.expectedID, *result.Project.ID)
					}
				}
				if assert.NotNil(t, result.Etag) {
					assert.NotEmpty(t, *result.Etag)
				}
			}
		})
	}
}

func TestUpdateProject(t *testing.T) {
	tests := []struct {
		name          string
		payload       *projsvc.UpdateProjectPayload
		setupMocks    func(*ProjectsService)
		expectedError bool
	}{
		{
			name: "success",
			payload: &projsvc.UpdateProjectPayload{
				Etag:        stringPtr("1"),
				ID:          stringPtr("project-1"),
				Slug:        "updated-slug",
				Name:        "Updated Project",
				Description: "Updated description",
				Public:      boolPtr(true),
				ParentUID:   stringPtr(""),
				Auditors:    []string{"user1", "user2"},
				Writers:     []string{"user3", "user4"},
			},
			setupMocks: func(service *ProjectsService) {
				mockNats := service.natsConn.(*nats.MockNATSConn)
				mockKV := service.projectsKV.(*nats.MockKeyValue)
				// Mock getting existing project
				projectData := `{"uid":"project-1","slug":"old-slug","name":"Old Project","description":"Old description","public":false,"parent_uid":"parent-uid","auditors":["user1"],"writers":["user2"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				mockKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte(projectData), 1), nil)
				// Mock updating project
				mockKV.On("Update", mock.Anything, "project-1", mock.Anything, uint64(1)).Return(uint64(1), nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.IndexProjectSubject), mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.UpdateAccessProjectSubject), mock.Anything).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "etag header is invalid",
			payload: &projsvc.UpdateProjectPayload{
				ID:   stringPtr("project-1"),
				Etag: stringPtr("invalid"),
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.projectsKV.(*nats.MockKeyValue)
				mockKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte("test"), 1), nil)
				mockKV.On("Update", mock.Anything, "project-1", mock.Anything, uint64(1)).Return(uint64(1), assert.AnError)
			},
			expectedError: true,
		},
		{
			name: "invalid parent UID",
			payload: &projsvc.UpdateProjectPayload{
				ID:        stringPtr("project-1"),
				Slug:      "test",
				Name:      "Test",
				Public:    boolPtr(true),
				ParentUID: stringPtr("invalid-parent-uid"),
				Auditors:  []string{"user1"},
				Writers:   []string{"user2"},
			},
			setupMocks:    func(_ *ProjectsService) {},
			expectedError: true,
		},
		{
			name: "parent project not found",
			payload: &projsvc.UpdateProjectPayload{
				ID:        stringPtr("project-1"),
				Slug:      "test",
				Name:      "Test",
				Public:    boolPtr(true),
				ParentUID: stringPtr("787620d0-d7de-449a-b0bf-9d28b13da818"),
				Auditors:  []string{"user1"},
				Writers:   []string{"user2"},
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.projectsKV.(*nats.MockKeyValue)
				mockKV.On("Get", mock.Anything, "787620d0-d7de-449a-b0bf-9d28b13da818").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
			},
			expectedError: true,
		},
		{
			name: "project not found",
			payload: &projsvc.UpdateProjectPayload{
				ID:        stringPtr("nonexistent"),
				Slug:      "test",
				Name:      "Test",
				Public:    boolPtr(true),
				ParentUID: stringPtr(""),
				Auditors:  []string{"user1"},
				Writers:   []string{"user2"},
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.projectsKV.(*nats.MockKeyValue)
				mockKV.On("Get", mock.Anything, "nonexistent").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			tt.setupMocks(service)

			result, err := service.UpdateProject(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if assert.NotNil(t, result) {
					assert.Equal(t, tt.payload.Slug, *result.Slug)
					assert.Equal(t, tt.payload.Name, *result.Name)
					assert.Equal(t, tt.payload.Description, *result.Description)
					assert.Equal(t, tt.payload.Public, result.Public)
					assert.Equal(t, tt.payload.ParentUID, result.ParentUID)
					assert.Equal(t, tt.payload.Auditors, result.Auditors)
					assert.Equal(t, tt.payload.Writers, result.Writers)
				}
			}
		})
	}
}

func TestDeleteProject(t *testing.T) {
	tests := []struct {
		name          string
		payload       *projsvc.DeleteProjectPayload
		setupMocks    func(*ProjectsService)
		expectedError bool
	}{
		{
			name: "success",
			payload: &projsvc.DeleteProjectPayload{
				ID:   stringPtr("project-1"),
				Etag: stringPtr("1"),
			},
			setupMocks: func(service *ProjectsService) {
				mockNats := service.natsConn.(*nats.MockNATSConn)
				mockKV := service.projectsKV.(*nats.MockKeyValue)
				// Mock getting existing project
				projectData := `{"uid":"project-1","slug":"test","name":"Test Project","description":"Test","public":false,"parent_uid":"parent-uid","auditors":["user1"],"writers":["user2"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				mockKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte(projectData), 1), nil)
				// Mock deleting project
				mockKV.On("Delete", mock.Anything, "project-1", mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.IndexProjectSubject), mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.DeleteAllAccessSubject), mock.Anything).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "etag header is invalid",
			payload: &projsvc.DeleteProjectPayload{
				ID:   stringPtr("project-1"),
				Etag: stringPtr("invalid"),
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.projectsKV.(*nats.MockKeyValue)
				mockKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte("test"), 1), nil)
				mockKV.On("Delete", mock.Anything, "project-1", mock.Anything).Return(assert.AnError)
			},
			expectedError: true,
		},
		{
			name: "project not found",
			payload: &projsvc.DeleteProjectPayload{
				ID: stringPtr("nonexistent"),
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.projectsKV.(*nats.MockKeyValue)
				mockKV.On("Get", mock.Anything, "nonexistent").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			tt.setupMocks(service)

			err := service.DeleteProject(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
