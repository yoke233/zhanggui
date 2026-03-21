#!/usr/bin/env bash
# list-actions.sh — List all actions for a work item.
#
# Usage:
#   ./list-actions.sh <work-item-id>
#
# Environment:
#   AI_WORKFLOW_SERVER_ADDR, AI_WORKFLOW_API_TOKEN

set -euo pipefail

WORK_ITEM_ID="${1:?Usage: list-actions.sh <work-item-id>}"

SERVER="${AI_WORKFLOW_SERVER_ADDR:?AI_WORKFLOW_SERVER_ADDR is required}"
TOKEN="${AI_WORKFLOW_API_TOKEN:-}"

AUTH_HEADER=""
if [ -n "$TOKEN" ]; then
  AUTH_HEADER="Authorization: Bearer ${TOKEN}"
fi

RESPONSE=$(curl -sf -X GET \
  "${SERVER}/api/work-items/${WORK_ITEM_ID}/actions" \
  -H "Content-Type: application/json" \
  ${AUTH_HEADER:+-H "$AUTH_HEADER"} 2>&1) || {
  echo "Error listing actions: ${RESPONSE}" >&2
  exit 1
}

echo "$RESPONSE"
