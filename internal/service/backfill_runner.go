// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	natspkg "github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/nats"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// BackfillIterator provides paged SOQL iterators for full and since-filtered
// backfill modes. Each method calls fn once per page of converted records.
type BackfillIterator interface {
	IterB2BOrgs(ctx context.Context, since *time.Time, fn func([]*model.B2BOrg) error) error
	IterProjectMemberships(ctx context.Context, since *time.Time, fn func([]*model.ProjectMembership) error) error
	IterKeyContacts(ctx context.Context, since *time.Time, fn func([]*model.KeyContact) error) error
}

// KeyContactSObjectReader fetches a single KeyContact by UID via the live sObject path.
// Defined here to avoid a direct service→salesforce dependency while keeping the
// port package free of infrastructure concerns.
type KeyContactSObjectReader interface {
	AssembleKeyContact(ctx context.Context, uid string) (*model.KeyContact, time.Time, error)
}

const (
	entityTypeB2BOrg            = "b2b_org"
	entityTypeProjectMembership = "project_membership"
	entityTypeKeyContact        = "key_contact"
)

// allBackfillTypes is the canonical ordered list of types the backfill supports.
var allBackfillTypes = []string{entityTypeB2BOrg, entityTypeProjectMembership, entityTypeKeyContact}

// Runner orchestrates a reindex run. It is safe to call Run concurrently
// from multiple goroutines (each run is independent). Full-mode runs acquire a
// per-type NATS KV lock so the same type is not reindexed simultaneously across
// pods.
type Runner struct {
	iter       BackfillIterator
	b2bReader  port.B2BOrgReader
	pmReader   port.ProjectMembershipReader
	kcReader   KeyContactSObjectReader
	publisher  port.MemberPublisher
	natsClient *natspkg.NATSClient
}

// NewRunner constructs a Runner.
func NewRunner(
	iter BackfillIterator,
	b2bReader port.B2BOrgReader,
	pmReader port.ProjectMembershipReader,
	kcReader KeyContactSObjectReader,
	publisher port.MemberPublisher,
	natsClient *natspkg.NATSClient,
) *Runner {
	return &Runner{
		iter:       iter,
		b2bReader:  b2bReader,
		pmReader:   pmReader,
		kcReader:   kcReader,
		publisher:  publisher,
		natsClient: natsClient,
	}
}

type runMode string

const (
	modeTargeted runMode = "targeted"
	modeFiltered runMode = "filtered"
	modeFull     runMode = "full"
)

// ClassifyMode returns the run mode for the given request.
func ClassifyMode(req BackfillRequest) runMode {
	if len(req.Items) > 0 {
		return modeTargeted
	}
	if req.Since != nil {
		return modeFiltered
	}
	return modeFull
}

func effectiveTypes(requested []string) []string {
	if len(requested) == 0 {
		return allBackfillTypes
	}
	return requested
}

// Run executes the backfill. It is intended to be called in a goroutine with
// context.WithoutCancel so it outlives the HTTP request.
func (r *Runner) Run(ctx context.Context, req BackfillRequest) {
	mode := ClassifyMode(req)
	log := slog.With(
		"run_id", req.RunID,
		"component", "backfill",
		"mode", string(mode),
		"dry_run", req.DryRun,
	)
	log.InfoContext(ctx, "backfill started")
	defer log.InfoContext(ctx, "backfill complete")

	if mode == modeTargeted {
		r.runTargeted(ctx, log, req)
		return
	}

	if mode == modeFull {
		log.WarnContext(ctx, "full reindex started", "full_reindex_started", true)
	}

	succeeded := make([]string, 0, len(allBackfillTypes))
	var failed, skipped []string

	for _, t := range effectiveTypes(req.Types) {
		var err error
		if mode == modeFull && r.natsClient != nil {
			release, lockErr := natspkg.AcquireFullRunLock(ctx, r.natsClient, req.RunID, t)
			if lockErr != nil {
				log.WarnContext(ctx, "full reindex skipped — lock held",
					"type", t,
					"full_reindex_rejected_lock_held", true,
					"error", lockErr)
				skipped = append(skipped, t)
				continue
			}
			err = r.runType(ctx, log, req, t)
			release()
		} else {
			err = r.runType(ctx, log, req, t)
		}

		if err != nil {
			log.ErrorContext(ctx, "backfill type failed",
				"type", t,
				"error", err)
			failed = append(failed, t)
		} else {
			succeeded = append(succeeded, t)
		}
	}

	log.InfoContext(ctx, "backfill summary",
		"succeeded", succeeded,
		"failed", failed,
		"skipped_locked", skipped)
}

