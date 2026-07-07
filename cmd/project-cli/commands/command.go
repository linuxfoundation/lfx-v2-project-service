// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package commands

import (
	"context"

	natsinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	osinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/opensearch"
)

// Command represents a top-level CLI command group (e.g. "sync").
type Command interface {
	Name() string
	Help() string
	Subcommands() map[string]Subcommand
}

// Subcommand represents a single runnable operation within a command group.
type Subcommand interface {
	Name() string
	Help() string
	Run(ctx context.Context, rc RunContext) error
}

// RunContext carries infrastructure configs and subcommand args. Connections are
// not established here — each subcommand dials only what it needs in its Run().
type RunContext struct {
	NatsConfig       natsinfra.NatsConfig
	OpenSearchConfig osinfra.Config
	JobRunID         string
	// DryRun is set by each subcommand after it parses its own --dry-run flag,
	// making the value available to any helper that receives a RunContext.
	DryRun bool
	Args   []string
}
