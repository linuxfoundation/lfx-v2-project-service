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

			repo := &NatsRepository{Projects: mockProjectsKV, ProjectSettings: mockSettingsKV}

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

func TestNatsRepository_GetProjectSettings(t *testing.T) {
	settings := &models.ProjectSettings{UID: "test-project-uid"}

	tests := []struct {
		name        string
		projectUID  string
		setupMocks  func(*MockKeyValue)
		wantErr     bool
		expectedErr error
	}{
		{
			name:       "successful get project settings",
			projectUID: "test-project-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				settingsData, _ := json.Marshal(settings)
				mockKV.On("Get", mock.Anything, "test-project-uid").Return(&MockKeyValueEntry{value: settingsData}, nil)
			},
			wantErr: false,
		},
		{
			// The store's own not-found is translated to the domain sentinel, so a
			// caller can tell an absent settings record from a broken store without
			// importing the store's error type.
			name:       "missing settings reports the domain not-found sentinel",
			projectUID: "non-existent-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("Get", mock.Anything, "non-existent-uid").Return(nil, jetstream.ErrKeyNotFound)
			},
			wantErr:     true,
			expectedErr: domain.ErrProjectNotFound,
		},
		{
			// Only the not-found case is translated: anything else reaches the caller
			// as it was, so a store outage is not reported as an absent project.
			name:       "key-value store error is not reported as not-found",
			projectUID: "test-project-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("Get", mock.Anything, "test-project-uid").Return(nil, errors.New("nats connection error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProjectsKV := &MockKeyValue{}
			mockSettingsKV := &MockKeyValue{}

			tt.setupMocks(mockSettingsKV)

			repo := &NatsRepository{Projects: mockProjectsKV, ProjectSettings: mockSettingsKV}

			result, err := repo.GetProjectSettings(context.Background(), tt.projectUID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tt.expectedErr != nil {
					assert.ErrorIs(t, err, tt.expectedErr)
				} else {
					assert.NotErrorIs(t, err, domain.ErrProjectNotFound)
				}
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, settings.UID, result.UID)
			}

			mockSettingsKV.AssertExpectations(t)
		})
	}
}

