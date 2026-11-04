<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Marketing Ops Grant

An ops-only utility for granting/revoking Marketing Ops (Campaign Impact and Campaigns tab)
access by adding or removing a user's membership of the LF-wide `team:marketing-ops`, with
built-in post-action verification.

## When to use this

- Giving a user Marketing Ops access across every project, or removing it.
- Ad-hoc verification of what a given FGA store actually resolves for a user (`check` mode).

There is no per-project Marketing Ops grant. The authorization model has no `marketing_ops`
relation, and per-project team objects (`team:marketing-ops-<uid>`) are not part of the access
model: teams are global, and project-service grants them on each project through the `global_*`
relations (see [`docs/fga-contract.md`](../../docs/fga-contract.md)). The `--project`,
`--global` and `--root-uid` options were removed and now fail with an explanation.

## Requirements

- `kubectl` pointed at the target cluster context, with permission to list
  deployments and to `run`/`get`/`logs`/`delete` pods in namespace `lfx`.
- `jq`.
- The UID of an active project to verify the result on (`--verify-project`).

## How it works

project-service writes `team:marketing-ops#member` on the `global_marketing_ops` relation of every
project whose stage is not `Prospect` or `Formation - Confidential`. In the
[FGA model](https://github.com/linuxfoundation/lfx-v2-helm/blob/main/charts/lfx-platform/files/model.fga),
`global_marketing_ops` feeds `campaign_manager` on that project and the cascading
`marketing_auditor`, so membership of `team:marketing-ops` is the whole grant.

`campaign_manager` is withheld on `Prospect` and `Formation - Confidential` projects, so pass an
active project as `--verify-project`; a grant verified on a withheld project fails its
`campaign_manager` check.

`grant` writes one tuple, `user:<username> member team:marketing-ops`, then re-checks
`marketing_auditor` and `campaign_manager` on the verify project and the membership tuple with
`--consistency HIGHER_CONSISTENCY` (bypassing OpenFGA's check-query cache), and fails loudly if any
of them aren't `true`. `revoke` deletes that tuple, asserts membership is `false`, and *reports*
(without asserting) the two derived relations, since they have independent grant paths
(`executive_director`, or a direct `marketing_auditor` team grant) that the revoke can't touch.
`check` reports the same three values without asserting anything.

`grant`/`revoke` are safe to re-run: writing a tuple that already exists, or deleting one that's
already gone, is treated as success rather than an error (`--on-duplicate`/`--on-missing ignore`).
This requires OpenFGA server **v1.10.0+**; confirm the running version in the target environment
(`kubectl --context <ctx> -n lfx exec <openfga-pod> -- /openfga version`) before relying on it.

## Usage

```bash
./marketing-ops-grant.sh grant  --env dev|prod --user <username> --verify-project <uid>
./marketing-ops-grant.sh revoke --env dev|prod --user <username> --verify-project <uid>
./marketing-ops-grant.sh check  --env dev|prod --user <username> --verify-project <uid>
```

### Examples

```bash
# Grant a user Marketing Ops access on every project, verified on one active project
./marketing-ops-grant.sh grant --env prod --user alice.example --verify-project 00000000-0000-0000-0000-000000000001

# Just report what's currently granted, without changing anything or asserting a result
./marketing-ops-grant.sh check --env prod --user alice.example --verify-project 00000000-0000-0000-0000-000000000001

# Revoke
./marketing-ops-grant.sh revoke --env prod --user alice.example --verify-project 00000000-0000-0000-0000-000000000001
```

Every prod `grant` and `revoke` changes access on every project, and `--user` is not validated
against the auth service, so both prompt you to re-type the username before writing anything.

## Configuring environments

The `dev`/`prod` kubectl context aliases are set near the top of `marketing-ops-grant.sh`
(`lfx-v2-dev` / `lfx-v2-prod`). Add the prod kubectl context first if it isn't already configured —
see your AWS/EKS access docs for the profile, region, and cluster name to use; it maps to whatever
alias `marketing-ops-grant.sh` expects for `--env prod`:

```bash
aws eks update-kubeconfig --region <region> --name <cluster-name> --profile <your-prod-profile> --alias lfx-v2-prod
```

The script reads the OpenFGA store ID from the selected environment's Heimdall
deployment. No environment-specific store ID is committed or accepted through
`FGA_STORE_ID`. For a prod grant or revoke it reads the store ID again after the
username confirmation and exits without writing if it changed.

Dev and prod are separate AWS accounts and EKS clusters (both happen to be named `lfx-v2`) with
independently seeded FGA stores — a `kubectl` context pointed at the wrong one will silently read
or write the wrong environment's data. The script always pins `--context "$CTX"` explicitly per
`--env`, so your shell's *current* context doesn't affect what it does — instead, confirm the named
context actually points where you expect, e.g. `kubectl --context lfx-v2-prod cluster-info`, before
running `grant` or `revoke`.
