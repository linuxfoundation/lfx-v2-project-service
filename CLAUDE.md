# Claude Development Guide for LFX V2 Member Service

This is the index. Implementation guidance lives under
`.claude/skills/member-service-dev/references/` and owned service docs
live under `docs/`. Read this file first to understand what this service is,
then jump to the right reference for the work at hand.

> **Central LFX skills:**
> - `lfx-skills:lfx` for cross-repo tasks, "where does X live" questions, owner/peer repo routing, or missing checkouts.
> - `lfx-skills:lfx-platform-architecture` for platform composition, V2 service classes, write/read/access-check flows, NATS/KV ownership, and handoff points across FGA, indexer, query, Heimdall, OpenFGA, Helm, or ArgoCD.
> - For the post-commit review lifecycle, launch the shared `lfx-skills:lfx-general-code-reviewer` through the Agent tool after every pre-PR commit.
> - **Local skills:**
>   - `member-service-dev` auto-attaches on Go and service paths (`**/*.go`, `cmd/**`, `internal/**`, `pkg/**`, `gen/**`, `Makefile`) and owns Go conventions, Goa boundaries, NATS/KV cache and RPC rules, tests, formatting, and the Salesforce-integration callout.
>   - `member-add-endpoint` is the entry point for adding or changing any membership HTTP endpoint (Goa design, regen, handler, tests, Heimdall ruleset update).
>   - Before opening a PR, run the repo-local cycle in order: `/member-service-pr-readiness` for branch/commit shape, then `/member-service-preflight` for mechanical Go validation.
> - Repo-local docs own concrete subjects, payloads, contracts, chart values, and domain behavior. If the plugin is missing, install with `/plugin marketplace add linuxfoundation/lfx-skills` then `/plugin install lfx-skills@lfx-skills`.

## Project Overview

The LFX V2 Member Service is a RESTful API service that provides membership
data for the Linux Foundation's LFX platform. It exposes endpoints for
querying project-scoped tiers, memberships, and key contacts, plus write
endpoints (POST/PUT/DELETE) for managing key contacts. Data is sourced from
Salesforce via SOQL queries, with NATS Key-Value caches to minimise
round-trips.

### Key technologies

- Language: Go 1.24+
- API framework: Goa v3 (code generation framework)
- Messaging: NATS with JetStream for KV caching and RPC
- Storage: NATS Key-Value caches in front of Salesforce:
  `membership-cache` for SOQL and resolver results, and
  `member-service-cache` for sObject conditional-GET support
- Primary data source: Salesforce REST API (SOQL queries via
  `github.com/k-capehart/go-salesforce/v3`)
- Authentication: JWT with Heimdall middleware
- Authorization: OpenFGA for fine-grained access control
- Container: Chainguard distroless images
- Orchestration: Kubernetes with Helm charts

## Service Boundaries

This service is a Salesforce-backed read/write proxy for membership data.
Reads are currently SOQL-backed and cached in NATS KV. Key-contact writes
mutate Salesforce `Project_Role__c` records and invalidate the relevant
`membership-cache` entries. There is no PostgreSQL and no sync job.

**This service does NOT publish FGA or indexer messages.** It does not emit
to `lfx.fga-sync.*` or `lfx.index.*` today. Do not assume PostgreSQL,
sync-job, indexer-publish, or fga-sync-publish behavior here. If publishing
is added later, route via `lfx-v2-indexer-service` and `lfx-v2-fga-sync`
canonical contracts and add a top-level per-contract doc under `docs/`.

## Agent Guidance

Repo-owned guidance is split. Convention and workflow references live under
`.claude/skills/member-service-dev/references/`. Domain ownership docs
live under `docs/agent-guidance/`, while chart facts live at top level under
`docs/`. Go coding conventions themselves are inline in
`.claude/skills/member-service-dev/SKILL.md`.

- [`nats-messaging.md`](.claude/skills/member-service-dev/references/nats-messaging.md):
  NATS subjects, RPC contract (`lfx.member.project-id-map.lookup` request
  and response shape), outbound calls to project-service, and KV bucket
  inventory.
- [`development-workflow.md`](.claude/skills/member-service-dev/references/development-workflow.md):
  prerequisites, Make targets, Goa code generation, JWT and Heimdall flow,
  OpenFGA `project` type, Docker, CI, and error-mapping reference.
- [`salesforce-cache.md`](docs/agent-guidance/salesforce-cache.md): the
  `membership-cache` bucket key prefixes and TTLs, cache freshness states
  (`Fresh`, `Stale`, `Expired`, `Miss`), `member-service-cache` sObject cache
  behavior, and the `ProjectResolver` UID-to-SFID resolution chains.
- [`salesforce-integration.md`](docs/agent-guidance/salesforce-integration.md):
  service env vars, Salesforce credential flows, local-dev setup,
  Kubernetes deployment, Helm wiring, and common pitfalls.
