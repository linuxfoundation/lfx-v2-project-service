#!/usr/bin/env bash
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
#
# Tests for global-grant-backfill.sh and verify-global-grants.sh. A fake
# kubectl on PATH serves deployment env values and captures the Job manifest,
# so no cluster is used.
#
# Run: bash scripts/global-grant-backfill/global-grant-backfill_test.sh

# check() evaluates its condition after the run, so the single quotes are intended.
# shellcheck disable=SC2016

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKFILL="${HERE}/global-grant-backfill.sh"
VERIFY="${HERE}/verify-global-grants.sh"

REPOSITORY="ghcr.io/linuxfoundation/lfx-v2-project-service/project-cli"
DIGEST="$(printf 'a%.0s' {1..64})"
IMAGE="${REPOSITORY}@sha256:${DIGEST}"
FAKE_NATS="nats://fake-nats.example:4222"
FAKE_STORE="01ARZ3NDEKTSV4RRFFQ69G5FAV"
FAILURES=0
PASSES=0

setup() {
  WORK="$(mktemp -d)"
  mkdir -p "${WORK}/bin"
  : >"${WORK}/calls"
  : >"${WORK}/kubeconfig"
  cat >"${WORK}/bin/kubectl" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
dir="${FAKE_KUBECTL_DIR:?}"
printf '%s\n' "$*" >>"${dir}/calls"
args=("$@")
ctx="" verb="" label=""
i=0
while [[ $i -lt ${#args[@]} ]]; do
  case "${args[$i]}" in
    --context) ctx="${args[$((i + 1))]}"; i=$((i + 2)); continue ;;
    -l) label="${args[$((i + 1))]}"; i=$((i + 2)); continue ;;
    -n|-o|-f) i=$((i + 2)); continue ;;
    --request-timeout=*) i=$((i + 1)); continue ;;
  esac
  [[ -z "$verb" ]] && verb="${args[$i]}"
  i=$((i + 1))
done
printf '%s\n' "$ctx" >>"${dir}/contexts"
case "$verb" in
  get)
    [[ -z "${FAKE_GET_FAIL:-}" ]] || exit 1
    case "$label" in
      app.kubernetes.io/name=lfx-v2-project-service) values="${FAKE_NATS_URLS}"; name="NATS_URL" ;;
      app.kubernetes.io/name=heimdall) values="${FAKE_STORE_IDS}"; name="OPENFGA_STORE_ID" ;;
      *) echo "unexpected label: ${label}" >&2; exit 1 ;;
    esac
    printf '%s' "$values" | jq -Rsc --arg name "$name" '{items: [split("\n")[]
      | select(length > 0)
      | {spec: {template: {spec: {containers: [{env: [{name: $name, value: .}]}]}}}}]}'
    ;;
  create)
    [[ -z "${FAKE_CREATE_FAIL:-}" ]] || exit 1
    cat >"${dir}/manifest.json"
    ;;
  *) echo "unexpected verb: ${verb}" >&2; exit 1 ;;
esac
FAKE
  chmod +x "${WORK}/bin/kubectl"
  [[ "$(PATH="${WORK}/bin:${PATH}" command -v kubectl)" == "${WORK}/bin/kubectl" ]] || {
    echo "fake kubectl is not first on PATH; refusing to run" >&2
    exit 1
  }
}

teardown() {
  rm -rf "$WORK"
}

# run SCRIPT STDIN ARGS... ; records RC, stdout and stderr under $WORK.
run() {
  local script="$1" input="$2"
  shift 2
  RC=0
  printf '%s' "$input" | env \
    PATH="${WORK}/bin:${PATH}" \
    KUBECONFIG="${WORK}/kubeconfig" \
    FAKE_KUBECTL_DIR="$WORK" \
    FAKE_NATS_URLS="${FAKE_NATS_URLS-$FAKE_NATS}" \
    FAKE_STORE_IDS="${FAKE_STORE_IDS-$FAKE_STORE}" \
    FAKE_GET_FAIL="${FAKE_GET_FAIL-}" \
    FAKE_CREATE_FAIL="${FAKE_CREATE_FAIL-}" \
    bash "$script" "$@" >"${WORK}/stdout" 2>"${WORK}/stderr" || RC=$?
}

check() {
  local name="$1" condition="$2"
  if eval "$condition"; then
    PASSES=$((PASSES + 1))
  else
    FAILURES=$((FAILURES + 1))
    echo "FAIL: ${name}"
    echo "  condition: ${condition}"
    echo "  rc=${RC}"
    sed 's/^/  stderr: /' "${WORK}/stderr"
  fi
}

manifest() {
  jq -c "$1" "${WORK}/manifest.json"
}

