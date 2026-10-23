<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Project Service Overview

This is the map of what the project service manages and how a write travels through it. It is the place to start when you need to understand the service before changing it. Exact payloads and subjects live in the contract docs it links to; this document does not repeat them.

| Looking for | Go to |
| --- | --- |
| HTTP endpoints, NATS subjects handled and published | [README.md](../README.md) |
| Building, running and testing locally | [DEVELOPMENT.md](../DEVELOPMENT.md) |
| Coding conventions, repo layout, review rules | [AGENTS.md](../AGENTS.md) |
| Messages sent to the indexer | [indexer-contract.md](indexer-contract.md) |
| Messages sent to fga-sync and the global team grants | [fga-contract.md](fga-contract.md) |
| What happens when a role holder has no LFID | [lfid-invite-flow.md](lfid-invite-flow.md) |
| Operational repair and inspection commands | [cmd/project-cli/README.md](../cmd/project-cli/README.md) |

## What the service owns

The service is the system of record for projects in the LFX platform and for the content attached to them. It stores everything in NATS JetStream (key-value buckets and one object store), publishes every change so the indexer and the access-control service stay in sync, and answers read-only questions from other services over NATS request/reply. There is no relational database.

## Resources

| Resource | What it is | Stored in |
| --- | --- | --- |
| Project base | Identity and descriptive fields: slug, name, stage, category, parent, legal entity, funding, URLs, contact email | `projects` KV |
| Project settings | The people and sensitive fields: roster of writers, auditors, meeting coordinators, mentorship program admins, plus executive director, program manager, opportunity owner, mission statement, announcement date | `project-settings` KV |
| Link | A named URL attached to a project, optionally inside a folder | `project-links` KV |
| Folder | A named container, unique by name within a project, for links and documents | `project-folders` KV |
| Document | A file attached to a project, optionally inside a folder. Metadata and file are stored separately | `project-documents-metadata` KV and `project-documents` object store |

Base and settings are separate records on purpose: access to settings is more restricted than access to the base (reading settings needs `auditor_guard`, writing needs `writer_guard`), and the two are indexed and authorized independently.

Domain types are in `internal/domain/models/`. Types that other services receive over NATS are in `pkg/events/`.

## Project rules worth knowing

