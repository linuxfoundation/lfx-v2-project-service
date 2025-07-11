// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockNatsMsg struct {
	mock.Mock
	data []byte
}

func (m *MockNatsMsg) Respond(data []byte) error {
	args := m.Called(data)
	return args.Error(0)
}

func (m *MockNatsMsg) Data() []byte {
	return m.data
}

// CreateMockNatsMsg creates a mock NATS message that can be used in tests
func CreateMockNatsMsg(data []byte) *MockNatsMsg {
	msg := MockNatsMsg{
		data: data,
	}
	return &msg
}

func TestHandleProjectGetName(t *testing.T) {
	tests := []struct {
		name          string
		projectID     string
		setupMocks    func(*ProjectsService, *MockNatsMsg)
		expectedError bool
	}{
		{
			name:      "success",
			projectID: "550e8400-e29b-41d4-a716-446655440000", // Valid UUID
			setupMocks: func(service *ProjectsService, msg *MockNatsMsg) {
				projectData := `{"uid":"550e8400-e29b-41d4-a716-446655440000","slug":"test-project","name":"Test Project","description":"Test description","managers":["user1"],"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-01-01T00:00:00Z"}`
				service.projectsKV.(*MockKeyValue).On("Get", mock.Anything, "550e8400-e29b-41d4-a716-446655440000").Return(&MockKeyValueEntry{value: []byte(projectData)}, nil)
				msg.On("Respond", []byte(projectData)).Return(nil)
			},
			expectedError: false,
		},
		{
			name:      "invalid UUID",
			projectID: "invalid-uuid",
			setupMocks: func(service *ProjectsService, msg *MockNatsMsg) {
				// No mocks needed for invalid UUID case
				msg.On("Respond", []byte(nil)).Return(nil)
			},
			expectedError: false,
		},
		{
			name:      "NATS KV not initialized",
			projectID: "550e8400-e29b-41d4-a716-446655440000",
			setupMocks: func(service *ProjectsService, msg *MockNatsMsg) {
				service.projectsKV = nil
				msg.On("Respond", []byte(nil)).Return(nil)
			},
			expectedError: false,
		},
		{
			name:      "error getting project",
			projectID: "550e8400-e29b-41d4-a716-446655440000",
			setupMocks: func(service *ProjectsService, msg *MockNatsMsg) {
				service.projectsKV.(*MockKeyValue).On("Get", mock.Anything, "550e8400-e29b-41d4-a716-446655440000").Return(&MockKeyValueEntry{}, assert.AnError)
				msg.On("Respond", []byte(nil)).Return(nil)
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			msg := CreateMockNatsMsg([]byte(tt.projectID))
			tt.setupMocks(service, msg)

			// Test that the function doesn't panic and handles the message
			assert.NotPanics(t, func() {
				service.HandleProjectGetName(msg)
			})

			msg.AssertExpectations(t)

			// For success case, verify that the mock was called as expected
			if tt.name == "success" {
				service.projectsKV.(*MockKeyValue).AssertExpectations(t)
			}

			// Note: The handler functions call msg.Respond() as required before returning.
			// We can see this from the error logs in the test output when the real nats.Msg.Respond()
			// is called (e.g., "error responding to NATS message").
			// In a more sophisticated test setup, you could use a custom mock or proxy to capture
			// the exact data passed to msg.Respond() for verification.
		})
	}
}

func TestHandleProjectSlugToUID(t *testing.T) {
	tests := []struct {
		name          string
		projectSlug   string
		setupMocks    func(*ProjectsService, *MockNatsMsg)
		expectedError bool
	}{
		{
			name:        "success",
			projectSlug: "test-project",
			setupMocks: func(service *ProjectsService, msg *MockNatsMsg) {
				projectUID := "550e8400-e29b-41d4-a716-446655440000"
				service.projectsKV.(*MockKeyValue).On("Get", mock.Anything, "slug/test-project").Return(&MockKeyValueEntry{value: []byte(projectUID)}, nil)
				msg.On("Respond", []byte(projectUID)).Return(nil)
			},
			expectedError: false,
		},
		{
			name:        "NATS KV not initialized",
			projectSlug: "test-project",
			setupMocks: func(service *ProjectsService, msg *MockNatsMsg) {
				service.projectsKV = nil
				msg.On("Respond", []byte(nil)).Return(nil)
			},
			expectedError: false,
		},
		{
			name:        "error getting project",
			projectSlug: "test-project",
			setupMocks: func(service *ProjectsService, msg *MockNatsMsg) {
				service.projectsKV.(*MockKeyValue).On("Get", mock.Anything, "slug/test-project").Return(&MockKeyValueEntry{}, assert.AnError)
				msg.On("Respond", []byte(nil)).Return(nil)
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupService()
			msg := CreateMockNatsMsg([]byte(tt.projectSlug))
			tt.setupMocks(service, msg)

			// Test that the function doesn't panic and handles the message
			assert.NotPanics(t, func() {
				service.HandleProjectSlugToUID(msg)
			})

			msg.AssertExpectations(t)

			// For success case, verify that the mock was called as expected
			if tt.name == "success" {
				service.projectsKV.(*MockKeyValue).AssertExpectations(t)
			}

			// Note: The handler functions call msg.Respond() as required before returning.
			// We can see this from the error logs in the test output when the real nats.Msg.Respond()
			// is called (e.g., "error responding to NATS message").
			// In a more sophisticated test setup, you could use a custom mock or proxy to capture
			// the exact data passed to msg.Respond() for verification.
		})
	}
}
