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

	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	natsinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

type documentAuditUsersSubcommand struct{}

func (s *documentAuditUsersSubcommand) Name() string { return "document-audit-users" }

func (s *documentAuditUsersSubcommand) Help() string {
	return "backfill created_by/updated_by profiles on project folders, links, and documents; re-index OpenSearch"
}

func (s *documentAuditUsersSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	slog.DebugContext(ctx, "starting subcommand", "subcommand", s.Name(), "args", rc.Args)

	fs := flag.NewFlagSet("document-audit-users", flag.ContinueOnError)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "usage: project-cli sync document-audit-users [flags]\n\nflags:\n")
		fs.PrintDefaults()
	}
	projectUID := fs.String("project-uid", "", "limit migration to a single project UID")
	resourceType := fs.String("resource-type", "", "optional filter: folder, link, or document")
	sleep := fs.Duration("sleep", 0, "wait between each auth-service lookup (e.g. 200ms, 1s)")
	dryRun := fs.Bool("dry-run", true, "log what would change without writing (pass --dry-run=false to write)")
	if err := fs.Parse(rc.Args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	includeFolders, includeLinks, includeDocuments, err := parseDocumentResourceType(*resourceType)
	if err != nil {
		return err
	}

	rc.DryRun = *dryRun
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

	runner := &documentAuditUsersRunner{
		repo:       repo,
		userReader: &natsinfra.UserReaderNATS{NatsConn: natsConn},
		publisher:  &natsinfra.MessageBuilder{NatsConn: natsConn},
		dryRun:     rc.DryRun,
		sleep:      *sleep,
		stats:      commands.NewStats(),
	}
	runner.stats.DryRun = rc.DryRun

	if err := runner.run(ctx, strings.TrimSpace(*projectUID), includeFolders, includeLinks, includeDocuments); err != nil {
		return err
	}

	runner.stats.Log(ctx, "sync document-audit-users")
	if runner.stats.Failed > 0 {
		return fmt.Errorf("%d resource(s) failed to migrate", runner.stats.Failed)
	}
	return nil
}

type documentAuditUsersRunner struct {
	repo       *natsinfra.NatsRepository
	userReader domain.UserReader
	publisher  domain.MessageBuilder
	dryRun     bool
	sleep      time.Duration
	stats      *commands.Stats
}

func (r *documentAuditUsersRunner) run(ctx context.Context, projectUID string, folders, links, documents bool) error {
	if folders {
		if err := r.migrateFolders(ctx, projectUID); err != nil {
			return err
		}
	}
	if links {
		if err := r.migrateLinks(ctx, projectUID); err != nil {
			return err
		}
	}
	if documents {
		if err := r.migrateDocuments(ctx, projectUID); err != nil {
			return err
		}
	}
	return nil
}

func (r *documentAuditUsersRunner) migrateFolders(ctx context.Context, projectUID string) error {
	folders, err := r.repo.ListFolders(ctx, projectUID)
	if err != nil {
		return fmt.Errorf("list folders: %w", err)
	}
	for _, folder := range folders {
		r.stats.Total++
		if err := r.migrateFolder(ctx, folder); err != nil {
			return err
		}
	}
	return nil
}

func (r *documentAuditUsersRunner) migrateFolder(ctx context.Context, folder *models.ProjectFolder) error {
	fresh, revision, err := r.repo.GetFolder(ctx, folder.ProjectUID, folder.UID)
	if err != nil {
		slog.WarnContext(ctx, "failed to re-read folder before audit backfill",
			"folder_uid", folder.UID, "project_uid", folder.ProjectUID, constants.ErrKey, err)
		r.stats.Failed++
		return nil
	}
	return r.applyAuditUsers(ctx, freshAuditResource{
		resourceType: "folder",
		uid:          fresh.UID,
		projectUID:   fresh.ProjectUID,
		name:         fresh.Name,
		createdBy:    fresh.CreatedBy,
		updatedBy:    fresh.UpdatedBy,
	}, func(profile *models.UserInfo) error {
		fresh.CreatedBy = profile
		fresh.UpdatedBy = models.CloneUserInfo(profile)
		if r.dryRun {
			return nil
		}
		if err := r.repo.UpdateFolder(ctx, fresh, revision); err != nil {
			return err
		}
		msg := indexerTypes.IndexerMessageEnvelope{
			Action:         indexerConstants.ActionUpdated,
			Data:           *fresh,
			IndexingConfig: fresh.IndexingConfig(),
		}
		return r.publisher.SendIndexerMessage(ctx, constants.IndexProjectFolderSubject, msg, false)
	})
}

func (r *documentAuditUsersRunner) migrateLinks(ctx context.Context, projectUID string) error {
	links, err := r.repo.ListAllLinks(ctx, projectUID)
	if err != nil {
		return fmt.Errorf("list links: %w", err)
	}
	for _, link := range links {
		r.stats.Total++
		if err := r.migrateLink(ctx, link); err != nil {
			return err
		}
	}
	return nil
}

