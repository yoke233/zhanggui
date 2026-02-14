Role: <architect|backend|frontend|qa|integrator|recorder>
Repo: <logical repo key from workflow.toml>
IssueRef: <owner/repo#number | local#id>
RunId: <YYYY-MM-DD-role-seq | none>
SpecRef: <path-or-url | none>
ContractsRef: <contracts@sha-or-tag|none>
Action: <claim|update|proposal|accept|reject|blocked|unblock|done>
Status: <todo|doing|blocked|review|done>
ResultCode: <dep_unresolved|test_failed|ci_failed|review_changes_requested|env_unavailable|permission_denied|output_unparseable|stale_run|manual_intervention|none>
ReadUpTo: <last-comment-id|none>
Trigger: <stable-trigger-id>

# Note: `RunId` in template maps to protocol field `run_id` (see docs/standards/naming-and-ids.md)
# ResultCode rules (aligned with current implementation):
# - Auto-generated blocked events must use a non-`none` ResultCode.
# - Auto-generated non-blocked events use `ResultCode: none` by default.
# - Manual structured comments may omit `ResultCode`; if present, it must be one of the allowed enum values.

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
