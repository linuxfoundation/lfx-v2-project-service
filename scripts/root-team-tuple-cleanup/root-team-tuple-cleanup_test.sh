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
  mkdir -p "${WORK}/bin" "${WORK}/pods" "${WORK}/tmp"
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
labels=""
output=""
i=0
while [[ $i -lt ${#args[@]} ]]; do
  a="${args[$i]}"
  case "$a" in
    -o) output="${args[$((i + 1))]}"; i=$((i + 2)); continue ;;
    --context|-n|--namespace|--image|--restart|-l|--request-timeout) i=$((i + 2)); continue ;;
    --labels=*) labels="${a#--labels=}"; i=$((i + 1)); continue ;;
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
    # Record the mode of the script's pod state file on every pod, so a test can
    # see whether it stays private across pods.
    for f in "${TMPDIR:?}"/root-team-tuple-cleanup-pod.*; do
      [[ -e "$f" ]] && { stat -c %a "$f" 2>/dev/null || stat -f %Lp "$f"; } >>"${dir}/pod-state-modes"
    done
    if [[ -n "${FAKE_RUN_ALREADY_EXISTS:-}" ]]; then
      # Another operator's pod already owns this name.
      printf '%s' "someone-else" >"${dir}/pods/${pod}.token"
      echo "Error from server (AlreadyExists): pods \"${pod}\" already exists" >&2
      exit 1
    fi
    echo "$pod" >>"${dir}/pod-names"
    printf '%s' "${labels#root-tuple-cleanup-token=}" >"${dir}/pods/${pod}.token"
    image="$(jq -r '.spec.containers[0].image' <<<"$overrides")"
    mapfile_args="$(jq -r '.spec.containers[0].args | join(" ")' <<<"$overrides")"
    echo "ctx=${ctx} image=${image} args=${mapfile_args}" >>"${dir}/calls"
    command="$(jq -r '.spec.containers[0].args[0]' <<<"$overrides")"
    sub="$(jq -r '.spec.containers[0].args[1]' <<<"$overrides")"
    state="${dir}/state.json"
    phase="Succeeded"
    out="{}"
    [[ -z "${FAKE_POD_OUT:-}" ]] || out="$FAKE_POD_OUT"
    if [[ "$command" == "nats" && "$sub" == "kv" ]]; then
      out="${FAKE_CANONICAL_ROOT_UID:?}"
    elif [[ "$command" == "model" && "$sub" == "get" ]]; then
      auditor_line='    define auditor: [user, team#member] or executive_director or writer or auditor from parent'
      marketing_line='    define marketing_ops: [team#member] or marketing_ops from parent'
      owner_line='    define owner: [team#member] or global_owner'
      if [[ -n "${FAKE_MODEL_HAS_OWNER_PARENT:-}" ]]; then
        owner_line='    define owner: [team#member] or global_owner or owner from parent'
      elif [[ -n "${FAKE_MODEL_RELEASE_3:-}" ]]; then
        owner_line='    define owner: global_owner'
        auditor_line='    define auditor: [user] or executive_director or writer or auditor from parent'
        marketing_line=''
      elif [[ -n "${FAKE_MODEL_NO_MARKETING_OPS:-}" ]]; then
        marketing_line=''
      fi
      out="$(printf '%s\n' 'model' '  schema 1.1' 'type project' '  relations' "$owner_line" "$auditor_line" "$marketing_line" 'type team' '  relations' '    define member: [user]')"
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
      if [[ -n "${FAKE_MUTATE_ON_SECOND_READ:-}" && $read_count -eq 2 ]]; then
        jq "$FAKE_MUTATE_ON_SECOND_READ" "$state" >"${state}.tmp" && mv "${state}.tmp" "$state"
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
        if [[ -n "${FAKE_MUTATE_ON_DELETE:-}" && ! -e "${dir}/mutated" ]]; then
          jq "$FAKE_MUTATE_ON_DELETE" "$state" >"${state}.tmp" && mv "${state}.tmp" "$state"
          : >"${dir}/mutated"
        fi
      fi
    fi
    printf '%s' "$out" >"${dir}/pods/${pod}.out"
    printf '%s' "$phase" >"${dir}/pods/${pod}.phase"
    ;;
  get)
    if [[ "$pod" == "deployments" && -n "${FAKE_DEPLOYMENTS_JSON:-}" ]]; then
      printf '%s\n' "$FAKE_DEPLOYMENTS_JSON"
    elif [[ "$pod" == "deployments" ]]; then
      gateway_model="${FAKE_GATEWAY_MODEL_ID:?}"
      gateway_store="${FAKE_GATEWAY_STORE_ID:?}"
      # A finished rollout: every replica updated, ready and available.
      updated=2; ready=2; available=2; total=2
      if [[ -n "${FAKE_ROLLOUT_INCOMPLETE:-}" ]]; then
        updated=1; ready=1; available=1; total=3
      fi
      if [[ -e "${dir}/gateway-read" ]]; then
        gateway_model="${FAKE_GATEWAY_MODEL_ID_LATER:-$gateway_model}"
        gateway_store="${FAKE_GATEWAY_STORE_ID_LATER:-$gateway_store}"
      fi
      : >"${dir}/gateway-read"
      jq -cn --arg id "$gateway_model" --arg store "$gateway_store" \
        --argjson updated "$updated" --argjson ready "$ready" \
        --argjson available "$available" --argjson total "$total" '{
        items: [{metadata: {generation: 1}, status: {observedGeneration: 1, replicas: $total,
          updatedReplicas: $updated, readyReplicas: $ready, availableReplicas: $available},
        spec: {replicas: 2, template: {spec: {containers: [{
          env: [
            {name: "OPENFGA_AUTH_MODEL_ID", value: $id},
            {name: "OPENFGA_STORE_ID", value: $store}
          ]
        }]}}}}]
      }'
    elif [[ "$output" == *labels* ]]; then
      cat "${dir}/pods/${pod}.token" 2>/dev/null || true
    else
      cat "${dir}/pods/${pod}.phase"
    fi
    ;;
  logs) cat "${dir}/pods/${pod}.out" ;;
  delete) rm -f "${dir}/pods/${pod}.out" "${dir}/pods/${pod}.phase" "${dir}/pods/${pod}.token" ;;
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
  OUT="$(PATH="${WORK}/bin:${PATH}" FAKE_KUBECTL_DIR="$WORK" TMPDIR="${WORK}/tmp" \
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
  OUT="$(printf '%s\n' "$input" | PATH="${WORK}/bin:${PATH}" FAKE_KUBECTL_DIR="$WORK" TMPDIR="${WORK}/tmp" \
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

