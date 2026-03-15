---
name: task-signal
description: Signal completion or rejection for a ThreadTask. Use when the engine dispatches you as part of a ThreadTask group to report your work result.
---

# Task Signal

You are running inside an `ai-workflow` managed ThreadTask.
Before your final response ends, emit exactly one signal so the scheduler can continue.

## Available Context

| Variable | Meaning |
|---|---|
| `AI_WORKFLOW_TASK_ID` | Current ThreadTask ID |
| `AI_WORKFLOW_TASK_GROUP_ID` | Current TaskGroup ID |
| `AI_WORKFLOW_TASK_TYPE` | Task type: `work` or `review` |
| `AI_WORKFLOW_OUTPUT_FILE` | Expected output file path |
| `AI_WORKFLOW_SERVER_ADDR` | API base URL |
| `AI_WORKFLOW_API_TOKEN` | Scoped bearer token |
| `CODEX_HOME` / `CLAUDE_CONFIG_DIR` | Agent home directory that contains `skills/task-signal/` |

## Signal Map

| Task type | Action | Use when |
|---|---|---|
| `work` | `complete` | Work finished, output file written |
| `review` | `complete` | Review passes (approve) |
| `review` | `reject` | Review fails, needs rework |

## Preferred Path: Run The Bundled Script

Do not assume the current working directory is the skill directory.
Resolve the skill path from the agent home first.

### PowerShell

```powershell
$skillHome = if ($env:CODEX_HOME) {
  Join-Path $env:CODEX_HOME "skills\\task-signal"
} elseif ($env:CLAUDE_CONFIG_DIR) {
  Join-Path $env:CLAUDE_CONFIG_DIR "skills\\task-signal"
} else {
  $null
}

if ($skillHome) {
  pwsh -NoProfile -File (Join-Path $skillHome "scripts\\signal.ps1") complete "outputs/my-output.md"
}
```

### POSIX shell

```bash
SKILL_HOME="${CODEX_HOME:-${CLAUDE_CONFIG_DIR:-}}/skills/task-signal"
bash "$SKILL_HOME/scripts/signal.sh" complete "outputs/my-output.md"
```

## Arguments

```
signal.sh <action> <output_file> [feedback]
```

- `action`: `complete` or `reject`
- `output_file`: path to the output file (relative to thread workspace)
- `feedback`: (reject only) reason for rejection

## Examples

```bash
# Work task completed
signal.sh complete "outputs/competitive-pricing-research.md"

# Review approved
signal.sh complete "outputs/competitive-pricing-review.md"

# Review rejected
signal.sh reject "outputs/competitive-pricing-review.md" "缺少东南亚市场数据"
```

## Rules

1. Emit exactly one signal per task execution.
2. Match the action to your task type and assessment.
3. Write your output file before signaling.
4. Signal before ending the response.
