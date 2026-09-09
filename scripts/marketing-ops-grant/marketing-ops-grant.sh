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
# `kubectl run`/`get`/`logs`/`delete` pods in namespace `lfx`.
#
# --global writes/reads against the root project, not a synthetic "ROOT"
# object — the root project's real OpenFGA object ID is a generated UUID
# (see scripts/root-project-setup/main.go), not the literal string "ROOT"
# (that's only its slug). You must resolve it once per environment and pass
# it explicitly with --root-uid, e.g. via a nats-box pod:
#   nats request lfx.projects-api.slug_to_uid "ROOT" --server=nats://lfx-platform-nats:4222
#
# Usage:
#   marketing-ops-grant.sh grant  --env dev|prod --user <username> (--project <uid>|--global --root-uid <uid> [--verify-project <uid>])
#   marketing-ops-grant.sh revoke --env dev|prod --user <username> (--project <uid>|--global --root-uid <uid>)
#   marketing-ops-grant.sh check  --env dev|prod --user <username> (--project <uid>|--global --root-uid <uid>)
#
# --verify-project <uid> (grant + --global only) additionally checks the given
# real project after the root write, confirming the cascade actually reached
# it rather than just resolving on the root object.
#
# For --env prod, the store ID is not committed here — export FGA_STORE_ID
# first (see README's "Configuring environments" section).
#
# Examples:
#   marketing-ops-grant.sh grant  --env prod --user alice.example --project 00000000-0000-0000-0000-000000000001
#   marketing-ops-grant.sh grant  --env prod --user alice.example --global --root-uid 00000000-0000-0000-0000-000000000002 \
#     --verify-project 00000000-0000-0000-0000-000000000001
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
ROOT_UID=""
VERIFY_PROJECT_UID=""
GLOBAL=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env|--user|--project|--root-uid|--verify-project)
      [[ $# -ge 2 ]] || { echo "$1 requires a value" >&2; usage; }
      ;;
  esac
  case "$1" in
    --env) ENV_NAME="$2"; shift 2 ;;
    --user) USERNAME="$2"; shift 2 ;;
    --project) PROJECT_UID="$2"; shift 2 ;;
    --global) GLOBAL=true; shift ;;
    --root-uid) ROOT_UID="$2"; shift 2 ;;
    --verify-project) VERIFY_PROJECT_UID="$2"; shift 2 ;;
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
if [[ "$GLOBAL" == true && -z "$ROOT_UID" ]]; then
  echo "--global requires --root-uid <uid> — the root project's real OpenFGA object ID." >&2
  echo "\"ROOT\" is only the root project's slug, not its object ID; resolve it once per" >&2
  echo "environment, e.g.: nats request lfx.projects-api.slug_to_uid \"ROOT\" --server=nats://lfx-platform-nats:4222" >&2
  exit 1
fi
if [[ -n "$VERIFY_PROJECT_UID" && "$GLOBAL" == false ]]; then
  echo "--verify-project only applies with --global (it samples a descendant project to confirm the cascade)." >&2
  exit 1
fi
# The script has no way to resolve a project UID to "is this the root project"
# on its own — that needs a live API/NATS lookup (see --root-uid above). This
# only catches the case where the operator already knows the root UID and
# passes it via --project instead of --global; an operator who doesn't know
# it gets no automatic protection here.
if [[ "$GLOBAL" == false && -n "$ROOT_UID" && "$PROJECT_UID" == "$ROOT_UID" ]]; then
  echo "--project matches the known root project UID. A grant/revoke on the root project has the" >&2
  echo "same org-wide blast radius as --global — use --global --root-uid ${ROOT_UID} instead so the" >&2
  echo "production confirmation gate below applies." >&2
  exit 1
fi

