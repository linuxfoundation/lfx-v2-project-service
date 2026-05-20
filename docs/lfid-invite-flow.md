# LFID Invite Flow — Project Service

This document describes how the project service handles users who are added to a project's settings roles (Writers, Auditors, Meeting Coordinators) but do not yet have an LF ID (LFID) account.

---

## Overview

When a user is added to a project's role list via `PUT /projects/{uid}/settings`, the service branches on whether the user has an LFID:

| User state | `username` field | Action |
|---|---|---|
| Has LFID | non-empty | Send a direct role-notification email via the email service |
| No LFID | empty | Send an invite request to the invite service; store returned invite metadata |

The invite service handles rendering and delivering the invite email to the recipient, and issues a unique invite UID that the project service uses to track the invite and route the subsequent acceptance event.

---

## Sending an Invite

**Triggered by:** `HandleProjectSettingsUpdated` — called when a `lfx.projects-api.project_settings.updated` event arrives and the diff contains newly added non-LFID users.

**NATS subject used:** `lfx.invite-service.send_invite` (request/reply)

**Request payload** (`inviteapi.SendInviteRequest`):

| Field | Value |
|---|---|
| `recipient_email` | User's email address |
| `recipient_name` | User's display name (falls back to email) |
| `inviter_name` | Actor's display name (resolved via auth-service); falls back to `"A project administrator"` |
| `resource_uid` | Project UID |
| `resource_name` | Project name |
| `resource_type` | `"project"` |
| `role` | `"Manage"` (Writers / Meeting Coordinators) or `"View"` (Auditors) |
| `return_url` | Deep link to the project page |
| `expiration_days` | `30` |

**On success**, the invite service returns an invite UID, the delivery email, and an expiry timestamp. The project service:

1. Writes the invite metadata (`uid`, `email`, `expires_at`) onto the matching user entry in the settings record (stored in the NATS KV store under key `project-settings/{project_uid}`).
2. Writes a secondary mapping entry: KV key `lookup/project-settings-invite/{invite_uid}` → value `{project_uid}`. This is used by `HandleInviteAccepted` to route the acceptance event without scanning all project settings.
3. Re-indexes the project settings so the `invite` object is queryable.

All steps are best-effort: a failure at any step is logged with `slog.WarnContext` and does not block further processing.

---

## Invite Acceptance

**Triggered by:** a `lfx.invite.accepted` event published by the LFX self-serve web app when a user completes LFID account creation and accepts their invite.

**NATS subscription:** queue-subscribed in `cmd/project-api/main.go` under the `ProjectsAPIQueue` consumer group.

**Message payload:**

```json
{
  "invite_uid": "<invite UID>",
  "username":   "<new LFID username>"
}
```

**Handler:** `(*ProjectsService).HandleInviteAccepted`

**Processing steps:**

1. Look up the project UID from the secondary KV mapping using `invite_uid`. If not found, the invite belongs to another service — silently ignored.
2. Load project settings with revision (optimistic concurrency).
3. Scan Writers, Auditors, and MeetingCoordinators for a user whose `invite.uid` matches `invite_uid`.
4. Set `username = <new username>`, clear the `invite` field.
5. Write the updated settings back (using the loaded revision).
6. Delete the secondary mapping entry (`lookup/project-settings-invite/{invite_uid}`).
7. Re-index project settings so the promoted user appears as an LFID user.

The `project_settings.updated` event fired by step 5 goes through `HandleProjectSettingsUpdated` again. The service skips re-sending a notification to users who were previously present as an email-only invited entry (`wasInvitedInOldSettings` check), preventing a duplicate email.

---

## KV Mapping Lifecycle

| Event | KV key | Action |
|---|---|---|
| Invite sent | `lookup/project-settings-invite/{invite_uid}` | Created |
| Invite accepted | `lookup/project-settings-invite/{invite_uid}` | Deleted |

The mapping is written in `storeInviteInfo` and read + deleted in `HandleInviteAccepted`. If the mapping is lost (e.g., service restart between send and accept), `HandleInviteAccepted` will not find the invite and will silently discard the event. The user's settings entry will still carry the pending `invite` object until a future manual correction.

---

## Timeout and Retry Behavior

- All outbound calls (invite service request/reply, KV read/write, indexer publish) run under `notificationTimeout` (5 seconds).
- `storeInviteInfo` retries up to 3 times on `ErrRevisionMismatch`. This handles the case where multiple non-LFID users are added in the same settings update and their concurrent write-backs race on the same KV revision.
- Errors from individual sends are logged but never propagated — the handler is entirely best-effort and always returns `nil`.

---

## Notification Suppression on Promotion

When a user is promoted from non-LFID (email-only) to LFID via invite acceptance, `HandleProjectSettingsUpdated` fires again because `UpdateProjectSettings` publishes a new `project_settings.updated` event. The diff logic sees the user as "new" (identity key changed from email-only to username). The service checks whether the user's email was present in the old settings as an email-only invited entry (`wasInvitedInOldSettings`); if so, the notification is suppressed. The user already received the invite email and does not need a second "you were added" email.
