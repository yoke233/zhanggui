Role: <architect|backend|frontend|qa|integrator|recorder>
Repo: <logical repo key from workflow.toml>
IssueRef: <owner/repo#number | local#id>
RunId: <YYYY-MM-DD-role-seq | none>
SpecRef: <path-or-url | none>
ContractsRef: <contracts@sha-or-tag|none>
Action: <claim|update|proposal|accept|reject|blocked|unblock|done>
Status: <todo|doing|blocked|review|done>
ReadUpTo: <last-comment-id|none>
Trigger: <stable-trigger-id>

# Note: `RunId` in template maps to protocol field `run_id` (see docs/standards/naming-and-ids.md)

Summary:
- <what changed or what was decided>

Changes:
- PR: <url|none>
- Commit: <git:<sha>|url|none>

Tests:
- Command: <cmd|none>
- Result: <pass|fail|n/a>
- Evidence: <url|none>

BlockedBy:
- <owner/repo#id | local#id | none>

OpenQuestions:
- @<role-or-user> <question>

Next:
- @<role-or-user> <next action>
