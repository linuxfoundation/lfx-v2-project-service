# Indexer Contract — Project Service

This document is the authoritative reference for all data the project service sends to the indexer service, which makes resources searchable via the [query service](https://github.com/linuxfoundation/lfx-v2-query-service).

**Update this document in the same PR as any change to indexer message construction.**

---

## Resource Types

- [Project](#project)
- [Project Settings](#project-settings)
- [Project Link](#project-link)
- [Project Folder](#project-folder)
- [Project Document](#project-document)

Project and project-settings delete messages send only the deleted UID. Link,
folder, and document delete messages include `IndexingConfig` with the owning
project access metadata so the indexer can retain the same access fields on the
delete record.

---

## Project

**Object type:** `project`

**NATS subject:** `lfx.index.project`

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
| `funding` | string (optional) | Legacy funding value |
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

**Object type:** `project_settings`

**NATS subject:** `lfx.index.project_settings`

**Source struct:** `internal/domain/models/project.go` — `ProjectSettings`

**Indexed on:** create, update, delete of project settings. Settings share the same UID as their parent project.

### Data Schema

| Field | Type | Description |
|---|---|---|
| `uid` | string | Project UID (same as the parent project) |
| `mission_statement` | string (optional) | Project mission statement |
| `announcement_date` | timestamp (optional) | Project announcement date (RFC3339) |
| `auditors` | []object | Users with audit access. Each object has `avatar` (string), `email` (string), `name` (string), `username` (string — LFX username), and optionally `invite` (object — see [Invite Object](#invite-object)) when the user has no LFID yet |
| `writers` | []object | Users with write access. Each object has `avatar` (string), `email` (string), `name` (string), `username` (string — LFX username), and optionally `invite` (object — see [Invite Object](#invite-object)) when the user has no LFID yet |
| `meeting_coordinators` | []object | Users with meeting coordinator access. Each object has `avatar` (string), `email` (string), `name` (string), `username` (string — LFX username), and optionally `invite` (object — see [Invite Object](#invite-object)) when the user has no LFID yet |
| `executive_director` | object (optional) | Executive director user. Object has `avatar` (string), `email` (string), `name` (string), `username` (string — LFX username) |
| `program_manager` | object (optional) | Program manager user. Object has `avatar` (string), `email` (string), `name` (string), `username` (string — LFX username) |
| `opportunity_owner` | object (optional) | Opportunity owner user. Object has `avatar` (string), `email` (string), `name` (string), `username` (string — LFX username) |
| `marketing_ops_team` | object (optional) | Global Marketing Ops team on ROOT only. Object has `uid` (string — team UID) and `name` (string, optional display name) |
| `created_at` | timestamp (optional) | Creation time (RFC3339); null if not yet set |
| `updated_at` | timestamp (optional) | Last update time (RFC3339); null if not yet set |

#### Invite Object

A legacy nested `invite` object may appear on a user in `writers`, `auditors`, or `meeting_coordinators` who has no LFID yet (their `username` is empty). The service no longer writes this object when sending invites (acceptance events are now routed by recipient email, not stored invite UID — see `docs/lfid-invite-flow.md`), but existing records may still carry it: it survives PUT round-trips and is cleared, with `username` populated, when the user accepts their invite.

| Field | Type | Description |
|---|---|---|
| `uid` | string | Invite UID returned by the invite service |
| `email` | string | Email address the invite was delivered to |
| `expires_at` | timestamp (optional) | Invite expiry time (RFC3339); absent if the invite service did not return an expiry |

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

## Project Link

**Object type:** `project_link`

**NATS subject:** `lfx.index.project_link`

**Source struct:** `internal/domain/models/link.go` — `ProjectLink`

**Indexed on:** create, delete of a project link.

### Data Schema

| Field | Type | Description |
|---|---|---|
| `uid` | string | Link unique identifier |
| `project_uid` | string | UID of the owning project |
| `folder_uid` | string (optional) | UID of the folder this link belongs to |
| `name` | string | Display name of the link |
| `url` | string | Target URL |
| `description` | string (optional) | Link description |
| `created_by_username` | string (optional) | Username of the creator |
| `created_at` | timestamp | Creation time (RFC3339) |
| `updated_at` | timestamp | Last update time (RFC3339) |

### Tags

| Tag Format | Example | Purpose |
|---|---|---|
| `{uid}` | `abc-123` | Direct lookup by UID |
| `project_link_uid:{uid}` | `project_link_uid:abc-123` | Find links by UID |
| `project_uid:{project_uid}` | `project_uid:proj-456` | Find all links for a project |
| `folder_uid:{folder_uid}` | `folder_uid:folder-789` | Find all links in a folder |

> `folder_uid` tag is only emitted when `folder_uid` is set and non-empty.

### Access Control (IndexingConfig)

| Field | Value |
|---|---|
| `access_check_object` | `project:{project_uid}` |
| `access_check_relation` | `viewer` |
| `history_check_object` | `project:{project_uid}` |
| `history_check_relation` | `auditor` |

### Search Behavior

| Field | Value |
|---|---|
| `sort_name` | `name` |
| `fulltext` | _(none)_ |
| `public` | _(not set)_ |

### Parent References

| Ref | Condition |
|---|---|
| `project:{project_uid}` | Always set |

---

## Project Folder

**Object type:** `project_folder`

**NATS subject:** `lfx.index.project_folder`

**Source struct:** `internal/domain/models/folder.go` — `ProjectFolder`

**Indexed on:** create, delete of a project folder.

### Data Schema

| Field | Type | Description |
|---|---|---|
| `uid` | string | Folder unique identifier |
| `project_uid` | string | UID of the owning project |
| `name` | string | Display name of the folder (unique per project) |
| `created_by_username` | string (optional) | Username of the creator |
| `created_at` | timestamp | Creation time (RFC3339) |
| `updated_at` | timestamp | Last update time (RFC3339) |

### Tags

| Tag Format | Example | Purpose |
|---|---|---|
| `{uid}` | `folder-123` | Direct lookup by UID |
| `project_folder_uid:{uid}` | `project_folder_uid:folder-123` | Find folders by UID |
| `project_uid:{project_uid}` | `project_uid:proj-456` | Find all folders for a project |

### Access Control (IndexingConfig)

| Field | Value |
|---|---|
| `access_check_object` | `project:{project_uid}` |
| `access_check_relation` | `viewer` |
| `history_check_object` | `project:{project_uid}` |
| `history_check_relation` | `auditor` |

### Search Behavior

| Field | Value |
|---|---|
| `sort_name` | `name` |
| `fulltext` | _(none)_ |
| `public` | _(not set)_ |

### Parent References

| Ref | Condition |
|---|---|
| `project:{project_uid}` | Always set |

---

## Project Document

**Object type:** `project_document`

**NATS subject:** `lfx.index.project_document`

**Source struct:** `internal/domain/models/document.go` — `ProjectDocument`

**Indexed on:** upload (create), delete of a project document.

### Data Schema

| Field | Type | Description |
|---|---|---|
| `uid` | string | Document unique identifier |
| `project_uid` | string | UID of the owning project |
| `folder_uid` | string (optional) | UID of the folder this document belongs to |
| `name` | string | Display name of the document (unique per project) |
| `description` | string (optional) | Document description |
| `file_name` | string | Original file name from the upload |
| `file_size` | int64 | File size in bytes |
| `content_type` | string | MIME type of the file |
| `uploaded_by_username` | string (optional) | Username of the uploader |
| `created_at` | timestamp | Creation time (RFC3339) |
| `updated_at` | timestamp | Last update time (RFC3339) |

### Tags

| Tag Format | Example | Purpose |
|---|---|---|
| `{uid}` | `doc-123` | Direct lookup by UID |
| `project_document_uid:{uid}` | `project_document_uid:doc-123` | Find documents by UID |
| `project_uid:{project_uid}` | `project_uid:proj-456` | Find all documents for a project |
| `folder_uid:{folder_uid}` | `folder_uid:folder-789` | Find all documents in a folder |
| `content_type:{content_type}` | `content_type:application/pdf` | Filter documents by MIME type |
| `uploaded_by:{username}` | `uploaded_by:alice` | Filter documents by uploader |

> `folder_uid` tag is only emitted when `folder_uid` is set and non-empty. `content_type` and `uploaded_by` tags are only emitted when their respective fields are non-empty.

### Access Control (IndexingConfig)

| Field | Value |
|---|---|
| `access_check_object` | `project:{project_uid}` |
| `access_check_relation` | `viewer` |
| `history_check_object` | `project:{project_uid}` |
| `history_check_relation` | `auditor` |

### Search Behavior

| Field | Value |
|---|---|
| `sort_name` | `name` |
| `fulltext` | _(none)_ |
| `public` | _(not set)_ |

### Parent References

| Ref | Condition |
|---|---|
| `project:{project_uid}` | Always set |
