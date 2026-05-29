# Claude Development Guide for LFX V2 Project Service

This guide provides essential information for Claude instances working with the LFX V2 Project Service codebase. It includes build commands, architecture patterns, and key technical decisions.

> **Central LFX skills:**
> - `/lfx-skills:lfx`: cross-repo routing, "where does X live" questions, owner/peer repos, missing checkouts.
> - `/lfx-skills:lfx-platform-architecture`: platform composition, V2 service classes (native, wrapper, proxy, platform), write/read/access-check flows, cross-service responsibilities, NATS/KV ownership, handoff points across Self Serve, FGA, indexer, query, Heimdall, OpenFGA, Helm, ArgoCD.
>
> **Repo-local project-service skills and docs:**
> - `/project-service-dev` at `.claude/skills/project-service-dev/` auto-attaches on Go and service paths and owns logging, errors, request context, pagination, generated-code boundary, NATS/KV publishing, tests, formatting, linting, and license headers for this repo.
> - `/project-service-pr-readiness` checks pre-PR shape only: branch/JIRA/conventional commits/rebase/DCO+GPG/diff size/protected files.
> - `/project-service-preflight` runs the mechanical Go pre-PR pipeline after readiness: working tree, license, formatting, lint, build, tests, protected files, commit verification, generated-code freshness, and change summary.
> - Repo-local docs under `docs/` own concrete subjects, payloads, emitted contracts, and domain behavior; this repo's chart owns project-service Helm values and templates.
> - If the central plugin is missing, install with `/plugin marketplace add linuxfoundation/lfx-skills` then `/plugin install lfx-skills@lfx-skills`.

## Project Overview

The LFX V2 Project Service is a RESTful API service that manages projects within the Linux Foundation's LFX platform. It provides CRUD operations for projects with built-in authorization and audit capabilities.

### Key Technologies

- **Language**: Go 1.24.x (`go.mod` declares `go 1.24.0`)
- **API Framework**: Goa v3 (code generation framework)
- **Messaging**: NATS with JetStream for event-driven architecture
- **Storage**: NATS Key-Value stores (no traditional database)
- **Authentication**: JWT with Heimdall middleware
- **Authorization**: OpenFGA for fine-grained access control
- **Container**: Chainguard distroless images
- **Orchestration**: Kubernetes with Helm charts

## Architecture Overview

The service follows **Clean Architecture** principles with clear separation of concerns:

```text
.github/                    # CI/CD workflow files for Github Actions

api/                        # API contracts
└── project/
    └── v1/
        ├── design/         # Goa API design specifications
        └── gen/            # Generated code (tracked; produced by make apigen)

charts/                     # Helm charts containing kubernetes template files for deployments

cmd/project-api/            # Presentation Layer (HTTP/NATS entry point)
├── service*.go            # HTTP and NATS handlers
└── main.go                # Application entry point

internal/                   # Core business logic
├── domain/                # Domain layer (interfaces, models, errors)
│   └── models/           # Domain entities
├── service/              # Service layer (business logic)
├── infrastructure/       # Infrastructure layer
│   ├── auth/            # JWT authentication
│   └── nats/            # NATS repository implementation
└── middleware/          # HTTP middleware

pkg/                    # Shared packages across services
└── constants/          # Shared constants

scripts/                # Scripts for services and miscellaneous tasks
```

### Key Design Principles

1. **Database Independence**: Repository interfaces allow switching storage backends
2. **Testability**: Each layer can be tested in isolation using mocks
3. **Event-Driven**: All data changes trigger NATS messages for downstream services
4. **Separation of Concerns**: Clear boundaries between layers

## Development Workflow

### Agent Guidance

Repo-owned guidance lives across the local skill, contract docs, and chart:

- `.claude/skills/project-service-dev/` (auto-attached path-scoped skill) owns Go coding conventions: generated-code boundary, logging, errors, request context, pagination, NATS/KV publishing, tests, formatting, linting, license headers. SKILL.md plus three references (`go-conventions.md`, `goa-and-codegen.md`, `nats-messaging.md`).
- `.claude/skills/project-service-pr-readiness/` owns the pre-PR shape check only: branch name, JIRA reference, conventional commits, rebase status, DCO plus GPG signing, diff size, and project-service protected files.
- `.claude/skills/project-service-preflight/` owns the mechanical pre-PR validation after readiness: working tree, license headers, Goa generation, formatting, lint, build, tests, protected files, commit verification, and PR change summary.
- `docs/indexer-contract.md` and `docs/fga-contract.md` are the authoritative
  per-resource references for what this service emits. Update them in the same
  PR as any publisher change.
