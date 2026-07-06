# project-cli

A CLI tool for running operational tasks against the project service. Commands are designed to be run manually by engineers or as one-off Kubernetes Jobs after incidents or data migrations.

## Why this exists

Operational scripts such as slug renames need a repeatable, auditable, cluster-native execution path with shared infrastructure wiring, dry-run support, structured logs, and deterministic exit codes. The CLI reuses the project-service NATS and OpenSearch client setup rather than duplicating connection logic in standalone scripts.

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
```

In CI, the image is built and published automatically by the existing `ko-build-*.yaml` workflows alongside the API image.

## Running as a Kubernetes Job

Enable the Helm job template and set slug parameters in `values.yaml`:

```yaml
jobs:
  renameProjectSlug:
    enabled: true
    oldSlug: old-slug
    newSlug: new-slug
    dryRun: "true"
    target: both
```

Then upgrade the chart or render the job manifest:

```sh
helm template lfx-v2-project-service ./charts/lfx-v2-project-service \
  --set jobs.renameProjectSlug.enabled=true \
  --set jobs.renameProjectSlug.oldSlug=old-slug \
  --set jobs.renameProjectSlug.newSlug=new-slug
```

Or create a one-off job from the published image:

```sh
kubectl create job lfx-project-cli-rename-slug \
  --image=ghcr.io/linuxfoundation/lfx-v2-project-service/project-cli:<tag> \
  --namespace=lfx \
  --env="OLD_SLUG=old-slug" \
  --env="NEW_SLUG=new-slug" \
  --env="DRY_RUN=true" \
  -- sync rename-project-slug
```

Monitor progress:

```sh
kubectl logs -f job/lfx-project-cli-rename-slug -n lfx
```

## Adding new commands

1. Create `cmd/project-cli/commands/<group>/` and implement the `Command` and `Subcommand` interfaces from `commands/command.go`.
2. Register the new command in `buildRegistry()` in `cmd/project-cli/main.go`.

No changes to shared infrastructure are required unless the new command needs a client that does not yet exist.
