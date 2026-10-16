// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"goa.design/goa/v3/security"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	domainmocks "github.com/linuxfoundation/lfx-v2-project-service/internal/domain/mocks"
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
				// Mock repository and publisher as ready
				projectService.ProjectRepository = &domainmocks.MockProjectRepository{}
				projectService.Publisher = &domainmocks.MockMessageBuilder{}
				projectService.Sender = &domainmocks.MockMessageBuilder{}
			},
			expectedError: false,
			expectedBody:  "OK\n",
		},
		{
			name: "repository not initialized",
			setupMocks: func(projectService *service.ProjectsService) {
				projectService.ProjectRepository = nil
				projectService.Publisher = &domainmocks.MockMessageBuilder{}
				projectService.Sender = &domainmocks.MockMessageBuilder{}
			},
			expectedError: true,
		},
		{
			name: "publisher not initialized",
			setupMocks: func(projectService *service.ProjectsService) {
				projectService.ProjectRepository = &domainmocks.MockProjectRepository{}
				projectService.Publisher = nil
				projectService.Sender = nil
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
				mockJwtAuth.On("ParsePrincipalAndEmail", mock.Anything, mock.Anything).Return("user1", "user1@example.com", nil)
			},
		},
		{
			name:          "valid token without email",
			bearerToken:   "test-valid-token-no-email",
			schema:        &security.JWTScheme{},
			expectedError: false,
			expectedUser:  "user1",
			setupMocks: func(mockJwtAuth *auth.MockJWTAuth) {
				mockJwtAuth.On("ParsePrincipalAndEmail", mock.Anything, mock.Anything).Return("user1", "", nil)
			},
		},
		{
			name:          "invalid token",
			bearerToken:   "invalid.token",
			schema:        &security.JWTScheme{},
			expectedError: true,
			setupMocks: func(mockJwtAuth *auth.MockJWTAuth) {
				mockJwtAuth.On("ParsePrincipalAndEmail", mock.Anything, mock.Anything).Return("", "", assert.AnError)
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

// goaErrorFields returns the Code and Message of a Goa-generated error type,
// whose Error() method returns an empty string.
func goaErrorFields(t *testing.T, err error) (code, message string) {
	t.Helper()
	v := reflect.ValueOf(err)
	require.Equal(t, reflect.Pointer, v.Kind())
	return v.Elem().FieldByName("Code").String(), v.Elem().FieldByName("Message").String()
}

func TestHandleError(t *testing.T) {
	unknownErr := errors.New("unknown failure")

	tests := []struct {
		name        string
		err         error
		wantType    any
		wantMessage string
		wantSame    bool
	}{
		{name: "service unavailable", err: domain.ErrServiceUnavailable, wantType: &projsvc.ServiceUnavailableError{}, wantMessage: domain.ErrServiceUnavailable.Error()},
		{name: "forbidden", err: domain.ErrForbidden, wantType: &projsvc.ForbiddenError{}, wantMessage: domain.ErrForbidden.Error()},
		{name: "validation failed", err: domain.ErrValidationFailed, wantType: &projsvc.BadRequestError{}, wantMessage: domain.ErrValidationFailed.Error()},
		{name: "invalid parent project", err: domain.ErrInvalidParentProject, wantType: &projsvc.BadRequestError{}, wantMessage: domain.ErrInvalidParentProject.Error()},
		{name: "cannot delete non-crowdfunding project", err: domain.ErrCannotDeleteNonCrowdfundingProject, wantType: &projsvc.BadRequestError{}, wantMessage: domain.ErrCannotDeleteNonCrowdfundingProject.Error()},
		{name: "archived requires dissolution date", err: domain.ErrArchivedRequiresDissolutionDate, wantType: &projsvc.BadRequestError{}, wantMessage: domain.ErrArchivedRequiresDissolutionDate.Error()},
		{name: "invalid content type", err: domain.ErrInvalidContentType, wantType: &projsvc.BadRequestError{}, wantMessage: domain.ErrInvalidContentType.Error()},
		{name: "file too large", err: domain.ErrFileTooLarge, wantType: &projsvc.BadRequestError{}, wantMessage: domain.ErrFileTooLarge.Error()},
		{name: "project not found", err: domain.ErrProjectNotFound, wantType: &projsvc.NotFoundError{}, wantMessage: domain.ErrProjectNotFound.Error()},
		{name: "document not found", err: domain.ErrDocumentNotFound, wantType: &projsvc.NotFoundError{}, wantMessage: domain.ErrDocumentNotFound.Error()},
		{name: "link not found", err: domain.ErrLinkNotFound, wantType: &projsvc.NotFoundError{}, wantMessage: domain.ErrLinkNotFound.Error()},
		{name: "folder not found", err: domain.ErrFolderNotFound, wantType: &projsvc.NotFoundError{}, wantMessage: domain.ErrFolderNotFound.Error()},
		{name: "revision mismatch", err: domain.ErrRevisionMismatch, wantType: &projsvc.ConflictError{}, wantMessage: domain.ErrRevisionMismatch.Error()},
		{name: "project slug exists", err: domain.ErrProjectSlugExists, wantType: &projsvc.ConflictError{}, wantMessage: domain.ErrProjectSlugExists.Error()},
		{name: "document name exists", err: domain.ErrDocumentNameExists, wantType: &projsvc.ConflictError{}, wantMessage: domain.ErrDocumentNameExists.Error()},
		{name: "folder name exists", err: domain.ErrFolderNameExists, wantType: &projsvc.ConflictError{}, wantMessage: domain.ErrFolderNameExists.Error()},
		{name: "folder not empty", err: domain.ErrFolderNotEmpty, wantType: &projsvc.ConflictError{}, wantMessage: domain.ErrFolderNotEmpty.Error()},
		{name: "internal", err: domain.ErrInternal, wantType: &projsvc.InternalServerError{}, wantMessage: domain.ErrInternal.Error()},
		{name: "unmarshal is masked as internal", err: domain.ErrUnmarshal, wantType: &projsvc.InternalServerError{}, wantMessage: domain.ErrInternal.Error()},
		{name: "wrapped sentinel is matched", err: fmt.Errorf("lookup failed: %w", domain.ErrProjectNotFound), wantType: &projsvc.NotFoundError{}, wantMessage: domain.ErrProjectNotFound.Error()},
		{name: "wrapped sentinel does not leak detail", err: fmt.Errorf("secret detail: %w", domain.ErrForbidden), wantType: &projsvc.ForbiddenError{}, wantMessage: domain.ErrForbidden.Error()},
		{name: "unknown error is returned unchanged", err: unknownErr, wantSame: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleError(context.Background(), tt.err)
			require.Error(t, got)

			if tt.wantSame {
				assert.Same(t, tt.err, got)
				return
			}

			require.IsType(t, tt.wantType, got)
			_, message := goaErrorFields(t, got)
			assert.Equal(t, tt.wantMessage, message)
		})
	}

	t.Run("nil error is returned as nil", func(t *testing.T) {
		assert.NoError(t, handleError(context.Background(), nil))
	})
}

func TestCreateResponse(t *testing.T) {
	baseErr := errors.New("something went wrong")

	tests := []struct {
		name     string
		code     int
		wantType any
		wantCode string
	}{
		{name: "bad request", code: http.StatusBadRequest, wantType: &projsvc.BadRequestError{}, wantCode: "400"},
		{name: "forbidden", code: http.StatusForbidden, wantType: &projsvc.ForbiddenError{}, wantCode: "403"},
		{name: "not found", code: http.StatusNotFound, wantType: &projsvc.NotFoundError{}, wantCode: "404"},
		{name: "conflict", code: http.StatusConflict, wantType: &projsvc.ConflictError{}, wantCode: "409"},
		{name: "internal server error", code: http.StatusInternalServerError, wantType: &projsvc.InternalServerError{}, wantCode: "500"},
		{name: "service unavailable", code: http.StatusServiceUnavailable, wantType: &projsvc.ServiceUnavailableError{}, wantCode: "503"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createResponse(tt.code, baseErr)
			require.IsType(t, tt.wantType, got)
			code, message := goaErrorFields(t, got)
			assert.Equal(t, tt.wantCode, code)
			assert.Equal(t, baseErr.Error(), message)
		})
	}

	t.Run("unsupported status code returns nil", func(t *testing.T) {
		assert.Nil(t, createResponse(http.StatusTeapot, baseErr))
	})
}

// Test cleanup
func TestMain(m *testing.M) {
	// Run tests
	m.Run()
}
