// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package port defines the domain-layer interfaces implemented by the
// infrastructure layer.
package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// ProjectResolver translates between v2 project UIDs, project slugs, and
// Salesforce Project__c.Id SFIDs. Implementations are expected to cache
// results to avoid repeated NATS RPC and SOQL round-trips.
type ProjectResolver interface {
	// SFIDFromUID resolves a v2 project UID to a Salesforce Project__c.Id.
	// Used when routing project-scoped SOQL queries. Returns a NotFound error
	// if the project does not exist in the project-service or in Salesforce.
	SFIDFromUID(ctx context.Context, projectUID string) (sfid string, err error)

	// UIDFromSlug resolves a project slug to a v2 project UID.
	// Used when populating ProjectUID on domain model objects returned from
	// Salesforce. Returns a NotFound error if the slug is not known to the
	// project-service.
	UIDFromSlug(ctx context.Context, slug string) (uid string, err error)

	// ResolveProject resolves a project identifier (either a v2 UUID or a slug)
	// to a fully-enriched ProjectInfo{UID, SFID, Name, Slug}. Used when adding a
	// project to a workspace to capture a write-time snapshot of project metadata.
	// Returns a Validation error (→ HTTP 400) if the project cannot be found.
	ResolveProject(ctx context.Context, idOrSlug string) (model.ProjectInfo, error)

	// ResolveProjectsBatch resolves a slice of project identifiers (UIDs or slugs)
	// in bulk, issuing a single batch SOQL query for name+slug enrichment after
	// per-item SFID resolution. Returns slices of the same length as input:
	// each index holds either a populated ProjectInfo or a non-nil error (but not
	// both). The caller decides whether to abort or collect partial results.
	ResolveProjectsBatch(ctx context.Context, idsOrSlugs []string) ([]model.ProjectInfo, []error)
}
