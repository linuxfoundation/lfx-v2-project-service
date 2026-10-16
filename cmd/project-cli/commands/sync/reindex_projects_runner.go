// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
	"golang.org/x/sync/errgroup"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/internal/osdiff"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// rootProjectSlug is the hidden ROOT project's slug (see constants.RootProjectSlug).
const rootProjectSlug = constants.RootProjectSlug

// projectRecordRepo is the narrow slice of natsinfra.NatsRepository this
// runner needs, so tests can fake it without a live NATS connection.
type projectRecordRepo interface {
	GetProjectBase(ctx context.Context, projectUID string) (*models.ProjectBase, error)
	ListAllProjectsBase(ctx context.Context) ([]*models.ProjectBase, error)
	GetProjectSettings(ctx context.Context, projectUID string) (*models.ProjectSettings, error)
}

type reindexProjectsRunner struct {
	repo          projectRecordRepo
	openSearch    *opensearchgo.Client
	publisher     domain.EventPublisher
	dryRun        bool
	all           bool
	force         bool
	includeAccess bool
	// keepDirectUsers stops the access message from deleting direct user tuples that settings
	// do not list. Only meaningful with includeAccess.
	keepDirectUsers bool
	concurrency     int
	stats           *commands.Stats
}

func (r *reindexProjectsRunner) run(ctx context.Context, projectUID string) error {
	var bases []*models.ProjectBase
	var excludedRoot int
	var rootBases []*models.ProjectBase
	if projectUID != "" {
		base, err := r.repo.GetProjectBase(ctx, projectUID)
		if err != nil {
			return fmt.Errorf("get project base %q: %w", projectUID, err)
		}
		if base.Slug == rootProjectSlug {
			slog.WarnContext(ctx, "explicit --project-uid targets the hidden root project; proceeding",
				"project_slug", rootProjectSlug, "project_uid", base.UID)
		}
		bases = []*models.ProjectBase{base}
	} else {
		var err error
		bases, err = r.repo.ListAllProjectsBase(ctx)
		if err != nil {
			return fmt.Errorf("list project bases: %w", err)
		}

		// The hidden ROOT project is never indexed. Full scans (including --all)
		// drop it before the OpenSearch diff so it is not even queried; an
		// explicit --project-uid above still reindexes whatever the operator named.
		// It is kept aside (not discarded) since --include-access still needs to
		// repair its FGA tuples below — ROOT exists specifically for team
		// permission assignment, and reindex-projects --all --include-access is
		// documented (docs/fga-contract.md) as the full-fleet FGA repair path.
		kept := make([]*models.ProjectBase, 0, len(bases))
		for _, base := range bases {
			if base.Slug == rootProjectSlug {
				excludedRoot++
				rootBases = append(rootBases, base)
				continue
			}
			kept = append(kept, base)
		}
		bases = kept
	}

	missing := map[string]osdiff.Missing{}
	// --force is only accepted alongside an explicit --project-uid (enforced at
	// flag parse time in reindex_projects.go), so it never widens the diff skip
	// beyond the single named project.
	if r.all || r.force {
		for _, base := range bases {
			missing[base.UID] = osdiff.Missing{Project: true, Settings: true}
		}
	} else {
		var err error
		missing, err = osdiff.Diff(ctx, r.openSearch, bases)
		if err != nil {
			return fmt.Errorf("diff opensearch: %w", err)
		}
	}

	missingProject, missingSettings := 0, 0
	for _, m := range missing {
		if m.Project {
			missingProject++
		}
		if m.Settings {
			missingSettings++
		}
	}
	slog.InfoContext(ctx, "reindex-projects scan complete",
		"total_projects", len(bases),
		"excluded_root_projects", excludedRoot,
		"missing_project_docs", missingProject,
		"missing_settings_docs", missingSettings,
	)

	r.stats.Total = len(bases)
	// The ROOT access-only repair pass below is a unit of work in its own right
	// (each entry resolves to exactly one Updated or Failed), so it's counted in
	// Total too, keeping Total == Updated+Skipped+Failed and the log's rate_per_sec
	// accurate.
	if r.all && r.includeAccess {
		r.stats.Total += len(rootBases)
	}

	var statsMu sync.Mutex
	var processed atomic.Int64

	// logProgress is shared by both worker groups below so a project counted in
	// stats.Total advances the same reporting boundary regardless of which group
	// processed it, keeping "processed" and "total" comparable in the log line.
	logProgress := func(gCtx context.Context) {
		if n := processed.Add(1); n%1000 == 0 {
			statsMu.Lock()
			total, u, f := r.stats.Total, r.stats.Updated, r.stats.Failed
			statsMu.Unlock()
			slog.InfoContext(gCtx, "reindex-projects progress",
				"processed", n, "total", total, "updated", u, "failed", f)
		}
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(r.concurrency)

	for _, base := range bases {
		base := base
		m := missing[base.UID]
		if !m.Any() {
			statsMu.Lock()
			r.stats.Skipped++
			statsMu.Unlock()
			logProgress(gCtx)
			continue
		}
		g.Go(func() error {
			err := r.reindexProject(gCtx, base, m)

			statsMu.Lock()
			if err != nil {
				r.stats.Failed++
			} else {
				r.stats.Updated++
			}
			statsMu.Unlock()
			logProgress(gCtx)

			return nil
		})
	}

	// ROOT is excluded from bases above, so it never gets indexer/search-index
	// publishes and is never diffed against OpenSearch, so it can never be found
	// "missing". Access-only repair only applies to the --all path, mirroring the
	// documented flag contract for every other project: --include-access fires only
	// for projects with a missing OpenSearch document, or when combined with --all
	// or --force. --force alone can never reach ROOT here since it requires an
	// explicit --project-uid, which already reindexes ROOT via the normal path.
	if r.all && r.includeAccess {
		for _, base := range rootBases {
			base := base
			g.Go(func() error {
				err := r.reindexRootAccess(gCtx, base)

				statsMu.Lock()
				if err != nil {
					r.stats.Failed++
				} else {
					r.stats.Updated++
				}
				statsMu.Unlock()
				logProgress(gCtx)

				if err != nil {
					slog.WarnContext(gCtx, "failed to republish root project access",
						"project_slug", rootProjectSlug, constants.ErrKey, err)
				}
				return nil
			})
		}
	}

	return g.Wait()
}

// reindexRootAccess republishes only the FGA access message for the hidden ROOT
// project. ROOT never gets indexer/search-index publishes (see run()), but
// --include-access still needs to repair its FGA tuples, since ROOT exists
// specifically for team permission assignment.
func (r *reindexProjectsRunner) reindexRootAccess(ctx context.Context, base *models.ProjectBase) error {
	if r.dryRun {
		slog.InfoContext(ctx, "dry-run: would republish root project access",
			"project_slug", rootProjectSlug)
		return nil
	}

	settings, err := r.repo.GetProjectSettings(ctx, base.UID)
	if err != nil {
		return fmt.Errorf("get project settings for root project: %w", err)
	}

	return r.publisher.PublishAccessMessage(ctx, fgaconstants.GenericUpdateAccessSubject, r.accessMessage(base, settings))
}

// accessMessage builds the FGA access message, preserving unlisted direct user tuples when
// keepDirectUsers is set.
func (r *reindexProjectsRunner) accessMessage(base *models.ProjectBase, settings *models.ProjectSettings) fgatypes.GenericFGAMessage {
	proj := service.NewProjectProjection(base, settings)
	if r.keepDirectUsers {
		return proj.ToFGAMessageKeepingDirectUsers()
	}
	return proj.ToFGAMessage()
}

func (r *reindexProjectsRunner) reindexProject(ctx context.Context, base *models.ProjectBase, m osdiff.Missing) error {
	if r.dryRun {
		slog.InfoContext(ctx, "dry-run: would reindex project",
			"project_uid", base.UID,
			"missing_project", m.Project,
			"missing_settings", m.Settings,
			"include_access", r.includeAccess,
		)
		return nil
	}

	// --all and --force both skip the OpenSearch diff, so a project's absence
	// from the index is never confirmed. ActionUpdated avoids falsely reporting
	// a create for a document that may already exist. Otherwise,
	// m.project/m.projectSettings come from a confirmed diff, so ActionCreated
	// is accurate.
	action := indexerConstants.ActionCreated
	if r.all || r.force {
		action = indexerConstants.ActionUpdated
	}

	// NATS delivery is always fire-and-forget (conn.Publish) since
	// lfx-v2-indexer-service#68 migrated the indexer to a JetStream durable
	// consumer. Errors from SendIndexerMessage reflect publish failures
	// (e.g. NATS not ready), not indexer processing outcomes.
	g := new(errgroup.Group)

	if m.Project {
		g.Go(func() error {
			msg := indexerTypes.IndexerMessageEnvelope{
				Action:         action,
				Data:           *base,
				IndexingConfig: base.IndexingConfig(),
			}
			return r.publisher.SendIndexerMessage(ctx, constants.IndexProjectSubject, msg)
		})
	}

	// Settings are read and published independently of the project base publish
	// above. CreateProject writes the base before settings and can return before
	// settings ever land (internal/infrastructure/nats/repository.go), so a settings
	// read failure here must not block an otherwise-repairable base publish.
	if m.Settings || r.includeAccess {
		g.Go(func() error {
			settings, err := r.repo.GetProjectSettings(ctx, base.UID)
			if err != nil {
				slog.WarnContext(ctx, "failed to read project settings before reindex",
					"project_uid", base.UID, constants.ErrKey, err)
				return fmt.Errorf("get project settings %q: %w", base.UID, err)
			}

			sg := new(errgroup.Group)

			if m.Settings {
				sg.Go(func() error {
					msg := indexerTypes.IndexerMessageEnvelope{
						Action:         action,
						Data:           *settings,
						IndexingConfig: settings.IndexingConfig(base.UID),
					}
					return r.publisher.SendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, msg)
				})
			}

			if r.includeAccess {
				sg.Go(func() error {
					return r.publisher.PublishAccessMessage(ctx, fgaconstants.GenericUpdateAccessSubject, r.accessMessage(base, settings))
				})
			}

			return sg.Wait()
		})
	}

	if err := g.Wait(); err != nil {
		slog.WarnContext(ctx, "failed to reindex project", "project_uid", base.UID, constants.ErrKey, err)
		return err
	}
	return nil
}
