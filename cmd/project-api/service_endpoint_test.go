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
			name:          "valid token with email",
			bearerToken:   "test-valid-token",
			schema:        &security.JWTScheme{},
			expectedError: false,
			expectedUser:  "user1",
			expectedEmail: "user1@example.com",
			setupMocks: func(mockJwtAuth *auth.MockJWTAuth) {
				mockJwtAuth.On("ParsePrincipalAndEmail", mock.Anything, mock.Anything, mock.Anything).Return("user1", "user1@example.com", nil)
			},
		},
		{
			name:          "valid token without email",
			bearerToken:   "test-valid-token-no-email",
			schema:        &security.JWTScheme{},
			expectedError: false,
			expectedUser:  "user1",
			setupMocks: func(mockJwtAuth *auth.MockJWTAuth) {
				mockJwtAuth.On("ParsePrincipalAndEmail", mock.Anything, mock.Anything, mock.Anything).Return("user1", "", nil)
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
			} else {
				assert.Nil(t, ctx.Value(constants.EmailContextID))
			}
		})
	}
}

// Test cleanup
func TestMain(m *testing.M) {
	// Run tests
	m.Run()
}
