---
name: member-add-endpoint
description: Use when adding a new HTTP endpoint to the lfx-v2-member-service (project-scoped membership, key contacts, B2B detour endpoints, or related membership resources). Covers the Goa design update, regeneration, handler implementation, test scaffolding, and the Heimdall ruleset update that authorization requires. Also use when modifying an existing endpoint's signature in a way that triggers a regen + ruleset update.
paths:
  - 'cmd/member-api/design/**'
  - 'cmd/member-api/service/**'
  - 'charts/lfx-v2-member-service/templates/ruleset.yaml'
allowed-tools: Read, Glob, Grep, Edit, Write, Bash
---

# Add a New Member Service Endpoint

The lfx-v2-member-service uses Goa v3 for code generation. Every new endpoint flows through the design DSL, regenerates the HTTP layer, gets implemented in the service struct, gets tested, and must be added to the Heimdall ruleset for authorization. Skipping the ruleset step ships an unauthorized endpoint.

For the broader development reference (Makefile targets, JWT/Heimdall flow, OpenFGA `project` type, error mapping), see `.claude/skills/member-service-dev/references/development-workflow.md`.

## Workflow

1. **Update the Goa design** in `cmd/member-api/design/membership.go`. Add the new `Method` block with payload, result, and HTTP binding. Declare `dsl.Security(JWTAuth)` unless the endpoint is intentionally public (none currently are).

2. **Regenerate the API layer**:

   ```bash
   make apigen
   ```

   This writes to `gen/`. Never hand-edit `gen/`; those files are overwritten on every regen.

3. **Implement the method** on the `membershipServicesrvc` struct in `cmd/member-api/service/membership_service.go`. The struct already holds `memberReaderOrchestrator`, `storage`, `auth`, `keyContactWriter`, and `b2bOrgReader`. Route membership reads through the orchestrator, key-contact writes through `keyContactWriter`, and B2B detour reads through `b2bOrgReader`. Return early on validation errors with `pkg/errors` domain types so `cmd/member-api/service/error.go` can map them.

4. **Add unit tests** in `cmd/member-api/service/membership_service_test.go` following the table-driven shape already used:

   ```go
   func TestEndpoint(t *testing.T) {
       tests := []struct {
           name       string
           payload    *membershipservice.Payload
           wantErr    bool
       }{
           // cases
       }
       for _, tt := range tests {
           t.Run(tt.name, func(t *testing.T) {
               // ...
           })
       }
   }
   ```

   Each function has exactly one corresponding test function with multiple cases. Mock data dependencies via `internal/infrastructure/mock/`. Existing service tests usually build auth with `auth.NewJWTAuth(auth.JWTAuthConfig{MockLocalPrincipal: "test-user"})`; use `auth.MockJWTAuth` only when the test needs explicit authenticator expectations.

5. **Update the Heimdall ruleset** in `charts/lfx-v2-member-service/templates/ruleset.yaml`. Add an `openfga_check` entry for the new path and verb. The current authorization shape is:

   - `GET /projects/{project_id}/*` requires `auditor` on `project:{project_id}`.
   - `POST/PUT/DELETE /projects/{project_id}/memberships/{id}/key_contacts[/{cid}]` requires `writer` on `project:{project_id}`.
   - `GET /b2b_orgs` and `GET /b2b_orgs/{b2b_org_uid}/memberships` are interim detour endpoints gated by `auditor` on the configured static LF project UID (`.Values.openfga.lfProjectUID`).

   New routes under `/projects/{project_id}/*` should follow the same relation pattern (`auditor` for reads, `writer` for writes) unless the endpoint genuinely needs different semantics.

6. **Verify**:

   ```bash
   make fmt     # if Go files changed
   make lint    # lint when practical
   make test    # unit suite with race and coverage profile
   make build   # ensures regen + impl compile
   ```

## Error mapping reminder

Return the existing domain errors so the mapper in `cmd/member-api/service/error.go` produces the right HTTP status:

- `Validation` -> 400
- `NotFound` -> 404
- `ServiceUnavailable` -> 503
- anything else -> 500

Do not return raw `error` values from new methods; wrap them through the domain error types.

## Anti-patterns

- Shipping a Goa method without updating `ruleset.yaml` in the same PR. Heimdall will reject the request (or worse, allow it under a stale rule) and the endpoint will be effectively broken.
- Editing files under `gen/` directly. They are clobbered on the next `make apigen`.
- Adding new dependencies to `membershipServicesrvc` for logic that belongs in an existing port or orchestrator. The service struct should stay thin; business logic belongs in `MemberReaderOrchestrator`, `keyContactWriter`, `b2bOrgReader`, or a sibling use-case type if one is genuinely needed.
- Adding the `JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL` path into production code paths. That env var is local-dev only.

## References

- `../member-service-dev/references/development-workflow.md`: Makefile targets, auth flow, OpenFGA `project` type, error mapping table, service architecture notes.
- `cmd/member-api/design/membership.go`: Goa DSL entry point.
- `cmd/member-api/service/membership_service.go`: service struct and existing method implementations.
- `cmd/member-api/service/error.go`: domain error to HTTP status mapping.
- `charts/lfx-v2-member-service/templates/ruleset.yaml`: Heimdall authorization rules.
