# ai-workflow v3 System Specification

> **Status:** Living document. Reflects codebase state as of 2026-03-05 (branch `feat/v22-a2a-refactor`).
>
> **Supersedes:** `spec-overview.md`, `spec-run-engine.md`, `spec-team-leader-layer.md`, `spec-api-config.md`, `spec-agent-drivers.md`, `PLAN.md`.

---

## 1. Overview

ai-workflow is an agent orchestration system that decomposes user goals into issues, schedules runs against those issues, and drives multi-stage ACP agent execution with review gates and auto-merge.

**Core loop:** `User/A2A message -> Issue -> Run -> Stages (ACP) -> Review -> Merge -> Done`

**Design principles:**
- Issue is the unit of work; Run is the unit of execution.
- All status changes go through validated state machines (`ValidateIssueTransition`, `TransitionStatus`).
- ACP (Agent Communication Protocol) over stdio is the sole execution path.
- Event-driven coordination via in-process `EventBus`.
- A2A (Agent-to-Agent) protocol is the primary external control plane.

---

## 2. Domain Model

### 2.1 Project

| Field | Type | Constraints |
|-------|------|-------------|
| ID | string | Primary key |
| Name | string | Required |
| RepoPath | string | Unique across system |
| DefaultBranch | string | Auto-detected at creation via `git rev-parse`, fallback `main` |
| GitHubOwner | string | Optional |
| GitHubRepo | string | Optional |

All Runs use `project.DefaultBranch` as base branch. No runtime HEAD detection.

### 2.2 Issue

The minimal deliverable unit with a multi-path state machine.

#### Fields

| Field | Type | Description |
|-------|------|-------------|
| ID | string | `issue-YYYYMMDD-xxxxxxxx` |
| ProjectID | string | Required |
| SessionID | string | Groups issues within a conversation |
| ParentID | string | Parent issue ID (decomposition hierarchy) |
| Title | string | Required, 1-120 chars after trim |
| Body | string | Detailed description |
| Template | string | Required, no whitespace. `"epic"` triggers decomposition |
| Labels | []string | Tags; `"decompose"` also triggers decomposition |
| AutoMerge | bool | When true, run completion triggers merge flow |
| Status | IssueStatus | Orchestration progress |
| State | IssueState | `open` / `closed` |
| FailPolicy | FailurePolicy | `block` (default) / `skip` / `human` |
| RunID | string | Associated Run ID (set when executing) |
| MergeRetries | int | Merge conflict retry counter |
| TriageInstructions | string | Reserved for future TL triage |
| SubmittedBy | string | Submitter identity |
| ExternalID | string | External system reference (e.g. GitHub issue number) |
| SupersededBy | string | Replacement issue ID |
| Priority | int | Scheduling priority |

#### IssueStatus State Machine

```
                    +-> decomposing -> decomposed -> done
                    |
draft -> reviewing -+-> queued -> ready -> executing -> merging -> done
                    |                          |           |
                    +-> abandoned              +-> failed <+
                                               |
                                               +-> queued (retry)
```

**Transition matrix** (idempotent self-transitions always valid):

| From | Valid To |
|------|----------|
| draft | reviewing, abandoned |
| reviewing | draft, queued, decomposing, abandoned |
| queued | ready, executing, failed, abandoned |
| ready | queued, executing, failed, abandoned |
| executing | queued, merging, done, failed, abandoned |
| merging | queued, done, failed, abandoned |
| decomposing | decomposed, failed, abandoned |
| decomposed | done, failed, abandoned |
| failed | queued, abandoned |
| done | superseded |
| superseded | (terminal) |
| abandoned | (terminal) |

#### FailurePolicy

| Value | Behavior |
|-------|----------|
| `block` | Halt session; block sibling issues |
| `skip` | Mark failed, continue siblings |
| `human` | Halt session; escalate to human |

### 2.3 Run

One execution instance bound to an Issue.

#### Fields

