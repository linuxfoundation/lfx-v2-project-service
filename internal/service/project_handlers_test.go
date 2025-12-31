// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

				return &ProjectsService{
					ProjectRepository: mockRepo,
					MessageBuilder:    &domain.MockMessageBuilder{},
					Auth:              &auth.MockJWTAuth{},
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
