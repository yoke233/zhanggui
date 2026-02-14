# Repository Guidelines

## Project Structure & Module Organization
`main.go` and `cmd/` contain the Cobra CLI entrypoint and commands.  
`internal/bootstrap/` is the composition root for startup concerns (`config`, `database`, `logging`, `app.go`).  
`internal/infrastructure/persistence/schema/` stores persistence schema models (currently `project_meta.go`).  
`internal/domain/` and `internal/usecase/` are reserved for business rules and application use cases as the project evolves.  
`configs/config.yaml` is the default runtime configuration.  
`docs/` and `openspec/` hold operating-model docs and change artifacts; treat them as workflow truth.

## Build, Test, and Development Commands
- `go mod tidy`: sync module dependencies.
- `go test ./...`: run all tests across packages.
- `go run . --help`: inspect CLI usage.
- `go run . init-db`: initialize/migrate schema in configured SQLite.
- `go run . --config configs/config.yaml init-db`: run with explicit config file.
- Add commands with Cobra CLI:
  - `go run github.com/spf13/cobra-cli@latest add issue`
  - `go run github.com/spf13/cobra-cli@latest add create --parent issueCmd`

## Coding Style & Naming Conventions
Run `gofmt -w` on changed Go files before committing.  
Use lowercase package names and keep command `Use` names kebab-case (for example, `init-db`).  
Prefer `context.Context` as the first parameter for bootstrap/data-layer methods.  
Use structured logging via `internal/bootstrap/logging` (`slog`), and attach contextual fields such as `component`, `command`, and `run_id`.

## Testing Guidelines
Use Go’s built-in `testing` package and name files `*_test.go`.  
Prefer table-driven tests for domain/usecase logic.  
Keep tests deterministic; for persistence tests, use temporary SQLite files or `:memory:`.  
Before opening a PR, run at least `go test ./...` and a quick CLI smoke test (`go run . init-db`).

## Commit & Pull Request Guidelines
Git history currently has a single commit (`Initial commit`), so no strict style is established yet.  
Adopt short imperative commit messages, ideally Conventional Commit prefixes (`feat:`, `fix:`, `chore:`).  
PRs should include: goal, key changes, validation commands run, schema/config impact, and related OpenSpec change path (for example, `openspec/changes/phase-1`).

## Security & Configuration Tips
Do not commit local runtime DB artifacts (for example, `.agents/state/*.sqlite`).  
Use environment variables (`ZG_DATABASE_DSN`, etc.) for local overrides instead of hardcoding machine-specific paths.  
When changing schema files, keep migrations idempotent and backward-safe.
