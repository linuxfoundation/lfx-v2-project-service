// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"strings"
	"time"

	opensearchgo "github.com/opensearch-project/opensearch-go/v2"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	natsinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	osinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/opensearch"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/env"
)

type reindexProjectsSubcommand struct{}

func (s *reindexProjectsSubcommand) Name() string { return "reindex-projects" }

func (s *reindexProjectsSubcommand) Help() string {
	return "diff project/project_settings OpenSearch documents against KV and republish missing indexer messages"
}

func (s *reindexProjectsSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	slog.DebugContext(ctx, "starting subcommand", "subcommand", s.Name(), "args", rc.Args)

	fs := flag.NewFlagSet("reindex-projects", flag.ContinueOnError)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "usage: project-cli sync reindex-projects [flags]\n\nflags:\n")
		fs.PrintDefaults()
	}
	projectUID := fs.String("project-uid", "", "limit to a single project UID (default all)")
	update := fs.Bool("update", false, "publish indexer messages (default is preview-only)")
	concurrency := fs.Int("concurrency", env.GetInt("CONCURRENCY", 50), "max concurrent project republishes")
	all := fs.Bool("all", false, "republish every project regardless of OpenSearch state, skipping the diff")
	includeAccess := fs.Bool("include-access", false, "also republish the FGA access message")
	if err := fs.Parse(rc.Args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	uid := strings.TrimSpace(*projectUID)
	if *all && uid != "" {
		return fmt.Errorf("--all and --project-uid are mutually exclusive")
	}
	if *concurrency < 1 {
		return fmt.Errorf("concurrency must be at least 1")
	}

	rc.DryRun = !*update
	ctx = context.WithValue(ctx, constants.AuthorizationContextID, "Bearer lfx-v2-project-service")

	natsConn, js, err := natsinfra.Connect(ctx, rc.NATSConfig)
	if err != nil {
		return err
	}
	defer natsConn.Close()

	repo, err := natsinfra.OpenRepository(ctx, js)
	if err != nil {
		return fmt.Errorf("open repository: %w", err)
	}

	var osClient *opensearchgo.Client
	if !*all {
		osClient, err = osinfra.NewClient(ctx, rc.OpenSearchConfig)
		if err != nil {
			return err
		}
	}

	slog.InfoContext(ctx, "reindex-projects configured",
		"concurrency", *concurrency,
		"update", *update,
		"all", *all,
		"include_access", *includeAccess,
		"opensearch_url", redactURL(rc.OpenSearchConfig.URL),
		"nats_url", redactURL(rc.NATSConfig.URL),
	)

	stats := commands.NewStats()
	stats.DryRun = rc.DryRun

	runner := &reindexProjectsRunner{
		repo:          repo,
		openSearch:    osClient,
		publisher:     &natsinfra.MessageBuilder{NatsConn: natsConn},
		dryRun:        rc.DryRun,
		all:           *all,
		includeAccess: *includeAccess,
		concurrency:   *concurrency,
		stats:         stats,
	}

	if err := runner.run(ctx, uid); err != nil {
		return err
	}

	stats.Log(ctx, "sync reindex-projects")
	if err := natsConn.FlushTimeout(5 * time.Second); err != nil {
		return fmt.Errorf("flush NATS connection: %w", err)
	}
	if stats.Failed > 0 {
		return fmt.Errorf("%d project(s) failed to reindex", stats.Failed)
	}
	return nil
}