- Service-local chart facts live in this repo's Helm chart under
  `charts/lfx-v2-project-service/`; shared chart conventions live in
  `lfx-v2-helm/docs/service-chart-patterns.md`.

Consumed cross-repo contracts:

- Generic FGA envelope: `lfx-v2-fga-sync/docs/fga-sync-contract.md`
- Generic indexer event contract: `lfx-v2-indexer-service/docs/indexer-contract.md`
- OpenFGA model: `lfx-v2-helm/charts/lfx-platform/templates/openfga/model.yaml`
- Service chart conventions: `lfx-v2-helm/docs/service-chart-patterns.md`

Use `/lfx-skills:lfx` if an owner repo is missing locally, the path has moved,
or the task needs additional peer repos.

Reusable V2 service architecture and native-service classification live in `/lfx-skills:lfx-platform-architecture`. This repo is a living native-service example, not the central owner of native-service architecture.

## Work cycle — post-commit and pre-PR reviews

> **CRITICAL — while the branch is pre-PR, post-commit review is mandatory.** After every commit on the local branch, launch all three reviewer subagents via the Agent tool in parallel: `lfx-skills:lfx-general-code-reviewer`, `lfx-skills:lfx-project-service-code-reviewer`, AND `lfx-skills:lfx-project-service-learnings-reviewer` (each with `run_in_background: true`) — then keep working while they run. If Claude displays plugin agents without the `lfx-skills:` namespace, use the equivalent displayed general, project-service, and learnings reviewer names. Before opening a PR, every running review must return clean (or remaining findings explicitly documented as trade-offs), the **full-branch sweep** must run clean if the branch has more than one commit (`branch` arg), AND `/project-service-pr-readiness` must clear every Critical finding before `/project-service-preflight` runs.
>
> **Once the PR is open, do NOT invoke these reviewers on iteration commits.** CodeRabbit + Copilot auto-trigger on every push and own the audit surface from that point. The general, project-service, and learnings reviewers are pre-PR insurance only.

### Post-commit (pre-PR phase, after every commit, asynchronous)

1. **Commit your work.** `git commit -s -S`. Do not wait for any prior review to finish.
2. **Immediately launch all three reviewer subagents in parallel.** Use `subagent_type: lfx-skills:lfx-general-code-reviewer`, `subagent_type: lfx-skills:lfx-project-service-code-reviewer`, and `subagent_type: lfx-skills:lfx-project-service-learnings-reviewer`, each with `run_in_background: true`.
3. **Post-commit mode prompt for all three reviewers (exact):** `target repo: lfx-v2-project-service\n\nReview the latest commit.` Append `extra: <focus>` on a new line only when there is a priority hint to add. Do NOT pass `branch` here. If this work cycle is launched from the LFX workspace parent, the `target repo:` line is required so all three reviewers operate in this repo.
4. **Keep working.** Start the next commit while the reviewers run. Do not block on them.
5. **When reviews return:** roll every Critical finding and every reasonable Important finding into the next commit.

### Pre-PR (drain the queue, sweep cumulative state, then open)

When the work is done and no more code commits are planned:

1. **Wait for every running review to complete.**
2. **If any returned review flags Critical or reasonable Important:** add a fix commit, launch all three reviewers again on the new state, wait, and loop until clean or explicitly documented as a trade-off.
3. **Full-branch sweep — only if the branch has more than one commit.** Launch `lfx-skills:lfx-general-code-reviewer`, `lfx-skills:lfx-project-service-code-reviewer`, and `lfx-skills:lfx-project-service-learnings-reviewer` again with prompt **`target repo: lfx-v2-project-service\nbranch\n\nReview the branch's diff against origin/main.`**. Address any new findings, then re-run the sweep until clean.
4. **Run `/project-service-pr-readiness`** for branch and commit shape only.
5. **Run `/project-service-preflight`** for mechanical Go validation and PR summary.
6. **Only then push and open the PR.**

### Post-PR iteration (responding to bot feedback on an open PR)

1. Wait for CodeRabbit + Copilot to comment after each push.
2. Triage every Critical and reasonable Important finding against current code.
3. Roll fixes into a `fix(review): ...` commit.
4. Push. Repeat until clean.

### Prerequisites

```bash
# Install Go 1.24.x
# Install pinned repo tooling, including Goa v3.22.6 and golangci-lint
make deps
```

### Common Development Tasks

#### 1. Generate API Code (REQUIRED after design changes)

```bash
make apigen
# or directly: goa gen github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/design -o api/project/v1
```

#### 2. Build the Service

```bash
make build
```

#### 3. Run Tests

```bash
make test              # Run unit tests
make test-verbose      # Verbose output
make test-coverage     # Generate coverage report
```

