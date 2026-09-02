#!/usr/bin/env bash
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
#
# Grant/revoke Marketing Ops (Campaign Impact + Campaigns) access via direct
# OpenFGA tuple writes, then verify the grant actually resolves.
#
# This is a cluster-access-required ops tool, NOT a replacement for
# lfx-v2-project-service PR #107's self-serve API. Use the API for normal
# per-project requests (no cluster access needed, validated, audited). Use
# this script only for:
#   - granting access across ALL projects at once (--global), which the API
#     deliberately does not expose (bigger blast radius, kept ops-only)
#   - ad-hoc verification of what's actually in a given FGA store
#
# Requires: kubectl pointed at the target cluster context, permission to
# `kubectl run`/`exec` in namespace `lfx`.
#
# Usage:
#   marketing-ops-grant.sh grant  --env dev|prod --user <username> (--project <uid>|--global)
#   marketing-ops-grant.sh revoke --env dev|prod --user <username> (--project <uid>|--global)
#   marketing-ops-grant.sh check  --env dev|prod --user <username> (--project <uid>|--global)
#
# Examples:
#   marketing-ops-grant.sh grant  --env prod --user alice.example --project 00000000-0000-0000-0000-000000000001
#   marketing-ops-grant.sh grant  --env prod --user alice.example --global
#   marketing-ops-grant.sh check  --env prod --user alice.example --project 00000000-0000-0000-0000-000000000001

set -euo pipefail

usage() {
  grep '^#' "$0" | sed -e 's/^#!.*//' -e 's/^# \{0,1\}//'
  exit 1
}

ACTION="${1:-}"
[[ "$ACTION" == "grant" || "$ACTION" == "revoke" || "$ACTION" == "check" ]] || usage
shift

ENV_NAME=""
USERNAME=""
PROJECT_UID=""
GLOBAL=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env) ENV_NAME="$2"; shift 2 ;;
    --user) USERNAME="$2"; shift 2 ;;
    --project) PROJECT_UID="$2"; shift 2 ;;
    --global) GLOBAL=true; shift ;;
    *) echo "Unknown arg: $1" >&2; usage ;;
  esac
done

[[ -n "$ENV_NAME" && -n "$USERNAME" ]] || usage
if [[ "$GLOBAL" == true && -n "$PROJECT_UID" ]]; then
  echo "Specify --project OR --global, not both." >&2; exit 1
fi
if [[ "$GLOBAL" == false && -z "$PROJECT_UID" ]]; then
  echo "Must specify --project <uid> or --global." >&2; exit 1
fi

case "$ENV_NAME" in
  dev)
    CTX="lfx-v2-dev"
    STORE_ID="01K1XF6SXV7JY5HZ25EZGCDNXE"
    ;;
  prod)
    CTX="lfx-v2-prod"
    STORE_ID="01K3S60BS505DDR3VF9RAZDVHG"
    ;;
  *)
    echo "Unknown --env '$ENV_NAME' (expected dev or prod)" >&2; exit 1 ;;
esac

if [[ "$GLOBAL" == true ]]; then
  TARGET_UID="ROOT"
else
  TARGET_UID="$PROJECT_UID"
fi
TEAM_ID="marketing-ops-${TARGET_UID}"
TEAM_OBJECT="team:${TEAM_ID}"
PROJECT_OBJECT="project:${TARGET_UID}"

NS="lfx"
FGA_API_URL="http://lfx-platform-openfga:8080"

# run_fga_pod POD_NAME ARG...   -- runs one ephemeral fga-cli pod with the given args, prints its logs, deletes it.
run_fga_pod() {
  local pod="$1"; shift
  local args_json
  args_json=$(printf '"%s",' "$@")
  args_json="[${args_json%,}]"

  kubectl --context "$CTX" run "$pod" -n "$NS" --image=openfga/cli:latest --restart=Never --overrides='{
    "spec": {"containers": [{
      "name": "'"$pod"'",
      "image": "openfga/cli:latest",
      "args": '"$args_json"',
      "env": [
        {"name": "FGA_API_URL", "value": "'"$FGA_API_URL"'"},
        {"name": "FGA_STORE_ID", "value": "'"$STORE_ID"'"}
      ],
      "resources": {"requests": {"cpu": "50m", "memory": "64Mi"}, "limits": {"cpu": "100m", "memory": "128Mi"}}
    }]}
  }' >/dev/null

  # fga-cli pods run-to-completion and never report Ready; poll phase instead
  # of waiting on a condition that will never be met.
  for _ in $(seq 1 10); do
    phase=$(kubectl --context "$CTX" get pod "$pod" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
    [[ "$phase" == "Succeeded" || "$phase" == "Failed" ]] && break
    sleep 1
  done

  local out
  out=$(kubectl --context "$CTX" logs "$pod" -n "$NS" 2>&1) || true
  kubectl --context "$CTX" delete pod "$pod" -n "$NS" --ignore-not-found >/dev/null 2>&1
  echo "$out"
}

# fga_check RELATION OBJECT   -- returns "true" or "false"
fga_check() {
  local relation="$1" object="$2"
  local pod="fga-check-$$-${RANDOM}"
  local out
  out=$(run_fga_pod "$pod" query check --consistency HIGHER_CONSISTENCY "user:${USERNAME}" "$relation" "$object")
  if echo "$out" | grep -qi '"allowed": *true'; then
    echo "true"
  else
    echo "false"
  fi
}

verify() {
  local expect="$1"  # "true" or "false" — the expected post-action state
  echo ""
  echo "Verifying access for user:${USERNAME} on ${PROJECT_OBJECT} (store ${STORE_ID}, env ${ENV_NAME})..."
  local ok=true
  for relation in marketing_ops marketing_auditor campaign_manager; do
    local result
    result=$(fga_check "$relation" "$PROJECT_OBJECT")
    if [[ "$result" == "$expect" ]]; then
      echo "  PASS  ${relation} = ${result}"
    else
      echo "  FAIL  ${relation} = ${result} (expected ${expect})"
      ok=false
    fi
  done
  local member_result
  member_result=$(fga_check member "$TEAM_OBJECT")
  if [[ "$member_result" == "$expect" ]]; then
    echo "  PASS  member of ${TEAM_OBJECT} = ${member_result}"
  else
    echo "  FAIL  member of ${TEAM_OBJECT} = ${member_result} (expected ${expect})"
    ok=false
  fi

  if [[ "$ok" == true ]]; then
    echo "Verification PASSED."
  else
    echo "Verification FAILED — see above." >&2
    exit 1
  fi
}

case "$ACTION" in
  grant)
    echo "Granting user:${USERNAME} marketing_ops via ${TEAM_OBJECT} -> ${PROJECT_OBJECT} (env=${ENV_NAME})"
    run_fga_pod "fga-write-team-$$" tuple write "${TEAM_OBJECT}#member" marketing_ops "$PROJECT_OBJECT"
    run_fga_pod "fga-write-member-$$" tuple write "user:${USERNAME}" member "$TEAM_OBJECT"
    verify "true"
    ;;
  revoke)
    echo "Revoking user:${USERNAME} from ${TEAM_OBJECT} (env=${ENV_NAME})"
    echo "Note: the team->project marketing_ops reference is left in place by design — access is controlled purely by team membership."
    run_fga_pod "fga-delete-member-$$" tuple delete "user:${USERNAME}" member "$TEAM_OBJECT"
    verify "false"
    ;;
  check)
    verify "true"
    ;;
esac
