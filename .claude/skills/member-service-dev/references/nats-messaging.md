<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# NATS Messaging (member-service)

Repo-local NATS subjects, RPC payload shapes, and KV bucket inventory.
General NATS/KV coding conventions live inline in the parent SKILL.md.
Platform NATS/KV ownership and handoffs live in
`lfx-skills:lfx-platform-architecture`.

## Inbound RPC handled by this service

Subjects are defined in `pkg/constants/nats.go`. All three are plain
`Subscribe` request/reply handlers, drained on shutdown.

```go
ProjectIDMapLookupSubject = "lfx.member.project-id-map.lookup"
SFIDToUUIDLookupSubject   = "lfx.member.sfid-to-uuid.lookup"
UUIDToSFIDLookupSubject   = "lfx.member.uuid-to-sfid.lookup"
```

### Project ID map lookup

Maps a v2 project UID to a Salesforce `Project__c.Id` for caller services
that need to correlate by SFID. Implemented in
`internal/infrastructure/nats/project_id_map_handler.go`. Resolution chains
KV cache → project-service NATS RPC → Salesforce SOQL.

| Field | Value |
| --- | --- |
| Subject | `lfx.member.project-id-map.lookup` |
| Transport | NATS core request/reply |
| Subscription | Plain `Subscribe`; drained on shutdown |

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

### Generic SFID ↔ UUID lookup

Two entity-agnostic translators implemented in
`internal/infrastructure/nats/id_map_handler.go`. Pure CPU — no Salesforce
call, no KV — using the invertible `pkg/sfuuid` UUID v8 encoding. 15-char
SFIDs are normalised to 18 internally; both forms are accepted.

| Subject | Request | Success response |
| --- | --- | --- |
| `lfx.member.sfid-to-uuid.lookup` | `{"sfid":"<15/18-char SFID>"}` | `{"uuid":"<uuid v8>"}` |
| `lfx.member.uuid-to-sfid.lookup` | `{"uuid":"<uuid v8>"}` | `{"sfid":"<15-char SFID>"}` |

All replies are always valid JSON. Callers must check for the `"error"` key
to detect failure.

## Outbound RPC this service makes

```go
projectGetSlugSubject   = "lfx.projects-api.get_slug"
projectSlugToUIDSubject = "lfx.projects-api.slug_to_uid"
```

Both are owned by `lfx-v2-project-service`. The member service consumes
them only inside `ProjectResolver` to translate UID, slug, and SFID.

## KV buckets

All three buckets are initialized by `internal/infrastructure/nats/client.go`
and created by the chart when enabled. Names live in
`pkg/constants/storage.go`.

| Bucket | Constant | Purpose |
| --- | --- | --- |
| `membership-cache` | `KVBucketNameCache` | SOQL-backed soft-TTL cache for tiers, memberships, key contacts, membership list batches, and resolver outputs. Uses `CachedValue[T]`, `CacheStatus`, and `TTLConfig`. 24 h bucket MaxAge. |
| `member-service-cache` | `KVBucketNameSObjectCache` | Salesforce sObject conditional-GET cache keyed by `{sobject_type}.{uid}`. Uses `SObjectCacheEntry`, not `CachedValue[T]`. Freshness via HTTP `ETag`/`Last-Modified`; 7-day bucket MaxAge backstop. |
| `org-settings` | `KVBucketNameOrgSettings` | Authoritative b2b_org access-control state (writers, auditors, pending invites), keyed `org-settings.{uid}` → raw `model.B2BOrgSettings` JSON. No soft-TTL envelopes and no MaxAge eviction; optimistic concurrency via KV revision (compare-and-set) on every PUT. |

Key layout, TTLs, freshness states, and resolver chains are documented in
`docs/agent-guidance/salesforce-cache.md`.

## Indexer and FGA-sync publishing

This service publishes to the indexer and FGA-sync on the write path. Subjects
are in `pkg/constants/subjects.go`:

| Direction | Subjects |
| --- | --- |
| Indexer | `lfx.index.b2b_org`, `lfx.index.b2b_org_settings`, `lfx.index.project_membership`, `lfx.index.key_contact` |
| FGA-sync | `lfx.fga-sync.update_access`, `lfx.fga-sync.delete_access` |

Publishing goes through `port.MemberPublisher`
(`internal/infrastructure/nats/publisher.go`). Creates/updates are
fire-and-forget and swallow publish errors (logged with
`publish_failed_for_backfill_repair=true`, recoverable via
`POST /admin/reindex`); deletes propagate publish errors. On a settings PUT,
FGA-sync is published before the indexer. Message shapes are owned by the
local contracts `docs/fga-contract.md` and `docs/indexer-contract.md`
(upstream: `lfx-v2-fga-sync` and `lfx-v2-indexer-service`); keep them in sync
with builder changes.
