# outbox usecase layout

- `service.go`: service struct + DTO definitions + shared errors.
- `create_issue.go`: create issue usecase.
- `claim_issue.go`: claim/assignee usecase.
- `unclaim_issue.go`: unclaim and reset to `state:todo`.
- `comment_issue.go`: append comment + workflow state transition usecase.
- `close_issue.go`: close issue usecase.
- `read_ops.go`: list/show query usecases.
- `lead_read_ops.go`: lead cache read helpers (active_run introspection).
- `lead_runner.go`: polling + cursor lead sync and worker dispatch orchestration.
- `lead_run_issue.go`: single-issue lead spawn/switch orchestration entry.
- `worker_runner.go`: worker runtime execution and work_result normalization.
- `workflow_profile.go`: workflow.toml runtime profile loading helpers.
- `workflow_policy.go`: workflow guards and normalization policy.
- `persistence_helpers.go`: repository-backed helpers for labels/events/dependency checks.
- `utils.go`: parsing and low-level value helpers.

Design rule:
- Usecase files should express behavior.
- Persistence helpers should not contain command formatting concerns.
- Cross-usecase abstractions are kept in `internal/ports`.
