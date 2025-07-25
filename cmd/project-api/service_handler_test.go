// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockNatsMsg struct {
	mock.Mock
	data    []byte
	subject string
}

func (m *MockNatsMsg) Respond(data []byte) error {
	args := m.Called(data)
	return args.Error(0)
}

func (m *MockNatsMsg) Data() []byte {
	return m.data
}

func (m *MockNatsMsg) Subject() string {
	return m.subject
}

// CreateMockNatsMsg creates a mock NATS message that can be used in tests
func CreateMockNatsMsg(data []byte) *MockNatsMsg {
	msg := MockNatsMsg{
		data: data,
	}
	return &msg
}

// CreateMockNatsMsgWithSubject creates a mock NATS message with a specific subject
func CreateMockNatsMsgWithSubject(data []byte, subject string) *MockNatsMsg {
	msg := MockNatsMsg{
		data:    data,
		subject: subject,
	}
	return &msg
}

// TestHandleProjectGetName tests the [HandleProjectGetName] function.
func TestHandleProjectGetName(t *testing.T) {
	tests := []struct {
		name          string
		projectID     string
		setupMocks    func(*ProjectsService, *MockNatsMsg)
		expectedName  string
		expectedError bool
	}{
		{
			name:      "success",
			projectID: "550e8400-e29b-41d4-a716-446655440000", // Valid UUID
			setupMocks: func(service *ProjectsService, _ *MockNatsMsg) {
				projectData := `{"uid":"550e8400-e29b-41d4-a716-446655440000","slug":"test-project","name":"Test Project","description":"Test description","public":true,"parent_uid":"","auditors":["user1"],"writers":["user2"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				service.kvStores.Projects.(*nats.MockKeyValue).On("Get", mock.Anything, "550e8400-e29b-41d4-a716-446655440000").Return(nats.NewMockKeyValueEntry([]byte(projectData), 123), nil)
			},
			expectedName:  "Test Project",
			expectedError: false,
		},
		{
			name:      "invalid UUID",
			projectID: "invalid-uuid",
			setupMocks: func(_ *ProjectsService, _ *MockNatsMsg) {
				// No mocks needed for invalid UUID case
			},
			expectedError: true,
		},
		{
			name:      "NATS KV not initialized",
			projectID: "550e8400-e29b-41d4-a716-446655440000",
			setupMocks: func(service *ProjectsService, _ *MockNatsMsg) {
				service.kvStores.Projects = nil
			},
			expectedError: true,
		},
		{
			name:      "error getting project",
			projectID: "550e8400-e29b-41d4-a716-446655440000",
			setupMocks: func(service *ProjectsService, _ *MockNatsMsg) {
				service.kvStores.Projects.(*nats.MockKeyValue).On("Get", mock.Anything, "550e8400-e29b-41d4-a716-446655440000").Return(&nats.MockKeyValueEntry{}, assert.AnError)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			msg := CreateMockNatsMsg([]byte(tt.projectID))
			tt.setupMocks(service, msg)

			// Test that the function doesn't panic and handles the message
			assert.NotPanics(t, func() {
				_, err := service.HandleProjectGetName(msg)
				if tt.expectedError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})

			// For success case, verify that the mock was called as expected
			if tt.name == "success" {
				service.kvStores.Projects.(*nats.MockKeyValue).AssertExpectations(t)
			}
		})
	}
}

// TestHandleProjectSlugToUID tests the [HandleProjectSlugToUID] function.
func TestHandleProjectSlugToUID(t *testing.T) {
	tests := []struct {
		name          string
		projectSlug   string
		setupMocks    func(*ProjectsService, *MockNatsMsg)
		expectedUID   string
		expectedError bool
	}{
		{
			name:        "success",
			projectSlug: "test-project",
			setupMocks: func(service *ProjectsService, _ *MockNatsMsg) {
				projectUID := "550e8400-e29b-41d4-a716-446655440000"
				service.kvStores.Projects.(*nats.MockKeyValue).On("Get", mock.Anything, "slug/test-project").Return(nats.NewMockKeyValueEntry([]byte(projectUID), 123), nil)
			},
			expectedUID:   "550e8400-e29b-41d4-a716-446655440000",
			expectedError: false,
		},
		{
			name:        "NATS KV not initialized",
			projectSlug: "test-project",
			setupMocks: func(service *ProjectsService, _ *MockNatsMsg) {
				service.kvStores.Projects = nil
			},
			expectedError: true,
		},
		{
			name:        "error getting project",
			projectSlug: "test-project",
			setupMocks: func(service *ProjectsService, _ *MockNatsMsg) {
				service.kvStores.Projects.(*nats.MockKeyValue).On("Get", mock.Anything, "slug/test-project").Return(&nats.MockKeyValueEntry{}, assert.AnError)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			msg := CreateMockNatsMsg([]byte(tt.projectSlug))
			tt.setupMocks(service, msg)

			// Test that the function doesn't panic and handles the message
			assert.NotPanics(t, func() {
				uid, err := service.HandleProjectSlugToUID(msg)
				if tt.expectedError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.expectedUID, string(uid))
				}
			})

			// For success case, verify that the mock was called as expected
			if tt.name == "success" {
				service.kvStores.Projects.(*nats.MockKeyValue).AssertExpectations(t)
			}
		})
	}
}

// TestHandleNatsMessage tests the [HandleNatsMessage] function.
func TestHandleNatsMessage(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		data    []byte
		wantNil bool // if true, expect Respond(nil)
	}{
		{
			name:    "project get name routes and responds",
			subject: constants.ProjectGetNameSubject,
			data:    []byte("some-id"),
			wantNil: false,
		},
		{
			name:    "project slug to UID routes and responds",
			subject: constants.ProjectSlugToUIDSubject,
			data:    []byte("some-slug"),
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
			service := setupService()
			msg := CreateMockNatsMsgWithSubject(tt.data, tt.subject)
			if tt.wantNil {
				msg.On("Respond", []byte(nil)).Return(nil).Once()
			} else {
				msg.On("Respond", mock.Anything).Return(nil).Once()
				// Set up a generic expectation for the Get method to avoid mock panics
				service.kvStores.Projects.(*nats.MockKeyValue).On("Get", mock.Anything, mock.Anything).Return(&nats.MockKeyValueEntry{}, nil)
			}

			assert.NotPanics(t, func() {
				service.HandleNatsMessage(msg)
			})

			msg.AssertExpectations(t)
		})
	}
}
