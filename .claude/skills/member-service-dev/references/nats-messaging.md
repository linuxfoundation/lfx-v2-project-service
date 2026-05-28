<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# NATS Messaging (member-service)

Repo-local NATS subjects, RPC payload shapes, and KV bucket inventory.
General NATS/KV coding conventions live inline in the parent SKILL.md.
Platform NATS/KV ownership and handoffs live in
`lfx-skills:lfx-platform-architecture`.

## Inbound RPC handled by this service

```go
ProjectIDMapLookupSubject = "lfx.member.project-id-map.lookup"
```

Maps a v2 project UID to a Salesforce `Project__c.Id` for caller services
that need to correlate by SFID. Implemented in
`internal/infrastructure/nats/project_id_map_handler.go`.

| Field | Value |
| --- | --- |
| Subject | `lfx.member.project-id-map.lookup` |
| Transport | NATS core request/reply |
| Subscription | Plain `Subscribe` today; drained on shutdown |

Request body (JSON):

```json
{"project_uid": "<v2 project UUID>"}
```

Success response:

```json
{"project_sfid": "<Salesforce Project__c.Id>"}
```

Error response:

```json
{"error": "<human-readable message>"}
```

The reply is always valid JSON. Callers must check for the `"error"` key to
detect failure.

## Outbound RPC this service makes

```go
projectGetSlugSubject   = "lfx.projects-api.get_slug"
projectSlugToUIDSubject = "lfx.projects-api.slug_to_uid"
```

Both are owned by `lfx-v2-project-service`. The member service consumes
them only inside `ProjectResolver` to translate UID, slug, and SFID.

## KV buckets

Both buckets are initialized by `internal/infrastructure/nats/client.go`
and created by the chart when enabled.

| Bucket | Constant | Purpose |
| --- | --- | --- |
| `membership-cache` | `KVBucketNameCache` | SOQL-backed cache for tiers, memberships, key contacts, B2B org search batches, membership list batches, and resolver outputs. Uses `CachedValue[T]`, `CacheStatus`, and `TTLConfig`. |
| `member-service-cache` | `KVBucketNameSObjectCache` | Salesforce sObject conditional-GET cache keyed by `{sobject_type}.{uid}`. Uses `SObjectCacheEntry`, not `CachedValue[T]`. The bucket and client exist today; current live HTTP providers still use the SOQL-backed readers. |

Key layout, TTLs, freshness states, and resolver chains are documented in
`docs/agent-guidance/salesforce-cache.md`.

## Not an indexer or FGA publisher

This service does not currently publish to `lfx.index.*` or
`lfx.fga-sync.*`. If publishing is added later, follow the canonical
contracts in:

- `lfx-v2-indexer-service/docs/indexer-contract.md`
- `lfx-v2-fga-sync/docs/fga-sync-contract.md`

and add a per-contract doc under `docs/` in this repo.
