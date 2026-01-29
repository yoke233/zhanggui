# Repository Guidelines

## Project Structure & Module Organization
- `cmd/`: executable entrypoints (`taskctl`, `zhanggui`).
- `internal/`: Go implementation (AG-UI handler, task bundle/approval, gateway, CLI commands).
- `contracts/`: versioned integration contracts and JSON schemas (e.g. `contracts/ag_ui/*`).
- `docs/`: spec-first design docs (start with `docs/README.md` and `FILE_STRUCTURE.md`).
- `fs/`: runtime data (ignored by git); do not commit.

## Build, Test, and Development Commands
- `go test ./...`: run all tests.
- `go test ./... -run TestName -count=1`: run a focused test without cache.
- `go build ./cmd/taskctl`: build the `taskctl` CLI.
- `go build ./cmd/zhanggui`: build the `zhanggui` server.
- `go run ./cmd/zhanggui serve --print-endpoints`: start local AG-UI demo server.
- `go run ./cmd/taskctl --help`: explore `run`, `inspect`, and `pack`.

## Coding Style & Naming Conventions
- Format with `gofmt` (`go fmt ./...`) before pushing (Go indentation is tabs).
- Follow standard Go naming: `CamelCase` exports, `mixedCase` locals, `lowercase` packages.
- Keep new code under `internal/<area>/`; add a new `cmd/<tool>/main.go` only for new binaries.

## Testing Guidelines
- Tests live next to code as `*_test.go`; use `TestXxx` naming.
- Prefer black-box tests (`package foo_test`) when validating public behavior (see `internal/agui`).

## Commit & Pull Request Guidelines
- Git history is currently minimal; use clear, scoped messages (e.g., `feat(taskctl): add pack flag`).
- PRs should include: what/why, how to test, linked issue/doc section, and updates to `contracts/` or `docs/` when behavior/protocol changes.

## Security & Configuration Tips
- Never commit secrets or runtime outputs (`.env*`, `fs/`, logs).
- Config loads via `--config` or `config.yaml` from `.` / `~/.taskctl` / `~/.zhanggui`; env prefixes are `TASKCTL_` and `ZHANGGUI_`.
