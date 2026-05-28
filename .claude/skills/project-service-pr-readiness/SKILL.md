---
name: project-service-pr-readiness
description: >
  Pre-PR shape check for local lfx-v2-project-service work. Audits branch name,
  JIRA reference, conventional commits, rebase status, DCO plus GPG signing,
  total diff size, and project-service protected files against the target base
  branch. Shape only: no Go code audit, no lint/build/test execution, and no PR
  side effects. Run before /project-service-preflight.
context: fork
allowed-tools: Bash, Read, Glob, Grep
---

<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Project Service PR Readiness

You are checking whether **local commits are shaped correctly to open a PR** for
`lfx-v2-project-service`: branch name, JIRA references, conventional commit
subjects, rebase status, DCO plus GPG signing, diff size, and protected-file
visibility.

This skill does **not** audit Go code, generated code contents, architecture, or
behavior. Do not run `make lint`, `make build`, `make test`, or `make apigen`
from this skill. Mechanical validation belongs to `/project-service-preflight`.
Coding and service conventions are owned by the auto-attached
`project-service-dev` skill.

**Output:** structured shape report with verdict `NOT READY`,
`READY WITH CHANGES`, or `READY`. No git mutations and no PR side effects.

---

## Mandatory Project-Service Protected Files

Use this repo-specific list. Do not import the central Angular list or generic
`lfx-skills` protected-file list.

- **Goa design and generated boundary:** `api/project/v1/design/**`,
  `api/project/v1/gen/**`.
- **Project public contracts:** Goa design files, generated OpenAPI files under
  `api/project/v1/gen/http/openapi*`, and README API-contract sections.
- **NATS/KV publisher contracts:** `pkg/constants/nats.go`,
  `internal/infrastructure/nats/message.go`,
  `internal/infrastructure/nats/repository.go`, `docs/indexer-contract.md`,
  `docs/fga-contract.md`.
- **Charts and deployment contracts:** `charts/lfx-v2-project-service/**`.
- **Dependency and build surfaces:** `go.mod`, `go.sum`, `Makefile`.
- **Agent guidance and local skills:** `CLAUDE.md`, `.claude/skills/**`.
- **Contract docs:** `docs/*contract*.md`.

Protected files may be legitimate in a PR, but the shape report must call them
out so the PR body can explain why they changed and who should review them.

---

## Phase 1 - Parse Arguments

Args format: `[base-branch] [extra instructions]`.

- First token, if it looks like a ref or branch name, is the base branch.
- Default base: `origin/main`.
- Normalize bare branch names such as `main` to `origin/main`.
- Treat the rest as optional focus text for the report.

## Phase 2 - Gather Shape Inputs

Run these from the repository root:

```bash
git fetch origin
git rev-parse --abbrev-ref HEAD
git diff --shortstat <base>...HEAD
git diff --name-only <base>...HEAD
git log --format='%H %s' <base>..HEAD
git log --format='%G? %h %s' <base>..HEAD
git log --format=%B <base>..HEAD
git merge-base --is-ancestor <base> HEAD
```

If there are no commits in `<base>..HEAD`, stop with:

```text
No commits to audit against <base> - make at least one commit on this branch.
```

## Phase 3 - PR-Shape Checks

Each finding must map to one of these checks. Do not add code-review findings.

### Branch Name

Expected shape:

- Includes an `LFXV2-1234` style ticket.
- Uses a readable topic prefix such as `feat/`, `fix/`, `docs/`, `test/`,
  `refactor/`, `chore/`, `build/`, or `ci/`.

Missing ticket is `CRITICAL`. Unclear topic prefix is `SHOULD_FIX`.

### JIRA Reference

Check commit subjects and bodies for `LFXV2-[0-9]+`.

- No branch-level or commit-level ticket reference: `CRITICAL`.
- Branch has a ticket but commits do not: `SHOULD_FIX`.

### Conventional Commits

Each commit subject should match:

```text
^(feat|fix|docs|test|refactor|chore|build|ci|perf|style|revert)(\([a-z0-9._/-]+\))?!?: .+
```

Invalid subjects are `SHOULD_FIX` unless the commit is an intentional merge or
revert that Git generated.

### Rebase Status

`git merge-base --is-ancestor <base> HEAD` must exit `0`. If not, the branch is
not based on current `<base>` and the finding is `CRITICAL`.

### DCO Plus GPG

For every commit in `<base>..HEAD`:

- GPG signature status from `%G?` should be `G` or `U`.
- Commit body must contain a `Signed-off-by:` trailer.

Missing signature or missing signoff is `CRITICAL`.

### Diff Size

Use additions from `git diff --shortstat <base>...HEAD`.

- `0-800` additions: pass.
- `801-1500` additions: `SHOULD_FIX` to consider splitting or explain scope.
- `>1500` additions: `CRITICAL` unless the large delta is mostly generated
  Goa output paired with design changes.

### Protected Files

Intersect `git diff --name-only <base>...HEAD` with the protected list above.

Report every match with category and expected PR-body note. Use these additional
shape checks for generated-code boundaries:

- `api/project/v1/gen/**` changed without `api/project/v1/design/**`:
  `CRITICAL`, because generated output is not tied to a design source change.
- `api/project/v1/design/**` changed without `api/project/v1/gen/**`:
  `SHOULD_FIX`, because generated code is probably stale until
  `/project-service-preflight` runs `make apigen`.
- Publisher files changed without the matching contract doc:
  `SHOULD_FIX`. For indexer/FGA publisher changes, expect
  `docs/indexer-contract.md` or `docs/fga-contract.md` in the same PR.

## Phase 4 - Render Findings

Emit each finding in this shape:

```json
{
  "severity": "CRITICAL | SHOULD_FIX | NIT",
  "rule": "pr-shape/<check-id>",
  "message": "...",
  "suggestion": "..."
}
```

## Phase 5 - Report

```markdown
# Project Service PR Readiness

**Branch:** `<current-branch>` -> `<base>`
**Commits:** N | **Additions:** +A | **Deletions:** -D
**Verdict:** NOT READY | READY WITH CHANGES | READY

## PR-shape sanity

| Check | Status | Detail |
| --- | --- | --- |
| Branch name | PASS | feat/LFXV2-1234-short-topic |
| JIRA ticket | PASS | Found LFXV2-1234 in branch and commits |
| Conventional commits | PASS | All commits valid |
| Branch rebased | PASS | origin/main is an ancestor |
| DCO + GPG signing | PASS | 3/3 commits signed and signed off |
| Diff size | PASS | 342 additions |
| Protected files | SHOULD_FIX | charts/lfx-v2-project-service/values.yaml |

## Verdict reasoning

<one line per CRITICAL or SHOULD_FIX finding>
```

### Verdict Rules

- **NOT READY**: any `CRITICAL` finding.
- **READY WITH CHANGES**: zero `CRITICAL`; at least one `SHOULD_FIX`.
- **READY**: zero `CRITICAL`, zero `SHOULD_FIX`.

## Companion Skills

- `/project-service-preflight`: mechanical Go validation. Run after this passes.
- `project-service-dev`: auto-attached coding convention skill for Go,
  generated-code boundaries, NATS/KV publishing, tests, linting, formatting,
  and license headers.
- `/lfx-skills:lfx` and `/lfx-skills:lfx-platform-architecture`: central
  cross-repo routing and platform architecture only, not PR-shape checks for
  this repo.
