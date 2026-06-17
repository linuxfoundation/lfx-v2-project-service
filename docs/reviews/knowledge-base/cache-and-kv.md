<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# NATS KV cache and bucket behavior

Patterns specific to this service's KV bucket model (`membership-cache` soft-TTL
envelope cache; `member-service-cache` sObject conditional-GET cache; `org-settings`
authoritative access-control bucket; `pubsub-state` CDC replay-cursor bucket). Recurring flags: serving stale entries because the
TTL is only checked by a background sweep, write paths that don't invalidate the relevant
cache entry, KV key prefix comment/code drift (dot vs slash), and reliance on the
unspecified NATS KV iteration order for pagination. Stale-serve and missing-invalidation
are Critical (serve wrong/expired data); the rest are Important.

**Read when:** any file under `internal/infrastructure/nats/**`, `pkg/constants/storage.go`,
`pkg/constants/nats.go`, or any handler in `cmd/member-api/service/**` /
`internal/service/**` that performs a key-contact / b2b-org / settings write. Also read
`docs/agent-guidance/salesforce-cache.md`. Cross-checked in Steps 3-4 of the
learnings-review playbook.

---

## `cache-and-kv/ttl-not-enforced-on-hit` — Critical

**Pattern:** a cache lookup returns the cached value without comparing the entry's
`expiresAt` / soft-TTL state on the read path; expiry is only enforced by a periodic
background sweep. This serves stale mappings for up to TTL + sweep-interval.

**Detect:** in `internal/infrastructure/nats/**`, for each cache-read helper, verify the
read path consults `CacheStatus` / `expires_at` (`CacheStatusFresh|Stale|Expired|Miss`)
and only returns `Fresh`/`Stale` (with refresh) entries. Flag a `return cached` that is
not gated by the freshness check, relying on an `evictExpired()` sweep alone.

**Empirical citation:** PR #10 `internal/infrastructure/project/resolver.go:80` (then at `internal/infrastructure/nats/project_resolver.go`) — Copilot — "Cache TTL isn't actually enforced on cache hits: ResolveB2BSFID returns the cached entry without checking entry.expiresAt, and evictExpired() only runs a full sweep once per projectResolverCacheCleanup. This can serve stale mappings for up to ~TTL+cleanup interval." (Maintainer accepted the residual risk for static SFID data but confirmed the observation is valid.)

**Failure message:** Cache TTL checked only by a background sweep, not on the read path — stale entries served for up to TTL + sweep interval.

**Fix:** check `CacheStatus` / `expires_at` on every read. Serve `Fresh` directly,
serve `Stale` while triggering a background refresh, and treat `Expired`/`Miss` as a
synchronous fetch — per the freshness-state table in `docs/agent-guidance/salesforce-cache.md`.

---

## `cache-and-kv/write-without-cache-invalidation` — Critical

**Pattern:** a key-contact / membership / b2b-org write path mutates Salesforce but does
not invalidate (or carry the key needed to invalidate) the corresponding
`membership-cache` entry, so reads keep serving the pre-write value until the soft TTL
(6h stale / 23h expire) lapses. A common shape is an update input struct missing the
`MembershipUID`, so `invalidateKeyContactsCache` is skipped.

**Detect:** for each write handler (`Create*`/`Update*`/`Delete*` on key-contacts,
memberships, b2b-orgs), confirm the input passed to the writer carries the UID(s) needed
to invalidate the relevant `key-contacts.{membership_uid}` / `membership.{uid}` /
`tier.{uid}` cache key, and that an invalidation actually runs after a successful write.

**Empirical citation:** PR #39 `cmd/member-api/service/membership_service.go:500` — Copilot — "The update input does not carry `p.MembershipUID`, so `KeyContactWriter.UpdateKeyContact` receives an empty membership UID and skips `invalidateKeyContactsCache`. After an update, the membership-level key-contact cache can remain stale until TTL expiry." Acted on: prabodhcs — "added `MembershipUID: p.MembershipUID` to the `KeyContactInput` struct so `KeyContactWriter.UpdateKeyContact` always invalidates the key-contacts cache entry after an update."

**Failure message:** Write path does not invalidate the affected membership-cache entry — reads serve the stale pre-write value until the soft TTL lapses.

