#!/usr/bin/env bash
# install-agent-skills.sh — Install ai-workflow skills into agent HOME directories.
#
# Usage:
#   bash scripts/setup/install-agent-skills.sh
#
# Installs to:
#   Claude:  $CLAUDE_CONFIG_DIR/CLAUDE.md  (default ~/.claude/CLAUDE.md)
#   Codex:   $CODEX_HOME/AGENTS.md         (default ~/.codex/AGENTS.md)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SKILL_SOURCE="$PROJECT_ROOT/configs/skills/ai-workflow-config.md"
SCHEMA_SOURCE="$PROJECT_ROOT/configs/config-schema.json"

MARKER_BEGIN="<!-- BEGIN:ai-workflow-config-skill -->"
MARKER_END="<!-- END:ai-workflow-config-skill -->"

# ── Helper ────────────────────────────────────────────────────────
inject_skill() {
    local target="$1"
    local dir
    dir="$(dirname "$target")"
    mkdir -p "$dir"

    # Remove old marker block if present.
    if [ -f "$target" ] && grep -qF "$MARKER_BEGIN" "$target"; then
        # Delete lines between markers (inclusive).
        local tmp
        tmp="$(mktemp)"
        awk "/$MARKER_BEGIN/{skip=1; next} /$MARKER_END/{skip=0; next} !skip" "$target" > "$tmp"
        mv "$tmp" "$target"
        echo "  removed old skill block from $target"
    fi

    # Append new block.
    {
        echo ""
        echo "$MARKER_BEGIN"
        cat "$SKILL_SOURCE"
        echo ""
        echo "$MARKER_END"
    } >> "$target"

    echo "  ✓ installed skill to $target"
}

# ── Claude ────────────────────────────────────────────────────────
echo "=== Claude ACP ==="
# Official env: CLAUDE_CONFIG_DIR (default ~/.claude)
CLAUDE_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
CLAUDE_TARGET="$CLAUDE_DIR/CLAUDE.md"
inject_skill "$CLAUDE_TARGET"

# ── Codex ─────────────────────────────────────────────────────────
echo "=== Codex ACP ==="
CODEX_TARGET="${CODEX_HOME:-$HOME/.codex}/AGENTS.md"
inject_skill "$CODEX_TARGET"

# ── Schema ────────────────────────────────────────────────────────
echo ""
echo "=== Schema ==="
if [ -f "$SCHEMA_SOURCE" ]; then
    echo "  schema available at: $SCHEMA_SOURCE"
else
    echo "  generating schema..."
    (cd "$PROJECT_ROOT" && go run ./cmd/gen-schema > "$SCHEMA_SOURCE")
    echo "  ✓ generated $SCHEMA_SOURCE"
fi

echo ""
echo "Done. Skills installed for Claude and Codex."
