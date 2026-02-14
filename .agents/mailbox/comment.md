Role: <architect|backend|frontend|qa|integrator|recorder>
Repo: <logical repo key from workflow.toml>
IssueRef: <owner/repo#number | local#id>
SpecRef: <path-or-url | none>
ContractsRef: <contracts@sha-or-tag|none>
Action: <claim|update|proposal|accept|reject|blocked|unblock|done>
Status: <todo|doing|blocked|review|done>
RunId: <YYYY-MM-DD>-<role>-<seq> | none
ReadUpTo: <last-comment-id|none>
Trigger: <workrun:<RunId> | manual:<id> | none>

# Rules:
# - Lead-normalized worker results MUST use `Trigger: workrun:<RunId>`.
# - If no PR is available, set `PR: none` and `Commit: git:<sha>`.
# - Only `run_id == active_run_id` may advance state; stale runs are audit-only.

Summary:
- <what changed or what was decided>

Changes:
- PR: <url|none>
- Commit: <git:<sha>|url|none>

Tests:
- Command: <cmd|none>
- Result: <pass|fail|n/a>
- Evidence: <url|none>

Review:
- Verdict: <review:approved|review:changes_requested|none>
- Evidence: <url|none>

BlockedBy:
- <owner/repo#id | local#id | none>

OpenQuestions:
- @<role-or-user> <question>

Next:
- @<role-or-user> <next action>

CloseChecklist:
- [ ] Changes 完整（无 PR 时 `PR: none` 且 `Commit: git:<sha>`）
- [ ] Tests 字段完整（可追溯或明确 n/a）
- [ ] Review 判定已写明 `review:approved` 或 `review:changes_requested`
- [ ] 若为 worker 规范化写回，Trigger 使用 `workrun:<RunId>`
