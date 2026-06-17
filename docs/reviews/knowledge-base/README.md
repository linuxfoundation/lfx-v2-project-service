<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Project Service review knowledge base

Empirical review patterns for `lfx-v2-project-service`, mined from this repo's
merged-PR review history (CodeRabbit, Copilot, and human maintainers). Each
entry encodes a pattern that was actually flagged on this repo and cleared the
promotion gate. This KB is the empirical surface; it does **not** duplicate the
documented rule audit owned by `lfx-skills:lfx-project-service-code-reviewer` or
the generic senior review owned by `lfx-skills:lfx-general-code-reviewer`.

Consumed by the `lfx-skills:lfx-project-service-learnings-reviewer` subagent,
which routes category files by changed-file path, matches each pattern's
`Detect:` rule against the diff, and applies `known-false-positives.md` last.

## Methodology

Built per the shared Service Review-KB Research Playbook
(`lfx-architecture-scratch/2026-05-DevX-Time-to-Merge/service-kb-research-playbook.md`).
Corpus: merged PRs only, full available history. Comments pulled from three
surfaces per PR (inline review threads, review bodies, issue/PR conversation)
via the `gh` CLI, with GraphQL `reviewThreads` resolution state as the acted-on
signal. Candidates were clustered, then promoted only if they cleared all hard
gates (repo-specific, mechanically detectable + fixable, currently relevant on
`origin/main`, not already enforced by gofmt/golangci-lint/go vet/CI) and at
least one value signal (recurrence ≥2 PRs, cost-of-miss for a
security/data/contract issue, or acted-on by a maintainer).

## Corpus stats

- **Merged PRs sampled:** 70 (full merged history, PR #1–#76; gaps are PRs not
  merged).
- **Comments by author bucket** (distinct review comments):
  - Inline review comments: 179 total — andrest50 66 (PR author, mostly replies),
    Copilot 25, jordane 22, coderabbitai[bot] 20, mauriciozanettisalomao 15,
    prabodhcs 12, bramwelt 12, emsearcy 7.
  - Review bodies: 38 — coderabbitai[bot] 16, copilot-pull-request-reviewer[bot]
    13, plus human reviewers.
  - Issue/PR-conversation comments: CodeRabbit walkthroughs on ~PRs #7–#60.
- **CodeRabbit:** ACTIVE. Posts inline comments, review-body nitpick/outside-diff
  blockquotes, and per-PR walkthrough issue-comments (from ~PR #7 onward).
- **Copilot:** ACTIVE (login `Copilot` for inline, `copilot-pull-request-reviewer[bot]`
  for review bodies).
- **Human reviewers / maintainers:** jordane, mauriciozanettisalomao, prabodhcs,
  bramwelt, emsearcy, dealako. Their comments are weighted highest.

The single most review-flagged surface is the Helm chart
(`charts/lfx-v2-project-service/`), followed by `internal/service/*_operations.go`,
the Goa design (`api/project/v1/design/types.go`), and
`internal/infrastructure/nats/`.

## Categories

| File | Patterns | Read when |
| --- | --- | --- |
| `chart-and-helm.md` | 5 | `charts/lfx-v2-project-service/**` changed |
| `nats-and-messaging.md` | 4 | `internal/service/*_operations.go`, `project_subscriber.go`, `document_subscriber.go`, `internal/infrastructure/nats/**`, `internal/domain/message.go`, `pkg/events/**`, or a new publish goroutine |
| `goa-design-and-validation.md` | 4 | `api/project/v1/design/**`, `api/project/v1/gen/**`, `cmd/project-api/service_endpoint_*.go`, or `charts/.../templates/ruleset.yaml` |
| `converters-and-errors.md` | 3 | `internal/service/converters.go`, `*_operations.go`, `internal/domain/errors.go`, `internal/domain/message.go`, `internal/infrastructure/nats/repository.go`, `pkg/events/**`, or `service_endpoint_project.go` |
| `logging-and-pii.md` | 2 | `internal/service/project_subscriber.go`, `internal/service/document_subscriber.go`, `internal/service/email/**`, `internal/infrastructure/middleware/**`, or any `slog.*Context` on user/notification data |
| `known-false-positives.md` | (floor filter) | always |

Total: 18 promoted patterns + the false-positive floor.

## Highest-value patterns

- `nats-and-messaging/goroutine-captures-request-ctx` and `/xsync-not-honored`
  (PR #60) — background indexer publishes that drop silently on request teardown,
  or async-when-sync-was-requested. Data-integrity + contract; recurred across
  every link/folder/document operation.
- `chart-and-helm/control-line-indentation` (PRs #7, #10) — the repo's most
  recurrent break; `helm template | kubectl apply` failures from `{{- if }}` at
  list-item indentation.
- `logging-and-pii/raw-user-identifiers-in-logs` (PR #70) — flagged blocking by a
  human maintainer; LFID/email in notification logs.
- `goa-design-and-validation/format-rejects-empty-string` (PRs #53, #63, #71) —
  `Format(FormatURI/Email)` on optional fields breaking GET→PUT round-trips.

## Maintenance

This is a living KB. As new PRs merge and accrue review history, re-run the
playbook's harvest, cluster new signal, and promote only what clears the gate.
Demote anything that becomes enforced by tooling or refactored away.

_Built 2026-05-29 from merged PRs #1–#76._
