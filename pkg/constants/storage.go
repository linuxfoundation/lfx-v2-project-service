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
)
