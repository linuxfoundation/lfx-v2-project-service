#!/usr/bin/env bash
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
#
# Run the existing project-cli full-fleet access republish as a one-off
# Kubernetes Job. This wrapper does not compute or write tuples itself.
#
# Dry-run by default. Add --apply to pass --update to project-cli.
#
# --documents-only republishes the project and settings documents without the
# access message: no OpenFGA tuple is written or deleted, so it is the run to
# use before the grants run.

set -euo pipefail

PROJECT_CLI_REPOSITORY="ghcr.io/linuxfoundation/lfx-v2-project-service/project-cli"
NS="lfx"
# The project-service deployment runs in its own namespace; the Job runs in $NS so that the
# NATS address it reads from that deployment resolves the same way for the Job.
PS_NS="project-service"

usage() {
  echo "usage: $(basename "$0") --env dev|prod --image <project-cli@sha256:digest> [--concurrency N] [--documents-only] [--apply]" >&2
  exit 2
}

die() {
  local code="$1"; shift
  echo "error: $*" >&2
  exit "$code"
}

ENV_NAME=""
IMAGE=""
CONCURRENCY=50
APPLY=false
DOCUMENTS_ONLY=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env) [[ $# -ge 2 ]] || usage; ENV_NAME="$2"; shift 2 ;;
    --image) [[ $# -ge 2 ]] || usage; IMAGE="$2"; shift 2 ;;
    --concurrency) [[ $# -ge 2 ]] || usage; CONCURRENCY="$2"; shift 2 ;;
    --documents-only) DOCUMENTS_ONLY=true; shift ;;
    --apply) APPLY=true; shift ;;
    -h|--help) usage ;;
    *) echo "unknown argument: $1" >&2; usage ;;
  esac
done

case "$ENV_NAME" in
  dev) CTX="lfx-v2-dev" ;;
  prod) CTX="lfx-v2-prod" ;;
  "") die 2 "--env is required (dev or prod)" ;;
  *) die 2 "unknown --env '${ENV_NAME}' (expected dev or prod)" ;;
esac

IMAGE_PREFIX="${PROJECT_CLI_REPOSITORY}@sha256:"
[[ "$IMAGE" == "${IMAGE_PREFIX}"* && "${IMAGE#"$IMAGE_PREFIX"}" =~ ^[0-9a-f]{64}$ ]] ||
  die 2 "--image must pin the project-cli repository by sha256 digest"
[[ "$CONCURRENCY" =~ ^[1-9][0-9]*$ ]] ||
  die 2 "--concurrency must be a positive integer"
((CONCURRENCY <= 200)) || die 2 "--concurrency must not exceed 200"

resolve_nats_url() {
  local deployments urls count
  deployments="$(kubectl --context "$CTX" --request-timeout=10s get deployments -n "$PS_NS" \
    -l app.kubernetes.io/name=lfx-v2-project-service -o json)" ||
    die 1 "could not read the project-service deployment"
  urls="$(jq -r '
    [.items[].spec.template.spec.containers[].env[]?
      | select(.name == "NATS_URL") | .value]
    | map(select(type == "string" and length > 0))
    | unique[]
  ' <<<"$deployments")" || die 1 "could not parse NATS_URL from the project-service deployment"
  count="$(grep -c . <<<"$urls" || true)"
  [[ "$count" -eq 1 ]] ||
    die 1 "expected one NATS_URL on the project-service deployment, found ${count}"
  NATS_URL="$urls"
}

confirm_apply() {
  [[ "$APPLY" == true ]] || return 0
  if [[ "$ENV_NAME" == "prod" && ! -t 0 ]]; then
    die 2 "--env prod --apply requires an interactive terminal"
  fi
  local expected="apply global grants to ${ENV_NAME}" answer=""
  [[ "$DOCUMENTS_ONLY" == true ]] && expected="republish project documents to ${ENV_NAME}"
  printf 'Type "%s" to start the write-enabled Job: ' "$expected" >&2
  read -r answer || true
  [[ "$answer" == "$expected" ]] || die 2 "confirmation did not match; no Job created"
}

job_manifest() {
  local name="$1" args_json
  local args=(sync reindex-projects --all)
  [[ "$DOCUMENTS_ONLY" == true ]] || args+=(--include-access)
  args+=(--concurrency "$CONCURRENCY")
  [[ "$APPLY" == true ]] && args+=(--update)
  args_json="$(printf '%s\n' "${args[@]}" | jq -R . | jq -cs .)"

  jq -cn \
    --arg name "$name" \
    --arg image "$IMAGE" \
    --arg natsURL "$NATS_URL" \
    --argjson args "$args_json" '{
      apiVersion: "batch/v1",
      kind: "Job",
      metadata: {
        name: $name,
        namespace: "lfx",
        labels: {
          "app.kubernetes.io/name": "project-global-grant-backfill",
          "app.kubernetes.io/component": "operations"
        }
      },
      spec: {
        backoffLimit: 0,
        activeDeadlineSeconds: 7200,
        ttlSecondsAfterFinished: 604800,
        template: {
          metadata: {labels: {"app.kubernetes.io/name": "project-global-grant-backfill"}},
          spec: {
            restartPolicy: "Never",
            automountServiceAccountToken: false,
            containers: [{
              name: "project-cli",
              image: $image,
              imagePullPolicy: "IfNotPresent",
              args: $args,
              env: [
                {name: "NATS_URL", value: $natsURL},
                {name: "LOG_LEVEL", value: "info"}
              ],
              securityContext: {
                allowPrivilegeEscalation: false,
                capabilities: {drop: ["ALL"]},
                readOnlyRootFilesystem: true,
                runAsNonRoot: true
              },
              resources: {
                requests: {cpu: "100m", memory: "128Mi"},
                limits: {cpu: "500m", memory: "512Mi"}
              }
            }]
          }
        }
      }
    }'
}

main() {
  command -v jq >/dev/null || die 2 "jq is required"
  resolve_nats_url
  confirm_apply

  local mode="dry-run" prefix="project-global-grants" name manifest
  [[ "$APPLY" == true ]] && mode="apply"
  [[ "$DOCUMENTS_ONLY" == true ]] && prefix="project-documents"
  name="${prefix}-${mode}-$(date -u +%Y%m%d%H%M%S)-$$"
  manifest="$(job_manifest "$name")"

  echo "Creating ${mode} Job ${name} in ${ENV_NAME}; image digest is pinned."
  printf '%s\n' "$manifest" |
    kubectl --context "$CTX" --request-timeout=10s create -f - >/dev/null ||
    die 1 "could not create Job ${name}"

  echo "Job created. It is retained for seven days for logs and exit status."
  echo "Inspect with:"
  echo "  kubectl --context ${CTX} -n ${NS} get job/${name}"
  echo "  kubectl --context ${CTX} -n ${NS} logs job/${name}"
}

main "$@"
