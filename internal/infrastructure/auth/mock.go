// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"log/slog"

	"github.com/stretchr/testify/mock"
)

// MockJWTAuth implements JWTAuth for testing
type MockJWTAuth struct {
	mock.Mock
}

func (m *MockJWTAuth) ParsePrincipal(ctx context.Context, token string, logger *slog.Logger) (string, error) {
	args := m.Called(ctx, token, logger)
	return args.String(0), args.Error(1)
}

// Ensure MockJWTAuth implements IJWTAuth interface
var _ IJWTAuth = (*MockJWTAuth)(nil)
