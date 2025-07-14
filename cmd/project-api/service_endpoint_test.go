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
	"goa.design/goa/v3/security"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// setupService creates a new ProjectsService with mocked external service APIs.
func setupService() *ProjectsService {
	if os.Getenv("DEBUG") == "true" {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	logger := slog.Default()
	service := &ProjectsService{
		logger:         logger,
		lfxEnvironment: constants.LFXEnvironmentDev,
		natsConn:       &MockNATSConn{},
		projectsKV:     &MockKeyValue{},
		auth:           &MockJwtAuth{},
	}

	return service
}

func TestGetProjects(t *testing.T) {
	tests := []struct {
		name           string
		payload        *projsvc.GetProjectsPayload
		setupMocks     func(*MockKeyValue)
		expectedError  bool
		expectedResult *projsvc.GetProjectsResult
	}{
		{
			name:    "success with projects",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockKV *MockKeyValue) {
				// Create mock key lister with project keys
				mockLister := &MockKeyLister{
					keys: []string{"project-1", "project-2"},
				}
				mockKV.On("ListKeys", mock.Anything).Return(mockLister, nil)

				// Mock project entries
				project1Data := `{"uid":"project-1","slug":"test-1","name":"Test Project 1","description":"Test 1","managers":["user1"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				project2Data := `{"uid":"project-2","slug":"test-2","name":"Test Project 2","description":"Test 2","managers":["user2"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`

				mockKV.On("Get", mock.Anything, "project-1").Return(&MockKeyValueEntry{value: []byte(project1Data)}, nil)
				mockKV.On("Get", mock.Anything, "project-2").Return(&MockKeyValueEntry{value: []byte(project2Data)}, nil)
			},
			expectedError: false,
			expectedResult: &projsvc.GetProjectsResult{
				Projects: []*projsvc.Project{
					{
						ID:          stringPtr("project-1"),
						Slug:        stringPtr("test-1"),
						Name:        stringPtr("Test Project 1"),
						Description: stringPtr("Test 1"),
						Managers:    []string{"user1"},
					},
					{
						ID:          stringPtr("project-2"),
						Slug:        stringPtr("test-2"),
						Name:        stringPtr("Test Project 2"),
						Description: stringPtr("Test 2"),
						Managers:    []string{"user2"},
					},
				},
			},
		},
		{
			name:    "success with no projects",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockKV *MockKeyValue) {
				mockLister := &MockKeyLister{keys: []string{}}
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
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("ListKeys", mock.Anything).Return(&MockKeyLister{}, assert.AnError)
			},
			expectedError: true,
		},
		{
			name:    "error getting project",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockKV *MockKeyValue) {
				mockLister := &MockKeyLister{keys: []string{"project-1"}}
				mockKV.On("ListKeys", mock.Anything).Return(mockLister, nil)
				mockKV.On("Get", mock.Anything, "project-1").Return(&MockKeyValueEntry{}, assert.AnError)
			},
			expectedError: true,
		},
		{
			name: "page token not supported",
			payload: &projsvc.GetProjectsPayload{
				PageToken: stringPtr("token"),
			},
			setupMocks:    func(mockKV *MockKeyValue) {},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			tt.setupMocks(service.projectsKV.(*MockKeyValue))

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
						assert.Equal(t, expectedProject.Managers, actualProject.Managers)
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
				Description: stringPtr("Test description"),
				Managers:    []string{"user1", "user2"},
			},
			setupMocks: func(service *ProjectsService) {
				mockNats := service.natsConn.(*MockNATSConn)
				mockKV := service.projectsKV.(*MockKeyValue)
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
			name: "slug already exists",
			payload: &projsvc.CreateProjectPayload{
				Slug:        "existing-project",
				Name:        "Test Project",
				Description: stringPtr("Test description"),
				Managers:    []string{"user1"},
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.projectsKV.(*MockKeyValue)
				mockKV.On("Put", mock.Anything, "slug/existing-project", mock.Anything).Return(uint64(1), jetstream.ErrKeyExists)
			},
			expectedError: true,
		},
		{
			name: "error creating slug mapping",
			payload: &projsvc.CreateProjectPayload{
				Slug:        "test-project",
				Name:        "Test Project",
				Description: stringPtr("Test description"),
				Managers:    []string{"user1"},
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.projectsKV.(*MockKeyValue)
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
				if tt.payload.Description != nil {
					assert.Equal(t, *tt.payload.Description, *result.Description)
				}
				assert.Equal(t, tt.payload.Managers, result.Managers)
				assert.NotEmpty(t, *result.ID)
			}
		})
	}
}

func TestGetOneProject(t *testing.T) {
	tests := []struct {
		name          string
		payload       *projsvc.GetOneProjectPayload
		setupMocks    func(*MockKeyValue)
		expectedError bool
		expectedID    string
	}{
		{
			name: "success",
			payload: &projsvc.GetOneProjectPayload{
				ProjectID: stringPtr("project-1"),
			},
			setupMocks: func(mockKV *MockKeyValue) {
				projectData := `{"uid":"project-1","slug":"test-1","name":"Test Project","description":"Test description","managers":["user1"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				mockKV.On("Get", mock.Anything, "project-1").Return(&MockKeyValueEntry{value: []byte(projectData)}, nil)
			},
			expectedError: false,
			expectedID:    "project-1",
		},
		{
			name: "project not found",
			payload: &projsvc.GetOneProjectPayload{
				ProjectID: stringPtr("nonexistent"),
			},
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("Get", mock.Anything, "nonexistent").Return(&MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
			},
			expectedError: true,
		},
		{
			name: "error getting project",
			payload: &projsvc.GetOneProjectPayload{
				ProjectID: stringPtr("project-1"),
			},
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("Get", mock.Anything, "project-1").Return(&MockKeyValueEntry{}, assert.AnError)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			tt.setupMocks(service.projectsKV.(*MockKeyValue))

			result, err := service.GetOneProject(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedID, *result.ID)
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
				ProjectID:   stringPtr("project-1"),
				Slug:        "updated-slug",
				Name:        "Updated Project",
				Description: stringPtr("Updated description"),
				Managers:    []string{"user1", "user2"},
			},
			setupMocks: func(service *ProjectsService) {
				mockNats := service.natsConn.(*MockNATSConn)
				mockKV := service.projectsKV.(*MockKeyValue)
				// Mock getting existing project
				projectData := `{"uid":"project-1","slug":"old-slug","name":"Old Project","description":"Old description","managers":["user1"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				mockKV.On("Get", mock.Anything, "project-1").Return(&MockKeyValueEntry{value: []byte(projectData), revision: 1}, nil)
				// Mock updating project
				mockKV.On("Update", mock.Anything, "project-1", mock.Anything, uint64(1)).Return(uint64(1), nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.IndexProjectSubject), mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.UpdateAccessProjectSubject), mock.Anything).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "project not found",
			payload: &projsvc.UpdateProjectPayload{
				ProjectID: stringPtr("nonexistent"),
				Slug:      "test",
				Name:      "Test",
				Managers:  []string{"user1"},
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.projectsKV.(*MockKeyValue)
				mockKV.On("Get", mock.Anything, "nonexistent").Return(&MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
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
				assert.NotNil(t, result)
				assert.Equal(t, tt.payload.Slug, *result.Slug)
				assert.Equal(t, tt.payload.Name, *result.Name)
				if tt.payload.Description != nil {
					assert.Equal(t, *tt.payload.Description, *result.Description)
				}
				assert.Equal(t, tt.payload.Managers, result.Managers)
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
				ProjectID: stringPtr("project-1"),
			},
			setupMocks: func(service *ProjectsService) {
				mockNats := service.natsConn.(*MockNATSConn)
				mockKV := service.projectsKV.(*MockKeyValue)
				// Mock getting existing project
				projectData := `{"uid":"project-1","slug":"test","name":"Test Project","description":"Test","managers":["user1"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				mockKV.On("Get", mock.Anything, "project-1").Return(&MockKeyValueEntry{value: []byte(projectData), revision: 1}, nil)
				// Mock deleting project
				mockKV.On("Delete", mock.Anything, "project-1", mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.IndexProjectSubject), mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.DeleteAllAccessSubject), mock.Anything).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "project not found",
			payload: &projsvc.DeleteProjectPayload{
				ProjectID: stringPtr("nonexistent"),
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.projectsKV.(*MockKeyValue)
				mockKV.On("Get", mock.Anything, "nonexistent").Return(&MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
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

func TestReadyz(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*ProjectsService)
		expectedError bool
		expectedBody  string
	}{
		{
			name: "service ready",
			setupMocks: func(service *ProjectsService) {
				service.natsConn.(*MockNATSConn).On("IsConnected").Return(true)
			},
			expectedError: false,
			expectedBody:  "OK\n",
		},
		{
			name: "NATS not connected",
			setupMocks: func(service *ProjectsService) {
				service.natsConn.(*MockNATSConn).On("IsConnected").Return(false)
			},
			expectedError: true,
		},
		{
			name: "NATS KV not initialized",
			setupMocks: func(service *ProjectsService) {
				service.projectsKV = nil
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			tt.setupMocks(service)

			result, err := service.Readyz(context.Background())

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedBody, string(result))
			}
		})
	}
}

func TestLivez(t *testing.T) {
	service := &ProjectsService{}

	result, err := service.Livez(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, "OK\n", string(result))
}

func TestJWTAuth(t *testing.T) {
	tests := []struct {
		name          string
		bearerToken   string
		schema        *security.JWTScheme
		expectedError bool
		setupMocks    func(*MockJwtAuth)
	}{
		{
			name: "valid token",
			// This token is just an example token value generated from jwt.io.
			bearerToken:   "eyJhbGciOiJQUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWUsImlhdCI6MTUxNjIzOTAyMn0.iOeNU4dAFFeBwNj6qdhdvm-IvDQrTa6R22lQVJVuWJxorJfeQww5Nwsra0PjaOYhAMj9jNMO5YLmud8U7iQ5gJK2zYyepeSuXhfSi8yjFZfRiSkelqSkU19I-Ja8aQBDbqXf2SAWA8mHF8VS3F08rgEaLCyv98fLLH4vSvsJGf6ueZSLKDVXz24rZRXGWtYYk_OYYTVgR1cg0BLCsuCvqZvHleImJKiWmtS0-CymMO4MMjCy_FIl6I56NqLE9C87tUVpo1mT-kbg5cHDD8I7MjCW5Iii5dethB4Vid3mZ6emKjVYgXrtkOQ-JyGMh6fnQxEFN1ft33GX2eRHluK9eg",
			schema:        &security.JWTScheme{},
			expectedError: false,
			setupMocks: func(mockJwtAuth *MockJwtAuth) {
				mockJwtAuth.On("parsePrincipal", mock.Anything, mock.Anything, mock.Anything).Return("user1", nil)
			},
		},
		{
			name:          "invalid token",
			bearerToken:   "invalid.token",
			schema:        &security.JWTScheme{},
			expectedError: true,
			setupMocks: func(mockJwtAuth *MockJwtAuth) {
				mockJwtAuth.On("parsePrincipal", mock.Anything, mock.Anything, mock.Anything).Return("", assert.AnError)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			tt.setupMocks(service.auth.(*MockJwtAuth))

			ctx, err := service.JWTAuth(context.Background(), tt.bearerToken, tt.schema)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				// For valid tokens, we expect the context to be modified
				if assert.NoError(t, err) {
					assert.NotEqual(t, context.Background(), ctx)
				}
			}
		})
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// Test cleanup
func TestMain(m *testing.M) {
	// Run tests
	m.Run()
}
