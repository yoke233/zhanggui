# Pull Request Template (Workflow V1.1)

Use this as a *recommended* PR description structure for worker outputs.

Goal: make PRs traceable to the Issue, and make evidence easy to audit.

## Meta
- IssueRef: <owner/repo#number | local#id>   # MUST (Issue)
- Repo: <contracts|backend|frontend|...>
- Role: <architect|backend|frontend|qa|integrator|recorder>
- ContractsRef: <contracts@sha-or-tag | none>

## Summary
- <what this PR changes, in 1-3 bullets>

## Scope
- In scope:
  - <item>
- Out of scope:
  - <item>

## Tests / CI
- Command: <cmd|none>
- Result: <pass|fail|n/a>
- Evidence: <CI url|log url|none>

## Notes
- <review guidance, risks, rollout plan, etc.>
