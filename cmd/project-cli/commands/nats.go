// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"

	natsinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// serviceAuthorization is the principal the CLI presents to code shared with
// the API that reads an authorization value from the context.
const serviceAuthorization = "Bearer lfx-v2-project-service"

// flushTimeout bounds how long Finish waits for published messages to drain.
const flushTimeout = 5 * time.Second

// WithServiceAuthorization returns ctx carrying the CLI's service authorization.
func WithServiceAuthorization(ctx context.Context) context.Context {
	return context.WithValue(ctx, constants.AuthorizationContextID, serviceAuthorization)
}

// ConnectRepository dials NATS and opens the project repository on it. The
// caller owns the returned connection and must Close it; on error nothing is
// left open.
func ConnectRepository(ctx context.Context, cfg natsinfra.Config) (*nats.Conn, *natsinfra.NatsRepository, error) {
	conn, js, err := natsinfra.Connect(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	repo, err := natsinfra.OpenRepository(ctx, js)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("open repository: %w", err)
	}
	return conn, repo, nil
}

// flusher is the part of *nats.Conn that Finish needs.
type flusher interface {
	FlushTimeout(timeout time.Duration) error
}

// Finish ends a run that published messages: it logs the summary, flushes the
// connection so nothing is lost on exit, and turns any failed records into an
// error worded "<n> <failureNoun>" (e.g. "3 project(s) failed to reindex").
func Finish(ctx context.Context, commandName string, stats *Stats, conn flusher, failureNoun string) error {
	stats.Log(ctx, commandName)
	if err := conn.FlushTimeout(flushTimeout); err != nil {
		return fmt.Errorf("flush NATS connection: %w", err)
	}
	if stats.Failed > 0 {
		return fmt.Errorf("%d %s", stats.Failed, failureNoun)
	}
	return nil
}
