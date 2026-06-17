<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Observability, error mapping, and infrastructure resilience

Patterns where the service mis-maps infrastructure failures, swallows error context in
logs, or has resilience gaps in its NATS/RPC plumbing. These are repo-specific because
they hinge on this service's domain-error family (`pkg/errors`), the `wrapError` mapper,
the project-id-map RPC handler, and the NATS client lifecycle. Mostly Important; the
RPC-failure-as-NotFound mapping is the highest-value one (corrupts incident signal).

**Read when:** any file under `pkg/errors/**`, `cmd/member-api/service/error.go`,
`internal/infrastructure/nats/project_rpc.go`,
`internal/infrastructure/nats/project_id_map_handler.go`,
`internal/infrastructure/nats/client.go`, or any handler that maps/logs errors.
Cross-checked in Steps 3-4 of the learnings-review playbook.

---

## `observability-and-resilience/infra-failure-mapped-to-notfound` — Important

**Pattern:** an outbound RPC (or resolver) maps *all* request failures to `NotFound`,
including infrastructure failures (disconnected NATS, permissions, serialization). The
HTTP API and the project-id-map RPC then return 404 for outages that should be 503,
masking incidents and breaking dashboards/alerts.

**Detect:** in `internal/infrastructure/nats/project_rpc.go` and the resolver, find error
mapping that returns `NewNotFound` (or HTTP 404) for any non-application error. Flag a
blanket `return NotFound` that does not distinguish a genuine "not found" reply from a
transport/timeout/serialization failure.

**Empirical citation:** PR #14 `internal/infrastructure/nats/project_rpc.go:64` — Copilot — "Both RPC methods map *all* request failures to `NotFound`, including infrastructure failures (e.g. disconnected NATS, permissions, serialization issues). This can cause the HTTP API and the project-id-map RPC to return 404/\"not found\" for outages that should be 503". Related discrimination work was acted on PR #42 `b2b_org_settings.go:86` (dealako `[blocking]`: distinguish optimistic-lock Conflict from transport/marshal failures → `NewUnexpected`).

**Failure message:** Infrastructure failure mapped to NotFound/404 — outages masquerade as "not found", corrupting incident signal.

**Fix:** distinguish a genuine application "not found" from transport/timeout/permission/
serialization failures. Return `NotFound` only for the former; map the latter to
`Unexpected` / `ServiceUnavailable` (503) with the underlying error preserved for
diagnosability.

---

## `observability-and-resilience/swallowed-error-in-log` — Important

**Pattern:** an error is logged without including the underlying error value, or a
fetch/conversion failure is silently swallowed with no log signal at all. Operators then
cannot see why a record was skipped, a child omitted, or an email not resolved.

**Detect:** find `slog.WarnContext`/`ErrorContext` calls that describe a failure but omit
an `"error", err` attribute, and `if err == nil { ... }` branches that drop the
non-nil-error case without any log.

**Empirical citation:** PR #40 `internal/infrastructure/salesforce/account_repo.go:78` — Copilot — "When a child Account SFID fails UUID conversion, the warning log omits the underlying conversion error, making debugging difficult. Include `convErr` (e.g. as an `error` field)". Acted on: prabodhcs — "added `\"error\", convErr` to the WarnContext call". Recurs PR #41 `key_contact_repo.go:335` (IterKeyContacts silently ignored `fetchPrimaryEmails` errors → added a `slog.WarnContext`).

**Failure message:** Error value omitted from the log (or failure swallowed with no log) — operators can't diagnose why a record was skipped.

**Fix:** include `"error", err` in the log attributes, and add at least a
`slog.WarnContext` on any swallowed fetch/conversion failure so the skip is visible.

---

## `observability-and-resilience/unbounded-rpc-handler-context` — Important

**Pattern:** a NATS subscription callback (the project-id-map RPC handler) uses
`context.Background()` with no timeout, so a slow/hung dependency (project-service RPC,
Salesforce, KV) can block the callback indefinitely, timing out requesters and building
goroutine/memory pressure under load.

**Detect:** in `internal/infrastructure/nats/project_id_map_handler.go` (and any NATS
message callback), confirm the handler derives a `context.WithTimeout` rather than calling
downstream dependencies on a bare `context.Background()`.

**Empirical citation:** PR #14 `internal/infrastructure/nats/project_id_map_handler.go:69` — Copilot — "The handler uses `context.Background()` with no timeout, so a slow/hung dependency (project-service RPC, Salesforce, KV) can block the subscription callback indefinitely. That can lead to requesters timing out and increased memory/goroutine pressure under load."

**Failure message:** NATS RPC handler runs on an unbounded `context.Background()` — a hung dependency blocks the callback indefinitely.

**Fix:** wrap the handler's downstream calls in a `context.WithTimeout` derived context so
a slow dependency fails fast instead of pinning the subscription goroutine.

---

## `observability-and-resilience/4xx-logged-at-error` — Nit

**Pattern:** `wrapError` (or a handler) logs expected 4xx control-flow errors
(`Validation`, `NotFound`) at ERROR level, inflating error-rate metrics and burying real
5xx issues. The repo already routes `NotImplemented`→Debug and `Conflict`/`PreconditionFailed`→Warn;
`Validation`/`NotFound` at Error is the residual inconsistency.

**Detect:** in `cmd/member-api/service/error.go` `wrapError`, check the log level used per
domain-error type. Flag `Validation`/`NotFound` logged at `slog.ErrorContext`.

**Empirical citation:** PR #14 `cmd/member-api/service/error.go:32` — Copilot — "`wrapError` logs every failure at error level, including expected 4xx control-flow errors (validation/not-found). This can inflate error-rate metrics and make real 5xx issues harder to spot. Recommendation: log `Validation`/`NotFound` at `Info` or `Debug`". (Note: as of `origin/main` this is partially addressed — `NotImplemented`→Debug, `Conflict`/`PreconditionFailed`→Warn — but `Validation`/`NotFound` still log at Error, so the residual case stands. Treat as a Nit unless the team decides to lower them.)

**Failure message:** Expected 4xx (Validation/NotFound) logged at ERROR — inflates error-rate metrics and buries real 5xx.

**Fix:** log expected 4xx control-flow errors at Info/Debug (or Warn), reserving Error for
genuine 5xx, consistent with the existing per-type levels in `wrapError`.
