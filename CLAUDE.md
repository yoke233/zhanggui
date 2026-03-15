# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run Commands

### Backend (Go)
```bash
# Start server
go run ./cmd/ai-flow server --port 8080

# Run backend unit / integration / e2e suites
pwsh -NoProfile -File ./scripts/test/backend-unit.ps1
pwsh -NoProfile -File ./scripts/test/backend-integration.ps1
pwsh -NoProfile -File ./scripts/test/backend-e2e.ps1
pwsh -NoProfile -File ./scripts/test/backend-real.ps1

# Run a single Go test package
go test ./internal/engine/...
go test ./internal/core/... -run TestSpecificFunction

# Build binary
go build -o ai-flow ./cmd/ai-flow
```

### Frontend (React/Vite/TypeScript in `web/`)
```bash
# Install dependencies
npm --prefix web install

# Dev server (port 5173)
npm --prefix web run dev -- --strictPort

# Run unit tests
npm --prefix web run test

# Type-check only
npm --prefix web run typecheck

# Production build
npm --prefix web run build
```

### Integration / Smoke Tests
```bash
pwsh -NoProfile -File ./scripts/test/suite-p3.ps1         # P3 regression suite
pwsh -NoProfile -File ./scripts/test/frontend-build.ps1   # Frontend build verification
```

## Architecture

### Domain Model (`internal/core/`)
Current execution model centers on runtime drivers and profiles:
- **Flow / Step / Execution** — primary orchestration entities.
- **Project / ResourceBinding / Artifact / Briefing** — project-scoped runtime resources.
- **Runtime Profile** — execution persona, configured under `runtime.agents.profiles`.

### Package Dependency Flow
```
cmd/ai-flow (CLI + server bootstrap)
  → internal/backend     (HTTP server/bootstrap)
  → internal/engine      (runtime execution)
  → internal/core        (domain types)
  → internal/support/... (config, ACP client, shared support packages)
  → internal/store       (persistence)
  → internal/skills      (runtime skills)
```

### Execution Path
Pipeline execution uses ACP over stdio:
- runtime agent launch comes from `runtime.agents.drivers`
- runtime role/persona selection comes from `runtime.agents.profiles`
- backend bootstraps runtime services from `internal/backend/*`

### Frontend (`web/`)
React 18 + Vite + Tailwind + Zustand. Embedded into Go binary via `web/embed.go`.

Current frontend active path:
- `src/App.tsx`
- `src/pages/`
- `src/layouts/AppLayout.tsx`
- `src/lib/apiClient.ts`
- `src/lib/wsClient.ts`
- legacy UI snapshots are under `web/archive-src/legacy-ui/`

Key layers:
- **State**: Zustand stores in `src/stores/`
- **API clients**: `src/lib/apiClient.ts`, `src/lib/wsClient.ts`
- **Types**: `src/types/` mirrors backend contract
- **UI primitives**: `src/components/ui/` — button, badge, card, dialog, input, select, table, textarea, separator
- **Path alias**: `@` → `./src` (configured in vite.config.ts and tsconfig)
- **Dev proxy**: `/api` → Go backend (default `http://127.0.0.1:8080`, override with `VITE_API_PROXY_TARGET`)

### Configuration
- Project-local config: `.ai-workflow/config.toml`
- Default config: `internal/support/config/defaults.toml`
- Hierarchy merge: project-local overrides defaults; `internal/support/config/` handles merging.

## Code Conventions

- **Go**: `gofmt` required. Package names lowercase. Files use `snake_case`. Domain types in `internal/core`, then extend outward.
- **TypeScript/React**: 2-space indent, double quotes, semicolons. Functional components with Tailwind.
- **Commits**: Conventional Commits format — `feat(scope):`, `fix:`, `test(scope):`, `chore:`.
- **New API/event/model**: define in `internal/core` first, then propagate to `engine` → `web` → `plugins`. Update `web/src/types` to keep frontend contract in sync.

## Key Environment Variables

- `AI_WORKFLOW_DB_PATH` — SQLite database path (required for `mcp-serve`)
- `AI_WORKFLOW_CHAT_PROVIDER` — chat backend selection
- `VITE_API_TOKEN` — frontend API token (dev only)
- `VITE_API_PROXY_TARGET` — dev proxy target for `/api` (default `http://127.0.0.1:8080`)
