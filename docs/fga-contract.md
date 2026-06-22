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
| `marketing_ops` | `"team:{teamUID}#member"` from `ProjectSettings.MarketingOpsTeam.UID` | Only on ROOT project (`ProjectBase.Slug == "ROOT"`) when team UID is non-empty |

> Usernames are the `Username` field of each `UserInfo` entry (LFX usernames). Before publishing, `enrichAllRoleFields` overwrites `Username` on every entry that includes an email address with the value returned by `lfx.auth-service.email_to_username`; unknown emails clear `Username` to an empty string. Entries with a username but no email are left untouched.

> Non-ROOT project FGA updates include `exclude_relations: ["marketing_ops"]` so child-project settings syncs never remove the ROOT marketing ops tuple.

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
| Delete project | `project` | `lfx.fga-sync.delete_access` | Always sent |
