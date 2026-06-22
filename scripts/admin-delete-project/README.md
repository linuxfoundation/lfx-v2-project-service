# admin-delete-project

Out-of-band admin utility for **removing projects that the regular
`DELETE /projects/:id` API refuses to delete** (e.g. projects whose
`funding_model != ["Crowdfunding"]` but which need to be cleaned up because
they are orphaned / no longer visible in the UI / created in error).

> This script bypasses the API-layer guards. It mirrors the cleanup the
> service would do internally (`internal/service/project_operations.go::DeleteProject` +
> `internal/infrastructure/nats/repository.go::DeleteProject`) by reusing the
> production `MessageBuilder` and the same NATS subjects / KV keys. Use with care.

## What it does, per UID

1. **Audit** (read-only): reads `projects/<uid>`, `projects/slug/<slug>`, and
   `project-settings/<uid>`. Captures all values + revisions to a JSON file.
2. **Child report (non-blocking)**: scans `project-links`, `project-folders`,
   and `project-documents-metadata` for any record that references this project
   and reports them (logged + captured in the audit file). Children do **not**
   block the delete: once the parent project is gone, any remaining children are
   orphaned and inert, so they are intentionally left in place.
3. **Publishes indexer deletes** to NATS (synchronous request/reply by default):
   - `lfx.index.project` with `{"action":"deleted","data":"<uid>"}`
   - `lfx.index.project_settings` with the same shape

   The indexer service consumes these and removes the matching documents from
   OpenSearch.
4. **Deletes NATS KV entries**:
   - `projects/<uid>` with `LastRevision` CAS (the production code path)
   - `projects/slug/<slug>` (no CAS — slug mapping is single-writer)
   - `project-settings/<uid>`

### What it does **not** do

- **OpenFGA cleanup.** `lfx.fga-sync.delete_access` is intentionally skipped.
  Per operator policy, FGA reconciliation is handled out-of-band by another
  job; the messages are idempotent and the orphaned tuples are inert because
  the corresponding `project:<uid>` objects no longer exist anywhere else.
- **OpenSearch write guarantee.** OpenSearch is updated synchronously via the
  indexer NATS subject (request/reply, ack-before-delete). If the indexer is
  unhealthy at the time of run, the publish fails and the script aborts before
  deleting the KV record. Use `--sync=false` for fire-and-forget (not
  recommended for production cleanups). See the verification section below.

## Safety properties

- **Dry-run by default.** `--dry-run=true` is the default; you must explicitly
  pass `--dry-run=false` to perform any writes.
- **Audit-before-write.** The JSON audit file is written to disk *before* any
  mutating call. Keep it as your rollback record.

  > ⚠️ **Sensitive data:** the audit file captures the full `project-settings` KV
  > value, which includes member emails and other PII. Delete the file after you
  > no longer need it as a rollback reference (e.g. once you've confirmed the
  > deletion was successful).
- **Idempotent.** Re-running after partial completion is safe — entries that
  are already gone are logged and skipped.
- **CAS on base delete.** Won't clobber a concurrent update to `projects/<uid>`.
- **Sync indexer publish (default).** We use NATS request/reply so we get an
  ack from a subscriber before deleting the KV record. Disable with `--sync=false`
  for fire-and-forget (not recommended for prod cleanups).
- **Children are non-blocking.** Any `project_link`, `project_folder`, or
  `project_document` referencing the UID is reported but left in place; the
  delete still proceeds.

## Usage

### 1. Port-forward NATS from the prod cluster

```bash
# Replace `lfx` / svc name to match your cluster.
kubectl -n lfx port-forward svc/lfx-platform-nats 4222:4222
```

Optionally, port-forward OpenSearch too — useful for the post-run verification step:

```bash
kubectl -n lfx port-forward svc/opensearch-cluster-master 9200:9200
```

### 2. Build the script

Build into `bin/scripts/` (the whole `bin/` tree is gitignored, so the
compiled binary never gets committed):

```bash
go build -o bin/scripts/admin-delete-project ./scripts/admin-delete-project
```

### 3. Dry-run first (default)

Set `NATS_USER` / `NATS_PASS` to match the credentials in your active NATS
context (`nats context info`). The URL and port must match the local port-forward
endpoint (check with `nats context ls`).

```bash
NATS_USER=local \
NATS_PASS='<password-from-nats-context>' \
./bin/scripts/admin-delete-project \
  --nats-url nats://0.0.0.0:62812 \
  --uid a6efa0cd-b8e2-4389-acb7-c4e77518ad39 \
  --uid 108c0a98-79d2-4406-902d-e6682b34cf97 \
  --skip-child-scan
```

> **Why `--skip-child-scan`?** These two projects have been verified to have no
> child links/folders/documents. Skipping the scan avoids opening four extra
> KV/ObjectStore handles over the port-forward, which would otherwise cause a
> prolonged wait.

This produces:

- Structured log lines describing every key/value it found.
- `./admin-delete-audit-<timestamp>.json` containing the full backup
  (base record, slug-mapping, settings) for every UID.

Review the audit file. Confirm the slugs and names match the projects you
intend to delete.

### 4. Execute

```bash
NATS_USER=local \
NATS_PASS='<password-from-nats-context>' \
./bin/scripts/admin-delete-project \
  --nats-url nats://0.0.0.0:62812 \
  --uid a6efa0cd-b8e2-4389-acb7-c4e77518ad39 \
  --uid 108c0a98-79d2-4406-902d-e6682b34cf97 \
  --skip-child-scan \
  --dry-run=false
```

Watch the logs. Expected per UID:

