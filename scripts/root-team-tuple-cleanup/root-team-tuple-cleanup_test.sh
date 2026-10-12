#!/usr/bin/env bash
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
#
# Tests for root-team-tuple-cleanup.sh. A fake kubectl on PATH emulates the
# ephemeral OpenFGA CLI pods against a JSON tuple file, so no cluster is used.
#
# Run: bash scripts/root-team-tuple-cleanup/root-team-tuple-cleanup_test.sh

# check() evaluates its condition after the run, so the single quotes are intended.
# shellcheck disable=SC2016

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="${HERE}/root-team-tuple-cleanup.sh"

ROOT_UID="00000000-0000-4000-8000-000000000001"
FAKE_STORE_ID="01ARZ3NDEKTSV4RRFFQ69G5FAV"
FAILURES=0
PASSES=0

setup() {
  WORK="$(mktemp -d)"
  mkdir -p "${WORK}/bin" "${WORK}/pods"
  : >"${WORK}/calls"
  cat >"${WORK}/bin/kubectl" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
dir="${FAKE_KUBECTL_DIR:?}"
args=("$@")
ctx=""
i=0
while [[ $i -lt ${#args[@]} ]]; do
  case "${args[$i]}" in
    --context) ctx="${args[$((i + 1))]}"; i=$((i + 2)); continue ;;
  esac
  i=$((i + 1))
done
verb=""
pod=""
overrides=""
i=0
while [[ $i -lt ${#args[@]} ]]; do
  a="${args[$i]}"
  case "$a" in
    --context|-n|--namespace|--image|--restart|-o|-l|--request-timeout) i=$((i + 2)); continue ;;
    --context=*|--request-timeout=*|--restart=*|--image=*|--ignore-not-found) i=$((i + 1)); continue ;;
    --overrides) overrides="${args[$((i + 1))]}"; i=$((i + 2)); continue ;;
    --overrides=*) overrides="${a#--overrides=}"; i=$((i + 1)); continue ;;
  esac
  if [[ -z "$verb" ]]; then
    verb="$a"
  elif [[ "$verb" == "get" || "$verb" == "delete" ]] && [[ "$a" == "pod" ]]; then
    :
  elif [[ -z "$pod" ]]; then
    pod="$a"
  fi
  i=$((i + 1))
done
case "$verb" in
  run)
    image="$(jq -r '.spec.containers[0].image' <<<"$overrides")"
    mapfile_args="$(jq -r '.spec.containers[0].args | join(" ")' <<<"$overrides")"
    echo "ctx=${ctx} image=${image} args=${mapfile_args}" >>"${dir}/calls"
    command="$(jq -r '.spec.containers[0].args[0]' <<<"$overrides")"
    sub="$(jq -r '.spec.containers[0].args[1]' <<<"$overrides")"
    state="${dir}/state.json"
    phase="Succeeded"
    out="{}"
    if [[ "$command" == "nats" && "$sub" == "kv" ]]; then
      out="${FAKE_CANONICAL_ROOT_UID:?}"
    elif [[ "$command" == "model" && "$sub" == "get" ]]; then
      if [[ -n "${FAKE_MODEL_HAS_OWNER_PARENT:-}" ]]; then
        out=$'model\n  schema 1.1\ntype project\n  relations\n    define owner: global_owner or owner from parent\ntype team\n  relations\n    define member: [user]'
      else
        out=$'model\n  schema 1.1\ntype project\n  relations\n    define owner: global_owner\ntype team\n  relations\n    define member: [user]'
      fi
    elif [[ "$sub" == "read" && -n "${FAKE_READ_GARBAGE:-}" ]]; then
      out="Error: simulated store outage"
    elif [[ "$sub" == "read" ]]; then
      count_file="${dir}/tuple-read-count"
      read_count=0
      [[ -e "$count_file" ]] && read_count="$(cat "$count_file")"
      read_count=$((read_count + 1))
      printf '%s' "$read_count" >"$count_file"
      if [[ -n "${FAKE_DROP_REPLACEMENT_ON_SECOND_READ:-}" && $read_count -eq 2 ]]; then
        jq '[.[] | select((.user == "team:formation#member" and .relation == "global_owner") | not)]' \
          "$state" >"${state}.tmp" && mv "${state}.tmp" "$state"
      fi
      object="$(jq -r '.spec.containers[0].args as $a | ($a | index("--object")) as $i | $a[$i + 1]' <<<"$overrides")"
      out="$(jq --arg o "$object" '{continuation_token: "", tuples: [.[] | select(.object == $o) | {key: .}]}' "$state")"
    elif [[ "$sub" == "delete" ]]; then
      user="$(jq -r '.spec.containers[0].args[2]' <<<"$overrides")"
      rel="$(jq -r '.spec.containers[0].args[3]' <<<"$overrides")"
      obj="$(jq -r '.spec.containers[0].args[4]' <<<"$overrides")"
      if [[ -n "${FAKE_FAIL_DELETE_USER:-}" && "$user" == "$FAKE_FAIL_DELETE_USER" ]]; then
        phase="Failed"
        out="simulated failure"
      else
        jq --arg u "$user" --arg r "$rel" --arg o "$obj" \
          '[.[] | select((.user == $u and .relation == $r and .object == $o) | not)]' \
          "$state" >"${state}.tmp" && mv "${state}.tmp" "$state"
        if [[ -n "${FAKE_SWAP_USER_ON_DELETE:-}" && ! -e "${dir}/user-swapped" ]]; then
          jq '[.[] | if .user == "user:writer-one" then .user = "user:writer-two" else . end]' \
            "$state" >"${state}.tmp" && mv "${state}.tmp" "$state"
          : >"${dir}/user-swapped"
        fi
      fi
    fi
    printf '%s' "$out" >"${dir}/pods/${pod}.out"
    printf '%s' "$phase" >"${dir}/pods/${pod}.phase"
    ;;
  get)
    if [[ "$pod" == "deployments" ]]; then
      jq -cn --arg id "${FAKE_GATEWAY_MODEL_ID:?}" --arg store "${FAKE_GATEWAY_STORE_ID:?}" '{
        items: [{spec: {template: {spec: {containers: [{
          env: [
            {name: "OPENFGA_AUTH_MODEL_ID", value: $id},
            {name: "OPENFGA_STORE_ID", value: $store}
          ]
        }]}}}}]
      }'
    else
      cat "${dir}/pods/${pod}.phase"
    fi
    ;;
  logs) cat "${dir}/pods/${pod}.out" ;;
  delete) rm -f "${dir}/pods/${pod}.out" "${dir}/pods/${pod}.phase" ;;
  *) echo "fake kubectl: unexpected verb ${verb}" >&2; exit 1 ;;
