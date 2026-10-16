// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	opensearchgo "github.com/opensearch-project/opensearch-go/v2"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	natsinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	osinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/opensearch"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/env"
)

type reindexProjectsSubcommand struct{}

func (s *reindexProjectsSubcommand) Name() string { return "reindex-projects" }

func (s *reindexProjectsSubcommand) Help() string {
	return "diff project/project_settings OpenSearch documents against KV and republish missing indexer messages"
}

func (s *reindexProjectsSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	slog.DebugContext(ctx, "starting subcommand", "subcommand", s.Name(), "args", rc.Args)

	fs := commands.NewFlagSet(s.Name(), "sync reindex-projects [flags]")
	projectUID := fs.String("project-uid", "", "limit to a single project UID (default all)")
	update := fs.Bool("update", false, "publish indexer messages (default is preview-only)")
	concurrency := fs.Int("concurrency", env.GetInt("CONCURRENCY", 50), "max concurrent project republishes")
	all := fs.Bool("all", false, "republish every project regardless of OpenSearch state, skipping the diff")
	force := fs.Bool("force", false, "with --project-uid, skip the OpenSearch diff and unconditionally republish that project")
	includeAccess := fs.Bool("include-access", false, "also republish the FGA access message")
	keepDirectUsers := fs.Bool("keep-direct-users", false, "with --include-access, do not delete direct user tuples that project settings do not list")
	if help, err := commands.ParseFlags(fs, rc.Args); help || err != nil {
		return err
	}
	if err := commands.RejectExtraArgs(fs); err != nil {
		return err
	}

	uid := strings.TrimSpace(*projectUID)
	if *all && uid != "" {
		return fmt.Errorf("--all and --project-uid are mutually exclusive")
	}
	if *force && uid == "" {
		return fmt.Errorf("--force requires --project-uid")
	}
	if *keepDirectUsers && !*includeAccess {
		return fmt.Errorf("--keep-direct-users requires --include-access")
	}
	if *concurrency < 1 {
		return fmt.Errorf("concurrency must be at least 1")
	}

	rc.DryRun = !*update
	ctx = commands.WithServiceAuthorization(ctx)

	natsConn, repo, err := commands.ConnectRepository(ctx, rc.NATSConfig)
	if err != nil {
		return err
	}
	defer natsConn.Close()

	skipDiff := *all || *force
	var osClient *opensearchgo.Client
	if !skipDiff {
		osClient, err = osinfra.NewClient(ctx, rc.OpenSearchConfig)
		if err != nil {
			return err
		}
	}

	slog.InfoContext(ctx, "reindex-projects configured",
		"concurrency", *concurrency,
		"update", *update,
		"all", *all,
		"force", *force,
		"include_access", *includeAccess,
		"keep_direct_users", *keepDirectUsers,
		"opensearch_url", redactURL(rc.OpenSearchConfig.URL),
		"nats_url", redactURL(rc.NATSConfig.URL),
	)

	stats := commands.NewStats()
	stats.DryRun = rc.DryRun

	runner := &reindexProjectsRunner{
		repo:            repo,
		openSearch:      osClient,
		publisher:       &natsinfra.MessageBuilder{NatsConn: natsConn},
		dryRun:          rc.DryRun,
		all:             *all,
		force:           *force,
		includeAccess:   *includeAccess,
		keepDirectUsers: *keepDirectUsers,
		concurrency:     *concurrency,
		stats:           stats,
	}

	if err := runner.run(ctx, uid); err != nil {
		return err
	}

	return commands.Finish(ctx, "sync reindex-projects", stats, natsConn, "project(s) failed to reindex")
}
