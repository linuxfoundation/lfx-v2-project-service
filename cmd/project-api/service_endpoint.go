// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"goa.design/goa/v3/security"
)

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
	principal, err := s.service.Auth.ParsePrincipal(ctx, bearerToken, slog.Default())
	if err != nil {
		return ctx, err
	}
	// Return a new context containing the principal as a value.
	return context.WithValue(ctx, constants.PrincipalContextID, principal), nil
}