| Field | Type | Description |
|-------|------|-------------|
| ID | string | `YYYYMMDD-xxxxxxxxxxxx` |
| ProjectID | string | Required |
| IssueID | string | Bound issue |
| Name | string | Display name |
| Description | string | Forwarded from issue body |
| Template | string | Stage template name |
| Status | RunStatus | Execution state |
| Conclusion | RunConclusion | Terminal outcome (only when completed) |
| CurrentStage | StageID | Active stage |
| Stages | []StageConfig | Ordered stage pipeline |
| BranchName | string | Git branch created by workspace plugin |
| WorktreePath | string | Isolated working directory |
| ErrorMessage | string | Last error description |
| MaxTotalRetries | int | Budget (default 5) |
| TotalRetries | int | Counter across all stages |
| Artifacts | map | PR number, trace ID, etc. |
| Config | map | Workflow profile, base branch, etc. |

#### RunStatus State Machine

```
queued -> in_progress -> completed (success/failure/timed_out/cancelled)
              |    ^          |
              |    +----------+ (retry: completed -> in_progress)
              v
        action_required
              |
              +-> completed
              +-> in_progress (resume)
              +-> queued (re-enqueue)
```

**Transition matrix** (idempotent self-transitions always valid):

| From | Valid To |
|------|----------|
| queued | in_progress, completed (abort) |
| in_progress | completed, action_required, queued (re-enqueue) |
| action_required | in_progress, completed, queued (re-enqueue) |
| completed | in_progress (retry from failure) |

**All status writes MUST use `(*Run).TransitionStatus(to)`.** Raw `.Status =` assignments are forbidden.

#### RunConclusion

| Value | Meaning |
|-------|---------|
| `success` | All stages passed |
| `failure` | Stage failed, retry exhausted or aborted |
| `timed_out` | Idle or wall-clock timeout fired |
| `cancelled` | Externally cancelled |

### 2.4 Stage

Stages define the execution pipeline within a Run.

#### StageID Constants

| ID | Built-in handler | Agent required |
|----|------------------|----------------|
| `setup` | `runWorktreeSetup` | No |
| `requirements` | ACP agent | Yes |
| `implement` | ACP agent | Yes |
| `review` | ACP agent | Yes |
| `fixup` | ACP agent (reuses implement session) | Yes |
| `test` | ACP agent | Yes |
| `merge` | `runMerge` (git merge) | No |
| `cleanup` | `runCleanup` (workspace teardown) | No |

#### StageConfig

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| Name | StageID | - | Stage identifier |
| Role | string | - | Role for agent resolution |
| Agent | string | - | Direct agent override |
| PromptTemplate | string | stage name | Template file in `prompt_templates/` |
| IdleTimeout | Duration | 5m (1m for setup/merge/cleanup, 3m for test) | Cancel if no agent output |
| Timeout | Duration | 0 | Wall-clock timeout (lower priority than IdleTimeout) |
| MaxRetries | int | 1 | Per-stage retry limit |
| OnFailure | string | `human` | `retry` / `skip` / `abort` / `human` |
| RequireHuman | bool | false | Pause after success for human approval |
| ReuseSessionFrom | StageID | - | Reuse ACP session from prior stage |

#### Templates

| Name | Stages |
|------|--------|
| `full` | setup, requirements, implement, review, fixup, test, merge, cleanup |
| `standard` | setup, requirements, implement, review, fixup, merge, cleanup |
| `quick` | setup, requirements, implement, review, merge, cleanup |
| `hotfix` | setup, implement, merge, cleanup |

#### Checkpoint

Each stage attempt records a Checkpoint:

| Field | Type | Description |
|-------|------|-------------|
| RunID | string | Parent run |
| StageName | StageID | Stage |
| Status | string | `in_progress` / `success` / `failed` / `skipped` / `invalidated` |
| AgentUsed | string | Resolved agent ID |
| RetryCount | int | Attempt number (0-based) |
| Error | string | Failure reason |

### 2.5 Event

All domain state changes emit events on the in-process EventBus.

#### Event Types (46 total)

**Stage execution:**
`stage_start`, `stage_complete`, `stage_failed`, `human_required`, `run_done`, `run_action_required`, `run_resumed`, `action_applied`, `agent_output`, `run_stuck`

**Team Leader:**
`team_leader_thinking`, `team_leader_files_changed`, `run_started`, `run_update`, `run_completed`, `run_failed`, `run_cancelled`

