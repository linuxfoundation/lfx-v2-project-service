// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import "context"

// MemberPublisher publishes domain events to the LFX event bus. It is used by
// write-path use cases to trigger downstream indexing and FGA synchronisation
// after successful mutations.
//
// Both methods accept a sync flag: when true the call blocks until the remote
// acknowledges the message (used for synchronous write paths); when false the
// message is fire-and-forget. Write handlers pass sync=false so that publish
// failures never block or fail an HTTP response.
//
// Publish-failure policy:
//   - Creates and updates: swallow the error and log at warn with
//     publish_failed_for_backfill_repair=true so the /admin/reindex backfill
//     can recover the record.
//   - Deletes: propagate the error to the caller; a delete without FGA/index
//     cleanup leaves dangling permissions.
type MemberPublisher interface {
	// Indexer publishes an indexer message to the given NATS subject.
	// msg must be a pre-built, JSON-serialisable value (e.g. *model.MemberIndexerMessage);
	// the publisher marshals and forwards it as-is.
	Indexer(ctx context.Context, subject string, msg any, sync bool) error

	// Access publishes an FGA synchronisation message to the given NATS subject.
	// msg must be JSON-serialisable (typically a fgatypes.GenericFGAMessage).
	Access(ctx context.Context, subject string, msg any, sync bool) error
}
