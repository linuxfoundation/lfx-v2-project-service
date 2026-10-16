# project-cli

A CLI tool for running operational tasks against the project service. Commands are designed to be run manually by engineers or as one-off Kubernetes Jobs after incidents or data migrations.

## Why this exists

Operational scripts such as slug renames need a repeatable, auditable, cluster-native execution path with shared infrastructure wiring, dry-run support, structured logs, and deterministic exit codes. The CLI reuses the project-service NATS and OpenSearch client setup rather than duplicating connection logic in standalone scripts.

Like `committee-cli`, **the Helm chart deploys only the API**. Operational jobs are not rendered by the chart — create a one-off Kubernetes Job when you need to run a command in-cluster.

`main` loads shared infrastructure config (`NATS_URL`, `OPENSEARCH_URL`) into `RunContext`. Each subcommand dials the clients it needs in its own `Run()` (for example `rename-project-slug` connects NATS JetStream and OpenSearch). Subcommand flags may read defaults from environment variables via `pkg/env`.

## Usage

No binary is committed to the repository. Build it first or use `go run` directly:

```sh
# Build once, then run the binary
go build -o bin/project-cli ./cmd/project-cli
./bin/project-cli <command> <subcommand> [subcommand flags]

# Or run without building (useful for one-off local runs)
go run ./cmd/project-cli <command> <subcommand> [subcommand flags]
```

To print the build version, run `project-cli version` (or `project-cli --version`). A binary built with `make build-cli` or the `ko` workflow reports its version, git commit and build time; a plain `go build` reports `dev`.

### Environment variables (global)

These env vars are read in `main` and passed to subcommands via `RunContext`. Subcommands establish connections in their own `Run()`.

| Env var | Default | Description |
|---|---|---|
| `NATS_URL` | `nats://localhost:4222` | NATS server address |
| `OPENSEARCH_URL` | `http://localhost:9200` | OpenSearch base URL |
| `LOG_LEVEL` | `info` | Log verbosity (e.g. `debug`) |
| `JOB_RUN_ID` | pod hostname or UUID | Run identifier included in structured logs |

### Commands

#### `sync rename-project-slug`

Renames a project slug across **both** OpenSearch (`resources` index) and NATS JetStream KV buckets in a single run. Connects to NATS and OpenSearch at the start of `Run()`. OpenSearch is updated first, then NATS KV.

For the `projects` bucket, the migration also keeps the secondary `slug/<slug>` -> uid lookup index (used by `GetProjectUIDFromSlug` and the `lfx.projects-api.slug_to_uid` NATS RPC) in sync with the record's `slug` field. Reconciliation is driven off each record's **observed current slug**, not whether the current run performed the field write, so a record left inconsistent by a prior partial failure (field renamed, index still stale) is repaired on rerun instead of becoming permanently unresolvable. A pre-flight check fails the operation for a given record (without mutating it) if `slug/<newSlug>` already resolves to a different project.

**Subcommand flags**

| Flag | Default | Description |
|---|---|---|
| `--old-slug` | `""` | Current slug (or first positional arg) |
| `--new-slug` | `""` | New slug (or second positional arg) |
| `--update` | `false` | Write changes to OpenSearch and NATS KV (default is preview-only, like the other `sync` subcommands) |
| `--dry-run` | `true` | Deprecated: previewing is the default. `--dry-run=false` still works and means `--update`; combining it with a contradictory `--update` is an error |
| `--concurrency` | `50` | Max concurrent NATS KV record updates per bucket |
| `--nats-buckets` | see below | Comma-separated KV bucket names |

Flag defaults can be overridden by environment variables (useful in Kubernetes Jobs):

