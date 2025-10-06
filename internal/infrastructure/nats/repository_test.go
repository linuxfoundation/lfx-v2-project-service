// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewNatsRepository(t *testing.T) {
	tests := []struct {
		name            string
		projects        INatsKeyValue
		projectSettings INatsKeyValue
	}{
		{
			name:            "create repository with valid key-value stores",
			projects:        &MockKeyValue{},
			projectSettings: &MockKeyValue{},
		},
		{
			name:            "create repository with nil projects store",
			projects:        nil,
			projectSettings: &MockKeyValue{},
		},
		{
			name:            "create repository with nil settings store",
			projects:        &MockKeyValue{},
			projectSettings: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewNatsRepository(tt.projects, tt.projectSettings)

			assert.NotNil(t, repo)
			assert.Equal(t, tt.projects, repo.Projects)
			assert.Equal(t, tt.projectSettings, repo.ProjectSettings)
		})
	}
}

func TestNatsRepository_GetProjectBase(t *testing.T) {
	now := time.Now()
	projectBase := &models.ProjectBase{
		UID:         "test-project-uid",
		Slug:        "test-project",
		Name:        "Test Project",
		Description: "Test Description",
		Public:      true,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}

	tests := []struct {
		name        string
		projectUID  string
		setupMocks  func(*MockKeyValue)
		expected    *models.ProjectBase
		wantErr     bool
		expectedErr error
	}{
		{
			name:       "successful get project base",
			projectUID: "test-project-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				projectData, _ := json.Marshal(projectBase)
				mockEntry := &MockKeyValueEntry{
					value: projectData,
				}
				mockKV.On("Get", mock.Anything, "test-project-uid").Return(mockEntry, nil)
			},
			expected: projectBase,
			wantErr:  false,
		},
		{
			name:       "project not found",
			projectUID: "non-existent-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("Get", mock.Anything, "non-existent-uid").Return(nil, jetstream.ErrKeyNotFound)
			},
			expected:    nil,
			wantErr:     true,
			expectedErr: domain.ErrProjectNotFound,
		},
		{
			name:       "key-value store error",
			projectUID: "test-project-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("Get", mock.Anything, "test-project-uid").Return(nil, errors.New("nats connection error"))
			},
			expected:    nil,
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name:       "invalid JSON data",
			projectUID: "test-project-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				mockEntry := &MockKeyValueEntry{
					value: []byte("invalid-json"),
				}
				mockKV.On("Get", mock.Anything, "test-project-uid").Return(mockEntry, nil)
			},
			expected:    nil,
			wantErr:     true,
			expectedErr: domain.ErrUnmarshal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProjectsKV := &MockKeyValue{}
			mockSettingsKV := &MockKeyValue{}

			tt.setupMocks(mockProjectsKV)

			repo := NewNatsRepository(mockProjectsKV, mockSettingsKV)

			result, err := repo.GetProjectBase(context.Background(), tt.projectUID)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.UID, result.UID)
				assert.Equal(t, tt.expected.Slug, result.Slug)
				assert.Equal(t, tt.expected.Name, result.Name)
				assert.Equal(t, tt.expected.Description, result.Description)
				assert.Equal(t, tt.expected.Public, result.Public)
			}

			mockProjectsKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_GetProjectBaseWithRevision(t *testing.T) {
	now := time.Now()
	projectBase := &models.ProjectBase{
		UID:         "test-project-uid",
		Slug:        "test-project",
		Name:        "Test Project",
		Description: "Test Description",
		Public:      true,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}

	tests := []struct {
		name        string
		projectUID  string
		setupMocks  func(*MockKeyValue)
		expected    *models.ProjectBase
		expectedRev uint64
		wantErr     bool
	}{
		{
			name:       "successful get with revision",
			projectUID: "test-project-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				projectData, _ := json.Marshal(projectBase)
				mockEntry := &MockKeyValueEntry{
					value:    projectData,
					revision: 123,
				}
				mockKV.On("Get", mock.Anything, "test-project-uid").Return(mockEntry, nil)
			},
			expected:    projectBase,
			expectedRev: 123,
			wantErr:     false,
		},
		{
			name:       "project not found",
			projectUID: "non-existent-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("Get", mock.Anything, "non-existent-uid").Return(nil, jetstream.ErrKeyNotFound)
			},
			expected:    nil,
			expectedRev: 0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProjectsKV := &MockKeyValue{}
			mockSettingsKV := &MockKeyValue{}

			tt.setupMocks(mockProjectsKV)

			repo := NewNatsRepository(mockProjectsKV, mockSettingsKV)

			result, revision, err := repo.GetProjectBaseWithRevision(context.Background(), tt.projectUID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				assert.Equal(t, uint64(0), revision)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.UID, result.UID)
				assert.Equal(t, tt.expectedRev, revision)
			}

			mockProjectsKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_CreateProject(t *testing.T) {
	now := time.Now()
	projectBase := &models.ProjectBase{
		UID:         "test-project-uid",
		Slug:        "test-project",
		Name:        "Test Project",
		Description: "Test Description",
		Public:      true,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}

	projectSettings := &models.ProjectSettings{
		UID:              "test-project-uid",
		MissionStatement: "Our mission",
		Writers: []models.UserInfo{
			{Username: "writer1", Name: "Writer One", Email: "writer1@example.com", Avatar: ""},
		},
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	tests := []struct {
		name        string
		setupMocks  func(*MockKeyValue, *MockKeyValue)
		wantErr     bool
		expectedErr error
	}{
		{
			name: "successful project creation",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				// Put slug mapping
				mockProjectsKV.On("Put", mock.Anything, "slug/test-project", []byte("test-project-uid")).Return(uint64(1), nil)
				// Put project base
				mockProjectsKV.On("Put", mock.Anything, "test-project-uid", mock.Anything).Return(uint64(1), nil)
				// Put project settings
				mockSettingsKV.On("Put", mock.Anything, "test-project-uid", mock.Anything).Return(uint64(1), nil)
			},
			wantErr: false,
		},
		{
			name: "slug already exists",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				// Slug mapping Put call fails with ErrKeyExists
				mockProjectsKV.On("Put", mock.Anything, "slug/test-project", []byte("test-project-uid")).Return(uint64(0), jetstream.ErrKeyExists)
			},
			wantErr:     true,
			expectedErr: domain.ErrProjectSlugExists,
		},
		{
			name: "error putting project base",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				// Put slug mapping succeeds
				mockProjectsKV.On("Put", mock.Anything, "slug/test-project", []byte("test-project-uid")).Return(uint64(1), nil)
				// Put project base fails
				mockProjectsKV.On("Put", mock.Anything, "test-project-uid", mock.Anything).Return(uint64(0), errors.New("nats error"))
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProjectsKV := &MockKeyValue{}
			mockSettingsKV := &MockKeyValue{}

			tt.setupMocks(mockProjectsKV, mockSettingsKV)

			repo := NewNatsRepository(mockProjectsKV, mockSettingsKV)

			err := repo.CreateProject(context.Background(), projectBase, projectSettings)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err)
				}
			} else {
				assert.NoError(t, err)
			}

			mockProjectsKV.AssertExpectations(t)
			mockSettingsKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_ProjectExists(t *testing.T) {
	tests := []struct {
		name       string
		projectUID string
		setupMocks func(*MockKeyValue)
		expected   bool
		wantErr    bool
	}{
		{
			name:       "project exists",
			projectUID: "existing-project-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				mockEntry := &MockKeyValueEntry{value: []byte("project-data")}
				mockKV.On("Get", mock.Anything, "existing-project-uid").Return(mockEntry, nil)
			},
			expected: true,
			wantErr:  false,
		},
		{
			name:       "project does not exist",
			projectUID: "non-existent-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("Get", mock.Anything, "non-existent-uid").Return(nil, jetstream.ErrKeyNotFound)
			},
			expected: false,
			wantErr:  false,
		},
		{
			name:       "key-value store error",
			projectUID: "test-project-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("Get", mock.Anything, "test-project-uid").Return(nil, errors.New("nats connection error"))
			},
			expected: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProjectsKV := &MockKeyValue{}
			mockSettingsKV := &MockKeyValue{}

			tt.setupMocks(mockProjectsKV)

			repo := NewNatsRepository(mockProjectsKV, mockSettingsKV)

			exists, err := repo.ProjectExists(context.Background(), tt.projectUID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, exists)
			}

			mockProjectsKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_GetProjectUIDFromSlug(t *testing.T) {
	tests := []struct {
		name        string
		slug        string
		setupMocks  func(*MockKeyValue)
		expected    string
		wantErr     bool
		expectedErr error
	}{
		{
			name: "successful slug to UID conversion",
			slug: "test-project",
			setupMocks: func(mockKV *MockKeyValue) {
				mockEntry := &MockKeyValueEntry{value: []byte("test-project-uid")}
				mockKV.On("Get", mock.Anything, "slug/test-project").Return(mockEntry, nil)
			},
			expected: "test-project-uid",
			wantErr:  false,
		},
		{
			name: "slug not found",
			slug: "non-existent-slug",
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("Get", mock.Anything, "slug/non-existent-slug").Return(nil, jetstream.ErrKeyNotFound)
			},
			expected:    "",
			wantErr:     true,
			expectedErr: domain.ErrProjectNotFound,
		},
		{
			name: "key-value store error",
			slug: "test-project",
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("Get", mock.Anything, "slug/test-project").Return(nil, errors.New("nats connection error"))
			},
			expected:    "",
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProjectsKV := &MockKeyValue{}
			mockSettingsKV := &MockKeyValue{}

			tt.setupMocks(mockProjectsKV)

			repo := NewNatsRepository(mockProjectsKV, mockSettingsKV)

			uid, err := repo.GetProjectUIDFromSlug(context.Background(), tt.slug)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err)
				}
				assert.Empty(t, uid)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, uid)
			}

			mockProjectsKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_ListAllProjects(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name              string
		setupMocks        func(*MockKeyValue, *MockKeyValue)
		expectedBaseCount int
		expectedSettCount int
		wantErr           bool
	}{
		{
			name: "successful list all projects",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				// Mock projects list
				projectUID1 := "project-1"
				projectUID2 := "project-2"
				mockLister := NewMockKeyLister([]string{projectUID1, projectUID2, "slug/project-1", "slug/project-2"})
				mockProjectsKV.On("ListKeys", mock.Anything).Return(mockLister, nil)

				// Mock project entries
				project1Data, _ := json.Marshal(&models.ProjectBase{UID: projectUID1, Name: "Project 1", CreatedAt: &now, UpdatedAt: &now})
				project2Data, _ := json.Marshal(&models.ProjectBase{UID: projectUID2, Name: "Project 2", CreatedAt: &now, UpdatedAt: &now})

				mockProjectsKV.On("Get", mock.Anything, projectUID1).Return(&MockKeyValueEntry{value: project1Data}, nil)
				mockProjectsKV.On("Get", mock.Anything, projectUID2).Return(&MockKeyValueEntry{value: project2Data}, nil)

				// Mock settings list
				mockSettingsLister := NewMockKeyLister([]string{projectUID1})
				mockSettingsKV.On("ListKeys", mock.Anything).Return(mockSettingsLister, nil)

				// Mock settings entry
				settings1Data, _ := json.Marshal(&models.ProjectSettings{UID: projectUID1, MissionStatement: "Mission 1", CreatedAt: &now, UpdatedAt: &now})
				mockSettingsKV.On("Get", mock.Anything, projectUID1).Return(&MockKeyValueEntry{value: settings1Data}, nil)
			},
			expectedBaseCount: 2,
			expectedSettCount: 1,
			wantErr:           false,
		},
		{
			name: "error listing project keys",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				mockProjectsKV.On("ListKeys", mock.Anything).Return(nil, errors.New("nats error"))
			},
			expectedBaseCount: 0,
			expectedSettCount: 0,
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProjectsKV := &MockKeyValue{}
			mockSettingsKV := &MockKeyValue{}

			tt.setupMocks(mockProjectsKV, mockSettingsKV)

			repo := NewNatsRepository(mockProjectsKV, mockSettingsKV)

			baseProjects, settingsProjects, err := repo.ListAllProjects(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, baseProjects)
				assert.Nil(t, settingsProjects)
			} else {
				assert.NoError(t, err)
				assert.Len(t, baseProjects, tt.expectedBaseCount)
				assert.Len(t, settingsProjects, tt.expectedSettCount)
			}

			mockProjectsKV.AssertExpectations(t)
			mockSettingsKV.AssertExpectations(t)
		})
	}
}
