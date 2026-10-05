#!/usr/bin/env bash
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
#
# Remove the legacy team tuples on the root project once the per-project
# global_* grants have replaced them.
#
# fga-sync's access reconciler only manages team subjects on global_*
# relations, so these five tuples on the root object are never removed by a
# project update or a reindex. They have to be deleted directly:
#
#   team:lf-staff#member           auditor
#   team:lf-contractor#member      auditor
#   team:formation#member          owner
#   team:product-support#member    owner
#   team:marketing-ops#member      marketing_ops   (or team:marketing-ops-<root-uid>#member)
#
# User tuples on the root project (the writers and auditors lists) are out of
# scope: they are cleared through the project settings update so the stored
# settings and FGA stay in agreement.
#
# Dry run by default. Nothing is deleted without --apply and a typed
# confirmation of the root project UID.
#
# Usage:
#   root-team-tuple-cleanup.sh --env dev|prod --root-uid <uuid> [--apply]
#
# The OpenFGA store and model IDs are resolved from the target environment's
# Heimdall deployment and are never committed or accepted as arguments.
#
# Exit codes:
#   0  nothing to delete, dry run complete, or apply verified
#   1  a read failed or returned unexpected output
#   2  invalid arguments or confirmation refused
#   3  preflight refused the object (see message)
#   4  apply did not complete or post-apply verification failed

set -euo pipefail

FGA_CLI_IMAGE="openfga/cli:v0.7.20@sha256:26acde96d90420e53fe361a740dcceba4b67f1c49e4f35117cd0c19c83ac5068"
NATS_CLI_IMAGE="natsio/nats-box@sha256:ffce8bd103383f179f8c7f11cf645726acf5d17280706c530c3b342dbe16334c"
FGA_API_URL="http://lfx-platform-openfga:8080"
NATS_URL="nats://lfx-platform-nats.lfx.svc.cluster.local:4222"
NS="lfx"

LEGACY_TUPLES=(
  "team:lf-staff#member auditor"
  "team:lf-contractor#member auditor"
  "team:formation#member owner"
  "team:product-support#member owner"
  "team:marketing-ops#member marketing_ops"
)

REPLACEMENT_TUPLES=(
  "team:formation#member global_owner"
  "team:product-support#member global_owner"
  "team:global-project-writers#member global_writer"
  "team:lf-staff#member global_auditor"
  "team:global-project-auditors#member global_auditor"
  "team:marketing-ops#member global_marketing_ops"
)

usage() {
  echo "usage: $(basename "$0") --env dev|prod --root-uid <uuid> [--apply]" >&2
  exit 2
}

die() {
  local code="$1"; shift
  echo "error: $*" >&2
  exit "$code"
}

ENV_NAME=""
ROOT_UID=""
APPLY=false
APPLY_STARTED=false

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --env) [[ $# -ge 2 ]] || usage; ENV_NAME="$2"; shift 2 ;;
      --root-uid) [[ $# -ge 2 ]] || usage; ROOT_UID="$2"; shift 2 ;;
      --apply) APPLY=true; shift ;;
      -h|--help) usage ;;
      *) echo "unknown argument: $1" >&2; usage ;;
    esac
  done
}

validate_root_uid() {
  if [[ "$(printf '%s' "$ROOT_UID" | tr '[:lower:]' '[:upper:]')" == "ROOT" ]]; then
    die 2 "--root-uid takes the root project UID, not its slug"
  fi
  if ! [[ "$ROOT_UID" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]]; then
    die 2 "--root-uid must be a lowercase UUID"
  fi
}

select_environment() {
  [[ -z "${FGA_STORE_ID:-}" ]] ||
    die 2 "FGA_STORE_ID is resolved from Heimdall; unset it to avoid an ambiguous target"
  case "$ENV_NAME" in
    dev) CTX="lfx-v2-dev" ;;
    prod) CTX="lfx-v2-prod" ;;
    "") die 2 "--env is required (dev or prod)" ;;
    *) die 2 "unknown --env '${ENV_NAME}' (expected dev or prod)" ;;
  esac
  OBJECT="project:${ROOT_UID}"
  MARKETING_ALT_TUPLE="team:marketing-ops-${ROOT_UID}#member marketing_ops"
}