| Env var | Default | Description |
|---|---|---|
| `OLD_SLUG` | — | Current slug (alternative to `--old-slug` or positional args) |
| `NEW_SLUG` | — | New slug (alternative to `--new-slug` or positional args) |
| `UPDATE` | `false` | Write changes (`true`/`false`) |
| `DRY_RUN` | `true` | Deprecated: `DRY_RUN=false` still means `UPDATE=true`. Setting `DRY_RUN` to any value logs a deprecation warning, and setting it together with `UPDATE` fails the run with "conflicting write mode" unless the two agree, so remove `DRY_RUN` when you add `UPDATE` |
| `CONCURRENCY` | `50` | Max concurrent NATS KV updates per bucket |
| `NATS_BUCKETS` | see below | Comma-separated KV bucket names |

Default `--nats-buckets` / `NATS_BUCKETS`:

`committee-members`, `committees`, `committee-settings`, `projects`, `project-settings`

Provide slugs either as two positional args **or** via `--old-slug` / `--new-slug` (or `OLD_SLUG` / `NEW_SLUG` env vars), not both.

**Exit code:** `0` on success, `1` on failure.

**Output:** Structured JSON logs include `job_run_id`, `old_slug`, `new_slug`, and `dry_run`. Per-store summaries log `store` (`opensearch` or `nats`), counts, and `dry_run`.

**Examples**

Preview across OpenSearch and NATS (safe first step, nothing is written):

```sh
NATS_URL=nats://localhost:4222 OPENSEARCH_URL=http://localhost:9200 \
  ./bin/project-cli sync rename-project-slug old-slug new-slug

# Or without building first:
NATS_URL=nats://localhost:4222 OPENSEARCH_URL=http://localhost:9200 \
  go run ./cmd/project-cli sync rename-project-slug old-slug new-slug
```

Apply changes (review the preview logs first):

```sh
./bin/project-cli sync rename-project-slug --update old-slug new-slug
# Or:
go run ./cmd/project-cli sync rename-project-slug --update old-slug new-slug
```

Lower NATS KV concurrency:

```sh
./bin/project-cli sync rename-project-slug --update --concurrency=20 old-slug new-slug
```

#### `sync document-audit-users`

Backfills `created_by` and `updated_by` user profile objects on project folders, links, and document metadata stored in NATS KV, then publishes indexer `ActionUpdated` messages so OpenSearch picks up display names for the Shared By column.

Connects to NATS at the start of `Run()`. Does **not** use OpenSearch directly — re-indexing is driven by indexer NATS messages.

**Subcommand flags**

| Flag | Default | Description |
|---|---|---|
| `--update` | `false` | Write KV changes and publish indexer messages (default is preview-only) |
| `--sleep` | `0` | Pause between auth-service profile lookups (e.g. `200ms`, `1s`) |
| `--project-uid` | `""` | Limit migration to one project |
| `--resource-type` | `""` | Optional filter: `folder`, `link`, or `document` |
| `--reindex-only` | `false` | Re-publish indexer messages without KV writes (recovery after partial migration) |

**Exit code:** `0` on success, `1` on failure.

**Examples**

Preview all document resources (default):

```sh
NATS_URL=nats://localhost:4222 \
  go run ./cmd/project-cli sync document-audit-users
```

Apply for one project with rate limiting:

```sh
go run ./cmd/project-cli sync document-audit-users \
  --update --project-uid=<uid> --sleep=200ms
```

Migrate documents only:

```sh
go run ./cmd/project-cli sync document-audit-users \
  --update --resource-type=document --sleep=200ms
```

#### `sync reindex-projects`

Diffs project and project_settings documents in the OpenSearch `resources` index against
the NATS KV project records, then republishes indexer messages for any documents found
missing. Use this to repair projects that exist in KV but never made it into the search
index (and are therefore invisible to the query service and self-serve), without touching
KV data. FGA data is untouched by default; pass `--include-access` to also republish the
FGA access message.

The hidden `ROOT` project (used for team permission assignment) is excluded from full
scans, including `--all`, since it must not appear in the search index. Passing
`--project-uid` with its UID explicitly still reindexes it.

Connects to NATS at the start of `Run()`. Connects to OpenSearch unless `--all` or
`--force` is set (see below).

**Subcommand flags**

