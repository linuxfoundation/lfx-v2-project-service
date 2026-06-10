<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Goa endpoint, design, and write-handler semantics

Patterns where the Goa design (`cmd/member-api/design/**`), the generated artifacts
(`gen/**`), the Heimdall ruleset, and the handler implementation drift apart, plus
write-handler correctness specific to the key-contact / b2b-org / settings endpoints:
`If-Match`/ETag guard parity across handlers, status-flip that bypasses
capacity/uniqueness checks, declared-but-unreachable errors, and enum examples that fail
their own validation. The capacity/uniqueness bypass and missing-If-Match guard are
Critical (data-integrity / lost-update); the rest are Important.

**Read when:** any file under `cmd/member-api/design/**`, `cmd/member-api/service/**`,
`internal/service/**` (validation / writer use-cases), or `gen/**`. When a design file
changes, also read the matching generated OpenAPI/CLI and
`charts/lfx-v2-member-service/templates/ruleset.yaml`. Cross-checked in Steps 3-4 of the
learnings-review playbook.

---

## `endpoint-and-goa/status-flip-bypasses-capacity` — Critical

**Pattern:** a key-contact `PUT` whose early-return guard only checks `roleChanging` and
`emailChanging` lets a status flip (Inactive→Active) reach the writer without re-running
the per-role capacity and (role,email) uniqueness sibling scan. A membership at its
per-role limit can be over-filled, or a duplicate re-activated, by re-activating an
inactive record.

**Detect:** in `internal/service/key_contact_validation.go` (and any future
`normalizeAndValidate*`), confirm the early-return guard that skips the sibling scan
includes a `statusActivating` term: `if !roleChanging && !emailChanging && !statusActivating`.
Flag a guard that omits the status-activation case.

**Empirical citation:** PR #37 `cmd/member-api/service/key_contact_validation.go:98` — dealako (maintainer, `[blocking]` B2) — "a `PUT` with `{\"status\": \"Active\"}` on an Inactive record reaches the writer with `roleChanging=false` and `emailChanging=false` — this early return fires and the sibling scan is skipped entirely. **Capacity bypass** ... **Duplicate bypass**". Acted on: prabodhcs — "added `statusActivating` flag; early return now requires `!roleChanging && !emailChanging && !statusActivating`. Capacity check fires on `roleChanging || statusActivating`."

**Failure message:** Status-flip (Inactive→Active) bypasses the capacity/uniqueness sibling scan — over-capacity and duplicate key-contacts can be created.

**Fix:** include a `statusActivating` term in the skip-guard so a re-activation re-runs
the capacity and uniqueness checks. Compare status with `strings.EqualFold` (see
`endpoint-and-goa/case-sensitive-status-compare`).

---

## `endpoint-and-goa/missing-ifmatch-guard-parity` — Critical

**Pattern:** the design declares `IfMatchAttribute()` + a `PreconditionFailed` response
(so the generated payload carries `IfMatch *string`), but one mutating handler omits the
ETag comparison that its sibling handlers perform. A stale-`If-Match` write then proceeds
instead of returning 412 — a lost-update hazard. Conversely, an unconditional update path
that always sets `IfUnmodifiedSince` turns every write into a conditional Salesforce PATCH
and can 412 on cache staleness.

**Detect:** for every mutating handler that the design gives an `IfMatch` field, confirm
it runs the `if p.IfMatch != nil && *p.IfMatch != "" { compute LFXEtag(current); compare; 412 }`
guard before writing — and that `IfUnmodifiedSince` is set only when the caller supplied
`If-Match`. Flag a handler with the design field but no guard, or an unconditional
`IfUnmodifiedSince`.

**Empirical citation:** PR #37 `cmd/member-api/service/membership_service.go:503` — dealako (maintainer, `[blocking]` B1) — "The design ... declares `IfMatchAttribute()`, a `PreconditionFailed` error ... But this handler fetches the contact and calls the writer immediately with no ETag comparison". Acted on: prabodhcs — "`DeleteKeyContact` now runs the same `If-Match` → ETag guard as `UpdateKeyContact`. Stale ETag returns 412 before touching the writer." Reinforced PR #42 `internal/service/key_contact_writer.go:202` (Copilot: `IfUnmodifiedSince` set unconditionally; fixed to only-when-`If-Match`-supplied).

**Failure message:** Mutating handler declares an `If-Match` field but skips the ETag guard (or sets `IfUnmodifiedSince` unconditionally) — lost-update or spurious 412.

**Fix:** mirror the `UpdateKeyContact` guard exactly across all mutating handlers that
declare `IfMatch`; only set `IfUnmodifiedSince` on the Salesforce PATCH when the caller
supplied `If-Match`.

---

## `endpoint-and-goa/case-sensitive-status-compare` — Important

**Pattern:** a Salesforce picklist value (`Status__c`) is compared with a byte-for-byte
`==` / `!=` while the same file compares role/email with `strings.EqualFold`. The
picklist label is admin-configurable, so a rename to `"INACTIVE"`/`"inactive"` would
silently miscount.

