# AI-Workflow 系统优化方案（V2，允许破坏性变更）

生成日期：2026-03-05
适用前提：允许破坏性改动；不迁移历史数据。

---

## 1. 目标与边界

### 1.1 目标

1. 把“多套调度 + 分散状态流转 + 松散事件约定”收敛为**单一编排内核**。
2. 明确 API/编排/存储分层，降低 `cmd` 与 `web` 对业务细节的耦合。
3. 在不保留旧数据的前提下，重做数据库模型，去掉历史兼容负担。
4. 给前端提供稳定、单语义的契约（仅 issue 命名，不再 plan/run 兼容漂移）。

### 1.2 非目标

1. 不做“平滑兼容旧 DB schema”。
2. 不保留旧接口别名（如 plan 别名、大小写历史路径等）。
3. 不优先做最小变更；优先做长期可维护性。

---

## 2. 对既有两份分析的吸收结论

## 2.1 采纳（合理）

1. **Issue 状态机需要强制化**：当前仅校验状态值合法，不校验转换合法。
2. **双调度器应统一**：当前同时存在 `engine.Scheduler` 与 `teamleader.DepScheduler`，控制面割裂。
3. **Stage 默认配置源要统一**：当前 `defaultStageConfig` 与 `schedulerDefaultStageConfig` 语义不一致。
4. **Web 层应瘦身**：HTTP 层不应直接持有 ACP/TL 细节。
5. **事件模型需要类型化**：减少 `map[string]string` 的隐式约定。

## 2.2 不采纳（需修正）

1. “可直接删除 run_events / RunEvent”不成立；当前运行时和查询链路仍依赖。
2. “IssueDone/IssueFailed/IssueDecomposed 是死事件”不成立；存在发布与消费。
3. “P0 删除评审壳层低风险”不成立；启动装配路径仍直接依赖 orchestrator/reviewer 结构。


## 2.3 新增（本轮排查补充）：P0/P1 立即修复清单

说明：以下项不与长期重构冲突，且属于“当前可直接导致启动失败或契约漂移”的前置门禁，应优先于大规模目录重构执行。

### 2.3.1 P0：配置模板与运行时 schema 对齐

现状问题：
1. `ai-flow config init` 默认复制 `configs/defaults.yaml`。
2. 运行时配置加载启用严格模式（unknown field fail-fast）。
3. 当前模板与 `internal/config/types.go` 的字段不一致（如 `run` vs `Run`、`pipeline` vs `Run`、`max_project_pipelines` vs `max_project_Runs`，以及多处未定义字段）。

修复动作：
1. 统一配置键名与结构，保证 `config init` 生成文件可被 `LoadGlobal` 成功解析。
2. 增加回归测试：`config init -> loadBootstrapConfig/LoadGlobal` 必须通过。
3. 约束默认配置单一来源，避免 `config.Defaults()` 与 `configs/defaults.yaml` 再次分叉。

验收标准：
1. 全新目录执行 `ai-flow config init` 后，`ai-flow project list` 不报 yaml unknown-field 错误。
2. CI 中新增“模板可加载”测试，防止回归。

### 2.3.2 P0：SQLite 迁移补齐（覆盖旧库升级）

现状问题：
1. 当前迁移主要验证“新库建表 + 幂等”，对“旧库缺列升级”覆盖不足。
2. 已出现旧库中 `run_events.run_id` 缺失导致启动失败的真实案例。
3. 运行时代码依赖的列（例如 `chat_sessions.agent_session_id`）在迁移路径上缺少显式补齐策略。

修复动作：
1. 增加列级自检迁移（按 `hasColumn` 补齐关键列与索引），而不是仅依赖 `CREATE TABLE IF NOT EXISTS`。
2. 为关键历史表建立升级迁移用例（构造旧 schema -> `applyMigrations` -> 验证列/索引齐备）。
3. 对启动期报错补充明确错误提示（标识缺列、建议命令）。

验收标准：
1. 旧库可直接升级并启动，不再出现 `no such column`。
2. 迁移测试覆盖“老版本缺列场景”，并纳入默认测试集。

### 2.3.3 P1：项目级配置能力决策（接线或删除）

现状问题：
1. `LoadProject/ProjectConfigPath` 已定义，但未在实际运行链路中接入。
2. 导致“能力已声明、行为不可达”的设计缺口，增加认知负担。

修复动作（二选一）：
1. 接线：在 bootstrap 中合并“全局配置 + 项目配置层”，明确覆盖优先级。
2. 删减：若短期不支持项目级配置，删除相关入口与说明，避免误导。

验收标准：
1. 文档、代码、运行行为三者一致，不存在“死能力”。

