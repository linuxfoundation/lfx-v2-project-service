<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Known false positives — applied LAST in every review pass

Findings that match any pattern below MUST be dropped, regardless of which
source (KB pattern file, rule file, checklist) produced them. This list is the
floor — even a quotable pattern does not survive if it matches a known false
positive.

Used by the `/project-service-learnings-reviewer` subagent (Step 4), the
knowledge-base reviewer of the pre-PR review round. The round's general
(`/lfx-skills:lfx-general-code-review`) and security
(`/lfx-skills:lfx-security-engineer`) reviewers do not read this file; it
filters knowledge-base findings only.

---

## Already enforced by tooling

### License-header complaints when the header is present

**Pattern matched:** finding states a `.go` / `.md` file is missing the
`Copyright The Linux Foundation` / `SPDX-License-Identifier: MIT` header when
`head -2 <file>` confirms it is present.

**Why false:** `make license-check`, the `license-header-check.yml` workflow,
and MegaLinter already enforce this. If CI passes, the header is there — the
bot misread it.

**Source:** `Makefile` (`license-check` target), `.github/workflows/license-header-check.yml`.

### "Run gofmt / golangci-lint / go vet" with no concrete rule

**Pattern matched:** any finding whose only substance is "run `make fmt`",
"`make lint`", "`go vet`", "`make build`", "`make test`" or "`make apigen`"
without a concrete violation, or a formatting/import-ordering nit on
hand-written Go.

**Why false:** deterministic tooling owns this. CI runs `make apigen`,
`make build`, `make build-cli` and `go test ./...`
(`.github/workflows/project-api-build.yml`), the license-header check
(`.github/workflows/license-header-check.yml`) and MegaLinter
(`.github/workflows/mega-linter.yml`); locally `/project-service-preflight`
runs `make check` (gofmt + lint + license), build, tests and generated-code
freshness. Re-surfacing it in a review is duplicate signal.

**Source:** `Makefile` (`check`, `build`, `build-cli`, `test`, `verify`
targets), `.github/workflows/project-api-build.yml`, `.mega-linter.yml`,
`revive.toml`, `.claude/skills/project-service-preflight/SKILL.md`.

### PR-shape findings: branch, Jira, DCO, GPG, rebase, diff size, protected files

**Pattern matched:** a code-review finding about the branch name, a missing
`LFXV2-NNNN` reference, a commit subject's conventional-commit shape, a missing
`Signed-off-by:` trailer or GPG signature, an unrebased branch, total diff
size, or a touched protected file lacking a PR-body note.

**Why false:** that surface has its own owners in this repo — the
`.githooks/commit-msg` hook (conventional subject shape, lowercase summary, no
trailing period, 72-character subject cap, `Signed-off-by:` trailer) and
`/project-service-pr-readiness` (branch name, presence of an `LFXV2-[0-9]+`
reference, conventional commits, rebase, DCO + GPG, diff size, protected
files). A code reviewer repeating them is duplicate signal.

Not covered: a *placeholder or wrong* ticket number such as `LFXV2-0000`.
Neither owner rejects it — the hook only prints `[LFXV2-NNNN]` in its example
text and never matches the ticket, and readiness accepts any `LFXV2-[0-9]+` —
so a reviewer finding about it is not a duplicate and stays.

**Source:** `.githooks/commit-msg` (subject regex line 21, summary checks
lines 37–59, DCO check lines 61–71);
`.claude/skills/project-service-pr-readiness/SKILL.md` (Phase 3 checks).
Carried over 2026-09-25 from the retired `project-service-code-reviewer`
skill's "Known False Positives" list, not from a PR thread.

---

## Go-version churn

### "Bump go.mod to the latest Go release"

