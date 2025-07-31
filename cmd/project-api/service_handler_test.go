// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// setupServiceForMessageHandling creates a service for testing message handlers
func setupServiceForMessageHandling() (*service.ProjectsService, *domain.MockProjectRepository) {
	mockRepo := &domain.MockProjectRepository{}
	mockMsg := &domain.MockMessageBuilder{}

	return &service.ProjectsService{
		ProjectRepository: mockRepo,
		MessageBuilder:    mockMsg,
	}, mockRepo
}

// TestHandleProjectGetName tests the HandleProjectGetName function.
func TestHandleProjectGetName(t *testing.T) {
	tests := []struct {
		name          string
		projectID     string
		setupMocks    func(*domain.MockProjectRepository)
		expectedName  string
		expectedError bool
	}{
		{
			name:      "success",
			projectID: "550e8400-e29b-41d4-a716-446655440000", // Valid UUID
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				now := time.Now()
				project := &models.ProjectBase{
					UID:       "550e8400-e29b-41d4-a716-446655440000",
					Slug:      "test-project",
					Name:      "Test Project",
					Public:    true,
					CreatedAt: &now,
					UpdatedAt: &now,
				}
				mockRepo.On("GetProjectBase", mock.Anything, "550e8400-e29b-41d4-a716-446655440000").Return(project, nil)
			},
			expectedName:  "Test Project",
			expectedError: false,
		},
		{
			name:      "invalid UUID",
			projectID: "invalid-uuid",
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				// No mocks needed for invalid UUID case
			},
			expectedError: true,
		},
		{
			name:      "project not found",
			projectID: "550e8400-e29b-41d4-a716-446655440000",
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectBase", mock.Anything, "550e8400-e29b-41d4-a716-446655440000").Return(nil, domain.ErrProjectNotFound)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo := setupServiceForMessageHandling()
			tt.setupMocks(mockRepo)

			msg := domain.NewMockMessage([]byte(tt.projectID), constants.ProjectGetNameSubject)

			result, err := service.HandleProjectGetName(msg)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedName, string(result))
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestHandleProjectSlugToUID tests the HandleProjectSlugToUID function.
func TestHandleProjectSlugToUID(t *testing.T) {
	tests := []struct {
		name          string
		projectSlug   string
		setupMocks    func(*domain.MockProjectRepository)
		expectedUID   string
		expectedError bool
	}{
		{
			name:        "success",
			projectSlug: "test-project",
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				projectUID := "550e8400-e29b-41d4-a716-446655440000"
				mockRepo.On("GetProjectUIDFromSlug", mock.Anything, "test-project").Return(projectUID, nil)
			},
			expectedUID:   "550e8400-e29b-41d4-a716-446655440000",
			expectedError: false,
		},
		{
			name:        "project not found",
			projectSlug: "nonexistent-project",
			setupMocks: func(mockRepo *domain.MockProjectRepository) {
				mockRepo.On("GetProjectUIDFromSlug", mock.Anything, "nonexistent-project").Return("", domain.ErrProjectNotFound)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo := setupServiceForMessageHandling()
			tt.setupMocks(mockRepo)

			msg := domain.NewMockMessage([]byte(tt.projectSlug), constants.ProjectSlugToUIDSubject)

			result, err := service.HandleProjectSlugToUID(msg)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedUID, string(result))
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestHandleMessage tests the HandleMessage function.
func TestHandleMessage(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		data    []byte
		wantNil bool // if true, expect Respond(nil)
	}{
		{
			name:    "project get name routes and responds",
			subject: constants.ProjectGetNameSubject,
			data:    []byte("550e8400-e29b-41d4-a716-446655440000"),
			wantNil: false,
		},
		{
			name:    "project slug to UID routes and responds",
			subject: constants.ProjectSlugToUIDSubject,
			data:    []byte("test-slug"),
			wantNil: false,
		},
		{
			name:    "unknown subject responds nil",
			subject: "unknown.subject",
			data:    []byte("test-data"),
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo := setupServiceForMessageHandling()
			msg := domain.NewMockMessage(tt.data, tt.subject)

			if !tt.wantNil {
				// Set up mock expectations for known subjects
				switch tt.subject {
				case constants.ProjectGetNameSubject:
					now := time.Now()
					project := &models.ProjectBase{
						UID:       "550e8400-e29b-41d4-a716-446655440000",
						Name:      "Test Project",
						CreatedAt: &now,
						UpdatedAt: &now,
					}
					mockRepo.On("GetProjectBase", mock.Anything, mock.Anything).Return(project, nil)
				case constants.ProjectSlugToUIDSubject:
					mockRepo.On("GetProjectUIDFromSlug", mock.Anything, mock.Anything).Return("test-uid", nil)
				default:
					// No mock expectations for unknown subjects
				}
				msg.On("Respond", mock.Anything).Return(nil).Once()
			} else {
				msg.On("Respond", []byte(nil)).Return(nil).Once()
			}

			assert.NotPanics(t, func() {
				service.HandleMessage(msg)
			})

			msg.AssertExpectations(t)
		})
	}
}
