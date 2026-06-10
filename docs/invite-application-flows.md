# Org Settings — Invite & Notification Flows

This document describes how writers and auditors are added to, updated in, and removed from an organisation's dashboard settings, including the full lifecycle of invites, LFID branch logic, email notifications, resend, and invite acceptance.

---

## Overview

Org settings carry two principal lists — **writers** (administrators) and **auditors** (viewers). Each entry has an invite status that drives downstream behaviour.

| Status | Meaning |
|---|---|
| `pending` | Invite sent; user has not yet created an LFID account |
| `accepted` | User has an LFID and access is live |
| `revoked` | Access was explicitly removed via `DeletePrincipal` |
| `expired` | Invite token passed its TTL without acceptance |

---

## CREATE — AddPrincipal

`POST /b2b_orgs/{uid}/settings/users`

Every add performs an LFID lookup first, then branches on whether the user already has an account.

### Existing-LFID path

The user already has an LFID. No invite is needed.

1. Entry written immediately with `invite_status: accepted`, `username: <sub>`, `accepted_at: now`.
2. **Role-assignment notification email** (v5 template) sent best-effort via the email service after a successful write.
3. Invite service is **not** called.

**Email (v5 template):**

| Field | Writer | Auditor |
|---|---|---|
| Subject | `User Role Assignment as Company Administrator` | `User Role Assignment as viewer` |
| Body role label | `an administrator` | `a viewer` |
| Login link | `LFX_SELF_SERVE_BASE_URL/org` | |
| Transport | NATS request/reply → `lfx.email-service.send_email` (10 s) | |

A transient email failure is logged and swallowed — it never blocks the write.

### No-LFID path

The user has no LFID account yet. An invite is created so the user can register.

1. Entry written with `invite_status: pending`.
2. `sendOrgInvite` fires a NATS request/reply to the invite service (`lfx.invite-service.send_invite`, 10 s timeout). The invite service renders and delivers the magic-link email.
3. Returned `invite_uid` is stored on the entry.
4. A failure from the invite service is logged and swallowed — entry stays `pending` with an empty `invite_uid`.

**Invite request payload** (`inviteapi.SendInviteRequest`):

| Field | Value |
|---|---|
| `recipient.email` | User's email |
| `recipient.name` | Display name (may be empty) |
| `inviter.name` | `"An organization administrator"` |
| `resource.uid` | Org UID |
| `resource.type` | `"b2b_org"` |
| `resource.name` | Org name (fetched via `GetB2BOrg`, best-effort) |
| `role` | `"Manage"` (writer) / `"View"` (auditor) |
| `return_url` | `LFX_SELF_SERVE_BASE_URL/org` (org dashboard deep link) |

---

## RESEND — tryResendInPlace

Triggered by `AddPrincipal` when the email already has a **single live `pending` entry for the same role**.

Instead of returning `409 Conflict`, the service re-sends the invite and refreshes the existing entry in-place. No new record is created.

**Conditions:**
- Exactly one live (non-revoked, non-expired) match.
- That match is `pending`.
- The requested role matches the existing entry's role.

**Effect:**
1. `sendOrgInvite` is called again — invite service re-delivers the magic-link email with a fresh token.
2. The entry's `invite_uuid`, `invited_at`, and `updated_at` are refreshed.
3. No role-assignment notification email is sent (user still has no LFID).

Any other combination — multiple live entries, different role, already accepted — returns `409 Conflict`.

---

## UPDATE — ChangePrincipalRole

`PUT /b2b_orgs/{uid}/settings/users/{email}`

Moves a principal between the writer and auditor lists, preserving the full entry (username, invite status, timestamps) so an accepted grant stays accepted.

**Rules:**
- If the entry is already in the target role with no duplicates, the write is skipped entirely — no revision bump, no FGA/indexer republish.
- If the same email appears in both lists (a bulk-PUT artifact), all copies are collapsed to a single entry in the target list; `mostLivePrincipal` selects which entry's state is carried forward.
- The last accepted writer (admin) cannot be moved to auditor — returns `403 Forbidden`.
- No email is sent on a role change.

---

## DELETE — DeletePrincipal

`DELETE /b2b_orgs/{uid}/settings/users/{email}`