- **Slug.** Unique across projects, matching `^[a-z][a-z0-9_\-]*[a-z0-9]$`. It can change; the slug index is kept consistent through the change (see [KV layout](#kv-layout)).
- **Parent.** `parent_uid` is empty or the UID of an existing project. A project cannot be its own parent; longer cycles are not rejected on write and are reported by `project-cli inspect parents`. When FGA is enabled, changing the parent requires `writer_guard` on both the old and the new parent.
- **Stage.** One of `Formation - Exploratory`, `Formation - Engaged`, `Active`, `Archived`, `Formation - On Hold`, `Formation - Disengaged`, `Prospect`. A project submitted as `Archived` must carry an `entity_dissolution_date`.
- **Stage and access.** `Prospect` and `Formation - Confidential` projects withhold the writer, auditor and marketing-ops global team grants; the owner grant is always sent. `Formation - Confidential` is not in the API's stage enum today; it is matched by exact spelling in `internal/service/global_grants.go`. See [fga-contract.md](fga-contract.md).
- **Name.** Must not be blank, start with `{` or contain control characters. A `{`-prefixed name would make every NATS read of the project look like an error reply.
- **Deletion.** Only a project whose funding model is exactly `["Crowdfunding"]` can be deleted through the API. Child links, folders and documents are not removed; `project-cli inspect orphans` lists what is left behind.
- **Roles.** Usernames in role lists are not trusted from the caller: on every write they are overwritten with the auth service's answer for the entry's email. A role holder with no LFID has no username, so they get an invite request instead of an email; see [lfid-invite-flow.md](lfid-invite-flow.md).

## KV layout

Records are JSON, keyed by UID. Several buckets also hold index keys next to the records. These are written and read only by the repository layer in `internal/infrastructure/nats/` and are named in `pkg/constants/nats.go`.

| Bucket | Key | Value | Purpose |
| --- | --- | --- | --- |
| `projects` | `<projectUID>` | project base JSON | The record |
| `projects` | `slug/<slug>` | project UID | Slug to UID lookup. Reserved with a conditional create, so two concurrent creates for one slug have exactly one winner |
| `project-settings` | `<projectUID>` | settings JSON | The record |
| `project-links` | `<linkUID>` | link JSON | The record |
| `project-links` | `lookup/project-links/<projectUID>/<linkUID>` | link UID | Per-project index, so a project's links can be listed |
| `project-folders` | `<folderUID>` | folder JSON | The record |
| `project-folders` | `lookup/project-folders/<sha256(projectUID\|name)>` | folder UID | Reserves the folder name within the project |
| `project-documents-metadata` | `<documentUID>` | document metadata JSON | The record |
| `project-documents-metadata` | `lookup/project-documents/<sha256(projectUID\|name)>` | document UID | Reserves the document name within the project |
| `project-documents` object store | `<documentUID>` | file bytes | The file |

Two rules follow from this layout:

- Anything accepting a UID from a caller must check it has the shape of a UID before using it as a key (`domain.IsValidUID`), so index and reservation keys cannot be read by guessing.
- The record, not the index key, is the source of truth. Listing code skips `lookup/` keys, and `project-cli inspect children` reports records without their index key and index keys without their record.

### Revisions

The JetStream revision of a record is its ETag. `GET` returns it as the `ETag` header; `PUT` and `DELETE` send it back as `If-Match`. Updates and deletes are compare-and-swap on that revision, and a stale revision becomes `ErrRevisionMismatch` (HTTP 409). Setting `SKIP_ETAG_VALIDATION=true` makes the service read the current revision itself; it exists for local development only.

## How a write travels

Every write follows the same shape: validate, write the KV record, then publish.

1. **Validate** the payload and, where it names other resources, check they exist (parent project, folder) and that the caller may use them.
2. **Write the record** with a conditional or revision-checked KV operation. Name and slug reservations are taken first and released if the record write fails.
3. **Publish** to the consumers that must follow the change:
   - the indexer, for search (`lfx.index.*`);
   - fga-sync, for access control (`lfx.fga-sync.update_access` or `delete_access`), a full sync of the object's relations;
   - for settings changes, the `project_settings.updated` event, which this service itself consumes to send role-change emails and invites;
   - for a new link or document, a created event that this service consumes to email the project's writers and auditors.

Projects and content publish differently:

- **Project writes** (create, update, delete, settings) publish concurrently and wait. If any publish fails, the request returns an internal error.
- **Link, folder and document writes** publish from background goroutines with a detached context (`publishIndexer` and the created-event goroutines). A failed publish is logged as a warning and the request still succeeds.

| Operation | Record written | Published |
| --- | --- | --- |
| Create project | base, settings, slug index | indexer (base and settings), FGA, settings-updated event |
| Update base | base, slug index if the slug changed | indexer; FGA only when `public`, `parent_uid` or the stage's grant gating changed |
| Update settings | settings | indexer, FGA, settings-updated event with the before and after snapshot |
| Delete project | base, slug index, settings | indexer deletes, FGA `delete_access` |
| Create link, folder or document | record, index or name reservation, and for a document the file | indexer (background); for a link or document also a created event (background) |

Update base deliberately does not republish access unless a field that affects it changed. The settings update is the authoritative publisher of role changes, and republishing access from a base update could re-grant a user that a concurrent settings write had just revoked.

### Documents and folders

- Upload accepts an allow-listed content type, checked against the declared type and the sniffed bytes, and files up to 10 MB. The name reservation is taken first, then the file is stored, then the metadata. A failure after a step undoes the earlier steps.
- A folder name is unique within its project. A folder that still contains a link or document cannot be deleted.

### When publishing fails

The KV record is written before anything is published and there is no outbox, so a failed publish leaves the data saved and the index or access state behind. For project writes the caller also sees an error; for link, folder and document writes the failure is only a warning in the logs, so the drift is silent. The repair tools are in `project-cli`: `inspect index-drift` lists records missing from search, `sync reindex-projects` republishes them (with `--include-access` it also republishes access), and `inspect settings-access` and `sync verify-global-grants` compare FGA tuples with what the records call for.

## Reads over NATS

Other services read project data through request/reply handlers rather than the HTTP API: names, slugs, logos, writers, settings, parent UID, slug-to-UID and project listing. The handlers are in `internal/service/project_handlers.go`, and the subjects are listed in [README.md](../README.md). A failed read is returned as a JSON error envelope, which is why project names are validated against a leading `{`.

## Authorization

Two layers, both outside the request handlers' business logic:

- **Heimdall** authenticates the JWT and enforces the per-route rules in `charts/lfx-v2-project-service/templates/ruleset.yaml`. Adding or changing an endpoint means updating that ruleset.
- **OpenFGA** holds the relations those rules check (`viewer`, `writer_guard`, `auditor_guard`, `owner`). This service never evaluates FGA itself except for one inline check: changing a project's parent asks fga-sync whether the caller may write both parents (`FGA_ENABLED`).

The per-endpoint requirements are listed in [AGENTS.md](../AGENTS.md#authorization-openfga).
