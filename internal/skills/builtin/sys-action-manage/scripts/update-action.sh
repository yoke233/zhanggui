#!/usr/bin/env bash
# update-action.sh — Update a pending action.
#
# Usage:
#   ./update-action.sh <action-id> '<json-payload>'
#
# Only pending actions can be edited.
#
# Environment:
#   AI_WORKFLOW_SERVER_ADDR, AI_WORKFLOW_API_TOKEN

set -euo pipefail

ACTION_ID="${1:?Usage: update-action.sh <action-id> '<json>'}"
PAYLOAD="${2:?Usage: update-action.sh <action-id> '<json>'}"

SERVER="${AI_WORKFLOW_SERVER_ADDR:?AI_WORKFLOW_SERVER_ADDR is required}"
TOKEN="${AI_WORKFLOW_API_TOKEN:-}"

AUTH_HEADER=""
if [ -n "$TOKEN" ]; then
  AUTH_HEADER="Authorization: Bearer ${TOKEN}"
fi

RESPONSE=$(curl -sf -X PUT \
  "${SERVER}/api/actions/${ACTION_ID}" \
  -H "Content-Type: application/json" \
  ${AUTH_HEADER:+-H "$AUTH_HEADER"} \
  -d "$PAYLOAD" 2>&1) || {
  echo "Error updating action: ${RESPONSE}" >&2
  exit 1
}

echo "$RESPONSE"
