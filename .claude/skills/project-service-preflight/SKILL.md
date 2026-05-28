---
name: project-service-preflight
description: >
  Mechanical pre-PR pipeline for lfx-v2-project-service. Checks working tree
  status, license headers, Go formatting, lint, build, tests, project-service
  protected files, commit verification, generated-code freshness, and PR change
  summary. Run after /project-service-pr-readiness has passed. Supports
  report-only or dry-run mode.
allowed-tools: Bash, Read, Edit, Glob, Grep, AskUserQuestion
---

<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Project Service Preflight

You are running the **mechanical pre-PR pipeline** for
`lfx-v2-project-service`. Every check here is shell-driven or
repository-contract driven. Do not perform broad code review here; Go coding
conventions are owned by the auto-attached `project-service-dev` skill, and
central cross-repo architecture questions belong to
`/lfx-skills:lfx-platform-architecture`.

Run this after `/project-service-pr-readiness` has passed. If readiness has not
run, stop and ask the contributor to run it first unless they explicitly request
preflight only.

---

## Modes and Arguments

Args format: `[base-branch] [--dry-run|report only] [extra instructions]`.

- Default base: `origin/main`.
- Normalize bare branch names such as `main` to `origin/main`.
- Default mode may modify the worktree through `make fmt`, header edits, or
  `make apigen` when design files changed.
- `--dry-run`, `--report-only`, or "report only" means do not modify tracked
  files. Use check-only commands and report what would need to change.
- Never commit, stash, push, or create a PR from this skill without explicit
  user instruction.

---

## Check 0 - Working Tree Status

Run:

```bash
git fetch origin
git status --short
git diff --stat <base>...HEAD
git log --format="%h %s%n%b" <base>..HEAD
```

Evaluate before proceeding:

- Uncommitted changes: ask whether to continue, commit, or stash. Do not stash
  or commit automatically.
- No commits ahead of `<base>`: stop and ask whether the contributor is on the
  correct branch.
- Commit messages missing `LFXV2-` references: report before mechanical checks.
- Commits missing `Signed-off-by:` trailers: report before mechanical checks.

## Check 1 - License Headers

Run:

```bash
make license-check
```

Repo-specific license rules:

- New hand-written `.go` files must start with:

  ```go
  // Copyright The Linux Foundation and each contributor to LFX.
  // SPDX-License-Identifier: MIT
  ```

- Exclude `api/project/v1/gen/**` and `vendor/**`; generated files are not
  manually edited.
- New Markdown docs should use the HTML-comment form. `SKILL.md` files keep YAML
  front matter first, then the HTML-comment license header.

In default mode, add missing headers when the correct insertion point is clear.
In dry-run mode, report missing headers only.

## Check 2 - Generated Goa Boundary

Inspect changed files:

```bash
git diff --name-only <base>...HEAD
```

If `api/project/v1/design/**` changed:

- Default mode: run `make apigen`, then report any generated files changed.
- Dry-run mode: do not run `make apigen`; require matching changes under
  `api/project/v1/gen/**` or report generated code as likely stale.

If `api/project/v1/gen/**` changed without a matching design change, flag it as
a protected-boundary issue and require an explanation. Generated code should be
produced by `make apigen`, not hand-edited.

## Check 3 - Formatting

Default mode:

```bash
make fmt
```

Dry-run mode:

```bash
make check
```

`make fmt` runs Go formatting for this repo and excludes generated Goa files
from the direct `gofmt -s -w` pass. If formatting changes files, report them.

## Check 4 - Lint

Run:

```bash
make lint
```

If `golangci-lint` is missing, ask whether to run `make deps`. If the
contributor declines dependency installation, run `go vet ./...` as a weaker
fallback and say that `make lint` was not completed.

## Check 5 - Build

Run:

```bash
make build
```

If build fails, report the package/file/line from the failure and stop before
tests unless the contributor asks to continue.

## Check 6 - Tests

Run:

```bash
make test
```

This repo's test target includes `-race` and a five-minute timeout. Test
failures block preflight.

## Check 7 - Protected Files

