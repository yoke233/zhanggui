#!/usr/bin/env bash
# signal.sh — Signal step decision to the AI Workflow engine.
#
# Usage:
#   ./signal.sh <decision> <reason> [metadata_json]
#
# Decisions: complete | need_help | approve | reject
#
# Reads from environment (injected by the engine):
#   AI_WORKFLOW_SERVER_ADDR, AI_WORKFLOW_STEP_ID, AI_WORKFLOW_API_TOKEN
#
# If the HTTP call fails (e.g. no network), falls back to printing
# AI_WORKFLOW_SIGNAL: line so the engine can parse it from output.

set -euo pipefail

DECISION="${1:?Usage: signal.sh <decision> <reason> [metadata_json]}"
REASON="${2:?Usage: signal.sh <decision> <reason> [metadata_json]}"
METADATA_JSON="${3:-}"

# Validate decision.
case "$DECISION" in
  complete|need_help|approve|reject) ;;
  *) echo "Error: decision must be one of: complete, need_help, approve, reject" >&2; exit 1 ;;
esac

PAYLOAD="{\"decision\":\"${DECISION}\",\"reason\":\"${REASON}\"}"
if [ -n "${METADATA_JSON}" ]; then
  case "${METADATA_JSON}" in
    \{*\})
      EXTRA_JSON="${METADATA_JSON#\{}"
      EXTRA_JSON="${EXTRA_JSON%\}}"
      if [ -n "${EXTRA_JSON}" ]; then
        PAYLOAD="{\"decision\":\"${DECISION}\",\"reason\":\"${REASON}\",${EXTRA_JSON}}"
      fi
      ;;
    *)
      echo "Error: metadata_json must be a compact JSON object" >&2
      exit 1
      ;;
  esac
fi

# Try HTTP first.
if [ -n "${AI_WORKFLOW_SERVER_ADDR:-}" ] && [ -n "${AI_WORKFLOW_STEP_ID:-}" ] && [ -n "${AI_WORKFLOW_API_TOKEN:-}" ]; then
  HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    "${AI_WORKFLOW_SERVER_ADDR}/api/steps/${AI_WORKFLOW_STEP_ID}/decision" \
    -H "Authorization: Bearer ${AI_WORKFLOW_API_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$PAYLOAD" 2>/dev/null || echo "000")

  if [ "$HTTP_CODE" -ge 200 ] && [ "$HTTP_CODE" -lt 300 ]; then
    echo "Signal sent via HTTP (${HTTP_CODE}): ${DECISION}"
    exit 0
  fi
  echo "HTTP signal failed (${HTTP_CODE}), falling back to output." >&2
fi

# Fallback: output the signal line for engine to parse.
echo "AI_WORKFLOW_SIGNAL: ${PAYLOAD}"
