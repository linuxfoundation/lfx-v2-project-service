// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
)

// ProjectsService implements the projsvc.Service interface
type ProjectsService struct {
	logger         *slog.Logger
	lfxEnvironment constants.LFXEnvironment
	projectsKV     INatsKeyValue
	natsConn       INatsConn
	auth           IJwtAuth
}

// IKeyValue is a NATS KV interface needed for the [ProjectsService].
type INatsKeyValue interface {
	ListKeys(context.Context, ...jetstream.WatchOpt) (jetstream.KeyLister, error)
	Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error)
	Put(context.Context, string, []byte) (uint64, error)
	Update(context.Context, string, []byte, uint64) (uint64, error)
	Delete(context.Context, string, ...jetstream.KVDeleteOpt) error
}

// INatsConn is a NATS connection interface needed for the [ProjectsService].
type INatsConn interface {
	IsConnected() bool
	Publish(subj string, data []byte) error
}

// IJwtAuth is a JWT authentication interface needed for the [ProjectsService].
type IJwtAuth interface {
	parsePrincipal(ctx context.Context, token string, logger *slog.Logger) (string, error)
}