case "$ENV_NAME" in
  dev)
    CTX="lfx-v2-dev"
    STORE_ID="${FGA_STORE_ID:-01K1XF6SXV7JY5HZ25EZGCDNXE}"
    ;;
  prod)
    CTX="lfx-v2-prod"
    # Prod store ID is production config and must not be committed (CLAUDE.md
    # "No PII in Source" / no production data in committed files) — export it.
    STORE_ID="${FGA_STORE_ID:?Set FGA_STORE_ID to the prod OpenFGA store ID before running --env prod (see README)}"
    ;;
  *)
    echo "Unknown --env '$ENV_NAME' (expected dev or prod)" >&2; exit 1 ;;
esac

if [[ "$GLOBAL" == true ]]; then
  TARGET_UID="$ROOT_UID"
else
  TARGET_UID="$PROJECT_UID"
fi
TEAM_ID="marketing-ops-${TARGET_UID}"
TEAM_OBJECT="team:${TEAM_ID}"
PROJECT_OBJECT="project:${TARGET_UID}"

NS="lfx"
FGA_API_URL="http://lfx-platform-openfga:8080"

json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  printf '%s' "$s"
}

  # run_fga_pod POD_NAME ARG...   -- runs one ephemeral fga-cli pod with the given
  # args, prints its logs, deletes it. Returns non-zero (in addition to printing
  # whatever was captured) if the pod failed, its logs couldn't be fetched, or it
  # never reached a terminal phase — callers must not treat that the same as a
  # successful command whose output happens not to match what they're looking for.
run_fga_pod() {
  local pod="$1"; shift
  local args_json="[" arg first=true
  for arg in "$@"; do
    if [[ "$first" == true ]]; then
      args_json+="\"$(json_escape "$arg")\""
      first=false
    else
      args_json+=",\"$(json_escape "$arg")\""
    fi
  done
  args_json+="]"

  # Checked explicitly rather than relying on `set -e`: this call sits inside
  # callers' `if ! out=$(run_fga_pod ...); then`, and errexit is suspended for
  # everything evaluated as part of an `if` condition — a failed `kubectl run`
  # (bad RBAC, API error) would otherwise fall through into the 120s poll below
  # instead of failing immediately.
  if ! kubectl --context "$CTX" --request-timeout=10s run "$pod" -n "$NS" --image=openfga/cli:v0.7.20 --restart=Never --overrides='{
    "spec": {"containers": [{
      "name": "'"$pod"'",
      "image": "openfga/cli:v0.7.20",
      "args": '"$args_json"',
      "env": [
        {"name": "FGA_API_URL", "value": "'"$FGA_API_URL"'"},
        {"name": "FGA_STORE_ID", "value": "'"$STORE_ID"'"}
      ],
      "resources": {"requests": {"cpu": "50m", "memory": "64Mi"}, "limits": {"cpu": "100m", "memory": "128Mi"}}
    }]}
  }' >/dev/null; then
    echo "run_fga_pod: kubectl run failed to create pod ${pod}" >&2
    return 1
  fi

  # fga-cli pods run-to-completion and never report Ready; poll the terminal
  # phase instead of a condition that will never be met. 120s tolerates a
  # cold-node image pull that the old 10s poll ceiling did not, while polling
  # every 2s (rather than a single 120s `kubectl wait`) lets a pod that fails
  # fast (e.g. ImagePullBackOff) get caught well before the deadline instead
  # of blocking the full timeout on every check verify_grant()/verify_revoke() make.
  # kubectl's --request-timeout defaults to 0 (no timeout), so an individual
  # `get` call could itself hang past the 120s poll deadline below; bound
  # every call explicitly.
  local phase="" deadline
  deadline=$(( $(date +%s) + 120 ))
  while :; do
    phase=$(kubectl --context "$CTX" --request-timeout=10s get pod "$pod" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
    [[ "$phase" == "Succeeded" || "$phase" == "Failed" ]] && break
    (( $(date +%s) >= deadline )) && break
    sleep 2
  done

  local out rc=0
  out=$(kubectl --context "$CTX" --request-timeout=10s logs "$pod" -n "$NS" 2>&1) || rc=1
  kubectl --context "$CTX" --request-timeout=10s delete pod "$pod" -n "$NS" --ignore-not-found >/dev/null 2>&1
  echo "$out"

  if [[ "$phase" != "Succeeded" ]]; then
    echo "run_fga_pod: pod ${pod} did not succeed (phase=${phase:-timed out})" >&2
    return 1
  fi
  return "$rc"
}

  # fga_check RELATION OBJECT   -- prints "true" or "false" and returns 0, or
  # prints nothing and returns non-zero if the underlying pod command failed —
  # a broken check must never be reported as a denied ("false") check.
