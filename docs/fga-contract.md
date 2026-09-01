# FGA Contract — Project Service

This document is the authoritative reference for all messages the project service sends to the fga-sync service, which writes and deletes [OpenFGA](https://openfga.dev/) relationship tuples to enforce access control.

The full OpenFGA type definitions (relations, schema) for all object types are defined in the [platform model](https://github.com/linuxfoundation/lfx-v2-helm/blob/main/charts/lfx-platform/templates/openfga/model.yaml).

**Update this document in the same PR as any change to FGA message construction.**

---

## Object Types

- [Project](#project)
- [Team](#team)

---

## Message Format

All messages use the generic FGA message format on the following NATS subjects:

| Subject | Used for |
|---|---|
| `lfx.fga-sync.update_access` | Create and update operations |
| `lfx.fga-sync.delete_access` | Delete operations |
| `lfx.fga-sync.member_put` | Add a user to a team |
| `lfx.fga-sync.member_remove` | Remove a user from a team |

Each message carries `object_type`, `operation`, and a `data` map. The sections below describe the `data` contents for each object type.

### Delivery Semantics

Project create, base update, and settings update publish `lfx.fga-sync.update_access` asynchronously. For those operations, `X-Sync` controls indexer synchronization only; it does not wait for FGA processing or OpenFGA convergence.

Project deletion also publishes `lfx.fga-sync.delete_access` asynchronously. `X-Sync` continues to control both project indexer deletion messages, but it does not wait for FGA deletion processing or OpenFGA convergence.

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
| `writer` | Usernames from `ProjectSettings.Writers` | Only when `Writers` is non-empty |
| `auditor` | Usernames from `ProjectSettings.Auditors` | Only when `Auditors` is non-empty |
| `meeting_coordinator` | Usernames from `ProjectSettings.MeetingCoordinators` | Only when `MeetingCoordinators` is non-empty |
| `executive_director` | Username from `ProjectSettings.ExecutiveDirector` | Only when `ExecutiveDirector.Username` is non-empty |

> Usernames are the `Username` field of each `UserInfo` entry (LFX usernames). Before publishing, `enrichAllRoleFields` overwrites `Username` on every entry that includes an email address with the value returned by `lfx.auth-service.email_to_username`; unknown emails clear `Username` to an empty string. Entries with a username but no email are left untouched.

### References

| Reference | Value | Condition |
|---|---|---|
| `parent` | `"project:{ParentUID}"` | Only when `ProjectBase.ParentUID` is non-empty |
| `marketing_ops` | `"team:marketing-ops-{uid}#member"` | Always sent |

> The `marketing_ops` reference grants `team:marketing-ops-{uid}#member` the project's `marketing_ops` relation (see the [FGA model](https://github.com/linuxfoundation/lfx-v2-helm/blob/main/charts/lfx-platform/files/model.fga)), which in turn implies `marketing_auditor` and `campaign_manager`. The team object is per-project (`marketing-ops-<projectUID>`), so membership in one project's team does not grant access to any other project. This reference is emitted unconditionally on every `update_access` publish — an empty team grants nobody, so it is inert until a member is added via [Team](#team) `member_put`.

### Delete

On delete, only `uid` is sent — all FGA tuples for `project:{uid}` are removed by the fga-sync service.

---

## Team

**Source structs:** `internal/service/marketing_ops_operations.go` — `AddMarketingOpsMember` / `RemoveMarketingOpsMember`

**Synced on:** granting or revoking a user's project-scoped Marketing Ops access via `POST /projects/{uid}/marketing-ops-members` and `DELETE /projects/{uid}/marketing-ops-members/{username}`.

Unlike `project`, `team` membership changes are **not** full syncs — `member_put`/`member_remove` add or remove a single tuple and leave the rest of the team's membership untouched.

### Data (`fgatypes.GenericMemberData`)

| Field | Value |
|---|---|
| `uid` | `"marketing-ops-{projectUID}"` |
| `username` | The username/LFID being granted or revoked |
| `relations` | `["member"]` |

### Add

`POST /projects/{uid}/marketing-ops-members` publishes, in order:
1. `lfx.fga-sync.update_access` for `project:{uid}` — the full project access message (see [Project](#project)), rebuilt from the DB, which always includes the `marketing_ops` reference. This backfills the team→project tuple for projects created before this reference existed.
2. `lfx.fga-sync.member_put` for `team:marketing-ops-{uid}` — writes `team:marketing-ops-{uid}#member@user:{username}`.

### Remove

`DELETE /projects/{uid}/marketing-ops-members/{username}` publishes `lfx.fga-sync.member_remove` for `team:marketing-ops-{uid}`, removing `team:marketing-ops-{uid}#member@user:{username}`. The team→project `marketing_ops` reference itself is left in place (it is only ever removed by deleting the project).

---

## Triggers

| Operation | Object Type | Subject | Notes |
|---|---|---|---|
| Create project | `project` | `lfx.fga-sync.update_access` | Always sent |
| Update project base | `project` | `lfx.fga-sync.update_access` | Always sent |
| Update project settings | `project` | `lfx.fga-sync.update_access` | Always sent |
| Delete project | `project` | `lfx.fga-sync.delete_access` | Always sent |
| Add marketing ops member | `project` | `lfx.fga-sync.update_access` | Full project sync, backfills the `marketing_ops` reference |
| Add marketing ops member | `team` | `lfx.fga-sync.member_put` | Adds the user to `team:marketing-ops-{uid}` |
| Remove marketing ops member | `team` | `lfx.fga-sync.member_remove` | Removes the user from `team:marketing-ops-{uid}` |
