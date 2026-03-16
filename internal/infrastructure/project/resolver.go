// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package project provides the ProjectResolver infrastructure implementation,
// which translates between v2 project UIDs, project slugs, and Salesforce
// Project__c.Id SFIDs. Resolution results are cached in the membership-cache
// NATS KV bucket to avoid repeated NATS RPC and SOQL round-trips.
package project

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/salesforce"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// Resolver implements port.ProjectResolver by chaining NATS RPC calls to the
// project-service with SOQL queries to Salesforce B2B, backed by a NATS KV
// cache for both directions of the mapping.
type Resolver struct {
	rpc   *nats.ProjectRPC
	repo  *salesforce.ProjectRepo
	cache *nats.Storage
}

// Ensure Resolver satisfies the port at compile time.
var _ port.ProjectResolver = (*Resolver)(nil)

// NewProjectResolver creates a new Resolver. All arguments are required.
func NewProjectResolver(
	rpc *nats.ProjectRPC,
	repo *salesforce.ProjectRepo,
	cache *nats.Storage,
) *Resolver {
	return &Resolver{
		rpc:   rpc,
		repo:  repo,
		cache: cache,
	}
}

// SFIDFromUID resolves a v2 project UID to a Salesforce Project__c.Id.
//
// Resolution chain:
//  1. Check KV cache (project-sfid/{uid}).
//  2. Call project-service NATS RPC to get the slug (lfx.projects-api.get_slug).
//  3. Cache the slug for the UID in KV (project-uid/{slug} → uid).
//  4. Query Salesforce SOQL for the Project__c.Id by Slug__c.
//  5. Cache the SFID in KV (project-sfid/{uid} → sfid) and return it.
//
// Returns a NotFound error if the project does not exist in the project-service
// or in Salesforce, and an internal error if Salesforce returns ambiguous results.
func (r *Resolver) SFIDFromUID(ctx context.Context, projectUID string) (string, error) {
	// Step 1: check KV cache.
	result, err := r.cache.GetProjectSFID(ctx, projectUID)
	if err != nil {
		slog.WarnContext(ctx, "failed to read project SFID from cache; proceeding with live lookup",
			"project_uid", projectUID,
			"error", err,
		)
	} else if result.Status == nats.CacheStatusFresh || result.Status == nats.CacheStatusStale {
		if result.Status == nats.CacheStatusStale {
			slog.DebugContext(ctx, "serving stale project SFID from cache",
				"project_uid", projectUID,
			)
		}
		return result.Value, nil
	}

	// Step 2: resolve UID → slug via project-service NATS RPC.
	slug, err := r.rpc.GetSlug(ctx, projectUID)
	if err != nil {
		return "", fmt.Errorf("resolving slug for project UID %s: %w", projectUID, err)
	}
	if slug == "" {
		return "", errs.NewNotFound("project not found", fmt.Errorf("uid: %s", projectUID))
	}

	// Step 3: also cache the slug → UID mapping as a side effect.
	if putErr := r.cache.PutProjectUID(ctx, slug, projectUID); putErr != nil {
		slog.WarnContext(ctx, "failed to cache project UID by slug",
			"slug", slug,
			"project_uid", projectUID,
			"error", putErr,
		)
	}

	// Step 4: query Salesforce for the Project__c.Id by Slug__c.
	sfid, err := r.repo.FetchSFIDBySlug(ctx, slug)
	if err != nil {
		return "", fmt.Errorf("resolving Salesforce SFID for project slug %q: %w", slug, err)
	}
	if sfid == "" {
		return "", errs.NewNotFound(
			"project exists in v2 but not found in Salesforce",
			fmt.Errorf("uid: %s, slug: %s", projectUID, slug),
		)
	}

	// Step 5: cache the UID → SFID mapping and return.
	if putErr := r.cache.PutProjectSFID(ctx, projectUID, sfid); putErr != nil {
		slog.WarnContext(ctx, "failed to cache project SFID",
			"project_uid", projectUID,
			"sfid", sfid,
			"error", putErr,
		)
	}

	return sfid, nil
}

// UIDFromSlug resolves a project slug to a v2 project UID.
//
// Resolution chain:
//  1. Check KV cache (project-uid/{slug}).
//  2. Call project-service NATS RPC to get the UID (lfx.projects-api.slug_to_uid).
//  3. Cache the slug → UID mapping in KV and return.
//
// Returns a NotFound error if the slug is not known to the project-service.
func (r *Resolver) UIDFromSlug(ctx context.Context, slug string) (string, error) {
	// Step 1: check KV cache.
	result, err := r.cache.GetProjectUID(ctx, slug)
	if err != nil {
		slog.WarnContext(ctx, "failed to read project UID from cache; proceeding with live lookup",
			"slug", slug,
			"error", err,
		)
	} else if result.Status == nats.CacheStatusFresh || result.Status == nats.CacheStatusStale {
		if result.Status == nats.CacheStatusStale {
			slog.DebugContext(ctx, "serving stale project UID from cache",
				"slug", slug,
			)
		}
		return result.Value, nil
	}

	// Step 2: resolve slug → UID via project-service NATS RPC.
	uid, err := r.rpc.SlugToUID(ctx, slug)
	if err != nil {
		return "", fmt.Errorf("resolving UID for project slug %q: %w", slug, err)
	}
	if uid == "" {
		return "", errs.NewNotFound("project not found", fmt.Errorf("slug: %s", slug))
	}

	// Step 3: cache the slug → UID mapping and return.
	if putErr := r.cache.PutProjectUID(ctx, slug, uid); putErr != nil {
		slog.WarnContext(ctx, "failed to cache project UID by slug",
			"slug", slug,
			"uid", uid,
			"error", putErr,
		)
	}

	return uid, nil
}
