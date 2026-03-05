# AI-Workflow 精简与抽象建议

> 基于源码逆向分析，按优先级排序的实施建议。
> 生成时间：2026-03-05

---

## 优先级总览

| 优先级 | 项目 | 工作量 | 风险 | 收益 |
|--------|------|--------|------|------|
| P0 | 删除 18 个死事件 + RunEvent 类型 | 小 | 极低 | 认知负荷减半 |
| P0 | 删除 Aggregator/ReviewOrchestrator/Reviewer 遗留层 | 小 | 低 | 调用链简化 |
| P1 | 权限类型去重（CapabilitiesConfig → ClientCapabilities 统一） | 中 | 低 | 消除 factory 转换样板 |
| P1 | Store 接口拆分 | 中 | 低 | 依赖精确化 |
| P2 | Run-Issue 单向引用 | 中 | 中 | 数据一致性 |
| P2 | 删除 DependsOn/Blocks 字段 | 小 | 低 | 清理废弃字段 |
| P3 | ConfigLayer 泛型化 | 大 | 中 | 减少 ~28 个镜像类型 |
| P3 | 运行模板可配置化 | 中 | 低 | 用户扩展性 |

---

## P0：零风险删除

### 1. 事件系统精简（37 → ~10）

**删除 18 个死事件**（发布后从未被消费）：
- `StageComplete`, `StageFailed`, `AgentOutput`, `RunStuck`
- `RunActionRequired`, `RunResumed`, `ActionApplied`
- `TeamLeaderFilesChanged`
- `IssueDone`, `IssueFailed`, `IssueDecomposed`
- 全部 6 个 GitHub 事件：`GitHubWebhookReceived`, `GitHubIssueOpened`, `GitHubIssueCommentCreated`, `GitHubPullRequestReviewSubmitted`, `GitHubPullRequestClosed`, `GitHubReconnected`

**删除 RunEvent 类型 + run_events 表**：
- `SaveRunEvent()` 在业务代码中从未被调用（仅测试）
- 持久化统一用 `ChatRunEvent`

**单消费者事件改回调**：
- `EventStageStart` → status_syncer 直接回调
- `EventHumanRequired` → status_syncer 直接回调

**保留的核心事件**（约 10 个）：
- 调度必须：`EventRunDone`, `EventRunFailed`
- WS 推送：`EventRunStarted`, `EventRunUpdate`, `EventRunCompleted`, `EventRunCancelled`
- Issue 生命周期：`EventIssueCreated`, `EventIssueDecomposing`
- TL 流式：`EventTeamLeaderThinking`
- 管理：`EventAdminOperation`

### 2. 评审系统去壳（6 接口 9 结构体 → 3 接口 4 结构体）

**删除**：
- `ReviewOrchestrator`（仅是 `TwoPhaseReview` 的兼容包装壳）
- `Reviewer` 接口（遗留，通过 `reviewerAdapter` 桥接后不再直接使用）
- `Aggregator` 接口 + `defaultIssueAggregator`（`Decide()` 从未被调用）
- `reviewerAdapter`（桥接层随之消失）
- `toTwoPhaseReview()` 转换方法

**保留**：
- `TwoPhaseReview`（核心执行引擎）
- `ReviewStore` 接口（持久化）
- `core.ReviewGate` 接口（3 个插件实现，真正多态）
- `DemandReviewer` 接口 + `defaultDemandReviewer`

---

## P1：结构优化

### 3. 权限/能力类型去重（12 → 7）

**完全重复对（直接统一）**：
- `config.CapabilitiesConfig` ↔ `acpclient.ClientCapabilities`（100% 重叠）
- `config.PermissionRule` ↔ `acpclient.PermissionRule`（100% 重叠）

**高度重复对（合并并扩展）**：
- `config.SessionConfig` ↔ `acpclient.SessionPolicy`（80%，后者多 `SessionIdleTTL`）

**方案**：在 `internal/core/` 定义权威类型，config 和 acpclient 都引用。factory.go 转换从 40 行缩至 ~5 行。

### 4. Store 接口拆分

当前 `core.Store` 有 35+ 方法。按聚合根拆分：

```go
type ProjectStore interface { ... }      // 5 方法
type RunStore interface { ... }          // 7 方法
type IssueStore interface { ... }        // 10+ 方法
type CheckpointStore interface { ... }   // 4 方法
type EventStore interface { ... }        // 2 方法

type Store interface {
    ProjectStore; RunStore; IssueStore; CheckpointStore; EventStore
    Close() error
}
```

每个消费者只依赖需要的切片，测试 mock 更轻量。

---

## P2：数据模型变更

### 5. Run-Issue 单向引用

当前双向引用 `Run.IssueID` + `Issue.RunID` 导致：
- `GetIssueByRun()` 需 2 层 fallback 查询
- `buildRunFromIssue()` 复制 5 个字段

**方案**：保留 `Run.IssueID`，删除 `Issue.RunID`。查询走 `SELECT issue_id FROM runs WHERE id=?`。

### 6. 删除废弃字段

- `Issue.DependsOn` / `Issue.Blocks` — V2 中已弃用，仍占数据库列
- `issues` 表的 `depends_on TEXT DEFAULT '[]'` 和 `blocks TEXT DEFAULT '[]'`

---

## P3：架构级优化

### 7. ConfigLayer 泛型化

28 个配置类型各有指针化 `*Layer` 镜像（用于多层合并），实际类型数翻倍到 ~56。可用泛型或 reflect 替代手写 merge。

### 8. 运行模板可配置化

4 个模板 (full/standard/quick/hotfix) 的阶段序列硬编码在 Go 代码中。迁移至 `configs/defaults.yaml` 允许用户自定义。
