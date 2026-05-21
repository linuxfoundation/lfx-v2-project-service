# FGA Contract — Member Service

This document is the authoritative reference for all messages the member service sends to the fga-sync service, which writes and deletes [OpenFGA](https://openfga.dev/) relationship tuples to enforce access control.

The full OpenFGA type definitions (relations, schema) for all object types are defined in the [platform model](https://github.com/linuxfoundation/lfx-v2-helm/blob/main/charts/lfx-platform/templates/openfga/model.yaml).

**Update this document in the same PR as any change to FGA message construction.**

---

## Object Types

- [B2B Org](#b2b-org)
- [Project Membership](#project-membership)

---

## Message Format

All messages use the generic FGA message format on the following NATS subjects:

| Subject | Used for |
|---|---|
| `lfx.fga-sync.update_access` | Create and update operations |
| `lfx.fga-sync.delete_access` | Delete operations |
| `lfx.fga-sync.member_put` | Grant a user a relation on an object |
| `lfx.fga-sync.member_remove` | Revoke a user's relation on an object |

---

## B2B Org

**Source struct:** `internal/domain/model/b2b_org.go` — `B2BOrg`

**Synced on:** create, update, reparent, delete of a B2B org.

### Access Config

| Field | Value |
|---|---|
| `object_type` | `b2b_org` |
| `public` | `false` |

### Relations

| Relation | Value | Condition |
|---|---|---|
| `global_org_admin` | `"team:{globalOrgAdminTeamUID}"` | On create only, when `globalOrgAdminTeamUID` is non-empty |
| `parent` | `"b2b_org:{ParentUID}"` | When `ParentUID` changes; empty clears the tuple |
| `child` | `["b2b_org:{child_uid}", ...]` | Updated on old/new parent when `ParentUID` changes |

> `parent` and `child` relations are excluded from the standard `update_access` message via `ExcludeRelations` and are managed by separate reparenting messages.

### Delete

On delete, only `uid` is sent — all FGA tuples for `b2b_org:{uid}` are removed by the fga-sync service.

---

## Project Membership

**Source struct:** `internal/domain/model/key_contact.go` — `KeyContact`

The member service manages the `key_contact` relation on `project_membership` objects when key contacts are created or removed. It does not issue `update_access` or `delete_access` for `project_membership` directly.

### Relations

| Relation | Value | Condition |
|---|---|---|
| `key_contact` | Contact's Authelia OIDC sub | On create/update via `member_put`; on delete/sub-change via `member_remove` |

---

## Triggers

| Operation | Object Type | Subject | Notes |
|---|---|---|---|
| Create B2B org | `b2b_org` | `lfx.fga-sync.update_access` | Sets `global_org_admin` tuple |
| Update B2B org | `b2b_org` | `lfx.fga-sync.update_access` | Always sent |
| Reparent B2B org | `b2b_org` | `lfx.fga-sync.update_access` | Up to 3 messages: org's own `parent`, old parent's `child` list, new parent's `child` list |
| Delete B2B org | `b2b_org` | `lfx.fga-sync.delete_access` | Always sent |
| Create key contact | `project_membership` | `lfx.fga-sync.member_put` | Only when contact has a resolved OIDC sub |
| Update key contact (sub change) | `project_membership` | `lfx.fga-sync.member_remove` + `lfx.fga-sync.member_put` | Revokes old sub, grants new sub |
| Delete key contact | `project_membership` | `lfx.fga-sync.member_remove` | Always sent when sub is known |
