#!/usr/bin/env bash
# create-action.sh — Create a new action for a work item.
#
# Usage:
#   ./create-action.sh <work-item-id> '<json-payload>'
#
# Environment:
#   AI_WORKFLOW_SERVER_ADDR, AI_WORKFLOW_API_TOKEN

set -euo pipefail

WORK_ITEM_ID="${1:?Usage: create-action.sh <work-item-id> '<json>'}"
PAYLOAD="${2:?Usage: create-action.sh <work-item-id> '<json>'}"

SERVER="${AI_WORKFLOW_SERVER_ADDR:?AI_WORKFLOW_SERVER_ADDR is required}"
TOKEN="${AI_WORKFLOW_API_TOKEN:-}"

AUTH_HEADER=""
if [ -n "$TOKEN" ]; then
  AUTH_HEADER="Authorization: Bearer ${TOKEN}"
fi

RESPONSE=$(curl -sf -X POST \
  "${SERVER}/api/work-items/${WORK_ITEM_ID}/actions" \
  -H "Content-Type: application/json" \
  ${AUTH_HEADER:+-H "$AUTH_HEADER"} \
  -d "$PAYLOAD" 2>&1) || {
  echo "Error creating action: ${RESPONSE}" >&2
  exit 1
}

echo "$RESPONSE"
