// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
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

	if isVersionRequest(positionals, parsed.SubArgs) {
		printVersion(os.Stdout)
		return nil
	}

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
		NATSConfig:       natsinfra.ConfigFromEnv(),
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

func printVersion(w io.Writer) {
	_, _ = fmt.Fprintf(w, "project-cli %s (commit %s, built %s)\n", Version, GitCommit, BuildTime)
}

func printUsage(w io.Writer, registry map[string]commands.Command) {
	_, _ = fmt.Fprintln(w, "usage: project-cli <command> <subcommand> [subcommand flags]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "environment variables:")
	_, _ = fmt.Fprintln(w, "  NATS_URL         NATS server address (default: nats://localhost:4222)")
	_, _ = fmt.Fprintln(w, "  OPENSEARCH_URL   OpenSearch base URL (default: http://localhost:9200)")
	_, _ = fmt.Fprintln(w, "  LOG_LEVEL        Log verbosity, e.g. debug (default: info)")
	_, _ = fmt.Fprintln(w, "  JOB_RUN_ID       Optional run identifier for structured logs")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Subcommand-specific environment variables are documented in cmd/project-cli/README.md.")
	_, _ = fmt.Fprintln(w, "commands:")
	for _, cmdName := range sortedKeys(registry) {
		cmd := registry[cmdName]
		_, _ = fmt.Fprintf(w, "  %-30s %s\n", cmd.Name(), cmd.Help())
		subs := cmd.Subcommands()
		for _, subName := range sortedKeys(subs) {
			_, _ = fmt.Fprintf(w, "    %-28s %s\n", subs[subName].Name(), subs[subName].Help())
		}
	}
	_, _ = fmt.Fprintf(w, "  %-30s %s\n", "version", "print the build version and exit")
}

// sortedKeys returns m's keys in order so help output is stable between runs.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
