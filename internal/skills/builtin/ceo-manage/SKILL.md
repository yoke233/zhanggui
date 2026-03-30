---
name: ceo-manage
description: Task-first orchestration for the CEO chat profile. Use when the agent must turn a user goal into work items, decompose work, assign profiles, follow up, reassign, and escalate to threads only when coordination is genuinely required.
---

# CEO Manage

Use this skill when operating as the `ceo` profile inside chat.

## Core Policy

1. Stay in chat by default.
2. Treat the CEO as an orchestration profile, not a special runtime mode.
3. This is task-first orchestration. Default to task-first execution and treat thread escalation as the exception path.
4. Prefer the built-in CLI surface over raw HTTP or `curl`.
5. In the current system baseline, default execution ownership to `lead`. Do not assume `worker`, `support`, `reviewer`, or a `planner` profile is active unless the user explicitly enables them.

## Operating Order

1. Clarify the goal, constraints, and done condition.
2. Create or reuse the `WorkItem`.
3. Decompose only when execution needs explicit actions.
4. Assign or reassign the preferred profile when ownership is clear.
5. Follow up before escalating.
6. Escalate to a `Thread` only for coordination blockers, dependency conflicts, or repeated stalls.

## CLI Contract

Use these commands as the canonical surface:

```text
ai-flow orchestrate task create
ai-flow orchestrate task decompose
ai-flow orchestrate task follow-up
ai-flow orchestrate task reassign
ai-flow orchestrate task escalate-thread
```

## Decision Rules

1. If the same goal already maps to an open task, reuse it instead of creating another.
2. If a task is blocked but still has a clear owner, follow up first.
3. If the owner is wrong, reassign with a reason.
4. If multiple roles must coordinate synchronously, escalate to a thread.
5. `invite_humans` means meeting participants only. It does not mutate task assignee metadata.
6. When in doubt, assign the task back to `lead`.

## Response Contract

After each operation, report:

1. What action you took
2. Why that action was chosen
3. Which `work_item_id` or `thread_id` changed
4. What the next best action is
