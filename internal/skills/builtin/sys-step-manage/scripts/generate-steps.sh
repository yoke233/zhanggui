#!/usr/bin/env bash
# generate-steps.sh — AI auto-decompose a task description into steps.
#
# Usage:
#   ./generate-steps.sh <work-item-id> '<description>'
#
# The backend uses the plan-actions planning service to generate a DAG
# and materializes the steps into the work item.
#
# Environment:
#   AI_WORKFLOW_SERVER_ADDR, AI_WORKFLOW_API_TOKEN

set -euo pipefail

WORK_ITEM_ID="${1:?Usage: generate-steps.sh <work-item-id> '<description>'}"
DESCRIPTION="${2:?Usage: generate-steps.sh <work-item-id> '<description>'}"

SERVER="${AI_WORKFLOW_SERVER_ADDR:?AI_WORKFLOW_SERVER_ADDR is required}"
TOKEN="${AI_WORKFLOW_API_TOKEN:-}"

AUTH_HEADER=""
if [ -n "$TOKEN" ]; then
  AUTH_HEADER="Authorization: Bearer ${TOKEN}"
fi

# Build JSON payload — escape description for safe embedding.
PAYLOAD=$(printf '{"description":"%s"}' "$(echo "$DESCRIPTION" | sed 's/\\/\\\\/g; s/"/\\"/g; s/\n/\\n/g')")

RESPONSE=$(curl -sf -X POST \
  "${SERVER}/api/work-items/${WORK_ITEM_ID}/generate-steps" \
  -H "Content-Type: application/json" \
  ${AUTH_HEADER:+-H "$AUTH_HEADER"} \
  -d "$PAYLOAD" 2>&1) || {
  echo "Error generating steps: ${RESPONSE}" >&2
  exit 1
}

echo "$RESPONSE"