- [`service-helm-chart.md`](docs/service-helm-chart.md):
  this repo's chart interface.

For NATS handlers and KV layout, the source under
`internal/infrastructure/nats/` is authoritative. This repo's own README
and source tree are the source of truth for the Salesforce-backed
membership cache model.

## Module Map

```text
cmd/member-api/               # Presentation Layer (HTTP entry point)
  design/                     # Goa API design specifications
  service/                    # Service handlers (implements Goa interfaces)
  http.go                     # HTTP server setup and middleware
  main.go                     # Application entry point
gen/                          # Generated code (DO NOT EDIT MANUALLY)
internal/
  domain/                     # Domain layer (auth, models, ports)
  infrastructure/             # Infrastructure layer
    auth/                     # JWT authentication (Heimdall)
    mock/                     # Mock repository for testing
    nats/                     # NATS KV caches and project RPC client
    project/                  # ProjectResolver implementation
    salesforce/               # Salesforce SOQL client and repositories
  middleware/                 # HTTP middleware
  service/                    # Business logic / use case orchestration
pkg/constants/                # Shared constants
charts/lfx-v2-member-service/ # Helm chart
```

### Data flow

```text
HTTP Request
    -> MembershipService (Goa handler)
    -> MemberReaderOrchestrator
    -> salesforce.MemberReader
       -> NATS KV cache (membership-cache)
          Fresh  -> return cached
          Stale  -> return cached + background refresh
          Expired/Miss -> Salesforce
       -> ProjectResolver.SFIDFromUID (project-scoped queries)
          -> NATS RPC -> project-service (get_slug)
          -> SOQL -> Salesforce (Project__c)
          -> KV cache write
       -> SOQL -> Salesforce REST API
       -> KV cache write
    -> response
```

See [`salesforce-cache.md`](docs/agent-guidance/salesforce-cache.md) for the
full resolver and cache contract.

## API Endpoints

All data endpoints are project-scoped under `/projects/{project_id}` where
`project_id` is the v2 project UUID.

| Method | Path | Description | OpenFGA Check |
|--------|------|-------------|---------------|
| GET | `/projects/{project_id}/tiers` | List membership tiers for a project | `auditor` on `project:{project_id}` |
| GET | `/projects/{project_id}/tiers/{tier_id}` | Get a specific tier | `auditor` on `project:{project_id}` |
| GET | `/projects/{project_id}/memberships` | List memberships for a project | `auditor` on `project:{project_id}` |
| GET | `/projects/{project_id}/memberships/{id}` | Get a specific membership | `auditor` on `project:{project_id}` |
| GET | `/projects/{project_id}/memberships/{id}/key_contacts` | List key contacts for a membership | `auditor` on `project:{project_id}` |
| GET | `/projects/{project_id}/memberships/{id}/key_contacts/{cid}` | Get a specific key contact | `auditor` on `project:{project_id}` |
| POST | `/projects/{project_id}/memberships/{id}/key_contacts` | Add a key contact | `writer` on `project:{project_id}` |
| PUT | `/projects/{project_id}/memberships/{id}/key_contacts/{cid}` | Update a key contact | `writer` on `project:{project_id}` |
| DELETE | `/projects/{project_id}/memberships/{id}/key_contacts/{cid}` | Remove a key contact | `writer` on `project:{project_id}` |
| GET | `/b2b_orgs` | Search B2B organizations (interim detour endpoint) | `auditor` on configured LF project |
| GET | `/b2b_orgs/{b2b_org_uid}/memberships` | List memberships for a B2B organization (interim detour endpoint) | `auditor` on configured LF project |
| GET | `/readyz` | Readiness probe | None |
| GET | `/livez` | Liveness probe | None |
| GET | `/debug/vars` | expvar JSON for port-forward/debug use; not exposed by HTTPRoute | None |
| GET | `/_memberships/openapi*.{json,yaml}` | OpenAPI spec files | None |

The legacy `/members/*` and `/memberships/*` endpoints return `410 Gone` with
a hint pointing to the replacement paths.

### Member search and filtering

The list endpoints accept a `filter` query parameter with semicolon-separated
`key=value` pairs:

```
GET /projects/{project_id}/memberships?filter=tier_uid=4c46585f-9f01-8bda-a0a5-f0c8eeef7fff
GET /projects/{project_id}/memberships?search_name=acme
```

| Filter Key | Match Type | Example |
|------------|------------|---------|
| `tier_uid` | Exact SOQL filter on Product2, decoded from tier UUID | `tier_uid=4c46585f-9f01-8bda-a0a5-f0c8eeef7fff` |
| `company_name` | Case-insensitive contains after the SOQL page is fetched | `company_name=Acme` |
| `project_slug` | Case-insensitive exact after fetch | `project_slug=kubernetes` |
| `tier_name` | Case-insensitive contains after fetch | `tier_name=Gold` |
| `status` | Case-insensitive exact after fetch; queries already return active memberships only | `status=Active` |
| `tier` | Case-insensitive exact after fetch | `tier=Gold` |
| `year` | Exact after fetch | `year=2026` |