**Detect:** in `internal/service/key_contact_validation.go` and
`internal/infrastructure/salesforce/key_contact_writer.go` (`writer.go` in the cited
era), grep for `Status` comparisons using `==`/`!=` against `constants.RoleStatus*`;
flag any that are not `strings.EqualFold`.

**Empirical citation:** PR #37 `cmd/member-api/service/key_contact_validation.go:46` — dealako (maintainer, `[minor]` M2) — "`kc.Status != constants.RoleStatusInactive` is a byte-for-byte comparison. SF `Status__c` is an admin-configurable picklist, and the same file already uses `strings.EqualFold` for role/email comparisons". Acted on: prabodhcs — "all three sites now use `strings.EqualFold`".

**Failure message:** Salesforce picklist status compared case-sensitively — an admin picklist relabel would silently miscount active/inactive records.

**Fix:** use `strings.EqualFold` for all `Status__c` comparisons, consistent with the
role/email comparisons in the same file.

---

## `endpoint-and-goa/declared-but-unreachable-error` — Important

**Pattern:** a list/search endpoint's Goa design declares a `NotFound` (404) error and
maps a response, but the handler returns 200 with an empty array when there are no
matches — so the 404 is unreachable and misleading. Or the inverse: the design declares
the error and the handler should enforce it (e.g. verify the parent org exists) but does
not.

**Detect:** for each endpoint, cross-check `dsl.Error(...)` / `dsl.Response(...)`
declarations in `cmd/member-api/design/membership.go` against the handler's actual return
paths. Flag a declared `NotFound` on a list/search endpoint that always returns 200, or a
declared error the handler never produces.

**Empirical citation:** PR #26 `cmd/member-api/design/membership.go:436` — Copilot — "`list-b2b-orgs` is a search/list endpoint and the service implementation returns 200 with an empty `orgs` array ... Defining a NotFound error here is misleading/unreachable". Maintainer agreed (emsearcy: "remove it"). The inverse was also acted on the same PR: `membership_service.go:491` added a `GetB2BOrg` existence check so the declared 404 on `/b2b_orgs/{uid}/memberships` is reachable.

**Failure message:** Goa-declared error is unreachable (or a declared error is never enforced) — design and handler disagree on the response contract.

**Fix:** either remove the unreachable error from the design (and regenerate via
`make apigen`), or add the handler check that makes the declared error reachable.

---

## `endpoint-and-goa/enum-example-fails-validation` — Important

**Pattern:** a Goa attribute enforces an enum (e.g. `role`), but the attribute's
`Example`/description uses a value that is not in the enum. The generated OpenAPI/CLI
examples then fail their own validation when copy-pasted.

**Detect:** when a design attribute declares `dsl.Enum(...)` (or maps to
`constants.KeyContactRoles`), verify every `dsl.Example` / description value for that
attribute is a member of the enum. Re-check the regenerated `gen/http/openapi*.yaml` and
`gen/http/cli/**`.

**Empirical citation:** PR #37 `cmd/member-api/design/type.go:450` — Copilot — "The `role` attribute example uses \"Voting Representative\", but this value is not in `constants.KeyContactRoles` (enum is now enforced). This will generate OpenAPI/CLI examples that fail validation when copy-pasted." Acted on: prabodhcs — "changed all role examples from \"Voting Representative\" (not in enum) to \"Technical Contact\" ... make apigen propagated the fix to all generated OpenAPI/CLI files." (Five distinct generated files were flagged the same way in this one PR.)

**Failure message:** Enum attribute example uses a non-enum value — generated OpenAPI/CLI examples fail their own validation.

**Fix:** change the example/description to a valid enum member and run `make apigen` so
all generated artifacts pick up the corrected example. Never hand-edit the `gen/` files.

---

## `endpoint-and-goa/redundant-publish-on-noop` — Important

**Pattern:** a mutating handler unconditionally publishes an `update`
indexer/FGA event after the writer returns, even when the writer treated the request as a
no-op (empty body, unchanged fields, title-only update). This generates noisy downstream
reindex/FGA work for PATCHes that changed nothing.

**Detect:** in `cmd/member-api/service/membership_service.go`, for each
`Update*`/`publish*Events` pair, check there is a no-op guard (`HasChanges()`,
`etag(current) == etag(after)`, or a both-nil short-circuit) before the publish.

**Empirical citation:** PR #36 `cmd/member-api/service/membership_service.go:195` — Copilot — "UpdateB2bOrg always publishes an \"updated\" indexer/FGA event after calling the writer, even when the request contains no mutable fields". Acted on: prabodhcs — "`UpdateB2bOrg` now short-circuits on `!input.HasChanges()`". Recurs PR #37 `membership_service.go:454` (ETag-based no-op guard added before `publishKeyContactEvents`) and PR #43 `org_settings_writer.go:124` (both-nil short-circuit).

**Failure message:** Update handler publishes an indexer/FGA event on a no-op write — spurious downstream reindex/FGA churn.

**Fix:** short-circuit before publishing when nothing changed — `HasChanges()`, an
ETag comparison of before/after, or a both-nil-input guard, matching the established
`UpdateB2bOrg` pattern.
