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
	natsinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	osinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/opensearch"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/env"
)

type renameProjectSlugSubcommand struct{}

func (s *renameProjectSlugSubcommand) Name() string { return "rename-project-slug" }

func (s *renameProjectSlugSubcommand) Help() string {
	return "rename a project slug across OpenSearch and NATS JetStream KV buckets"
}

func (s *renameProjectSlugSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	fs := commands.NewFlagSet(s.Name(), "sync rename-project-slug [flags] [<old-slug> <new-slug>]")
	update := fs.Bool("update", env.GetBool("UPDATE", false), "write changes to OpenSearch and NATS KV (default is preview-only)")
	dryRun := fs.Bool("dry-run", env.GetBool("DRY_RUN", true), "deprecated: preview is the default; use --update to write")
	concurrency := fs.Int("concurrency", env.GetInt("CONCURRENCY", 50), "max concurrent NATS KV record updates per bucket")
	natsBuckets := fs.String("nats-buckets", env.Get("NATS_BUCKETS", strings.Join(defaultNATSBuckets, ",")), "comma-separated NATS KV bucket names to migrate")
	oldSlugFlag := fs.String("old-slug", env.Get("OLD_SLUG", ""), "current slug (alternative to first positional arg)")
	newSlugFlag := fs.String("new-slug", env.Get("NEW_SLUG", ""), "new slug (alternative to second positional arg)")
	if help, err := commands.ParseFlags(fs, rc.Args); help || err != nil {
		return err
	}

	apply, err := resolveWriteMode(ctx, fs, *update, *dryRun)
	if err != nil {
		return err
	}
	isDryRun := !apply

	oldSlug, newSlug, err := resolveSlugs(*oldSlugFlag, *newSlugFlag, fs.Args())
	if err != nil {
		return err
	}

	rc.DryRun = isDryRun

	natsConn, js, err := natsinfra.Connect(ctx, rc.NATSConfig)
	if err != nil {
		return err
	}
	defer natsConn.Close()

	osClient, err := osinfra.NewClient(ctx, rc.OpenSearchConfig)
	if err != nil {
		return err
	}

	ctx = withSlugContext(ctx, oldSlug, newSlug, isDryRun)

	slog.InfoContext(ctx, "rename-project-slug configured",
		"concurrency", *concurrency,
		"opensearch_url", redactURL(rc.OpenSearchConfig.URL),
		"nats_url", redactURL(rc.NATSConfig.URL),
	)

	runner := NewRenameSlugRunner(osClient, js)
	return runner.Run(ctx, RenameSlugOptions{
		OldSlug:     oldSlug,
		NewSlug:     newSlug,
		DryRun:      isDryRun,
		Concurrency: *concurrency,
		NATSBuckets: parseNATSBuckets(*natsBuckets),
	})
}

// resolveWriteMode reports whether the run should write. --update is the
// switch, as on the other sync subcommands (this one also reads UPDATE from
// the environment, which they do not), and a run previews unless it is given. --dry-run (or DRY_RUN) is the older spelling of the
// same choice and is still honoured so existing invocations and Job manifests
// keep working. Contradictory settings fail rather than guess which one was
// meant for a command that rewrites data.
func resolveWriteMode(ctx context.Context, fs *flag.FlagSet, update, dryRun bool) (bool, error) {
	updateSet, dryRunSet := specified(fs, "update", "UPDATE"), specified(fs, "dry-run", "DRY_RUN")
	if dryRunSet {
		slog.WarnContext(ctx, "--dry-run and DRY_RUN are deprecated: previewing is the default; pass --update (or UPDATE=true) to write")
	}
	switch {
	case updateSet && dryRunSet:
		if update == dryRun {
			return false, fmt.Errorf("conflicting write mode: update=%t and dry-run=%t; use --update alone", update, dryRun)
		}
		return update, nil
	case updateSet:
		return update, nil
	case dryRunSet:
		return !dryRun, nil
	}
	return false, nil
}

// specified reports whether a setting was given explicitly, as a flag on the
// command line or through its environment variable.
func specified(fs *flag.FlagSet, flagName, envName string) bool {
	set := strings.TrimSpace(os.Getenv(envName)) != ""
	fs.Visit(func(f *flag.Flag) {
		if f.Name == flagName {
			set = true
		}
	})
	return set
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
