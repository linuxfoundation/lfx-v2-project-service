<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# FGA-sync and indexer message construction

Patterns in how this service builds and publishes its FGA-sync (`update_access`) and
indexer messages for b2b-org hierarchy and key-contacts. The service now DOES publish
these (added in the b2b-org settings work, PRs #36-#44) — so the message shape is a live
contract. Recurring flags: concurrent `update_access` messages racing each other because
the base message lacks `exclude_relations`, duplicate `member:` tags when a user is both
writer and auditor, and indexer publishes that happen before the username is resolved.
The relation-clobbering race is Critical (drops just-added FGA tuples); the rest are
Important.

**Read when:** any file under `internal/service/**` or `internal/domain/model/**`
touching FGA / indexer message building (`message_builders.go`, `b2b_org_settings.go`,
`*_writer.go`, `member_message.go`), `pkg/constants/subjects.go`, or
`docs/fga-contract.md`. Cross-checked in Steps 3-4 of the learnings-review playbook.

---

## `fga-and-indexer/update-access-race-without-exclude-relations` — Critical

**Pattern:** a base `update_access` FGA message (no `exclude_relations`) is published
concurrently with a relation-specific message (e.g. `parent`). Because the base message
covers all relations, if it is processed after the specific one it overwrites the
object's access state and removes the just-added relation tuples.

**Detect:** in `internal/service/**` / `internal/domain/model/**`, when more than one
`update_access` / FGA message is published for the same object (especially via an
`errgroup` / concurrent publish), verify the base message sets
`ExcludeRelations: []string{...}` for any relation that a sibling message owns.

**Empirical citation:** PR #39 `cmd/member-api/service/membership_service.go:315` — Copilot — "These parent-tuple updates are published concurrently with the base `b2b_org` `update_access` message above. Because the base message has no `exclude_relations`, if it is processed after the parent-specific message it can replace the object's access state and remove the just-added [parent tuples]". Acted on: prabodhcs — "`buildB2BOrgFGAMessage` now includes `ExcludeRelations: []string{\"parent\"}` so the base `update_access` message never touches the `parent` relation regardless of processing order."

**Failure message:** Concurrent base `update_access` message lacks `exclude_relations` — processing order can clobber just-added relation tuples.

**Fix:** set `ExcludeRelations` on the base `update_access` message for every relation a
concurrently-published sibling message owns, so message processing order cannot remove
those tuples.

---

## `fga-and-indexer/duplicate-member-tag` — Important

**Pattern:** the indexer `Tags()` builder emits a `member:{username}` tag once per loop
(writers loop, auditors loop), so a user who is both an accepted writer and an accepted
auditor produces the `member:` tag twice. Downstream consumers see a duplicate.

**Detect:** in `internal/domain/model/b2b_org_settings.go` `Tags()` (and any tag/relation
builder iterating multiple role slices), verify a dedup set (`emittedMemberTags`) guards
the cross-role `member:` tag so it is emitted at most once, while role-specific tags
(`writers.username:`, `auditors.username:`) may still be emitted per role.

**Empirical citation:** PR #44 `internal/domain/model/b2b_org_settings.go:181` — dealako (maintainer) — "If the same username appears as an accepted entry in both `Writers` and `Auditors` ... the `member:<username>` tag will be emitted twice". Acted on: prabodhcs — "added an `emittedMemberTags` set in `Tags()` so `member:{username}` is emitted exactly once ... Added `TestB2BOrgSettings_Tags_MemberTagDeduplication` asserting count == 1".

**Failure message:** Cross-role `member:` tag emitted more than once — a user in two role slices produces duplicate tags.

**Fix:** track emitted `member:` usernames in a set so the cross-role tag is emitted at
most once; keep role-specific tags per role. Add a dedup test.

---

## `fga-and-indexer/index-before-username-resolved` — Important

**Pattern:** an indexer publish for a key-contact happens before `kc.Username` (the OIDC
sub) is resolved, so the indexed document is missing the resolved username on the
update/email-change path even though the create path includes it.

**Detect:** in `cmd/member-api/service/membership_service.go`, confirm the
`publishKeyContactIndexer` call is ordered AFTER the username-resolution block on both the
email-change and role-only paths. Flag a publish that precedes
`resolveSubForContact`/username assignment.

**Empirical citation:** PR #39 `cmd/member-api/service/membership_service.go:529` — Copilot — "The update indexer event is published before `kc.Username` is populated, so updated key-contact documents (including email-change updates) are indexed without the resolved username even though create indexes it." Acted on: prabodhcs — "moved `publishKeyContactIndexer` to after the username resolution block (both email-change and role-only paths). The indexed document now always carries the resolved sub."

**Failure message:** Indexer publish precedes username resolution — updated key-contact documents are indexed without the resolved sub.

**Fix:** resolve and set the username before calling `publishKeyContactIndexer` on every
update path, so the indexed document always carries the sub.

---

## `fga-and-indexer/global-admin-ref-on-update` — Important

**Pattern:** an FGA message builder always includes the `global_org_admin` team
reference, including on update events, even though that reference is intended for the
create path only (it establishes the admin team for a newly-created org).

**Detect:** in the FGA message builder / `publishB2BOrgEvents`, verify
`globalOrgAdminTeamUID` is passed only when `action == ActionCreated` and is empty on
updates.

**Empirical citation:** PR #36 `cmd/member-api/service/membership_service.go:222` — Copilot — "publishB2BOrgEvents always passes s.globalOrgAdminTeamUID into buildB2BOrgFGAMessage, which means the global_org_admin reference will also be included on updates ... The builder comment indicates the global admin reference is intended for create only". Acted on: prabodhcs — "`globalOrgAdminTeamUID` is now passed only when `action == ActionCreated`; update events receive an empty string."

**Failure message:** `global_org_admin` reference included on update FGA events — it is intended for the create path only.

**Fix:** pass `globalOrgAdminTeamUID` only when `action == ActionCreated`; pass empty
string on updates.

---

## `fga-and-indexer/unused-subject-constant` — Important

**Pattern:** a NATS subject constant is added but never referenced because the indexing
strategy uses tags on an existing doc rather than a separate per-record subject. Dead
subject constants imply an unbuilt publish path and mislead readers about what the service
emits.

**Detect:** when a new subject constant is added in `pkg/constants/subjects.go`, grep the
tree for references. Flag a subject constant with zero call-site references.

**Empirical citation:** PR #44 `pkg/constants/subjects.go:15` — Copilot — "`IndexB2BOrgMemberSubject` is added but does not appear to be referenced anywhere in the codebase ... the actual implementation publishes per-user info as `writers.username:*` / `auditors.username:*`". Acted on: prabodhcs — "removed `IndexB2BOrgMemberSubject` constant; per-user indexing is handled via `member:{username}` tags on the org-level settings doc, no separate subject needed."

**Failure message:** New NATS subject constant has no call sites — dead constant implies an unbuilt publish path.

**Fix:** remove the unused subject constant, or wire the publish path that uses it. Keep
`pkg/constants/subjects.go` to subjects the service actually emits/consumes.
