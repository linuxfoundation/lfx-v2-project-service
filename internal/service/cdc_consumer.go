// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
)

// CDCConsumer consumes normalized CDCEvents from a CDCSubscriber and dispatches
// each one to the appropriate handler. It is the single active consumer in
// consumer mode (enforced at the Kubernetes level via replicas:1 + Recreate —
// no application-level lease is needed).
//
// For each entity the handler:
//  1. Captures current record state (for reparenting diff on b2b_org).
//  2. Invalidates the sObject cache so the subsequent re-fetch is fresh.
//  3. Re-fetches the record from Salesforce via the reader port.
//  4. Publishes indexer + FGA fan-out messages (push-and-forget).
//  5. On DELETE: publishes a delete indexer event; no re-fetch.
type CDCConsumer struct {
	subscriber              port.CDCSubscriber
	memberReader            port.MemberReader
	projectMembershipReader port.ProjectMembershipReader
	b2bOrgReader            port.B2BOrgReader
	cacheInvalidator        port.CacheInvalidator
	publisher               port.MemberPublisher
	globalOrgAdminTeamUID   string
}

// CDCConsumerOption configures a CDCConsumer.
type CDCConsumerOption func(*CDCConsumer)

func WithCDCSubscriber(s port.CDCSubscriber) CDCConsumerOption {
	return func(o *CDCConsumer) { o.subscriber = s }
}

func WithCDCMemberReader(r port.MemberReader) CDCConsumerOption {
	return func(o *CDCConsumer) { o.memberReader = r }
}

func WithCDCProjectMembershipReader(r port.ProjectMembershipReader) CDCConsumerOption {
	return func(o *CDCConsumer) { o.projectMembershipReader = r }
}

func WithCDCB2BOrgReader(r port.B2BOrgReader) CDCConsumerOption {
	return func(o *CDCConsumer) { o.b2bOrgReader = r }
}

func WithCDCCacheInvalidator(i port.CacheInvalidator) CDCConsumerOption {
	return func(o *CDCConsumer) { o.cacheInvalidator = i }
}

func WithCDCPublisher(p port.MemberPublisher) CDCConsumerOption {
	return func(o *CDCConsumer) { o.publisher = p }
}

func WithCDCGlobalOrgAdminTeamUID(uid string) CDCConsumerOption {
	return func(o *CDCConsumer) { o.globalOrgAdminTeamUID = uid }
}

