<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Root team tuple cleanup

Deletes the legacy team tuples on the root project once each project carries
its own `global_*` team grants.

The fga-sync access reconciler manages team subjects only on `global_*`
relations, so a project update or a `project-cli sync reindex-projects` run
never removes these tuples. This script deletes them directly:

| Subject | Relation |
| --- | --- |
| `team:lf-staff#member` | `auditor` |
| `team:lf-contractor#member` | `auditor` |
| `team:formation#member` | `owner` |
| `team:product-support#member` | `owner` |
| `team:marketing-ops#member` or `team:marketing-ops-<root-uid>#member` | `marketing_ops` |

User tuples on the root project are out of scope. Clear the root project's
writers and auditors through the project settings update instead, so the
stored settings and FGA stay in agreement.

## Usage

```bash
# Dry run (default): reads the root object and prints the plan.
./scripts/root-team-tuple-cleanup/root-team-tuple-cleanup.sh --env dev --root-uid <root-project-uid>

# Apply: asks you to type the root project UID before deleting.
./scripts/root-team-tuple-cleanup/root-team-tuple-cleanup.sh --env dev --root-uid <root-project-uid> --apply
```

The OpenFGA store and model IDs are read from the selected environment's
Heimdall deployment; no environment-specific identifier is committed or
accepted from the caller. `--env prod --apply` requires an interactive
terminal.

Requires `jq` and `kubectl` access to the target context (`lfx-v2-dev` or
`lfx-v2-prod`) with permission to run, read, and delete pods in namespace
`lfx`. The script first resolves `slug/ROOT` from the `projects` NATS KV bucket
and refuses a different `--root-uid`. NATS reads and FGA calls run in
short-lived, digest-pinned CLI pods with service-account token mounting
disabled.

## Safety checks

The script refuses to delete anything (exit 3) when the object:

- does not match the canonical `slug/ROOT` mapping in the projects bucket;
- has a `parent` tuple, so it is not the root project;
- is served by a Heimdall-pinned authorization model whose project `owner`
  still includes `owner from parent`;
- is served by a model whose project `owner`, `auditor` or `marketing_ops` no
  longer accepts `team#member` subjects, because the printed rollback
  commands could not be written back;
- is missing any replacement grant: `global_owner` for `formation` and
  `product-support`, `global_writer` for `global-project-writers`,
  `global_auditor` for `lf-staff` and `global-project-auditors`, and
  `global_marketing_ops` for `marketing-ops`;
- carries a team tuple outside the legacy and replacement lists;
- has a legacy tuple with a condition.

A dry run performed before the fallback-removal model is deployed, or after
the `team#member` restrictions are dropped, prints a warning and continues
through tuple validation, so operators can prepare the plan early. `--apply`
keeps both model checks as hard refusals.

After the confirmation prompt, `--apply` re-reads Heimdall's store and model
IDs, the root identity, the model, and the tuples, and exits 3 without
deleting if the store ID, the model ID, or the delete plan changed.

It never deletes a `user:` subject. After `--apply` it re-reads the object and
exits 4 unless every legacy tuple is gone, every replacement grant is still
present without a condition, and the complete set of user subjects, relations, and conditions is
unchanged. The dry run prints the
`fga tuple write` commands that recreate each tuple it would delete.
Those rollback lines assume `fga` is configured for the same API and store;
run them from an equivalent in-cluster CLI pod if the local CLI cannot reach
the service.

The script cannot prove every rollout dependency. Before `--apply`, operators
must also confirm that:

- all project-scoped application checks use the replacement guard relations;
- project search documents have been republished with the guard relations;
- the per-project global-grant backfill has been reconciled by relation and
  project class;
- the contractor access-removal announcement lead time has elapsed;
- organization-auditor reconciliation no longer derives its team set from the
  legacy root tuples.

Running `scripts/marketing-ops-grant` with `--global` writes a
`marketing_ops` tuple on the root project again; do not use it after this
cleanup.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Nothing to delete, dry run complete, or apply verified |
| 1 | A read failed or returned unexpected output |
| 2 | Invalid arguments, or confirmation refused |
| 3 | Preflight refused the object |
| 4 | Apply incomplete, or post-apply verification failed |

## Tests

```bash
bash scripts/root-team-tuple-cleanup/root-team-tuple-cleanup_test.sh
```

The tests put a fake `kubectl` on `PATH` that emulates the CLI pods against a
JSON tuple file; they need no cluster.
