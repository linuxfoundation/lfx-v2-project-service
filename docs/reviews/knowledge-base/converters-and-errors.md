<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Converters, wire types, and domain errors

Patterns in the layer that converts between domain models, the Goa API types,
and the `pkg/events` NATS wire types — plus how upstream errors are mapped to
this repo's domain sentinels. The converter patterns are subtle wire-format
bugs (nil vs empty slice, empty-string pointers that should be nil); the error
patterns are about keeping the `internal/domain/errors.go` sentinel + HTTP
mapping discipline that the repo deliberately uses.

**Read when:** `internal/service/converters.go`,
`internal/service/*_operations.go`, `internal/domain/errors.go`,
`internal/domain/message.go`, `internal/infrastructure/nats/repository.go`,
`pkg/events/**`, or `cmd/project-api/service_endpoint_project.go` (the
`handleError` mapping) changed.

---

## `converters-and-errors/nil-vs-empty-slice` — Important

**Pattern:** a converter that builds a NATS event / API wire slice uses `make([]T, len(users))` unconditionally, turning a `nil` input slice into an empty `[]` on the wire. `nil` serializes as JSON `null` and `[]` as `[]` — a semantic difference that breaks downstream diff/patch consumers.

**Detect:** in `converters.go`, for functions converting `[]models.UserInfo` (or any domain slice) to wire/API types, check for an unconditional `make(...)`. The repo convention is an explicit `if users == nil { return nil }` guard before allocating — note `== nil`, not `len(...) == 0`, so an explicitly-empty `[]` is preserved as `[]`.

**Empirical citation:** PR #64 `internal/service/converters.go:592` (CodeRabbit + Copilot) — "the unconditional `make` turns a `nil` domain slice into an empty slice. This changes JSON serialization from `null` to `[]`, which can alter downstream diff/patch behavior." Recorded as a repo learning: "prefer `if users == nil { return nil }` over `if len(users) == 0 { return nil }` to preserve the semantic distinction." Live in `domainUsersToEvent` (`if users == nil { return nil }`).

**Failure message:** Converter collapses a nil slice to empty (`null` → `[]`) — wire-format change that breaks downstream diff/patch.

**Fix:** guard with `if users == nil { return nil }` (use `== nil`, not `len == 0`) before the `make`, mirroring `domainUsersToEvent` / `convertUsersToAPI`.

---

## `converters-and-errors/empty-string-pointer-not-normalized` — Important

**Pattern:** an optional pointer field (e.g. `folderUID *string`) is set from a request part/payload even when the underlying value is the empty string, leaving a non-nil pointer to `""`. With `omitempty` on a non-nil pointer the field is not omitted, so the record is stored / indexed with `folder_uid:""` instead of being absent.

**Detect:** where an optional `*string` is assigned from a multipart value, payload field, or env var, check that an empty value is normalized to `nil` (the repo pattern is `if folderUID != nil && *folderUID == "" { folderUID = nil }`). Flag direct `&value` assignment without the empty-string check.

**Empirical citation:** PR #60 `internal/service/document_operations.go` (CodeRabbit) — "`folderUID` is only validated when non-empty, but it's not normalized to `nil` when it's an empty string ... the document metadata will be stored with `folder_uid:''` ... Normalize empty folder UID pointers to `nil` (same pattern as `CreateLink`)." Fixed consistently across `CreateLink`, `CreateFolder`, `UploadDocument`.

**Failure message:** Optional pointer set to a non-nil empty string — stored/indexed as `field:""` instead of being omitted.

**Fix:** normalize to nil before use: `if p != nil && *p == "" { p = nil }`, matching the `CreateLink` / `CreateFolder` pattern.

---

## `converters-and-errors/raw-upstream-error-at-boundary` — Critical

**Pattern:** a raw NATS / KV / upstream HTTP error is returned past the service layer toward the Goa boundary instead of being mapped to a domain sentinel from `internal/domain/errors.go`. `handleError` then can't map it to the correct HTTP status, and internal error text can leak.

**Detect:** in service / repository code, find returns of raw `jetstream.*` / `nats.*` / upstream errors that flow up to `cmd/project-api/service_endpoint_project.go`. Confirm each is wrapped/translated to a `domain.Err*` sentinel (e.g. KV wrong-sequence → `domain.ErrRevisionMismatch`, key-not-found → `domain.ErrProjectNotFound`). New sentinels must be added to both `errors.go` and the `handleError` switch.

**Empirical citation:** anchored in `internal/domain/errors.go` and the `handleError` mapping the repo enforces; the sentinel approach was explicitly endorsed in PR #2 `internal/domain/errors.go:11` (mauriciozanettisalomao) — "I like this approach (separating internal error handling flow from the HTTP errors)." The `project-service-dev` skill: "Do not return raw NATS, KV, or upstream HTTP errors directly to the Goa layer; map them to a domain sentinel first."

**Failure message:** Raw upstream (NATS/KV/HTTP) error reaching the Goa boundary — `handleError` can't map it and internal detail may leak.

**Fix:** translate the upstream error to a `domain.Err*` sentinel (wrapping with `%w` to preserve `errors.Is`), and ensure `handleError` maps that sentinel to the documented HTTP status (404/409/400/500/503).