masked_store_id() {
  printf '%s...%s' "${STORE_ID:0:4}" "${STORE_ID: -4}"
}

CURRENT_POD=""
POD_SEQ=0
POD_STATE_FILE=""

init_pod_state_file() {
  POD_STATE_FILE="$(mktemp "${TMPDIR:-/tmp}/root-team-tuple-cleanup-pod.XXXXXX")" ||
    die 1 "could not create a private pod state file"
  chmod 600 "$POD_STATE_FILE" ||
    die 1 "could not secure the pod state file"
}

cleanup_pod() {
  local pod="$CURRENT_POD"
  if [[ -z "$pod" && -n "$POD_STATE_FILE" && -s "$POD_STATE_FILE" ]]; then
    pod="$(cat "$POD_STATE_FILE")"
  fi
  if [[ -n "$pod" && -n "${CTX:-}" ]]; then
    kubectl --context "$CTX" --request-timeout=10s delete pod "$pod" -n "$NS" --ignore-not-found >/dev/null 2>&1 || true
  fi
  CURRENT_POD=""
  [[ -z "$POD_STATE_FILE" ]] || rm -f -- "$POD_STATE_FILE"
}

handle_signal() {
  local code="$1"
  cleanup_pod
  if [[ "$APPLY_STARTED" == true ]]; then
    echo "interrupted during apply; state may be partial, re-run the dry run" >&2
  fi
  trap - EXIT
  exit "$code"
}

trap cleanup_pod EXIT
trap 'handle_signal 130' INT
trap 'handle_signal 143' TERM

pod_overrides() {
  local pod="$1" image="$2" env_json="$3"; shift 3
  local args_json
  # Arguments travel as newline-separated JSON strings rather than through
  # jq --args, which would parse fga flags such as --object as jq options.
  args_json="$(printf '%s\n' "$@" | jq -R . | jq -cs .)" || return 1
  jq -cn --arg name "$pod" --arg image "$image" --argjson env "$env_json" \
    --argjson args "$args_json" '{
    spec: {
      automountServiceAccountToken: false,
      containers: [{
        name: $name,
        image: $image,
        args: $args,
        env: $env,
        securityContext: {allowPrivilegeEscalation: false, capabilities: {drop: ["ALL"]}},
        resources: {requests: {cpu: "50m", memory: "64Mi"}, limits: {cpu: "100m", memory: "128Mi"}}
      }]
    }
  }'
}

# run_cli_pod IMAGE ENV_JSON ARG...   -- runs one ephemeral CLI pod, prints its logs and
# deletes it. Returns non-zero if the pod could not be created, did not reach
# Succeeded within 120s, or its logs could not be fetched.
run_cli_pod() {
  local image="$1" env_json="$2"; shift 2
  POD_SEQ=$((POD_SEQ + 1))
  local pod="root-tuple-cleanup-$$-${POD_SEQ}"
  local overrides
  if ! overrides="$(pod_overrides "$pod" "$image" "$env_json" "$@")" || [[ -z "$overrides" ]]; then
    echo "could not build pod spec for ${pod}" >&2
    return 1
  fi
  CURRENT_POD="$pod"
  printf '%s' "$pod" >"$POD_STATE_FILE"
  if ! kubectl --context "$CTX" --request-timeout=10s run "$pod" -n "$NS" --image="$image" \
      --restart=Never --overrides="$overrides" >/dev/null; then
    echo "pod ${pod} could not be created" >&2
    cleanup_pod
    return 1
  fi

  local phase="" deadline
  deadline=$(($(date +%s) + 120))
  while :; do
    phase="$(kubectl --context "$CTX" --request-timeout=10s get pod "$pod" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    [[ "$phase" == "Succeeded" || "$phase" == "Failed" ]] && break
    (($(date +%s) >= deadline)) && break
    sleep 2
  done

  local out rc=0
  out="$(kubectl --context "$CTX" --request-timeout=10s logs "$pod" -n "$NS" 2>&1)" || rc=1
  cleanup_pod
  printf '%s\n' "$out"
  if [[ "$phase" != "Succeeded" ]]; then
    echo "pod ${pod} did not succeed (phase=${phase:-timed out})" >&2
    return 1
  fi
  return "$rc"
}