# heimdall_deployments STORES MODELS   -- a Heimdall deployment list with one container
# per ID; STORES and MODELS are JSON arrays.
heimdall_deployments() {
  jq -cn --argjson stores "$1" --argjson models "$2" '{items: [{spec: {template: {spec: {containers: (
    [$stores[] | {env: [{name: "OPENFGA_STORE_ID", value: .}]}]
    + [$models[] | {env: [{name: "OPENFGA_AUTH_MODEL_ID", value: .}]}]
  )}}}}]}'
}

test_refuses_zero_or_several_gateway_ids() {
  local name deployments reason
  local id_a="01ARZ3NDEKTSV4RRFFQ69G5FAV" id_b="01ARZ3NDEKTSV4RRFFQ69G5FAW"
  # reason is read only inside a check() condition.
  # shellcheck disable=SC2034
  for name in no-store two-stores no-model two-models; do
    case "$name" in
      no-store) deployments="$(heimdall_deployments '[]' "[\"${id_a}\"]")"; reason="one store ID on the Heimdall deployment, found 0" ;;
      two-stores) deployments="$(heimdall_deployments "[\"${id_a}\",\"${id_b}\"]" "[\"${id_a}\"]")"; reason="one store ID on the Heimdall deployment, found 2" ;;
      no-model) deployments="$(heimdall_deployments "[\"${id_a}\"]" '[]')"; reason="one model ID on the Heimdall deployment, found 0" ;;
      two-models) deployments="$(heimdall_deployments "[\"${id_a}\"]" "[\"${id_a}\",\"${id_b}\"]")"; reason="one model ID on the Heimdall deployment, found 2" ;;
    esac
    setup; write_state "$(full_state)"
    FAKE_DEPLOYMENTS_JSON="$deployments" run_script --env dev --root-uid "$ROOT_UID"
    check "${name} on Heimdall refused" '[[ $STATUS -eq 1 && "$OUT" == *"$reason"* ]]'
    check "no pod for ${name} on Heimdall" '[[ ! -s "${WORK}/calls" ]]'
    teardown
  done
}