| Flag | Default | Description |
|---|---|---|
| `--update` | `false` | Publish indexer messages (default is preview-only) |
| `--project-uid` | `""` | Limit to a single project UID (default all) |
| `--concurrency` | `50` | Max concurrent project republishes |
| `--all` | `false` | Republish every project regardless of OpenSearch state, skipping the diff and the OpenSearch connection entirely; the `ROOT` project is still excluded |
| `--force` | `false` | Requires `--project-uid`. Skips the OpenSearch diff for that one project and unconditionally republishes it, even if its documents already exist (e.g. to repair stale field data such as a mismatched `slug`) |
| `--include-access` | `false` | Also republish the FGA access message. Fires only for projects with at least one missing OpenSearch document, or when combined with `--all` or `--force` |
| `--keep-direct-users` | `false` | With `--include-access`, mark the settings-derived user relations as excluded, so fga-sync writes every tuple the message carries but does not delete a direct user tuple that project settings do not list. For backfills; ordinary edits keep the full sync. Requires `--include-access` |

**Exit code:** `0` on success, `1` on failure.

**Examples**

Preview which documents are missing across all projects:

```sh
go run ./cmd/project-cli sync reindex-projects
```

Repair a single project (only republishes documents confirmed missing from OpenSearch):

```sh
go run ./cmd/project-cli sync reindex-projects --project-uid=<uid> --update
```

Force-repair a single project even though its OpenSearch document already exists, e.g. to
fix a stale/incorrect field such as `slug`:

```sh
go run ./cmd/project-cli sync reindex-projects --project-uid=<uid> --force --update
```

Force a full republish of every project, bypassing the OpenSearch diff:

```sh
go run ./cmd/project-cli sync reindex-projects --all --update
```

Republish FGA access for every project (full FGA repair):

```sh
go run ./cmd/project-cli sync reindex-projects --all --include-access --update
```

#### `sync verify-global-grants`

Performs a read-only comparison of the expected global team grants derived
from the complete NATS project set with the `global_*` tuples stored in
OpenFGA. Expected policy is defined independently from the production grant
builder so a builder regression cannot change both sides of the comparison.

The JSON report contains aggregate counts by project class and relation only
and does not print tuple subjects. Existing NATS repository diagnostics can
name a failing project key, so retain logs in an access-controlled system. The
command exits nonzero for any OpenFGA read error, missing tuple, unexpected
tuple, or conditioned global grant. A conditioned tuple cannot satisfy the
unconditioned contract, so it is counted as conditioned, missing, and
unexpected.

Required environment variables are `NATS_URL`, `OPENFGA_API_URL`, and
`OPENFGA_STORE_ID`. OpenFGA reads request higher consistency. Before reading
projects, the command confirms that `OPENFGA_STORE_ID` names an existing store,
because OpenFGA answers reads against an unknown store with an empty result.

```sh
go run ./cmd/project-cli sync verify-global-grants --concurrency=20
```

For cluster execution, use the digest-pinned wrapper documented in
`scripts/global-grant-backfill/README.md`.

### Inspect commands

The `inspect` group is read-only. It never writes to NATS, OpenSearch or OpenFGA. Each command prints a JSON report to stdout and exits nonzero when it finds a problem, so it can gate a Job. Findings name project UIDs, slugs and record UIDs, so keep the logs in an access-controlled system.

#### `inspect project`

Shows one project across every store: the KV base and settings records, the `slug/<slug>` index entry, whether the OpenSearch `project` and `project_settings` documents exist, the `global_*` OpenFGA grants against the expected set, and the number of links, folders and documents. Pass the project UID or its slug. Flags go before the argument.

| Flag | Default | Description |
|---|---|---|
| `--skip-opensearch` | `false` | Do not check the OpenSearch documents |
| `--skip-fga` | `false` | Do not check the OpenFGA grants |

The OpenFGA check runs when `OPENFGA_API_URL` and `OPENFGA_STORE_ID` are set, and is reported as skipped when both are unset. The hidden `ROOT` project is never indexed, so its OpenSearch check is skipped. A conditioned global grant is reported once, as conditioned, where `sync verify-global-grants` also counts it as missing and unexpected; both fail the run.

