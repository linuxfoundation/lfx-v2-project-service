// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"testing"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewProjectsService(t *testing.T) {
	tests := []struct {
		name string
		auth auth.IJWTAuth
	}{
		{
			name: "create service with valid dependencies",
			auth: &auth.MockJWTAuth{},
		},
		{
			name: "create service with nil auth",
			auth: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewProjectsService(tt.auth)

			assert.NotNil(t, service)
			assert.Equal(t, tt.auth, service.Auth)
			assert.Nil(t, service.MessageBuilder) // Should be set separately
		})
	}
}

func TestProjectsService_ServiceReady(t *testing.T) {
	tests := []struct {
		name          string
		setupService  func() *ProjectsService
		expectedReady bool
	}{
		{
			name: "service ready with all dependencies",
			setupService: func() *ProjectsService {
				return &ProjectsService{
					ProjectRepository: &domain.MockProjectRepository{},
					MessageBuilder:    &domain.MockMessageBuilder{},
					Auth:              &auth.MockJWTAuth{},
				}
			},
			expectedReady: true,
		},
		{
			name: "service not ready - missing repository",
			setupService: func() *ProjectsService {
				return &ProjectsService{
					ProjectRepository: nil,
					MessageBuilder:    &domain.MockMessageBuilder{},
					Auth:              &auth.MockJWTAuth{},
				}
			},
			expectedReady: false,
		},
		{
			name: "service not ready - missing message builder",
			setupService: func() *ProjectsService {
				return &ProjectsService{
					ProjectRepository: &domain.MockProjectRepository{},
					MessageBuilder:    nil,
					Auth:              &auth.MockJWTAuth{},
				}
			},
			expectedReady: false,
		},
		{
			name: "service not ready - missing both critical dependencies",
			setupService: func() *ProjectsService {
				return &ProjectsService{
					ProjectRepository: nil,
					MessageBuilder:    nil,
					Auth:              &auth.MockJWTAuth{},
				}
			},
			expectedReady: false,
		},
		{
			name: "service ready without auth (auth is not checked in ServiceReady)",
			setupService: func() *ProjectsService {
				return &ProjectsService{
					ProjectRepository: &domain.MockProjectRepository{},
					MessageBuilder:    &domain.MockMessageBuilder{},
					Auth:              nil,
				}
			},
			expectedReady: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := tt.setupService()
			ready := service.ServiceReady()
			assert.Equal(t, tt.expectedReady, ready)
		})
	}
}

func TestProjectsService_Dependencies(t *testing.T) {
	t.Run("service maintains dependency references", func(t *testing.T) {
		mockRepo := &domain.MockProjectRepository{}
		mockAuth := &auth.MockJWTAuth{}
		mockBuilder := &domain.MockMessageBuilder{}

		service := NewProjectsService(mockAuth)
		service.ProjectRepository = mockRepo
		service.MessageBuilder = mockBuilder

		// Verify dependencies are correctly set
		assert.Same(t, mockRepo, service.ProjectRepository)
		assert.Same(t, mockAuth, service.Auth)
		assert.Same(t, mockBuilder, service.MessageBuilder)
	})
}

func TestProjectsService_Interfaces(t *testing.T) {
	t.Run("service implements MessageHandler interface", func(t *testing.T) {
		service := &ProjectsService{}
		assert.Implements(t, (*domain.MessageHandler)(nil), service)
	})
}

// Setup helper for common test scenarios
func setupServiceForTesting() (*ProjectsService, *domain.MockProjectRepository, *domain.MockMessageBuilder, *auth.MockJWTAuth) {
	mockRepo := &domain.MockProjectRepository{}
	mockBuilder := &domain.MockMessageBuilder{}
	mockAuth := &auth.MockJWTAuth{}

	service := NewProjectsService(mockAuth)
	service.ProjectRepository = mockRepo
	service.MessageBuilder = mockBuilder

	return service, mockRepo, mockBuilder, mockAuth
}

// Mock message for testing
type mockMessage struct {
	subject string
	data    []byte
	mock.Mock
}

func (m *mockMessage) Subject() string {
	return m.subject
}

func (m *mockMessage) Data() []byte {
	return m.data
}

func (m *mockMessage) Respond(data []byte) error {
	args := m.Called(data)
	return args.Error(0)
}

func newMockMessage(subject string, data []byte) *mockMessage {
	return &mockMessage{
		subject: subject,
		data:    data,
	}
}