fga_check() {
  local relation="$1" object="$2"
  local pod="fga-check-$$-${RANDOM}"
  local out
  if ! out=$(run_fga_pod "$pod" query check --consistency HIGHER_CONSISTENCY "user:${USERNAME}" "$relation" "$object"); then
    echo "$out" >&2
    return 1
  fi
  if echo "$out" | grep -qi '"allowed": *true'; then
    echo "true"
  elif echo "$out" | grep -qi '"allowed": *false'; then
    echo "false"
  else
    echo "fga_check: could not determine result for ${relation} on ${object}:" >&2
    echo "$out" >&2
    return 1
  fi
}

# verify_grant   -- asserts the full post-grant state: all three derived
# relations true, and team membership true. Only used after `grant`, where all
# four are genuinely expected true — unlike revoke (see verify_revoke below),
# a grant has no other path that would make this assertion a false failure.
verify_grant() {
  echo ""
  echo "Verifying access for user:${USERNAME} on ${PROJECT_OBJECT} (store ${STORE_ID}, env ${ENV_NAME})..."
  local ok=true
  for relation in marketing_ops marketing_auditor campaign_manager; do
    local result
    if ! result=$(fga_check "$relation" "$PROJECT_OBJECT"); then
      echo "  ERROR checking ${relation} — fga-cli command failed, not a real allow/deny result" >&2
      ok=false
      continue
    fi
    if [[ "$result" == "true" ]]; then
      echo "  PASS  ${relation} = ${result}"
    else
      echo "  FAIL  ${relation} = ${result} (expected true)"
      ok=false
    fi
  done
  local member_result
  if ! member_result=$(fga_check member "$TEAM_OBJECT"); then
    echo "  ERROR checking member of ${TEAM_OBJECT} — fga-cli command failed, not a real allow/deny result" >&2
    ok=false
  elif [[ "$member_result" == "true" ]]; then
    echo "  PASS  member of ${TEAM_OBJECT} = ${member_result}"
  else
    echo "  FAIL  member of ${TEAM_OBJECT} = ${member_result} (expected true)"
    ok=false
  fi

  if [[ -n "$VERIFY_PROJECT_UID" ]]; then
    echo ""
    echo "Verifying cascade to descendant project:${VERIFY_PROJECT_UID}..."
    for relation in marketing_ops marketing_auditor campaign_manager; do
      local result
      if ! result=$(fga_check "$relation" "project:${VERIFY_PROJECT_UID}"); then
        echo "  ERROR checking ${relation} on project:${VERIFY_PROJECT_UID}" >&2
        ok=false
        continue
      fi
      if [[ "$result" == "true" ]]; then
        echo "  PASS  ${relation} = true (inherited from root)"
      else
        echo "  FAIL  ${relation} = false — cascade did not reach project:${VERIFY_PROJECT_UID}" >&2
        ok=false
      fi
    done
  fi

  if [[ "$ok" == true ]]; then
    echo "Verification PASSED."
  else
    echo "Verification FAILED — see above." >&2
    exit 1
  fi
}