esac
FAKE
  chmod +x "${WORK}/bin/kubectl"
}

teardown() {
  rm -rf "$WORK"
}

# write_state JSON   -- the tuple store the fake kubectl reads and mutates.
write_state() {
  printf '%s' "$1" >"${WORK}/state.json"
}

tuple() {
  printf '{"user":"%s","relation":"%s","object":"project:%s"}' "$1" "$2" "${3:-$ROOT_UID}"
}

# full_state   -- the documented root object: five legacy team tuples, the
# replacement global grants, and user tuples standing in for the roster.
full_state() {
  local tuples=(
    "$(tuple 'team:lf-staff#member' auditor)"
    "$(tuple 'team:lf-contractor#member' auditor)"
    "$(tuple 'team:formation#member' owner)"
    "$(tuple 'team:product-support#member' owner)"
    "$(tuple 'team:marketing-ops#member' marketing_ops)"
    "$(tuple 'team:formation#member' global_owner)"
    "$(tuple 'team:product-support#member' global_owner)"
    "$(tuple 'team:global-project-writers#member' global_writer)"
    "$(tuple 'team:lf-staff#member' global_auditor)"
    "$(tuple 'team:global-project-auditors#member' global_auditor)"
    "$(tuple 'team:marketing-ops#member' global_marketing_ops)"
    "$(tuple 'user:writer-one' writer)"
    "$(tuple 'user:auditor-one' auditor)"
    "$(tuple 'team:lf-staff#member' auditor '00000000-0000-4000-8000-000000000099')"
  )
  local IFS=,
  printf '[%s]' "${tuples[*]}"
}

