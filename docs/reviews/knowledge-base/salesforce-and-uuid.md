<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Salesforce, SOQL, and UID/SFID conversion

Patterns where the v2-UUID world and the Salesforce `Project__c.Id`/Account/Contact
SFID world cross over incorrectly: SFID/UUID conversion errors silently swallowed,
synthetic UUIDs minted for empty SFIDs, SOQL string composition double-escaping or
allowing injection, and project-scoped SOQL that bypasses `ProjectResolver`. This is
the most-flagged repo-specific surface (human maintainers + Copilot, recurring across
PRs #10, #20, #21, #23, #36, #37, #40). Conversion-error swallowing is Critical
(silently drops a foreign key); the rest are Important.

**Read when:** any file under `internal/infrastructure/salesforce/**`, `pkg/sfuuid/**`,
`internal/infrastructure/project/**`, or any `.go` calling `sfuuid.ToSFID` /
`sfuuid.ToUUID`, building SOQL strings, or resolving project UID↔SFID. Cross-checked
in Steps 3-4 of the learnings-review playbook (KB-match gate in Step 3, false-positive
filter in Step 4).

---

## `salesforce-and-uuid/swallowed-sfid-conversion-error` — Critical

**Pattern:** a call to `sfuuid.ToUUID(...)` or `sfuuid.ToSFID(...)` discards the error
(`x, _ := sfuuid.To...`) or only acts on the value, so a conversion failure silently
zeroes out a foreign key (membership UID, account SFID, asset/Product2 reference)
instead of propagating. Downstream consumers then see an empty key.

**Detect:** grep for `sfuuid.ToUUID` / `sfuuid.ToSFID` in `internal/**` and
`cmd/**`; flag any call site that assigns to `_` for the error, or maps the model
field without checking and propagating the conversion error when the raw input was
non-empty.

**Empirical citation:** PR #23 `internal/infrastructure/salesforce/sobject_readers.go:321` — Copilot — "`sobjectProjectRoleToModel` ignores the error from `sfuuid.ToUUID(raw.AssetID)`. Since `Asset__c` is the foreign key to the membership Asset, a conversion failure will silently zero out `MembershipUID`". Same PR flagged `sobject_readers.go:217` (Product2Id). Recurs at PR #37 `internal/infrastructure/salesforce/writer.go:134` ("AccountSFID conversion ignores errors ... will cause new-Contact inserts to lose the AccountId") and PR #40 `account_repo.go:78`.

**Failure message:** SFID/UUID conversion error swallowed — a failure silently zeroes a foreign key (membership/account/asset reference) instead of surfacing.

**Fix:** propagate the conversion error when the raw input was non-empty (return
`(*model.X, error)` and pass the error through), or apply the established fallback used
for `MembershipUID` — on conversion failure fall back to the raw input value rather
than `""`. Add a `slog.WarnContext(ctx, ..., "error", convErr)` at minimum so the skip
is visible in logs.

---

## `salesforce-and-uuid/synthetic-uuid-for-empty-sfid` — Important

**Pattern:** a response builder generates a deterministic v8 UUID from a Salesforce
SFID even when the source SFID is empty, emitting a non-empty synthetic ID for data
that was actually missing. "Missing must stay missing" — an empty SFID must map to an
empty UID.

**Detect:** in response/model conversion functions (e.g.
`membership_service_response.go`, `*_to_model` / `*ToResponse` helpers), find
`sfuuid.ToUUID`/`ToSFID` calls that are not guarded by an `if raw == "" { return "" }`
(or `emptyString`) check.

**Empirical citation:** PR #10 `cmd/member-api/service/membership_service_response.go:37` — Copilot — "memberUID/membershipTierUID always generate a deterministic UUID even when the input SFID is empty ... this can end up emitting a non-empty synthetic ID for missing data". Maintainer endorsed: emsearcy — "Both `memberUID` and `membershipTierUID` now return `emptyString` when the input SFID is empty, preserving \"missing stays missing\" semantics."

**Failure message:** Synthetic UUID minted from an empty SFID — missing data is emitted as a non-empty fake ID.

**Fix:** guard the conversion: return the empty-string sentinel when the input SFID is
empty, only minting a UUID for a present SFID.

---

## `salesforce-and-uuid/project-scoped-soql-bypasses-resolver` — Critical

**Pattern:** a project-scoped SOQL query is built with a v2 project UUID placed
directly into the `WHERE` clause (against `Project__c.Id` or a related lookup), instead
of first resolving the UUID to a Salesforce `Project__c.Id` via `ProjectResolver`.
Salesforce stores SFIDs, so the query silently returns zero rows.

**Detect:** in `internal/infrastructure/salesforce/**`, find SOQL strings whose
`WHERE` binds a value that originated from a `project_id`/`project_uid` path parameter
without an intervening `resolver.SFIDFromUID` / `resolveProjectFilterID` call.

**Empirical citation:** matches the documented contract in `docs/agent-guidance/salesforce-cache.md` — "Every project-scoped SOQL query requires a Salesforce `Project__c.Id` in its `WHERE` clause ... Without `ProjectResolver`, all list endpoints would silently return zero results." Reinforced by the B2B-org resolver work at PR #26 `internal/infrastructure/salesforce/member_reader.go:597` (emsearcy: per-record `UIDFromSlug` resolution for B2B pages).

**Failure message:** Project-scoped SOQL uses a raw v2 UUID in WHERE — Salesforce stores SFIDs, so the query returns zero rows.

**Fix:** resolve the UUID to a Salesforce `Project__c.Id` through `ProjectResolver`
(`SFIDFromUID`) before composing the SOQL, mirroring `resolveProjectFilterID`.

---

## `salesforce-and-uuid/soql-like-double-escape-or-injection` — Important

**Pattern:** a SOQL `LIKE` term is escaped twice (e.g. `escapeLikeSOQL` then
`quoteSOQL`, both escaping `'` and `\`), corrupting search terms that contain an
apostrophe or backslash — or, conversely, a value is interpolated into SOQL without
quoting/escaping at all.

**Detect:** in `internal/infrastructure/salesforce/soql.go`,
`membership_repo.go`, and any SOQL builder, trace each user-supplied term through the
escape/quote helpers and check it is escaped exactly once. Flag `quoteSOQL("%" + escapeLikeSOQL(term) + "%")`-style chains where both helpers escape the same characters.

**Empirical citation:** PR #21 `internal/infrastructure/salesforce/soql.go:42` — Copilot — "`escapeLikeSOQL` escapes single quotes to `\'`, but the result is then passed to `quoteSOQL` at the call site. Since `quoteSOQL` also escapes `'` and `\`, terms containing an apostrophe (e.g. \"Bob's\") will end up with an extra literal backslash". Same PR flagged `membership_repo.go:237`.

**Failure message:** SOQL LIKE term escaped twice (or not escaped) — apostrophe/backslash terms produce a wrong pattern or allow injection.

**Fix:** escape each term exactly once. Reserve `escapeLikeSOQL` for the `%`/`_`
wildcard semantics and let `quoteSOQL` own the `'`/`\` escaping; do not stack both for
the same characters. Never interpolate an un-quoted user value into SOQL.

---

## `salesforce-and-uuid/sf-error-text-string-matching` — Important

**Pattern:** Salesforce error handling (e.g. duplicate-detection self-heal) matches on
`strings.Contains(err.Error(), "...")` of the go-salesforce/v3 error text. The library
does not guarantee its error-string format as public API, so a library upgrade can
silently disable the self-heal path and turn idempotent inserts into 500s.

**Detect:** grep for `strings.Contains` / `strings.HasPrefix` on a `.Error()` string in
`internal/infrastructure/salesforce/**`, especially around `DUPLICATE_VALUE` /
self-heal logic in `writer.go`.

**Empirical citation:** PR #37 `internal/infrastructure/salesforce/writer.go:143` — dealako (maintainer, `[minor]`) — "`go-salesforce/v3` surfaces Salesforce error codes via `Error()` text, but the wrapping format is not part of the library's public API contract. A future library upgrade ... could silently disable this self-heal path". Acted on: prabodhcs — "replaced `strings.Contains` with `isDuplicateSFError` which parses the raw SF JSON body for the `errorCode` field. Added a 6-case unit test pinning the go-salesforce v3 error format so a library change fails the test loudly".

**Failure message:** Salesforce error matched by `.Error()` substring — a go-salesforce upgrade can silently break the self-heal/idempotency path.

**Fix:** parse the structured `errorCode` from the Salesforce JSON error body (see
`isDuplicateSFError`) instead of substring-matching the rendered string, and pin the
format with a unit test that fails loudly on a library change.