no_job_created() {
  [[ ! -e "${WORK}/manifest.json" ]] && ! grep -qE '(^| )create( |$)' "${WORK}/calls"
}

no_kubectl_calls() {
  [[ ! -s "${WORK}/calls" ]]
}

hardened_manifest() {
  [[ "$(manifest '.spec.backoffLimit')" == "0" ]] &&
    [[ "$(manifest '.spec.ttlSecondsAfterFinished')" == "604800" ]] &&
    [[ "$(manifest '.spec.template.spec.restartPolicy')" == '"Never"' ]] &&
    [[ "$(manifest '.spec.template.spec.automountServiceAccountToken')" == "false" ]] &&
    [[ "$(manifest '.spec.template.spec.containers | length')" == "1" ]] &&
    [[ "$(manifest '.spec.template.spec.containers[0].image')" == "\"${IMAGE}\"" ]] &&
    [[ "$(manifest '.spec.template.spec.containers[0].securityContext')" == \
      '{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true,"runAsNonRoot":true}' ]]
}

output_hides() {
  ! grep -qF -- "$1" "${WORK}/stdout" && ! grep -qF -- "$1" "${WORK}/stderr"
}

test_backfill_rejects_bad_arguments() {
  local args
  for args in \
    "--image ${IMAGE}" \
    "--env staging --image ${IMAGE}" \
    "--env dev" \
    "--env dev --image ${REPOSITORY}:latest" \
    "--env dev --image ghcr.io/example/other@sha256:${DIGEST}" \
    "--env dev --image ghcrXio/linuxfoundation/lfx-v2-project-service/project-cli@sha256:${DIGEST}" \
    "--env dev --image ${IMAGE}0" \
    "--env dev --image ${REPOSITORY}@sha256:$(printf 'A%.0s' {1..64})" \
    "--env dev --image ${IMAGE} --concurrency 0" \
    "--env dev --image ${IMAGE} --concurrency 201" \
    "--env dev --image ${IMAGE} --concurrency 5x" \
    "--env dev --image ${IMAGE} --update"; do
    setup
    # shellcheck disable=SC2086
    run "$BACKFILL" "" $args
    check "backfill rejects: ${args}" '[[ $RC -eq 2 ]] && no_kubectl_calls'
    teardown
  done
}

test_backfill_dev_dry_run() {
  setup
  run "$BACKFILL" "" --env dev --image "$IMAGE"
  check "dev dry-run succeeds" '[[ $RC -eq 0 ]]'
  check "dev dry-run uses only the dev context" '[[ "$(sort -u "${WORK}/contexts")" == "lfx-v2-dev" ]]'
  check "dry-run args omit --update" \
    '[[ "$(manifest ".spec.template.spec.containers[0].args")" == "[\"sync\",\"reindex-projects\",\"--all\",\"--include-access\",\"--concurrency\",\"50\"]" ]]'
  check "dry-run Job name says dry-run" '[[ "$(manifest ".metadata.name")" == "\"project-global-grants-dry-run-"* ]]'
  check "backfill deadline is two hours" '[[ "$(manifest ".spec.activeDeadlineSeconds")" == "7200" ]]'
  check "backfill manifest is hardened" 'hardened_manifest'
  check "backfill env carries resolved NATS_URL only" \
    '[[ "$(manifest ".spec.template.spec.containers[0].env")" == "[{\"name\":\"NATS_URL\",\"value\":\"${FAKE_NATS}\"},{\"name\":\"LOG_LEVEL\",\"value\":\"info\"}]" ]]'
  check "backfill output hides NATS_URL" 'output_hides "$FAKE_NATS"'
  check "backfill reads the project-service deployment in its own namespace" \
    'grep -qE "get deployments -n project-service -l app.kubernetes.io/name=lfx-v2-project-service" "${WORK}/calls"'
  check "backfill Job is created in the lfx namespace" '[[ "$(manifest ".metadata.namespace")" == "\"lfx\"" ]]'
  teardown
}

test_backfill_prod_dry_run_needs_no_tty() {
  setup
  run "$BACKFILL" "" --env prod --image "$IMAGE" --concurrency 10
  check "prod dry-run succeeds without a terminal" '[[ $RC -eq 0 ]]'
  check "prod dry-run uses only the prod context" '[[ "$(sort -u "${WORK}/contexts")" == "lfx-v2-prod" ]]'
  check "prod dry-run passes concurrency and no --update" \
    '[[ "$(manifest ".spec.template.spec.containers[0].args | .[-2:]")" == "[\"--concurrency\",\"10\"]" ]]'
  teardown
}

