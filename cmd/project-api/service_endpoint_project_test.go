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
		kvStores:       KVStores{Projects: &nats.MockKeyValue{}, ProjectSettings: &nats.MockKeyValue{}},
		auth:           &MockJwtAuth{},
	}

	return service
}

func TestGetProjects(t *testing.T) {
	tests := []struct {
		name           string
		payload        *projsvc.GetProjectsPayload
		setupMocks     func(*nats.MockKeyValue, *nats.MockKeyValue)
		expectedError  bool
		expectedResult *projsvc.GetProjectsResult
	}{
		{
			name:    "success with projects",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockProjectsKV *nats.MockKeyValue, mockSettingsKV *nats.MockKeyValue) {
				// Create mock key lister with project keys
				mockProjectsLister := nats.NewMockKeyLister([]string{"project-1", "project-2"})
				mockProjectsKV.On("ListKeys", mock.Anything).Return(mockProjectsLister, nil)

				// Mock project entries
				project1Data := `{"uid":"project-1","slug":"test-1","name":"Test Project 1","description":"Test 1","public":true,"parent_uid":"","created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				project2Data := `{"uid":"project-2","slug":"test-2","name":"Test Project 2","description":"Test 2","public":false,"parent_uid":"parent-uid","created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`

				mockProjectsKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte(project1Data), 123), nil)
				mockProjectsKV.On("Get", mock.Anything, "project-2").Return(nats.NewMockKeyValueEntry([]byte(project2Data), 123), nil)

				// Create mock key lister with project settings keys
				mockSettingsLister := nats.NewMockKeyLister([]string{"project-1", "project-2"})
				mockSettingsKV.On("ListKeys", mock.Anything).Return(mockSettingsLister, nil)

				// Mock project settings entries
				settings1Data := `{"uid":"project-1","writers":["user2"],"auditors":["user1"]}`
				settings2Data := `{"uid":"project-2","writers":["user3"],"auditors":["user2"]}`

				mockSettingsKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte(settings1Data), 123), nil)
				mockSettingsKV.On("Get", mock.Anything, "project-2").Return(nats.NewMockKeyValueEntry([]byte(settings2Data), 123), nil)
			},
			expectedError: false,
			expectedResult: &projsvc.GetProjectsResult{
				Projects: []*projsvc.ProjectFull{
					{
						UID:         stringPtr("project-1"),
						Slug:        stringPtr("test-1"),
						Name:        stringPtr("Test Project 1"),
						Description: stringPtr("Test 1"),
						Public:      boolPtr(true),
						Writers:     []string{"user2"},
						Auditors:    []string{"user1"},
					},
					{
						UID:         stringPtr("project-2"),
						Slug:        stringPtr("test-2"),
						Name:        stringPtr("Test Project 2"),
						Description: stringPtr("Test 2"),
						Public:      boolPtr(false),
						ParentUID:   stringPtr("parent-uid"),
						Writers:     []string{"user3"},
						Auditors:    []string{"user2"},
					},
				},
			},
		},
		{
			name:    "success with no projects",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockKV *nats.MockKeyValue, mockSettingsKV *nats.MockKeyValue) {
				mockLister := nats.NewMockKeyLister([]string{})
				mockKV.On("ListKeys", mock.Anything).Return(mockLister, nil)
				mockSettingsKV.On("ListKeys", mock.Anything).Return(nats.NewMockKeyLister([]string{}), nil)
			},
			expectedError: false,
			expectedResult: &projsvc.GetProjectsResult{
				Projects: []*projsvc.ProjectFull{},
			},
		},
		{
			name:    "error listing keys",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockKV *nats.MockKeyValue, mockSettingsKV *nats.MockKeyValue) {
				mockKV.On("ListKeys", mock.Anything).Return(&nats.MockKeyLister{}, assert.AnError)
			},
			expectedError: true,
		},
		{
			name:    "error getting project",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockKV *nats.MockKeyValue, mockSettingsKV *nats.MockKeyValue) {
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
			tt.setupMocks(service.kvStores.Projects.(*nats.MockKeyValue), service.kvStores.ProjectSettings.(*nats.MockKeyValue))

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
						assert.Equal(t, *expectedProject.UID, *actualProject.UID)
						assert.Equal(t, *expectedProject.Slug, *actualProject.Slug)
						assert.Equal(t, *expectedProject.Name, *actualProject.Name)
						if expectedProject.Description != nil {
							assert.Equal(t, *expectedProject.Description, *actualProject.Description)
						}
						assert.Equal(t, expectedProject.Public, actualProject.Public)
						if expectedProject.ParentUID != nil {
							assert.Equal(t, *expectedProject.ParentUID, *actualProject.ParentUID)
						}
						assert.Equal(t, expectedProject.Writers, actualProject.Writers)
						assert.Equal(t, expectedProject.Auditors, actualProject.Auditors)
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
				Writers:     []string{"user1", "user2"},
				Auditors:    []string{"user3", "user4"},
			},
			setupMocks: func(service *ProjectsService) {
				mockNats := service.natsConn.(*nats.MockNATSConn)
				mockProjectsKV := service.kvStores.Projects.(*nats.MockKeyValue)
				mockSettingsKV := service.kvStores.ProjectSettings.(*nats.MockKeyValue)

				mockProjectsKV.On("Get", mock.Anything, "slug/test-project").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
				// Mock successful slug mapping creation
				mockProjectsKV.On("Put", mock.Anything, "slug/test-project", mock.Anything).Return(uint64(1), nil)
				// Mock successful project creation
				mockProjectsKV.On("Put", mock.Anything, mock.Anything, mock.Anything).Return(uint64(1), nil)
				// Mock successful project settings creation
				mockSettingsKV.On("Put", mock.Anything, mock.Anything, mock.Anything).Return(uint64(1), nil)

				// Mock NATS messages
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.IndexProjectSubject), mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.UpdateAccessProjectSubject), mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.IndexProjectSettingsSubject), mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.UpdateAccessProjectSettingsSubject), mock.Anything).Return(nil)
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
				Writers:     []string{"user1", "user2"},
				Auditors:    []string{"user3", "user4"},
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
				mockKV := service.kvStores.Projects.(*nats.MockKeyValue)
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
				mockKV := service.kvStores.Projects.(*nats.MockKeyValue)
				// Mock slug check - slug already exists (Get succeeds)
				existingProjectUID := "existing-project-uid"
				mockKV.On("Get", mock.Anything, "slug/existing-project").Return(nats.NewMockKeyValueEntry([]byte(existingProjectUID), 1), nil)
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
				mockKV := service.kvStores.Projects.(*nats.MockKeyValue)
				mockKV.On("Get", mock.Anything, "slug/test-project").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
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
				assert.Equal(t, tt.payload.Writers, result.Writers)
				assert.Equal(t, tt.payload.Auditors, result.Auditors)
			}
		})
	}
}

