<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Go Conventions (Reference)

Contents: dependency-injection style and mock layout for lfx-v2-project-service. The summary rules (logging, errors, request context, pagination, formatting, license header) live in `SKILL.md`. This reference is the place to look only when SKILL.md sends you here for depth.

## Dependency injection

- Services receive their collaborators by interface in struct fields, not by package-global accessors. See `internal/service/project_service.go::ProjectsService`.
- Repositories are interfaces declared in `internal/domain/repository.go`. Concrete implementations live in `internal/infrastructure/`.
- The Goa endpoint adapter `ProjectsAPI` in `cmd/project-api/` owns only translation: payload to service call, error to HTTP via `handleError`, result to Goa result type. No business logic in the adapter.
- `main.go` is the only place that constructs concrete implementations and wires them into `ProjectsService`. Tests construct their own `ProjectsService` with mocks (`setupAPI` in `service_endpoint_project_test.go`).

## Mock layout

- All domain-level mocks live in `internal/domain/mock.go`: `MockProjectRepository`, `MockDocumentRepository`, `MockLinkRepository`, `MockFolderRepository`, `MockMessageBuilder`.
- Auth mocks live in `internal/infrastructure/auth/` next to the real auth code.
- Use `github.com/stretchr/testify/mock` exclusively. Do not add a second mock framework.
- Build mocks with the table-driven pattern: each table row's `setupMocks func(...)` programs only the calls that row needs.

## Naming

- One Go file per logical concern: `project_operations.go`, `document_operations.go`, `link_operations.go`, `folder_operations.go`, `converters.go`. Match that grain.
- One test file per source file: `*_test.go` next to it.
- Exported types and methods get `// Name describes...` comments. Unexported helpers may skip comments when the code is self-explanatory.

## Where to add new code

| Concern | File |
| --- | --- |
| New domain error | `internal/domain/errors.go` (and the switch in `cmd/project-api/service_endpoint_project.go::handleError`) |
| New NATS subject or KV bucket name | `pkg/constants/nats.go` |
| New HTTP header constant or context key | `pkg/constants/http.go` |
| New repository method | interface in `internal/domain/repository.go`, implementation in `internal/infrastructure/nats/`, mock in `internal/domain/mock.go` |
| New endpoint (after design + apigen) | `cmd/project-api/service_endpoint_*.go` plus `internal/service/*_operations.go` |
| New middleware | `internal/infrastructure/middleware/` with a `*_test.go` alongside |
| New shared structured-log field | a `log.AppendCtx` call inside the middleware or service that has the value |
