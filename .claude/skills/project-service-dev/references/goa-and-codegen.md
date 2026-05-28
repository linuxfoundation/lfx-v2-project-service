<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Goa and Code Generation (Reference)

Project-service Goa layout, type split, and ETag/If-Match wiring. The summary generated-code rule lives in `SKILL.md`; read this file when adding or changing a design file, regenerating, or touching optimistic concurrency.

## Local layout

- Design source: `api/project/v1/design/`
- Generated code: `api/project/v1/gen/` (tracked; never edit by hand)
- Endpoint adapters: `cmd/project-api/service_endpoint_*.go`
- Hand-written business logic: `internal/service/`

Run `make apigen` after any change under `api/project/v1/design/`. The Makefile pins `GOA_VERSION=v3.22.6`; do not regenerate with a different version casually. Generated files are committed in this repo, so include the resulting `api/project/v1/gen/` diffs with the design change.

## Project type split

Project data is modeled as four Goa types:

- `project-base`
- `project-base-with-readonly-attributes`
- `project-settings`
- `project-settings-with-readonly-attributes`

The split exists because settings need stricter access than base data (settings require `auditor`/`writer`, base requires `viewer`). Other services should copy this only when their own access model needs the same separation.

## ETag and If-Match wiring

- GET endpoints that return mutable project data set the `ETag` response header. The value is the NATS KV revision (as a string). Project base/settings set `constants.ETagContextID` so the custom encoder in `cmd/project-api/main.go` can write the header; link/folder/document handlers return the generated result header value directly.
- PUT and DELETE endpoints accept `If-Match` and refuse the write when the revision does not match. Generated Goa decoders place the value on payload fields such as `payload.IfMatch`; service code parses that field and passes the expected revision to the repository.
- A revision mismatch surfaces as `domain.ErrRevisionMismatch` and maps to HTTP 409 via `handleError`.
- Goa design must declare both the `ETag` response header and the `If-Match` request header on the relevant endpoints; if either is missing, generated code will silently drop it.

## Adding a new endpoint

1. Edit `api/project/v1/design/project.go` (or the relevant design file).
2. Run `make apigen`. Commit the design file and the generated `api/project/v1/gen/` changes together.
3. Implement the interface method in `cmd/project-api/service_endpoint_<resource>.go`. Translate payload to service call, route errors through `handleError`.
4. Add or extend the operations file in `internal/service/`.
5. Add the table-driven test case alongside.
6. Update `charts/lfx-v2-project-service/templates/ruleset.yaml` if the new endpoint needs an OpenFGA check.
