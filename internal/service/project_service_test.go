// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	domainmocks "github.com/linuxfoundation/lfx-v2-project-service/internal/domain/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewProjectsService(t *testing.T) {
	tests := []struct {
		name   string
		config ServiceConfig
	}{
		{
			name:   "zero config",
			config: ServiceConfig{},
		},
		{
			name: "config is stored as given",
			config: ServiceConfig{
				SkipEtagValidation:  true,
				LFXSelfServeBaseURL: "https://app.example.com",
				EmailsEnabled:       true,
				InvitesEnabled:      true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewProjectsService(tt.config, ServiceDeps{})

			assert.NotNil(t, service)
			assert.Equal(t, tt.config, service.Config)
			assert.Nil(t, service.Publisher) // nil because no deps provided
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
				mockUserReader := &domainmocks.MockUserReader{}
				mockBuilder := &domainmocks.MockMessageBuilder{}
				resolver := NewUserResolver(mockUserReader)
				return &ProjectsService{
					ProjectRepository:  &domainmocks.MockProjectRepository{},
					DocumentRepository: &domainmocks.MockDocumentRepository{},
					LinkRepository:     &domainmocks.MockLinkRepository{},
					FolderRepository:   &domainmocks.MockFolderRepository{},
					Publisher:          mockBuilder,
					Sender:             mockBuilder,
					UserReader:         mockUserReader,
					Resolver:           resolver,
					Dispatcher:         NewNotificationDispatcher(mockBuilder, resolver, false, false),
				}
			},
			expectedReady: true,
		},
		{
			name: "service not ready - missing repository",
			setupService: func() *ProjectsService {
				return &ProjectsService{
					ProjectRepository: nil,
					Publisher:         &domainmocks.MockMessageBuilder{},
					Sender:            &domainmocks.MockMessageBuilder{},
				}
			},
			expectedReady: false,
		},
		{
			name: "service not ready - missing publisher",
			setupService: func() *ProjectsService {
				return &ProjectsService{
					ProjectRepository: &domainmocks.MockProjectRepository{},
					Publisher:         nil,
					Sender:            &domainmocks.MockMessageBuilder{},
				}
			},
			expectedReady: false,
		},
		{
			name: "service not ready - missing both publisher and sender",
			setupService: func() *ProjectsService {
				return &ProjectsService{
					ProjectRepository: nil,
					Publisher:         nil,
					Sender:            nil,
				}
			},
			expectedReady: false,
		},
		{
			name: "service ready without auth (auth is not checked in ServiceReady)",
			setupService: func() *ProjectsService {
				mockUserReader := &domainmocks.MockUserReader{}
				mockBuilder := &domainmocks.MockMessageBuilder{}
				resolver := NewUserResolver(mockUserReader)
				return &ProjectsService{
					ProjectRepository:  &domainmocks.MockProjectRepository{},
					DocumentRepository: &domainmocks.MockDocumentRepository{},
					LinkRepository:     &domainmocks.MockLinkRepository{},
					FolderRepository:   &domainmocks.MockFolderRepository{},
					Publisher:          mockBuilder,
					Sender:             mockBuilder,
					UserReader:         mockUserReader,
					Resolver:           resolver,
					Dispatcher:         NewNotificationDispatcher(mockBuilder, resolver, false, false),
				}
			},
			expectedReady: true,
		},
		{
			name: "service not ready - missing dispatcher",
			setupService: func() *ProjectsService {
				mockUserReader := &domainmocks.MockUserReader{}
				mockBuilder := &domainmocks.MockMessageBuilder{}
				return &ProjectsService{
					ProjectRepository:  &domainmocks.MockProjectRepository{},
					DocumentRepository: &domainmocks.MockDocumentRepository{},
					LinkRepository:     &domainmocks.MockLinkRepository{},
					FolderRepository:   &domainmocks.MockFolderRepository{},
					Publisher:          mockBuilder,
					Sender:             mockBuilder,
					UserReader:         mockUserReader,
					Resolver:           NewUserResolver(mockUserReader),
					Dispatcher:         nil,
				}
			},
			expectedReady: false,
		},
		{
			name: "service not ready - missing user reader",
			setupService: func() *ProjectsService {
				mockBuilder := &domainmocks.MockMessageBuilder{}
				return &ProjectsService{
					ProjectRepository:  &domainmocks.MockProjectRepository{},
					DocumentRepository: &domainmocks.MockDocumentRepository{},
					LinkRepository:     &domainmocks.MockLinkRepository{},
					FolderRepository:   &domainmocks.MockFolderRepository{},
					Publisher:          mockBuilder,
					Sender:             mockBuilder,
					UserReader:         nil,
				}
			},
			expectedReady: false,
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
		mockRepo := &domainmocks.MockProjectRepository{}
		mockBuilder := &domainmocks.MockMessageBuilder{}

		service := NewProjectsService(ServiceConfig{}, ServiceDeps{
			ProjectRepository: mockRepo,
			Publisher:         mockBuilder,
			Sender:            mockBuilder,
		})

		// Verify dependencies are correctly set
		assert.Same(t, mockRepo, service.ProjectRepository)
		assert.Same(t, mockBuilder, service.Publisher)
	})
}

func TestProjectsService_Interfaces(t *testing.T) {
	t.Run("service implements MessageHandler interface", func(t *testing.T) {
		service := &ProjectsService{}
		assert.Implements(t, (*domain.MessageHandler)(nil), service)
	})
}

func TestProjectsService_publishIndexer(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		msg     any
		mockErr error
	}{
		{
			name:    "dispatches to publisher with correct subject and message",
			subject: "test.subject",
			msg:     "test-message",
		},
		{
			name:    "swallows publisher error without panicking",
			subject: "test.subject",
			msg:     "test-message",
			mockErr: errors.New("nats unavailable"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan struct{})
			mockPub := &domainmocks.MockMessageBuilder{}
			mockPub.On("SendIndexerMessage", mock.Anything, tt.subject, tt.msg).
				Return(tt.mockErr).
				Run(func(_ mock.Arguments) { close(done) })

			svc := &ProjectsService{Publisher: mockPub}
			svc.publishIndexer(context.Background(), tt.subject, tt.msg)

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("publishIndexer goroutine did not call SendIndexerMessage within timeout")
			}

			mockPub.AssertExpectations(t)
		})
	}
}

// Setup helper for common test scenarios
func setupServiceForTesting() (*ProjectsService, *domainmocks.MockProjectRepository, *domainmocks.MockMessageBuilder) {
	mockRepo := &domainmocks.MockProjectRepository{}
	mockBuilder := &domainmocks.MockMessageBuilder{}
	mockUserReader := &domainmocks.MockUserReader{}
	resolver := NewUserResolver(mockUserReader)

	svc := NewProjectsService(ServiceConfig{}, ServiceDeps{
		ProjectRepository:  mockRepo,
		DocumentRepository: &domainmocks.MockDocumentRepository{},
		LinkRepository:     &domainmocks.MockLinkRepository{},
		FolderRepository:   &domainmocks.MockFolderRepository{},
		Publisher:          mockBuilder,
		Sender:             mockBuilder,
		UserReader:         mockUserReader,
		Resolver:           resolver,
		Dispatcher:         NewNotificationDispatcher(mockBuilder, resolver, false, false),
	})

	return svc, mockRepo, mockBuilder
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
