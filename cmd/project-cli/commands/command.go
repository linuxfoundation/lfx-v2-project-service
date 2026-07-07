// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package commands

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
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

// RunContext carries wired infrastructure and subcommand args.
type RunContext struct {
	OpenSearch    *opensearchgo.Client
	JetStream     jetstream.JetStream
	OpenSearchURL string
	NATSURL       string
	JobRunID      string
	// DryRun is set by each subcommand after it parses its own --dry-run flag,
	// making the value available to any helper that receives a RunContext.
	DryRun bool
	Args   []string
}

// Stats tracks counters for a command run.
type Stats struct {
	Total   int
	Updated int
	Skipped int
	Failed  int
	DryRun  bool
	start   time.Time
}

// NewStats creates a Stats with the start time set to now.
func NewStats() *Stats {
	return &Stats{start: time.Now()}
}

// Log emits the run summary as a structured JSON log line.
func (s *Stats) Log(ctx context.Context, commandName string) {
	duration := time.Since(s.start)
	rate := 0.0
	if duration.Seconds() > 0 {
		rate = float64(s.Total) / duration.Seconds()
	}
	slog.InfoContext(ctx, commandName+" complete",
		"total", s.Total,
		"updated", s.Updated,
		"skipped", s.Skipped,
		"failed", s.Failed,
		"dry_run", s.DryRun,
		"duration_ms", duration.Milliseconds(),
		"rate_per_sec", rate,
	)
}