**Issue lifecycle:**
`issue_created`, `issue_reviewing`, `review_done`, `issue_approved`, `issue_queued`, `issue_ready`, `issue_executing`, `issue_done`, `issue_failed`, `issue_decomposing`, `issue_decomposed`, `issue_dependency_changed`

**Merge:**
`issue_merging`, `issue_merged`, `issue_merge_conflict`, `issue_merge_retry`, `merge_failed`, `auto_merged`

**GitHub integration:**
`github_webhook_received`, `github_issue_opened`, `github_issue_comment_created`, `github_pull_request_review_submitted`, `github_pull_request_closed`, `github_reconnected`

**Admin:**
`admin_operation`

---

## 3. Execution Engine

### 3.1 ACP Protocol

The sole execution path. CLI agent plugins have been removed.

```
Executor
  -> RoleResolver.Resolve(stage.Role)
  -> AgentProfile + RoleProfile
  -> ACP LaunchConfig -> Initialize -> NewSession -> Prompt
  -> stageEventBridge converts session updates -> EventAgentOutput
```

**Session pool:** Executor maintains `acpPool[runID:stageID]`. Stages can declare `ReuseSessionFrom` to share sessions (e.g., fixup reuses implement). Pool is cleaned up when the run ends.

### 3.2 Stage Execution Flow

For each stage in `run.Stages`:

1. **Built-in stages** (setup/merge/cleanup): execute directly, skip ACP.
2. **Agent stages**: resolve role -> render prompt -> execute via ACP (or `testStageFunc` in tests).
3. On **success**: record `CheckpointSuccess`, advance to next stage.
4. On **failure**: evaluate `OnFailure` reaction rules:
   - `retry` -> re-attempt if within `MaxRetries`
   - `skip` -> record `CheckpointSkipped`, continue
   - `human` -> transition run to `action_required`, return
   - `abort` -> fail the run
5. After all stages succeed: transition run to `completed` with `ConclusionSuccess`, publish `EventRunDone`.

### 3.3 Idle Timeout

Each agent stage monitors `stageEventBridge.lastActivity` (atomic timestamp updated on every `HandleSessionUpdate`). A background goroutine (`startIdleChecker`) cancels the context when no activity is detected for `stage.IdleTimeout`.

Priority: `IdleTimeout > 0` takes precedence over `Timeout > 0`. Both zero means no timeout.

### 3.4 Run Actions

Users can interact with paused runs (`action_required` status):

| Action | Effect |
|--------|--------|
| `approve` | Mark current stage approved, resume execution |
| `reject` | Invalidate stage checkpoint, set error message |
| `modify` | Update run description/config |
| `skip` | Skip current stage |
| `rerun` | Re-execute from current stage |
| `change_role` | Change stage agent role |
| `abort` | Fail the run |
| `pause` | Transition to action_required |
| `resume` | Transition back to in_progress, resume |

---

## 4. Scheduling & Orchestration

### 4.1 DepScheduler

The DepScheduler manages issue-to-run lifecycle within sessions:

1. **Session grouping:** Issues are grouped by `SessionID`. Each session maintains an independent `runningSession` snapshot.
2. **Profile queue:** Issues transition `queued -> ready` via `markReadyByProfileQueueLocked`, ordered by `WorkflowProfileType` priority: `strict > normal > fast_release`.
3. **Dispatch:** `dispatchReadyAcrossSessions` round-robins across sessions, creates Runs from ready issues, acquires semaphore slots, and launches execution goroutines.
4. **Event handling:** `OnEvent` processes `EventRunDone/Failed/IssueMerged/MergeFailed/MergeRetry/IssueFailed/MergeConflict`, transitions issue status, releases slots, and triggers re-dispatch.
5. **Concurrency:** Semaphore-based (`maxConcurrent`). Slot acquired at dispatch, released on run completion/failure.
6. **Recovery:** `RecoverExecutingIssues` replays terminal events for in-flight runs after crash.

### 4.2 Run Scheduler (engine.Scheduler)

Polls for `queued` runs and dispatches them:

- `maxGlobal` / `maxPerProject` concurrency limits.
- Busy worktree deduplication (no two runs share a worktree path).
- CAS mark via `TryMarkRunInProgress` to prevent double-dispatch.

### 4.3 Auto-Merge Flow

When `issue.AutoMerge = true` and `EventRunDone` fires:

1. DepScheduler transitions issue to `merging`.
2. External merge handler (AutoMergeHandler) executes:
   - Test gate: run `go test` on changed packages (10 min timeout).
   - Create PR (draft -> ready).
   - Merge PR.
3. On success: publish `EventIssueMerged` -> issue transitions to `done`.
4. On failure: publish `EventMergeFailed` -> issue transitions to `failed`.
5. On conflict: publish `EventIssueMergeConflict` -> can retry with `EventIssueMergeRetry` (increments `MergeRetries`).

---

## 5. A2A Protocol (Agent-to-Agent)

Primary external control plane. JSON-RPC 2.0 over HTTP.

### 5.1 Endpoint

```
POST /api/v1/a2a
Authorization: Bearer <token>
```

Agent card: `GET /.well-known/agent-card.json` (A2A protocol version 0.3).

### 5.2 Methods

| Method | Description |
|--------|-------------|
| `ai.workflow.message.send` | Create issue or reply to INPUT_REQUIRED task |
| `ai.workflow.tasks.get` | Get task status and artifacts |
| `ai.workflow.tasks.cancel` | Cancel/abandon a task |
| `ai.workflow.tasks.list` | List tasks with filtering and pagination |
| `ai.workflow.message.stream` | SSE streaming variant of message.send |

### 5.3 Task State Mapping

| Issue Status | A2A Task State |
|--------------|----------------|
| draft | submitted |
| reviewing | input-required |
| queued, ready, executing, merging, decomposing, decomposed | working |
| done | completed |
| failed | failed |
| superseded, abandoned | canceled |

### 5.4 SendMessage Flow

1. Create Issue with `AutoMerge=true`, `FailPolicy=block`, `Template="standard"`.
2. Auto-approve: transition `draft -> reviewing -> queued` (bypasses human review).
3. DepScheduler picks up issue, creates Run, dispatches execution.
4. Client polls `tasks/get` until state reaches `completed` or `failed`.

### 5.5 Follow-up (Reply to INPUT_REQUIRED)

When `TaskID` is non-empty in SendMessage, the bridge treats it as a reply:
- Issue must be in `reviewing` status.
- Conversation text is forwarded as approve feedback.
- Issue transitions to `queued` and execution resumes.

### 5.6 Task Artifacts

`tasks/get` enriches the snapshot with Run artifacts when available:
- `branch_name`: Git branch
- `pr_number`, `pr_url`: Pull request info
- `conclusion`: Run conclusion

---

## 6. REST API

### 6.1 Base Path

All REST endpoints under `/api/v3`. Legacy `/api/v1` (except health and A2A) and `/api/v2` routes have been removed.

### 6.2 Routes

**Projects**
- `GET /api/v3/projects` — list
- `GET /api/v3/projects/{id}` — get
- `POST /api/v3/projects` — create
- `PUT /api/v3/projects/{id}` — update
- `DELETE /api/v3/projects/{id}` — delete

**Issues**
- `GET /api/v3/issues` — list (query: `project_id`)
- `GET /api/v3/issues/{id}` — get
- `POST /api/v3/issues` — create
- `POST /api/v3/issues/{id}/action` — apply action (approve/reject/abandon)
- `GET /api/v3/issues/{id}/changes` — timeline
- `GET /api/v3/issues/{id}/reviews` — review records

**Runs**
- `GET /api/v3/runs` — list (query: `project_id`)
- `GET /api/v3/runs/{id}` — get
- `GET /api/v3/runs/{id}/events` — event stream
- `POST /api/v3/runs/{id}/action` — apply run action
- `GET /api/v3/runs/{id}/checkpoints` — checkpoints

**Sessions**
- `POST /api/v3/sessions` — create/continue chat session
- `GET /api/v3/sessions/{id}/runs/events` — session run events
- `POST /api/v3/sessions/{id}/runs/cancel` — cancel active run

**Events**
- `GET /api/v3/events` — unified event query (scope, project_id, run_id, issue_id, session_id, event_type filters)