# run_script ARG...   -- runs the script with the fake kubectl; sets STATUS and OUT.
run_script() {
  set +e
  OUT="$(PATH="${WORK}/bin:${PATH}" FAKE_KUBECTL_DIR="$WORK" \
    FAKE_CANONICAL_ROOT_UID="${FAKE_CANONICAL_ROOT_UID:-$ROOT_UID}" \
    FAKE_GATEWAY_MODEL_ID="${FAKE_GATEWAY_MODEL_ID:-01ARZ3NDEKTSV4RRFFQ69G5FAV}" \
    FAKE_GATEWAY_STORE_ID="${FAKE_GATEWAY_STORE_ID:-$FAKE_STORE_ID}" \
    bash "$SCRIPT" "$@" 2>&1 </dev/null)"
  STATUS=$?
  set -e
}

# run_script_stdin INPUT ARG...   -- as run_script, with INPUT on stdin.
run_script_stdin() {
  local input="$1"; shift
  set +e
  OUT="$(printf '%s\n' "$input" | PATH="${WORK}/bin:${PATH}" FAKE_KUBECTL_DIR="$WORK" \
    FAKE_CANONICAL_ROOT_UID="${FAKE_CANONICAL_ROOT_UID:-$ROOT_UID}" \
    FAKE_GATEWAY_MODEL_ID="${FAKE_GATEWAY_MODEL_ID:-01ARZ3NDEKTSV4RRFFQ69G5FAV}" \
    FAKE_GATEWAY_STORE_ID="${FAKE_GATEWAY_STORE_ID:-$FAKE_STORE_ID}" \
    bash "$SCRIPT" "$@" 2>&1)"
  STATUS=$?
  set -e
}

delete_calls() {
  grep -c 'args=tuple delete' "${WORK}/calls" || true
}

has_tuple() {
  jq -e --arg u "$1" --arg r "$2" --arg o "project:${3:-$ROOT_UID}" \
    'any(.[]; .user == $u and .relation == $r and .object == $o)' "${WORK}/state.json" >/dev/null
}

check() {
  local name="$1" cond="$2"
  if eval "$cond"; then
    PASSES=$((PASSES + 1))
  else
    FAILURES=$((FAILURES + 1))
    echo "FAIL: ${name}"
    echo "  condition: ${cond}"
    echo "  status: ${STATUS:-unset}"
    echo "  output:"
    printf '%s\n' "${OUT:-}" | sed 's/^/    /'
  fi
}

test_rejects_literal_root_slug() {
  setup; write_state "$(full_state)"
  run_script --env dev --root-uid ROOT
  check "literal ROOT rejected" '[[ $STATUS -eq 2 && "$OUT" == *"slug"* ]]'
  check "no pod started" '[[ ! -s "${WORK}/calls" ]]'
  teardown
}

test_rejects_malformed_uids() {
  local uid
  for uid in "00000000-0000-4000-8000-00000000000A" "not-a-uuid" "${ROOT_UID}x" ""; do
    setup; write_state "$(full_state)"
    run_script --env dev --root-uid "$uid"
    check "malformed uid '${uid}' rejected" '[[ $STATUS -eq 2 ]]'
    check "no pod for '${uid}'" '[[ ! -s "${WORK}/calls" ]]'
    teardown
  done
}

test_rejects_unknown_env() {
  setup; write_state "$(full_state)"
  run_script --env staging --root-uid "$ROOT_UID"
  check "unknown env rejected" '[[ $STATUS -eq 2 ]]'
  run_script --root-uid "$ROOT_UID"
  check "missing env rejected" '[[ $STATUS -eq 2 ]]'
  teardown
}