// NewCDCConsumer constructs a CDCConsumer.
func NewCDCConsumer(opts ...CDCConsumerOption) *CDCConsumer {
	o := &CDCConsumer{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Run subscribes to channel, processes events until ctx is cancelled, and
// persists the replay cursor after each event. It blocks until ctx is done.
func (o *CDCConsumer) Run(ctx context.Context, channel string, replay port.ReplayStore) error {
	replayID, err := replay.Load(ctx, channel)
	if err != nil {
		return err
	}

	eventCh, err := o.subscriber.Subscribe(ctx, channel, replayID, replay)
	if err != nil {
		return err
	}

	for event := range eventCh {
		// Give each handler a short-lived background context so that an
		// in-flight Salesforce fetch or NATS cache write is not aborted by a
		// concurrent graceful shutdown. 30 s matches the graceful-shutdown
		// window; any handler that runs longer than that is already a problem.
		handleCtx, handleCancel := context.WithTimeout(context.Background(), 30*time.Second)
		handleErr := o.handle(handleCtx, event)
		handleCancel()
		if handleErr != nil {
			// Log and continue — /admin/reindex is the backstop for missed events.
			slog.ErrorContext(ctx, "cdc: event handling failed, continuing",
				"entity", event.Entity,
				"change_type", event.ChangeType,
				"record_ids", event.RecordIDs,
				"error", handleErr,
			)
		}

		// Commit-after-process: persist cursor regardless of handler error so
		// a transient failure doesn't block the stream indefinitely.
		//
		// Use a short-lived background context for the Save so that a
		// graceful shutdown (which cancels ctx) does not prevent the last
		// replay cursor from being committed. Without this the final event
		// would be re-processed on every restart.
		saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
		saveErr := replay.Save(saveCtx, channel, event.ReplayID)
		saveCancel()
		if saveErr != nil {
			slog.WarnContext(ctx, "cdc: failed to save replay cursor",
				"channel", channel, "error", saveErr)
		}
	}

	return ctx.Err()
}

// handle dispatches a single CDCEvent to the correct entity handler.
func (o *CDCConsumer) handle(ctx context.Context, event model.CDCEvent) error {
	// GAP_* change types signal that Salesforce could not deliver granular
	// events (overflow, create gap, etc.). We re-fetch the record as an upsert
	// but log a WARN so operators know granular delivery was interrupted and
	// can cross-check /admin/reindex if needed.
	if strings.HasPrefix(string(event.ChangeType), "GAP_") {
		slog.WarnContext(ctx, "cdc: GAP event received — granular delivery interrupted, treating as upsert",
			"entity", event.Entity,
			"change_type", event.ChangeType,
			"record_ids", event.RecordIDs,
		)
	}

	switch event.Entity {
	case "Account":
		return o.handleAccount(ctx, event)
	case "Asset":
		return o.handleAsset(ctx, event)
	case "Project_Role__c":
		return o.handleProjectRole(ctx, event)
	default:
		slog.DebugContext(ctx, "cdc: unhandled entity, skipping", "entity", event.Entity)
		return nil
	}
}

// dispatchRecordIDs iterates over the record IDs in a CDC event and calls
// deleteFn or upsertFn for each one depending on the change type. Errors are
// logged and swallowed so a single bad record does not abort the rest of the
// batch — /admin/reindex is the backstop for missed records.
func (o *CDCConsumer) dispatchRecordIDs(
	ctx context.Context,
	entity string,
	event model.CDCEvent,
	deleteFn func(context.Context, string) error,
	upsertFn func(context.Context, string) error,
) error {
	for _, id := range event.RecordIDs {
		var err error
		// Match both DELETE and GAP_DELETE. HasSuffix would also match UNDELETE
		// (which ends with "DELETE") and route it to the wrong path.
		if event.ChangeType == model.CDCChangeDelete || event.ChangeType == model.CDCChangeGapDelete {
			err = deleteFn(ctx, id)
		} else {
			err = upsertFn(ctx, id)
		}
		if err != nil {
			slog.ErrorContext(ctx, "cdc: handler failed",
				"entity", entity, "uid", id, "change_type", event.ChangeType, "error", err)
		}
	}
	return nil
}

// ── Account (b2b_org) ─────────────────────────────────────────────────────────

func (o *CDCConsumer) handleAccount(ctx context.Context, event model.CDCEvent) error {
	return o.dispatchRecordIDs(ctx, "Account", event,
		o.handleAccountDelete, o.handleAccountUpsert)
}

func (o *CDCConsumer) handleAccountUpsert(ctx context.Context, uid string) error {
	// Capture old record BEFORE cache eviction for reparenting diff.
	// If the cache is cold, GetB2BOrg returns the post-change record — in that
	// case oldOrg == new org, no reparenting messages are emitted (safe).
	var oldOrg *model.B2BOrg
	if current, err := o.b2bOrgReader.GetB2BOrg(ctx, uid); err == nil {
		oldOrg = current
	}

	if err := o.cacheInvalidator.InvalidateB2BOrg(ctx, uid); err != nil {
		slog.WarnContext(ctx, "cdc: b2b_org cache invalidation failed, continuing",
			"uid", uid, "error", err, "publish_failed_for_backfill_repair", true)
	}

	org, err := o.b2bOrgReader.GetB2BOrg(ctx, uid)
	if err != nil {
		return err
	}

	// CDC always passes globalOrgAdminTeamUID (not create-only like the writer).
	publishB2BOrgUpsertEvents(ctx, o.b2bOrgReader, o.publisher, oldOrg, org, indexerConstants.ActionUpdated, o.globalOrgAdminTeamUID)
	return nil
}

func (o *CDCConsumer) handleAccountDelete(ctx context.Context, uid string) error {
	if err := o.cacheInvalidator.InvalidateB2BOrg(ctx, uid); err != nil {
		slog.WarnContext(ctx, "cdc: b2b_org cache invalidation failed on delete",
			"uid", uid, "error", err)
	}

	stubOrg := &model.B2BOrg{UID: uid}
	PublishB2BOrgIndexer(ctx, o.publisher, stubOrg, indexerConstants.ActionDeleted)

	// nil access (writers/auditors) = preserve; empty = clear. For delete we
	// pass nil to let FGA sync handle cleanup based on the delete indexer event.
	fgaMsg := BuildB2BOrgFGAMessage(stubOrg, o.globalOrgAdminTeamUID, nil, nil, nil)
	if err := o.publisher.Access(ctx, constants.FGASyncUpdateAccessSubject, fgaMsg, false); err != nil {
		slog.WarnContext(ctx, "cdc: b2b_org delete FGA publish failed",
			"uid", uid, "error", err, "publish_failed_for_backfill_repair", true)
	}
	return nil
}

// ── Asset (project_membership) ────────────────────────────────────────────────

func (o *CDCConsumer) handleAsset(ctx context.Context, event model.CDCEvent) error {
	return o.dispatchRecordIDs(ctx, "Asset", event,
		o.handleAssetDelete,
		func(ctx context.Context, id string) error {
			return o.handleAssetUpsert(ctx, id, event.ChangeType)
		})
}

func (o *CDCConsumer) handleAssetUpsert(ctx context.Context, uid string, changeType model.CDCChangeType) error {

	// Evict the sObject cache entry so the next read goes to Salesforce.
	// This must happen before AssembleProjectMembership, which reads through
	// the same sObject cache, to guarantee a fresh copy of the changed record.
	if err := o.cacheInvalidator.InvalidateProjectMembership(ctx, uid); err != nil {
		slog.WarnContext(ctx, "cdc: project_membership cache invalidation failed",
			"uid", uid, "error", err, "publish_failed_for_backfill_repair", true)
	}

	// Re-fetch via the sObject path (Asset + Account + Product2 + Project__c)
	// rather than the SOQL/membership-cache path. The sObject path bypasses
	// the membership-cache TTL so the published record reflects the CDC change.
	pm, _, err := o.projectMembershipReader.AssembleProjectMembership(ctx, uid)
	if err != nil {
		return err
	}

	action := indexerConstants.ActionUpdated
	if changeType == model.CDCChangeCreate {
		action = indexerConstants.ActionCreated
	}

	PublishProjectMembershipIndexer(ctx, o.publisher, pm, action)
	PublishProjectMembershipFGA(ctx, o.publisher, pm)
	return nil
}

func (o *CDCConsumer) handleAssetDelete(ctx context.Context, uid string) error {
	if err := o.cacheInvalidator.InvalidateProjectMembership(ctx, uid); err != nil {
		slog.WarnContext(ctx, "cdc: project_membership cache invalidation failed on delete",
			"uid", uid, "error", err)
	}

	stubPM := &model.ProjectMembership{UID: uid}
	PublishProjectMembershipIndexer(ctx, o.publisher, stubPM, indexerConstants.ActionDeleted)
	return nil
}

// ── Project_Role__c (key_contact) ─────────────────────────────────────────────

func (o *CDCConsumer) handleProjectRole(ctx context.Context, event model.CDCEvent) error {
	return o.dispatchRecordIDs(ctx, "Project_Role__c", event,
		o.handleProjectRoleDelete,
		func(ctx context.Context, id string) error {
			return o.handleProjectRoleUpsert(ctx, id, event.ChangeType)
		})
}

func (o *CDCConsumer) handleProjectRoleUpsert(ctx context.Context, uid string, changeType model.CDCChangeType) error {

	if err := o.cacheInvalidator.InvalidateKeyContact(ctx, uid); err != nil {
		slog.WarnContext(ctx, "cdc: key_contact cache invalidation failed",
			"uid", uid, "error", err, "publish_failed_for_backfill_repair", true)
	}

	kc, err := o.memberReader.GetKeyContact(ctx, uid)
	if err != nil {
		return err
	}

	action := indexerConstants.ActionUpdated
	if changeType == model.CDCChangeCreate {
		action = indexerConstants.ActionCreated
	}

	PublishKeyContactIndexer(ctx, o.publisher, kc, action)
	PublishKeyContactFGA(ctx, o.publisher, kc)
	return nil
}

func (o *CDCConsumer) handleProjectRoleDelete(ctx context.Context, uid string) error {
	if err := o.cacheInvalidator.InvalidateKeyContact(ctx, uid); err != nil {
		slog.WarnContext(ctx, "cdc: key_contact cache invalidation failed on delete",
			"uid", uid, "error", err)
	}

	stubKC := &model.KeyContact{UID: uid}
	PublishKeyContactIndexer(ctx, o.publisher, stubKC, indexerConstants.ActionDeleted)

	// key_contact delete FGA revoke: best-effort, alertable on failure.
	// Uses GenericMemberRemoveSubject to match the key_contact_writer delete path.
	// The username is not available from the CDC event — the FGA sync
	// service performs cleanup by object-id when username is empty.
	if err := o.publisher.Access(ctx, fgaconstants.GenericMemberRemoveSubject,
		BuildKeyContactFGARemoveMessage(uid, ""), false); err != nil {
		// fga_revoke_failed_dangling_tuple=true signals a dangling FGA tuple:
		// the key_contact was deleted in Salesforce but the FGA relation was not
		// revoked. Unlike publish_failed_for_backfill_repair, this cannot be
		// recovered by /admin/reindex — requires a targeted FGA sync or
		// re-sending the remove message manually.
		slog.ErrorContext(ctx, "cdc: key_contact delete FGA revoke failed — dangling tuple requires manual cleanup",
			"uid", uid, "error", err, "fga_revoke_failed_dangling_tuple", true)
	}
	return nil
}
