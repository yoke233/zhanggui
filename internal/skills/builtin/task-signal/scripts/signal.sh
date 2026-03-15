#!/usr/bin/env bash
# signal.sh — Signal ThreadTask completion or rejection to the AI Workflow engine.
#
# Usage:
#   ./signal.sh <action> <output_file> [feedback]
#
# Actions: complete | reject
#
# Reads from environment (injected by the engine):
#   AI_WORKFLOW_SERVER_ADDR, AI_WORKFLOW_TASK_ID, AI_WORKFLOW_API_TOKEN
#
# If the HTTP call fails, falls back to printing
# AI_WORKFLOW_TASK_SIGNAL: line so the engine can parse it from output.

set -euo pipefail

ACTION="${1:?Usage: signal.sh <action> <output_file> [feedback]}"
OUTPUT_FILE="${2:?Usage: signal.sh <action> <output_file> [feedback]}"
FEEDBACK="${3:-}"

# Validate action.
case "$ACTION" in
  complete|reject) ;;
  *) echo "Error: action must be one of: complete, reject" >&2; exit 1 ;;
esac

PAYLOAD="{\"action\":\"${ACTION}\",\"output_file_path\":\"${OUTPUT_FILE}\",\"feedback\":\"${FEEDBACK}\"}"

# Try HTTP first.
if [ -n "${AI_WORKFLOW_SERVER_ADDR:-}" ] && [ -n "${AI_WORKFLOW_TASK_ID:-}" ] && [ -n "${AI_WORKFLOW_API_TOKEN:-}" ]; then
  HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    "${AI_WORKFLOW_SERVER_ADDR}/api/v1/thread-tasks/${AI_WORKFLOW_TASK_ID}/signal" \
    -H "Authorization: Bearer ${AI_WORKFLOW_API_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$PAYLOAD" 2>/dev/null || echo "000")

  if [ "$HTTP_CODE" -ge 200 ] && [ "$HTTP_CODE" -lt 300 ]; then
    echo "Signal sent via HTTP (${HTTP_CODE}): ${ACTION}"
    exit 0
  fi
  echo "HTTP signal failed (${HTTP_CODE}), falling back to output." >&2
fi

# Fallback: output the signal line for engine to parse.
echo "AI_WORKFLOW_TASK_SIGNAL: ${PAYLOAD}"
