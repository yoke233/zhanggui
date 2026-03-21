#!/usr/bin/env bash
# get-action.sh — Get details of a specific action.
#
# Usage:
#   ./get-action.sh <action-id>
#
# Environment:
#   AI_WORKFLOW_SERVER_ADDR, AI_WORKFLOW_API_TOKEN

set -euo pipefail

ACTION_ID="${1:?Usage: get-action.sh <action-id>}"

SERVER="${AI_WORKFLOW_SERVER_ADDR:?AI_WORKFLOW_SERVER_ADDR is required}"
TOKEN="${AI_WORKFLOW_API_TOKEN:-}"

AUTH_HEADER=""
if [ -n "$TOKEN" ]; then
  AUTH_HEADER="Authorization: Bearer ${TOKEN}"
fi

RESPONSE=$(curl -sf -X GET \
  "${SERVER}/api/actions/${ACTION_ID}" \
  -H "Content-Type: application/json" \
  ${AUTH_HEADER:+-H "$AUTH_HEADER"} 2>&1) || {
  echo "Error getting action: ${RESPONSE}" >&2
  exit 1
}

echo "$RESPONSE"
