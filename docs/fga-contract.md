# FGA Contract — Project Service

This document is the authoritative reference for all messages the project service sends to the fga-sync service, which writes and deletes [OpenFGA](https://openfga.dev/) relationship tuples to enforce access control.

The full OpenFGA type definitions (relations, schema) for all object types are defined in the [platform model](https://github.com/linuxfoundation/lfx-v2-helm/blob/main/charts/lfx-platform/templates/openfga/model.yaml).

**Update this document in the same PR as any change to FGA message construction.**

---

## Object Types

- [Project](#project)

---

## Message Format

All messages use the generic FGA message format on the following NATS subjects:

| Subject | Used for |
|---|---|
| `lfx.fga-sync.update_access` | Create and update operations |
| `lfx.fga-sync.delete_access` | Delete operations |

Each message carries `object_type`, `operation`, and a `data` map. The sections below describe the `data` contents for each object type.

### Delivery Semantics

Project create, base update, and settings update publish `lfx.fga-sync.update_access` asynchronously. For those operations, `X-Sync` no longer changes indexer behavior: `CreateProject`, both update methods, and `DeleteProject` always call `SendIndexerMessage` inside an `errgroup` and `g.Wait()` regardless of `X-Sync`, and `SendIndexerMessage` now ignores the sync flag (always `conn.Publish`). `X-Sync` does not wait for FGA processing or OpenFGA convergence.

Project deletion also publishes `lfx.fga-sync.delete_access` asynchronously. `X-Sync` has no effect on project indexer deletion behavior and does not wait for FGA deletion processing or OpenFGA convergence. (For link, folder, and document sub-resources, `X-Sync` still controls whether the publish error is surfaced inline or swallowed in a background goroutine — but the NATS delivery is always fire-and-forget either way.)

---

## Project

**Source structs:** `internal/domain/models/project.go` — `ProjectBase` and `ProjectSettings`

**Synced on:** create, update of project base, update of project settings, delete of a project.

### Access Config

| Field | Value |
|---|---|
| `object_type` | `project` |
| `public` | `ProjectBase.Public` (passed through directly) |

### Relations

| Relation | Value | Condition |
|---|---|---|
| `writer` | Usernames from `ProjectSettings.Writers` | Only when at least one writer has a non-empty username |
| `auditor` | Usernames from `ProjectSettings.Auditors` | Only when at least one auditor has a non-empty username |
| `meeting_coordinator` | Usernames from `ProjectSettings.MeetingCoordinators` | Only when at least one meeting coordinator has a non-empty username |
| `executive_director` | Username from `ProjectSettings.ExecutiveDirector` | Only when `ExecutiveDirector.Username` is non-empty |

> Usernames are the `Username` field of each `UserInfo` entry (LFX usernames). Before persisting a create or settings update, `enrichAllRoleFields` overwrites `Username` on every entry that includes an email address with the value returned by `lfx.auth-service.email_to_username`. Unknown emails (`ErrUserNotFound`) clear the request username so a caller-supplied value cannot become an FGA principal. `convertUsersFromAPI` then **preserves any already-stored LFID** matched by email, so a lookup miss cannot omit the relation key and cause fga-sync to delete existing tuples. Account deletion still scrubs usernames via `HandleUserDeleted`. Entries with a username but no email are left untouched. Empty usernames are omitted from the published relation lists (pending invites are not FGA principals). When a role slice is empty, the relation key is omitted so fga-sync deletes that relation — that is intentional empty-list semantics, not a lookup failure.

### References

| Reference | Value | Condition |
|---|---|---|
| `parent` | `"project:{ParentUID}"` | Only when `ProjectBase.ParentUID` is non-empty |

### Delete

On delete, only `uid` is sent — all FGA tuples for `project:{uid}` are removed by the fga-sync service.

---

## Triggers

| Operation | Object Type | Subject | Notes |
|---|---|---|---|
| Create project | `project` | `lfx.fga-sync.update_access` | Always sent |
| Update project base | `project` | `lfx.fga-sync.update_access` | Always sent |
| Update project settings | `project` | `lfx.fga-sync.update_access` | Always sent |
| Invite acceptance (`HandleInviteAccepted`) | `project` | `lfx.fga-sync.update_access` | After KV promotion of email-only entries to LFID; indexer is also refreshed. `project_settings.updated` is not emitted. |
| Username scrub (`HandleUserDeleted`) | `project` | `lfx.fga-sync.update_access` | After KV username clear; indexer is also refreshed. `project_settings.updated` is not emitted. |
| Delete project | `project` | `lfx.fga-sync.delete_access` | Always sent |
| `project-cli sync reindex-projects --include-access` | `project` | `lfx.fga-sync.update_access` | Manual repair path, opt-in only — see `cmd/project-cli/README.md` |
