# [kind:<task|bug|question|proposal|blocker>] <Short Title>

## Meta
- SpecRef: <path-or-url | none>
- ContractsRef: <contracts@sha-or-tag | none>
- Repo: <logical repo key from workflow.toml>
- Role: <architect|backend|frontend|qa|integrator|recorder>
- Priority: <p0|p1|p2|p3 | none>

## Goal
<What outcome is expected>

## Scope
- In scope:
  - <item>
- Out of scope:
  - <item>

## Dependencies
- DependsOn:
  - <owner/repo#id | local#id | PR url | git:<sha> | none>
- BlockedBy:
  - <owner/repo#id | local#id | none>

## Acceptance Criteria
- [ ] <observable condition 1>
- [ ] <observable condition 2>
- [ ] Evidence link(s) provided in comments

## Start Conditions
Hard (must):

- [ ] Issue is open
- [ ] Assignee is set (claimed)
- [ ] No `needs-human` label
- [ ] All `DependsOn` are resolved

Soft (recommended):

- [ ] `state:doing` present (for queue clarity; should not block Phase 1)

## Routing
- ToRoles:
  - <to:backend>
  - <to:qa>

Note: `to:<role>` is an Outbox label. When creating the issue/issue, apply these as actual labels (not only in the body).

## Notes
<Free-form discussion context>