test_rejects_store_id_override() {
  setup; write_state "$(full_state)"
  FGA_STORE_ID="$FAKE_STORE_ID" run_script --env dev --root-uid "$ROOT_UID"
  check "store ID override rejected" '[[ $STATUS -eq 2 && "$OUT" == *"resolved from Heimdall"* ]]'
  check "no pod for store ID override" '[[ ! -s "${WORK}/calls" ]]'
  teardown
}

test_refuses_invalid_gateway_store_id() {
  setup; write_state "$(full_state)"
  FAKE_GATEWAY_STORE_ID="not-a-store-id" run_script --env dev --root-uid "$ROOT_UID"
  check "invalid gateway store ID refused" '[[ $STATUS -eq 1 && "$OUT" == *"not a ULID"* ]]'
  check "no pod for invalid gateway store ID" '[[ ! -s "${WORK}/calls" ]]'
  teardown
}

test_dry_run_is_default_and_deletes_nothing() {
  setup; write_state "$(full_state)"
  run_script --env dev --root-uid "$ROOT_UID"
  check "dry run exits 0" '[[ $STATUS -eq 0 ]]'
  check "dry run issues no delete" '[[ $(delete_calls) -eq 0 ]]'
  check "dry run plans five deletes" '[[ $(grep -c "^  delete " <<<"$OUT") -eq 5 ]]'
  check "dry run prints rollback writes" '[[ $(grep -c "fga tuple write team:" <<<"$OUT") -eq 5 ]]'
  check "dry run does not print usernames" '[[ "$OUT" != *"writer-one"* && "$OUT" != *"auditor-one"* ]]'
  check "dry run reports user tuple count" '[[ "$OUT" == *"user tuples on root: 2"* ]]'
  check "reads use the dev context" 'grep -q "^ctx=lfx-v2-dev " "${WORK}/calls"'
  check "reads use higher consistency" 'grep -q "args=tuple read .*--consistency HIGHER_CONSISTENCY" "${WORK}/calls"'
  check "fga image is pinned by digest" 'grep -q "image=openfga/cli:v0.7.20@sha256:26acde96d90420e53fe361a740dcceba4b67f1c49e4f35117cd0c19c83ac5068 " "${WORK}/calls"'
  check "nats image is pinned by digest" 'grep -q "image=natsio/nats-box@sha256:ffce8bd103383f179f8c7f11cf645726acf5d17280706c530c3b342dbe16334c " "${WORK}/calls"'
  check "root identity comes from slug index" 'grep -q "args=nats kv get projects slug/ROOT --raw --server " "${WORK}/calls"'
  check "model read uses gateway model ID" 'grep -q "args=model get --model-id 01ARZ3NDEKTSV4RRFFQ69G5FAV --format fga" "${WORK}/calls"'
  teardown
}

