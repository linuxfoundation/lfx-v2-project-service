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

- **Slug.** Unique across projects, matching `^[a-z][a-z0-9_\-]*[a-z0-9]$`. It can change: the new slug is reserved first, and the old slug's index key is removed after the update. If that removal fails the old key is left behind and logged; `project-cli inspect slug-index` finds such entries (see [KV layout](#kv-layout)).
- **Parent.** `parent_uid` is empty or the UID of an existing project. A project cannot be its own parent; longer cycles are not rejected on write and are reported by `project-cli inspect parents`. When FGA is enabled, changing the parent requires `writer_guard` on both the old and the new parent.
- **Stage.** One of `Formation - Exploratory`, `Formation - Engaged`, `Active`, `Archived`, `Formation - On Hold`, `Formation - Disengaged`, `Prospect`. A project submitted as `Archived` must carry an `entity_dissolution_date`.
- **Stage and access.** `Prospect` and `Formation - Confidential` projects withhold the writer, auditor and marketing-ops global team grants; the owner grant is always sent. `Formation - Confidential` is not in the API's stage enum today; it is matched by exact spelling in `internal/service/global_grants.go`. See [fga-contract.md](fga-contract.md).
- **Name.** Must not be blank, start with `{` or contain control characters. A `{`-prefixed name would make every NATS read of the project look like an error reply.
- **Deletion.** Only a project whose funding model is exactly `["Crowdfunding"]` can be deleted through the API. Child links, folders and documents are not removed; `project-cli inspect orphans` lists what is left behind.
- **Roles.** A role entry with an email is re-resolved from the auth service by that email on every write, and the answer replaces any username the caller sent. When the auth service does not know the email, the caller's username is cleared but the LFID already stored for that email is kept. An entry without an email keeps whatever username it carries; that is how machine-to-machine principals are retained, and it is not checked against the auth service. A role holder with no username gets an invite request instead of an email; see [lfid-invite-flow.md](lfid-invite-flow.md).

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

Three rules follow from this layout:

- A link, folder or document UID from a caller must have the shape of a UID before it is used as a key (`domain.IsValidUID`, applied in `kvCollection.getForProject`), so index and reservation keys cannot be read by guessing. Project UIDs are validated as UUIDs at the HTTP and NATS boundaries.
- A sub-resource must belong to the project in the route. Heimdall authorizes only the project in the path (`/projects/:uid/...`), so the service itself confirms the record's `project_uid` matches and reports it as not found otherwise (`kvCollection.getForProject`). A new sub-resource read or write must go through it or repeat the check, otherwise a writer on one project could reach another project's content.
- The record, not the index key, is the source of truth. Listing code skips index keys (`slug/` in the `projects` bucket, `lookup/` elsewhere), and `project-cli inspect children` reports records without their index key and index keys without their record.

### Revisions

The JetStream revision of a record is its ETag. `GET` returns it as the `ETag` header; `PUT` and `DELETE` send it back as `If-Match`. Updates and deletes are compare-and-swap on that revision, and a stale revision becomes `ErrRevisionMismatch` (HTTP 409). Setting `SKIP_ETAG_VALIDATION=true` makes the service read the current revision itself; it exists for local development only.

## How a write travels

Every write follows the same shape: validate, write the KV record, then publish.

1. **Validate** the payload and, where it names other resources, check they exist (parent project, folder) and that the caller may use them.
2. **Write the record** with a conditional or revision-checked KV operation. Name and slug reservations are taken first and released again when the record write is confirmed not to have committed (for a new project, only after re-reading the base).
3. **Publish** to the consumers that must follow the change:
   - the indexer, for search (`lfx.index.*`);
   - fga-sync, for access control (`lfx.fga-sync.update_access` or `delete_access`), a full sync of the project's relations. Only project writes publish to fga-sync: links, folders and documents have no relations of their own and are authorized through their project;
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
| Delete link, folder or document | record, then its index or name-reservation key; for a document, its file (background, a failure is only logged) | indexer delete (background) |

Update base deliberately does not republish access unless a field that affects it changed. The settings update is the authoritative publisher of role changes, and republishing access from a base update could re-grant a user that a concurrent settings write had just revoked.

### Documents and folders

- Upload needs a name, a file name and a non-empty file of up to 10 MB, with an allow-listed content type. The bytes are sniffed: an allow-listed sniffed type replaces the declared one, a sniffed type outside the list is rejected, and when sniffing cannot tell (`application/octet-stream`, as for legacy `.doc` or `.xls`) the declared type is kept unverified. The name reservation is taken first, then the file is stored, then the metadata. A failure after a step undoes the earlier steps.
- A folder name is unique within its project. A folder that still contains a link or document cannot be deleted.

### Settings written by events

Project settings are also written by two inbound event handlers, not only by `PUT /projects/{uid}/settings`: `HandleInviteAccepted` promotes email-only role holders to their LFID once they accept an invite, and `HandleUserDeleted` scrubs a deleted user's username (`lfx.v1-sync-helper.user.deleted`). Both update the settings with a revision check, then republish the indexer and FGA messages through `republishProjectSettings`. No request is waiting on them, so a failure there is logged rather than returned.

### When publishing fails

The KV record is written before anything is published and there is no outbox, so a failed publish leaves the data saved and the index or access state behind. For project writes the caller also sees an error; for link, folder and document writes the failure is only a warning in the logs, so the drift is silent.

Creating a project has one more partial state. The base record and slug are written first, then the settings. If the settings write fails, the base and the slug are deliberately kept, the request returns an error and nothing is published: the project exists in KV without settings, a search document or FGA tuples, and retrying with the same slug is rejected because the slug is taken. `project-cli inspect project` shows what exists in each store for one project.

The repair and inspection tools are in `project-cli`:

- `inspect index-drift` lists records missing from search. Only projects can be republished, with `sync reindex-projects` (and `--include-access` to republish access too); a missing link, folder or document has no repair command yet.
- `inspect settings-access` and `sync verify-global-grants` compare FGA tuples with what the records call for.
- `inspect children` checks index keys against records, and `inspect orphans` lists content whose project is gone.

## Reads over NATS

Other services read project data through request/reply handlers rather than the HTTP API: names, slugs, logos, writers, settings, parent UID, slug-to-UID and project listing. The handlers are in `internal/service/project_handlers.go`, and the subjects are listed in [README.md](../README.md). A failed read is returned as a JSON error envelope, which is why project names are validated against a leading `{`.

## Authorization

Two layers, both outside the request handlers' business logic:

- **Heimdall** authenticates the JWT and enforces the per-route rules in `charts/lfx-v2-project-service/templates/ruleset.yaml`. Adding or changing an endpoint means updating that ruleset.
- **OpenFGA** holds the relations those rules check (`viewer`, `writer_guard`, `auditor_guard`, `owner`, and `writer` on the parent for `POST /projects`, which must stay `writer` rather than `writer_guard`; see [AGENTS.md](../AGENTS.md#authorization-openfga)). This service never evaluates FGA itself except for one inline check: changing a project's parent asks fga-sync whether the caller may write both parents (`FGA_ENABLED`).

The per-endpoint requirements are listed in [AGENTS.md](../AGENTS.md#authorization-openfga).