#### 4. Run the Service Locally

```bash
# Basic run
make run

# With debug logging
make debug

# With custom flags (direct go run)
go run ./cmd/project-api -d -p 8080
```

#### 5. Lint and Format Code

```bash
make fmt    # Format code
make lint   # Run golangci-lint
make check  # Check format and lint without modifying
```

## Code Generation (Goa Framework)

The service uses Goa v3 for API code generation. This is **critical** to understand:

1. **Design First**: API is defined in `api/project/v1/design/` files
2. **Generated Code**: Running `make apigen` generates to `api/project/v1/gen/`:
   - HTTP server/client code
   - Service interfaces
   - OpenAPI specifications
   - Type definitions

   Generated files under `api/project/v1/gen/` are tracked in git. Include them
   in the same change as the design file updates; never hand-edit them.
3. **Implementation**: You implement the generated interfaces in `cmd/project-api/service*.go` files

### Adding New Endpoints

See the auto-attached `project-service-dev` skill (sections "Generated code boundary" and "References" pointing at `references/goa-and-codegen.md`) for the design-first recipe. Repo-specific note: any design change that affects authorization must also update `charts/lfx-v2-project-service/templates/ruleset.yaml` in the same PR.

## NATS Messaging Patterns

Subjects, KV bucket inventory, optimistic-locking pattern, and publish order live in `.claude/skills/project-service-dev/references/nats-messaging.md` (auto-attached). The generic FGA envelope lives in `lfx-v2-fga-sync/docs/fga-sync-contract.md`, and this repo's emitted FGA data lives in `docs/fga-contract.md`. Read them before changing subjects, envelopes, or KV layout.

## Testing Patterns

See the auto-attached `project-service-dev` skill ("Tests" section) for test framework, mock layout, and table-driven conventions.

## Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | HTTP listen port | 8080 | No |
| `NATS_URL` | NATS server URL | nats://localhost:4222 | No |
| `LOG_LEVEL` | Log level | info | No |
| `JWKS_URL` | JWT verification endpoint | - | No |
| `AUDIENCE` | JWT audience | lfx-v2-project-service | No |
| `JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL` | Mock auth for local dev | - | No |
| `SKIP_ETAG_VALIDATION` | Skip `If-Match` revision checks for local development | false | No |

## Authorization (OpenFGA)

When deployed, the service uses OpenFGA for authorization:

- **GET /projects** - No check (public list)
- **POST /projects** - Requires `writer` on parent (if specified)
- **GET /projects/:id** - Requires `viewer` on project
- **GET /projects/:id/settings** - Requires `auditor` on project
- **PUT /projects/:id** - Requires `writer` on project
- **PUT /projects/:id/settings** - Requires `writer` on project
- **DELETE /projects/:id** - Requires `owner` on project

## Local Development Setup

There are two main development setup options documented in DEVELOPMENT.md:

### Option A: Full Platform Setup

For integration testing with complete LFX stack:

- Install lfx-platform Helm chart (includes NATS, Heimdall, OpenFGA, Authelia, Traefik)
- Use `make helm-install-local` with values.local.yaml
- Full authentication and authorization enabled

### Option B: Minimal Setup

For rapid development:

```bash
# Just run NATS locally
docker run -d -p 4222:4222 nats:latest -js

# Create KV stores
nats kv add projects --history=20 --storage=file
nats kv add project-settings --history=20 --storage=file
nats kv add project-links --history=20 --storage=file
nats kv add project-folders --history=20 --storage=file
nats kv add project-documents-metadata --history=20 --storage=file
nats object add project-documents --storage=file

# Run service with mock auth
export NATS_URL=nats://localhost:4222
export JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL=test-user
make run
```

**Security Note**: Option B bypasses all authentication/authorization - only for local development.

### New Helm Commands

- `make helm-install-local`: Install with local values
- `make helm-restart`: Restart deployment pod
- `make docker-build`: Build Docker image

## Docker Build

```bash
# Build from repository root
docker build -t lfx-v2-project-service:latest .

# The Dockerfile uses:
# - Chainguard Go image for building
# - Chainguard static image for runtime (distroless)
# - Multi-stage build for minimal image size
```

## Kubernetes Deployment

```bash
# Install Helm chart
helm install lfx-v2-project-service ./charts/lfx-v2-project-service/ -n lfx

# Update deployment
helm upgrade lfx-v2-project-service ./charts/lfx-v2-project-service/ -n lfx

# View generated manifests
helm template lfx-v2-project-service ./charts/lfx-v2-project-service/ -n lfx
```

### Helm Configuration