func (r *documentAuditUsersRunner) migrateLink(ctx context.Context, link *models.ProjectLink) error {
	fresh, revision, err := r.repo.GetLink(ctx, link.ProjectUID, link.UID)
	if err != nil {
		slog.WarnContext(ctx, "failed to re-read link before audit backfill",
			"link_uid", link.UID, "project_uid", link.ProjectUID, constants.ErrKey, err)
		r.stats.Failed++
		return nil
	}
	return r.applyAuditUsers(ctx, freshAuditResource{
		resourceType: "link",
		uid:          fresh.UID,
		projectUID:   fresh.ProjectUID,
		name:         fresh.Name,
		createdBy:    fresh.CreatedBy,
		updatedBy:    fresh.UpdatedBy,
	}, func(profile *models.UserInfo) error {
		fresh.CreatedBy = profile
		fresh.UpdatedBy = models.CloneUserInfo(profile)
		if r.dryRun {
			return nil
		}
		if err := r.repo.UpdateLink(ctx, fresh, revision); err != nil {
			return err
		}
		msg := indexerTypes.IndexerMessageEnvelope{
			Action:         indexerConstants.ActionUpdated,
			Data:           *fresh,
			IndexingConfig: fresh.IndexingConfig(),
		}
		return r.publisher.SendIndexerMessage(ctx, constants.IndexProjectLinkSubject, msg, false)
	})
}

func (r *documentAuditUsersRunner) migrateDocuments(ctx context.Context, projectUID string) error {
	docs, err := r.repo.ListAllDocuments(ctx, projectUID)
	if err != nil {
		return fmt.Errorf("list documents: %w", err)
	}
	for _, doc := range docs {
		r.stats.Total++
		if err := r.migrateDocument(ctx, doc); err != nil {
			return err
		}
	}
	return nil
}

func (r *documentAuditUsersRunner) migrateDocument(ctx context.Context, doc *models.ProjectDocument) error {
	fresh, revision, err := r.repo.GetDocumentMetadata(ctx, doc.ProjectUID, doc.UID)
	if err != nil {
		slog.WarnContext(ctx, "failed to re-read document before audit backfill",
			"document_uid", doc.UID, "project_uid", doc.ProjectUID, constants.ErrKey, err)
		r.stats.Failed++
		return nil
	}
	return r.applyAuditUsers(ctx, freshAuditResource{
		resourceType: "document",
		uid:          fresh.UID,
		projectUID:   fresh.ProjectUID,
		name:         fresh.Name,
		createdBy:    fresh.CreatedBy,
		updatedBy:    fresh.UpdatedBy,
	}, func(profile *models.UserInfo) error {
		fresh.CreatedBy = profile
		fresh.UpdatedBy = models.CloneUserInfo(profile)
		if r.dryRun {
			return nil
		}
		if err := r.repo.UpdateDocumentMetadata(ctx, fresh, revision); err != nil {
			return err
		}
		msg := indexerTypes.IndexerMessageEnvelope{
			Action:         indexerConstants.ActionUpdated,
			Data:           *fresh,
			IndexingConfig: fresh.IndexingConfig(),
		}
		return r.publisher.SendIndexerMessage(ctx, constants.IndexProjectDocumentSubject, msg, false)
	})
}

type freshAuditResource struct {
	resourceType string
	uid          string
	projectUID   string
	name         string
	createdBy    *models.UserInfo
	updatedBy    *models.UserInfo
}

func (r *documentAuditUsersRunner) applyAuditUsers(
	ctx context.Context,
	res freshAuditResource,
	apply func(*models.UserInfo) error,
) error {
	if !models.AuditUserNeedsMigration(res.createdBy) {
		r.stats.Skipped++
		return nil
	}

	username := models.AuditCreatorUsername(res.createdBy)
	profile := service.ResolveAuditUserProfile(ctx, r.userReader, username)
	if profile == nil {
		r.stats.Skipped++
		return nil
	}

	slog.InfoContext(ctx, "document audit user drift detected",
		"resource_type", res.resourceType,
		"resource_uid", res.uid,
		"project_uid", res.projectUID,
		"name", res.name,
		"username", username,
		"resolved_name", profile.Name,
		"dry_run", r.dryRun,
	)

	if r.dryRun {
		r.stats.Updated++
		if r.sleep > 0 {
			return documentAuditSleep(ctx, r.sleep)
		}
		return nil
	}

	if err := apply(profile); err != nil {
		slog.WarnContext(ctx, "failed to migrate document audit users",
			"resource_type", res.resourceType,
			"resource_uid", res.uid,
			"project_uid", res.projectUID,
			constants.ErrKey, err,
		)
		r.stats.Failed++
		return nil
	}

	r.stats.Updated++
	if r.sleep > 0 {
		return documentAuditSleep(ctx, r.sleep)
	}
	return nil
}

func parseDocumentResourceType(raw string) (folders, links, documents bool, err error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return true, true, true, nil
	}
	switch raw {
	case "folder":
		return true, false, false, nil
	case "link":
		return false, true, false, nil
	case "document":
		return false, false, true, nil
	default:
		return false, false, false, fmt.Errorf("invalid --resource-type %q: want folder, link, or document", raw)
	}
}

func documentAuditSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