test_refuses_parentless_non_root_object() {
  setup; write_state "$(full_state)"
  FAKE_CANONICAL_ROOT_UID="00000000-0000-4000-8000-000000000002" \
    run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "parentless non-root object refused" '[[ $STATUS -eq 3 && "$OUT" == *"canonical slug/ROOT mapping"* ]]'
  check "no FGA read for non-root object" '! grep -q "args=tuple read" "${WORK}/calls"'
  check "no delete for non-root object" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_refuses_object_with_parent() {
  setup
  write_state "$(full_state | jq --arg o "project:${ROOT_UID}" '. + [{user: "project:11111111-1111-4111-8111-111111111111", relation: "parent", object: $o}]')"
  run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "object with a parent refused" '[[ $STATUS -eq 3 && "$OUT" == *"parent"* ]]'
  check "no delete for non-root object" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_refuses_model_with_owner_parent_fallback() {
  setup; write_state "$(full_state)"
  FAKE_MODEL_HAS_OWNER_PARENT=1 run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "owner from parent model refused" '[[ $STATUS -eq 3 && "$OUT" == *"owner from parent"* ]]'
  check "no tuple read before model fallback removed" '! grep -q "args=tuple read" "${WORK}/calls"'
  check "no delete before model fallback removed" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_dry_run_warns_but_reports_plan_before_model_release() {
  setup; write_state "$(full_state)"
  FAKE_MODEL_HAS_OWNER_PARENT=1 run_script --env dev --root-uid "$ROOT_UID"
  check "pre-release dry run succeeds with warning" '[[ $STATUS -eq 0 && "$OUT" == *"warning:"* ]]'
  check "pre-release dry run still reports five deletes" '[[ $(grep -c "^  delete " <<<"$OUT") -eq 5 ]]'
  check "pre-release dry run issues no delete" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_refuses_invalid_gateway_model_id() {
  setup; write_state "$(full_state)"
  FAKE_GATEWAY_MODEL_ID="not-a-model-id" \
    run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "invalid gateway model ID refused" '[[ $STATUS -eq 1 && "$OUT" == *"not a ULID"* ]]'
  check "no model or tuple read with invalid model ID" '! grep -q -e "args=model get" -e "args=tuple read" "${WORK}/calls"'
  check "no delete with invalid model ID" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_refuses_when_replacement_grant_missing() {
  setup
  write_state "$(full_state | jq '[.[] | select((.user == "team:global-project-writers#member" and .relation == "global_writer") | not)]')"
  run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "missing global_writer refused" '[[ $STATUS -eq 3 && "$OUT" == *"global-project-writers"* ]]'
  check "no delete without replacements" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_refuses_unrecognised_team_tuple() {
  setup
  write_state "$(full_state | jq --arg o "project:${ROOT_UID}" '. + [{user: "team:unknown#member", relation: "writer", object: $o}]')"
  run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "unrecognised team tuple refused" '[[ $STATUS -eq 3 && "$OUT" == *"team:unknown#member"* ]]'
  check "no delete with unrecognised team tuple" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_refuses_conditioned_legacy_tuple() {
  setup
  write_state "$(full_state | jq '[.[] | if .user == "team:lf-staff#member" and .relation == "auditor" then . + {condition: {name: "c"}} else . end]')"
  run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "conditioned legacy tuple refused" '[[ $STATUS -eq 3 && "$OUT" == *"condition"* ]]'
  check "no delete with condition" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_apply_deletes_exactly_the_legacy_tuples() {
  setup; write_state "$(full_state)"
  run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "apply exits 0" '[[ $STATUS -eq 0 ]]'
  check "apply issues five deletes" '[[ $(delete_calls) -eq 5 ]]'
  check "no delete targets a user subject" '! grep -q "args=tuple delete user:" "${WORK}/calls"'
  check "every delete ignores a missing tuple" '[[ $(grep -c "args=tuple delete .* --on-missing ignore" "${WORK}/calls") -eq 5 ]]'
  check "lf-staff auditor removed" '! has_tuple "team:lf-staff#member" auditor'
  check "lf-contractor auditor removed" '! has_tuple "team:lf-contractor#member" auditor'
  check "formation owner removed" '! has_tuple "team:formation#member" owner'
  check "product-support owner removed" '! has_tuple "team:product-support#member" owner'
  check "marketing_ops removed" '! has_tuple "team:marketing-ops#member" marketing_ops'
  check "replacement grants kept" 'has_tuple "team:lf-staff#member" global_auditor && has_tuple "team:formation#member" global_owner'
  check "user tuples kept" 'has_tuple "user:writer-one" writer && has_tuple "user:auditor-one" auditor'
  check "other project untouched" 'has_tuple "team:lf-staff#member" auditor 00000000-0000-4000-8000-000000000099'
  check "verification passed" '[[ "$OUT" == *"Verification passed"* ]]'
  teardown
}

test_apply_handles_per_root_marketing_team() {
  setup
  write_state "$(full_state | jq --arg u "team:marketing-ops-${ROOT_UID}#member" '[.[] | if .user == "team:marketing-ops#member" and .relation == "marketing_ops" then .user = $u else . end]')"
  run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "per-root marketing team accepted" '[[ $STATUS -eq 0 ]]'
  check "per-root marketing team removed" '! has_tuple "team:marketing-ops-${ROOT_UID}#member" marketing_ops'
  teardown
}

test_refuses_both_legacy_marketing_team_forms() {
  setup
  write_state "$(full_state | jq --arg u "team:marketing-ops-${ROOT_UID}#member" --arg o "project:${ROOT_UID}" '. + [{user: $u, relation: "marketing_ops", object: $o}]')"
  run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "both marketing team forms refused" '[[ $STATUS -eq 3 && "$OUT" == *"both legacy marketing_ops tuple forms"* ]]'
  check "no delete with both marketing team forms" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_apply_requires_matching_confirmation() {
  setup; write_state "$(full_state)"
  run_script_stdin "wrong" --env dev --root-uid "$ROOT_UID" --apply
  check "mismatched confirmation refused" '[[ $STATUS -eq 2 ]]'
  check "no delete without confirmation" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_apply_rechecks_plan_after_confirmation() {
  setup; write_state "$(full_state)"
  FAKE_DROP_REPLACEMENT_ON_SECOND_READ=1 \
    run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "stale replacement plan refused" '[[ $STATUS -eq 3 && "$OUT" == *"replacement grant missing"* ]]'
  check "no delete after plan changed" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_prod_apply_requires_terminal() {
  setup; write_state "$(full_state)"
  run_script_stdin "$ROOT_UID" --env prod --root-uid "$ROOT_UID" --apply
  check "prod apply without a terminal refused" '[[ $STATUS -eq 2 && "$OUT" == *"terminal"* ]]'
  check "no prod delete without a terminal" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_prod_dry_run_masks_store_id() {
  setup; write_state "$(full_state)"
  run_script --env prod --root-uid "$ROOT_UID"
  check "prod dry run exits 0" '[[ $STATUS -eq 0 ]]'
  check "prod store ID is masked" '[[ "$OUT" != *"$FAKE_STORE_ID"* ]]'
  check "reads use the prod context" 'grep -q "^ctx=lfx-v2-prod " "${WORK}/calls"'
  teardown
}

test_failed_delete_reports_partial_state() {
  setup; write_state "$(full_state)"
  FAKE_FAIL_DELETE_USER="team:formation#member" run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "failed delete exits 4" '[[ $STATUS -eq 4 ]]'
  check "failed delete names the remaining tuple" '[[ "$OUT" == *"still present: team:formation#member owner"* ]]'
  teardown
}

test_same_count_user_tuple_change_fails_verification() {
  setup; write_state "$(full_state)"
  FAKE_SWAP_USER_ON_DELETE=1 run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "same-count user change exits 4" '[[ $STATUS -eq 4 ]]'
  check "same-count user change is reported without identity" '[[ "$OUT" == *"user tuple set changed"* && "$OUT" != *"writer-two"* ]]'
  teardown
}

test_unreadable_store_is_not_treated_as_empty() {
  setup; write_state "$(full_state)"
  FAKE_READ_GARBAGE=1 run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "unreadable read exits 1" '[[ $STATUS -eq 1 ]]'
  check "no delete after unreadable read" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_term_trap_exits_instead_of_continuing() {
  setup
  set +e
  OUT="$(bash -c 'source <(sed "$d" "$1"); kill -TERM $$; echo continued' _ "$SCRIPT" 2>&1)"
  STATUS=$?
  set -e
  check "TERM exits 143" '[[ $STATUS -eq 143 ]]'
  check "TERM does not continue" '[[ "$OUT" != *"continued"* ]]'
  teardown
}

test_rerun_after_cleanup_is_a_no_op() {
  setup
  write_state "$(full_state | jq '[.[] | select((.relation == "auditor" or .relation == "owner" or .relation == "marketing_ops") and (.user | startswith("team:")) and .object != "project:00000000-0000-4000-8000-000000000099" | not)]')"
  run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "rerun exits 0" '[[ $STATUS -eq 0 && "$OUT" == *"Nothing to delete"* ]]'
  check "rerun issues no delete" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

for t in $(declare -F | awk '{print $3}' | grep '^test_'); do
  "$t"
done

echo "${PASSES} passed, ${FAILURES} failed"
[[ $FAILURES -eq 0 ]]
