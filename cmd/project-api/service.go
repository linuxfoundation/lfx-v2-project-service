// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// ProjectsService implements the projsvc.Service interface
type ProjectsService struct {
	lfxEnvironment constants.LFXEnvironment
	projectsKV     nats.INatsKeyValue
	natsConn       nats.INatsConn
	auth           IJwtAuth
}

// IJwtAuth is a JWT authentication interface needed for the [ProjectsService].
type IJwtAuth interface {
	parsePrincipal(ctx context.Context, token string, logger *slog.Logger) (string, error)
}
