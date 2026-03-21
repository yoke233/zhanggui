#!/usr/bin/env bash
# delete-action.sh — Delete a pending action.
#
# Usage:
#   ./delete-action.sh <action-id>
#
# Only pending actions can be deleted.
#
# Environment:
#   AI_WORKFLOW_SERVER_ADDR, AI_WORKFLOW_API_TOKEN

set -euo pipefail

ACTION_ID="${1:?Usage: delete-action.sh <action-id>}"

SERVER="${AI_WORKFLOW_SERVER_ADDR:?AI_WORKFLOW_SERVER_ADDR is required}"
TOKEN="${AI_WORKFLOW_API_TOKEN:-}"

AUTH_HEADER=""
if [ -n "$TOKEN" ]; then
  AUTH_HEADER="Authorization: Bearer ${TOKEN}"
fi

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
  "${SERVER}/api/actions/${ACTION_ID}" \
  -H "Content-Type: application/json" \
  ${AUTH_HEADER:+-H "$AUTH_HEADER"} 2>/dev/null || echo "000")

if [ "$HTTP_CODE" -ge 200 ] && [ "$HTTP_CODE" -lt 300 ]; then
  echo "{\"deleted\":true,\"action_id\":${ACTION_ID}}"
else
  echo "Error deleting action: HTTP ${HTTP_CODE}" >&2
  exit 1
fi
