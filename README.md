# zhanggui

已初始化为基于清洁架构的 Go 项目骨架，核心技术栈：

- `cobra`：CLI 框架（通过 `cobra-cli` 初始化）
- `viper`：配置管理（文件 + 环境变量）
- `gorm`：数据库访问层
- `github.com/glebarez/sqlite`：无 CGO SQLite 驱动
- `log/slog`：结构化日志（结合 `context.Context` 传递日志元信息）
- `bubbletea` + `lipgloss`：Lead 控制台 TUI

## 目录结构

```text
cmd/                                    # Cobra 命令层（interface adapters）
internal/bootstrap/                     # 组合根（配置、数据库初始化）
internal/domain/                        # 领域层占位
internal/usecase/                       # 用例层（含 outbox 用例与抽象接口）
internal/infrastructure/cache/          # Cache 适配层（当前 SQLite，后续可替换 Redis）
internal/infrastructure/persistence/
  sqlite/model/                         # SQLite 持久化模型（仅本地 backend）
configs/                                # 配置文件
```

## 运行方式

1. 安装依赖

```powershell
go mod tidy
```

2. 初始化数据库 schema

```powershell
go run . init-db
```

3. 查看帮助

```powershell
go run . --help
```

4. Phase-1 本地 outbox 命令（create -> claim -> comment -> close）

```powershell
go run . outbox create --title '[kind:task] demo' --body-file mailbox/phase-1-pilot-issue.md --label kind:task --label to:backend --label state:todo
go run . outbox claim --issue local#1 --assignee lead-backend --actor lead-backend --body-file mailbox/phase-1-pilot-comment-claim.md
go run . outbox comment --issue local#1 --actor lead-backend --state review --body-file mailbox/phase-1-pilot-comment-review.md
go run . outbox close --issue local#1 --actor lead-integrator --body-file mailbox/phase-1-pilot-comment-done.md
go run . outbox show --issue local#1
```

5. Phase-2 Lead 单次调度（sqlite outbox）

```powershell
go run . lead run --role backend --assignee lead-backend --workflow workflow.toml --once
```

6. Phase-2.1 PM 控制台（TUI，全局视图）

```powershell
go run . console pm --workflow workflow.toml --refresh-interval 5s
```

控制台快捷键：

- `↑/k`、`↓/j`：切换 issue
- `g`：刷新队列
- `c`：claim / unclaim
- `s`：spawn worker
- `w`：switch worker（强制新 run）
- `r`：normalize + reply
- `b`：blocked / unblock
- `D`：cleanup workdir（清理当前 ActiveRun 对应的 worktree）
- `x`：close issue
- `q`：退出

兼容说明：

- `go run . console lead ...` 仍可用，但已标记为 deprecated（建议迁移到 `console pm`）。

7. 运行 lint

```powershell
go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run
```

## 使用 `cobra-cli add` 扩展命令

本项目使用 `cobra-cli` 生成命令骨架，推荐继续用同一方式扩展：

1. 新增一级命令（默认挂到 `rootCmd`）

```powershell
go run github.com/spf13/cobra-cli@latest add issue
```

2. 新增子命令（通过 `--parent` 指定父命令变量名）

```powershell
go run github.com/spf13/cobra-cli@latest add create --parent issueCmd
```

3. 当前项目示例（已创建）

```powershell
go run github.com/spf13/cobra-cli@latest add init-db
```

说明：`--parent` 需要填写 Go 代码里的命令变量名，默认是 `rootCmd`。

## 配置

默认配置文件：`configs/config.yaml`  
可通过参数覆盖：

```powershell
go run . --config D:\path\to\config.yaml init-db
```

环境变量前缀：`ZG_`  
例如覆盖数据库路径：

```powershell
$env:ZG_DATABASE_DSN = 'state/custom.sqlite'
```

## 说明

当前已具备：

- Phase-1 本地闭环（Issue 创建、claim、结构化 comment、close）
- Phase-2 Lead 调度（polling/cursor/run_id）
- Phase-2.1 本地 Lead 控制台（TUI：队列、详情、动作、审计）

Outbox 的 Cache 走用例层抽象接口，当前使用 SQLite 适配器，未来可替换 Redis 而不改用例层。

## 日志与上下文约定

- 日志统一使用 `slog`，并通过 `context.Context` 注入与透传日志字段。
- 关键流程可通过 `logging.WithAttrs(...)` 在 context 中附加字段（例如 `command`、`run_id`、`issue_ref`）。
- 预留 OpenTelemetry 对接方式：可通过 context 注入 `trace_id`/`span_id`（`logging.WithTelemetry(...)`）。
- 除框架强制签名（如 `main`、`init`、GORM `TableName()`）外，项目方法统一以 `context.Context` 作为第一参数。
