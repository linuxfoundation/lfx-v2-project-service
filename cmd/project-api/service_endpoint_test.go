// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"goa.design/goa/v3/security"
)

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
				service.natsConn.(*nats.MockNATSConn).On("IsConnected").Return(true)
			},
			expectedError: false,
			expectedBody:  "OK\n",
		},
		{
			name: "NATS not connected",
			setupMocks: func(service *ProjectsService) {
				service.natsConn.(*nats.MockNATSConn).On("IsConnected").Return(false)
			},
			expectedError: true,
		},
		{
			name: "NATS KV not initialized",
			setupMocks: func(service *ProjectsService) {
				service.kvStores.Projects = nil
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
			} else if assert.NoError(t, err) {
				// For valid tokens, we expect the context to be modified
				assert.NotEqual(t, context.Background(), ctx)
			}
		})
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// Helper function to create boolean pointers
func boolPtr(b bool) *bool {
	return &b
}

// Test cleanup
func TestMain(m *testing.M) {
	// Run tests
	m.Run()
}