## Where to make each change

| Task | Read |
| --- | --- |
| Add or change a membership HTTP endpoint | Local `member-add-endpoint` skill (procedural recipe) plus [`development-workflow.md`](.claude/skills/member-service-dev/references/development-workflow.md) (reference) |
| Go conventions, Goa boundaries, NATS/KV cache and RPC rules, tests, format, license | Local `member-service-dev` skill (auto-attaches on Go paths) |
| Check PR branch/commit shape before opening a PR | Local `member-service-pr-readiness` skill, then local `member-service-preflight` |
| Run mechanical before-PR validation | Local `member-service-preflight` skill after `member-service-pr-readiness` |
| Change cache TTLs or freshness behavior | [`salesforce-cache.md`](docs/agent-guidance/salesforce-cache.md) |
| Change a NATS subject or RPC contract | [`nats-messaging.md`](.claude/skills/member-service-dev/references/nats-messaging.md) |
| Wire up new env vars or auth flow | [`salesforce-integration.md`](docs/agent-guidance/salesforce-integration.md) |
| Change chart values or templates | [`service-helm-chart.md`](docs/service-helm-chart.md) |
| Add a Goa design change | [`development-workflow.md`](.claude/skills/member-service-dev/references/development-workflow.md) ("Goa code generation") |
| Reason about current state vs target state (FGA/indexer graduation plan) | [`ARCHITECTURE.md`](ARCHITECTURE.md) |

## Work cycle — post-commit and pre-PR reviews

> **CRITICAL — while the branch is pre-PR, post-commit review is mandatory.** After every commit on the local branch, launch the `lfx-skills:lfx-general-code-reviewer` subagent via the Agent tool (`subagent_type: lfx-skills:lfx-general-code-reviewer`, `run_in_background: true`) — then keep working while it runs. If Claude displays plugin agents without the `lfx-skills:` namespace, use the equivalent displayed general reviewer name. Before opening a PR, every running review must return clean (or remaining findings explicitly documented as trade-offs), the **full-branch sweep** must run clean if the branch has more than one commit (`branch` arg), AND `/member-service-pr-readiness` must clear every Critical finding before `/member-service-preflight` runs.
>
> **Once the PR is open, do NOT invoke the general reviewer on iteration commits.** CodeRabbit + Copilot auto-trigger on every push and own the audit surface from that point. The general reviewer is pre-PR insurance only.

### Post-commit (pre-PR phase, after every commit, asynchronous)

1. **Commit your work.** `git commit --signoff -S`. Do not wait for any prior review to finish.
2. **Immediately launch the general reviewer subagent.** Use `subagent_type: lfx-skills:lfx-general-code-reviewer`, `run_in_background: true`.
3. **Post-commit mode prompt (exact):** `target repo: lfx-v2-member-service\n\nReview the latest commit.` Append `extra: <focus>` on a new line only when there is a priority hint to add. Do NOT pass `branch` here. If this work cycle is launched from the LFX workspace parent, the `target repo:` line is required so the reviewer operates in this repo.
4. **Keep working.** Start the next commit while the reviewer runs. Do not block on it.
5. **When the review returns:** roll every Critical finding and every reasonable Important finding into the next commit.

### Pre-PR (drain the queue, sweep cumulative state, then open)

When the work is done and no more code commits are planned:

1. **Wait for every running review to complete.**
2. **If any returned review flags Critical or reasonable Important:** add a fix commit, launch the general reviewer again on the new state, wait, and loop until clean or explicitly documented as a trade-off.
3. **Full-branch sweep — only if the branch has more than one commit.** Launch `lfx-skills:lfx-general-code-reviewer` again with prompt **`target repo: lfx-v2-member-service\nbranch\n\nReview the branch's diff against origin/main.`**. Address any new findings, then re-run the sweep until clean.
4. **Run `/member-service-pr-readiness`** for branch and commit shape only.
5. **Run `/member-service-preflight`** for mechanical Go validation and PR summary.
6. **Only then push and open the PR.**

### Post-PR iteration (responding to bot feedback on an open PR)

1. Wait for CodeRabbit + Copilot to comment after each push.
2. Triage every Critical and reasonable Important finding against current code.
3. Roll fixes into a `fix(review): ...` commit.
4. Push. Repeat until clean.

## Resources

See also [`README.md`](README.md) for human-facing onboarding.

- [Goa Framework Docs](https://goa.design/docs/)
- [NATS JetStream Docs](https://docs.nats.io/jetstream)
- [OpenFGA Docs](https://openfga.dev/docs)
- [Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [go-salesforce library](https://github.com/k-capehart/go-salesforce)