run_fga_pod() {
  local env_json
  env_json="$(jq -cn --arg url "$FGA_API_URL" --arg store "$STORE_ID" \
    '[{name: "FGA_API_URL", value: $url}, {name: "FGA_STORE_ID", value: $store}]')"
  run_cli_pod "$FGA_CLI_IMAGE" "$env_json" "$@"
}

read_canonical_root_uid() {
  local uid
  uid="$(run_cli_pod "$NATS_CLI_IMAGE" '[]' nats kv get projects slug/ROOT --raw --server "$NATS_URL")" || return 1
  if ! [[ "$uid" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]]; then
    echo "slug/ROOT returned an invalid UID" >&2
    return 1
  fi
  printf '%s\n' "$uid"
}

verify_root_identity() {
  local canonical_uid
  canonical_uid="$(read_canonical_root_uid)" || die 1 "could not resolve the canonical ROOT project UID"
  [[ "$ROOT_UID" == "$canonical_uid" ]] || die 3 "--root-uid does not match the canonical slug/ROOT mapping"
}

resolve_heimdall_openfga_config() {
  local deployments store_ids model_ids store_count model_count
  deployments="$(kubectl --context "$CTX" --request-timeout=10s get deployments -n "$NS" \
    -l app.kubernetes.io/name=heimdall -o json)" || return 1
  store_ids="$(jq -r '
    [.items[].spec.template.spec.containers[].env[]?
      | select(.name == "OPENFGA_STORE_ID") | .value]
    | unique[]
  ' <<<"$deployments")" || return 1
  model_ids="$(jq -r '
    [.items[].spec.template.spec.containers[].env[]?
      | select(.name == "OPENFGA_AUTH_MODEL_ID") | .value]
    | unique[]
  ' <<<"$deployments")" || return 1
  store_count="$(grep -c . <<<"$store_ids" || true)"
  model_count="$(grep -c . <<<"$model_ids" || true)"
  [[ "$store_count" -eq 1 ]] || {
    echo "expected one store ID on the Heimdall deployment, found ${store_count}" >&2
    return 1
  }
  [[ "$model_count" -eq 1 ]] || {
    echo "expected one model ID on the Heimdall deployment, found ${model_count}" >&2
    return 1
  }
  [[ "$store_ids" =~ ^[0-9A-HJKMNP-TV-Z]{26}$ ]] || {
    echo "Heimdall OPENFGA_STORE_ID is not a ULID" >&2
    return 1
  }
  [[ "$model_ids" =~ ^[0-9A-HJKMNP-TV-Z]{26}$ ]] || {
    echo "Heimdall OPENFGA_AUTH_MODEL_ID is not a ULID" >&2
    return 1
  }
  STORE_ID="$store_ids"
  GATEWAY_MODEL_ID="$model_ids"
}

verify_deployed_model() {
  local model owner_definition
  model="$(run_fga_pod model get --model-id "$GATEWAY_MODEL_ID" --format fga)" ||
    die 1 "could not read Heimdall's deployed authorization model"
  owner_definition="$(awk '
    /^type project$/ { in_project=1; next }
    /^type / { in_project=0 }
    in_project && /^[[:space:]]*define owner:/ { print; exit }
  ' <<<"$model")"
  [[ -n "$owner_definition" ]] || die 1 "deployed model has no project owner definition"
  if [[ "$owner_definition" == *"owner from parent"* ]]; then
    if [[ "$APPLY" == true ]]; then
      die 3 "deployed model still has owner from parent; complete the fallback-removal release first"
    fi
    echo "warning: deployed model still has owner from parent; --apply will refuse until the fallback-removal release completes" >&2
  fi
}

