---
name: sys-step-manage
description: Manage execution steps (actions) within a work item. Create, list, update, delete steps, or let AI auto-decompose a task into a DAG of steps. Use when orchestrating work item execution plans.
---

# Step Management

You can manage execution steps (also called actions) within a work item using the scripts below.

## Available Context

| Variable | Meaning |
|---|---|
| `AI_WORKFLOW_SERVER_ADDR` | Backend API base URL (e.g. `http://127.0.0.1:8080`) |
| `AI_WORKFLOW_API_TOKEN` | Bearer token for API authentication |
| `CODEX_HOME` / `CLAUDE_CONFIG_DIR` | Agent home directory containing `skills/sys-step-manage/` |

Resolve the skill directory before calling any script:

### POSIX shell

```bash
SKILL_HOME="${CODEX_HOME:-${CLAUDE_CONFIG_DIR:-}}/skills/sys-step-manage"
```

### PowerShell

```powershell
$skillHome = if ($env:CODEX_HOME) {
  Join-Path $env:CODEX_HOME "skills\sys-step-manage"
} elseif ($env:CLAUDE_CONFIG_DIR) {
  Join-Path $env:CLAUDE_CONFIG_DIR "skills\sys-step-manage"
} else {
  $null
}
```

## Operations

### Create Step

Create a new step for a work item.

```bash
bash "$SKILL_HOME/scripts/create-step.sh" <work-item-id> '<json>'
```

```powershell
pwsh -NoProfile -File "$skillHome\scripts\create-step.ps1" <work-item-id> '<json>'
```

JSON fields:

| Field | Required | Description |
|---|---|---|
| `name` | yes | Unique step name (lowercase, dash-separated) |
| `type` | yes | `exec` (implementation), `gate` (review/approval), or `composite` |
| `description` | no | What this step should accomplish |
| `agent_role` | no | `worker`, `gate`, `lead`, or `support` |
| `required_capabilities` | no | Capability tags the assigned agent must have |
| `acceptance_criteria` | no | Conditions for step completion |
| `timeout` | no | Go duration string (e.g. `30m`, `1h`) |
| `max_retries` | no | Max retry count on failure |

Example:

```bash
bash "$SKILL_HOME/scripts/create-step.sh" 5 '{"name":"implement-auth","type":"exec","description":"Implement JWT authentication","agent_role":"worker","required_capabilities":["backend"],"acceptance_criteria":["JWT tokens are issued on login","Token validation middleware works"]}'
```

### List Steps

List all steps for a work item.

```bash
bash "$SKILL_HOME/scripts/list-steps.sh" <work-item-id>
```

```powershell
pwsh -NoProfile -File "$skillHome\scripts\list-steps.ps1" <work-item-id>
```

### Get Step

Get details of a specific step.

```bash
bash "$SKILL_HOME/scripts/get-step.sh" <step-id>
```

```powershell
pwsh -NoProfile -File "$skillHome\scripts\get-step.ps1" <step-id>
```

### Update Step

Update a pending step (only pending steps can be edited).

```bash
bash "$SKILL_HOME/scripts/update-step.sh" <step-id> '<json>'
```

```powershell
pwsh -NoProfile -File "$skillHome\scripts\update-step.ps1" <step-id> '<json>'
```

All fields are optional — only provided fields are applied. Same fields as create, plus `position`.

### Delete Step

Delete a pending step.

```bash
bash "$SKILL_HOME/scripts/delete-step.sh" <step-id>
```

```powershell
pwsh -NoProfile -File "$skillHome\scripts\delete-step.ps1" <step-id>
```

### Generate Steps (AI Decomposition)

Let the backend AI decompose a task description into a DAG of steps and create them automatically.

```bash
bash "$SKILL_HOME/scripts/generate-steps.sh" <work-item-id> '<description>'
```

```powershell
pwsh -NoProfile -File "$skillHome\scripts\generate-steps.ps1" <work-item-id> '<description>'
```

This calls the `plan-actions` planning service on the backend. Use it when you want a quick auto-decomposition. For more control, create steps manually.

## Step Design Guidelines

When creating steps manually, follow these rules:

1. Each step name must be unique, lowercase, dash-separated.
2. Prefer fewer outcome-oriented steps over many tiny procedural steps.
3. Choose `agent_role` based on the nature of the work:
   - `worker` — implementation, coding, testing
   - `gate` — review, approval, quality validation
   - `lead` — orchestration, planning (rare for individual steps)
   - `support` — analysis, research
4. Insert a `gate` step whenever review or approval is needed.
5. Include at least one acceptance criterion per step.
6. Use `type: composite` only when the step should expand into a sub-workflow.

## Output

All scripts output JSON to stdout. On error, a message is written to stderr with a non-zero exit code.
