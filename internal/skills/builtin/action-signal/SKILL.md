---
name: action-signal
description: Emit the terminal decision for the current ai-workflow action run. Use when this skill is auto-injected for exec or gate actions and the engine needs exactly one final signal such as complete, need_help, approve, or reject before the response ends.
---

# Action Signal

You are running inside an `ai-workflow` managed action.
Before your final response ends, emit exactly one terminal signal so the engine can continue.

## Available Context

| Variable | Meaning |
|---|---|
| `AI_WORKFLOW_ACTION_TYPE` | Current action type: `exec` or `gate` |
| `AI_WORKFLOW_ACTION_ID` | Current action ID |
| `AI_WORKFLOW_WORK_ITEM_ID` | Current work item ID |
| `AI_WORKFLOW_RUN_ID` | Current run ID |
| `AI_WORKFLOW_SERVER_ADDR` | Decision API base URL |
| `AI_WORKFLOW_API_TOKEN` | Scoped bearer token for the decision API |
| `CODEX_HOME` / `CLAUDE_CONFIG_DIR` | Agent home directory that contains `skills/action-signal/` |

## Decision Map

| Action type | Decision | Use when |
|---|---|---|
| `exec` | `complete` | Work finished and acceptance criteria are met |
| `exec` | `need_help` | You are blocked and need human help |
| `gate` | `approve` | Review passes |
| `gate` | `reject` | Review fails and needs rework |

## Preferred Path: Run The Bundled Script

Do not assume the current working directory is the skill directory.
Resolve the skill path from the agent home first.

For artifact-producing workflows, the bundled scripts also accept an optional third argument:
compact JSON metadata that will be merged into the decision payload.
Use that to attach fields such as `summary`, `artifact_namespace`, `artifact_type`,
`artifact_format`, `artifact_relpath`, `artifact_title`, `producer_skill`, and `producer_kind`.

### PowerShell

```powershell
$skillHome = if ($env:CODEX_HOME) {
  Join-Path $env:CODEX_HOME "skills\\action-signal"
} elseif ($env:CLAUDE_CONFIG_DIR) {
  Join-Path $env:CLAUDE_CONFIG_DIR "skills\\action-signal"
} else {
  $null
}

if ($skillHome) {
  pwsh -NoProfile -File (Join-Path $skillHome "scripts\\signal.ps1") complete "implemented auth module with tests"
}
```

### POSIX shell

```bash
SKILL_HOME="${CODEX_HOME:-${CLAUDE_CONFIG_DIR:-}}/skills/action-signal"
bash "$SKILL_HOME/scripts/signal.sh" complete "implemented auth module with tests"
```

Use the matching decision for your action type. The bundled scripts try HTTP first and fall back to output if HTTP is unavailable.

### Structured Metadata Example

```powershell
$metadata = '{"summary":"login flow review complete","artifact_namespace":"gstack","artifact_type":"review_report","artifact_format":"markdown","artifact_relpath":".ai-workflow/artifacts/gstack/review/2026-03-21-login-flow.md","artifact_title":"Login Flow Review","producer_skill":"gstack-review","producer_kind":"skill"}'
pwsh -NoProfile -File (Join-Path $skillHome "scripts\\signal.ps1") reject "missing null-handling in callback flow" $metadata
```

## Fallback Path: Emit The Signal Line Directly

If you cannot reliably run the script, print one raw standalone line in your response:

```text
AI_WORKFLOW_SIGNAL: {"decision":"<decision>","reason":"<reason>"}
```

You may include extra JSON fields on that same line when you need structured metadata:

```text
AI_WORKFLOW_SIGNAL: {"decision":"complete","reason":"implemented auth module","summary":"design note completed","artifact_namespace":"gstack","artifact_type":"design_doc","artifact_relpath":".ai-workflow/artifacts/gstack/office-hours/2026-03-21-login-flow.md","producer_skill":"gstack-office-hours","producer_kind":"skill"}
```

Requirements for the fallback line:

1. Keep it on a single line.
2. Do not prefix it with bullets, quotes, or labels.
3. Do not emit more than one `AI_WORKFLOW_SIGNAL:` line.

## Examples

```text
AI_WORKFLOW_SIGNAL: {"decision":"complete","reason":"implemented auth module with tests"}
AI_WORKFLOW_SIGNAL: {"decision":"need_help","reason":"cannot resolve dependency conflict after verifying available versions"}
AI_WORKFLOW_SIGNAL: {"decision":"approve","reason":"all acceptance criteria passed and no blocking defects found"}
AI_WORKFLOW_SIGNAL: {"decision":"reject","reason":"missing error handling in payment flow"}
```

## Rules

1. Emit exactly one terminal signal per run.
2. Match the decision to `AI_WORKFLOW_ACTION_TYPE`.
3. Keep `reason` concise, specific, and actionable.
4. Emit the signal before ending the response.
5. For artifact-producing workflows, include artifact metadata in the HTTP or fallback payload.
