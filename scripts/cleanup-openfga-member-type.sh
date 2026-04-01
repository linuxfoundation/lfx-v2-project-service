#!/usr/bin/env bash
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
#
# cleanup-openfga-member-type.sh
#
# Deletes all tuples associated with the now-removed `member` OpenFGA type:
#   1. All `team:membership-auditors#member -> auditor -> member:<uuid>` tuples
#   2. All `user:* -> member -> team:membership-auditors` tuples
#
# This is the inverse of scripts/create-membership-auditors-team.sh, which is
# also being removed as part of this cleanup.
#
# Prerequisites:
#   - kubectl port-forward to the target OpenFGA on localhost:8080
#   - jq installed
#
# Usage:
#   ./scripts/cleanup-openfga-member-type.sh [--dry-run]

set -euo pipefail

BASE_URL="${OPENFGA_URL:-http://localhost:8080}"
STORE_ID="${OPENFGA_STORE_ID:-01K3S60BS505DDR3VF9RAZDVHG}"
BATCH_SIZE=100
DRY_RUN=false

if [[ "${1:-}" == "--dry-run" ]]; then
	DRY_RUN=true
	echo "=== DRY RUN MODE — no deletions will be performed ==="
fi

echo "Store:    $STORE_ID"
echo "Base URL: $BASE_URL"
echo ""

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# Send a batch delete to /write; on dry-run just print the tuples
delete_batch() {
	local tuples_json="$1"
	local count
	count=$(echo "$tuples_json" | jq 'length')

	if [[ "$DRY_RUN" == true ]]; then
		echo "[DRY RUN] Would delete $count tuples:"
		echo "$tuples_json" | jq -r '.[] | "  \(.user) -> \(.relation) -> \(.object)"'
		return
	fi

	local payload
	payload=$(jq -n --argjson keys "$tuples_json" '{"deletes": {"tuple_keys": $keys}}')

	local resp
	resp=$(curl -s -X POST "${BASE_URL}/stores/${STORE_ID}/write" \
		-H 'Content-Type: application/json' \
		-d "$payload")

	# OpenFGA returns an empty body on success, or a JSON error object on failure
	if echo "$resp" | jq -e '.code' >/dev/null 2>&1; then
		echo "ERROR deleting batch: $(echo "$resp" | jq -r '.message')"
		echo "Failing batch:"
		echo "$tuples_json" | jq .
		exit 1
	fi

	echo "  Deleted $count tuples"
}

# Paginate through /read using the given tuple_key JSON fragment, collecting all
# results into the global variable COLLECTED_TUPLES as a JSON array of
# {user, relation, object} objects.
collect_tuples() {
	local filter_json="$1"
	COLLECTED_TUPLES="[]"
	local token=""
	local page=0

	while true; do
		local body=""
		if [[ -z "$token" ]]; then
			body=$(jq -n --argjson tk "$filter_json" '{"tuple_key": $tk, "page_size": 100}')
		else
			body=$(jq -n --argjson tk "$filter_json" --arg ct "$token" \
				'{"tuple_key": $tk, "page_size": 100, "continuation_token": $ct}')
		fi

		local resp
		resp=$(curl -s -X POST "${BASE_URL}/stores/${STORE_ID}/read" \
			-H 'Content-Type: application/json' \
			-d "$body")

		if echo "$resp" | jq -e '.code' >/dev/null 2>&1; then
			echo "ERROR reading tuples: $(echo "$resp" | jq -r '.message')"
			exit 1
		fi

		local batch
		batch=$(echo "$resp" | jq '[.tuples[] | {user: .key.user, relation: .key.relation, object: .key.object}]')
		local batch_count
		batch_count=$(echo "$batch" | jq 'length')
		page=$((page + 1))

		COLLECTED_TUPLES=$(printf '%s\n%s' "$COLLECTED_TUPLES" "$batch" | jq -s 'add')
		echo "  Page $page: $batch_count tuples (running total: $(echo "$COLLECTED_TUPLES" | jq 'length'))"

		token=$(echo "$resp" | jq -r '.continuation_token // ""')
		[[ -z "$token" ]] && break
	done
}

# Flush COLLECTED_TUPLES to the API in batches of BATCH_SIZE
delete_all_collected() {
	local total
	total=$(echo "$COLLECTED_TUPLES" | jq 'length')
	echo "  Total to delete: $total"
	[[ "$total" -eq 0 ]] && return

	local offset=0
	while [[ $offset -lt $total ]]; do
		local batch
		batch=$(echo "$COLLECTED_TUPLES" | jq --argjson o "$offset" --argjson s "$BATCH_SIZE" '.[$o:$o+$s]')
		delete_batch "$batch"
		offset=$((offset + BATCH_SIZE))
	done
}

# ---------------------------------------------------------------------------
# Step 1: Delete all member:* object tuples
#   team:membership-auditors#member -> auditor -> member:<uuid>
#
# The /read API requires either a full object ID or both a user and an object
# type prefix. We use user + object-type-prefix to page through all of them.
# ---------------------------------------------------------------------------

echo "=== Step 1: Collecting all member:* object tuples ==="
collect_tuples '{"user": "team:membership-auditors#member", "object": "member:"}'

echo ""
echo "=== Step 1: Deleting ==="
delete_all_collected

echo ""
echo "=== Step 1 complete ==="
echo ""

# ---------------------------------------------------------------------------
# Step 2: Delete all user -> member -> team:membership-auditors tuples
# ---------------------------------------------------------------------------

echo "=== Step 2: Collecting user memberships of team:membership-auditors ==="
collect_tuples '{"object": "team:membership-auditors", "relation": "member"}'

echo ""
echo "  Users found:"
echo "$COLLECTED_TUPLES" | jq -r '.[] | "  \(.user)"'

echo ""
echo "=== Step 2: Deleting ==="
delete_all_collected

echo ""
echo "=== Step 2 complete ==="
echo ""

echo "=== All done ==="
