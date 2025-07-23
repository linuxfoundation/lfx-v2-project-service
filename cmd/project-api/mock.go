// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"

	"github.com/stretchr/testify/mock"
)

// MockJwtAuth is a mock implementation of the [IJwtAuth] interface.
type MockJwtAuth struct {
	mock.Mock
}

// parsePrincipal is a mock method for the [IJwtAuth] interface.
func (m *MockJwtAuth) parsePrincipal(ctx context.Context, token string, logger *slog.Logger) (string, error) {
	args := m.Called(ctx, token, logger)
	return args.String(0), args.Error(1)
}