**Fix:** thread the membership/org UID into the writer input and invalidate the matching
`membership-cache` key on successful write, matching the established `UpdateKeyContact`
pattern.

---

## `cache-and-kv/kv-key-prefix-comment-drift` — Important

**Pattern:** a doc comment, ARCHITECTURE.md table, or constant comment describes a NATS
KV key prefix with the wrong delimiter (slash `key-contacts/` instead of the actual dot
`key-contacts.`) or the wrong type/shape, diverging from the `keyPrefix*` constants in
`internal/infrastructure/nats/storage.go`. KV key naming is load-bearing — drift misleads
the next maintainer about the real key layout.

**Detect:** when a diff touches a `keyPrefix*` constant, a cache key builder, or a KV-key
comment/table, cross-check the delimiter and shape against the actual constant in
`storage.go` and the table in `docs/agent-guidance/salesforce-cache.md`. Flag slash-vs-dot
mismatches and stale `[]*model.ProjectKeyContact` (now `KeyContact`) type references.

**Empirical citation:** PR #25 `internal/infrastructure/nats/storage.go:103` — Copilot — "This comment says cached contacts are stored under keys prefixed with \"key-contacts/\", but the actual key prefix constant used is \"key-contacts.\" (dot) ... key naming matters". Recurs at PR #13 `ARCHITECTURE.md:77` (slash-vs-dot in the KV key table, maintainer fixed) and PR #21 `member_reader.go:214` (missing optional `search:<term>` segment in the key doc).

**Failure message:** KV key prefix comment/table uses the wrong delimiter or stale type — diverges from the `keyPrefix*` constants in `storage.go`.

**Fix:** make the comment/table match the actual constant exactly (dot-delimited
`prefix.{uid}`), including optional segments like `search:<term>` and the current domain
type names.

---

## `cache-and-kv/nondeterministic-kv-iteration` — Important

**Pattern:** pagination or "top N" selection is built on NATS KV key iteration, whose
order is unspecified. The same request can return different/again-out-of-order results
between calls (offset pagination drift, wrong `[0]`).

**Detect:** in `internal/infrastructure/nats/**`, find iteration over `kv.Keys()` /
batch keys feeding an `offset`/`limit` slice or a `[0]` pick without an explicit sort of
the key/UID slice first.

**Empirical citation:** PR #4 `internal/infrastructure/nats/storage.go:108` — Copilot — "Pagination order is non-deterministic here because NATS KV key iteration order is unspecified. With `offset` pagination this can cause the same request to return different members between calls. Consider sorting `allKeys` ... before slicing."

**Failure message:** Pagination/selection relies on unspecified NATS KV iteration order — same request can return different results across calls.

**Fix:** sort the key/UID slice deterministically before slicing for pagination or
selecting a fixed element.

---

## `cache-and-kv/corrupt-entry-not-deleted-on-miss` — Important

**Pattern:** when a cached value fails to unmarshal, the read treats it as a miss and
logs a warning but leaves the corrupted entry in the bucket, so every subsequent request
re-logs the warning and re-misses until the TTL expires.

**Detect:** in cache `Get`/read helpers, find an unmarshal-failure branch that returns a
miss (`return nil, nil`) without a best-effort `kv.Delete(ctx, key)` of the bad entry.

**Empirical citation:** PR #23 `internal/infrastructure/nats/sobject_cache.go:97` — Copilot — "When unmarshalling a cached value fails, `Get` logs a warning and treats it as a miss but leaves the corrupted entry in the KV bucket. This can cause repeated warnings and repeated cache misses until the TTL expires. Consider deleting the key on unmarshal failure (best-effort)". Acted on: emsearcy — "Added a best-effort `kv.Delete` immediately after the unmarshal warning log ... The `return nil, nil` (cache miss) path is preserved so a single bad entry never blocks the request."

**Failure message:** Corrupt cache entry left in the bucket on unmarshal failure — repeated warnings and misses until TTL expiry.

**Fix:** on unmarshal failure, best-effort `kv.Delete(ctx, key)` (log its own failure at
warn), then return the cache-miss so a single bad entry never blocks the request.
