#!/usr/bin/env bash
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
#
# Tests for marketing-ops-grant.sh. A fake kubectl on PATH serves Heimdall's
# deployment and records every pod the script creates, so no cluster is used.
#
# Run: bash scripts/marketing-ops-grant/marketing-ops-grant_test.sh

# check() evaluates its condition after the run, so the single quotes are intended.
# shellcheck disable=SC2016

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="${HERE}/marketing-ops-grant.sh"

ROOT_UID="00000000-0000-4000-8000-000000000001"
PROJECT_UID="00000000-0000-4000-8000-000000000002"
# Read only inside check() conditions.
# shellcheck disable=SC2034
FGA_CLI_IMAGE="openfga/cli:v0.7.20@sha256:26acde96d90420e53fe361a740dcceba4b67f1c49e4f35117cd0c19c83ac5068"
STORE_A="01ARZ3NDEKTSV4RRFFQ69G5FAV"
STORE_B="01BX5ZZKBKACTAV9WEVGEMMVRZ"
FAILURES=0
PASSES=0

setup() {
  WORK="$(mktemp -d)"
  mkdir -p "${WORK}/bin"
  : >"${WORK}/runs"
  : >"${WORK}/lookups"
  cat >"${WORK}/bin/kubectl" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
dir="${FAKE_KUBECTL_DIR:?}"
case " $* " in
  *" get deployments "*)
    echo x >>"${dir}/lookups"
    if [[ -n "${FAKE_DEPLOYMENTS_JSON:-}" ]]; then
      printf '%s\n' "$FAKE_DEPLOYMENTS_JSON"
      exit 0
    fi
    n="$(wc -l <"${dir}/lookups" | tr -d ' ')"
    store="$FAKE_STORE_FIRST"
    if [[ "$n" -gt 1 && -n "${FAKE_STORE_LATER:-}" ]]; then
      store="$FAKE_STORE_LATER"
    fi
    printf '{"items":[{"spec":{"template":{"spec":{"containers":[{"env":[{"name":"OPENFGA_STORE_ID","value":"%s"}]}]}}}}]}\n' "$store"
    ;;
  *" run "*) printf 'run %s\n' "$*" >>"${dir}/runs" ;;
  *" get pod "*) printf Succeeded ;;
  *" logs "*) printf '%s\n' "${FAKE_LOGS:-{\"allowed\":true\}}" ;;
  *) : ;;
esac
FAKE
  chmod +x "${WORK}/bin/kubectl"
}

teardown() {
  rm -rf "$WORK"
}

# run_grant STORE_FIRST STORE_LATER -- prod --global grant with the operator
# typing the matching username at the confirmation prompt.
run_grant() {
  RC=0
  OUT="$(printf 'alice.example\n' | PATH="${WORK}/bin:${PATH}" FAKE_KUBECTL_DIR="$WORK" \
    FAKE_STORE_FIRST="$1" FAKE_STORE_LATER="${2:-}" \
    bash "$SCRIPT" grant --env prod --user alice.example --global --root-uid "$ROOT_UID" 2>&1)" || RC=$?
}

# run_check   -- dev per-project check, which runs pods without a confirmation prompt.
run_check() {
  RC=0
  OUT="$(PATH="${WORK}/bin:${PATH}" FAKE_KUBECTL_DIR="$WORK" FAKE_STORE_FIRST="$STORE_A" \
    bash "$SCRIPT" check --env dev --user alice.example --project "$PROJECT_UID" 2>&1)" || RC=$?
}

# deployments_with_stores ID...   -- a Heimdall deployment list carrying each ID.
deployments_with_stores() {
  local id containers="" sep=""
  for id in "$@"; do
    containers+="${sep}{\"env\":[{\"name\":\"OPENFGA_STORE_ID\",\"value\":\"${id}\"}]}"
    sep=","
  done
  printf '{"items":[{"spec":{"template":{"spec":{"containers":[%s]}}}}]}' "$containers"
}

