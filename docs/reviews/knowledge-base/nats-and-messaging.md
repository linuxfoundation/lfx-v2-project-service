<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# NATS and messaging

Patterns around this service's NATS publishing, request/reply, and the
background-publish goroutines used by the sub-resource operations (links,
folders, documents) and the project subscriber. These are data-integrity and
contract patterns: a publish that silently never completes leaves the indexer
or downstream services out of sync while the HTTP caller sees a 2xx. The
`xSync` / `X-Sync` semantics and the detached-context fix recur across every
sub-resource operation, so they are weighted high despite living in only a few
PRs.

**Read when:** any file under `internal/service/*_operations.go`,
`internal/service/project_subscriber.go`,
`internal/infrastructure/nats/message.go`, `internal/infrastructure/nats/repository.go`,
`internal/domain/message.go`, or `pkg/events/**` changed; or any new goroutine
that publishes a NATS message.

---

## `nats-and-messaging/goroutine-captures-request-ctx` — Critical

**Pattern:** a background goroutine that publishes an indexer/access/event message captures the request-scoped `ctx`. When the HTTP request completes, that `ctx` is canceled and the publish is interrupted or dropped — the endpoint already returned 2xx, so indexing/access silently never happens.

**Detect:** find `go func() { ... }()` (or `g.Go(...)`) blocks that call `SendIndexerMessage` / `SendAccessMessage` / `Publish*` and close over the handler's `ctx`. The correct repo pattern is to derive a detached context first: `bgCtx := context.WithoutCancel(ctx)` and use `bgCtx` inside the goroutine.

**Empirical citation:** PR #60 `internal/service/document_operations.go:110` (CodeRabbit) — "Both locations ... spawn goroutines that capture `ctx`, which is canceled when the HTTP request completes. This causes indexer sends and blob deletions to be interrupted or dropped even though the response indicates success. Use `context.Background()` or `context.WithTimeout()` for background operations". Fixed across `link_operations.go`, `folder_operations.go`, `document_operations.go` with `bgCtx := context.WithoutCancel(ctx)` (still present in current code).

**Failure message:** Background publish goroutine captures the request-scoped ctx — request teardown cancels the publish while the endpoint already returned 2xx.

**Fix:** derive `bgCtx := context.WithoutCancel(ctx)` (optionally `context.WithTimeout`) before spawning the goroutine and use `bgCtx` for the publish, mirroring `link_operations.go` / `folder_operations.go` / `document_operations.go`.

---

## `nats-and-messaging/xsync-not-honored` — Critical

**Pattern:** an operation takes an `xSync bool` (from the `X-Sync` header) but always publishes the indexer/event message in a goroutine, so `xSync=true` still returns before the publish completes and publish errors are never surfaced. This breaks the documented synchronous-operation contract.

**Detect:** in `*_operations.go` functions that accept `xSync bool`, check that when `xSync` is true the `SendIndexerMessage` call is made inline (so the request blocks on the ack and the error is returned) and only spawned in a goroutine when `xSync` is false. Flag any path that publishes in a goroutine unconditionally despite an `xSync` parameter.

**Empirical citation:** PR #60 `internal/service/document_operations.go:110` (CodeRabbit) — "The document indexer publish is always executed in a goroutine, so `xSync=true` still returns before the publish completes. To honor X-Sync semantics, publish inline when `xSync` is true (or use an errgroup) and only fire-and-forget when `xSync` is false." Recurred on `CreateFolder`, `DeleteFolder`, `DeleteLink`, `DeleteDocument` in the same PR; all fixed (`if xSync { ... inline ... }` present in current code).

**Failure message:** Operation accepts `xSync` but always publishes async — synchronous requests return before the publish completes and errors are swallowed.

**Fix:** when `xSync` is true, call `SendIndexerMessage` inline and return its error to the caller; only spawn the detached-context goroutine when `xSync` is false. Follow the existing `if xSync { ... } else { go func(){ ... }() }` shape.

---

## `nats-and-messaging/request-without-timeout` — Important

**Pattern:** a NATS request/reply helper (`SendEmailRequest`, `SendInviteRequest`) accepts a `ctx` but does not bound the call with a timeout, so a non-responding peer service can block the subscription callback indefinitely. The repo's convention is a bounded `defaultRequestTimeout`.

**Detect:** in `internal/infrastructure/nats/message.go`, for any request/reply call (`requestMessage`, `RequestMsgWithContext`, `Request`), confirm a timeout is applied — either `defaultRequestTimeout` (the repo constant) or a `context.WithTimeout` derived from the inbound ctx. Flag request paths that pass a long-lived service ctx with no deadline.

**Empirical citation:** PR #65 `internal/infrastructure/nats/message.go:217` (Copilot) — "SendEmailRequest uses RequestMsgWithContext with whatever ctx is passed, but no timeout is enforced ... a non-responding email service could cause this call (and the NATS subscription callback) to block indefinitely. Wrap the request with a context.WithTimeout (e.g., defaultRequestTimeout) or use the existing Request(timeout) helper." The repo defines `const defaultRequestTimeout = time.Second * 10` for exactly this.

**Failure message:** NATS request/reply call has no timeout — a non-responding peer can block the subscription callback indefinitely.

**Fix:** bound the request with `defaultRequestTimeout` (via `requestMessage`/`Request`) or a `context.WithTimeout` derived from the inbound context.

---

## `nats-and-messaging/publish-before-storage` — Critical

**Pattern:** an indexer / FGA-access / project-event message is published before the NATS KV storage write succeeds. If the write then fails, downstream services index or grant access to data that was never persisted.

**Detect:** in operation functions, verify the storage write (KV `Put`/`Create`/`Update`) returns successfully before any `SendIndexerMessage` / `SendAccessMessage` / `SendProjectEventMessage`. Flag any publish that runs before the write or in a path that can be reached when the write failed.

**Empirical citation:** anchored in the repo's own contract — `.claude/skills/project-service-dev/references/nats-messaging.md` and `SKILL.md`: "Publish indexer and FGA messages only after the storage write succeeds, not before." `docs/indexer-contract.md` and `docs/fga-contract.md` define the per-resource publish triggers this ordering protects. (Cost-of-miss: data-integrity.)

**Failure message:** Message published before the storage write succeeded — downstream services may index/authorize unpersisted data.

**Fix:** order the storage write first and only publish after it returns nil. If publishing must happen for several resources, gate every publish behind the successful write.
