// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package constants defines shared constant values used across the service.
package constants

// NATS Key-Value store bucket names.
const (
	// KVBucketNameCache is the name of the single KV bucket used for all cached
	// membership records. Keys within the bucket are namespaced by type prefix
	// (e.g. "tier/{uid}", "membership/{uid}", "key-contacts/{membership_uid}",
	// "project-sfid/{uid}", "project-uid/{slug}") to avoid collisions.
	KVBucketNameCache = "membership-cache"

	// KVBucketNameSObjectCache is the name of the KV bucket used for the new
	// sObject REST API cache. Keys use the pattern "{type}.{uid}" (e.g.
	// "b2b_org.{uid}", "project_membership.{uid}"). Values carry HTTP conditional
	// GET metadata (ETag, Last-Modified) alongside the JSON-encoded sObject body,
	// enabling If-None-Match / If-Modified-Since cache validation on re-fetch.
	KVBucketNameSObjectCache = "member-service-cache"

	// KVBucketNameOrgSettings is the name of the KV bucket for authoritative
	// b2b_org settings (writers, auditors, pending invites) and the per-org
	// membership-UID index. No MaxAge TTL — entries are never silently evicted.
	// Mirrors the committee-service "committee-settings" bucket pattern.
	KVBucketNameOrgSettings = "org-settings"
)