check() {
  local name="$1" cond="$2"
  if eval "$cond"; then
    PASSES=$((PASSES + 1))
  else
    FAILURES=$((FAILURES + 1))
    echo "FAIL: ${name}"
    echo "  rc=${RC} output: ${OUT}"
  fi
}

test_store_switch_during_confirmation_is_refused() {
  setup
  run_grant "$STORE_A" "$STORE_B"
  check "refuses when the store changes after the confirmation" '[[ "$RC" -ne 0 ]]'
  check "creates no pod after the store changed" '[[ ! -s "${WORK}/runs" ]]'
  check "says the store changed" 'grep -qi "changed" <<<"$OUT"'
  check "does not print a full store id" '! grep -q "$STORE_A" <<<"$OUT" && ! grep -q "$STORE_B" <<<"$OUT"'
  teardown
}

test_stable_store_still_writes() {
  setup
  run_grant "$STORE_A" ""
  check "writes when the store is unchanged" '[[ -s "${WORK}/runs" ]]'
  check "looks the store up again after the confirmation" '[[ "$(wc -l <"${WORK}/lookups" | tr -d " ")" -ge 2 ]]'
  teardown
}

test_caller_store_id_is_refused() {
  setup
  FGA_STORE_ID="$STORE_B" run_check
  check "refuses a caller-set FGA_STORE_ID" '[[ "$RC" -ne 0 && "$OUT" == *"resolved from Heimdall"* ]]'
  check "does not look Heimdall up for a caller-set store" '[[ ! -s "${WORK}/lookups" ]]'
  check "creates no pod for a caller-set store" '[[ ! -s "${WORK}/runs" ]]'
  teardown
}

test_heimdall_store_id_must_be_one_ulid() {
  local name deployments reason
  # reason is read only inside a check() condition.
  # shellcheck disable=SC2034
  for name in none two malformed; do
    case "$name" in
      none) deployments='{"items":[]}'; reason="found 0" ;;
      two) deployments="$(deployments_with_stores "$STORE_A" "$STORE_B")"; reason="found 2" ;;
      malformed) deployments="$(deployments_with_stores "not-a-store-id")"; reason="not a ULID" ;;
    esac
    setup
    FAKE_DEPLOYMENTS_JSON="$deployments" run_check
    check "refuses ${name} Heimdall store ID" '[[ "$RC" -ne 0 && "$OUT" == *"$reason"* ]]'
    check "creates no pod for ${name} Heimdall store ID" '[[ ! -s "${WORK}/runs" ]]'
    teardown
  done
}

test_pod_output_masks_store_id() {
  setup
  # Only an undecidable check echoes the raw pod output, so that is where the ID could leak.
  FAKE_LOGS="Error: store ${STORE_A} unavailable" run_check
  check "an undecidable check fails" '[[ "$RC" -ne 0 && "$OUT" == *"could not determine result"* ]]'
  check "pod output does not print the full store ID" '! grep -q "$STORE_A" <<<"$OUT"'
  check "pod output carries the masked store ID" 'grep -q "${STORE_A:0:4}\.\.\.${STORE_A: -4}" <<<"$OUT"'
  teardown
}

test_pods_use_the_digest_pinned_image() {
  setup
  run_check
  check "pods were created" '[[ -s "${WORK}/runs" ]]'
  check "every pod passes the pinned --image" '[[ $(grep -c "^run " "${WORK}/runs") -eq $(grep -cF -- "--image=${FGA_CLI_IMAGE}" "${WORK}/runs") ]]'
  check "every pod spec names the pinned image" '[[ $(grep -c "^run " "${WORK}/runs") -eq $(grep -cF "\"image\": \"${FGA_CLI_IMAGE}\"" "${WORK}/runs") ]]'
  teardown
}

test_store_switch_during_confirmation_is_refused
test_stable_store_still_writes
test_caller_store_id_is_refused
test_heimdall_store_id_must_be_one_ulid
test_pod_output_masks_store_id
test_pods_use_the_digest_pinned_image

echo "${PASSES} passed, ${FAILURES} failed"
[[ "$FAILURES" -eq 0 ]]
