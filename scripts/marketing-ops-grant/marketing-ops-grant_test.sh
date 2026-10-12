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
    n="$(wc -l <"${dir}/lookups" | tr -d ' ')"
    store="$FAKE_STORE_FIRST"
    if [[ "$n" -gt 1 && -n "${FAKE_STORE_LATER:-}" ]]; then
      store="$FAKE_STORE_LATER"
    fi
    printf '{"items":[{"spec":{"template":{"spec":{"containers":[{"env":[{"name":"OPENFGA_STORE_ID","value":"%s"}]}]}}}}]}\n' "$store"
    ;;
  *" run "*) echo run >>"${dir}/runs" ;;
  *" get pod "*) printf Succeeded ;;
  *" logs "*) echo '{"allowed":true}' ;;
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

test_store_switch_during_confirmation_is_refused
test_stable_store_still_writes

echo "${PASSES} passed, ${FAILURES} failed"
[[ "$FAILURES" -eq 0 ]]