**Repo**
- `GET /api/v3/projects/{id}/repo/status` — git status
- `GET /api/v3/projects/{id}/repo/tree` — directory listing
- `GET /api/v3/projects/{id}/repo/diff` — file diff

**Admin**
- `POST /api/v3/admin/webhooks/replay` — replay webhook
- `POST /api/v3/admin/force-ready` — force issue to ready (bypass state machine)

**WebSocket**
- `GET /api/v3/ws` — real-time event stream

**Health**
- `GET /api/v1/health` — health check (kept on v1 for compatibility)

### 6.3 Error Format

```json
{
  "error": "human-readable message",
  "code": "MACHINE_READABLE_CODE",
  "details": {}
}
```

---

## 7. Agent Configuration

### 7.1 Agent Profiles

Defined in `configs/defaults.yaml`:

| Agent | Launch Command | Capabilities |
|-------|---------------|--------------|
| `claude` | `npx -y @zed-industries/claude-agent-acp@latest` | fs_read, fs_write, terminal |
| `codex` | `npx -y @zed-industries/codex-acp@latest` | fs_read, fs_write, terminal |

### 7.2 Role Profiles

| Role | Agent | fs_read | fs_write | terminal | MCP | Purpose |
|------|-------|---------|----------|----------|-----|---------|
| team_leader | claude | Y | Y | Y | Y | Orchestration, planning |
| worker | codex | Y | Y | Y | N | Code implementation |
| reviewer | claude | Y | N | N | N | Code review |
| aggregator | claude | Y | N | N | N | Review aggregation |
| decomposer | claude | Y | N | N | Y | Issue decomposition |
| plan_parser | claude | Y | N | N | N | Plan parsing |

### 7.3 Role Resolution

`stage.Role -> RoleResolver.Resolve(role) -> (AgentProfile, RoleProfile)`

Capabilities are intersected: `min(agent.capabilities_max, role.capabilities)`.

---

## 8. Issue Decomposition

When an issue has `Template="epic"` or `Labels` contains `"decompose"`:

1. `ApplyIssueAction(approve)` transitions to `decomposing` instead of `queued`.
2. Decomposer agent (role: `decomposer`) generates `DecomposeSpec`:
   - `ParentID`: parent issue ID
   - `ProjectID`: target project (default: inherit parent)
   - `Children`: list of child issue specs with titles, bodies, dependencies, labels
3. Child issues are created with `ParentID` set, status `draft`.
4. Parent transitions to `decomposed`.
5. Children follow normal lifecycle independently.

---

## 9. Persistence

### 9.1 Store Interface

SQLite-backed (`modernc.org/sqlite`), migrations in code.

**Core operations:**
- Project CRUD
- Issue CRUD + `GetActiveIssues`, `GetChildIssues`, `GetIssueByRun`
- Run CRUD + `GetActiveRuns`, `ListRunnableRuns`, `TryMarkRunInProgress`
- Checkpoint CRUD + `InvalidateCheckpointsFromStage`
- IssueChange timeline
- ReviewRecord CRUD
- ChatSession + ChatMessage CRUD
- UnifiedEvent (scope-filtered) + RunEvent (legacy)
- RunAction recording

### 9.2 Key Tables

`projects`, `issues`, `runs`, `checkpoints`, `review_records`, `issue_changes`, `events`, `run_events`, `chat_sessions`, `chat_run_events`, `actions`

---

## 10. Frontend

React + Vite + Tailwind + Zustand. Embedded into Go binary via `web/embed.go`.

### 10.1 Views

| View | Path | Description |
|------|------|-------------|
| ChatView | `/chat` | Team Leader conversation |
| A2AChatView | `/a2a` | A2A task monitoring |
| BoardView | `/board` | Issue kanban board |
| RunView | `/run/{id}` | Run detail with stage progress |

### 10.2 API Client

All frontend requests target `/api/v3`. TypeScript types in `web/src/types/` mirror Go domain types.

Key type additions:
- `RunConclusion`: `"success" | "failure" | "timed_out" | "cancelled"`
- `Run.conclusion`, `Run.issue_id`
- `Issue.parent_id`, `Issue.merge_retries`, `Issue.triage_instructions`

