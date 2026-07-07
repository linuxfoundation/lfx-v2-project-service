// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands/sync"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/log"
	natsinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	osinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/opensearch"
)

// Build-time variables set via ldflags.
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	if err := run(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	registry := buildRegistry()

	const positionalLimit = 2
	parsed := splitArgs(os.Args[1:], positionalLimit)
	positionals := parsed.Positionals

	if hasHelpFlag(parsed.SubArgs) {
		switch len(positionals) {
		case 0, 1:
			printUsage(os.Stdout, registry)
			return nil
		default:
			if grp, ok := registry[positionals[0]]; ok {
				if sub, ok := grp.Subcommands()[positionals[1]]; ok {
					_ = sub.Run(ctx, commands.RunContext{Args: []string{"--help"}})
					return nil
				}
			}
			fmt.Fprintf(os.Stderr, "unknown command: %s %s\n\n", positionals[0], positionals[1])
			printUsage(os.Stderr, registry)
			return fmt.Errorf("unknown command: %s %s", positionals[0], positionals[1])
		}
	}

	log.InitStructureLogConfig()

	jobRunID := resolveJobRunID()
	ctx = log.AppendCtx(ctx, slog.String("job_run_id", jobRunID))

	if len(positionals) < 2 {
		printUsage(os.Stderr, registry)
		return fmt.Errorf("usage: project-cli <command> <subcommand> [subcommand flags]")
	}
	commandName := positionals[0]
	subcommandName := positionals[1]

	cmd, ok := registry[commandName]
	if !ok {
		printUsage(os.Stderr, registry)
		return fmt.Errorf("unknown command: %s", commandName)
	}

	sub, ok := cmd.Subcommands()[subcommandName]
	if !ok {
		printUsage(os.Stderr, registry)
		return fmt.Errorf("unknown subcommand: %s %s", commandName, subcommandName)
	}

	rc := commands.RunContext{
		NatsConfig:       natsinfra.NatsConfigFromEnv(),
		OpenSearchConfig: osinfra.ConfigFromEnv(),
		JobRunID:         jobRunID,
		Args:             parsed.SubArgs,
	}

	return sub.Run(ctx, rc)
}

func resolveJobRunID() string {
	for _, key := range []string{"JOB_RUN_ID", "HOSTNAME"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return uuid.NewString()
}

func buildRegistry() map[string]commands.Command {
	syncCmd := sync.NewCommand()
	return map[string]commands.Command{
		syncCmd.Name(): syncCmd,
	}
}

func printUsage(w io.Writer, registry map[string]commands.Command) {
	_, _ = fmt.Fprintln(w, "usage: project-cli <command> <subcommand> [subcommand flags]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "environment variables:")
	_, _ = fmt.Fprintln(w, "  NATS_URL         NATS server address (default: nats://localhost:4222)")
	_, _ = fmt.Fprintln(w, "  OPENSEARCH_URL   OpenSearch base URL (default: http://localhost:9200)")
	_, _ = fmt.Fprintln(w, "  LOG_LEVEL        Log verbosity, e.g. info (default: debug)")
	_, _ = fmt.Fprintln(w, "  JOB_RUN_ID       Optional run identifier for structured logs")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Subcommand-specific environment variables are documented in cmd/project-cli/README.md.")
	_, _ = fmt.Fprintln(w, "commands:")
	for _, cmd := range registry {
		_, _ = fmt.Fprintf(w, "  %-30s %s\n", cmd.Name(), cmd.Help())
		for _, sub := range cmd.Subcommands() {
			_, _ = fmt.Fprintf(w, "    %-28s %s\n", sub.Name(), sub.Help())
		}
	}
}
