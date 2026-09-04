// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
	"golang.org/x/sync/errgroup"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// resourcesIndex is the shared OpenSearch index queried and written by the
// indexer service. There is no exported constant for it (see
// rename_project_slug_runner.go), so the literal is repeated here.
const resourcesIndex = "resources"

const osIDChunkSize = 1000

// rootProjectSlug identifies the hidden ROOT project used for team permission
// assignment. It is created by scripts/root-project-setup (package main, so that
// script's own const is not importable) and must not appear in the search index.
// Compared case-sensitively on purpose: API slugs are validated against
// ^[a-z][a-z0-9_\-]*[a-z0-9]$ (api/project/v1/design/types.go), so "ROOT" cannot
// collide with a user slug, while a case-insensitive match would wrongly exclude a
// legitimate project slugged "root".
const rootProjectSlug = "ROOT"

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
	publisher     domain.MessageBuilder
	dryRun        bool
	all           bool
	includeAccess bool
	concurrency   int
	stats         *commands.Stats
}

// osMissing tracks, per project UID, whether the project and project_settings
// OpenSearch documents are absent. The two are resolved independently since
// a project can be missing one without the other.
type osMissing struct {
	project         bool
	projectSettings bool
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

	missing := map[string]osMissing{}
	if r.all {
		for _, base := range bases {
			missing[base.UID] = osMissing{project: true, projectSettings: true}
		}
	} else {
		var err error
		missing, err = r.diffOpenSearch(ctx, bases)
		if err != nil {
			return fmt.Errorf("diff opensearch: %w", err)
		}
	}

	missingProject, missingSettings := 0, 0
	for _, m := range missing {
		if m.project {
			missingProject++
		}
		if m.projectSettings {
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

	var statsMu sync.Mutex
	var processed atomic.Int64

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(r.concurrency)

	for _, base := range bases {
		base := base
		m := missing[base.UID]
		if !m.project && !m.projectSettings {
			statsMu.Lock()
			r.stats.Skipped++
			statsMu.Unlock()
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

			if n := processed.Add(1); n%1000 == 0 {
				statsMu.Lock()
				u, f := r.stats.Updated, r.stats.Failed
				statsMu.Unlock()
				slog.InfoContext(gCtx, "reindex-projects progress",
					"processed", n, "total", len(bases), "updated", u, "failed", f)
			}
			return nil
		})
	}

	// ROOT is excluded from bases above, so it never gets indexer/search-index
	// publishes. --include-access still needs to repair its FGA tuples, so give it
	// an access-only pass here rather than silently dropping that repair.
	if r.includeAccess {
		for _, base := range rootBases {
			base := base
			g.Go(func() error {
				if err := r.reindexRootAccess(gCtx, base); err != nil {
					slog.WarnContext(gCtx, "failed to republish root project access",
						"project_slug", rootProjectSlug, constants.ErrKey, err)
					return err
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

	proj := service.NewProjectProjection(base, settings)
	return r.publisher.PublishAccessMessage(ctx, fgaconstants.GenericUpdateAccessSubject, proj.ToFGAMessage())
}

func (r *reindexProjectsRunner) reindexProject(ctx context.Context, base *models.ProjectBase, m osMissing) error {
	if r.dryRun {
		slog.InfoContext(ctx, "dry-run: would reindex project",
			"project_uid", base.UID,
			"missing_project", m.project,
			"missing_settings", m.projectSettings,
			"include_access", r.includeAccess,
		)
		return nil
	}

	// --all skips the OpenSearch diff, so a project's absence from the index is
	// never confirmed. ActionUpdated avoids falsely reporting a create for a
	// document that may already exist. Outside --all, m.project/m.projectSettings
	// come from a confirmed diff, so ActionCreated is accurate.
	action := indexerConstants.ActionCreated
	if r.all {
		action = indexerConstants.ActionUpdated
	}

	// sync=true (request/reply) so a NATS timeout or unreachable indexer surfaces as a
	// per-project failure instead of a silently-dropped fire-and-forget publish.
	g := new(errgroup.Group)

	if m.project {
		g.Go(func() error {
			msg := indexerTypes.IndexerMessageEnvelope{
				Action:         action,
				Data:           *base,
				IndexingConfig: base.IndexingConfig(),
			}
			return r.publisher.SendIndexerMessage(ctx, constants.IndexProjectSubject, msg, true)
		})
	}

	// Settings are read and published independently of the project base publish
	// above. CreateProject writes the base before settings and can return before
	// settings ever land (internal/infrastructure/nats/repository.go), so a settings
	// read failure here must not block an otherwise-repairable base publish.
	if m.projectSettings || r.includeAccess {
		g.Go(func() error {
			settings, err := r.repo.GetProjectSettings(ctx, base.UID)
			if err != nil {
				slog.WarnContext(ctx, "failed to read project settings before reindex",
					"project_uid", base.UID, constants.ErrKey, err)
				return fmt.Errorf("get project settings %q: %w", base.UID, err)
			}

			sg := new(errgroup.Group)

			if m.projectSettings {
				sg.Go(func() error {
					msg := indexerTypes.IndexerMessageEnvelope{
						Action:         action,
						Data:           *settings,
						IndexingConfig: settings.IndexingConfig(base.UID),
					}
					return r.publisher.SendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, msg, true)
				})
			}

			if r.includeAccess {
				sg.Go(func() error {
					proj := service.NewProjectProjection(base, settings)
					return r.publisher.PublishAccessMessage(ctx, fgaconstants.GenericUpdateAccessSubject, proj.ToFGAMessage())
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

// diffOpenSearch resolves, for every project UID, whether its "project" and
// "project_settings" documents already exist in the resources index.
func (r *reindexProjectsRunner) diffOpenSearch(ctx context.Context, bases []*models.ProjectBase) (map[string]osMissing, error) {
	missing := make(map[string]osMissing, len(bases))
	for _, base := range bases {
		missing[base.UID] = osMissing{project: true, projectSettings: true}
	}

	var ids []string
	for _, base := range bases {
		ids = append(ids, "project:"+base.UID, "project_settings:"+base.UID)
	}

	for _, chunk := range chunkStrings(ids, osIDChunkSize) {
		found, err := r.queryExistingIDs(ctx, chunk)
		if err != nil {
			return nil, err
		}
		for id := range found {
			uid, docType, ok := splitResourceID(id)
			if !ok {
				continue
			}
			m := missing[uid]
			switch docType {
			case "project":
				m.project = false
			case "project_settings":
				m.projectSettings = false
			}
			missing[uid] = m
		}
	}

	return missing, nil
}

func (r *reindexProjectsRunner) queryExistingIDs(ctx context.Context, ids []string) (map[string]struct{}, error) {
	body, err := jsonBody(map[string]any{
		"size":    len(ids),
		"_source": false,
		"query": map[string]any{
			"ids": map[string]any{"values": ids},
		},
	})
	if err != nil {
		return nil, err
	}

	res, err := r.openSearch.Search(
		r.openSearch.Search.WithContext(ctx),
		r.openSearch.Search.WithIndex(resourcesIndex),
		r.openSearch.Search.WithBody(body),
	)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.IsError() {
		raw, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("search error %s: %s", res.Status(), raw)
	}

	var result struct {
		TimedOut bool `json:"timed_out"`
		Shards   struct {
			Failed int `json:"failed"`
		} `json:"_shards"`
		Hits struct {
			Hits []struct {
				ID string `json:"_id"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}
	if result.TimedOut || result.Shards.Failed > 0 {
		return nil, fmt.Errorf("opensearch returned partial result (timed_out=%v, shards_failed=%d)",
			result.TimedOut, result.Shards.Failed)
	}

	found := make(map[string]struct{}, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		found[hit.ID] = struct{}{}
	}
	return found, nil
}

func chunkStrings(items []string, size int) [][]string {
	var chunks [][]string
	for len(items) > 0 {
		n := size
		if n > len(items) {
			n = len(items)
		}
		chunks = append(chunks, items[:n])
		items = items[n:]
	}
	return chunks
}

// splitResourceID splits an OpenSearch "resources" index _id of the form
// "<object_type>:<uid>" into its UID and object type.
func splitResourceID(id string) (uid, docType string, ok bool) {
	for _, prefix := range []string{"project_settings:", "project:"} {
		if len(id) > len(prefix) && id[:len(prefix)] == prefix {
			return id[len(prefix):], prefix[:len(prefix)-1], true
		}
	}
	return "", "", false
}
