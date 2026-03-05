# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run Commands

### Backend (Go)
```bash
# Start server
go run ./cmd/ai-flow server --port 8080

# Run all backend tests
pwsh -NoProfile -File ./scripts/test/backend-all.ps1

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
pwsh -NoProfile -File ./scripts/test/v2-smoke.ps1        # V2 API smoke
pwsh -NoProfile -File ./scripts/test/p3-integration.ps1   # Full integration
pwsh -NoProfile -File ./scripts/test/frontend-build.ps1   # Frontend build verification
```

## Architecture

### Domain Model (`internal/core/`)
Four core concepts drive the system:
- **Issue** — minimal deliverable unit with state machine (`draft → reviewing → queued → ready → executing → done/failed`), plus decomposition states (`decomposing → decomposed → superseded`).
- **Profile** — agent execution persona (role + capabilities). Configured in `configs/defaults.yaml` under `roles:`.
- **Run** — one execution instance (input = issue + profile, output = events + result). Status follows GitHub Actions model: `queued → in_progress → completed | action_required`.
- **Team Leader** — orchestration entry point: decomposes issues, selects profiles, manages runs and reviews.

### Package Dependency Flow
```
cmd/ai-flow (CLI + server bootstrap)
  → internal/web        (HTTP handlers, chi router, WebSocket, A2A endpoint)
  → internal/teamleader (Team Leader agent, issue manager, scheduler, A2A bridge)
  → internal/engine     (Run executor, ACP stage execution, review prompts)
  → internal/core       (Domain types, state machines, plugin interfaces)
  → internal/acpclient  (ACP protocol client, role resolver, permission policy)
  → internal/eventbus   (In-process pub/sub for domain events)
  → internal/github     (GitHub App integration, webhooks, PR lifecycle, status sync)
  → internal/plugins/   (Pluggable implementations, wired via factory)
  → internal/config     (YAML config loading with hierarchy merge)
```

### Plugin System (`internal/plugins/`)
Plugins implement `core.*` interfaces and are wired in `plugins/factory/`:
- **store-sqlite** — persistence (SQLite via modernc.org, migrations in-code)
- **workspace-worktree** / **workspace-clone** — git worktree or clone-based isolation
- **scm-local-git** / **scm-github** — source control operations
- **tracker-local** / **tracker-github** — issue tracking backends
- **review-ai-panel** / **review-github-pr** / **review-local** — review gate strategies
- **agent-claude** / **agent-codex** — ACP agent adapters (Claude, Codex)
- **notifier-desktop** — desktop notifications

### Execution Path
Pipeline execution uses ACP (Agent Communication Protocol) over stdio:
- `engine.Executor.runACPStage()` is the primary path — spawns an ACP agent process, sends prompts, streams events.
- `ACPHandlerFactory` interface defined in `engine/`, implemented in `cmd/ai-flow/commands.go` to break circular dependency.
- `acpclient.RoleResolver` maps stage roles to agent configs from `configs/defaults.yaml`.
- `stageEventBridge` converts ACP session updates into `core.Event` published on `eventbus.Bus`.

### Frontend (`web/`)
React + Vite + Tailwind + Zustand. Embedded into Go binary via `web/embed.go`.
- Views: `ChatView`, `A2AChatView`, `BoardView`, `RunView` (in `src/views/`)
- State: Zustand stores in `src/stores/` (projects, runs, chat)
- API clients: `src/lib/apiClient.ts` (REST), `src/lib/wsClient.ts` (WebSocket), `src/lib/a2aClient.ts` (A2A)
- Types mirror backend in `src/types/` (api, workflow, ws, a2a)

### Configuration
- Project-local config: `.ai-workflow/config.yaml` (generated via `ai-flow config init`)
- Default config: `configs/defaults.yaml` (agent definitions, role profiles, prompt templates)
- Hierarchy merge: project-local overrides defaults; `internal/config/` handles merging.

## Code Conventions

- **Go**: `gofmt` required. Package names lowercase. Files use `snake_case`. Domain types in `internal/core`, then extend outward.
- **TypeScript/React**: 2-space indent, double quotes, semicolons. Functional components with Tailwind.
- **Commits**: Conventional Commits format — `feat(scope):`, `fix:`, `test(scope):`, `chore:`.
- **New API/event/model**: define in `internal/core` first, then propagate to `engine` → `web` → `plugins`. Update `web/src/types` to keep frontend contract in sync.

## Key Environment Variables

- `AI_WORKFLOW_DB_PATH` — SQLite database path (required for `mcp-serve`)
- `AI_WORKFLOW_CHAT_PROVIDER` — chat backend selection
- `VITE_API_TOKEN` — frontend API token (dev only)