test_dry_run_is_default_and_deletes_nothing() {
  setup; write_state "$(full_state)"
  run_script --env dev --root-uid "$ROOT_UID"
  check "dry run exits 0" '[[ $STATUS -eq 0 ]]'
  check "dry run issues no delete" '[[ $(delete_calls) -eq 0 ]]'
  check "dry run plans five deletes" '[[ $(grep -c "^  delete " <<<"$OUT") -eq 5 ]]'
  check "dry run prints rollback writes" '[[ $(grep -c "fga tuple write team:" <<<"$OUT") -eq 5 ]]'
  check "every rollback write pins the validated gateway model" '[[ $(grep -c "fga tuple write team:.* --model-id 01ARZ3NDEKTSV4RRFFQ69G5FAV$" <<<"$OUT") -eq 5 ]]'
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

test_dry_run_separates_absent_from_planned_deletes() {
  setup
  write_state "$(full_state | jq '[.[] | select((.user == "team:lf-contractor#member" and .relation == "auditor") | not)]')"
  run_script --env dev --root-uid "$ROOT_UID"
  check "partial dry run exits 0" '[[ $STATUS -eq 0 ]]'
  check "the removed tuple is reported absent" '[[ "$OUT" == *"  already absent team:lf-contractor#member auditor"* ]]'
  check "the removed tuple is not planned" '[[ "$OUT" != *"  delete team:lf-contractor#member auditor"* ]]'
  check "the present tuples are still planned" '[[ $(grep -c "^  delete " <<<"$OUT") -eq 4 ]]'
  check "rollback covers only the planned deletes" '[[ $(grep -c "fga tuple write team:" <<<"$OUT") -eq 4 && "$OUT" != *"fga tuple write team:lf-contractor#member"* ]]'
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

test_refuses_model_without_team_restrictions() {
  setup; write_state "$(full_state)"
  FAKE_MODEL_RELEASE_3=1 run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "release-3 model refused" '[[ $STATUS -eq 3 && "$OUT" == *"no longer accepts team#member on project owner auditor marketing_ops"* ]]'
  check "no tuple read after the restrictions are gone" '! grep -q "args=tuple read" "${WORK}/calls"'
  check "no delete after the restrictions are gone" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_refuses_model_without_marketing_ops() {
  setup; write_state "$(full_state)"
  FAKE_MODEL_NO_MARKETING_OPS=1 run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "model without marketing_ops refused" '[[ $STATUS -eq 3 && "$OUT" == *"team#member on project marketing_ops;"* ]]'
  check "no delete without marketing_ops" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_dry_run_warns_on_release_3_model() {
  setup; write_state "$(full_state)"
  FAKE_MODEL_RELEASE_3=1 run_script --env dev --root-uid "$ROOT_UID"
  check "release-3 dry run warns" '[[ $STATUS -eq 0 && "$OUT" == *"warning: deployed model no longer accepts team#member"* ]]'
  check "release-3 dry run issues no delete" '[[ $(delete_calls) -eq 0 ]]'
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

test_refuses_conditioned_replacement_grant() {
  setup
  write_state "$(full_state | jq '[.[] | if .user == "team:global-project-writers#member" and .relation == "global_writer" then . + {condition: {name: "c", context: {k: "ctx-secret-value"}}} else . end]')"
  run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "conditioned replacement grant refused" '[[ $STATUS -eq 3 && "$OUT" == *"condition: team:global-project-writers#member global_writer"* ]]'
  check "condition context is not printed" '[[ "$OUT" != *"ctx-secret-value"* ]]'
  check "no delete with conditioned replacement" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_replacement_conditioned_during_apply_fails_verification() {
  setup; write_state "$(full_state)"
  FAKE_MUTATE_ON_DELETE='[.[] | if .user == "team:global-project-writers#member" and .relation == "global_writer" then . + {condition: {name: "c"}} else . end]' \
    run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "replacement conditioned during apply exits 4" '[[ $STATUS -eq 4 ]]'
  check "conditioned replacement is reported" '[[ "$OUT" == *"replacement grant missing or conditioned after apply: team:global-project-writers#member global_writer"* ]]'
  teardown
}

test_user_condition_change_fails_verification() {
  setup
  write_state "$(full_state | jq '[.[] | if .user == "user:writer-one" then . + {condition: {name: "c", context: {k: "ctx-before-value"}}} else . end]')"
  FAKE_MUTATE_ON_DELETE='[.[] | if .user == "user:writer-one" then .condition.context.k = "ctx-after-value" else . end]' \
    run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "user condition change exits 4" '[[ $STATUS -eq 4 ]]'
  check "user condition change is reported" '[[ "$OUT" == *"user tuple set changed"* ]]'
  check "user condition context is not printed" '[[ "$OUT" != *"ctx-before-value"* && "$OUT" != *"ctx-after-value"* && "$OUT" != *"writer-one"* ]]'
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

test_global_auditors_team_on_root_auditor_is_legacy() {
  setup
  write_state "$(full_state | jq --arg o "project:${ROOT_UID}" '. + [{user: "team:global-project-auditors#member", relation: "auditor", object: $o}]')"
  run_script --env dev --root-uid "$ROOT_UID"
  check "dry run accepts the global auditors team on auditor" '[[ $STATUS -eq 0 ]]'
  check "dry run plans its delete" '[[ "$OUT" == *"  delete team:global-project-auditors#member auditor"* ]]'
  check "dry run prints its rollback" '[[ "$OUT" == *"fga tuple write team:global-project-auditors#member auditor project:${ROOT_UID}"* ]]'
  run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "apply with the global auditors team exits 0" '[[ $STATUS -eq 0 ]]'
  check "apply issues six deletes" '[[ $(delete_calls) -eq 6 ]]'
  check "global auditors team auditor removed" '! has_tuple "team:global-project-auditors#member" auditor'
  check "global auditors team global_auditor kept" 'has_tuple "team:global-project-auditors#member" global_auditor'
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

test_apply_rechecks_preflight_after_confirmation() {
  setup; write_state "$(full_state)"
  FAKE_DROP_REPLACEMENT_ON_SECOND_READ=1 \
    run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "replacement removed during confirmation refused" '[[ $STATUS -eq 3 && "$OUT" == *"replacement grant missing"* ]]'
  check "no delete after replacement removed" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_apply_refuses_changed_delete_plan_after_confirmation() {
  setup; write_state "$(full_state)"
  FAKE_MUTATE_ON_SECOND_READ='[.[] | select((.user == "team:lf-contractor#member" and .relation == "auditor") | not)]' \
    run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "changed delete plan refused" '[[ $STATUS -eq 3 && "$OUT" == *"delete plan changed during confirmation"* ]]'
  check "no delete after the plan changed" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_apply_refuses_store_change_after_confirmation() {
  setup; write_state "$(full_state)"
  FAKE_GATEWAY_STORE_ID_LATER="01ARZ3NDEKTSV4RRFFQ69G5FAW" \
    run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "store change after confirmation refused" '[[ $STATUS -eq 3 && "$OUT" == *"store or model ID changed"* ]]'
  check "changed store ID is not printed" '[[ "$OUT" != *"01ARZ3NDEKTSV4RRFFQ69G5FAW"* ]]'
  check "no delete after store change" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_apply_refuses_model_change_after_confirmation() {
  setup; write_state "$(full_state)"
  FAKE_GATEWAY_MODEL_ID_LATER="01ARZ3NDEKTSV4RRFFQ69G5FAW" \
    run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "model change after confirmation refused" '[[ $STATUS -eq 3 && "$OUT" == *"store or model ID changed"* ]]'
  check "no delete after model change" '[[ $(delete_calls) -eq 0 ]]'
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
  # The rollback lines now print the model ID, so it must differ from the store ID this test masks.
  FAKE_GATEWAY_MODEL_ID="01ARZ3NDEKTSV4RRFFQ69G5FAW" run_script --env prod --root-uid "$ROOT_UID"
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

test_pod_state_file_stays_private_across_pods() {
  setup; write_state "$(full_state)"
  (umask 022; run_script --env dev --root-uid "$ROOT_UID")
  check "several pods ran" '[[ $(wc -l <"${WORK}/pod-state-modes") -ge 3 ]]'
  check "state file is 0600 for every pod" '[[ -z "$(grep -v "^600$" "${WORK}/pod-state-modes")" ]]'
  check "state file is removed at exit" '[[ -z "$(ls "${WORK}/tmp")" ]]'
  teardown
}

test_pod_creation_collision_does_not_delete_a_foreign_pod() {
  setup; write_state "$(full_state)"
  FAKE_RUN_ALREADY_EXISTS=1 run_script --env dev --root-uid "$ROOT_UID"
  check "creation failure stops the run" '[[ $STATUS -ne 0 && "$OUT" == *"could not be created"* ]]'
  check "the other operator's pod is left alone" '[[ "$(cat "${WORK}"/pods/*.token)" == "someone-else" ]]'
  teardown
}

test_pod_names_carry_a_random_ownership_token() {
  setup; write_state "$(full_state)"
  run_script --env dev --root-uid "$ROOT_UID"
  check "every pod name is a 64-bit random token plus a sequence" '[[ -z "$(grep -Ev "^root-tuple-cleanup-[0-9a-f]{16}-[0-9]+$" "${WORK}/pod-names")" ]]'
  check "the token is the same for one invocation" '[[ $(cut -d- -f4 "${WORK}/pod-names" | sort -u | wc -l) -eq 1 ]]'
  teardown
}

test_apply_refused_while_heimdall_is_rolling_out() {
  setup; write_state "$(full_state)"
  FAKE_ROLLOUT_INCOMPLETE=1 run_script_stdin "$ROOT_UID" --env dev --root-uid "$ROOT_UID" --apply
  check "apply refused mid-rollout" '[[ $STATUS -eq 3 && "$OUT" == *"still rolling out"* ]]'
  check "no tuple read mid-rollout" '! grep -q "args=tuple read" "${WORK}/calls"'
  check "no delete mid-rollout" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

test_dry_run_warns_while_heimdall_is_rolling_out() {
  setup; write_state "$(full_state)"
  FAKE_ROLLOUT_INCOMPLETE=1 run_script --env dev --root-uid "$ROOT_UID"
  check "mid-rollout dry run warns" '[[ $STATUS -eq 0 && "$OUT" == *"warning: Heimdall is still rolling out"* ]]'
  check "mid-rollout dry run issues no delete" '[[ $(delete_calls) -eq 0 ]]'
  teardown
}

# write_lib   -- the script without its final `main "$@"`, for sourcing its functions
# and traps. Sourced from a file, not a process substitution: bash 3.2 sources
# nothing from <(...), which would let a test pass without loading the script.
write_lib() {
  sed '$d' "$SCRIPT" >"${WORK}/lib.sh"
}

test_term_trap_exits_instead_of_continuing() {
  setup; write_lib
  set +e
  OUT="$(bash -c 'source "$1"; kill -TERM $$; echo continued' _ "${WORK}/lib.sh" 2>&1)"
  STATUS=$?
  set -e
  check "TERM exits 143" '[[ $STATUS -eq 143 ]]'
  check "TERM does not continue" '[[ "$OUT" != *"continued"* ]]'
  teardown
}

test_pod_output_masks_store_id() {
  setup; write_lib
  set +e
  OUT="$(PATH="${WORK}/bin:${PATH}" FAKE_KUBECTL_DIR="$WORK" TMPDIR="${WORK}/tmp" \
    FAKE_POD_OUT="Error: store ${FAKE_STORE_ID} unavailable" \
    bash -c 'source "$1"; CTX=lfx-v2-dev; STORE_ID="$2"; init_pod_state_file; run_fga_pod store get' \
    _ "${WORK}/lib.sh" "$FAKE_STORE_ID" 2>&1)"
  STATUS=$?
  set -e
  check "pod output is printed" '[[ $STATUS -eq 0 && "$OUT" == *"unavailable"* ]]'
  check "pod output does not print the full store ID" '[[ "$OUT" != *"$FAKE_STORE_ID"* ]]'
  check "pod output carries the masked store ID" '[[ "$OUT" == *"${FAKE_STORE_ID:0:4}...${FAKE_STORE_ID: -4}"* ]]'
  teardown
}

# signal_with_current_pod TOKEN_FILE_CONTENT   -- TERM arrives while CURRENT_POD names a
# pod, as it does during a create that collided with someone else's pod.
signal_with_current_pod() {
  mkdir -p "${WORK}/pods"
  printf '%s' "$1" >"${WORK}/pods/target-pod.token"
  write_lib
  set +e
  OUT="$(PATH="${WORK}/bin:${PATH}" FAKE_KUBECTL_DIR="$WORK" TMPDIR="${WORK}/tmp" \
    bash -c 'source "$1"; CTX=ctx; POD_TOKEN=aaaaaaaaaaaaaaaa; CURRENT_POD=target-pod; kill -TERM $$' _ "${WORK}/lib.sh" 2>&1)"
  STATUS=$?
  set -e
}

test_signal_does_not_delete_a_pod_without_our_label() {
  setup
  signal_with_current_pod "someone-else"
  check "TERM still exits 143" '[[ $STATUS -eq 143 ]]'
  check "a foreign pod survives the signal path" '[[ -e "${WORK}/pods/target-pod.token" ]]'
  check "the skipped delete is reported by pod name" '[[ "$OUT" == *"target-pod"* ]]'
  teardown
}

test_signal_deletes_a_pod_with_our_label() {
  setup
  signal_with_current_pod "aaaaaaaaaaaaaaaa"
  check "our own pod is deleted on the signal path" '[[ ! -e "${WORK}/pods/target-pod.token" ]]'
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