### 2.3.4 P1：前端运行状态与后端状态机收敛

现状问题：
1. 后端 Run 状态已收敛为 `queued/in_progress/action_required/completed + conclusion`。
2. 前端类型仍保留 `created/running/waiting_review/done/failed/timeout` 等旧值，形成契约噪音。
3. `Plan*` 类型别名仍保留“逐步删除”痕迹。

修复动作：
1. 前端类型改为只保留当前后端有效状态；历史值走迁移层或直接删除。
2. 清理 `Plan*` 别名与相关兼容注释，统一 issue/run/session 语义。
3. 增加 API 契约测试（前端类型断言 + 关键接口样本）。

验收标准：
1. 前后端状态枚举一致，前端不再持有旧状态兜底分支。
2. 代码中不再出现“逐步删除”的兼容别名。

### 2.3.5 推荐执行顺序（本周）

1. `P0-配置`：先修模板与 schema 对齐（最快恢复新环境可用性）。
2. `P0-迁移`：补齐旧库升级路径（最快恢复已有环境可用性）。
3. `P1-前端契约`：删旧状态与别名（降低后续改造干扰）。
4. `P1-项目配置`：明确接线或删除（清理无效抽象）。

---

## 3. 目标架构

```text
cmd/ai-flow
  └─ bootstrap (依赖注入)

internal/
  ├─ domain/                 # 纯领域模型 + 状态机
  │   ├─ issue.go
  │   ├─ run.go
  │   ├─ state_machine.go
  │   └─ events.go
  │
  ├─ orchestrator/           # 唯一编排内核
  │   ├─ scheduler.go        # 唯一调度器
  │   ├─ lifecycle.go        # 状态转换与事件统一入口
  │   ├─ review.go
  │   └─ execution.go
  │
  ├─ agent/                  # ACP/A2A 交互层
  │   ├─ acp_client.go
  │   ├─ a2a_bridge.go
  │   └─ role_resolver.go
  │
  ├─ api/                    # 仅 HTTP/WS 序列化与鉴权
  │   ├─ server.go
  │   ├─ handlers_*.go
  │   └─ ws.go
  │
  ├─ store/                  # 存储抽象与实现
  │   ├─ repository.go
  │   └─ sqlite/
  │
  └─ github/                 # 保留，改为订阅 typed event
```

关键原则：
1. API 不直接 import `teamleader/engine` 细节。
2. 只有 orchestrator 可以改 Issue/Run 状态。
3. 所有状态变更必须通过统一 Transition API。

---

## 4. 核心设计

## 4.1 单一调度器

删除并合并：
1. 删除 `engine.Scheduler` 与 `teamleader.DepScheduler` 的并行运行模型。
2. 新建 `orchestrator.Scheduler`：
   - 输入：Issue ready 队列
   - 输出：Run 执行任务
   - 统一并发：`max_global_runs` + `max_project_runs`
   - 统一抢占：单点 CAS (`TryMarkRunInProgress` 风格)

好处：
1. 避免两套 loop 的竞争与重复计数。
2. 所有 Run 启动路径一致，超时/重试策略一致。

## 4.2 状态机强制化

### Issue 状态机

新增 `ValidateIssueTransition(from,to)`，并在所有变更点强制调用。

建议状态（保留当前业务语义）：
- `draft`
- `reviewing`
- `queued`
- `ready`
- `executing`
- `decomposing`
- `decomposed`
- `done`
- `failed`
- `abandoned`
- `superseded`

### Run 状态机

保留双轴：
- `status`: `queued/in_progress/action_required/completed`
- `conclusion`: `success/failure/timed_out/cancelled`

新增 `TransitionRun()` 统一入口，禁止裸写 `Run.Status`。

## 4.3 状态变更与事件绑定

新增统一入口：
- `IssueLifecycle.Transition(ctx, issueID, toStatus, reason, meta)`
- `RunLifecycle.Transition(ctx, runID, toStatus, conclusion, reason, meta)`

行为保证：
1. 先校验状态转换。
2. 再落库（issue/run + change log）。
3. 最后发布 typed event。
4. 三者失败策略一致（事务或补偿）。

## 4.4 事件模型（Typed Event）

用 Envelope + Typed Payload 替代散乱 `map[string]string`：

- `EventEnvelope`
  - `type`
  - `scope` (`issue/run/session/system`)
  - `scope_id`
  - `project_id`
  - `occurred_at`
  - `payload_json`

- Payload 结构按事件类型定义（Go struct），通过注册表解码。

目标：
1. 事件字段稳定。
2. 减少发布/消费方“猜字段名”。
3. 更容易做 API 输出与前端类型生成。

---

## 5. 数据模型（Schema V4，破坏性重建）

