# Indexer Contract — Project Service

This document is the authoritative reference for all data the project service sends to the indexer service, which makes resources searchable via the [query service](https://github.com/linuxfoundation/lfx-v2-query-service).

**Update this document in the same PR as any change to indexer message construction.**

---

## Resource Types

- [Project](#project)
- [Project Settings](#project-settings)

---

## Project

**Source struct:** `internal/domain/models/project.go` — `ProjectBase`

**Indexed on:** create, update, delete of a project.

### Data Schema

These fields are indexed and queryable via `filters` or `cel_filter` in the query service.

| Field | Type | Description |
|---|---|---|
| `uid` | string | Project unique identifier |
| `slug` | string | URL-safe project identifier |
| `name` | string | Project name |
| `description` | string (optional) | Project description |
| `public` | bool | Whether the project is publicly visible |
| `is_foundation` | bool | Whether the project is a foundation |
| `parent_uid` | string (optional) | UID of the parent project (if nested) |
| `stage` | string (optional) | Project lifecycle stage |
| `category` | string (optional) | Project category |
| `legal_entity_type` | string (optional) | Legal entity type |
| `legal_entity_name` | string (optional) | Legal entity name |
| `legal_parent_uid` | string (optional) | UID of the legal parent entity |
| `funding_model` | []string (optional) | Funding model types |
| `entity_dissolution_date` | timestamp (optional) | Date the legal entity was dissolved |
| `entity_formation_document_url` | string (optional) | URL to the formation document |
| `formation_date` | timestamp (optional) | Date the project was formed |
| `autojoin_enabled` | bool | Whether auto-join is enabled |
| `charter_url` | string (optional) | URL to the project charter |
| `logo_url` | string (optional) | URL to the project logo |
| `website_url` | string (optional) | Project website URL |
| `repository_url` | string (optional) | Project repository URL |
| `created_at` | timestamp (optional) | Creation time (RFC3339); null if not yet set |
| `updated_at` | timestamp (optional) | Last update time (RFC3339); null if not yet set |

### Tags

| Tag Format | Example | Purpose |
|---|---|---|
| `project_slug:{value}` | `project_slug:kubernetes` | Find projects by slug |

> The `project_slug` tag is only emitted when `slug` is non-empty.

### Access Control (IndexingConfig)

| Field | Value |
|---|---|
| `access_check_object` | `project:{uid}` |
| `access_check_relation` | `viewer` |
| `history_check_object` | `project:{uid}` |
| `history_check_relation` | `writer` |

### Search Behavior

| Field | Value |
|---|---|
| `fulltext` | `name`, `slug`, `description` (non-empty values only) |
| `name_and_aliases` | `name`, `slug` (non-empty values only) |
| `sort_name` | `name` |
| `public` | set from `project.public` |

### Parent References

| Ref | Condition |
|---|---|
| `project:{parent_uid}` | Only when `parent_uid` is set |

---

## Project Settings

**Source struct:** `internal/domain/models/project.go` — `ProjectSettings`

**Indexed on:** create, update, delete of project settings. Settings share the same UID as their parent project.

### Data Schema

| Field | Type | Description |
|---|---|---|
| `uid` | string | Project UID (same as the parent project) |
| `mission_statement` | string (optional) | Project mission statement |
| `announcement_date` | timestamp (optional) | Project announcement date (RFC3339) |
| `auditors` | []object | Users with audit access. Each object has `avatar` (string), `email` (string), `name` (string), `username` (string — holds the user ID / sub value) |
| `writers` | []object | Users with write access. Each object has `avatar` (string), `email` (string), `name` (string), `username` (string — holds the user ID / sub value) |
| `meeting_coordinators` | []object | Users with meeting coordinator access. Each object has `avatar` (string), `email` (string), `name` (string), `username` (string — holds the user ID / sub value) |
| `created_at` | timestamp (optional) | Creation time (RFC3339); null if not yet set |
| `updated_at` | timestamp (optional) | Last update time (RFC3339); null if not yet set |

### Tags

Tags are sent as template placeholders inside `IndexingConfig.Tags` and resolved by the indexer using the document's own field values.

| Tag Template | Resolved Example | Purpose |
|---|---|---|
| `{{ uid }}` | `cbef1ed5-17dc-4a50-84e2-6cddd70f6878` | Direct lookup by UID |
| `{{ mission_statement }}` | `Advancing open source...` | Lookup by mission statement text |

> Tags are only emitted when the corresponding field is non-empty.

### Access Control (IndexingConfig)

| Field | Value |
|---|---|
| `access_check_object` | `project:{project_uid}` (the parent project UID, not the settings UID) |
| `access_check_relation` | `auditor` |
| `history_check_object` | `project:{project_uid}` (the parent project UID, not the settings UID) |
| `history_check_relation` | `writer` |

### Search Behavior

| Field | Value |
|---|---|
| `fulltext` | _(none)_ |
| `name_and_aliases` | _(none)_ |
| `sort_name` | _(none)_ |
| `public` | _(not set)_ |

### Parent References

| Ref | Condition |
|---|---|
| `project:{uid}` | Always set (when `uid` is non-empty) |

---

## Access Control Messages

In addition to indexer messages, the project service publishes access control messages to the fga-sync service.

### Project Access Message

**Subject:** `lfx.update_access.project`

**Published on:** create, update of a project or project settings.

**Schema:**

| Field | Type | Description |
|---|---|---|
| `data.uid` | string | Project UID |
| `data.public` | bool | Whether the project is publicly visible |
| `data.parent_uid` | string | UID of the parent project (empty string if none) |
| `data.writers` | []string | Usernames of users with write access |
| `data.auditors` | []string | Usernames of users with audit access |
| `data.meeting_coordinators` | []string | Usernames of users with meeting coordinator access |

**Subject:** `lfx.delete_all_access.project`

**Published on:** delete of a project. The message body is the project UID string (not a JSON object).

---

## NATS Subjects Summary

| Subject | Direction | Trigger |
|---|---|---|
| `lfx.index.project` | Outbound | Project created, updated, or deleted |
| `lfx.index.project_settings` | Outbound | Project settings created, updated, or deleted |
| `lfx.update_access.project` | Outbound | Project or project settings created or updated |
| `lfx.delete_all_access.project` | Outbound | Project deleted |
| `lfx.projects-api.project_settings.updated` | Outbound | Project settings updated (contains before/after state) |