- OpenFGA can be disabled for local development
- NATS KV buckets are created automatically
- Heimdall middleware handles JWT validation
- Traefik IngressRoute for HTTP routing

## CI/CD Pipeline

GitHub Actions workflows:

- **mega-linter.yml**: Comprehensive linting (Go, YAML, Docker, etc.)
- **project-api-build.yml**: Build and test on PRs
- **license-header-check.yml**: Ensure proper licensing

### PR Checks

1. Generate API code
2. Build binary
3. Run unit tests
4. Lint with MegaLinter

## Common Pitfalls and Solutions

The generated-code boundary, ETag/If-Match handling, NATS publishing order, and request context propagation are covered by the auto-attached `project-service-dev` skill. Repo-specific validation rules to keep in mind:

- **Slug format**: must match `^[a-z][a-z0-9_\-]*[a-z0-9]$`.
- **Parent project**: `parent_uid` must be the empty string or a valid UUID.
- **NATS connectivity**: the service refuses to start if `NATS_URL` is wrong or NATS is down. Confirm NATS is reachable before running the service.

## Mock Data Loading

Use the provided script to load test data:

```bash
cd scripts/load_mock_data
go run main.go -bearer-token "your-token" -num-projects 10
```

## Key Implementation Details

### 1. Project Data Split

Projects are split into two parts for access control:

- **Base**: Core project info (stored in `projects` KV)
- **Settings**: Sensitive settings (stored in `project-settings` KV)

### 2. Message Publishing

Every data modification publishes NATS messages:

- Index messages for search service
- Access control updates for authorization service

### 3. Request Context

Request-context keys (`request-id`, `principal`, `authorization`, `etag`) and the typed-key pattern live in the auto-attached `project-service-dev` skill. The full constant set is in `pkg/constants/http.go`.

### 4. Error Handling

Domain sentinels and HTTP mapping live in the auto-attached `project-service-dev` skill. The sentinels themselves are declared in `internal/domain/errors.go` and mapped to HTTP in `cmd/project-api/service_endpoint_project.go::handleError`. New errors must be added to both.

## Debugging Tips

1. **Enable Debug Logging**: Run with `-d` flag or set `LOG_LEVEL=debug`
2. **Check NATS Messages**: Use `nats sub "lfx.>"` to monitor all messages
3. **Verify KV Data**: Use `nats kv get projects <uid>` to check stored data
4. **HTTP Traces**: Middleware logs all requests with timing
5. **Generated Code**: Check `api/project/v1/gen/` directory for Goa-generated interfaces

## Documentation Structure

Repo documentation follows the routing model described in the central-skills block at the top of this file. Where each kind of guidance lives:

- **README.md**: project overview, quick start, API endpoints, deployment setup.
- **DEVELOPMENT.md**: human developer guide with build, test, and deploy workflows.
- **CLAUDE.md** (this file): high-level routing for AI assistants and repo-specific facts that don't belong in an auto-attached skill.
- **SECURITY.md**: vulnerability reporting and security policy for this repo.
- **`.claude/skills/project-service-dev/`**: auto-attached path-scoped skill that owns Go coding conventions (logging, errors, request context, pagination, generated-code boundary, NATS/KV publishing, tests, formatting, linting, license headers). SKILL.md plus references under `references/`.
- **`.claude/skills/project-service-pr-readiness/`**: repo-local shape check before PR creation.
- **`.claude/skills/project-service-preflight/`**: repo-local mechanical Go preflight after readiness.
- **`docs/indexer-contract.md`, `docs/fga-contract.md`**: authoritative per-resource references for what this service emits. Update in the same PR as any publisher change.
- **`charts/lfx-v2-project-service/`**: service-local Helm templates and values. Shared chart conventions live in `lfx-v2-helm/docs/service-chart-patterns.md`.
- **Central LFX skills** (`/lfx-skills:lfx`, `/lfx-skills:lfx-platform-architecture`): cross-repo routing, platform composition, V2 service classes, write/read/access-check flows.

Avoid duplicating content across these surfaces. Cross-reference instead.

## Contributing Guidelines

1. **Design First**: Update Goa design files before implementation
2. **Test Coverage**: Write comprehensive unit tests
3. **Mock External Deps**: Use mocks for repository and message builder
4. **Follow Clean Architecture**: Respect layer boundaries
5. **Update Docs**: Keep documentation current and avoid duplication
6. **Lint Clean**: Ensure `make check` passes

## Resources

- [Goa Framework Docs](https://goa.design/docs/)
- [NATS JetStream Docs](https://docs.nats.io/jetstream)
- [OpenFGA Docs](https://openfga.dev/docs)
- [Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