func TestGetOneProject(t *testing.T) {
	tests := []struct {
		name          string
		payload       *projsvc.GetOneProjectBasePayload
		setupMocks    func(*nats.MockKeyValue)
		expectedError bool
		expectedID    string
	}{
		{
			name: "success",
			payload: &projsvc.GetOneProjectBasePayload{
				UID: stringPtr("project-1"),
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
			payload: &projsvc.GetOneProjectBasePayload{
				UID: stringPtr("nonexistent"),
			},
			setupMocks: func(mockKV *nats.MockKeyValue) {
				mockKV.On("Get", mock.Anything, "nonexistent").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
			},
			expectedError: true,
		},
		{
			name: "error getting project",
			payload: &projsvc.GetOneProjectBasePayload{
				UID: stringPtr("project-1"),
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
			tt.setupMocks(service.kvStores.Projects.(*nats.MockKeyValue))

			result, err := service.GetOneProjectBase(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if assert.NotNil(t, result) {
					if assert.NotNil(t, result.Project) {
						assert.Equal(t, tt.expectedID, *result.Project.UID)
					}
					if assert.NotNil(t, result.Etag) {
						assert.NotEmpty(t, *result.Etag)
					}
				}
			}
		})
	}
}

func TestGetOneProjectSettings(t *testing.T) {
	tests := []struct {
		name           string
		payload        *projsvc.GetOneProjectSettingsPayload
		setupMocks     func(*ProjectsService)
		expectedError  bool
		expectedResult *projsvc.GetOneProjectSettingsResult
	}{
		{
			name: "success",
			payload: &projsvc.GetOneProjectSettingsPayload{
				UID: stringPtr("project-1"),
			},
			setupMocks: func(service *ProjectsService) {
				mockProjectsKV := service.kvStores.Projects.(*nats.MockKeyValue)
				mockSettingsKV := service.kvStores.ProjectSettings.(*nats.MockKeyValue)

				// Mock checking if project exists
				projectData := `{"uid":"project-1","slug":"test","name":"Test Project","description":"Test","public":false,"parent_uid":"parent-uid","created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				mockProjectsKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte(projectData), 1), nil)

				// Mock getting settings
				settingsData := `{"uid":"project-1","mission_statement":"Our mission","announcement_date":"2023-01-01T00:00:00Z","writers":["user1","user2"],"auditors":["user3"]}`
				mockSettingsKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte(settingsData), 2), nil)
			},
			expectedError: false,
			expectedResult: &projsvc.GetOneProjectSettingsResult{
				ProjectSettings: &projsvc.ProjectSettings{
					UID:              stringPtr("project-1"),
					MissionStatement: stringPtr("Our mission"),
					AnnouncementDate: stringPtr("2023-01-01"),
					Writers:          []string{"user1", "user2"},
					Auditors:         []string{"user3"},
				},
				Etag: stringPtr("2"),
			},
		},
		{
			name: "missing project UID",
			payload: &projsvc.GetOneProjectSettingsPayload{
				UID: nil,
			},
			setupMocks:    func(_ *ProjectsService) {},
			expectedError: true,
		},
		{
			name: "project not found",
			payload: &projsvc.GetOneProjectSettingsPayload{
				UID: stringPtr("project-1"),
			},
			setupMocks: func(service *ProjectsService) {
				mockProjectsKV := service.kvStores.Projects.(*nats.MockKeyValue)
				mockProjectsKV.On("Get", mock.Anything, "project-1").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
			},
			expectedError: true,
		},
		{
			name: "settings not found",
			payload: &projsvc.GetOneProjectSettingsPayload{
				UID: stringPtr("project-1"),
			},
			setupMocks: func(service *ProjectsService) {
				mockProjectsKV := service.kvStores.Projects.(*nats.MockKeyValue)
				mockSettingsKV := service.kvStores.ProjectSettings.(*nats.MockKeyValue)

				// Mock project exists
				projectData := `{"uid":"project-1","slug":"test","name":"Test Project","description":"Test","public":false,"parent_uid":"parent-uid","created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				mockProjectsKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte(projectData), 1), nil)

				// Mock settings not found
				mockSettingsKV.On("Get", mock.Anything, "project-1").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			tt.setupMocks(service)

			result, err := service.GetOneProjectSettings(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.NotNil(t, result.ProjectSettings)
				assert.Equal(t, *tt.expectedResult.ProjectSettings.UID, *result.ProjectSettings.UID)
				assert.Equal(t, *tt.expectedResult.ProjectSettings.MissionStatement, *result.ProjectSettings.MissionStatement)
				assert.Equal(t, *tt.expectedResult.ProjectSettings.AnnouncementDate, *result.ProjectSettings.AnnouncementDate)
				assert.Equal(t, tt.expectedResult.ProjectSettings.Writers, result.ProjectSettings.Writers)
				assert.Equal(t, tt.expectedResult.ProjectSettings.Auditors, result.ProjectSettings.Auditors)
				assert.Equal(t, *tt.expectedResult.Etag, *result.Etag)
			}
		})
	}
}

func TestUpdateProject(t *testing.T) {
	tests := []struct {
		name          string
		payload       *projsvc.UpdateProjectBasePayload
		setupMocks    func(*ProjectsService)
		expectedError bool
	}{
		{
			name: "success",
			payload: &projsvc.UpdateProjectBasePayload{
				UID:         stringPtr("project-1"),
				Slug:        "updated-slug",
				Name:        "Updated Project",
				Description: "Updated description",
				Public:      boolPtr(true),
				ParentUID:   stringPtr(""),
				Etag:        stringPtr("1"),
			},
			setupMocks: func(service *ProjectsService) {
				mockNats := service.natsConn.(*nats.MockNATSConn)
				mockKV := service.kvStores.Projects.(*nats.MockKeyValue)
				// Mock getting existing project
				projectData := `{"uid":"project-1","slug":"old-slug","name":"Old Project","description":"Old description","public":false,"parent_uid":"parent-uid","auditors":["user1"],"writers":["user2"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				mockKV.On("Get", mock.Anything, "slug/updated-slug").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
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
			payload: &projsvc.UpdateProjectBasePayload{
				UID:  stringPtr("project-1"),
				Etag: stringPtr("invalid"),
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.kvStores.Projects.(*nats.MockKeyValue)
				mockKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte("test"), 1), nil)
				mockKV.On("Update", mock.Anything, "project-1", mock.Anything, uint64(1)).Return(uint64(1), assert.AnError)
			},
			expectedError: true,
		},
		{
			name: "project not found",
			payload: &projsvc.UpdateProjectBasePayload{
				UID:       stringPtr("nonexistent"),
				Slug:      "test",
				Name:      "Test",
				Public:    boolPtr(true),
				ParentUID: stringPtr(""),
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.kvStores.Projects.(*nats.MockKeyValue)
				mockKV.On("Get", mock.Anything, "nonexistent").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
			},
			expectedError: true,
		},
		{
			name: "project slug already exists",
			payload: &projsvc.UpdateProjectBasePayload{
				UID:  stringPtr("project-1"),
				Slug: "existing-project",
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.kvStores.Projects.(*nats.MockKeyValue)
				mockKV.On("Get", mock.Anything, "slug/existing-project").Return(nats.NewMockKeyValueEntry([]byte("existing-project-uid"), 1), nil)
			},
			expectedError: true,
		},
		{
			name: "invalid parent UID",
			payload: &projsvc.UpdateProjectBasePayload{
				UID:       stringPtr("project-1"),
				Slug:      "test",
				Name:      "Test",
				Public:    boolPtr(true),
				ParentUID: stringPtr("invalid-parent-uid"),
			},
			setupMocks:    func(_ *ProjectsService) {},
			expectedError: true,
		},
		{
			name: "parent project not found",
			payload: &projsvc.UpdateProjectBasePayload{
				UID:       stringPtr("project-1"),
				Slug:      "test",
				Name:      "Test",
				Public:    boolPtr(true),
				ParentUID: stringPtr("787620d0-d7de-449a-b0bf-9d28b13da818"),
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.kvStores.Projects.(*nats.MockKeyValue)
				mockKV.On("Get", mock.Anything, "787620d0-d7de-449a-b0bf-9d28b13da818").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			tt.setupMocks(service)

			result, err := service.UpdateProjectBase(context.Background(), tt.payload)

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
				}
			}
		})
	}
}

func TestUpdateProjectSettings(t *testing.T) {
	tests := []struct {
		name          string
		payload       *projsvc.UpdateProjectSettingsPayload
		setupMocks    func(*ProjectsService)
		expectedError bool
	}{
		{
			name: "success",
			payload: &projsvc.UpdateProjectSettingsPayload{
				UID:              stringPtr("project-1"),
				Etag:             stringPtr("2"),
				MissionStatement: stringPtr("Updated mission"),
				AnnouncementDate: stringPtr("2023-12-01"),
				Writers:          []string{"user1", "user2", "user3"},
				Auditors:         []string{"user4"},
			},
			setupMocks: func(service *ProjectsService) {
				mockNats := service.natsConn.(*nats.MockNATSConn)
				mockProjectsKV := service.kvStores.Projects.(*nats.MockKeyValue)
				mockSettingsKV := service.kvStores.ProjectSettings.(*nats.MockKeyValue)

				// Mock checking if project exists
				projectData := `{"uid":"project-1","slug":"test","name":"Test Project","description":"Test","public":false,"parent_uid":"parent-uid","created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				mockProjectsKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte(projectData), 1), nil)

				// Mock updating settings
				mockSettingsKV.On("Update", mock.Anything, "project-1", mock.Anything, uint64(2)).Return(uint64(3), nil)

				// Mock NATS messages
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.IndexProjectSettingsSubject), mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.UpdateAccessProjectSettingsSubject), mock.Anything).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "missing project UID",
			payload: &projsvc.UpdateProjectSettingsPayload{
				UID:              nil,
				Etag:             stringPtr("1"),
				MissionStatement: stringPtr("Updated mission"),
			},
			setupMocks:    func(_ *ProjectsService) {},
			expectedError: true,
		},
		{
			name: "missing etag",
			payload: &projsvc.UpdateProjectSettingsPayload{
				UID:              stringPtr("project-1"),
				Etag:             nil,
				MissionStatement: stringPtr("Updated mission"),
			},
			setupMocks:    func(_ *ProjectsService) {},
			expectedError: true,
		},
		{
			name: "project not found",
			payload: &projsvc.UpdateProjectSettingsPayload{
				UID:              stringPtr("project-1"),
				Etag:             stringPtr("1"),
				MissionStatement: stringPtr("Updated mission"),
			},
			setupMocks: func(service *ProjectsService) {
				mockProjectsKV := service.kvStores.Projects.(*nats.MockKeyValue)
				mockProjectsKV.On("Get", mock.Anything, "project-1").Return(&nats.MockKeyValueEntry{}, jetstream.ErrKeyNotFound)
			},
			expectedError: true,
		},
		{
			name: "wrong etag",
			payload: &projsvc.UpdateProjectSettingsPayload{
				UID:              stringPtr("project-1"),
				Etag:             stringPtr("999"),
				MissionStatement: stringPtr("Updated mission"),
			},
			setupMocks: func(service *ProjectsService) {
				mockProjectsKV := service.kvStores.Projects.(*nats.MockKeyValue)
				mockSettingsKV := service.kvStores.ProjectSettings.(*nats.MockKeyValue)

				// Mock project exists
				projectData := `{"uid":"project-1","slug":"test","name":"Test Project","description":"Test","public":false,"parent_uid":"parent-uid","created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				mockProjectsKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte(projectData), 1), nil)

				// Mock update failing due to wrong etag
				mockSettingsKV.On("Update", mock.Anything, "project-1", mock.Anything, uint64(999)).Return(uint64(0), fmt.Errorf("wrong last sequence"))
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			tt.setupMocks(service)

			result, err := service.UpdateProjectSettings(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, *tt.payload.UID, *result.UID)
				assert.Equal(t, *tt.payload.MissionStatement, *result.MissionStatement)
				assert.Equal(t, *tt.payload.AnnouncementDate, *result.AnnouncementDate)
				assert.Equal(t, tt.payload.Writers, result.Writers)
				assert.Equal(t, tt.payload.Auditors, result.Auditors)
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
				UID:  stringPtr("project-1"),
				Etag: stringPtr("1"),
			},
			setupMocks: func(service *ProjectsService) {
				mockNats := service.natsConn.(*nats.MockNATSConn)
				mockKV := service.kvStores.Projects.(*nats.MockKeyValue)
				// Mock getting existing project
				projectData := `{"uid":"project-1","slug":"test","name":"Test Project","description":"Test","public":false,"parent_uid":"parent-uid","auditors":["user1"],"writers":["user2"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				mockKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte(projectData), 1), nil)
				// Mock deleting project
				mockKV.On("Delete", mock.Anything, "project-1", mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.IndexProjectSubject), mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.DeleteAllAccessSubject), mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.DeleteAllAccessProjectSettingsSubject), mock.Anything).Return(nil)
				mockNats.On("Publish", fmt.Sprintf("%s%s", service.lfxEnvironment, constants.IndexProjectSettingsSubject), mock.Anything).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "etag header is invalid",
			payload: &projsvc.DeleteProjectPayload{
				UID:  stringPtr("project-1"),
				Etag: stringPtr("invalid"),
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.kvStores.Projects.(*nats.MockKeyValue)
				mockKV.On("Get", mock.Anything, "project-1").Return(nats.NewMockKeyValueEntry([]byte("test"), 1), nil)
				mockKV.On("Delete", mock.Anything, "project-1", mock.Anything).Return(assert.AnError)
			},
			expectedError: true,
		},
		{
			name: "project not found",
			payload: &projsvc.DeleteProjectPayload{
				UID: stringPtr("nonexistent"),
			},
			setupMocks: func(service *ProjectsService) {
				mockKV := service.kvStores.Projects.(*nats.MockKeyValue)
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
