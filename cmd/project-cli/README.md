# project-cli

A CLI tool for running operational tasks against the project service. Commands are designed to be run manually by engineers or as one-off Kubernetes Jobs after incidents or data migrations.

## Why this exists

Operational scripts such as slug renames need a repeatable, auditable, cluster-native execution path with shared infrastructure wiring, dry-run support, structured logs, and deterministic exit codes. The CLI reuses the project-service NATS and OpenSearch client setup rather than duplicating connection logic in standalone scripts.

Like `committee-cli`, **the Helm chart deploys only the API**. Operational jobs are not rendered by the chart — create a one-off Kubernetes Job when you need to run a command in-cluster.

## Usage

```text
project-cli <command> <subcommand> [subcommand flags]
```

### Environment variables

| Env var | Default | Description |
|---|---|---|
| `NATS_URL` | `nats://localhost:4222` | NATS server address |
| `OPENSEARCH_URL` | `http://localhost:9200` | OpenSearch base URL |
| `LOG_LEVEL` | `debug` | Log verbosity (e.g. `info`) |
| `JOB_RUN_ID` | pod hostname or UUID | Run identifier included in structured logs |
| `OLD_SLUG` | — | Current slug (Job env alternative to flags) |
| `NEW_SLUG` | — | New slug (Job env alternative to flags) |
| `DRY_RUN` | `true` | Preview without writing (`true`/`false`) |
| `TARGET` | `both` | `opensearch`, `nats`, or `both` |
| `CONCURRENCY` | `50` | Max concurrent NATS KV updates per bucket |
| `NATS_BUCKETS` | see command help | Comma-separated KV buckets to scan |

### Commands

#### `sync rename-project-slug`

Renames a project slug across OpenSearch (`resources` index) and NATS JetStream KV buckets.

**Subcommand flags**

| Flag | Default | Description |
|---|---|---|
| `--old-slug` | `""` | Current slug (or first positional arg) |
| `--new-slug` | `""` | New slug (or second positional arg) |
| `--target` | `both` | Stores to migrate: `opensearch`, `nats`, or `both` |
| `--dry-run` | `true` | Preview changes without writing |
| `--concurrency` | `50` | Max concurrent NATS KV record updates per bucket |
| `--nats-buckets` | committee + project buckets | Comma-separated KV bucket names |

**Exit code:** `0` on success, `1` on failure.

**Examples**

Dry-run against both stores (safe first step):

```sh
NATS_URL=nats://localhost:4222 OPENSEARCH_URL=http://localhost:9200 \
  project-cli sync rename-project-slug old-slug new-slug
```

Apply OpenSearch changes only:

```sh
project-cli sync rename-project-slug --target=opensearch --dry-run=false old-slug new-slug
```

Apply NATS KV changes with lower concurrency:

```sh
project-cli sync rename-project-slug --target=nats --dry-run=false --concurrency=20 old-slug new-slug
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

### Quick start (`kubectl create job`)

Use this when the command only needs flags and the image defaults are acceptable for your cluster:

```sh
kubectl create job lfx-project-cli-rename-slug-dry-run \
  --image=ghcr.io/linuxfoundation/lfx-v2-project-service/project-cli:<tag> \
  --namespace=lfx \
  -- sync rename-project-slug old-slug new-slug
```

`kubectl create job` does not set environment variables. For in-cluster runs you typically need `NATS_URL`, `OPENSEARCH_URL`, and slug settings — use the manifest below instead.

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
            - name: TARGET
              value: both
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

### Monitor and re-run

```sh
kubectl get jobs -n lfx | grep project-cli-rename-slug
kubectl logs -f job/<job-name> -n lfx
```

The Job is kept after completion so its logs and exit status remain accessible as a run record. Re-trigger by creating a new Job (new `generateName` prefix or a different job name).

## Adding new commands

1. Create `cmd/project-cli/commands/<group>/` and implement the `Command` and `Subcommand` interfaces from `commands/command.go`.
2. Register the new command in `buildRegistry()` in `cmd/project-cli/main.go`.

No changes to the Helm chart are required. Document the new command here with a `kubectl create job` or Job manifest example when it is intended for in-cluster use.
