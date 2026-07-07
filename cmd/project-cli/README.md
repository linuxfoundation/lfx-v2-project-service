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

**Subcommand flags**

| Flag | Default | Description |
|---|---|---|
| `--old-slug` | `""` | Current slug (or first positional arg) |
| `--new-slug` | `""` | New slug (or second positional arg) |
| `--dry-run` | `true` | Preview changes without writing |
| `--concurrency` | `50` | Max concurrent NATS KV record updates per bucket |
| `--nats-buckets` | see below | Comma-separated KV bucket names |

Flag defaults can be overridden by environment variables (useful in Kubernetes Jobs):

| Env var | Default | Description |
|---|---|---|
| `OLD_SLUG` | — | Current slug (alternative to `--old-slug` or positional args) |
| `NEW_SLUG` | — | New slug (alternative to `--new-slug` or positional args) |
| `DRY_RUN` | `true` | Preview without writing (`true`/`false`) |
| `CONCURRENCY` | `50` | Max concurrent NATS KV updates per bucket |
| `NATS_BUCKETS` | see below | Comma-separated KV bucket names |

Default `--nats-buckets` / `NATS_BUCKETS`:

`committee-members`, `committees`, `committee-settings`, `projects`, `project-settings`

Provide slugs either as two positional args **or** via `--old-slug` / `--new-slug` (or `OLD_SLUG` / `NEW_SLUG` env vars), not both.

**Exit code:** `0` on success, `1` on failure.

**Output:** Structured JSON logs include `job_run_id`, `old_slug`, `new_slug`, and `dry_run`. Per-store summaries log `store` (`opensearch` or `nats`), counts, and `dry_run`.

**Examples**

Dry-run across OpenSearch and NATS (safe first step):

```sh
NATS_URL=nats://localhost:4222 OPENSEARCH_URL=http://localhost:9200 \
  ./bin/project-cli sync rename-project-slug old-slug new-slug

# Or without building first:
NATS_URL=nats://localhost:4222 OPENSEARCH_URL=http://localhost:9200 \
  go run ./cmd/project-cli sync rename-project-slug old-slug new-slug
```

Apply changes (review dry-run logs first):

```sh
./bin/project-cli sync rename-project-slug --dry-run=false old-slug new-slug
# Or:
go run ./cmd/project-cli sync rename-project-slug --dry-run=false old-slug new-slug
```

Lower NATS KV concurrency:

```sh
./bin/project-cli sync rename-project-slug --dry-run=false --concurrency=20 old-slug new-slug
```

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
            - name: DRY_RUN
              value: "true"
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

Run a live migration by setting `DRY_RUN` to `"false"` (and reviewing dry-run logs first).

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

## Adding new commands

1. Create `cmd/project-cli/commands/<group>/` and implement the `Command` and `Subcommand` interfaces from `commands/command.go`.
2. Register the new command in `buildRegistry()` in `cmd/project-cli/main.go`.
3. Use `pkg/env` for flag defaults backed by environment variables when Jobs need env-based configuration.

No changes to the Helm chart are required. Document the new command here with a `kubectl create job` or Job manifest example when it is intended for in-cluster use.