test_backfill_dev_apply_with_confirmation() {
  setup
  run "$BACKFILL" $'apply global grants to dev\n' --env dev --image "$IMAGE" --apply
  check "dev apply succeeds with the exact phrase" '[[ $RC -eq 0 ]]'
  check "apply args end with --update" \
    '[[ "$(manifest ".spec.template.spec.containers[0].args")" == "[\"sync\",\"reindex-projects\",\"--all\",\"--include-access\",\"--concurrency\",\"50\",\"--update\"]" ]]'
  check "apply Job name says apply" '[[ "$(manifest ".metadata.name")" == "\"project-global-grants-apply-"* ]]'
  teardown
}

test_backfill_documents_only() {
  setup
  run "$BACKFILL" "" --env dev --image "$IMAGE" --documents-only
  check "documents-only dry-run succeeds" '[[ $RC -eq 0 ]]'
  check "documents-only args carry neither --include-access nor --update" \
    '[[ "$(manifest ".spec.template.spec.containers[0].args")" == "[\"sync\",\"reindex-projects\",\"--all\",\"--concurrency\",\"50\"]" ]]'
  check "documents-only manifest never mentions access" '! grep -q -- "--include-access" "${WORK}/manifest.json"'
  check "documents-only Job name says documents" '[[ "$(manifest ".metadata.name")" == "\"project-documents-dry-run-"* ]]'
  check "documents-only manifest is hardened" 'hardened_manifest'
  teardown

  setup
  run "$BACKFILL" $'republish project documents to dev\n' --env dev --image "$IMAGE" --documents-only --apply
  check "documents-only apply succeeds with its own phrase" '[[ $RC -eq 0 ]]'
  check "documents-only apply args end with --update and carry no --include-access" \
    '[[ "$(manifest ".spec.template.spec.containers[0].args")" == "[\"sync\",\"reindex-projects\",\"--all\",\"--concurrency\",\"50\",\"--update\"]" ]]'
  check "documents-only apply Job name says apply" '[[ "$(manifest ".metadata.name")" == "\"project-documents-apply-"* ]]'
  teardown

  # Each mode accepts only its own phrase, so one cannot be typed by habit for the other.
  setup
  run "$BACKFILL" $'apply global grants to dev\n' --env dev --image "$IMAGE" --documents-only --apply
  check "documents-only apply refuses the grants phrase" '[[ $RC -eq 2 ]] && no_job_created'
  teardown

  setup
  run "$BACKFILL" $'republish project documents to dev\n' --env dev --image "$IMAGE" --apply
  check "grants apply refuses the documents phrase" '[[ $RC -eq 2 ]] && no_job_created'
  teardown

  setup
  run "$BACKFILL" $'republish project documents to prod\n' --env prod --image "$IMAGE" --documents-only --apply
  check "documents-only prod apply without a terminal is refused" '[[ $RC -eq 2 ]] && no_job_created'
  teardown
}

test_backfill_apply_wrong_phrase() {
  local phrase
  for phrase in "" $'yes\n' $'apply global grants to prod\n' $'Apply global grants to dev\n'; do
    setup
    run "$BACKFILL" "$phrase" --env dev --image "$IMAGE" --apply
    check "dev apply refuses phrase '${phrase%$'\n'}'" '[[ $RC -eq 2 ]] && no_job_created'
    teardown
  done
}

test_backfill_prod_apply_requires_tty() {
  setup
  run "$BACKFILL" $'apply global grants to prod\n' --env prod --image "$IMAGE" --apply
  check "prod apply without a terminal is refused" '[[ $RC -eq 2 ]] && no_job_created'
  check "prod apply refusal names the terminal requirement" 'grep -q "interactive terminal" "${WORK}/stderr"'
  teardown
}

test_backfill_nats_url_resolution_failures() {
  setup
  FAKE_NATS_URLS="" run "$BACKFILL" "" --env dev --image "$IMAGE"
  check "missing NATS_URL creates no Job" '[[ $RC -eq 1 ]] && no_job_created'
  teardown

  setup
  FAKE_NATS_URLS=$'nats://one:4222\nnats://two:4222' run "$BACKFILL" "" --env dev --image "$IMAGE"
  check "ambiguous NATS_URL creates no Job" '[[ $RC -eq 1 ]] && no_job_created'
  check "ambiguous NATS_URL error hides values" 'output_hides "nats://one" && output_hides "nats://two"'
  teardown

  setup
  FAKE_NATS_URLS=$'nats://same:4222\nnats://same:4222' run "$BACKFILL" "" --env dev --image "$IMAGE"
  check "duplicate identical NATS_URL is accepted" '[[ $RC -eq 0 ]]'
  teardown

  setup
  FAKE_GET_FAIL=1 run "$BACKFILL" "" --env dev --image "$IMAGE"
  check "deployment read failure creates no Job" '[[ $RC -eq 1 ]] && no_job_created'
  teardown

  setup
  FAKE_CREATE_FAIL=1 run "$BACKFILL" "" --env dev --image "$IMAGE"
  check "Job create failure exits nonzero" '[[ $RC -eq 1 ]] && grep -q "could not create Job" "${WORK}/stderr"'
  teardown
}