```sh
go run ./cmd/project-cli inspect project --skip-fga example-project
```

#### `inspect slug-index`

Checks every `slug/<slug>` entry in the `projects` bucket against its project record, and every project against its slug entry. It reports `dangling_index` (the entry points at a UID with no record), `mismatched_index` (the record has a different slug), `missing_index` (the project has no entry), `wrong_index` (the entry points at another project) and `unreadable_record`. These are the states `sync rename-project-slug` repairs.

#### `inspect orphans`

Lists links, folders and documents whose project no longer exists (`orphan_link`, `orphan_folder`, `orphan_document`). `admin-delete-project` leaves these children behind on purpose, so they accumulate.

#### `inspect index-drift`

Lists projects whose `project` or `project_settings` OpenSearch document is missing (`missing_project_document`, `missing_settings_document`). It runs the same diff as `sync reindex-projects`, without publishing anything, and leaves out the hidden `ROOT` project. Needs `NATS_URL` and `OPENSEARCH_URL`.

```sh
go run ./cmd/project-cli inspect index-drift
```

#### `inspect settings-access`

Compares each project's direct OpenFGA tuples with what its settings call for: the `writer`, `auditor`, `meeting_coordinator`, `executive_director` and `mentorship_program_admin` rosters, plus the `parent` tuple. The expectation comes from the same message builder that `sync reindex-projects --include-access` publishes, so an email-only user, who has no username and no tuple, is not reported. Findings are `access_missing`, `access_unexpected` and `access_conditioned` with a count per relation and no usernames, plus `missing_settings` and `access_read_error`. To repair a project, run `sync reindex-projects --project-uid <uid> --force --include-access` (or `--all --include-access` for the fleet). Without `--all` or `--force`, reindex-projects skips a project whose OpenSearch documents both exist, and `--keep-direct-users` leaves `access_unexpected` tuples in place. It needs `NATS_URL`, `OPENFGA_API_URL` and `OPENFGA_STORE_ID`, and takes `--concurrency` (default 20, at most 100). Global team grants are checked by `sync verify-global-grants`.

#### `inspect children`

Checks that every link, folder and document record has its lookup key, and that every lookup key has a record: `link_missing_lookup`, `link_stale_lookup`, `link_wrong_lookup` and the same for `folder` and `document`. A link without its key is invisible to the project's link list; a folder or document without one can have its name reused, and a stale key blocks its name. It also reports `document_missing_file` for a document with no file in the object store, and `<resource>_unreadable_record`. `--skip-files` leaves out the object store check, which reads one object's metadata per document.

#### `inspect parents`

Reports projects whose `parent_uid` names a project that does not exist (`missing_parent`), is the project itself (`self_parent`), or loops back through its own ancestors (`parent_cycle`, once per cycle). A bad parent breaks the access a project inherits.

## Building

### Local binary

```sh
make build-cli
# produces bin/project-cli
```

Or directly with Go:

```sh
go build -o bin/project-cli ./cmd/project-cli
```

### Docker image

```sh
make docker-build-cli
# tags: ghcr.io/linuxfoundation/lfx-v2-project-service/project-cli:latest
# (via DOCKER_CLI_IMAGE in the Makefile; ko/CI publish the same registry path)
```

In CI, the image is built and published automatically by the existing `ko-build-*.yaml` workflows alongside the API image.

## Running as a Kubernetes Job

Create a one-off Job from the published `project-cli` image. Replace `<tag>` with the image tag you want to run (for example the chart `appVersion` or a CI build tag).

`kubectl create job` does not set environment variables. For in-cluster runs you need cluster `NATS_URL`, `OPENSEARCH_URL`, and slug settings — use the manifest below.

### Recommended: apply a Job manifest

