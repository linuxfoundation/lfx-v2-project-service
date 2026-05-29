<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Doc, comment, and contract drift

The single highest-recurrence surface on this repo: code comments, doc-comments,
`CLAUDE.md`/`README.md`/`ARCHITECTURE.md` tables, and the owned `docs/fga-contract.md`
that describe behavior the code no longer does. Flagged by all three reviewer buckets
(emsearcy, prabodhcs, Copilot) on PRs #10, #12, #13, #14, #18, #20, #25, #26, #34, #35,
#36, #40, #41, #42, #43, #44. Individually each is a Nit, but the *category* clears the
bar by recurrence and by the contract-violation cost when the drifting doc is an owned
contract (`docs/fga-contract.md`, the NATS/cache docs, the CLAUDE.md endpoint table).
Contract-doc drift is Important; pure code-comment drift is a Nit (surfaced only when it
recurs in the same diff).

**Read when:** any diff that touches a `.go` doc-comment, `CLAUDE.md`, `README.md`,
`ARCHITECTURE.md`, or `docs/**`, especially alongside a behavior change in the same PR.
Cross-checked in Steps 3-4 of the learnings-review playbook.

---

## `docs-and-comments-drift/contract-doc-vs-code` — Important

**Pattern:** an owned contract doc states a shape the code does not emit. Seen with
`docs/fga-contract.md` type-prefixing relation values that the message builders emit raw,
the CLAUDE.md/README endpoint or OpenFGA-check table listing removed/renamed routes, and
the bucket-count / domain-type-name in CLAUDE.md/README lagging the code.

**Detect:** when a diff changes message-builder output, a route, an OpenFGA check, a KV
bucket, or a domain type, cross-check the matching row in `docs/fga-contract.md`,
`CLAUDE.md`, `README.md`, and `ARCHITECTURE.md`. Flag a contract/table row that no longer
matches the code.

**Empirical citation:** PR #42 `docs/fga-contract.md:52` — Copilot — "This table documents `writer`/`auditor` values as `\"user:{username}\"`, but the message builders/tests use raw usernames/subs ... only object references are type-prefixed (e.g., `team:` / `b2b_org:`)." Acted on: prabodhcs — "the relation value column now reads raw username/OIDC sub string ... The `user:` prefix was incorrect." Recurs PR #40 `CLAUDE.md:151` (OpenFGA check for `POST /b2b_orgs` was wrong — `member on team:{globalOrgAdminTeamUID}`, not `writer on b2b_org:{uid}`) and PR #43 `README.md:51` / `CLAUDE.md:285` (bucket count 2→3; `model.OrgSettings`→`model.B2BOrgSettings`).

**Failure message:** Owned contract doc / endpoint table disagrees with the code it describes — consumers will build against the wrong shape.

**Fix:** update the contract doc / CLAUDE.md / README / ARCHITECTURE row to match the
code exactly in the same PR. For endpoint and OpenFGA-check tables, copy the authority
from the Goa design and the deployed `ruleset.yaml`. For generated examples, fix the
design and run `make apigen`.

---

## `docs-and-comments-drift/doc-comment-vs-behavior` — Nit

**Pattern:** a Go doc-comment describes behavior the function no longer has: a "no-op"
mock that actually returns `NotImplemented`, an "unconditional put" comment over a
`kv.Create` exclusive-create, a "plain-text fallback" comment over a JSON error response,
a stale domain type name (`ProjectKeyContact` after rename to `KeyContact`,
`OrgSettings` vs `B2BOrgSettings`), or a comment claiming a field/check that was removed.

**Detect:** for each changed function/struct, read its doc-comment and confirm it matches
the current implementation: mock return values, KV write semantics
(`kv.Create`=exclusive vs `kv.Put`=unconditional), reply encoding, domain type names, and
field-level claims (e.g. "Full-replace on PUT" vs nil-keep / explicit-empty-clear).

**Empirical citation:** PR #35 `internal/infrastructure/mock/membership.go:352` — Copilot — "The comment describes MockKeyContactWriter as a \"no-op\" implementation, but the methods return NotImplemented errors." Acted on: prabodhcs — "updated the doc comment to say 'stub that returns NotImplemented for all writes'." Recurs PR #42 `b2b_org_settings_writer.go:19` (unconditional-put vs `kv.Create` exclusive), PR #43 `project_id_map_handler.go:91` ("plain-text fallback" vs JSON), PR #25 `member_reader.go:698` (`ProjectKeyContact`→`KeyContact`), PR #42 `b2b_org_settings.go:69` ("Full-replace on PUT" vs nil-keep/explicit-clear, dealako).

**Failure message:** Doc-comment describes behavior the function no longer has — misleads the next maintainer (mock return, KV write semantics, reply encoding, or renamed type).

**Fix:** correct the doc-comment to match the implementation in the same change — exact
mock behavior, the real KV write semantics (`kv.Create` = exclusive-create / Conflict on
collision), the real reply encoding, and current domain type names.

---

## `docs-and-comments-drift/comment-claims-absent-field-or-check` — Nit

**Pattern:** a comment asserts a struct field, validation, or guard that the code does
not contain — e.g. "includes `IsDeleted` to satisfy go-salesforce" on a struct with no
`IsDeleted` field, or a comment claiming a zero-offset timestamp validation that the code
does not perform.

**Detect:** when a comment references a specific field name or a specific check, confirm
that field/check exists in the adjacent code. Flag a comment that names something absent.

**Empirical citation:** PR #26 `internal/infrastructure/salesforce/models.go:149` — Copilot — "The comment above `soqlAccount` says an `IsDeleted` field is included to satisfy the go-salesforce library, but the struct does not actually define `IsDeleted`." Acted on (emsearcy): "the misleading `IsDeleted` mention has been removed". Recurs PR #42 `internal/service/backfill_request.go:83` (comment claims a zero-offset guard that the code does not implement — drop the comment or implement the check).

**Failure message:** Comment references a field/check that is not present in the code — remove the comment or add the thing it describes.

**Fix:** either remove the inaccurate comment or add the field/validation it describes.
Prefer making the comment match the simplest correct description of what the code actually
does.