# read_root_tuples   -- prints one "user relation condition" line per tuple on
# the root object, where condition is "-" or the condition as key-sorted JSON,
# so a changed condition name or context changes the line.
read_root_tuples() {
  local raw
  raw="$(run_fga_pod tuple read --object "$OBJECT" --max-pages 0 \
    --consistency HIGHER_CONSISTENCY --output-format json)" || return 1
  if ! jq -e '.tuples | type == "array"' <<<"$raw" >/dev/null 2>&1; then
    echo "tuple read returned no tuples array" >&2
    return 1
  fi
  jq -r '.tuples[].key
    | "\(.user) \(.relation) \(if .condition then (.condition
        | walk(if type == "object" then to_entries | sort_by(.key) | from_entries else . end)
        | tojson) else "-" end)"' <<<"$raw"
}

# has KEY   -- KEY is "user relation"; true whatever the condition. Reads TUPLES.
has() {
  awk -v key="$1" '$1 " " $2 == key { found = 1 } END { exit !found }' <<<"$TUPLES"
}

# has_unconditioned KEY   -- KEY is "user relation"; true only without a condition.
has_unconditioned() {
  grep -qxF -e "$1 -" <<<"$TUPLES"
}

user_tuple_count() {
  grep -c '^user:' <<<"$TUPLES" || true
}

user_tuple_snapshot() {
  grep '^user:' <<<"$TUPLES" | LC_ALL=C sort || true
}

is_allowlisted() {
  local key="$1" t
  for t in "${LEGACY_TUPLES[@]}" "$MARKETING_ALT_TUPLE" "${REPLACEMENT_TUPLES[@]}"; do
    [[ "$key" == "$t" ]] && return 0
  done
  return 1
}

preflight() {
  local problems=0 t user relation flag
  if grep -q '^[^ ]* parent ' <<<"$TUPLES"; then
    echo "refused: ${OBJECT} has a parent tuple, so it is not the root project" >&2
    return 3
  fi
  if has "team:marketing-ops#member marketing_ops" && has "$MARKETING_ALT_TUPLE"; then
    echo "refused: both legacy marketing_ops tuple forms are present; expected exactly one" >&2
    problems=1
  fi
  for t in "${REPLACEMENT_TUPLES[@]}"; do
    if ! has "$t"; then
      echo "refused: replacement grant missing: ${t}" >&2
      problems=1
    fi
  done
  while read -r user relation flag; do
    [[ -z "$user" || "$user" != team:* ]] && continue
    if ! is_allowlisted "${user} ${relation}"; then
      echo "refused: unrecognised team tuple on root: ${user} ${relation}" >&2
      problems=1
    elif [[ "$flag" != "-" ]]; then
      echo "refused: tuple carries a condition: ${user} ${relation}" >&2
      problems=1
    fi
  done <<<"$TUPLES"
  [[ $problems -eq 0 ]] || return 3
}

collect_deletes() {
  local t
  TO_DELETE=()
  for t in "${LEGACY_TUPLES[@]}" "$MARKETING_ALT_TUPLE"; do
    if has "$t"; then
      TO_DELETE+=("$t")
    fi
  done
}

print_plan() {
  local t
  for t in "${LEGACY_TUPLES[@]}"; do
    has "$t" || echo "  already absent ${t}"
  done
  # Bash 3.2 treats an empty array expansion as unbound under set -u.
  for t in ${TO_DELETE[@]+"${TO_DELETE[@]}"}; do
    echo "  delete ${t}"
  done
}

delete_plan_snapshot() {
  printf '%s\n' ${TO_DELETE[@]+"${TO_DELETE[@]}"} | LC_ALL=C sort
}

print_summary() {
  echo "Environment: ${ENV_NAME} (context ${CTX}, store $(masked_store_id))"
  echo "Object: ${OBJECT}"
  echo "user tuples on root: $(user_tuple_count)"
  grep '^user:' <<<"$TUPLES" | awk '{print $2}' | sort | uniq -c | awk '{print "  " $2 ": " $1}' || true
}

