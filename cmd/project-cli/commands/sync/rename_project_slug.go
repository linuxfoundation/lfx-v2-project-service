// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/log"
)

type renameProjectSlugSubcommand struct{}

func (s *renameProjectSlugSubcommand) Name() string { return "rename-project-slug" }

func (s *renameProjectSlugSubcommand) Help() string {
	return "rename a project slug across OpenSearch and NATS JetStream KV buckets"
}

func (s *renameProjectSlugSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	fs := flag.NewFlagSet("rename-project-slug", flag.ContinueOnError)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "usage: project-cli sync rename-project-slug [flags] [<old-slug> <new-slug>]\n\nflags:\n")
		fs.PrintDefaults()
	}
	target := fs.String("target", envOrDefault("TARGET", "both"), "stores to migrate: opensearch, nats, or both")
	dryRun := fs.Bool("dry-run", envBoolOrDefault("DRY_RUN", true), "preview changes without writing")
	concurrency := fs.Int("concurrency", envIntOrDefault("CONCURRENCY", 50), "max concurrent NATS KV record updates per bucket")
	natsBuckets := fs.String("nats-buckets", envOrDefault("NATS_BUCKETS", strings.Join(DefaultNATSBuckets, ",")), "comma-separated NATS KV bucket names to migrate")
	oldSlugFlag := fs.String("old-slug", envOrDefault("OLD_SLUG", ""), "current slug (alternative to first positional arg)")
	newSlugFlag := fs.String("new-slug", envOrDefault("NEW_SLUG", ""), "new slug (alternative to second positional arg)")
	if err := fs.Parse(rc.Args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	oldSlug, newSlug, err := resolveSlugs(*oldSlugFlag, *newSlugFlag, fs.Args())
	if err != nil {
		return err
	}

	ctx = withSlugContext(ctx, oldSlug, newSlug, *dryRun)

	slog.InfoContext(ctx, "rename-project-slug configured",
		"target", *target,
		"concurrency", *concurrency,
		"opensearch_url", redactURL(rc.OpenSearchURL),
		"nats_url", redactURL(rc.NATSURL),
	)

	runner := NewRenameSlugRunner(rc.OpenSearch, rc.JetStream)
	return runner.Run(ctx, RenameSlugOptions{
		OldSlug:     oldSlug,
		NewSlug:     newSlug,
		Target:      *target,
		DryRun:      *dryRun,
		Concurrency: *concurrency,
		NATSBuckets: parseNATSBuckets(*natsBuckets),
	})
}

func resolveSlugs(oldSlugFlag, newSlugFlag string, slugArgs []string) (string, string, error) {
	oldSlug := strings.TrimSpace(oldSlugFlag)
	newSlug := strings.TrimSpace(newSlugFlag)

	hasFlagSlugs := oldSlug != "" || newSlug != ""
	hasPosArgs := len(slugArgs) > 0

	if hasFlagSlugs && hasPosArgs {
		return "", "", fmt.Errorf("provide slugs either as positional args OR via --old-slug/--new-slug flags, not both")
	}
	if hasPosArgs {
		if len(slugArgs) != 2 {
			return "", "", fmt.Errorf("expected exactly 2 positional args (<old-slug> <new-slug>), got %d", len(slugArgs))
		}
		oldSlug = strings.TrimSpace(slugArgs[0])
		newSlug = strings.TrimSpace(slugArgs[1])
	}
	if oldSlug == "" || newSlug == "" {
		return "", "", fmt.Errorf("usage: project-cli sync rename-project-slug [flags] <old-slug> <new-slug>\n       or set --old-slug and --new-slug (or OLD_SLUG/NEW_SLUG env vars)")
	}
	return oldSlug, newSlug, nil
}

func withSlugContext(ctx context.Context, oldSlug, newSlug string, dryRun bool) context.Context {
	ctx = log.AppendCtx(ctx, slog.String("old_slug", oldSlug))
	ctx = log.AppendCtx(ctx, slog.String("new_slug", newSlug))
	ctx = log.AppendCtx(ctx, slog.Bool("dry_run", dryRun))
	return ctx
}

func envOrDefault(key, defaultValue string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultValue
}

func envBoolOrDefault(key string, defaultValue bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultValue
	}
	switch strings.ToLower(v) {
	case "true", "1", "t", "yes":
		return true
	case "false", "0", "f", "no":
		return false
	default:
		return defaultValue
	}
}

func envIntOrDefault(key string, defaultValue int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultValue
	}
	var parsed int
	if _, err := fmt.Sscanf(v, "%d", &parsed); err != nil || parsed < 1 {
		return defaultValue
	}
	return parsed
}
