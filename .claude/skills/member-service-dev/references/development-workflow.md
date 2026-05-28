<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Development Workflow (member-service)

Workflow, commands, and integration touchpoints unique to this repo.
Coding conventions live in the parent SKILL.md. The procedural recipe for
adding or changing an HTTP endpoint lives in the local
`member-add-endpoint` skill, not here.

## Contents

1. Prerequisites
2. Make targets
3. Goa code generation
4. JWT and Heimdall flow
5. OpenFGA model and ruleset
6. Docker and CI
7. Error-mapping reference

## Prerequisites

```bash
# Go 1.24+ in PATH
make deps       # installs goa v3.25.3 from GOA_VERSION
make setup-dev  # installs golangci-lint v2.2.2 from GOLANGCI_LINT_VERSION
```

## Make targets

```bash
make apigen          # Regenerate Goa code into gen/ after design changes
make build           # Build the member-api binary
make test            # go test -v -race -coverprofile=coverage.out ./...
make run             # Run locally
make fmt             # go fmt ./... plus gofmt -s -w on non-generated Go files
make lint            # golangci-lint
make helm-templates  # Render the service Helm chart
```

There is no `make check`, `make debug`, `make test-verbose`, or
`make test-coverage` target today. For debug logging, build and run the
binary with `-d` or set `LOG_LEVEL=debug`.

## Goa code generation

The service uses Goa v3.

1. Design lives in `cmd/member-api/design/`.
2. `make apigen` writes to `gen/`: HTTP server/client, service interfaces,
   OpenAPI spec, type definitions.
3. Implement the generated interfaces in
   `cmd/member-api/service/membership_service.go`.

Never hand-edit anything under `gen/`; it is overwritten on every regen.
The full add-endpoint recipe (design, regen, handler, tests, Heimdall
ruleset update) is owned by the `member-add-endpoint` skill.

## JWT and Heimdall flow

JWT authentication is implemented in `internal/infrastructure/auth/`:

- `JWTAuth`: validates Heimdall-issued JWTs via JWKS.
- `MockJWTAuth`: test mock implementing `domain.Authenticator`.

Request path:

1. Heimdall intercepts the request and validates the upstream OIDC token.
2. Heimdall mints a signed JWT with a `principal` claim and forwards it.
3. The service validates the Heimdall JWT inside the Goa `JWTAuth` security
   handler.
4. The principal is stored in context under
   `constants.PrincipalContextID`.

Setting `JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL` skips JWT validation and
uses the env var value as the principal. Local development only; never let
this code path activate in production.

## OpenFGA model and ruleset

The `project` type is defined in `lfx-v2-helm`:

```dsl
type project
  relations
    define auditor: [user, team#member]
    define writer: [user, team#member]
```

Heimdall ruleset checks for this service:

- `GET /projects/{project_id}/*`: requires `auditor` on
  `project:{project_id}`.
- `POST`, `PUT`, `DELETE` `/projects/{project_id}/memberships/{id}/key_contacts[/{cid}]`:
  requires `writer` on `project:{project_id}`.
- `GET /b2b_orgs` and `GET /b2b_orgs/{b2b_org_uid}/memberships`: interim
  detour endpoints gated by `auditor` on the static LF project UID from
  `.Values.openfga.lfProjectUID`.

Ruleset YAML lives in
`charts/lfx-v2-member-service/templates/ruleset.yaml`.

## Docker and CI

```bash
docker build -t lfx-v2-member-service:latest .
```

Dockerfile uses Chainguard Go for building and Chainguard static
(distroless) for runtime, multi-stage for minimal image size.

GitHub Actions workflows:

- `mega-linter.yml`: Go, YAML, Docker linting.
- `member-api-build.yml`: build and test on PRs.
- `ko-build-main.yml`, `ko-build-branch.yml`, `ko-build-tag.yml`: image
  build workflows.
- `license-header-check.yml`: license header enforcement.

## Error-mapping reference

Domain errors map to HTTP status in
`cmd/member-api/service/error.go`:

| Domain error | HTTP status |
| --- | --- |
| `Validation` | 400 |
| `NotFound` | 404 |
| `ServiceUnavailable` | 503 |
| anything else | 500 |

`pkg/errors.Conflict` exists, but `wrapError` does not currently map it to
409 and the active Goa methods do not declare conflict responses. If a new
endpoint needs conflict behavior, update the Goa design and `wrapError` in
the same change. Return domain errors from new methods; do not return raw
`error` values.
