// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"goa.design/goa/v3/security"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

func TestReadyz(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*service.ProjectsService)
		expectedError bool
		expectedBody  string
	}{
		{
			name: "service ready",
			setupMocks: func(projectService *service.ProjectsService) {
				// Mock repository and message builder as ready
				projectService.ProjectRepository = &domain.MockProjectRepository{}
				projectService.MessageBuilder = &domain.MockMessageBuilder{}
			},
			expectedError: false,
			expectedBody:  "OK\n",
		},
		{
			name: "repository not initialized",
			setupMocks: func(projectService *service.ProjectsService) {
				projectService.ProjectRepository = nil
				projectService.MessageBuilder = &domain.MockMessageBuilder{}
			},
			expectedError: true,
		},
		{
			name: "message builder not initialized",
			setupMocks: func(projectService *service.ProjectsService) {
				projectService.ProjectRepository = &domain.MockProjectRepository{}
				projectService.MessageBuilder = nil
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, _, _ := setupAPI()
			tt.setupMocks(api.service)

			result, err := api.Readyz(context.Background())

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
	api := &ProjectsAPI{}

	result, err := api.Livez(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, "OK\n", string(result))
}

func TestJWTAuth(t *testing.T) {
	tests := []struct {
		name          string
		bearerToken   string
		schema        *security.JWTScheme
		expectedError bool
		expectedEmail string
		expectedUser  string
		setupMocks    func(*auth.MockJWTAuth)
	}{
		{
			name: "valid token with email",
			// This token is just an example token value generated from jwt.io.
			bearerToken:   "eyJhbGciOiJQUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWUsImlhdCI6MTUxNjIzOTAyMn0.iOeNU4dAFFeBwNj6qdhdvm-IvDQrTa6R22lQVJVuWJxorJfeQww5Nwsra0PjaOYhAMj9jNMO5YLmud8U7iQ5gJK2zYyepeSuXhfSi8yjFZfRiSkelqSkU19I-Ja8aQBDbqXf2SAWA8mHF8VS3F08rgEaLCyv98fLLH4vSvsJGf6ueZSLKDVXz24rZRXGWtYYk_OYYTVgR1cg0BLCsuCvqZvHleImJKiWmtS0-CymMO4MMjCy_FIl6I56NqLE9C87tUVpo1mT-kbg5cHDD8I7MjCW5Iii5dethB4Vid3mZ6emKjVYgXrtkOQ-JyGMh6fnQxEFN1ft33GX2eRHluK9eg",
			schema:        &security.JWTScheme{},
			expectedError: false,
			expectedUser:  "user1",
			expectedEmail: "user1@example.com",
			setupMocks: func(mockJwtAuth *auth.MockJWTAuth) {
				mockJwtAuth.On("ParsePrincipalAndEmail", mock.Anything, mock.Anything, mock.Anything).Return("user1", "user1@example.com", nil)
			},
		},
		{
			name:          "invalid token",
			bearerToken:   "invalid.token",
			schema:        &security.JWTScheme{},
			expectedError: true,
			setupMocks: func(mockJwtAuth *auth.MockJWTAuth) {
				mockJwtAuth.On("ParsePrincipalAndEmail", mock.Anything, mock.Anything, mock.Anything).Return("", "", assert.AnError)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, _, _ := setupAPI()
			tt.setupMocks(api.service.Auth.(*auth.MockJWTAuth))

			ctx, err := api.JWTAuth(context.Background(), tt.bearerToken, tt.schema)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedUser, ctx.Value(constants.PrincipalContextID))
			if tt.expectedEmail != "" {
				assert.Equal(t, tt.expectedEmail, ctx.Value(constants.EmailContextID))
			}
		})
	}
}

// Test cleanup
func TestMain(m *testing.M) {
	// Run tests
	m.Run()
}