**Pattern matched:** CodeRabbit/Copilot suggesting the `go` directive in `go.mod`
be raised to the newest released Go (e.g. "update to 1.26.3 for the latest
features"), or warning that a `go 1.25` bump will break MegaLinter.

**Why false:** the module's `go` version is driven by Minimum Version Selection
from dependencies (e.g. the invite/email service), not by chasing the newest
release. The team intentionally tracks the dependency-driven version and the
linter image; the bump suggestion is noise.

**Source:** PR #70 `go.mod:5` — maintainer accepted the MVS explanation and
CodeRabbit dropped the suggestion ("MVS doing its job by honouring the `go`
directive from the invite-service dependency. Happy to drop the suggestion").

---

## Go-language misreads

### Loop-variable capture in goroutines (Go 1.22+)

**Pattern matched:** finding that a `for _, x := range xs { g.Go(func(){ ... x ... }) }`
goroutine captures a shared loop variable and will observe the wrong/last value,
recommending `x := x` rebinding.

**Why false:** this repo targets Go 1.25 (`go.mod` declares `go 1.25.0`). Since
Go 1.22 the loop variable is re-bound per iteration, so each closure captures its
own copy. The classic capture hazard does not apply here.

**Source:** PR #70 `internal/service/project_subscriber.go` — "Not a bug in Go
1.22+. Starting with Go 1.22, loop variables are re-bound on each iteration ...
This repo targets Go 1.25, so the classic re-use hazard no longer applies here."
(Note: the detached-context goroutine pattern in `nats-and-messaging.md` is a
*separate, real* issue — that one is about ctx cancellation, not loop variables.)

### "String comparison of errors is fragile" when the code already uses `errors.Is`

**Pattern matched:** Copilot comment saying error handling does "string
comparison of error messages" / "string-based error checking" and recommending
`errors.Is`, on a line that already calls `errors.Is(err, domain.Err...)` or
`errors.Is(err, jetstream.Err...)`.

**Why false:** Copilot repeatedly misread the existing `errors.Is(...)` calls as
string comparison. The code already uses the recommended pattern.

**Source:** PR #2 `internal/service/project_operations.go:144`, `:341`, `:344`,
`:432`, `:492` and `internal/infrastructure/nats/repository.go:322`, `:398`,
`:451` — every flagged line already used `errors.Is`.

---

## Generated code

### Style/refactor nits on `api/project/v1/gen/**`

**Pattern matched:** any maintainability/refactor/"simplify this switch"/"extract
a variable" suggestion targeting a file under `api/project/v1/gen/**` (or the old
`cmd/project-api/gen/**`).

**Why false:** that tree is Goa-generated and tracked, regenerated by
`make apigen`; hand edits are overwritten. The correct response is to change the
design, not the generated file. (A genuine *unpaired* generated diff — gen
changed without a design change, or vice versa — is real and belongs to
`goa-design-and-validation/hand-edited-generated-code`.)

**Source:** PR #1 `cmd/project-api/gen/http/cli/project_service/cli.go:187` — the
default-case suggestion was withdrawn: "Sorry, I didn't notice it is from the gen
folder. We are good here." PR #16 `server.go:618` — "code generated by goa and
should not be modified directly."

---

## Premature abstraction nits

### "Extract this into a helper function" for a single/low-use block

**Pattern matched:** Copilot suggesting a small (3-5 line) duplicated block be
pulled into a named helper, when it is used once or twice and inlining is
clearer.

**Why false:** premature abstraction. These were consistently treated as
optional nits, not acted on, on this repo.

**Source:** PR #16 `cmd/project-api/main.go:182` ("extract `koDataDir`"), PR #25
`scripts/root-project-setup/main.go:87` ("extract `parseCommaSeparatedValues`"),
PR #19 `internal/infrastructure/middleware/request_logger.go:40` ("extract `isHealthCheckPath`").

---

## Review-automation quirks

### CodeRabbit `🏁 Script executed:` / verification dumps

**Pattern matched:** any text quoting a CodeRabbit `🏁 Script executed:`,
`🧩 Analysis chain`, `🧠 Learnings used`, `🤖 Prompt for AI Agents`, or
`💡 Verification agent` block.

**Why false:** internal CodeRabbit reasoning/reconnaissance, not a finding.
Surfacing it is noise.

### CodeRabbit collapsed nitpick / outside-diff blockquotes

**Pattern matched:** items inside a CodeRabbit `🧹 Nitpick comments (N)` or
`🔭 Outside diff range comments (N)` collapsed `<summary>` block that were not
also posted as actionable inline comments.

**Why false:** these are demoted-tier by design (CodeRabbit's "Actionable
comments posted: N" counts only the inline ones). Promote only if the same item
recurs across PRs or was explicitly acted on.

### Copy-editing suggestions on docs / Chart.yaml / README

**Pattern matched:** rewording suggestions for `Chart.yaml` descriptions, README
phrasing, or doc copy that are purely cosmetic and unrelated to a contract.

**Why false:** out of scope; the bots flag copy on every touched doc and the team
does not act on cosmetic rewordings. (A contract-doc *content* drift — e.g.
`docs/indexer-contract.md` not matching a publisher change — is real and is owned
by `/lfx-skills:lfx-general-code-review`, not this list.)

---

## How to add a new entry

When a finding from CodeRabbit / Copilot / a reviewer is one the team has
explicitly decided is not relevant for this repo:

1. Add an entry with **Pattern matched**, **Why false**, and (where applicable)
   **Source** (PR #N + quote, or the tool/config that already enforces it).
   Exception: an entry carried over from the retired
   `project-service-code-reviewer` skill has no PR thread behind it; its
   **Source** line says so and carries the date it was carried over.
2. If the pattern previously lived in a category file, remove it there — don't
   keep it in both places.
3. Add only patterns the bots will surface repeatedly, or a one-time misread
   worth recording. This file should grow slowly; past ~30 entries, re-audit.