# verify_revoke   -- a revoke only ever removes one tuple: the user's
# membership in TEAM_OBJECT, so that's the only thing safe to assert false.
# marketing_auditor and campaign_manager have independent grant paths in the
# model (executive_director, or an ancestor --global grant via `from parent`)
# that a project-level revoke can never touch, so asserting them false would
# fail a revoke that worked correctly for anyone holding access another way.
# Report those three instead of asserting them.
verify_revoke() {
  echo ""
  echo "Verifying access for user:${USERNAME} on ${PROJECT_OBJECT} (store ${STORE_ID}, env ${ENV_NAME})..."
  local member_result
  if ! member_result=$(fga_check member "$TEAM_OBJECT"); then
    echo "  ERROR checking member of ${TEAM_OBJECT} — fga-cli command failed, not a real allow/deny result" >&2
    exit 1
  fi
  if [[ "$member_result" == "false" ]]; then
    echo "  PASS  member of ${TEAM_OBJECT} = ${member_result}"
  else
    echo "  FAIL  member of ${TEAM_OBJECT} = ${member_result} (expected false)" >&2
    exit 1
  fi

  echo "  The relations below are reported, not asserted: a non-false value after a successful"
  echo "  revoke means access via another path (executive_director, or an ancestor --global grant),"
  echo "  not a failed revoke."
  for relation in marketing_ops marketing_auditor campaign_manager; do
    local result
    if ! result=$(fga_check "$relation" "$PROJECT_OBJECT"); then
      echo "  ERROR checking ${relation} — fga-cli command failed, not a real allow/deny result" >&2
      exit 1
    fi
    echo "  ${relation} = ${result}"
  done
}

if [[ ( "$ACTION" == "grant" || "$ACTION" == "revoke" ) && "$ENV_NAME" == "prod" && "$GLOBAL" == true ]]; then
  if [[ "$ACTION" == "grant" ]]; then
    echo "You are about to grant user:${USERNAME} marketing_ops access to EVERY project in prod"
    echo "(via ${TEAM_OBJECT} -> ${PROJECT_OBJECT}). This username is NOT validated against the"
    echo "auth service — a typo grants org-wide access to the wrong person."
  else
    echo "You are about to revoke user:${USERNAME} from EVERY project in prod"
    echo "(via ${TEAM_OBJECT}). This username is NOT validated against the auth service —"
    echo "a typo revokes org-wide access from the wrong person."
  fi
  read -r -p "Type the username again to confirm (${USERNAME}): " CONFIRM_USERNAME
  if [[ "$CONFIRM_USERNAME" != "$USERNAME" ]]; then
    echo "Confirmation did not match. Aborting." >&2
    exit 1
  fi
fi

case "$ACTION" in
  grant)
    echo "Granting user:${USERNAME} marketing_ops via ${TEAM_OBJECT} -> ${PROJECT_OBJECT} (env=${ENV_NAME})"
    # --on-duplicate requires OpenFGA server v1.10.0+ (lfx-v2-helm's openfga
    # chart floor spans this — confirm the running server version in the
    # target env before using this against a store that predates it).
    run_fga_pod "fga-write-team-$$" tuple write "${TEAM_OBJECT}#member" marketing_ops "$PROJECT_OBJECT" --on-duplicate ignore
    run_fga_pod "fga-write-member-$$" tuple write "user:${USERNAME}" member "$TEAM_OBJECT" --on-duplicate ignore
    verify_grant
    ;;
  revoke)
    echo "Revoking user:${USERNAME} from ${TEAM_OBJECT} (env=${ENV_NAME})"
    echo "Note: the team->project marketing_ops reference is left in place by design — access is controlled purely by team membership."
    run_fga_pod "fga-delete-member-$$" tuple delete "user:${USERNAME}" member "$TEAM_OBJECT" --on-missing ignore
    verify_revoke
    ;;
  check)
    echo "Current access for user:${USERNAME} on ${PROJECT_OBJECT} (store ${STORE_ID}, env ${ENV_NAME}):"
    for relation in marketing_ops marketing_auditor campaign_manager; do
      if ! result=$(fga_check "$relation" "$PROJECT_OBJECT"); then
        echo "  ERROR checking ${relation} — fga-cli command failed, not a real allow/deny result" >&2
        exit 1
      fi
      echo "  ${relation} = ${result}"
    done
    if ! member_result=$(fga_check member "$TEAM_OBJECT"); then
      echo "  ERROR checking member of ${TEAM_OBJECT} — fga-cli command failed, not a real allow/deny result" >&2
      exit 1
    fi
    echo "  member of ${TEAM_OBJECT} = ${member_result}"
    ;;
esac
