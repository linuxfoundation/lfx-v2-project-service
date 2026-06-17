<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# NATS Messaging (Reference)

Contents: subject inventory, KV bucket inventory, optimistic-locking pattern, and publish ordering for lfx-v2-project-service. The summary NATS rule (use constants, queue groups, don't write other services' buckets, drain on shutdown) lives in `SKILL.md`.

## Inbound RPC subjects (handled by this service)

```go
"lfx.projects-api.queue"          // queue group
"lfx.projects-api.get_name"       // get project name by UID
"lfx.projects-api.get_slug"       // get project slug by UID
"lfx.projects-api.get_logo"       // get project logo URL by UID
"lfx.projects-api.slug_to_uid"    // convert slug to UID
"lfx.projects-api.get_parent_uid" // get parent project UID
```

All five live as `Project*Subject` constants in `pkg/constants/nats.go`.

## Outbound subjects (published by this service)

```go
"lfx.index.project"
"lfx.index.project_settings"
"lfx.index.project_link"
"lfx.index.project_folder"
"lfx.index.project_document"
"lfx.projects-api.project_settings.updated"
"lfx.projects-api.project_document.created"
"lfx.projects-api.project_link.created"
"lfx.fga-sync.update_access"
"lfx.fga-sync.delete_access"
```

`lfx.index.*` envelopes are owned by `lfx-v2-indexer-service`. `lfx.fga-sync.*` envelopes are owned by `lfx-v2-fga-sync`. For per-resource fields, see `docs/indexer-contract.md` and `docs/fga-contract.md`. The `lfx.projects-api.*.created` payloads are `events.ProjectDocumentCreatedMessage` and `events.ProjectLinkCreatedMessage` in `pkg/events/`; this service also subscribes to them itself (`internal/service/document_subscriber.go`) to send upload-notification emails.

## Owned KV buckets

| Bucket | Purpose | History |
| --- | --- | --- |
| `projects` | Base project info | enabled |
| `project-settings` | Sensitive settings, stricter access | enabled |
| `project-links` | Project link records | enabled |
| `project-folders` | Project folder records | enabled |
| `project-documents-metadata` | Document metadata | enabled |
| `project-documents` | Document binaries (NATS object store) | n/a |

Other services must use this service's request/reply subjects rather than reading or writing these buckets directly.

Lookup keys also live inside the owning buckets:

- `projects`: `slug/{slug}` maps project slug to UID.
- `project-links`: `lookup/project-links/{project_uid}/{link_uid}` supports per-project link listing.
- `project-folders`: `lookup/project-folders/{hash}` reserves a per-project folder name.
- `project-documents-metadata`: `lookup/project-documents/{hash}` reserves a per-project document name.

## Optimistic locking

- Each KV record carries a NATS-assigned revision.
- GET endpoints set the `ETag` response header to that revision (as a string).
- PUT and DELETE require `If-Match`; generated Goa code places it on payload fields such as `payload.IfMatch`, and the service parses that value before passing the expected revision to the repository.
- A stale revision must surface as `domain.ErrRevisionMismatch`, mapped to HTTP 409 by `handleError`.

## Publish order

Every successful write performs storage first, then publish. Project base/settings writes use synchronous goroutine groups in the request path; link/folder/document writes publish synchronously only when `X-Sync: true`, otherwise they publish in a `context.WithoutCancel(ctx)` background goroutine.

1. Validate input and authorize via Heimdall (handled before the request reaches the service layer).
2. Read current revision if the operation is conditional.
3. Write to the owning KV bucket. Bail on error; no message is published.
4. Publish indexer message (`lfx.index.<resource>`) for resources that participate in search.
5. Publish FGA access message (`lfx.fga-sync.update_access` on create/update, `lfx.fga-sync.delete_access` on delete) for resources with their own access model.
6. Publish local notification subjects where documented (`lfx.projects-api.project_settings.updated`, `lfx.projects-api.project_document.created`, `lfx.projects-api.project_link.created`).

Storage is not rolled back for publish failures. Project base/settings publish failures return `domain.ErrInternal` from the request after the storage write. Link/folder/document publish failures return an error when `X-Sync: true`; when `X-Sync` is omitted or false, they are logged with `slog.WarnContext` in the background. The `MessageBuilder` also logs NATS send failures with the `subject`.

Publisher implementation: `internal/infrastructure/nats/message.go`.