```text
published indexer delete   subject=lfx.index.project
published indexer delete   subject=lfx.index.project_settings
deleted projects/<uid>     revision=<n>
deleted slug reverse-lookup slug_key=slug/<slug>
deleted project-settings/<uid>
project deleted            uid=<...> slug=<...>
```

### 5. Verify

NATS KV — the records should be gone:

```bash
nats kv get projects a6efa0cd-b8e2-4389-acb7-c4e77518ad39        # expect: key not found
nats kv get projects slug/os-tsc                                  # expect: key not found
nats kv get project-settings a6efa0cd-b8e2-4389-acb7-c4e77518ad39 # expect: key not found
```

OpenSearch — check that no docs remain for the deleted UID:

```bash
curl -s 'http://localhost:9200/resources/_count' \
  -H 'content-type: application/json' \
  -d '{"query":{"term":{"object_id":"<uid>"}}}' | jq '.count'
# expect: 0
```

You should see `0`. If a doc lingers, the indexer service either didn't get the
message (check its logs / consumer lag on the NATS subject) or errored while
processing the delete. As a last-resort manual cleanup you can delete directly:

```bash
curl -X DELETE 'http://localhost:9200/resources/_doc/project:<uid>'
curl -X DELETE 'http://localhost:9200/resources/_doc/project_settings:<uid>'
```

## Rollback

The script uses `kv.Delete()` (not `kv.Purge()`), which writes a tombstone and
preserves the full key history in NATS JetStream. You have two recovery paths.

### Option A — Restore from the NATS KV history (no audit file needed)

```bash
# Inspect history — the revision before the tombstone is the value to restore.
nats kv history projects <uid>
nats kv history projects slug/<slug>
nats kv history project-settings <uid>
```

To restore a specific revision directly:

```bash
# Find the last non-deleted revision number from `nats kv history` output, then:
nats kv get projects <uid> --revision <N>   # confirms value
# JetStream does not have a native "rollback to revision N" CLI command.
# Use Option B (from audit file) to put the exact bytes back.
```

### Option B — Restore from the audit JSON file (recommended)

The audit file written before any delete contains the raw bytes for all three
keys. Replace `AUDIT_FILE` and `UID` with real values.

```bash
AUDIT_FILE=./admin-delete-audit-<timestamp>.json

# --- os-tsc ---
UID=a6efa0cd-b8e2-4389-acb7-c4e77518ad39

# 1. Restore projects/<uid>
jq -r --arg u "$UID" '.[] | select(.uid == $u) | .base.value' "$AUDIT_FILE" \
  | base64 -d \
  | nats kv put projects "$UID" -

# 2. Restore projects/slug/<slug>  (slug_kv.key already contains "slug/os-tsc")
SLUG_KEY=$(jq -r --arg u "$UID" '.[] | select(.uid == $u) | .slug_kv.key' "$AUDIT_FILE")
jq -r --arg u "$UID" '.[] | select(.uid == $u) | .slug_kv.value' "$AUDIT_FILE" \
  | base64 -d \
  | nats kv put projects "$SLUG_KEY" -

# 3. Restore project-settings/<uid>
jq -r --arg u "$UID" '.[] | select(.uid == $u) | .settings.value' "$AUDIT_FILE" \
  | base64 -d \
  | nats kv put project-settings "$UID" -
```

Repeat the same block with `UID=108c0a98-79d2-4406-902d-e6682b34cf97` for
`operations-health-sig`.

If the NATS server requires credentials add `--context nats_development` (or
`NATS_USER` / `NATS_PASS` if using explicit flags).

### OpenSearch rollback

After restoring the NATS KV records, trigger re-indexing by publishing an
upsert message. The indexer service will re-create the OpenSearch documents.

```bash
# Publish an upsert for the project record — the indexer picks it up.
# Replace <uid> and provide the same JSON payload that was in the KV store.
VALUE=$(jq -r --arg u "a6efa0cd-b8e2-4389-acb7-c4e77518ad39" \
  '.[] | select(.uid == $u) | .base.value' "$AUDIT_FILE" | base64 -d)
nats pub lfx.index.project "{\"action\":\"upserted\",\"data\":$VALUE}"

# And for settings:
SETTINGS=$(jq -r --arg u "a6efa0cd-b8e2-4389-acb7-c4e77518ad39" \
  '.[] | select(.uid == $u) | .settings.value' "$AUDIT_FILE" | base64 -d)
nats pub lfx.index.project_settings "{\"action\":\"upserted\",\"data\":$SETTINGS}"
```

Verify with:

```bash
curl -s 'http://localhost:9200/resources/_search' \
  -H 'content-type: application/json' \
  -d '{"query":{"term":{"object_id.keyword":"a6efa0cd-b8e2-4389-acb7-c4e77518ad39"}}}' | jq '.hits.total'
# expect: {"value": 1, ...}
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `--nats-url` | `$NATS_URL` or `nats://localhost:4222` | NATS server URL |
| `--nats-user` | `$NATS_USER` | NATS username (required when server needs auth) |
| `--nats-password` | `$NATS_PASS` | NATS password (required when server needs auth) |
| `--uid` | _required, repeatable_ | Project UID to delete |
| `--dry-run` | `true` | Audit + plan only; no writes. Pass `--dry-run=false` to execute. |
| `--audit-file` | `./admin-delete-audit-<ts>.json` | Path for the rollback JSON file |
| `--sync` | `true` | Synchronous indexer publish (request/reply) |
| `--skip-child-scan` | `false` | Skip scanning child KV buckets (use when children already verified absent) |
| `--cascade-children` | `false` | Also delete child links/folders/documents (default: leave orphaned) |
| `--verbose` | `false` | Verbose logging |
