#!/usr/bin/env bash
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
#
# setup-global-org-admin-team.sh
#
# Seeds (or updates) the global org-admin team membership tuples in OpenFGA.
# Each user in ADMIN_USERS gets a `member` relation on `team:<TEAM_UID>`,
# which transitively grants `writer` and `auditor` access to every b2b_org.
#
# Prerequisites:
#   - kubectl port-forward to the target OpenFGA on localhost:8080, OR
#     run from inside the cluster (lfx-platform-nats-box has curl + jq)
#   - jq installed
#
# Required env vars:
#   OPENFGA_STORE_ID     — OpenFGA store ID for the target environment
#   TEAM_UID             — the globalOrgAdminTeamUID for the environment
#   ADMIN_USERS          — comma-separated LFID usernames (e.g. "username1,username2")
#
# Optional env vars:
#   OPENFGA_URL          — default: http://localhost:8080
#
# Usage:
#   # Port-forward first (local):
#   kubectl -n lfx port-forward svc/lfx-platform-openfga 8080:8080
#
#   OPENFGA_STORE_ID=01K1XF6SXV7JY5HZ25EZGCDNXE \
#   TEAM_UID=f90e4098-7e43-470e-ba37-944bf0575a42 \
#   ADMIN_USERS=jmedev,asitha,andrest50dev,audiyoung \
#   ./scripts/setup-global-org-admin-team.sh [--dry-run]
#

set -euo pipefail

BASE_URL="${OPENFGA_URL:-http://localhost:8080}"
STORE_ID="${OPENFGA_STORE_ID:?OPENFGA_STORE_ID must be set}"
TEAM_UID="${TEAM_UID:?TEAM_UID must be set}"
ADMIN_USERS="${ADMIN_USERS:?ADMIN_USERS must be set (comma-separated LFID usernames)}"
DRY_RUN=false

if [[ "${1:-}" == "--dry-run" ]]; then
	DRY_RUN=true
	echo "=== DRY RUN MODE — no writes will be performed ==="
fi

echo "Store:    $STORE_ID"
echo "Team UID: $TEAM_UID"
echo "Base URL: $BASE_URL"
echo ""

TEAM_OBJECT="team:${TEAM_UID}"

# ---------------------------------------------------------------------------
# Step 1: Read existing members so we can report what already exists
# ---------------------------------------------------------------------------

echo "=== Step 1: Reading existing team members ==="

existing_resp=$(curl -s -X POST "${BASE_URL}/stores/${STORE_ID}/read" \
	-H 'Content-Type: application/json' \
	-d "$(jq -n --arg obj "$TEAM_OBJECT" '{"tuple_key":{"object":$obj,"relation":"member"},"page_size":100}')")

if echo "$existing_resp" | jq -e '.code' >/dev/null 2>&1; then
	echo "ERROR reading existing tuples: $(echo "$existing_resp" | jq -r '.message')"
	exit 1
fi

existing_users=$(echo "$existing_resp" | jq -r '[.tuples[]?.key.user] | .[]' 2>/dev/null || true)
if [[ -n "$existing_users" ]]; then
	echo "Existing members:"
	echo "$existing_users" | sed 's/^/  /'
else
	echo "  (none)"
fi
echo ""

# ---------------------------------------------------------------------------
# Step 2: Determine which users need to be written
# ---------------------------------------------------------------------------

echo "=== Step 2: Computing writes ==="

IFS=',' read -ra users <<< "$ADMIN_USERS"

tuples_to_write="[]"
for raw_user in "${users[@]}"; do
	username=$(echo "$raw_user" | tr -d '[:space:]')
	[[ -z "$username" ]] && continue
	fga_user="user:${username}"

	if echo "$existing_users" | grep -qx "$fga_user"; then
		echo "  SKIP (already member): $fga_user"
	else
		echo "  WRITE: $fga_user -> member -> $TEAM_OBJECT"
		tuples_to_write=$(echo "$tuples_to_write" | jq \
			--arg u "$fga_user" --arg r "member" --arg o "$TEAM_OBJECT" \
			'. + [{"user":$u,"relation":$r,"object":$o}]')
	fi
done
echo ""

write_count=$(echo "$tuples_to_write" | jq 'length')
if [[ "$write_count" -eq 0 ]]; then
	echo "All users are already members — nothing to do."
	exit 0
fi

# ---------------------------------------------------------------------------
# Step 3: Write new member tuples
# ---------------------------------------------------------------------------

echo "=== Step 3: Writing $write_count tuple(s) ==="

if [[ "$DRY_RUN" == true ]]; then
	echo "[DRY RUN] Would write:"
	echo "$tuples_to_write" | jq -r '.[] | "  \(.user) -> \(.relation) -> \(.object)"'
	exit 0
fi

payload=$(jq -n --argjson keys "$tuples_to_write" '{"writes":{"tuple_keys":$keys}}')
resp=$(curl -s -X POST "${BASE_URL}/stores/${STORE_ID}/write" \
	-H 'Content-Type: application/json' \
	-d "$payload")

if echo "$resp" | jq -e '.code' >/dev/null 2>&1; then
	echo "ERROR writing tuples: $(echo "$resp" | jq -r '.message')"
	exit 1
fi

echo "  Done — $write_count tuple(s) written."
echo ""

# ---------------------------------------------------------------------------
# Step 4: Verify
# ---------------------------------------------------------------------------

echo "=== Step 4: Verifying ==="

verify_resp=$(curl -s -X POST "${BASE_URL}/stores/${STORE_ID}/read" \
	-H 'Content-Type: application/json' \
	-d "$(jq -n --arg obj "$TEAM_OBJECT" '{"tuple_key":{"object":$obj,"relation":"member"},"page_size":100}')")

echo "Current members of $TEAM_OBJECT:"
echo "$verify_resp" | jq -r '.tuples[]?.key.user' | sed 's/^/  /'
echo ""
echo "=== Done ==="