Removes a principal entirely — revokes an accepted grant or cancels a pending invite.

**Rules:**
- Removes all copies of the email from both writers and auditors.
- The last accepted writer (admin) cannot be removed — returns `403 Forbidden`.
- No email is sent on removal.
- FGA sync is triggered only for the relation(s) that actually contained the entry, so removing an auditor never re-syncs the writers list.

---

## Invite Acceptance

When a no-LFID user completes LFID account creation via LFX Self Serve, the invite service publishes an event. The member service subscribes and promotes matching pending entries across all orgs.

**NATS subscription:** `lfx.invite-service.invite_accepted` (queue group `lfx-v2-member-service`)

**Event payload** (`inviteapi.InviteServiceAcceptedEvent`):

| Field | Meaning |
|---|---|
| `accepted_by` | New LFID username (`sub`) |
| `recipient.email` | Email address for matching pending entries |
| `resource.type` | Resource type — must be `"b2b_org"` for org events |
| `resource.uid` | Org UID of the resource the invite was for |
| `role` | `"Manage"` or `"View"` — used as tie-breaker only |

**Handler:** `InviteAcceptedService.Handle` → `tryAcceptInviteInOrg`

**Processing steps:**

1. **Type filter:** if `resource.type != "b2b_org"`, return immediately with no KV access. This drops the flood of committee/project acceptances that arrive on the same subscription.
2. **Scan:** `ListSettingsOrgUIDs` returns all org UIDs. For each org, `tryAcceptInviteInOrg` runs.
3. **Per-org list-authoritative matching:** collect all pending entries with no username whose normalised email matches `recipient.email`.
   - Matches in exactly one list → promote all of them.
   - Matches in both writers **and** auditors → tie-break on `role`: `"Manage"` → writers, `"View"` → auditors; unknown/empty role → skip that org (no over-grant).
4. Patch each selected entry: `username = accepted_by`, `invite_status: accepted`, `accepted_at: now`, `invite_uuid: ""`.
5. Write via `Update` (one call per org, carrying only the changed list(s)) — triggers FGA sync and indexer republish.
6. On `409 Conflict` (concurrent write), retry up to 3 times with a fresh read.

All errors per org are logged and not returned — the handler is best-effort because the NATS core subscription has no ACK/NAK.

---

## Status Transitions

| From | Operation | To |
|---|---|---|
| _(new)_ | `AddPrincipal` — has LFID | `accepted` |
| _(new)_ | `AddPrincipal` — no LFID | `pending` |
| `pending` | Resend (`AddPrincipal`, same role) | `pending` (refreshed) |
| `pending` | `invite_accepted` event | `accepted` |
| `pending` | TTL passes | `expired` |
| `accepted` / `pending` | `DeletePrincipal` | _(entry deleted entirely)_ |
| `expired` | `AddPrincipal` | old entry removed, new entry created from scratch |
| `accepted` / `pending` | `ChangePrincipalRole` | entry moves to target list (status preserved) |

---

## NATS Subjects

| Subject | Direction | Purpose |
|---|---|---|
| `lfx.invite-service.send_invite` | request/reply (10 s) | Create invite and deliver magic-link email (no-LFID path) |
| `lfx.invite-service.invite_accepted` | subscribe (queue group) | Promote pending entries to accepted on LFID account creation |
| `lfx.email-service.send_email` | request/reply (10 s) | Deliver role-assignment notification (existing-LFID path) |

---

## Legacy ACS Mapping

| ACS operation | v2 equivalent | Email sent |
|---|---|---|
| `sendInvite` (no LFID) | `AddPrincipal` → no-LFID branch | Invite service delivers magic-link email |
| `sendInvite` (has LFID) | `AddPrincipal` → existing-LFID branch | v5 role-assignment notification |
| `resendInvite` | `AddPrincipal` → `tryResendInPlace` (pending, same role) | Invite service re-delivers magic-link |
| `deleteInvite` | `DeletePrincipal` | None |
| `sendBulkInvite` / `sendNewEmployees` / `labelEmployeesAdm` | `Update` (bulk PUT) | None (out of scope) |

---

## Scope

This flow applies only to **org settings roles** (Writers / Auditors). It is distinct from any future org-level membership invite API that may manage explicit invite records.
