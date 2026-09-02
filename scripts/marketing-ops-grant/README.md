# Marketing Ops Grant

An ops-only utility for granting/revoking Marketing Ops (Campaign Impact and Campaigns tab)
access via direct OpenFGA tuple writes, with built-in post-action verification.

## When to use this vs. the self-serve API

`POST /projects/:uid/marketing-ops-members` and `DELETE /projects/:uid/marketing-ops-members/:username`
(see the main [README](../../README.md)) are the self-serve way to grant/revoke Marketing Ops access
scoped to a single project — no cluster access required, the username is validated against the auth
service, and the call is audited like any other API request. **Use the API for normal requests.**

Use this script only for:

- Granting access across **all** projects at once (`--global`). The API intentionally does not
  expose this — a single self-serve call granting org-wide access has a much bigger blast radius
  than a per-project grant, so it stays gated behind whoever already has prod cluster access.
- Ad-hoc verification of what a given FGA store actually contains for a user (`check` mode).

## Requirements

- `kubectl` pointed at the target cluster context, with permission to `run`/`get`/`logs`/`delete`
  pods in namespace `lfx`.
- The environment's OpenFGA store ID (see below) — the script does not discover it for you.

## How it works

Marketing Ops access flows through a per-project OpenFGA team object, `team:marketing-ops-<uid>`,
which is granted the project's `marketing_ops` relation. `marketing_ops` implies both
`marketing_auditor` and `campaign_manager` (see the [FGA model](https://github.com/linuxfoundation/lfx-v2-helm/blob/main/charts/lfx-platform/files/model.fga)),
so one team grant covers all three. Membership in that team, not the team->project reference
itself, is what's granted or revoked per user.

`--global` targets the special `ROOT` project (see `scripts/root-project-setup`) instead of a
specific project UID. Because `marketing_ops` also resolves `from parent`, a grant on `ROOT`
cascades down to every project in the hierarchy — the same mechanism a project-scoped grant uses,
just applied one level higher.

Every `grant`/`revoke` run automatically re-checks `marketing_ops`, `marketing_auditor`,
`campaign_manager`, and the underlying team membership tuple with `--consistency HIGHER_CONSISTENCY`
(bypassing OpenFGA's check-query cache) and fails loudly if any of them don't match the expected
post-action state.

## Usage

```bash
./marketing-ops-grant.sh grant  --env dev|prod --user <username> (--project <uid>|--global)
./marketing-ops-grant.sh revoke --env dev|prod --user <username> (--project <uid>|--global)
./marketing-ops-grant.sh check  --env dev|prod --user <username> (--project <uid>|--global)
```

### Examples

```bash
# Grant a user Marketing Ops access on one project
./marketing-ops-grant.sh grant --env prod --user alice.example --project 00000000-0000-0000-0000-000000000001

# Grant a user Marketing Ops access on every project
./marketing-ops-grant.sh grant --env prod --user alice.example --global

# Just verify what's currently granted, without changing anything
./marketing-ops-grant.sh check --env prod --user alice.example --project 00000000-0000-0000-0000-000000000001

# Revoke
./marketing-ops-grant.sh revoke --env prod --user alice.example --project 00000000-0000-0000-0000-000000000001
```

`revoke` only removes the user's team membership — the team->project `marketing_ops` reference is
left in place by design, matching the API's behavior. Access is controlled purely by team
membership, which stays trivially re-grantable without re-establishing the reference tuple.

## Configuring environments

The `dev`/`prod` OpenFGA store IDs and kubectl contexts are set near the top of
`marketing-ops-grant.sh`. Update them if either environment's store is recreated, and add the prod
kubectl context first if it isn't already configured:

```bash
aws eks update-kubeconfig --region us-west-2 --name lfx-v2 --profile lfx-prod-readonly --alias lfx-v2-prod
```

Dev and prod are separate AWS accounts and EKS clusters (both happen to be named `lfx-v2`) with
independently seeded FGA stores — a `kubectl` context pointed at the wrong one will silently read
or write the wrong environment's data. Confirm the intended context (`kubectl config current-context`)
before running `grant` or `revoke`.
