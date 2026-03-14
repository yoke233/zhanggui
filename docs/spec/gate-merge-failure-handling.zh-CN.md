# Gate PR 合并失败处理机制

> 状态：已实现
>
> 最后按代码核对：2026-03-14
>
> 对应实现：`internal/application/flow/gate.go`、`internal/application/flow/gate_merge.go`
>
> 关联文档：[gate-human-intervention.zh-CN.md](gate-human-intervention.zh-CN.md)（信号模型、评估链）

## 概述

Gate 评估通过后，若配置了 `merge_on_pass: true`，引擎会尝试自动合并 PR。合并失败时根据失败类型分流处理，核心区分是 **dirty（文件冲突）** 和 **非 dirty** 两条路径。

## 合并触发条件

Gate action 的 `config` 字段控制合并行为：

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `merge_on_pass` | `false` | 是否在 gate 通过后自动合并 PR |
| `merge_method` | `"squash"` | 合并策略：squash / merge / rebase |

合并前需解析 PR 号（优先从 gate run 结果元数据取，回退到上游 action 的 run 结果扫描）。

## 失败分流决策树

```
applyGatePass()
  │
  ├── mergePRIfConfigured()
  │     │
  │     ├── merge_on_pass: false ──→ 跳过合并，直接 Gate → done
  │     │
  │     └── merge 失败 ──→ handleMergeConflictBlock(err)
  │           │
  │           ├── MergeError + MergeableState == "dirty"
  │           │     ① recordMergeConflict(): 写入 SignalContext (summary="merge_conflict")
  │           │     ② 发布 EventGateAwaitingHuman
  │           │     ③ Gate → blocked
  │           │     ④ return nil（不进重做循环，等待人工解决冲突）
  │           │
  │           └── 其他情况（behind / unstable / blocked / draft / 通用错误）
  │                 ① formatMergeFailureFeedback(): 按 provider + state 生成带 hint 的反馈
  │                 ② defaultGateResetTargets(): 确定要重置的上游 action IDs
  │                 ③ processGateReject(): 进入标准重做循环
  │                 ④ 重做次数超限 → Gate → blocked
```

### 为什么 dirty 单独处理

`dirty` = 文件级冲突（两个分支修改了同一文件的同一区域），agent 无法自动解决。如果让 agent 重做，只会浪费重做次数而不解决问题。因此直接阻塞等人工 merge。

### 非 dirty 为什么走重做

| MergeableState | 含义 | agent 可修复性 |
|----------------|------|---------------|
| `behind` | 分支落后于 base | 可以，rebase/merge base |
| `unstable` | CI 检查未通过 | 可以，修复代码问题 |
| `blocked` | 保护规则阻止 | 可能，视规则而定 |
| `draft` | 草稿 PR | 可以，标记为 ready |

这些情况 agent 有合理概率自行修复，所以给重做机会，受 `max_rework_rounds`（默认 3）限制。

## 反馈模板系统

合并失败时，引擎通过 `formatMergeFailureFeedback()` 生成结构化反馈，写入上游 action 的 `SignalFeedback`，让 agent 知道为什么失败以及如何修复。

### 模板变量

```go
type mergeReworkTemplateVars struct {
    PRNumber       int    // PR 编号
    PRURL          string // PR 链接
    Provider       string // github / codeup / ...
    MergeableState string // dirty / behind / unstable / ...
    Message        string // 原始错误信息
    Hint           string // 针对该 state 的修复建议
}
```

### Provider 差异化提示

通过 `PRFlowPrompts` 配置，支持按 provider 定制不同的提示语：

```
PRFlowPrompts
  ├── Global     (默认提示)
  ├── GitHub     (GitHub 专用覆盖)
  └── CodeUp     (Codeup 专用覆盖)
```

每个 provider 可定制：
- `MergeReworkFeedback`: Go template 格式的反馈模板
- `MergeStates.Dirty/Behind/Blocked/Unstable/Draft/Default`: 各状态的修复 hint

查找优先级：provider 专用 → 全局默认 → 硬编码兜底。

### 反馈写入路径

合并失败反馈通过 `recordGateRework()` 写入上游 action 的 `SignalFeedback`：

```
SignalFeedback {
    ActionID:       upstream_action_id,
    Source:         "system",
    Summary:        "merge failed: ...",
    Content:        "Reason: ...\nPR: ...\nHint: ...",
    SourceActionID: gate_action_id,
    Payload: {
        merge_error, pr_number, pr_url,
        mergeable_state, merge_provider, merge_action_hint
    }
}
```

Agent 在重做时通过 `step_context` MCP 工具读取 rework_history，获得完整的失败原因和修复建议。

## 冲突阻塞记录

dirty 冲突阻塞时，通过 `recordMergeConflict()` 在 gate action 上写入 `SignalContext`：

```
SignalContext {
    ActionID: gate_action_id,
    Summary:  "merge_conflict",
    Content:  "{reason}\n\nMerge Error: {err}\nAction: {hint}",
    Payload:  { merge_error, pr_url, mergeable_state, merge_action_hint }
}
```

同时发布 `EventGateAwaitingHuman` 事件，前端可据此展示阻塞通知。

## 测试覆盖

| 测试文件 | 测试场景 |
|----------|----------|
| `gate_merge_conflict_test.go` | dirty → blocked + SignalContext + EventGateAwaitingHuman |
| `gate_merge_conflict_test.go` | behind → 不处理（返回 false，走重做） |
| `gate_merge_conflict_test.go` | 通用错误 → 不处理 |
| `gate_feedback_test.go` | formatMergeFailureFeedback 生成正确的模板输出 |
| `gate_feedback_test.go` | 自定义模板覆盖默认 |
| `gate_feedback_test.go` | provider 专用 vs 通用提示 |
