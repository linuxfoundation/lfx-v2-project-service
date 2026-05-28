<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Salesforce-backed cache and ProjectResolver

This service has no PostgreSQL and no sync job. Current live membership and
B2B detour endpoints fetch on demand from Salesforce SOQL and cache results in
NATS JetStream KV. The repo also contains an initialized sObject conditional
GET cache for the target architecture.

## NATS KV cache

### `membership-cache` bucket

All records share the `membership-cache` bucket. Keys are namespaced by a type
prefix to avoid collisions.

| Key pattern | Contents | Soft TTL |
| --- | --- | --- |
| `tier.{uid}` | `CachedValue[*model.MembershipTier]` | 6 h stale / 23 h expire |
| `membership.{uid}` | `CachedValue[*model.ProjectMembership]` | 6 h stale / 23 h expire |
| `key-contacts.{membership_uid}` | `CachedValue[[]*model.KeyContact]` | 6 h stale / 23 h expire |
| `project-sfid.{project_uid}` | `CachedValue[string]` (Salesforce Project__c.Id) | 6 h stale / 23 h expire |
| `project-uid.{slug}` | `CachedValue[string]` (v2 project UUID) | 6 h stale / 23 h expire |
| `soql.{template}.{params...}.{batch}` | `MembershipBatchCacheEntry` for paged SOQL results | 6 h stale / 23 h expire |
| `soql.b2b-orgs.{params...}.{batch}` | `B2BOrgBatchCacheEntry` for B2B org search results | 6 h stale / 23 h expire |

The NATS bucket itself has a 24-hour `MaxAge` (hard eviction), which is always
later than the soft `expires_at` timestamp inside each envelope.

### `member-service-cache` bucket

This bucket stores raw Salesforce sObject REST API responses with HTTP
conditional request metadata. It is initialized by the NATS client and chart,
and is used by `SObjectClient`; the current live HTTP provider wiring still
uses SOQL-backed `MemberReader` and `B2BOrgReader` implementations. It does
not use `CachedValue` soft TTLs; freshness is governed by Salesforce `ETag`
and `Last-Modified` revalidation. The bucket TTL is a 7-day backstop for quiet
records, and the client rewrites unchanged entries after `304 Not Modified` to
reset that TTL.

| Key pattern | Contents |
| --- | --- |
| `b2b_org.{uid}` | Salesforce Account sObject cache entry |
| `project_membership.{uid}` | Salesforce Asset sObject cache entry |
| `key_contact.{uid}` | Salesforce Project_Role__c sObject cache entry |
| `membership_tier.{uid}` | Salesforce Product2 sObject cache entry |

### Cache freshness states

Defined in `internal/infrastructure/nats/cache.go`:

| Status | Meaning | Caller behaviour |
| --- | --- | --- |
| `CacheStatusFresh` | Within stale threshold | Serve immediately. |
| `CacheStatusStale` | Past stale threshold, not yet expired | Serve immediately; trigger background refresh goroutine. |
| `CacheStatusExpired` | Past expiry threshold | Do not serve; fetch synchronously from Salesforce. |
| `CacheStatusMiss` | Key not present in bucket | Fetch synchronously from Salesforce. |

## ProjectResolver

`internal/infrastructure/project/resolver.go` implements `port.ProjectResolver`.
It is the bridge between the v2 project UUID world and the Salesforce
`Project__c.Id` world.

### Why it exists

Every project-scoped SOQL query requires a Salesforce `Project__c.Id` in its
`WHERE` clause. The HTTP API receives a v2 UUID (`project_id` path parameter).
Without `ProjectResolver`, all list endpoints would silently return zero
results.

### Resolution chain: `SFIDFromUID`

```text
SFIDFromUID(ctx, projectUID)
    |
    +-- 1. KV cache lookup: project-sfid.{uid}
    |        Fresh/Stale -> return cached SFID
    |
    +-- 2. NATS RPC -> project-service (lfx.projects-api.get_slug)
    |        returns slug string
    |
    +-- 3. KV cache write: project-uid.{slug} -> uid  (side-effect)
    |
    +-- 4. SOQL query -> Salesforce
    |        SELECT Id FROM Project__c WHERE Slug__c = '<slug>'
    |
    +-- 5. KV cache write: project-sfid.{uid} -> sfid
            return sfid
```

### Resolution chain: `UIDFromSlug`

```text
UIDFromSlug(ctx, slug)
    |
    +-- 1. KV cache lookup: project-uid.{slug}
    |        Fresh/Stale -> return cached UID
    |
    +-- 2. NATS RPC -> project-service (lfx.projects-api.slug_to_uid)
    |        returns uid string
    |
    +-- 3. KV cache write: project-uid.{slug} -> uid
            return uid
```

### Registration

`NewProjectResolver` in `internal/infrastructure/project/resolver.go` wires
together `*nats.ProjectRPC`, `*salesforce.ProjectRepo`, and `*nats.Storage`.
The resolver is constructed in `cmd/member-api/service/providers.go` and passed
to `salesforce.NewMemberReader`.