// The revision-carrying read shares the not-found translation with
// GetProjectSettings. Covered separately because the two are distinct entry
// points, and callers of this one test for the domain sentinel too.
func TestNatsRepository_GetProjectSettingsWithRevision(t *testing.T) {
	storeErr := errors.New("nats connection error")

	tests := []struct {
		name         string
		setupMocks   func(*MockKeyValue)
		wantMission  string
		wantRev      uint64
		wantErr      bool
		wantErrIs    error
		wantNotErrIs error
	}{
		{
			name: "missing settings reports the domain not-found sentinel",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, "test-project-uid").Return(nil, jetstream.ErrKeyNotFound)
			},
			wantErr:   true,
			wantErrIs: domain.ErrProjectNotFound,
		},
		{
			name: "success returns settings with revision",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, "test-project-uid").Return(
					NewMockKeyValueEntry(mustMarshal(t, &models.ProjectSettings{UID: "test-project-uid", MissionStatement: "Test mission"}), 12), nil)
			},
			wantMission: "Test mission",
			wantRev:     12,
		},
		{
			name: "key-value store error is returned as-is, not as not-found",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, "test-project-uid").Return(nil, storeErr)
			},
			wantErr:      true,
			wantErrIs:    storeErr,
			wantNotErrIs: domain.ErrProjectNotFound,
		},
		{
			name: "invalid stored json returns the unmarshal error",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, "test-project-uid").Return(NewMockKeyValueEntry([]byte("{not json"), 1), nil)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSettingsKV := &MockKeyValue{}
			tt.setupMocks(mockSettingsKV)
			repo := &NatsRepository{Projects: &MockKeyValue{}, ProjectSettings: mockSettingsKV}

			settings, revision, err := repo.GetProjectSettingsWithRevision(context.Background(), "test-project-uid")

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					assert.ErrorIs(t, err, tt.wantErrIs)
				}
				if tt.wantNotErrIs != nil {
					assert.NotErrorIs(t, err, tt.wantNotErrIs)
				}
				assert.Nil(t, settings)
				assert.Zero(t, revision)
			} else {
				require.NoError(t, err)
				require.NotNil(t, settings)
				assert.Equal(t, tt.wantMission, settings.MissionStatement)
				assert.Equal(t, tt.wantRev, revision)
			}
			mockSettingsKV.AssertExpectations(t)
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
		expectedErr error
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
			expectedErr: domain.ErrProjectNotFound,
		},
		{
			name:       "key-value store error returns internal error",
			projectUID: "test-project-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("Get", mock.Anything, "test-project-uid").Return(nil, errors.New("nats error"))
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name:       "invalid stored json returns unmarshal error",
			projectUID: "test-project-uid",
			setupMocks: func(mockKV *MockKeyValue) {
				mockKV.On("Get", mock.Anything, "test-project-uid").Return(NewMockKeyValueEntry([]byte("{not json"), 1), nil)
			},
			wantErr:     true,
			expectedErr: domain.ErrUnmarshal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProjectsKV := &MockKeyValue{}
			mockSettingsKV := &MockKeyValue{}

			tt.setupMocks(mockProjectsKV)

			repo := &NatsRepository{Projects: mockProjectsKV, ProjectSettings: mockSettingsKV}

			result, revision, err := repo.GetProjectBaseWithRevision(context.Background(), tt.projectUID)

			if tt.wantErr {
				assert.Equal(t, tt.expectedErr, err)
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
				// Create slug mapping (conditional)
				mockProjectsKV.On("Create", mock.Anything, "slug/test-project", []byte("test-project-uid")).Return(uint64(1), nil)
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
				// Create slug mapping fails because key already exists
				mockProjectsKV.On("Create", mock.Anything, "slug/test-project", []byte("test-project-uid")).Return(uint64(0), jetstream.ErrKeyExists)
			},
			wantErr:     true,
			expectedErr: domain.ErrProjectSlugExists,
		},
		{
			name: "error creating slug mapping (infra error)",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				// Create slug mapping fails with a generic infra error
				mockProjectsKV.On("Create", mock.Anything, "slug/test-project", []byte("test-project-uid")).Return(uint64(0), errors.New("nats error"))
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name: "error putting project base: base not committed",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				// Create slug mapping succeeds
				mockProjectsKV.On("Create", mock.Anything, "slug/test-project", []byte("test-project-uid")).Return(uint64(1), nil)
				// Put project base fails
				mockProjectsKV.On("Put", mock.Anything, "test-project-uid", mock.Anything).Return(uint64(0), errors.New("nats error"))
				// Re-read confirms base was not committed
				mockProjectsKV.On("Get", mock.Anything, "test-project-uid").Return(nil, jetstream.ErrKeyNotFound)
				// Rollback: deleteProjectSlugMapping reads the slug key then deletes it
				mockProjectsKV.On("Get", mock.Anything, "slug/test-project").Return(&MockKeyValueEntry{value: []byte("test-project-uid"), revision: 1}, nil)
				mockProjectsKV.On("Delete", mock.Anything, "slug/test-project", mock.Anything).Return(nil)
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name: "error putting project base: base not committed, rollback delete fails",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				// Create slug mapping succeeds
				mockProjectsKV.On("Create", mock.Anything, "slug/test-project", []byte("test-project-uid")).Return(uint64(1), nil)
				// Put project base fails
				mockProjectsKV.On("Put", mock.Anything, "test-project-uid", mock.Anything).Return(uint64(0), errors.New("nats error"))
				// Re-read confirms base was not committed
				mockProjectsKV.On("Get", mock.Anything, "test-project-uid").Return(nil, jetstream.ErrKeyNotFound)
				// Rollback: slug key found but Delete fails — still returns ErrInternal
				mockProjectsKV.On("Get", mock.Anything, "slug/test-project").Return(&MockKeyValueEntry{value: []byte("test-project-uid"), revision: 1}, nil)
				mockProjectsKV.On("Delete", mock.Anything, "slug/test-project", mock.Anything).Return(errors.New("delete failed"))
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name: "error putting project base: base committed (keep slug reservation)",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				// Create slug mapping succeeds
				mockProjectsKV.On("Create", mock.Anything, "slug/test-project", []byte("test-project-uid")).Return(uint64(1), nil)
				// Put project base fails (ambiguous — ack lost)
				mockProjectsKV.On("Put", mock.Anything, "test-project-uid", mock.Anything).Return(uint64(0), errors.New("nats error"))
				// Re-read finds the base — write was committed; slug reservation must be kept
				baseData, _ := json.Marshal(projectBase)
				mockProjectsKV.On("Get", mock.Anything, "test-project-uid").Return(&MockKeyValueEntry{value: baseData}, nil)
				// No rollback calls expected
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name: "error putting project base: verification read fails (keep slug reservation)",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				// Create slug mapping succeeds
				mockProjectsKV.On("Create", mock.Anything, "slug/test-project", []byte("test-project-uid")).Return(uint64(1), nil)
				// Put project base fails
				mockProjectsKV.On("Put", mock.Anything, "test-project-uid", mock.Anything).Return(uint64(0), errors.New("nats error"))
				// Re-read also fails — outcome unknown; slug reservation must be kept
				mockProjectsKV.On("Get", mock.Anything, "test-project-uid").Return(nil, errors.New("nats error"))
				// No rollback calls expected
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name: "error putting project settings",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				// Create slug mapping succeeds
				mockProjectsKV.On("Create", mock.Anything, "slug/test-project", []byte("test-project-uid")).Return(uint64(1), nil)
				// Put project base succeeds — base is now confirmed committed
				mockProjectsKV.On("Put", mock.Anything, "test-project-uid", mock.Anything).Return(uint64(2), nil)
				// Put project settings fails — base and slug must NOT be rolled back
				mockSettingsKV.On("Put", mock.Anything, "test-project-uid", mock.Anything).Return(uint64(0), errors.New("nats error"))
				// No rollback calls expected (base is real, destroying it would be data loss)
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

			repo := &NatsRepository{Projects: mockProjectsKV, ProjectSettings: mockSettingsKV}

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

			repo := &NatsRepository{Projects: mockProjectsKV, ProjectSettings: mockSettingsKV}

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

			repo := &NatsRepository{Projects: mockProjectsKV, ProjectSettings: mockSettingsKV}

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
		{
			name: "error getting a project base entry",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				mockProjectsKV.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{"project-1"}), nil)
				mockProjectsKV.On("Get", mock.Anything, "project-1").Return(nil, errors.New("nats error"))
			},
			wantErr: true,
		},
		{
			name: "invalid project base json",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				mockProjectsKV.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{"project-1"}), nil)
				mockProjectsKV.On("Get", mock.Anything, "project-1").Return(NewMockKeyValueEntry([]byte("{not json"), 1), nil)
			},
			wantErr: true,
		},
		{
			name: "error listing project settings keys",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				mockProjectsKV.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{}), nil)
				mockSettingsKV.On("ListKeys", mock.Anything).Return(nil, errors.New("nats error"))
			},
			wantErr: true,
		},
		{
			name: "error getting a project settings entry",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				mockProjectsKV.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{}), nil)
				mockSettingsKV.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{"project-1"}), nil)
				mockSettingsKV.On("Get", mock.Anything, "project-1").Return(nil, errors.New("nats error"))
			},
			wantErr: true,
		},
		{
			name: "invalid project settings json",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				mockProjectsKV.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{}), nil)
				mockSettingsKV.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{"project-1"}), nil)
				mockSettingsKV.On("Get", mock.Anything, "project-1").Return(NewMockKeyValueEntry([]byte("{not json"), 1), nil)
			},
			wantErr: true,
		},
		{
			name: "settings lookup keys are skipped",
			setupMocks: func(mockProjectsKV, mockSettingsKV *MockKeyValue) {
				mockProjectsKV.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{}), nil)
				mockSettingsKV.On("ListKeys", mock.Anything).Return(NewMockKeyLister([]string{"lookup/some-index", "project-1"}), nil)
				mockSettingsKV.On("Get", mock.Anything, "project-1").Return(
					NewMockKeyValueEntry([]byte(`{"uid":"project-1"}`), 1), nil)
			},
			expectedBaseCount: 0,
			expectedSettCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProjectsKV := &MockKeyValue{}
			mockSettingsKV := &MockKeyValue{}

			tt.setupMocks(mockProjectsKV, mockSettingsKV)

			repo := &NatsRepository{Projects: mockProjectsKV, ProjectSettings: mockSettingsKV}

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

