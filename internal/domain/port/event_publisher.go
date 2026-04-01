// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import "context"

// EventPublisher publishes domain events to the LFX event bus. It is used by
// write-path use cases to trigger downstream indexing and FGA synchronization
// after successful mutations.
//
// NOTE: The parameter types for PublishIndexerEvent (cfg) and
// PublishFGASyncEvent (msg) are currently defined as `any` because the
// indexertypes and fgasync packages do not yet exist in this repository. They
// will be replaced with concrete types (indexertypes.IndexingConfig and
// fgasync.GenericFGAMessage respectively) when those packages are added in a
// later ticket.
type EventPublisher interface {
	// PublishIndexerEvent publishes an event to the LFX indexer so that the
	// given object is re-indexed. objectType identifies the resource type (e.g.
	// "b2b_org"), action identifies the mutation ("create", "update",
	// "delete"), data is the serializable payload, and cfg carries indexer
	// routing configuration (placeholder type until the indexertypes package
	// exists).
	PublishIndexerEvent(ctx context.Context, objectType, action string, data any, cfg any) error

	// PublishFGASyncEvent publishes a generic FGA synchronization message so
	// that OpenFGA permission tuples are kept consistent after a mutation. msg
	// is the synchronization payload (placeholder type until the fgasync
	// package exists).
	PublishFGASyncEvent(ctx context.Context, msg any) error
}
