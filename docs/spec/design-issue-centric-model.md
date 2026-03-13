# Issue-Centric Execution Model

> 状态：部分实现
> Updated: 2026-03-13
> Current implementation status: the frontend primary route is `/work-items`; backend public REST still uses `/issues`; `/flows` only survives as a frontend compatibility redirect; the internal core model remains `Issue`.

> Design rationale for the unified Issue model that replaces the former Flow + Issue pair.

## Current Compatibility Reality

This document describes the dominant direction of the codebase, but the rename is not fully complete.

- The core domain already treats `Issue` as the unified work unit that replaces the former `Flow + Issue` pair.
- The codebase still retains compatibility naming in several places, such as `FlowScheduler`, `PRFlow` prompts, and some `flow`-prefixed modules/errors.
- The web app uses `/work-items` as the primary route, while legacy `/issues` and `/flows` routes redirect there for compatibility.

Read this spec as "current architecture direction plus compatibility layer", not as "every Flow-era concept has been physically removed from the repository".

## Core Principle

**One Issue = one repo, one worktree, one PR, one complete delivery.**

## Why Not GitHub's Model

GitHub treats Issue and PR as loosely coupled (many-to-many, convention-based linking via `fixes #123`). This works for humans who can judge scope and completion, but fails for AI agents that need deterministic boundaries:

- **Where do I work?** → `Issue.ResourceBindingID` binds to a specific repo
- **What's my scope?** → `Issue.Body` + Steps define the work
- **When am I done?** → All Steps complete (including gate pass + PR merge)

We use **1:1 strong binding** (one Issue = one PR) instead of GitHub's loose coupling because agents need clear, unambiguous work boundaries.

## Architecture: Two-Layer DAG

### Layer 1: Issue DAG (cross-repo parallelism)

Issues declare dependencies on other Issues via `DependsOn`. This forms a project-level DAG where each node is an independent work unit with its own worktree.

```
Project: "User Auth System"
  ├── Issue A: "Backend auth API"    (repo: backend)     ← no deps, runs immediately
  ├── Issue B: "Frontend login UI"   (repo: frontend)    ← no deps, runs in parallel with A
  └── Issue C: "Integration tests"   (repo: backend)     ← DependsOn: [A, B], waits
```

**Parallelism is safe** because each Issue gets its own worktree (branch). Even two Issues on the same repo run on different branches.

### Layer 2: Step Sequence (within-issue execution pipeline)

Steps within an Issue execute sequentially (ordered by `Position`). No Step-level DAG — steps share the same worktree, so parallel execution risks file conflicts.

```
Issue "Backend auth API"
  Step 1: implement          (exec, worker)
  Step 2: commit_push        (exec, builtin)
  Step 3: open_pr            (exec, builtin)
  Step 4: review_merge_gate  (gate, reject → back to Step 1)
```

Gate steps remain at the Step level. A gate rejection resets earlier steps for rework within the same Issue. This is an internal execution loop, not a cross-Issue concern.

## Entity Relationships

```
Project
  ├── ResourceBinding[]          (multi-repo support)
  └── Issue[]                    (DAG via DependsOn)
       ├── ResourceBindingID     (which repo)
       ├── DependsOn: Issue[]    (cross-issue ordering)
       └── Step[]                (ordered by Position)
            └── Execution[]      (attempts per step)
```

## Key Design Decisions

### Issue absorbs Flow

At the domain-model level, the former `Flow` entity (execution container with Steps) is merged into `Issue`. This eliminates:
- A redundant entity and its CRUD surface
- A confusing 1:1 relationship (Issue → FlowID → Flow)
- Two parallel status tracks (IssueStatus + FlowStatus)

Issue now carries both planning metadata (title, body, priority, labels) and execution state (status lifecycle, steps, metadata).

Implementation note: the repository still contains Flow-era compatibility names in scheduler/prompt/UI modules, so this is semantically true before it is fully true as a repository-wide rename.

### Step drops DependsOn, uses Position

Steps within an Issue are ordered by `Position` (integer). The former `Step.DependsOn` DAG is removed because:
- Steps share a worktree → parallel execution is unsafe
- Sequential execution is simpler and sufficient
- Real parallelism belongs at the Issue level (separate worktrees)

### Gate stays at Step level

The PR review gate (implement → commit → open_pr → gate) is an intra-Issue execution loop. It doesn't make sense to split these into separate Issues because they operate on the same repo/branch/worktree.

### Composite Steps create child Issues (not sub-Flows)

When a composite step needs decomposition, it creates child Issues (not sub-Flows). Each child Issue can target a different repo and run in its own worktree.

## Status Lifecycle

```
open → accepted → queued → running → done → closed
                                   → failed
                                   → blocked
                     → cancelled
```

- `open`: created, not yet planned
- `accepted`: steps defined, ready to be queued
- `queued`: submitted to scheduler, waiting for capacity
- `running`: actively executing steps
- `blocked`: waiting for external intervention
- `failed`: execution failed (retries exhausted)
- `done`: all steps completed successfully
- `cancelled`: manually cancelled
- `closed`: archived/completed lifecycle

## Multi-Repo Workflow Example

```
Project: "Payment System" (binds repo: backend, repo: frontend, repo: infra)

Issue 1: "Payment API endpoints"
  ResourceBinding: backend
  Steps: implement → commit → open_pr → gate
  DependsOn: []

Issue 2: "Payment UI components"
  ResourceBinding: frontend
  Steps: implement → commit → open_pr → gate
  DependsOn: []

Issue 3: "Payment infra config"
  ResourceBinding: infra
  Steps: implement → commit → open_pr → gate
  DependsOn: []

Issue 4: "Integration verification"
  ResourceBinding: backend
  Steps: write_tests → commit → open_pr → gate
  DependsOn: [1, 2, 3]
```

Issues 1, 2, 3 run in parallel (different repos, different worktrees). Issue 4 waits for all three to complete before starting.
