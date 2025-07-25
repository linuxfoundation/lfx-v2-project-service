// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
)

// ProjectsService implements the projsvc.Service interface
type ProjectsService struct {
	kvStores KVStores
	natsConn nats.INatsConn
	auth     IJwtAuth
}

// KVStores is a collection of NATS KV stores for the service.
type KVStores struct {
	Projects        nats.INatsKeyValue
	ProjectSettings nats.INatsKeyValue
}

// IJwtAuth is a JWT authentication interface needed for the [ProjectsService].
type IJwtAuth interface {
	parsePrincipal(ctx context.Context, token string, logger *slog.Logger) (string, error)
}