Save as `rename-project-slug-job.yaml`, edit slugs and URLs for your environment, then apply:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  generateName: lfx-project-cli-rename-slug-
  namespace: lfx
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 604800
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: project-cli
          image: ghcr.io/linuxfoundation/lfx-v2-project-service/project-cli:<tag>
          args:
            - sync
            - rename-project-slug
          env:
            - name: NATS_URL
              value: nats://lfx-platform-nats.lfx.svc.cluster.local:4222
            - name: OPENSEARCH_URL
              value: http://opensearch-cluster-master.lfx.svc.cluster.local:9200
            - name: LOG_LEVEL
              value: info
            - name: JOB_RUN_ID
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: OLD_SLUG
              value: old-slug
            - name: NEW_SLUG
              value: new-slug
            - name: UPDATE
              value: "false"
            - name: CONCURRENCY
              value: "50"
          resources:
            limits:
              cpu: 500m
              memory: 512Mi
            requests:
              cpu: 100m
              memory: 128Mi
```

```sh
kubectl apply -f rename-project-slug-job.yaml
```

Run a live migration by setting `UPDATE` to `"true"` (and reviewing the preview logs first). Manifests that still set `DRY_RUN` keep working but log a deprecation warning for any value. When switching a manifest to `UPDATE`, delete its `DRY_RUN` entry: leaving `DRY_RUN: "true"` next to `UPDATE: "true"` fails the run with a conflicting-write-mode error rather than guessing.

### Quick start (`kubectl create job`)

Only suitable for local/minimal setups where default `NATS_URL` / `OPENSEARCH_URL` inside the cluster are acceptable (usually not in LFX deployments):

```sh
kubectl create job lfx-project-cli-rename-slug-dry-run \
  --image=ghcr.io/linuxfoundation/lfx-v2-project-service/project-cli:<tag> \
  --namespace=lfx \
  -- sync rename-project-slug old-slug new-slug
```

### Monitor and re-run

```sh
kubectl get jobs -n lfx | grep project-cli-rename-slug
kubectl logs -f -n lfx job/<job-name>
```

The Job is kept after completion so its logs and exit status remain accessible as a run record. Re-trigger by applying a new manifest (`generateName` assigns a unique name) or creating a new `kubectl create job`.

## Code layout

```text
cmd/project-cli/
  main.go, args.go      entry point, argument splitting, usage
  commands/             the command framework and the command groups
    command.go, flags.go, nats.go   interfaces and helpers every subcommand uses
    sync/               a command group: what an operator runs as `project-cli sync ...`
    inspect/            a command group: `project-cli inspect ...` (read-only)
  internal/             libraries the command groups call; they are not commands
    osdiff/  fgaread/  rootproject/
```

Anything under `commands/<group>/` is something an operator can run. Code that more than one group needs goes in `internal/<name>/` (importable only inside `cmd/project-cli`), so a command group never imports another group's internals. Helpers that every subcommand needs (flags, NATS setup, stats) stay in `commands/`.

## Adding new commands

1. Create `cmd/project-cli/commands/<group>/` and implement the `Command` and `Subcommand` interfaces from `commands/command.go`. Build the group's `Subcommands()` with `commands.IndexSubcommands(...)` so each name is declared once, on the subcommand itself.
2. Register the new command in `buildRegistry()` in `cmd/project-cli/main.go`.
3. In a subcommand's `Run`, use the shared helpers in `commands/` instead of repeating the boilerplate:
   - `commands.NewFlagSet`, `commands.ParseFlags` and `commands.RejectExtraArgs` for flags, `--help` and stray arguments.
   - `commands.ConnectRepository` to dial NATS and open the project repository, and `commands.WithServiceAuthorization` for code that reads the authorization from the context.
   - `internal/osdiff` for the OpenSearch document diff, `internal/fgaread` for the OpenFGA grant reader and its expected-grant contract, and `internal/rootproject` for the hidden ROOT project's slug.
   - `commands.NewStats` and `commands.Finish` to count records, log the summary, flush NATS and turn failures into a nonzero exit.
4. Use `pkg/env` for flag defaults backed by environment variables when Jobs need env-based configuration.

No changes to the Helm chart are required. Document the new command here with a `kubectl create job` or Job manifest example when it is intended for in-cluster use.