Use this repo-specific protected list. Do not use central Angular protected
paths or a generic lfx-skills hook.

- **Goa design and generated boundary:** `api/project/v1/design/**`,
  `api/project/v1/gen/**`.
- **Project public contracts:** generated OpenAPI files under
  `api/project/v1/gen/http/openapi*`, README API-contract sections.
- **NATS/KV publisher contracts:** `pkg/constants/nats.go`,
  `internal/infrastructure/nats/message.go`,
  `internal/infrastructure/nats/repository.go`, `docs/indexer-contract.md`,
  `docs/fga-contract.md`.
- **Charts and deployment contracts:** `charts/lfx-v2-project-service/**`.
- **Dependency and build surfaces:** `go.mod`, `go.sum`, `Makefile`.
- **Agent guidance and local skills:** `CLAUDE.md`, `.claude/skills/**`.
- **Contract docs:** `docs/*contract*.md`.

Run:

```bash
git diff --name-only <base>...HEAD
```

For every protected match, report:

- file path;
- protected category;
- whether a paired contract/doc/generated update is present;
- what should be called out in the PR description.

Protected files do not automatically fail preflight, but unexplained
generated-code, publisher-contract, dependency, or chart changes should be
marked `ISSUES FOUND` until documented or corrected.

## Check 8 - Commit Verification

Run:

```bash
git status --short
git log --format="%G? %h %s%n%b" <base>..HEAD
```

Verify:

- all intended changes are committed or intentionally left uncommitted;
- every commit subject follows conventional-commit shape;
- every commit contains `Signed-off-by:`;
- every commit or commit body references an `LFXV2-` ticket;
- every commit is GPG-signed with status `G` or `U`.

## Check 9 - Change Summary

Generate:

```bash
git diff --stat <base>...HEAD
git diff --name-status <base>...HEAD
```

Summarize for the PR body:

1. New files and why they exist.
2. Modified Go packages by layer: Goa design/generated, endpoint adapter,
   service, domain, middleware, NATS infrastructure, shared constants.
3. Public contract changes: OpenAPI, README API behavior, indexer/FGA docs.
4. NATS/KV publisher changes and paired docs.
5. Helm chart changes.
6. Dependency/build changes in `go.mod`, `go.sum`, or `Makefile`.
7. Tests added or changed.

---

## Results Report

Use this format:

```text
PREFLIGHT RESULTS
-----------------------------------------
PASS Working tree      - Clean, N commits ahead of origin/main
PASS License headers   - All hand-written Go files have headers
PASS Goa generation    - Up to date
PASS Formatting        - Clean
PASS Linting           - make lint passed
PASS Build             - make build passed
PASS Tests             - make test passed
PASS Protected files   - 2 touched, documented for PR body
PASS Commits           - Conventional, signed, signed off, JIRA-linked
-----------------------------------------
READY FOR PR

Change summary:
- ...
```

If there are issues:

```text
PREFLIGHT RESULTS
-----------------------------------------
PASS Working tree      - Clean, N commits ahead of origin/main
FAIL License headers   - 1 file missing header
PASS Goa generation    - Up to date
FAIL Formatting        - 2 files need gofmt
PASS Linting           - make lint passed
PASS Build             - make build passed
FAIL Tests             - 1 package failed
WARN Protected files   - go.mod and charts changed; explain in PR body
FAIL Commits           - 1 commit missing Signed-off-by
-----------------------------------------
ISSUES FOUND - Fix before submitting
```

### Final Verdict

- All required checks pass: `READY FOR PR`.
- Only protected-file documentation remains: `READY WITH PR NOTES`.
- Any failed required check: `ISSUES FOUND`.

## Scope Boundaries

This skill does:

- Run or prescribe mechanical checks for this Go service.
- Preserve report-only/dry-run behavior.
- Surface protected project-service files and required PR notes.
- Produce a PR-ready change summary.

This skill does not:

- Perform code review or architecture review.
- Decide cross-repo ownership.
- Hand-edit generated Goa code.
- Commit, push, create a branch, or open a PR unless explicitly asked.
