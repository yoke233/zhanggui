#!/usr/bin/env bash
# dev.sh — 一键启动 ai-workflow 开发环境 (后端 + 前端)
#
# 用法:
#   bash scripts/dev.sh              # 启动后端 + 前端 dev server
#   bash scripts/dev.sh --backend    # 仅后端
#   bash scripts/dev.sh --frontend   # 仅前端
#   bash scripts/dev.sh --build      # 构建二进制 (含前端嵌入) 后启动
#
# 环境变量:
#   PORT          后端端口 (默认 8080)
#   FRONTEND_PORT 前端端口 (默认 5173)
#   BUILD_TAGS    Go build tags (默认 dev)

set -euo pipefail

cd "$(dirname "$0")/.."

PORT="${PORT:-8080}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"
BUILD_TAGS="${BUILD_TAGS:-dev}"

MODE="all"
USE_BUILD=false
for arg in "$@"; do
    case "$arg" in
        --backend)  MODE="backend" ;;
        --frontend) MODE="frontend" ;;
        --build)    USE_BUILD=true ;;
        *) echo "Unknown arg: $arg"; exit 1 ;;
    esac
done

cleanup() {
    echo ""
    echo "==> Stopping..."
    kill $BACKEND_PID 2>/dev/null || true
    kill $FRONTEND_PID 2>/dev/null || true
    wait 2>/dev/null || true
    echo "==> Done."
}

BACKEND_PID=0
FRONTEND_PID=0

# ── 依赖检查 ──
check_deps() {
    if ! command -v go &>/dev/null; then
        echo "ERROR: go not found. Install Go 1.25+" >&2
        exit 1
    fi
    if [ "$MODE" != "backend" ]; then
        if ! command -v node &>/dev/null; then
            echo "ERROR: node not found. Install Node 22+" >&2
            exit 1
        fi
        if [ ! -d web/node_modules ]; then
            echo "==> Installing frontend dependencies..."
            npm --prefix web ci
        fi
    fi
}

# ── 后端 ──
start_backend() {
    if $USE_BUILD; then
        echo "==> Building binary (tags: $BUILD_TAGS)..."
        go build -tags "$BUILD_TAGS" -o ./ai-flow ./cmd/ai-flow
        echo "==> Starting ai-flow binary on :$PORT"
        ./ai-flow server --port "$PORT" &
        BACKEND_PID=$!
    else
        echo "==> Starting backend (go run, tags: $BUILD_TAGS) on :$PORT"
        go run -tags "$BUILD_TAGS" ./cmd/ai-flow server --port "$PORT" &
        BACKEND_PID=$!
    fi
}

# ── 前端 ──
start_frontend() {
    echo "==> Starting frontend dev server on :$FRONTEND_PORT"
    VITE_API_BASE_URL="http://127.0.0.1:$PORT/api/v1" \
        npm --prefix web run dev -- --port "$FRONTEND_PORT" --strictPort &
    FRONTEND_PID=$!
}

# ── Main ──
check_deps
trap cleanup EXIT INT TERM

case "$MODE" in
    backend)
        start_backend
        wait $BACKEND_PID
        ;;
    frontend)
        start_frontend
        wait $FRONTEND_PID
        ;;
    all)
        start_backend
        start_frontend
        echo ""
        echo "==> Backend:  http://127.0.0.1:$PORT"
        echo "==> Frontend: http://127.0.0.1:$FRONTEND_PORT"
        echo "==> Press Ctrl+C to stop"
        echo ""
        wait
        ;;
esac
