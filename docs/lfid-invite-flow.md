# LFID Invite Flow — Project Service

This document describes how the project service handles users who are added to a project's settings roles (Writers, Auditors, Meeting Coordinators) but do not yet have an LF ID (LFID) account.

---

## Overview

When a user is added to a project's role list via `PUT /projects/{uid}/settings`, the service branches on whether the user has an LFID:

| User state | `username` field | Action |
|---|---|---|
| Has LFID | non-empty | Send a direct role-notification email via the email service |
| No LFID | empty | Send an invite request to the invite service; no invite state is stored (acceptance is reconciled by recipient email) |

The invite service handles rendering and delivering the invite email to the recipient. The project service does not store any invite state: when the invite is later accepted, the invite service publishes an enriched acceptance event that carries the recipient email, and the project service reconciles by email.

---

## Sending an Invite

**Triggered by:** `HandleProjectSettingsUpdated` — called when a `lfx.projects-api.project_settings.updated` event arrives and the diff contains non-LFID users who gained roles (newly added users, or new roles on a role change; removals are silently skipped). Invites are deduplicated by mapped invite role, so a user gaining both Writer and Meeting Coordinator receives a single `Manage` invite.

**NATS subject used:** `lfx.invite-service.send_invite` (request/reply)

**LFID pre-check:** Before building the invite request, the dispatcher calls `UsernameByEmail` on the auth service to determine whether the recipient already has an LFX account. If a username is found, `recipient_has_account` is set to `true` and the invite service renders a simpler "Accept invitation" email; otherwise it renders the full "Accept invitation & create account" email. The lookup is best-effort: unexpected errors (excluding `ErrUserNotFound`) are logged at WARN level and the field falls back to `false` so invites are never blocked.

**Request payload** (`inviteapi.SendInviteRequest`, structured fields):

| Field | Value |
|---|---|
| `recipient.email` | User's email address |
| `recipient.name` | User's display name (falls back to username, then email) |
| `inviter.name` | Actor's display name (resolved via auth-service); falls back to `"A project administrator"` |
| `resource.uid` | Project UID |
| `resource.name` | Project name |
| `resource.type` | `"project"` |
| `role` | `"Manage"` (Writers / Meeting Coordinators) or `"View"` (Auditors) |
| `return_url` | Deep link to the project page |
| `expiration_days` | `30` |
| `recipient_has_account` | `true` if the recipient's email resolves to a known LFID; `false` otherwise (see LFID pre-check above) |

**On success**, the invite service returns an invite UID, the delivery email, and an expiry timestamp. The project service only logs the invite UID — it does not write invite metadata into the settings record or any lookup key. (Legacy settings records may still carry a read-only `invite` object on user entries from the earlier design; it survives PUT round-trips and is cleared on promotion.)

The send is best-effort: a failure is logged with `slog.WarnContext` and does not block further processing.

---

## Invite Acceptance

**Triggered by:** a `lfx.invite-service.invite_accepted` event (`inviteapi.InviteServiceAcceptedSubject`) published by the invite service after it has processed an acceptance. The event (`inviteapi.InviteServiceAcceptedEvent`) embeds the full invite, so subscribers get enriched context without a separate lookup.

**NATS subscription:** queue-subscribed in `cmd/project-api/main.go` under the `ProjectsAPIQueue` consumer group.

**Message payload** (relevant fields of the embedded invite):

```json
{
  "uid":         "<invite UID>",
  "recipient":   { "email": "<recipient email>", "name": "..." },
  "role":        "Manage | View",
  "accepted_by": "<new LFID username>"
}
```

**Handler:** `(*ProjectsService).HandleInviteAccepted`

**Processing steps:**

1. Unmarshal and guard: discard (log + return `nil`) unless `uid`, `accepted_by`, a non-empty normalized `recipient.email`, and a recognized `role` (`Manage` or `View`) are all present.
2. List **all** project settings (`ListAllProjectsSettings`; `lookup/` keys are skipped).
3. For each project whose role-appropriate slices (`Manage` → Writers + Meeting Coordinators, `View` → Auditors) contain an email-only entry (`username == ""`) matching the normalized recipient email, promote that project via `promoteInvitedUserInProjectSettings`:
   - Re-read settings with revision (optimistic concurrency).
   - Set `username = accepted_by` and clear any legacy `invite` field on every matching email-only entry.
   - Write back with the loaded revision; retry up to 3 times on `ErrRevisionMismatch`.
   - Re-index the project settings so the promoted user appears as an LFID user, and publish `lfx.fga-sync.update_access` so OpenFGA receives the new grant. `publishInvitePromotionSideEffects` reloads settings and retries indexer/FGA publishes (same `scrubMaxRetries` budget as username scrub). The repository write alone does not emit FGA or `project_settings.updated`.

Accepting a single invite intentionally reconciles **every** project where the same email has a pending email-only entry for the same role, not only the project that issued the invite. The operation is idempotent: entries already promoted are skipped.

> A full-scan of all project settings runs on each acceptance event. Replacing it with an email → `[project_uid]` index lookup is a known TODO in `HandleInviteAccepted`.

---

## Timeout and Retry Behavior

- Invite-service request/reply and auth-service actor lookups run under `notificationTimeout` (5 seconds), scoped per operation.
- `HandleInviteAccepted` uses `settingsScanTimeout` (2 minutes) for both `ListAllProjectsSettings` and each per-project promotion, because a full settings-bucket scan is not a single RPC.
- `promoteInvitedUserInProjectSettings` retries up to 3 times on `ErrRevisionMismatch` within its project's window. This handles concurrent writers racing on the same KV revision.
- `publishInvitePromotionSideEffects` retries indexer and FGA publishes up to `scrubMaxRetries` times, reloading settings on each attempt, so a transient NATS or project-base read failure does not permanently omit the new grant.
- If a promotion fails (timeout or exhausted retries), the email-only entry remains pending until another acceptance event for the same email/role arrives or the settings are corrected manually.
- Errors from individual sends are logged but never propagated — the handler is entirely best-effort and always returns `nil`.

---

## Notification Suppression on Promotion

Invite promotion writes settings through the repository, not the HTTP `UpdateProjectSettings` operation, so it does **not** publish `lfx.projects-api.project_settings.updated`. Role-change emails and further invites are therefore not retriggered. FGA and indexer side effects are published directly by `publishInvitePromotionSideEffects`.
