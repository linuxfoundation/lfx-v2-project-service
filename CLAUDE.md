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
> - `/project-service-code-reviewer` at `.claude/skills/project-service-code-reviewer/` and `/project-service-learnings-reviewer` at `.claude/skills/project-service-learnings-reviewer/` are the repo-owned review brains loaded by the pre-PR review subagents — not skills a developer invokes by hand. Guidance names them only by these `/...` forms; the `SKILL.md` frontmatter and the directory paths under `.claude/skills/` are written without the slash. A child that cannot load one of these skills from the Skill tool reads that skill's `SKILL.md` file directly.
> - Repo-local docs under `docs/` own concrete subjects, payloads, emitted contracts, and domain behavior; this repo's chart owns project-service Helm values and templates.
> - If the central plugin is missing, install with `/plugin marketplace add linuxfoundation/lfx-skills` then `/plugin install lfx-skills@lfx-skills`.

## Developer Standards

These rules apply to all contributors and AI agents working in this repo. Read this section first — it governs commit signing, message format, PR shape, and data hygiene across all work.

### Commit Signing

Every commit must carry both a GPG signature and a DCO sign-off:

```bash
git commit -s -S
# -s  adds: Signed-off-by: Your Name <your@email.com>
# -S  attaches a GPG signature
```

**One-time git config setup:**

```bash
# Tell git which key to use (replace with your key ID from the command below)
gpg --list-secret-keys --keyid-format LONG
git config --global user.signingkey <YOUR_KEY_ID>
git config --global commit.gpgsign true
```

