// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"goa.design/goa/v3/security"
)

// handleError converts domain errors to HTTP errors and logs client-facing failures.
func handleError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, domain.ErrServiceUnavailable):
		return createResponse(http.StatusServiceUnavailable, domain.ErrServiceUnavailable)
	case errors.Is(err, domain.ErrValidationFailed):
		slog.WarnContext(ctx, "request validation failed", constants.ErrKey, err)
		return createResponse(http.StatusBadRequest, domain.ErrValidationFailed)
	case errors.Is(err, domain.ErrRevisionMismatch):
		return createResponse(http.StatusConflict, domain.ErrRevisionMismatch)
	case errors.Is(err, domain.ErrInvalidParentProject):
		slog.WarnContext(ctx, "bad request", constants.ErrKey, err)
		return createResponse(http.StatusBadRequest, domain.ErrInvalidParentProject)
	case errors.Is(err, domain.ErrCannotDeleteNonCrowdfundingProject):
		slog.WarnContext(ctx, "bad request", constants.ErrKey, err)
		return createResponse(http.StatusBadRequest, domain.ErrCannotDeleteNonCrowdfundingProject)
	case errors.Is(err, domain.ErrArchivedRequiresDissolutionDate):
		slog.WarnContext(ctx, "bad request", constants.ErrKey, err)
		return createResponse(http.StatusBadRequest, domain.ErrArchivedRequiresDissolutionDate)
	case errors.Is(err, domain.ErrInvalidContentType), errors.Is(err, domain.ErrFileTooLarge):
		slog.WarnContext(ctx, "bad request", constants.ErrKey, err)
		return createResponse(http.StatusBadRequest, err)
	case errors.Is(err, domain.ErrProjectNotFound):
		return createResponse(http.StatusNotFound, domain.ErrProjectNotFound)
	case errors.Is(err, domain.ErrDocumentNotFound):
		return createResponse(http.StatusNotFound, domain.ErrDocumentNotFound)
	case errors.Is(err, domain.ErrLinkNotFound):
		return createResponse(http.StatusNotFound, domain.ErrLinkNotFound)
	case errors.Is(err, domain.ErrFolderNotFound):
		return createResponse(http.StatusNotFound, domain.ErrFolderNotFound)
	case errors.Is(err, domain.ErrProjectSlugExists):
		return createResponse(http.StatusConflict, domain.ErrProjectSlugExists)
	case errors.Is(err, domain.ErrDocumentNameExists):
		return createResponse(http.StatusConflict, domain.ErrDocumentNameExists)
	case errors.Is(err, domain.ErrFolderNameExists):
		return createResponse(http.StatusConflict, domain.ErrFolderNameExists)
	case errors.Is(err, domain.ErrFolderNotEmpty):
		return createResponse(http.StatusConflict, domain.ErrFolderNotEmpty)
	case errors.Is(err, domain.ErrInternal), errors.Is(err, domain.ErrUnmarshal):
		return createResponse(http.StatusInternalServerError, domain.ErrInternal)
	}
	return err
}

// nilStr returns empty string if pointer is nil, otherwise the value.
func nilStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// createResponse creates a response error based on the HTTP status code.
func createResponse(code int, err error) error {
	switch code {
	case http.StatusBadRequest:
		return &projsvc.BadRequestError{
			Code:    strconv.Itoa(code),
			Message: err.Error(),
		}
	case http.StatusNotFound:
		return &projsvc.NotFoundError{
			Code:    strconv.Itoa(code),
			Message: err.Error(),
		}
	case http.StatusConflict:
		return &projsvc.ConflictError{
			Code:    strconv.Itoa(code),
			Message: err.Error(),
		}
	case http.StatusInternalServerError:
		return &projsvc.InternalServerError{
			Code:    strconv.Itoa(code),
			Message: err.Error(),
		}
	case http.StatusServiceUnavailable:
		return &projsvc.ServiceUnavailableError{
			Code:    strconv.Itoa(code),
			Message: err.Error(),
		}
	default:
		return nil
	}
}

// Readyz checks if the service is able to take inbound requests.
func (s *ProjectsAPI) Readyz(_ context.Context) ([]byte, error) {
	if !s.service.ServiceReady() {
		return nil, createResponse(http.StatusServiceUnavailable, domain.ErrServiceUnavailable)
	}
	return []byte("OK\n"), nil
}

// Livez checks if the service is alive.
func (s *ProjectsAPI) Livez(_ context.Context) ([]byte, error) {
	// This always returns as long as the service is still running. As this
	// endpoint is expected to be used as a Kubernetes liveness check, this
	// service must likewise self-detect non-recoverable errors and
	// self-terminate.
	return []byte("OK\n"), nil
}

// JWTAuth implements Auther interface for the JWT security scheme.
func (s *ProjectsAPI) JWTAuth(ctx context.Context, bearerToken string, _ *security.JWTScheme) (context.Context, error) {
	if !s.service.ServiceReady() {
		return nil, createResponse(http.StatusServiceUnavailable, domain.ErrServiceUnavailable)
	}

	// Parse the Heimdall-authorized principal from the token.
	principal, email, err := s.service.Auth.ParsePrincipalAndEmail(ctx, bearerToken)
	if err != nil {
		return ctx, err
	}
	ctx = context.WithValue(ctx, constants.PrincipalContextID, principal)
	if email != "" {
		ctx = context.WithValue(ctx, constants.EmailContextID, email)
	}
	return ctx, nil
}