## 5.1 策略

1. **不做旧版本迁移**。
2. 启动时检测 schema version，不匹配则直接重建数据库。
3. 提供 `ai-flow reset-db` 明确入口（删除 `.ai-workflow/data.db*` 后重建）。

## 5.2 表结构建议

保留：
1. `projects`
2. `issues`
3. `runs`
4. `checkpoints`
5. `review_records`
6. `issue_changes`

重做：
1. 合并 `run_events` + `chat_run_events` 为 `events`（统一事件流）。
2. 删除 `issues.depends_on` / `issues.blocks` JSON 列，改成关系表 `issue_edges`：
   - `issue_id`
   - `target_issue_id`
   - `edge_type` (`depends_on|blocks|parent_child`)

可删：
1. `issues.run_id`（改为仅 `runs.issue_id` 单向引用）。

---

## 6. API 与前端契约收敛

## 6.1 REST

目标仅保留 issue 语义，不再有 plan/run 历史兼容层：
1. Issue 写接口：`/api/v3/projects/{project_id}/issues/...`
2. Run 读接口：`/api/v3/runs/...`
3. Event 读接口：
   - `/api/v3/runs/{id}/events`
   - `/api/v3/sessions/{id}/events`
   - 本质同一 `events` 表按 scope 查询

## 6.2 WS

仅保留必要订阅：
1. `subscribe_issue`
2. `subscribe_run`
3. `subscribe_session`

消息统一 envelope 格式，字段稳定。

## 6.3 前端

1. 彻底移除 `plan` 别名方法。
2. RunView 维持只读，不回归 run action UI。
3. 前端类型直接基于 v3 契约生成（或至少单源定义）。

---

## 7. 破坏性变更清单

1. DB 全量重建，旧数据不可用。
2. 删除 v1/v2 的历史兼容接口与别名，切 v3。
3. 删除 `plan` 命名兼容路径。
4. 删除 Issue 上的 JSON 依赖字段（改关系表）。
5. 删除裸写状态入口，所有调用改为 lifecycle transition。

---

## 8. 分阶段实施（建议）

## 阶段 A：内核收敛（1-2 周）

1. 引入 `domain/state_machine.go` 与 `orchestrator/lifecycle.go`。
2. 把 Issue/Run 状态写入改为统一 transition API。
3. 保持现有 API，不改协议。

验收：
1. 无任何直接 `issue.Status = ...`、`run.Status = ...` 的业务裸写。
2. 单测覆盖每个非法转换。

## 阶段 B：调度统一（1-2 周）

1. 合并两套 scheduler 为 `orchestrator.Scheduler`。
2. 删除 `defaultStageConfig` 双源，改单源配置。

验收：
1. 仅一个调度 loop 负责发车。
2. 同模板 Run 的 timeout/idleTimeout 行为一致。

## 阶段 C：存储重建（1 周）

1. 落地 Schema V4（events + issue_edges）。
2. 增加 `reset-db` 命令并修改启动检测逻辑。

验收：
1. 干净库可一键启动。
2. Event 查询链路（REST/WS/MCP）全部走新表。

## 阶段 D：API/前端收敛（1-2 周）

1. 发布 `/api/v3`，移除历史兼容端点。
2. 前端 API client 与类型统一切换。

验收：
1. 前端不再包含 `plan`/兼容 run 方法。
2. Board/Run/Chat/A2A 四个主视图通过回归。

## 阶段 E：模块落盘（1 周）

1. `web -> api` 包重命名与依赖裁剪。
2. `cmd/ai-flow/commands.go` 缩至纯装配。

验收：
1. HTTP 层不直接 import orchestrator 细节实现包。
2. bootstrap 文件行数显著下降（目标 < 300 行）。

---

## 9. 度量指标（完成定义）

1. 状态变更入口数：
   - 目标：Issue/Run 各 1 个公开入口。
2. 调度器数量：
   - 目标：1 个。
3. 事件存储表：
   - 目标：1 个（`events`）。
4. API 命名一致性：
   - 目标：仅 issue/run/session 主语，无 plan/run 历史兼容。
5. 模块耦合：
   - 目标：`api` 不直接依赖 `teamleader/engine` 旧实现包。

---

## 10. 第一批可立即执行任务（本周）

1. 建立 `domain` + `orchestrator` 目录骨架与接口。
2. 先实现 `ValidateIssueTransition` + `IssueLifecycle.Transition`。
3. 把 Manager/Scheduler/Decompose/ChildCompletion 的状态写入迁移到 lifecycle。
4. 增加状态机回归测试（非法转换、回滚、失败策略）。

这 4 项完成后，再进入调度器合并和 schema 重建。
