# AI Workflow

**An intelligent orchestration platform for AI agent pipelines — plan, execute, and monitor multi-agent work from a single dashboard.**

AI Workflow turns your requirements into structured execution pipelines. You describe what needs to be done as Work Items; the system breaks them into Actions (a DAG of steps), dispatches each Action to the right AI agent, and tracks every Run to completion — with built-in gates, retries, and human intervention points.

## Key Features

- **Work Item Management** — Create, prioritize, and track units of work through their full lifecycle (open → accepted → queued → running → done).
- **DAG-based Execution** — Actions within a Work Item form a dependency graph. Independent actions run in parallel; gates enforce quality checkpoints.
- **Multi-Agent Runtime** — Configure multiple AI agent drivers (Claude, Codex, etc.) with distinct capability profiles. The scheduler matches each action to the best-fit agent.
- **Live Monitoring** — Real-time dashboard with analytics, usage tracking, scheduled inspections, and a unified activity journal for full audit trails.
- **Project Organization** — Group Work Items under Projects, bind them to Git repositories, and manage resources per project.
- **Conversational Threads** — AI-human chat threads linked to Work Items for context-rich collaboration.
- **Desktop & Web** — Web console served from the Go binary; optional Tauri wrapper for a native desktop experience.

## Architecture

```
┌─────────────────────────────────────────┐
│              Web Console                │
│         React · Vite · Tailwind         │
├─────────────────────────────────────────┤
│             REST / WebSocket            │
├─────────────────────────────────────────┤
│            Go Backend Server            │
│  ┌───────────┐ ┌──────────┐ ┌────────┐ │
│  │ Scheduler  │ │  Engine  │ │  Gate  │ │
│  │  (DAG)    │ │ (Actions)│ │Evaluator│ │
│  └───────────┘ └──────────┘ └────────┘ │
│  ┌───────────┐ ┌──────────┐ ┌────────┐ │
│  │  Journal  │ │  Agent   │ │ Skills │ │
│  │  (Audit)  │ │ Runtime  │ │        │ │
│  └───────────┘ └──────────┘ └────────┘ │
├─────────────────────────────────────────┤
│   SQLite · ACP (Agent Communication)    │
└─────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Go 1.23+
- Node.js 20+
- Git

### 1. Install frontend dependencies

```bash
npm --prefix web install
```

### 2. Start the backend server

```bash
go run ./cmd/ai-flow server --port 8080
```

The server will:
- Create a default config at `.ai-workflow/config.toml` if none exists
- Expose health check at `/health`
- Serve the API under `/api`

### 3. Start the frontend dev server

```bash
npm --prefix web run dev
```

### 4. Open the console

- Frontend: `http://localhost:5173`
- API: `http://localhost:8080/api`

## Quality Gates

Preferred local validation uses native `go` / `npm` commands, matching GitHub Actions:

```bash
gofmt -w $(git ls-files '*.go')
go vet ./...
go test -p 4 -timeout 20m ./...
npm --prefix web ci
npm --prefix web run lint
npm --prefix web run test
npm --prefix web run build
CGO_ENABLED=0 go build -o ./dist/ai-flow ./cmd/ai-flow
```

PowerShell scripts under `scripts/test/` remain available for local Windows smoke and manual regression, but CI no longer depends on them.

## CI/CD

GitHub Actions now covers the full frontend/backend pipeline:

| Workflow | Purpose | Trigger |
|---------|---------|---------|
| `CI` | Backend `gofmt`/`go vet`/`go test`, frontend `lint`/`test`/`build`, plus embedded release build verification | Pull requests, pushes to `main` |
| `Docker` | Validate Docker image on PRs; publish multi-arch images to `ghcr.io/<owner>/<repo>` on `main` and version tags | Pull requests, pushes to `main`, tags `v*` |
| `Release` | Build cross-platform binaries with embedded frontend and publish GitHub Release assets | Tags `v*`, manual dispatch |

## Configuration

Runtime config lives in `.ai-workflow/config.toml` (created automatically on first run). You can override the data directory with the `AI_WORKFLOW_DATA_DIR` environment variable.

The config file is hot-reloaded — changes take effect immediately without restarting the server.

### Agent Drivers

Agent drivers are configured under `[runtime.agents.drivers]`. Each driver points to an AI agent binary (e.g., Claude CLI, Codex) and declares its capabilities (filesystem access, terminal access).

### Agent Profiles

Profiles under `[runtime.agents.profiles]` define execution personas — role, allowed capabilities, and session strategy. The scheduler uses profiles to match actions to the best agent.

## Core Concepts

| Concept | Description |
|---------|-------------|
| **Work Item** | A unit of work with title, priority, labels, and dependencies. Lifecycle: `open` → `accepted` → `queued` → `running` → `done`. |
| **Action** | A step within a Work Item's execution pipeline. Types: `exec` (do work), `gate` (quality check), `plan` (generate sub-actions). Actions form a DAG. |
| **Run** | A single attempt to execute an Action. Supports retries with error classification (transient / permanent / need_help). |
| **Project** | Organizational container for grouping Work Items. |
| **Thread** | A conversation between human and AI agents, optionally linked to a Work Item. |
| **Inspection** | Scheduled or manual audit of project health (cron or on-demand). |
| **Activity Journal** | Unified append-only audit log capturing state changes, tool calls, agent outputs, signals, and usage across all runs. |

## Desktop App

An optional Tauri desktop wrapper is available:

```bash
npm install
npm run tauri:dev     # development
npm run tauri:build   # production build
```

## License

Private repository. All rights reserved.
