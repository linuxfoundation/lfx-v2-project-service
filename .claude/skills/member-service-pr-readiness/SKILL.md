---
name: member-service-pr-readiness
description: >
  Shape-only pre-PR check for local lfx-v2-member-service work. Audits the
  branch name, LFXV2 ticket reference, conventional commit subjects, rebase
  status, DCO and GPG signing per commit, total diff size, and protected
  member-service files touched against the target base branch. Does not audit
  code behavior or run build/test/lint checks; run /member-service-preflight
  after this passes.
context: fork
allowed-tools: Bash, Read, Glob, Grep
---

<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Member Service PR Readiness

You are checking whether local commits are shaped correctly to open a PR for
`lfx-v2-member-service`. This is a shape check only: branch naming, JIRA
reference, conventional commits, rebase status, DCO + GPG signing, diff size,
and protected files.

Do not audit Go code behavior here. Do not run `make fmt`, `make lint`,
`make test`, `make build`, or code-review pattern checks. Mechanical
validation belongs in `/member-service-preflight` after this skill returns
clean enough to proceed.

Output a structured shape report with verdict `NOT READY`,
`READY WITH CHANGES`, or `READY`. No git mutations, no PR side effects.

## Phase 1 - Parse arguments

Args format: `[base-branch] [extra instructions]`.

- First token, if it looks like a ref or branch name, is the base branch.
- Default base: `origin/main`.
- Normalize bare branch names such as `main` to `origin/main`.
- Treat remaining text as extra context only; do not widen the audit beyond
  shape checks.

## Phase 2 - Gather PR-shape inputs

Fetch first when network access is available:

```bash
git fetch origin
```

Then run:

```bash
git rev-parse --abbrev-ref HEAD
git diff --shortstat <base>...HEAD
git diff --name-only <base>...HEAD
git log --format='%H %s' <base>..HEAD
git log --format='%G? %h %s' <base>..HEAD
git log --format=%B <base>..HEAD
git merge-base --is-ancestor <base> HEAD; echo $?
```

If there are no commits between `<base>` and `HEAD`, stop with:

```text
No commits to audit against <base> - make at least one commit on this branch.
```

## Phase 3 - Protected file set

Use this repo-local protected-path set. It is intentionally specific to
member-service and must not be replaced with a central or generic LFX list.

| Area | Protected paths |
| --- | --- |
| Salesforce integration/cache docs | `docs/agent-guidance/salesforce-integration.md`, `docs/agent-guidance/salesforce-cache.md` |
| Membership API | `cmd/member-api/**`, `gen/**` |
| Current-vs-target architecture | `ARCHITECTURE.md` |
| NATS and project-ID mapping | `.claude/skills/member-service-dev/references/nats-messaging.md`, `internal/infrastructure/nats/**`, `internal/infrastructure/project/**`, `pkg/constants/nats.go`, `pkg/constants/storage.go` |
| Charts | `charts/**` |
| Go dependencies | `go.mod`, `go.sum` |
| Build system | `Makefile` |
| Agent guidance | `CLAUDE.md`, `.claude/skills/**` |
| Service docs | `README.md`, `docs/**` |

Protected files are not automatically wrong. They require explicit PR
description coverage and, where applicable, code-owner or maintainer review.

## Phase 4 - Shape checks

Evaluate these items only:

- **Branch name:** should include an `LFXV2-<number>` ticket and a clear work
  type, for example `feat/LFXV2-1234-member-cache`.
- **JIRA ticket:** at least one `LFXV2-<number>` reference must appear in the
  branch name, commit subject, or commit body.
- **Conventional commits:** each subject should match
  `^(feat|fix|docs|test|refactor|chore|ci|build|perf|style)(\\([a-z0-9._-]+\\))?!?: .+`.
- **Rebase status:** `git merge-base --is-ancestor <base> HEAD` should return
  `0`.
- **DCO + GPG:** every commit should have a `Signed-off-by:` trailer and a
  good or trusted signature from `git log --format='%G?'` (`G`, `U`, or `Y`).
- **Diff size:** use additions/deletions from `git diff --shortstat`. Flag
  very large PRs for split consideration when additions exceed 800 or total
  touched files exceed 30, unless the size is generated `gen/` output from a
  Goa change and that is explicitly documented.
- **Protected files:** intersect `git diff --name-only <base>...HEAD` with
  the protected-path set above.

Emit each finding as:

```json
{
  "severity": "CRITICAL | SHOULD_FIX | NIT",
  "rule": "member-service-pr-shape/<item-id>",
  "message": "...",
  "suggestion": "..."
}
```

Severity guide:

- `CRITICAL`: no commits, no JIRA ticket, unrecoverable base comparison,
  unsigned/missing-DCO commits, or non-conventional commit subjects that will
  block repository policy.
- `SHOULD_FIX`: branch not rebased, large diff, protected files that need PR
  callout or owner review.
- `NIT`: small branch-name clarity issues when the JIRA reference exists
  elsewhere.

## Phase 5 - Render report

```markdown
# Member Service PR Readiness

**Branch:** `<current-branch>` -> `<base>`
**Commits:** N | **Additions:** +A | **Deletions:** -D
**Verdict:** NOT READY | READY WITH CHANGES | READY

## PR-shape sanity

| Check | Status | Detail |
| --- | --- | --- |
| Branch name | PASS | feat/LFXV2-1234-member-cache |
| JIRA ticket | PASS | Found LFXV2-1234 |
| Conventional commits | PASS | All commits match |
| Branch rebased | PASS | origin/main is an ancestor |
| DCO + GPG signing | PASS | 3/3 signed and signed off |
| Diff size | SHOULD_FIX | 940 additions; consider split or explain generated output |
| Protected files | SHOULD_FIX | charts/ and go.mod touched; call out in PR |

## Verdict reasoning

<one line per CRITICAL or SHOULD_FIX finding>
```

Verdict rules:

- `NOT READY`: any `CRITICAL` finding.
- `READY WITH CHANGES`: zero `CRITICAL`, one or more `SHOULD_FIX` findings.
- `READY`: zero `CRITICAL`, zero `SHOULD_FIX`.

## Companion skill

Run `/member-service-preflight` after this shape check. It owns the
mechanical Go preflight: working tree status, license headers, formatting,
lint, build, tests, protected files, commit verification, and change summary.