test_backfill_env_value_is_not_injectable() {
  setup
  local hostile='nats://x"},{"name":"EVIL","value":"1'
  FAKE_NATS_URLS="$hostile" run "$BACKFILL" "" --env dev --image "$IMAGE"
  check "hostile NATS_URL stays one env value" \
    '[[ $RC -eq 0 ]] && [[ "$(manifest ".spec.template.spec.containers[0].env | length")" == "2" ]] && [[ "$(jq -r ".spec.template.spec.containers[0].env[0].value" "${WORK}/manifest.json")" == "$hostile" ]]'
  teardown
}

test_verify_rejects_bad_arguments() {
  local args
  for args in \
    "--image ${IMAGE}" \
    "--env dev" \
    "--env dev --image ${REPOSITORY}:latest" \
    "--env dev --image ${IMAGE} --concurrency 0" \
    "--env dev --image ${IMAGE} --concurrency 101" \
    "--env dev --image ${IMAGE} --apply"; do
    setup
    # shellcheck disable=SC2086
    run "$VERIFY" "" $args
    check "verify rejects: ${args}" '[[ $RC -eq 2 ]] && no_kubectl_calls'
    teardown
  done
}

test_verify_creates_read_only_job() {
  setup
  run "$VERIFY" "" --env dev --image "$IMAGE"
  check "verify succeeds" '[[ $RC -eq 0 ]]'
  check "verify uses only the dev context" '[[ "$(sort -u "${WORK}/contexts")" == "lfx-v2-dev" ]]'
  check "verify args are read-only" \
    '[[ "$(manifest ".spec.template.spec.containers[0].args")" == "[\"sync\",\"verify-global-grants\",\"--concurrency\",\"20\"]" ]]'
  check "verify manifest never contains --update" '! grep -q -- "--update" "${WORK}/manifest.json"'
  check "verify deadline is one hour" '[[ "$(manifest ".spec.activeDeadlineSeconds")" == "3600" ]]'
  check "verify manifest is hardened" 'hardened_manifest'
  check "verify env carries resolved values" \
    '[[ "$(manifest ".spec.template.spec.containers[0].env")" == "[{\"name\":\"NATS_URL\",\"value\":\"${FAKE_NATS}\"},{\"name\":\"OPENFGA_API_URL\",\"value\":\"http://lfx-platform-openfga:8080\"},{\"name\":\"OPENFGA_STORE_ID\",\"value\":\"${FAKE_STORE}\"},{\"name\":\"LOG_LEVEL\",\"value\":\"info\"}]" ]]'
  check "verify output hides store ID and NATS_URL" 'output_hides "$FAKE_STORE" && output_hides "$FAKE_NATS"'
  check "verify reads project-service in its namespace and heimdall in lfx" \
    'grep -qE "get deployments -n project-service -l app.kubernetes.io/name=lfx-v2-project-service" "${WORK}/calls" && grep -qE "get deployments -n lfx -l app.kubernetes.io/name=heimdall" "${WORK}/calls"'
  teardown
}

test_verify_store_id_failures() {
  local ids
  for ids in "" "not-a-ulid" "01ARZ3NDEKTSV4RRFFQ69G5FAU0" "01arz3ndektsv4rrffq69g5fav" $'01ARZ3NDEKTSV4RRFFQ69G5FAV\n01ARZ3NDEKTSV4RRFFQ69G5FAW'; do
    setup
    FAKE_STORE_IDS="$ids" run "$VERIFY" "" --env prod --image "$IMAGE"
    check "verify refuses store ID '${ids//$'\n'/,}'" '[[ $RC -eq 1 ]] && no_job_created'
    teardown
  done
}

main() {
  test_backfill_rejects_bad_arguments
  test_backfill_dev_dry_run
  test_backfill_prod_dry_run_needs_no_tty
  test_backfill_dev_apply_with_confirmation
  test_backfill_documents_only
  test_backfill_apply_wrong_phrase
  test_backfill_prod_apply_requires_tty
  test_backfill_nats_url_resolution_failures
  test_backfill_env_value_is_not_injectable
  test_verify_rejects_bad_arguments
  test_verify_creates_read_only_job
  test_verify_store_id_failures
  echo "passed: ${PASSES} failed: ${FAILURES}"
  [[ $FAILURES -eq 0 ]]
}

main "$@"