func (r *Runner) runType(ctx context.Context, log *slog.Logger, req BackfillRequest, sfType string) error {
	var total, published int

	logPage := func(pageLen int) {
		log.InfoContext(ctx, "backfill page processed",
			"type", sfType, "page_size", pageLen,
			"total_so_far", total, "published_so_far", published)
	}

	switch sfType {
	case entityTypeB2BOrg:
		return r.iter.IterB2BOrgs(ctx, req.Since, func(orgs []*model.B2BOrg) error {
			for _, org := range orgs {
				total++
				if !req.DryRun {
					PublishB2BOrgIndexer(ctx, r.publisher, org, indexerConstants.ActionUpdated)
					published++
				}
			}
			logPage(len(orgs))
			return nil
		})
	case entityTypeProjectMembership:
		return r.iter.IterProjectMemberships(ctx, req.Since, func(pms []*model.ProjectMembership) error {
			for _, pm := range pms {
				total++
				if !req.DryRun {
					PublishProjectMembershipIndexer(ctx, r.publisher, pm, indexerConstants.ActionUpdated)
					PublishProjectMembershipFGA(ctx, r.publisher, pm)
					published++
				}
			}
			logPage(len(pms))
			return nil
		})
	case entityTypeKeyContact:
		if req.Since != nil {
			log.WarnContext(ctx, "since filter on key_contact only checks Project_Role__c.LastModifiedDate; Contact/Asset field changes are not captured",
				"since_filter_misses_joined_fields", true)
		}
		return r.iter.IterKeyContacts(ctx, req.Since, func(kcs []*model.KeyContact) error {
			for _, kc := range kcs {
				total++
				if !req.DryRun {
					PublishKeyContactIndexer(ctx, r.publisher, kc, indexerConstants.ActionUpdated)
					published++
				}
			}
			logPage(len(kcs))
			return nil
		})
	default:
		return fmt.Errorf("unhandled backfill type: %q", sfType)
	}
}

func (r *Runner) runTargeted(ctx context.Context, log *slog.Logger, req BackfillRequest) {
	var notFound, published int

	for _, item := range req.Items {
		switch item.Type {
		case entityTypeB2BOrg:
			org, err := r.b2bReader.GetB2BOrg(ctx, item.UID)
			if err != nil {
				if errs.IsNotFound(err) {
					log.WarnContext(ctx, "targeted item not found", "type", item.Type, "uid", item.UID, "not_found", true)
					notFound++
				} else {
					log.WarnContext(ctx, "targeted item fetch error", "type", item.Type, "uid", item.UID, "error", err,
						"publish_failed_for_backfill_repair", true)
				}
				continue
			}
			if !req.DryRun {
				PublishB2BOrgIndexer(ctx, r.publisher, org, indexerConstants.ActionUpdated)
				published++
			}

		case entityTypeProjectMembership:
			pm, _, err := r.pmReader.AssembleProjectMembership(ctx, item.UID)
			if err != nil {
				if errs.IsNotFound(err) {
					log.WarnContext(ctx, "targeted item not found", "type", item.Type, "uid", item.UID, "not_found", true)
					notFound++
				} else {
					log.WarnContext(ctx, "targeted item fetch error", "type", item.Type, "uid", item.UID, "error", err,
						"publish_failed_for_backfill_repair", true)
				}
				continue
			}
			if !req.DryRun {
				PublishProjectMembershipIndexer(ctx, r.publisher, pm, indexerConstants.ActionUpdated)
				PublishProjectMembershipFGA(ctx, r.publisher, pm)
				published++
			}

		case entityTypeKeyContact:
			kc, _, err := r.kcReader.AssembleKeyContact(ctx, item.UID)
			if err != nil {
				if errs.IsNotFound(err) {
					log.WarnContext(ctx, "targeted item not found", "type", item.Type, "uid", item.UID, "not_found", true)
					notFound++
				} else {
					log.WarnContext(ctx, "targeted item fetch error", "type", item.Type, "uid", item.UID, "error", err,
						"publish_failed_for_backfill_repair", true)
				}
				continue
			}
			if !req.DryRun {
				PublishKeyContactIndexer(ctx, r.publisher, kc, indexerConstants.ActionUpdated)
				published++
			}
		}
	}

	log.InfoContext(ctx, "targeted backfill complete",
		"total_items", len(req.Items),
		"published", published,
		"not_found", notFound,
		"would_publish_count", len(req.Items)-notFound)
}
