<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Logging and PII

Patterns around this service's `log/slog` usage and, most importantly, keeping
user-identifying values out of application logs. The PII pattern was flagged by
both CodeRabbit and a human maintainer as blocking on the project-member
notification flow, which logs per-recipient detail; it is a data-handling
pattern with a high cost of miss.

**Read when:** any `internal/service/project_subscriber.go`,
`internal/service/email/**`, `internal/middleware/**`, or any Go file that calls
`slog.*Context` while handling user/notification data changed.

---

## `logging-and-pii/raw-user-identifiers-in-logs` — Critical

**Pattern:** a log line in a notification / invite / member flow includes raw user-identifying fields — `username` (LFID), recipient email (`to`), or similar. The flow is already traceable by `project_uid` and `role`, so logging the identifier is avoidable PII retention.

**Detect:** in `project_subscriber.go`, the email package, or any per-recipient handler, scan `slog.InfoContext` / `WarnContext` / `ErrorContext` key/value lists for `"username"`, `"to"`, `"email"`, or an LFID/email value (`add.User.Username`, `recipientEmail`). The acceptable correlates are `project_uid` and `role` (plus a non-identifying index/hash if per-recipient debugging is truly needed).

**Empirical citation:** PR #70 `internal/service/project_subscriber.go:66` (CodeRabbit) — "These new log fields write LFID/email values (`username`, `to`) into application logs. That is avoidable PII retention for a flow that is already traceable by `project_uid` and `role`; please omit them". Escalated by maintainer dealako (`internal/service/project_subscriber.go:66`): "**[blocking]** PII still in logs ... This line still emits `"username", add.User.Username`. The same applies to `"to", recipientEmail`". Fixed; current `project_subscriber.go` logs `project_uid` / `role` only.

**Failure message:** Raw user identifier (LFID `username` / recipient `email`) written to logs — avoidable PII retention.

**Fix:** drop the `username` / `to` / `email` log fields; keep `project_uid` and `role`. If per-recipient debugging is needed, use a non-identifying correlate (recipient index or a salted hash), not the raw value. (Never log bearer tokens or raw `Authorization` headers either — see `project-service-dev` skill.)

---

## `logging-and-pii/non-context-or-non-slog-logging` — Important

**Pattern:** runtime service logging uses `fmt.Println` / `fmt.Printf` / `log.Print*`, or a non-`Context` slog call (`slog.Info` instead of `slog.InfoContext`). The non-`Context` variants drop the request-scoped attributes (`method`, `path`, `req_header_etag`, request id, principal) that middleware appends.

**Detect:** in runtime (non-test, non-script) Go files, grep for `fmt.Print*`, `log.Print*`/`log.Println`, and `slog.Info(`/`slog.Warn(`/`slog.Error(`/`slog.Debug(` without the `Context` suffix. The repo uses `slog.*Context(ctx, ...)` everywhere and `log.AppendCtx` for request-scoped fields.

**Empirical citation:** anchored in the repo's logging contract — `.claude/skills/project-service-dev/SKILL.md`: "Use `log/slog` only. Do not use `fmt.Println`, `fmt.Printf`, `log.Print*` ... Always use the `*Context` variants ... so the context attributes appended by middleware flow through." Health-endpoint logging discipline (`/livez` `/readyz` at DEBUG) was an explicit repo change (PR #19, "do not log /livez and /readyz requests").

**Failure message:** Runtime logging uses a non-slog or non-Context call — request-scoped log attributes are lost.

**Fix:** switch to `slog.*Context(ctx, msg, ...)` and attach request-scoped fields via `log.AppendCtx`. Keep `/livez` and `/readyz` at DEBUG; everything else at INFO.