func TestNatsRepository_UpdateProjectBase(t *testing.T) {
	now := time.Now()

	projectUID := "00000000-0000-0000-0000-000000000001"
	oldSlug := "old-slug"
	newSlug := "new-slug"

	makeProjectBase := func(slug string) *models.ProjectBase {
		return &models.ProjectBase{
			UID:       projectUID,
			Slug:      slug,
			Name:      "Test Project",
			CreatedAt: &now,
			UpdatedAt: &now,
		}
	}

	makeProjectEntry := func(slug string) *MockKeyValueEntry {
		data, _ := json.Marshal(makeProjectBase(slug))
		return NewMockKeyValueEntry(data, 5)
	}

	tests := []struct {
		name        string
		payload     *models.ProjectBase
		revision    uint64
		setupMocks  func(*MockKeyValue)
		wantErr     bool
		expectedErr error
	}{
		{
			name:     "slug unchanged - successful update",
			payload:  makeProjectBase(oldSlug),
			revision: 5,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil)
				kv.On("Update", mock.Anything, projectUID, mock.Anything, uint64(5)).Return(uint64(6), nil)
			},
			wantErr: false,
		},
		{
			name:     "slug changed - successful: reserves new, CAS succeeds, removes old with ownership check",
			payload:  makeProjectBase(newSlug),
			revision: 5,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil)
				kv.On("Create", mock.Anything, "slug/"+newSlug, []byte(projectUID)).Return(uint64(2), nil)
				kv.On("Update", mock.Anything, projectUID, mock.Anything, uint64(5)).Return(uint64(6), nil)
				kv.On("Get", mock.Anything, "slug/"+oldSlug).Return(NewMockKeyValueEntry([]byte(projectUID), 1), nil)
				kv.On("Delete", mock.Anything, "slug/"+oldSlug, mock.Anything).Return(nil)
			},
			wantErr: false,
		},
		{
			name:     "slug changed - stale If-Match: reserves new, CAS fails, rolls back new slug",
			payload:  makeProjectBase(newSlug),
			revision: 3,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil)
				kv.On("Create", mock.Anything, "slug/"+newSlug, []byte(projectUID)).Return(uint64(2), nil)
				kv.On("Update", mock.Anything, projectUID, mock.Anything, uint64(3)).Return(uint64(0), errors.New("wrong last sequence"))
				kv.On("Get", mock.Anything, "slug/"+newSlug).Return(NewMockKeyValueEntry([]byte(projectUID), 2), nil)
				kv.On("Delete", mock.Anything, "slug/"+newSlug, mock.Anything).Return(nil)
			},
			wantErr:     true,
			expectedErr: domain.ErrRevisionMismatch,
		},
		{
			name:     "slug changed - new slug already taken by another project",
			payload:  makeProjectBase(newSlug),
			revision: 5,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil)
				kv.On("Create", mock.Anything, "slug/"+newSlug, []byte(projectUID)).Return(uint64(0), jetstream.ErrKeyExists)
				// Idempotency check: entry belongs to a different project
				kv.On("Get", mock.Anything, "slug/"+newSlug).Return(NewMockKeyValueEntry([]byte("other-project-uid"), 9), nil)
			},
			wantErr:     true,
			expectedErr: domain.ErrProjectSlugExists,
		},
		{
			name:     "slug changed - ErrKeyExists but Get fails: returns ErrInternal not ErrProjectSlugExists",
			payload:  makeProjectBase(newSlug),
			revision: 5,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil)
				kv.On("Create", mock.Anything, "slug/"+newSlug, []byte(projectUID)).Return(uint64(0), jetstream.ErrKeyExists)
				kv.On("Get", mock.Anything, "slug/"+newSlug).Return(nil, errors.New("nats store error"))
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name:     "slug changed - idempotent retry (our UID) then CAS fails: does NOT roll back new slug",
			payload:  makeProjectBase(newSlug),
			revision: 3,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil)
				kv.On("Create", mock.Anything, "slug/"+newSlug, []byte(projectUID)).Return(uint64(0), jetstream.ErrKeyExists)
				// Our UID — idempotent retry path (weCreatedReservation stays false)
				kv.On("Get", mock.Anything, "slug/"+newSlug).Return(NewMockKeyValueEntry([]byte(projectUID), 9), nil)
				kv.On("Update", mock.Anything, projectUID, mock.Anything, uint64(3)).Return(uint64(0), errors.New("wrong last sequence"))
				// Delete must NOT be called: we did not create the reservation
			},
			wantErr:     true,
			expectedErr: domain.ErrRevisionMismatch,
		},
		{
			name:     "slug changed - ErrKeyExists but entry already owned by us (idempotent retry): CAS succeeds",
			payload:  makeProjectBase(newSlug),
			revision: 5,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil)
				kv.On("Create", mock.Anything, "slug/"+newSlug, []byte(projectUID)).Return(uint64(0), jetstream.ErrKeyExists)
				// Idempotency check: entry already belongs to this project
				kv.On("Get", mock.Anything, "slug/"+newSlug).Return(NewMockKeyValueEntry([]byte(projectUID), 9), nil)
				kv.On("Update", mock.Anything, projectUID, mock.Anything, uint64(5)).Return(uint64(6), nil)
				kv.On("Get", mock.Anything, "slug/"+oldSlug).Return(NewMockKeyValueEntry([]byte(projectUID), 1), nil)
				kv.On("Delete", mock.Anything, "slug/"+oldSlug, mock.Anything).Return(nil)
			},
			wantErr: false,
		},
		{
			name:     "slug changed - non-CAS error and GetProjectBase re-read fails: reservation kept, returns ErrInternal",
			payload:  makeProjectBase(newSlug),
			revision: 5,
			setupMocks: func(kv *MockKeyValue) {
				// First call: initial GetProjectBase to detect slug change.
				// .Once() ensures the second Get(projectUID) call picks up the next mock.
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil).Once()
				kv.On("Create", mock.Anything, "slug/"+newSlug, []byte(projectUID)).Return(uint64(2), nil)
				kv.On("Update", mock.Anything, projectUID, mock.Anything, uint64(5)).Return(uint64(0), errors.New("nats store error"))
				// Second call: re-read before rollback fails — reservation must be kept.
				kv.On("Get", mock.Anything, projectUID).Return(nil, errors.New("nats store error")).Once()
				// Delete must NOT be called: re-read failed so reservation is kept
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name:     "slug changed - old mapping belongs to different project: skips old delete, update still succeeds",
			payload:  makeProjectBase(newSlug),
			revision: 5,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil)
				kv.On("Create", mock.Anything, "slug/"+newSlug, []byte(projectUID)).Return(uint64(2), nil)
				kv.On("Update", mock.Anything, projectUID, mock.Anything, uint64(5)).Return(uint64(6), nil)
				kv.On("Get", mock.Anything, "slug/"+oldSlug).Return(NewMockKeyValueEntry([]byte("other-project-uid"), 1), nil)
				// Delete must NOT be called for the old slug
			},
			wantErr: false,
		},
		{
			name:     "slug changed - reserving the new slug fails with a store error: returns ErrInternal",
			payload:  makeProjectBase(newSlug),
			revision: 5,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil)
				kv.On("Create", mock.Anything, "slug/"+newSlug, []byte(projectUID)).Return(uint64(0), errors.New("nats store error"))
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name:     "slug changed - stale If-Match and the slug rollback fails: still returns revision mismatch",
			payload:  makeProjectBase(newSlug),
			revision: 3,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil)
				kv.On("Create", mock.Anything, "slug/"+newSlug, []byte(projectUID)).Return(uint64(2), nil)
				kv.On("Update", mock.Anything, projectUID, mock.Anything, uint64(3)).Return(uint64(0), errors.New("wrong last sequence"))
				kv.On("Get", mock.Anything, "slug/"+newSlug).Return(nil, errors.New("nats store error"))
			},
			wantErr:     true,
			expectedErr: domain.ErrRevisionMismatch,
		},
		{
			name:     "slug changed - non-CAS error and re-read shows slug unchanged: rolls back the reservation",
			payload:  makeProjectBase(newSlug),
			revision: 5,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil).Once()
				kv.On("Create", mock.Anything, "slug/"+newSlug, []byte(projectUID)).Return(uint64(2), nil)
				kv.On("Update", mock.Anything, projectUID, mock.Anything, uint64(5)).Return(uint64(0), errors.New("nats store error"))
				// Re-read confirms the base still carries the old slug, so the new reservation is released.
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil).Once()
				kv.On("Get", mock.Anything, "slug/"+newSlug).Return(NewMockKeyValueEntry([]byte(projectUID), 2), nil)
				kv.On("Delete", mock.Anything, "slug/"+newSlug, mock.Anything).Return(errors.New("nats store error"))
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name:     "slug changed - removing the old slug mapping fails: update still succeeds",
			payload:  makeProjectBase(newSlug),
			revision: 5,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil)
				kv.On("Create", mock.Anything, "slug/"+newSlug, []byte(projectUID)).Return(uint64(2), nil)
				kv.On("Update", mock.Anything, projectUID, mock.Anything, uint64(5)).Return(uint64(6), nil)
				kv.On("Get", mock.Anything, "slug/"+oldSlug).Return(nil, errors.New("nats store error"))
			},
			wantErr: false,
		},
		{
			name:     "slug unchanged - CAS fails with wrong last sequence",
			payload:  makeProjectBase(oldSlug),
			revision: 3,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(oldSlug), nil)
				kv.On("Update", mock.Anything, projectUID, mock.Anything, uint64(3)).Return(uint64(0), errors.New("wrong last sequence"))
			},
			wantErr:     true,
			expectedErr: domain.ErrRevisionMismatch,
		},
		{
			name:     "project not found",
			payload:  makeProjectBase(oldSlug),
			revision: 5,
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, projectUID).Return(nil, jetstream.ErrKeyNotFound)
			},
			wantErr:     true,
			expectedErr: domain.ErrProjectNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProjectsKV := &MockKeyValue{}
			mockSettingsKV := &MockKeyValue{}

			tt.setupMocks(mockProjectsKV)

			repo := &NatsRepository{Projects: mockProjectsKV, ProjectSettings: mockSettingsKV}
			err := repo.UpdateProjectBase(context.Background(), tt.payload, tt.revision)

			if tt.wantErr {
				require.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err)
				}
			} else {
				assert.NoError(t, err)
			}

			mockProjectsKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_DeleteProject(t *testing.T) {
	now := time.Now()

	projectUID := "00000000-0000-0000-0000-000000000001"
	slug := "test-slug"

	makeProjectEntry := func() *MockKeyValueEntry {
		data, _ := json.Marshal(&models.ProjectBase{
			UID:       projectUID,
			Slug:      slug,
			Name:      "Test Project",
			CreatedAt: &now,
			UpdatedAt: &now,
		})
		return NewMockKeyValueEntry(data, 7)
	}

	tests := []struct {
		name        string
		revision    uint64
		setupMocks  func(*MockKeyValue, *MockKeyValue)
		wantErr     bool
		expectedErr error
	}{
		{
			name:     "successful delete: CAS base, verifies slug ownership, deletes settings",
			revision: 7,
			setupMocks: func(projectsKV, settingsKV *MockKeyValue) {
				projectsKV.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(), nil)
				projectsKV.On("Delete", mock.Anything, projectUID, mock.Anything).Return(nil)
				projectsKV.On("Get", mock.Anything, "slug/"+slug).Return(NewMockKeyValueEntry([]byte(projectUID), 3), nil)
				projectsKV.On("Delete", mock.Anything, "slug/"+slug, mock.Anything).Return(nil)
				settingsKV.On("Delete", mock.Anything, projectUID).Return(nil)
			},
			wantErr: false,
		},
		{
			name:     "slug mapping belongs to different project: skips slug delete, still deletes settings",
			revision: 7,
			setupMocks: func(projectsKV, settingsKV *MockKeyValue) {
				projectsKV.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(), nil)
				projectsKV.On("Delete", mock.Anything, projectUID, mock.Anything).Return(nil)
				projectsKV.On("Get", mock.Anything, "slug/"+slug).Return(NewMockKeyValueEntry([]byte("other-project-uid"), 3), nil)
				// Delete must NOT be called for the slug mapping
				settingsKV.On("Delete", mock.Anything, projectUID).Return(nil)
			},
			wantErr: false,
		},
		{
			name:     "revision mismatch on base delete",
			revision: 3,
			setupMocks: func(projectsKV, settingsKV *MockKeyValue) {
				projectsKV.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(), nil)
				projectsKV.On("Delete", mock.Anything, projectUID, mock.Anything).Return(errors.New("wrong last sequence"))
			},
			wantErr:     true,
			expectedErr: domain.ErrRevisionMismatch,
		},
		{
			name:     "project not found",
			revision: 7,
			setupMocks: func(projectsKV, settingsKV *MockKeyValue) {
				projectsKV.On("Get", mock.Anything, projectUID).Return(nil, jetstream.ErrKeyNotFound)
			},
			wantErr:     true,
			expectedErr: domain.ErrProjectNotFound,
		},
		{
			name:     "slug Get returns non-NotFound error: DeleteProject returns ErrInternal",
			revision: 7,
			setupMocks: func(projectsKV, settingsKV *MockKeyValue) {
				projectsKV.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(), nil)
				projectsKV.On("Delete", mock.Anything, projectUID, mock.Anything).Return(nil)
				projectsKV.On("Get", mock.Anything, "slug/"+slug).Return(nil, errors.New("nats store error"))
				// settings delete is not reached because slug mapping delete returns ErrInternal
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name:     "slug LastRevision CAS delete fails: DeleteProject returns ErrInternal",
			revision: 7,
			setupMocks: func(projectsKV, settingsKV *MockKeyValue) {
				projectsKV.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(), nil)
				projectsKV.On("Delete", mock.Anything, projectUID, mock.Anything).Return(nil)
				projectsKV.On("Get", mock.Anything, "slug/"+slug).Return(NewMockKeyValueEntry([]byte(projectUID), 3), nil)
				projectsKV.On("Delete", mock.Anything, "slug/"+slug, mock.Anything).Return(errors.New("nats store error"))
				// settings delete is not reached because slug mapping delete returns ErrInternal
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name:     "slug already absent (ErrKeyNotFound): deleteProjectSlugMapping returns nil",
			revision: 7,
			setupMocks: func(projectsKV, settingsKV *MockKeyValue) {
				projectsKV.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(), nil)
				projectsKV.On("Delete", mock.Anything, projectUID, mock.Anything).Return(nil)
				projectsKV.On("Get", mock.Anything, "slug/"+slug).Return(nil, jetstream.ErrKeyNotFound)
				settingsKV.On("Delete", mock.Anything, projectUID).Return(nil)
			},
			wantErr: false,
		},
		{
			name:     "slug mapping vanishes before the CAS delete (ErrKeyNotFound): treated as already removed",
			revision: 7,
			setupMocks: func(projectsKV, settingsKV *MockKeyValue) {
				projectsKV.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(), nil)
				projectsKV.On("Delete", mock.Anything, projectUID, mock.Anything).Return(nil)
				projectsKV.On("Get", mock.Anything, "slug/"+slug).Return(NewMockKeyValueEntry([]byte(projectUID), 3), nil)
				projectsKV.On("Delete", mock.Anything, "slug/"+slug, mock.Anything).Return(jetstream.ErrKeyNotFound)
				settingsKV.On("Delete", mock.Anything, projectUID).Return(nil)
			},
			wantErr: false,
		},
		{
			name:     "base delete fails with a non-revision error: ErrInternal",
			revision: 7,
			setupMocks: func(projectsKV, settingsKV *MockKeyValue) {
				projectsKV.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(), nil)
				projectsKV.On("Delete", mock.Anything, projectUID, mock.Anything).Return(errors.New("nats store error"))
			},
			wantErr:     true,
			expectedErr: domain.ErrInternal,
		},
		{
			name:     "settings delete fails: ErrInternal",
			revision: 7,
			setupMocks: func(projectsKV, settingsKV *MockKeyValue) {
				projectsKV.On("Get", mock.Anything, projectUID).Return(makeProjectEntry(), nil)
				projectsKV.On("Delete", mock.Anything, projectUID, mock.Anything).Return(nil)
				projectsKV.On("Get", mock.Anything, "slug/"+slug).Return(NewMockKeyValueEntry([]byte(projectUID), 3), nil)
				projectsKV.On("Delete", mock.Anything, "slug/"+slug, mock.Anything).Return(nil)
				settingsKV.On("Delete", mock.Anything, projectUID).Return(errors.New("nats store error"))
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

			repo := &NatsRepository{Projects: mockProjectsKV, ProjectSettings: mockSettingsKV}
			err := repo.DeleteProject(context.Background(), projectUID, tt.revision)

			if tt.wantErr {
				require.Error(t, err)
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

func TestNatsRepository_ProjectSlugExists(t *testing.T) {
	tests := []struct {
		name       string
		setupMocks func(*MockKeyValue)
		want       bool
		wantErr    error
	}{
		{
			name: "slug exists",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, "slug/test-slug").Return(NewMockKeyValueEntry([]byte("project-uid"), 1), nil)
			},
			want: true,
		},
		{
			name: "slug missing is not an error",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, "slug/test-slug").Return(nil, jetstream.ErrKeyNotFound)
			},
			want: false,
		},
		{
			name: "store error is returned",
			setupMocks: func(kv *MockKeyValue) {
				kv.On("Get", mock.Anything, "slug/test-slug").Return(nil, errors.New("nats error"))
			},
			want:    false,
			wantErr: domain.ErrInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKV := &MockKeyValue{}
			tt.setupMocks(mockKV)
			repo := &NatsRepository{Projects: mockKV, ProjectSettings: &MockKeyValue{}}

			got, err := repo.ProjectSlugExists(context.Background(), "test-slug")

			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.want, got)
			mockKV.AssertExpectations(t)
		})
	}
}

func TestNatsRepository_UpdateProjectSettings(t *testing.T) {
	settings := &models.ProjectSettings{UID: "test-project-uid", MissionStatement: "Test mission"}

	tests := []struct {
		name      string
		updateErr error
		wantErr   error
	}{
		{name: "success", updateErr: nil, wantErr: nil},
		{name: "revision mismatch", updateErr: errors.New("nats: wrong last sequence: 5"), wantErr: domain.ErrRevisionMismatch},
		{name: "key exists conflict", updateErr: jetstream.ErrKeyExists, wantErr: domain.ErrRevisionMismatch},
		{name: "other store error", updateErr: errors.New("nats error"), wantErr: domain.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSettingsKV := &MockKeyValue{}
			mockSettingsKV.On("Update", mock.Anything, "test-project-uid", mock.Anything, uint64(5)).Return(uint64(6), tt.updateErr)
			repo := &NatsRepository{Projects: &MockKeyValue{}, ProjectSettings: mockSettingsKV}

			err := repo.UpdateProjectSettings(context.Background(), settings, 5)

			assert.Equal(t, tt.wantErr, err)
			mockSettingsKV.AssertExpectations(t)
		})
	}
}