print_rollback() {
  local t
  echo "Rollback (re-create the deleted tuples):"
  for t in "${TO_DELETE[@]}"; do
    echo "  fga tuple write ${t} ${OBJECT}"
  done
}

confirm_apply() {
  if [[ "$ENV_NAME" == "prod" && ! -t 0 ]]; then
    die 2 "--env prod --apply requires an interactive terminal"
  fi
  local answer=""
  printf 'Type the root project UID to delete %d tuple(s) in %s: ' "${#TO_DELETE[@]}" "$ENV_NAME" >&2
  read -r answer || true
  [[ "$answer" == "$ROOT_UID" ]] || die 2 "confirmation did not match; nothing deleted"
}

apply_deletes() {
  local t user relation
  for t in "${TO_DELETE[@]}"; do
    read -r user relation <<<"$t"
    [[ "$user" == team:* ]] || die 4 "internal error: refusing to delete non-team subject ${user}"
    echo "Deleting ${user} ${relation} ${OBJECT}"
    if ! run_fga_pod tuple delete "$user" "$relation" "$OBJECT" --on-missing ignore >/dev/null; then
      echo "delete failed for ${user} ${relation}; stopping" >&2
      return 1
    fi
  done
}

verify_after_apply() {
  local users_before="$1" failed=0 t
  TUPLES="$(read_root_tuples)" || die 4 "post-apply read failed; state unknown, re-run the dry run"
  for t in "${LEGACY_TUPLES[@]}" "$MARKETING_ALT_TUPLE"; do
    if has "$t"; then
      echo "  still present: ${t}" >&2
      failed=1
    fi
  done
  for t in "${REPLACEMENT_TUPLES[@]}"; do
    if ! has_unconditioned "$t"; then
      echo "  replacement grant missing or conditioned after apply: ${t}" >&2
      failed=1
    fi
  done
  if [[ "$(user_tuple_snapshot)" != "$users_before" ]]; then
    echo "  user tuple set changed during cleanup" >&2
    failed=1
  fi
  return "$failed"
}

main() {
  command -v jq >/dev/null || die 2 "jq is required"
  parse_args "$@"
  validate_root_uid
  select_environment
  init_pod_state_file
  resolve_heimdall_openfga_config ||
    die 1 "could not resolve Heimdall's OpenFGA store and model IDs"
  verify_root_identity
  verify_deployed_model

  TUPLES="$(read_root_tuples)" || die 1 "could not read tuples on ${OBJECT}"
  print_summary
  preflight || exit 3

  echo "Plan:"
  collect_deletes
  print_plan
  if [[ ${#TO_DELETE[@]} -eq 0 ]]; then
    echo "Nothing to delete."
    exit 0
  fi
  print_rollback

  if [[ "$APPLY" != true ]]; then
    echo "Dry run: no changes made. Re-run with --apply to delete."
    exit 0
  fi

  local planned_before users_before apply_rc=0
  planned_before="$(delete_plan_snapshot)"
  confirm_apply

  # The confirmation prompt is intentionally unbounded. Re-read every
  # precondition afterwards so the deletes never act on a stale plan.
  resolve_heimdall_openfga_config ||
    die 1 "could not refresh Heimdall's OpenFGA store and model IDs"
  verify_root_identity
  verify_deployed_model
  TUPLES="$(read_root_tuples)" || die 1 "could not refresh tuples on ${OBJECT}"
  preflight || exit 3
  collect_deletes
  [[ "$(delete_plan_snapshot)" == "$planned_before" ]] ||
    die 3 "delete plan changed during confirmation; nothing deleted, re-run the dry run"
  users_before="$(user_tuple_snapshot)"
  APPLY_STARTED=true
  apply_deletes || apply_rc=1
  echo "Verifying ${OBJECT}..."
  if ! verify_after_apply "$users_before" || [[ $apply_rc -ne 0 ]]; then
    die 4 "apply incomplete; see the lines above and the rollback commands"
  fi
  echo "Verification passed: legacy team tuples removed, replacement grants and user tuples unchanged."
}

main "$@"
