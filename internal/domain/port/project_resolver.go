// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package port defines the domain-layer interfaces implemented by the
// infrastructure layer.
package port

import "context"

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
}