For GPG key generation and uploading your public key to GitHub, see the [GitHub GPG documentation](https://docs.github.com/en/authentication/managing-commit-signature-verification).

If you forget the sign-off on the last commit, fix it with:

```bash
git commit --amend -s
```

### Commit Message Format

Follow Angular conventional commits. Jira tickets are optional — include one when the work has a known ticket, anywhere in the commit message (subject or body). The preferred placement is at the end of the subject line.

```text
type(scope): summary [LFXV2-NNNN]
```

| Part | Rule |
|---|---|
| `type` | Required: `feat` \| `fix` \| `docs` \| `test` \| `refactor` \| `chore` \| `build` \| `ci` \| `perf` \| `style` \| `revert` |
| `(scope)` | Optional but recommended; lowercase, e.g. `(project)`, `(nats)`, `(service)` |
| `!` | Optional breaking-change marker, placed after scope: `feat(api)!:` |
| `summary` | Lowercase first letter, imperative mood, no trailing period; max 72 chars total on first line |
| `[LFXV2-NNNN]` | Optional; include when a Jira ticket exists — omit entirely if there is none |

**Examples:**

```text
feat(project): add slug validation on create [LFXV2-1234]
fix(nats): handle stale KV entry on concurrent update [LFXV2-5678]
refactor(service): extract email renderer into dedicated package
docs: update NATS subject table in README
chore: bump golangci-lint to v1.62
feat(api)!: remove deprecated slug endpoint [LFXV2-9999]
```

The `commit-msg` hook enforces type format, lowercase summary, no trailing period, 72-char limit, and DCO sign-off (see Pre-commit Hooks below). Ticket inclusion and placement are conventions, not mechanically enforced.

### Pull Request Standards

**Title** — same pattern as the commit message:

```text
type(scope): summary [LFXV2-NNNN]
```

**Required description sections:**

```markdown
## Summary
What changed and why (2–4 bullet points).

## Ticket
[LFXV2-NNNN](https://linuxfoundation.atlassian.net/browse/LFXV2-NNNN)
*(Omit this section entirely if there is no associated Jira ticket.)*

## Changes
- Bullet describing each meaningful change made in this PR.

## API Changes
*(Required when the PR touches `api/`, `cmd/project-api/service_endpoint_*.go`,
or Goa design files. Omit this section entirely otherwise.)*

| Endpoint | Method | Change Type | Before | After | Breaking? |
|---|---|---|---|---|---|
| `/projects/:id` | PUT | New field | — | `display_name` | No |
```

### No PII in Source

**No production data may appear anywhere in committed files** — code, tests, comments, or documentation. This covers real names, email addresses, organization names, user IDs, and domain names from production or staging environments.

Approved fake-data conventions for tests and mocks:

| Data type | Approved pattern |
|---|---|
| Names | `Test User`, `Alice Example`, `Bob Fixture` |
| Emails | `*@example.com` — e.g. `alice@example.com` |
| UUIDs | Sequential: `00000000-0000-0000-0000-000000000001` |
| Orgs | `Test Org`, `Example Foundation` |
| Domains | `example.com`, `test.invalid` |

Real-looking names, corporate domains, or UUIDs that appear to come from production data must not appear even if slightly modified or "anonymized." When in doubt, make the data unmistakably fictional.

### Pre-commit Hooks

Hooks live in `.githooks/` and are installed by `make deps` (also available standalone as `make hooks`). Two hooks run on every commit:

**`pre-commit`** (runs before the message is written):
- Auto-formats staged `.go` files with `gofmt` and re-stages them
- Checks license headers across all tracked source files

**`commit-msg`** (runs after the message is written):
- Validates Angular conventional commit format
- Rejects placeholder Jira tickets (`[LFXV2-0000]`)
- Verifies the `Signed-off-by:` DCO trailer is present

If a hook blocks your commit, fix the issue and re-run `git commit`. To amend a missing sign-off: `git commit --amend -s`.

## Project Overview

The LFX V2 Project Service is a RESTful API service that manages projects within the Linux Foundation's LFX platform. It provides CRUD operations for projects with built-in authorization and audit capabilities.

### Key Technologies

- **Language**: Go 1.25+
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
        └── gen/            # Generated code (gitignored)

charts/                     # Helm charts containing kubernetes template files for deployments

cmd/project-api/            # Presentation Layer (HTTP entry point, Goa endpoint adapters)
├── service_endpoint_*.go  # Goa endpoint adapters (project, link, folder, document)
├── http.go                # HTTP server wiring
└── main.go                # Application entry point, NATS subscription wiring

internal/                   # Core business logic
├── domain/                # Domain layer (interfaces, models, errors, mocks)
│   └── models/           # Domain entities (project, link, folder, document)
├── service/               # Service layer (business logic, NATS RPC handlers, event subscriber)
│   ├── *_operations.go   # Per-resource business orchestration
│   ├── project_handlers.go    # Inbound NATS request/reply RPC handlers
│   ├── project_subscriber.go  # Inbound NATS event subscribers (settings updates, invite acceptance)
│   ├── document_subscriber.go # Inbound NATS event subscribers (document/link created notifications)
│   ├── converters.go     # Domain ↔ Goa ↔ pkg/events wire-type converters; ProjectProjection for multi-output fan-out
│   ├── user_resolver.go  # UserResolver: centralised user identity lookup (JWT, auth service, fallback)
│   ├── notification_dispatcher.go # NotificationDispatcher: role-change email/invite orchestration
│   └── email/            # Email template rendering (one file per email type)
└── infrastructure/        # Infrastructure layer
    ├── auth/             # JWT authentication
    ├── log/              # Structured logging helpers (AppendCtx, InitStructureLogConfig)
    ├── middleware/        # HTTP middleware (auth, request ID, body limit, logger)
    └── nats/             # NATS repository, object store, message builder, user reader

pkg/                    # Shared packages across services
├── constants/          # Shared constants (NATS subjects, KV buckets, HTTP, access control)
└── events/             # NATS event wire types consumed by other services

scripts/                # Scripts for services and miscellaneous tasks
```

### Key Design Principles

1. **Database Independence**: Repository interfaces allow switching storage backends
2. **Testability**: Each layer can be tested in isolation using mocks
3. **Event-Driven**: All data changes trigger NATS messages for downstream services
4. **Separation of Concerns**: Clear boundaries between layers

## Development Workflow

### Prerequisites

```bash
# Install Go 1.25+
# Install Goa framework
go install goa.design/goa/v3/cmd/goa@v3.22.6

# Install linting tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
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

## Pre-PR review

Before a PR exists, local review uses the same three reviewers in two modes: **post-commit review** while development continues, and one **full-branch review** immediately before opening the PR.

Every review batch launches exactly THREE generic background subagents together, all with `subagent_type: general-purpose`, `model: opus` (Opus 5), and `run_in_background: true`. At most one batch may be active. The reviewers load exactly one skill each:

1. `/lfx-skills:lfx-general-code-review`
2. `/project-service-code-reviewer`
3. `/project-service-learnings-reviewer`

The reviewers only report findings. They never edit tracked files, stage, commit, push, or write GitHub state; the parent performs all changes.

### Shared reviewer prompt

Give each reviewer one complete prompt. Start with its loading policy, then append the common instructions.

- General: `Load /lfx-skills:lfx-general-code-review with the Skill tool. If that skill is unavailable, do not review unguided and do not read a replacement SKILL.md from any checkout or cache; return INCOMPLETE.`
- Repo code: `Load /project-service-code-reviewer with the Skill tool. If and only if that skill is unavailable in this child's current session, locate the lfx-v2-project-service repo root and read <repo-root>/.claude/skills/project-service-code-reviewer/SKILL.md. Follow that file as the sole review guidance. Do not search another path or use another skill or agent. If the file is missing, return INCOMPLETE.`
- Repo learnings: `Load /project-service-learnings-reviewer with the Skill tool. If and only if that skill is unavailable in this child's current session, locate the lfx-v2-project-service repo root and read <repo-root>/.claude/skills/project-service-learnings-reviewer/SKILL.md. Follow that file as the sole review guidance. Do not search another path or use another skill or agent. If the file is missing, return INCOMPLETE.`

```text
target repo: lfx-v2-project-service
repo root: <absolute repo root>
target_sha: <full target SHA>
base_sha: <full base SHA>
review exactly: git diff <full base SHA> <full target SHA>
range label: <mode-specific range label>

The repo root and SHA range above are authoritative. Do not re-derive the range from HEAD or origin/main. Read added or modified code from <target_sha>:<path>, deleted code from <base_sha>:<path>, and both revisions for a rename. Never use a moving working-tree copy as code evidence. Load current rule, contract, checklist, architecture, and knowledge-base policy as the assigned skill directs.

Report findings only. Follow the assigned skill's report conventions and return its complete findings. Prepend `Reviewed range: <full base SHA>..<full target SHA>`, then `Skill: /lfx-skills:lfx-general-code-review`, `Skill: /project-service-code-reviewer`, or `Skill: /project-service-learnings-reviewer`, matching that reviewer. If a repo reviewer used its allowed file fallback, append `; read from: <exact path>` to its Skill line. If incomplete, put `INCOMPLETE — <reason>` first, then the same two verification lines.
```

Accept a batch only when all three reviewers return non-empty, complete reports for the pinned full-SHA range, name their exact assigned `/...` skill, and report no unauthorized fallback path. If any reviewer fails these checks, reject the entire batch; never accept or rerun only one reviewer.

### Mode 1 — Post-commit review

Use this mode after normal development commits while work continues.

1. Commit with `git commit -s -S`.
2. Maintain `reviewed_through_sha`: the latest commit fully covered by an accepted post-commit batch. Before the first batch, initialize it to the parent of the first pending commit. Never advance it for a failed or incomplete batch.
3. When no batch is active, set `base_sha=$reviewed_through_sha` and `target_sha=$(git rev-parse HEAD)`. Label a one-commit range `the latest commit`; if commits accumulated, label it `the commits since the last review`.
4. Launch the three reviewers together with that exact range. If another batch is already active, let it finish; the next batch will cover everything from the unchanged `reviewed_through_sha` through the then-current `HEAD`.
5. If the batch is invalid and `HEAD` is unchanged, rerun all three with the same pins. If `HEAD` changed, rerun all three over the coalesced range from the unchanged `reviewed_through_sha` through current `HEAD`.
6. After a valid batch, advance `reviewed_through_sha` to its `target_sha`. Verify its findings against current code and address every Critical and reasonable Important finding in a later commit; that commit is reviewed by the next post-commit batch.
7. The final planned commit skips post-commit review and moves directly to Mode 2. Leave `reviewed_through_sha` unchanged. If development resumes before Mode 2 starts, the next post-commit batch covers the entire pending range from that unchanged SHA.

### Mode 2 — Full-branch review before opening the PR

Entering this mode ends post-commit review for this PR attempt. Finish any active post-commit batch, then do not return to Mode 1.

1. Run `git fetch origin`, set `target_sha=$(git rev-parse HEAD)` and `base_sha=$(git merge-base origin/main HEAD)`, and launch the three reviewers together once against the whole branch range. Use the shared prompt with the range label `the branch's diff against origin/main` and review `git diff <full base SHA> <full target SHA>`. Never use `reviewed_through_sha` for this review.
2. If the batch is operationally incomplete, retry the complete three-reviewer batch without changing the branch until one valid result returns. This obtains the one review; it is not another review pass.
3. Fix the issues raised by that review, run the repository's documentation-currency checks, `/project-service-pr-readiness`, and `/project-service-preflight`, then create the signed/DCO commit. Do not run the local reviewers again for that commit.
4. Push and open the PR. From that point onward, use Post-PR review only.

## Post-PR review

Once the PR exists, never run the local post-commit reviewers or another local full-branch review. PR iteration uses Copilot and every other configured GitHub code-review agent/bot.

1. After every push, wait for the configured GitHub reviewers to finish reviewing the current head, then enumerate every unresolved review thread. Collect compatible feedback into a batch rather than making one-comment-at-a-time commits.
2. Work in an isolated background task when safe so the developer can continue. Never allow two writers to edit the same worktree or race commits or pushes; otherwise handle the feedback synchronously.
3. Verify every finding against the current head, actual runtime/API contracts, repository guidance, and approved PR scope. Never assume a bot is correct and never silently ignore a finding.
4. For a genuine in-scope issue, make the smallest focused fix and validate it. Otherwise, tell the developer why and post an evidence-backed rebuttal. Escalate architecture, security, ownership, and excluded-surface questions instead of guessing.
5. Comment before resolving every thread. For a fix, cite the fix commit and validation evidence; for a rebuttal, give the reason and evidence. Every thread must end fixed-and-explained or rebutted-and-explained.
6. Group compatible fixes into one signed/DCO commit, push, wait for reviews on the new head, and repeat until no unresolved actionable threads remain and required checks are green.
7. Do not merge as part of this automated iteration. Merge only after a separate explicit human instruction.

## Code Generation (Goa Framework)

The service uses Goa v3 for API code generation. This is **critical** to understand:

1. **Design First**: API is defined in `api/project/v1/design/` files
2. **Generated Code**: Running `make apigen` generates to `api/project/v1/gen/`:
   - HTTP server/client code
   - Service interfaces
   - OpenAPI specifications
   - Type definitions
3. **Implementation**: You implement the generated interfaces in `cmd/project-api/service*.go` files

### Adding New Endpoints

1. Update `api/project/v1/design/project.go` with new method
2. Run `make apigen` (from repository root) to regenerate code
3. Implement the Goa endpoint adapter in `cmd/project-api/service_endpoint_*.go` (translation only); put business logic in `internal/service/*_operations.go`
4. Add tests alongside the implementation (`internal/service/*_operations_test.go` and the adapter test)
5. Update Heimdall ruleset in `charts/*/templates/ruleset.yaml`

## NATS Messaging Patterns

The service uses NATS for:

1. **Storage**: Key-Value stores for project data
2. **Events**: Publishing events on data changes
3. **RPC**: Handling requests from other services

### Key-Value Stores

- `projects`: Base project information
- `project-settings`: Project settings (separated for access control)
- `project-links`: Project link records
- `project-folders`: Project folder records
- `project-documents-metadata`: Project document metadata
- `project-documents`: Project document binaries (NATS object store)

All bucket names live as constants in `pkg/constants/nats.go`.

### API Endpoints and Message Subjects

Complete API endpoint documentation and NATS message handlers are now documented in README.md.

There are two distinct NATS patterns in this service — both use `QueueSubscribe` but for different purposes:

**Request/reply RPC** (`internal/service/project_handlers.go`): another service sends a request and blocks waiting for a response. The handler calls `msg.Respond(data)` to return data to the caller.

**Event subscriptions** (`internal/service/project_subscriber.go` and `internal/service/document_subscriber.go`): the service reacts to events that were already published (including by itself). No caller is waiting — the handler is fire-and-forget and never calls `msg.Respond`.

```go
// Inbound RPC — request/reply, caller blocks waiting for response
"lfx.projects-api.get_name"            // Get project name by UID
"lfx.projects-api.get_slug"            // Get project slug by UID
"lfx.projects-api.get_logo"            // Get project logo URL by UID
"lfx.projects-api.get_writers"         // Get project writers by UID
"lfx.projects-api.slug_to_uid"         // Convert slug to UID
"lfx.projects-api.get_parent_uid"      // Get parent project UID

// Inbound events — fire-and-forget, no reply expected
"lfx.projects-api.project_settings.updated" // Self-published; sends role notification emails / invites on member changes
"lfx.invite-service.invite_accepted"   // From invite-service (enriched event); promotes matching email-only users to LFID across all projects
"lfx.projects-api.project_document.created" // Self-published; emails project writers/auditors about the new document
"lfx.projects-api.project_link.created"     // Self-published; emails project writers/auditors about the new link

// Outbound events (published by this service)
"lfx.index.project"                    // Project created/updated/deleted for indexing
"lfx.index.project_settings"           // Settings created/updated/deleted for indexing
"lfx.index.project_link"               // Link created/deleted for indexing
"lfx.index.project_folder"             // Folder created/deleted for indexing
"lfx.index.project_document"           // Document created/deleted for indexing
"lfx.projects-api.project_settings.updated" // Settings changed (before/after snapshot)
"lfx.projects-api.project_document.created" // File document uploaded (events.ProjectDocumentCreatedMessage)
"lfx.projects-api.project_link.created"     // Link added (events.ProjectLinkCreatedMessage)
"lfx.fga-sync.update_access"           // Generic FGA access control updates
"lfx.fga-sync.delete_access"           // Generic FGA access control deletion

// Outbound request/reply (published by this service, awaits a response)
"lfx.email-service.send_email"         // Request to email service for role notifications
"lfx.invite-service.send_invite"       // Request to invite service for non-LFID users
```

### FGA Sync Message Format

The service uses the generic FGA sync handlers for access control. All messages use the `GenericFGAMessage` envelope:

```go
// Update access control (full sync) — fgatypes.GenericAccessData
GenericFGAMessage{
    ObjectType: "project",
    Operation: "update_access",
    Data: GenericAccessData{
        UID: "project-uid",
        Public: true,
        Relations: map[string][]string{
            "writer": []string{"username1", "username2"},
            "auditor": []string{"username3"},
            "meeting_coordinator": []string{"username4"},
            "executive_director": []string{"username5"},
        },
        References: map[string][]string{
            "parent": []string{"project:parent-uid"},
        },
    },
}

// Delete all access control — fgatypes.GenericDeleteData
GenericFGAMessage{
    ObjectType: "project",
    Operation: "delete_access",
    Data: GenericDeleteData{
        UID: "project-uid",
    },
}
```

**Key Points:**

- Relations map user roles to usernames (e.g., `"writer": ["user1", "user2"]`)
- References map object relationships with formatted UIDs (e.g., `"parent": ["project:parent-uid"]`)
- Update operations are full sync - any relations not included will be removed
- Delete operations remove all access control tuples for the resource

## Testing Patterns

### Unit Tests

- Mock all external dependencies (repository, message builder)
- Test each layer in isolation
- Use table-driven tests for comprehensive coverage
- Write one function tests containing multiple test cases that focus on a single function
- Focus on testing exported functions of packages
- Unit tests should be alongside the implementation code with the same file name with a suffix of `*_test.go`
- **IMPORTANT**: Each function should have exactly ONE corresponding test function (e.g., `SendIndexProject` → `TestMessageBuilder_SendIndexProject`) which can have multiple tests cases within it.
- Add test cases within existing test functions if the function you are trying to test already has one rather than creating new test functions

### Example Test Structure

```go
func TestEndpoint(t *testing.T) {
    tests := []struct {
        name       string
        payload    *projsvc.Payload
        setupMocks func(*domain.MockRepo, *domain.MockMsg)
        wantErr    bool
    }{
        // Test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            api, mockRepo, mockMsg := setupAPI()
            tt.setupMocks(mockRepo, mockMsg)
            // Test logic
        })
    }
}
```

## Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | HTTP listen port | 8080 | No |
| `NATS_URL` | NATS server URL | nats://localhost:4222 | No |
| `LOG_LEVEL` | Log level | info | No |
| `JWKS_URL` | JWT verification endpoint | - | No |
| `AUDIENCE` | JWT audience | lfx-v2-project-service | No |
| `JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL` | Mock auth for local dev | - | No |
| `SKIP_ETAG_VALIDATION` | Skip If-Match/ETag revision enforcement on writes (`true` to skip; local dev only) | false | No |
| `LFX_ENVIRONMENT` | Deployment environment (`prod`/`production`, `staging`/`stg`/`stage`, `dev`/`development`); drives the default self-serve base URL when `LFX_SELF_SERVE_BASE_URL` is empty; defaults to prod when unset | - | No |
| `LFX_SELF_SERVE_BASE_URL` | Base URL for project links in notification emails; takes precedence over `LFX_ENVIRONMENT` | derived from `LFX_ENVIRONMENT` (prod when unset) | No |
| `EMAILS_ENABLED` | Gate for outbound role-notification emails to LFID users (`true` to enable) | false | No |
| `INVITES_ENABLED` | Gate for outbound invite requests to non-LFID users (`true` to enable) | false | No |

## Authorization (OpenFGA)

When deployed, the service uses OpenFGA for authorization:

- **GET /projects** - Denied in deployed environments (local development only)
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

### 1. Forgetting to Generate Code

**Problem**: Changes to design files not reflected in implementation
**Solution**: Always run `make apigen` after modifying design files

### 2. ETag Handling

**Problem**: Concurrent updates without proper ETag validation
**Solution**: Always include If-Match header in PUT/DELETE requests (server responds with ETag header on GET request)

### 3. NATS Connection

**Problem**: Service fails to start due to NATS connection
**Solution**: Ensure NATS is running and NATS_URL is correct

### 4. Slug Validation

**Problem**: Invalid slug format causes API errors
**Solution**: Slugs must match `^[a-z][a-z0-9_\-]*[a-z0-9]$`

### 5. Parent Project Validation

**Problem**: Creating projects with invalid parent_uid
**Solution**: parent_uid must be empty string or valid UUID

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

### 3. NATS Event Wire Types (`pkg/events/`)

NATS message payload types that other services consume belong in `pkg/events/`, **not** `internal/`. This lets downstream services (e.g., `lfx-v2-invite-service`) import the canonical struct definitions directly.

- Domain types in `internal/domain/models/` may differ from wire types and can evolve independently.
- Explicit converter functions in `internal/service/converters.go` map from domain → event type before publishing.
- Example: `DomainSettingsToEvent(*models.ProjectSettings) events.ProjectSettings`
- When a single `(base, settings)` domain pair needs to produce multiple output shapes, construct a `ProjectProjection` via `NewProjectProjection(base, settings)` and call the appropriate methods (`ToFull`, `ToFGAMessage`, `ToEventSettings`, etc.) rather than forwarding the pair to each standalone converter independently.

**Rule:** if a struct appears in a NATS message payload, it belongs in `pkg/events/`, not `internal/`.

### 4. Request Context

Important context values:

- `request-id`: Unique request identifier
- `authorization`: JWT token from header
- `etag`: ETag value for optimistic concurrency (sent as If-Match header in requests)

### 5. Error Handling

Domain errors are named sentinels in `internal/domain/errors.go`, mapped to HTTP status codes by `handleError` in `cmd/project-api/service_endpoint_project.go`:

- `ErrProjectNotFound` / `ErrDocumentNotFound` / `ErrLinkNotFound` / `ErrFolderNotFound` → 404
- `ErrProjectSlugExists` / `ErrRevisionMismatch` / `ErrDocumentNameExists` / `ErrFolderNameExists` / `ErrFolderNotEmpty` → 409
- `ErrValidationFailed` / `ErrInvalidParentProject` / `ErrInvalidContentType` / `ErrFileTooLarge` / `ErrCannotDeleteNonCrowdfundingProject` → 400
- `ErrInternal` / `ErrUnmarshal` → 500
- `ErrServiceUnavailable` → 503

### 6. Service Modules — use these, do not duplicate

The `internal/service/` layer contains three focused modules extracted to avoid duplication. **Always reach for these before adding new logic inline.**

| Module | File | Purpose | How to use |
|---|---|---|---|
| `UserResolver` | `user_resolver.go` | Centralised user identity lookup: resolves display names from JWT, auth service, or falls back gracefully | Call `s.Resolver.ResolveDisplayName(ctx, events.Actor{Username: username})` or `s.Resolver.ResolveRequestingUser(ctx)` — never inline the auth-service lookup |
| `NotificationDispatcher` | `notification_dispatcher.go` | Orchestrates role-change emails and invite requests for LFID and non-LFID users, respects `EmailsEnabled`/`InvitesEnabled` feature flags | Call `s.Dispatcher.Dispatch(ctx, projectUID, name, url, actor, changes)` from `HandleProjectSettingsUpdated` — never add notification logic to subscribers or operations directly |
| `ProjectProjection` | `converters.go` | Bundles a `(base, settings)` domain pair and exposes typed output methods for each target type universe | Call `NewProjectProjection(base, settings)` once then `.ToFull()`, `.ToFGAMessage()`, `.ToEventSettings()`, etc. — never forward the same pair to multiple standalone converters |

**`resolveRevision` pattern** (`project_operations.go`): write operations (update, delete) must resolve the resource revision either from an `If-Match` header or by fetching it from the repository. Use the `s.resolveRevision(ctx, ifMatch, fetchFn)` helper — do not duplicate the `SkipEtagValidation` branching inline.

## Debugging Tips

1. **Enable Debug Logging**: Run with `-d` flag or set `LOG_LEVEL=debug`
2. **Check NATS Messages**: Use `nats sub "lfx.>"` to monitor all messages
3. **Verify KV Data**: Use `nats kv get projects <uid>` to check stored data
4. **HTTP Traces**: Middleware logs all requests with timing
5. **Generated Code**: Check `api/project/v1/gen/` directory for Goa-generated interfaces

## Documentation Structure

The project has a clear documentation hierarchy:

- **README.md**: Project overview, quick start, API endpoints, deployment setup
- **DEVELOPMENT.md**: Comprehensive developer guide with build/test/deploy workflows
- **CLAUDE.md**: AI assistant instructions and technical details (this file)

Key documentation patterns:

- README focuses on getting the service running quickly
- DEVELOPMENT.md covers the full development workflow
- Avoid duplicating content between files - use cross-references instead

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
