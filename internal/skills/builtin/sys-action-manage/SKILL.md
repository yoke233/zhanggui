---
name: sys-action-manage
description: Manage execution actions within a work item. Create, list, update, delete actions, or let AI auto-decompose a task into a DAG of actions. Use when orchestrating work item execution plans.
---

# Action Management

You can manage execution actions within a work item using the scripts below.

## Available Context

| Variable | Meaning |
|---|---|
| `AI_WORKFLOW_SERVER_ADDR` | Backend API base URL (e.g. `http://127.0.0.1:8080`) |
| `AI_WORKFLOW_API_TOKEN` | Bearer token for API authentication |
| `CODEX_HOME` / `CLAUDE_CONFIG_DIR` | Agent home directory containing `skills/sys-action-manage/` |

Resolve the skill directory before calling any script:

### POSIX shell

```bash
SKILL_HOME="${CODEX_HOME:-${CLAUDE_CONFIG_DIR:-}}/skills/sys-action-manage"
```

### PowerShell

```powershell
$skillHome = if ($env:CODEX_HOME) {
  Join-Path $env:CODEX_HOME "skills\sys-action-manage"
} elseif ($env:CLAUDE_CONFIG_DIR) {
  Join-Path $env:CLAUDE_CONFIG_DIR "skills\sys-action-manage"
} else {
  $null
}
```

## Operations

### Create Action

Create a new action for a work item.

```bash
bash "$SKILL_HOME/scripts/create-action.sh" <work-item-id> '<json>'
```

```powershell
pwsh -NoProfile -File "$skillHome\scripts\create-action.ps1" <work-item-id> '<json>'
```

JSON fields:

| Field | Required | Description |
|---|---|---|
| `name` | yes | Unique action name (lowercase, dash-separated) |
| `type` | yes | `exec` (implementation), `gate` (review/approval), or `composite` |
| `description` | no | What this action should accomplish |
| `agent_role` | no | `worker`, `gate`, `lead`, or `support` |
| `required_capabilities` | no | Capability tags the assigned agent must have |
| `acceptance_criteria` | no | Conditions for action completion |
| `timeout` | no | Go duration string (e.g. `30m`, `1h`) |
| `max_retries` | no | Max retry count on failure |

Example:

```bash
bash "$SKILL_HOME/scripts/create-action.sh" 5 '{"name":"implement-auth","type":"exec","description":"Implement JWT authentication","agent_role":"worker","required_capabilities":["backend"],"acceptance_criteria":["JWT tokens are issued on login","Token validation middleware works"]}'
```

### List Actions

List all actions for a work item.

```bash
bash "$SKILL_HOME/scripts/list-actions.sh" <work-item-id>
```

```powershell
pwsh -NoProfile -File "$skillHome\scripts\list-actions.ps1" <work-item-id>
```

### Get Action

Get details of a specific action.

```bash
bash "$SKILL_HOME/scripts/get-action.sh" <action-id>
```

```powershell
pwsh -NoProfile -File "$skillHome\scripts\get-action.ps1" <action-id>
```

### Update Action

Update a pending action (only pending actions can be edited).

```bash
bash "$SKILL_HOME/scripts/update-action.sh" <action-id> '<json>'
```

```powershell
pwsh -NoProfile -File "$skillHome\scripts\update-action.ps1" <action-id> '<json>'
```

All fields are optional — only provided fields are applied. Same fields as create, plus `position`.

### Delete Action

Delete a pending action.

```bash
bash "$SKILL_HOME/scripts/delete-action.sh" <action-id>
```

```powershell
pwsh -NoProfile -File "$skillHome\scripts\delete-action.ps1" <action-id>
```

### Generate Actions (AI Decomposition)

Let the backend AI decompose a task description into a DAG of actions and create them automatically.

```bash
bash "$SKILL_HOME/scripts/generate-actions.sh" <work-item-id> '<description>'
```

```powershell
pwsh -NoProfile -File "$skillHome\scripts\generate-actions.ps1" <work-item-id> '<description>'
```

This calls the `plan-actions` planning service on the backend. Use it when you want a quick auto-decomposition. For more control, create actions manually.

## Action Design Guidelines

When creating actions manually, follow these rules:

1. Each action name must be unique, lowercase, dash-separated.
2. Prefer fewer outcome-oriented actions over many tiny procedural actions.
3. Choose `agent_role` based on the nature of the work:
   - `worker` — implementation, coding, testing
   - `gate` — review, approval, quality validation
   - `lead` — orchestration, planning (rare for individual steps)
   - `support` — analysis, research
4. Insert a `gate` action whenever review or approval is needed.
5. Include at least one acceptance criterion per action.
6. Use `type: composite` only when the action should expand into a sub-workflow.

## Output

All scripts output JSON to stdout. On error, a message is written to stderr with a non-zero exit code.