---

## 11. Configuration Hierarchy

```
Built-in defaults < configs/defaults.yaml < .ai-workflow/config.yaml (project) < Environment variables
```

Key environment variables:
- `AI_WORKFLOW_DB_PATH` — SQLite database path
- `AI_WORKFLOW_CHAT_PROVIDER` — Chat backend selection

---

## 12. Context & Memory (OpenViking)

> **Full spec:** `spec-context-memory.md`

OpenViking provides two capabilities for the system:

### 12.1 Project Knowledge Pre-digestion

Projects are imported into OpenViking at registration. OpenViking auto-generates L0/L1 summaries for every directory and file. TL can quickly understand any project's design via `overview()` / `abstract()` without reading raw source code.

```
Project registered → AddResource(project.RepoPath)  → async L0/L1 generation
Issue merged       → incremental update changed files
```

### 12.2 Execution Experience Accumulation

All ACP sessions call `session.Commit()` on completion. OpenViking auto-extracts `cases` (problem-solution pairs) and `patterns` (behavioral patterns) into per-role memory pools (`agent/memories/`), isolated by `agent_id`.

### 12.3 Primary Consumer: Team Leader

TL is the only role that actively queries OpenViking via on-demand MCP tools:

| Tool | Purpose |
|------|---------|
| `context_overview(uri)` | L1 summary of a project module |
| `context_abstract(uri)` | L0 quick scan across directories |
| `context_read(uri)` | Full file content |
| `context_search(query)` | Cross-project semantic search |
| `memory_search(query)` | Recall past execution experiences |
| `memory_save(content, tags)` | Manually save observations (P1) |

Worker/Reviewer do not query OpenViking (they work in worktrees with project files directly available). Their sessions still commit experiences via `session.Commit()`.

### 12.4 Configuration

```yaml
context:
  provider: openviking        # openviking | context-sqlite | mock
  openviking:
    url: "http://localhost:1933"
    api_key: ""
  fallback: context-sqlite
```

Fallback to SQLite: CRUD works, L0/L1/Search unavailable (returns empty).

### 12.5 Core Interface

```go
type ContextStore interface {
    Plugin
    Read(ctx context.Context, uri string) ([]byte, error)
    Write(ctx context.Context, uri string, content []byte) error
    List(ctx context.Context, uri string) ([]ContextEntry, error)
    Remove(ctx context.Context, uri string) error
    Abstract(ctx context.Context, uri string) (string, error)
    Overview(ctx context.Context, uri string) (string, error)
    Find(ctx context.Context, query string, opts FindOpts) ([]ContextResult, error)
    Search(ctx context.Context, query string, sessionID string, opts SearchOpts) ([]ContextResult, error)
    AddResource(ctx context.Context, path string, opts AddResourceOpts) error
    CreateSession(ctx context.Context, id string) (ContextSession, error)
    GetSession(ctx context.Context, id string) (ContextSession, error)
}
```

Plugin slot: `SlotContext`. Implementations: `context-openviking` (primary), `context-sqlite` (fallback), `context-mock` (test).

---

## 13. Non-Goals & Future Reservations

**Detailed design (separate spec docs):**
- Orchestration modes (Pipeline + Collaboration) — see `spec-orchestration-modes.md`
- Distributed deployment (TL local + workers cloud) — see `spec-distributed-deployment.md`
- Context & Memory (OpenViking integration) — see `spec-context-memory.md`

**Not implemented (data positions reserved):**
- `issues.triage_instructions` — pre-positioned for TL ACP triage routing
- `issues.submitted_by` — pre-positioned for multi-user token scoping
- EscalationRouter / Directive / InboxItem — domain types and config skeleton only
- Cross-project decomposition — `DecomposeSpec.ProjectID` field exists but defaults to parent

**Explicitly removed:**
- CLI agent plugins (`core.AgentPlugin`, `core.RuntimePlugin`)
- `/api/v1` and `/api/v2` route groups (except `/api/v1/health` and A2A)
- `task/plan` business entities and DAG runtime scheduling
- `Secretary` naming (replaced by Team Leader)
