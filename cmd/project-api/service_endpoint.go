// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"goa.design/goa/v3/security"
)

// createResponse creates a response error based on the HTTP status code.
func createResponse(code int, message string) error {
	switch code {
	case http.StatusBadRequest:
		return &projsvc.BadRequestError{
			Code:    strconv.Itoa(code),
			Message: message,
		}
	case http.StatusNotFound:
		return &projsvc.NotFoundError{
			Code:    strconv.Itoa(code),
			Message: message,
		}
	case http.StatusConflict:
		return &projsvc.ConflictError{
			Code:    strconv.Itoa(code),
			Message: message,
		}
	case http.StatusInternalServerError:
		return &projsvc.InternalServerError{
			Code:    strconv.Itoa(code),
			Message: message,
		}
	case http.StatusServiceUnavailable:
		return &projsvc.ServiceUnavailableError{
			Code:    strconv.Itoa(code),
			Message: message,
		}
	default:
		return nil
	}
}

// keyValueStoreReady checks if the key-value stores are ready for use
// by checking if the stores are not nil in the code.
func keyValueStoreReady(kvStores KVStores) bool {
	return kvStores.Projects != nil && kvStores.ProjectSettings != nil
}

// Readyz checks if the service is able to take inbound requests.
func (s *ProjectsService) Readyz(_ context.Context) ([]byte, error) {
	if s.natsConn == nil || !keyValueStoreReady(s.kvStores) {
		return nil, createResponse(http.StatusServiceUnavailable, "service unavailable")
	}
	if !s.natsConn.IsConnected() {
		return nil, createResponse(http.StatusServiceUnavailable, "NATS connection not established")
	}
	return []byte("OK\n"), nil
}

// Livez checks if the service is alive.
func (s *ProjectsService) Livez(_ context.Context) ([]byte, error) {
	// This always returns as long as the service is still running. As this
	// endpoint is expected to be used as a Kubernetes liveness check, this
	// service must likewise self-detect non-recoverable errors and
	// self-terminate.
	return []byte("OK\n"), nil
}

// JWTAuth implements Auther interface for the JWT security scheme.
func (s *ProjectsService) JWTAuth(ctx context.Context, bearerToken string, _ *security.JWTScheme) (context.Context, error) {
	// Parse the Heimdall-authorized principal from the token.
	principal, err := s.auth.parsePrincipal(ctx, bearerToken, slog.Default())
	if err != nil {
		return ctx, err
	}
	// Return a new context containing the principal as a value.
	return context.WithValue(ctx, constants.PrincipalContextID, principal), nil
}
