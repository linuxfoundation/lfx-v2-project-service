#!/usr/bin/env bash
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
#
# Create a read-only Job that compares the project set in NATS with the
# corresponding global_* tuples in OpenFGA. The verifier's report contains
# aggregate counts only; see the README for what its logs can contain.

set -euo pipefail

PROJECT_CLI_REPOSITORY="ghcr.io/linuxfoundation/lfx-v2-project-service/project-cli"
FGA_API_URL="http://lfx-platform-openfga:8080"
NS="lfx"

usage() {
  echo "usage: $(basename "$0") --env dev|prod --image <project-cli@sha256:digest> [--concurrency N]" >&2
  exit 2
}

die() {
  local code="$1"; shift
  echo "error: $*" >&2
  exit "$code"
}

ENV_NAME=""
IMAGE=""
CONCURRENCY=20
while [[ $# -gt 0 ]]; do
  case "$1" in
    --env) [[ $# -ge 2 ]] || usage; ENV_NAME="$2"; shift 2 ;;
    --image) [[ $# -ge 2 ]] || usage; IMAGE="$2"; shift 2 ;;
    --concurrency) [[ $# -ge 2 ]] || usage; CONCURRENCY="$2"; shift 2 ;;
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
((CONCURRENCY <= 100)) || die 2 "--concurrency must not exceed 100"

deployment_env_value() {
  local label="$1" name="$2" deployments values count
  deployments="$(kubectl --context "$CTX" --request-timeout=10s get deployments -n "$NS" \
    -l "app.kubernetes.io/name=${label}" -o json)" ||
    die 1 "could not read the ${label} deployment"
  values="$(jq -r --arg name "$name" '
    [.items[].spec.template.spec.containers[].env[]?
      | select(.name == $name) | .value]
    | map(select(type == "string" and length > 0))
    | unique[]
  ' <<<"$deployments")" || die 1 "could not parse ${name} from the ${label} deployment"
  count="$(grep -c . <<<"$values" || true)"
  [[ "$count" -eq 1 ]] ||
    die 1 "expected one ${name} on the ${label} deployment, found ${count}"
  printf '%s\n' "$values"
}

main() {
  command -v jq >/dev/null || die 2 "jq is required"
  local nats_url store_id name args_json manifest
  nats_url="$(deployment_env_value lfx-v2-project-service NATS_URL)"
  store_id="$(deployment_env_value heimdall OPENFGA_STORE_ID)"
  [[ "$store_id" =~ ^[0-9A-HJKMNP-TV-Z]{26}$ ]] ||
    die 1 "Heimdall OPENFGA_STORE_ID is not a ULID"

  name="verify-global-grants-$(date -u +%Y%m%d%H%M%S)-$$"
  args_json="$(printf '%s\n' sync verify-global-grants --concurrency "$CONCURRENCY" | jq -R . | jq -cs .)"
  manifest="$(jq -cn \
    --arg name "$name" \
    --arg image "$IMAGE" \
    --arg natsURL "$nats_url" \
    --arg fgaURL "$FGA_API_URL" \
    --arg storeID "$store_id" \
    --argjson args "$args_json" '{
      apiVersion: "batch/v1",
      kind: "Job",
      metadata: {
        name: $name,
        namespace: "lfx",
        labels: {
          "app.kubernetes.io/name": "project-global-grant-verification",
          "app.kubernetes.io/component": "operations"
        }
      },
      spec: {
        backoffLimit: 0,
        activeDeadlineSeconds: 3600,
        ttlSecondsAfterFinished: 604800,
        template: {
          metadata: {labels: {"app.kubernetes.io/name": "project-global-grant-verification"}},
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
                {name: "OPENFGA_API_URL", value: $fgaURL},
                {name: "OPENFGA_STORE_ID", value: $storeID},
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
    }')"

  printf '%s\n' "$manifest" |
    kubectl --context "$CTX" --request-timeout=10s create -f - >/dev/null ||
    die 1 "could not create verification Job ${name}"
  echo "Read-only verification Job ${name} created in ${ENV_NAME}."
  echo "Inspect with:"
  echo "  kubectl --context ${CTX} -n ${NS} get job/${name}"
  echo "  kubectl --context ${CTX} -n ${NS} logs job/${name}"
}

main "$@"
