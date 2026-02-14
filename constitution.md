# 宪法（Project Constitution）

版本：v1.0  
状态：Active  
适用范围：`D:\project\zhanggui` 全仓

## 第一条：规则优先级

发生冲突时，按以下顺序执行：

1. 运行时真源：`workflow.toml`
2. 项目宪法：`constitution.md`
3. 稳定规范：`docs/standards/*`
4. 流程与协议：`docs/operating-model/*` + `docs/workflow/*`
5. 功能文档：`docs/features/*` + `docs/prd/*`

## 第二条：单一真源原则

1. 配置真源唯一：`workflow.toml`（根目录）。
2. 协作真源唯一：Outbox Issue（当前 backend 为 SQLite，`state/outbox.sqlite`）。
3. Claim 真源唯一：`assignee` 字段，不以文本 `/claim` 为事实。
4. 交付证据真源：Issue 时间线中的结构化 comment（Changes + Tests + Next）。
5. 禁止恢复或新增第二份 `workflow.toml` 到其它目录。

## 第三条：架构分层边界

1. `cmd/` 只负责 CLI 参数解析与调用用例，不写业务规则和 SQL。
2. `internal/bootstrap/` 只负责配置、数据库初始化、依赖装配。
3. `internal/domain/` 放纯规则与契约校验，不依赖基础设施。
4. `internal/usecase/` 负责业务流程编排，不直接依赖 `gorm.DB`，仅依赖 `internal/ports/`。
5. `internal/ports/` 放跨用例通用抽象接口（如 `Cache`、`OutboxRepository`、`UnitOfWork`）。
6. `internal/infrastructure/` 放外部依赖适配实现（SQLite、未来 Redis/GitHub）。
7. SQLite 模型仅放在 `internal/infrastructure/persistence/sqlite/model/`，不得冒充通用领域模型。
8. 组合根（wiring）必须使用 `go.uber.org/fx` 在 `internal/bootstrap/module.go` 完成；`cmd/` 不得 import `internal/infrastructure/*`。

## 第四条：Outbox 用例组织规范

`internal/usecase/outbox/` 目录必须保持“按职责聚合、按用例拆分”：

1. `service.go`：Service 与 DTO、共享错误。
2. `create_issue.go`：创建 Issue。
3. `claim_issue.go`：claim 与 assignee 流转。
4. `comment_issue.go`：回填 comment 与状态推进。
5. `close_issue.go`：关闭与证据校验。
6. `read_ops.go`：查询（list/show）。
7. `workflow_policy.go`：工作流策略与规则。
8. `persistence_helpers.go`：持久化辅助函数（通过 `OutboxRepository` 端口）。
9. `utils.go`：解析与基础工具函数。

## 第五条：Phase-1 工作流硬约束

以下规则是硬条件：

1. 进入执行态前必须已 claim（`assignee` 非空）。
2. 带 `needs-human` 的 issue 不得自动推进。
3. `DependsOn` 未满足时必须进入 `state:blocked`。
4. 关闭前必须有结构化证据：`Changes` 与 `Tests`。
5. `state:*` 缺失不阻塞开工，但推进时可由系统补齐。
6. 自然语言结果必须被规范化为结构化 comment 后再入库。

## 第六条：命名与标识规范

1. 协作主键：`IssueRef`（协议层对应 `issue_ref`）。
2. 执行主键：`RunId`（协议层对应 `run_id`）。
3. 本地 IssueRef canonical：`local#<issue_id>`。
4. 状态标签只允许：`state:todo|doing|blocked|review|done`。
5. 命令名与子命令名使用 kebab-case。

## 第七条：配置与环境规范

1. 默认配置文件：`configs/config.yaml`。
2. 默认数据库路径：`state/outbox.sqlite`。
3. 环境变量前缀：`ZG_`。
4. 本地运行态文件不得入库：`state/`、`*.sqlite`。
5. 任何配置变更必须同步更新 `workflow.toml` 与相关文档。

## 第八条：测试与交付门槛

1. 新增业务规则必须配套 `*_test.go`（至少一条正例 + 一条反例）。
2. 合并前必须通过：`go test ./...`。
3. CLI 行为变更需至少一条命令级烟测记录。
4. `docs/prd/tdd/phase-1-test-spec.md` 与 `docs/prd/tdd/contract-tests.md` 是验收规格真源，代码测试必须覆盖其硬约束。

## 第九条：日志与上下文规范

1. 除框架强制签名外，方法首参统一 `context.Context`。
2. 统一使用 `slog`，并写入最小字段：`component`、`command`。
3. 涉及协作流转时应补充：`issue_ref`、`run_id`（如有）。

## 第十条：变更治理

1. 影响本宪法任一条款的改动，必须同步修改 `constitution.md`。
2. 架构层改动需同时更新 `README.md` 与对应 `docs/` 说明。
3. 不允许“代码已变更，规范未更新”的长期漂移。

## 第十一条：事务与一致性

1. 多步写操作必须有统一事务边界：使用 `internal/ports/unit_of_work.go` 的 `UnitOfWork.WithTx(ctx, fn)`。
2. 仓储适配器必须从 `context.Context` 读取事务句柄（`TxFromContext`），确保同一事务可跨多次 repo 调用生效。
3. 禁止用例层自行创建/管理数据库事务对象（例如 `*gorm.DB.Begin()`）；用例只允许操作 `ports.UnitOfWork` + `ports.*Repository`。
4. 允许“拒绝操作但写入审计记录”的模式：当发生 blocked（如 `needs-human`、依赖未满足）时，必须先在事务内落库 `state:blocked` 与可审计 event，再向调用方返回错误（业务拒绝不等于回滚审计证据）。

## 附录：当前项目最小执行命令

```powershell
go run . init-db
go run . outbox create --title '[kind:task] demo' --body-file mailbox/phase-1-pilot-issue.md --label kind:task --label to:backend --label state:todo
go run . outbox claim --issue local#1 --assignee lead-backend --actor lead-backend --body-file mailbox/phase-1-pilot-comment-claim.md
go run . outbox comment --issue local#1 --actor lead-backend --state review --body-file mailbox/phase-1-pilot-comment-review.md
go run . outbox close --issue local#1 --actor lead-integrator --body-file mailbox/phase-1-pilot-comment-done.md
go run . outbox show --issue local#1
```
