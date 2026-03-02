# DAG 转换补充规则（P2a）

> 本文档是 [spec-secretary-layer.md](../../spec-secretary-layer.md) 的 **补充决策文档**，只记录对 spec 的覆盖、细化或新增规则。
> DAG 基础架构（数据结构、校验、调度流程、失败策略、崩溃恢复）见 spec Section 六。
> 审核架构（Reviewer/Aggregator/循环）见 spec Section 三。
>
> 版本：V1（最小可执行）
> 日期：2026-03-01

---

## 1. 覆盖 spec 的已确认决策

以下规则覆盖 spec 中的默认值，已在讨论中确认：

| 规则 | spec 原值 | 本文确认值 | 原因 |
|------|----------|-----------|------|
| AI 审核最大轮数 | `MaxRounds: 3` | `max_rounds = 2` | 减少 token 消耗，2 轮足够定位绝大多数问题 |
| 审核通过后流程 | approve → 直接 DAG 调度 | approve → `waiting_human(final_approval)` → 人工确认 → DAG 调度 | 人工兜底，防止 AI 误判 |

---

## 2. 强门禁状态机（spec Section 三 补充）

spec 描述了审核循环（Reviewer → Aggregator → approve/fix/escalate），本文补充完整的端到端状态流转：

```
draft
  │ 用户触发 /plan
  ▼
reviewing ◄─────────────────────────────┐
  │ 3 Reviewer 并行 + Aggregator        │
  │                                     │
  ├─ fix → 替换 TaskPlan ──► reviewing  │ (消耗 1 轮)
  │                                     │
  ├─ approve ──► waiting_human          │
  │   wait_reason=final_approval        │
  │   ├─ 人工通过 → executing           │
  │   └─ 人工驳回(必填反馈) ────────────┘ (Secretary 重生成)
  │                                     │
  └─ 超限(>2轮) ──► waiting_human       │
      wait_reason=feedback_required     │
      └─ 人工反馈(必填) ───────────────┘ (Secretary 重生成)
```

### 人工反馈格式（两段式必填）

适用于 `wait_reason = feedback_required` 和 `final_approval` 驳回两种场景：

1. **问题类型**（必选枚举）：`missing_node` / `cycle` / `self_dependency` / `bad_granularity` / `coverage_gap` / `other`
2. **具体说明**（必填，最少 20 字）
3. **期望修正方向**（可选）

后端标准化输入：

```json
{
  "plan_id": "plan-xxx",
  "revision_from": 2,
  "wait_reason": "feedback_required",
  "feedback": {
    "category": "cycle",
    "detail": "task-a -> task-b -> task-c -> task-a 构成闭环",
    "expected_direction": "拆成先后两层并删除冗余边"
  },
  "ai_review_summary": {
    "rounds": 2,
    "last_decision": "escalate",
    "top_issues": ["DAG_CYCLE_DETECTED"]
  }
}
```

### Secretary 重生成约束

重生成 prompt 必须包含四段输入：

1. 原始对话摘要
2. 上一版 TaskPlan（完整 JSON）
3. AI review 问题摘要（结构化）
4. 人类反馈（标准化 JSON）

硬约束：保持需求语义不丢失、优先修正依赖关系和任务边界、输出严格 JSON。

---

## 3. 传递约简（spec Section 六 补充）

spec 的 DAG 校验只包含环检测/缺失引用/自依赖。本文新增传递约简步骤：

- 若存在 `A → B` 且 `B → C`，同时存在 `A → C`，则 `A → C` 为冗余边
- 删除冗余边，保证可达关系不变
- 执行时机：DAG 校验通过后、调度开始前
- 目标：最大化并行度

---

## 4. DAG 校验失败的衔接（spec Section 六 补充）

spec 的 DAG.Validate() 返回 error 后未定义后续处理。本文补充：

1. 校验失败 → Plan 进入 `waiting_human`（`wait_reason=feedback_required`）
2. 用户提交两段式反馈（同 Section 2 格式）
3. Secretary 重生成新版 → 重新进入 AI review 强门禁

错误码：`DAG_CYCLE_DETECTED` / `DAG_MISSING_NODE` / `DAG_SELF_DEPENDENCY`

---

## 5. `skip` 策略下 hard/weak 依赖判定（spec Section 六 补充）

spec 的 skip 策略只描述了"唯一上游 → skipped"的简单规则。本文细化为 LLM 辅助 + 规则兜底机制。

### 依赖强度定义

V1 不在 `depends_on` 字段中新增类型，运行时推断：

- **hard**：下游明确需要上游产物才能执行
- **weak**：上游更多是建议顺序或质量增强

### 判定流程

1. **LLM 依赖建议**：输入 TaskItem 描述 + 边两端上下文，输出 `suggested_strength` / `confidence` / `reason`
2. **规则命中**：生成 `hard_hits[]` / `weak_hits[]`
3. **裁决器**：
   - 存在 `hard_hits` → `hard`
   - 无 `hard_hits` 且 LLM 建议 `hard` 且 `confidence ≥ 0.8` → `hard`
   - 其他 → `weak`
   - 冲突或信息不足 → `hard`（安全优先）

### 规则命中定义

**硬依赖**（任一即 hard）：
1. 下游显式引用上游产物（API / schema / 文件 / 函数）
2. 下游为 implement/code_review/fixup 且缺少上游产物无法执行
3. 上游是下游唯一有效前置

**弱依赖**（仅在无硬命中时生效）：
1. 仅为建议顺序，不影响执行
2. 仅影响质量优化，不影响功能闭环
3. 下游具备独立输入，不依赖上游具体产物

### skip 执行算法

当上游 `U` 失败时，对每个直接下游 `D`：

1. `U → D` 判为 hard → `D.status = skipped`（记录 `skipped_due_to_hard_dependency`）
2. `U → D` 判为 weak → 从 D 的有效入边移除 U，重算 `in_degree(D)`，若为 0 则 ready 入队
3. 每次判定写审计日志

### 审计记录结构

```go
type EdgeAssessment struct {
    From            string
    To              string
    LLMSuggested    string   // hard|weak
    LLMConfidence   float64
    LLMReason       string
    HardRuleHits    []string
    WeakRuleHits    []string
    FinalStrength   string   // hard|weak
    DecisionSource  string   // rule|llm|fallback_safe
}
```

关键参数：`llm_hard_confidence_threshold = 0.8`

### 安全护栏

1. 无法可靠判定强弱 → 默认 hard（宁可少并行）
2. 下游被 skipped 后支持人工 retry 上游恢复链路
3. 人工可在 waiting_human 下 replan 生成新版本

---

## 6. V1 必测边界案例

| # | 场景 | 预期 |
|---|------|------|
| 1 | 单节点无依赖 | 立即 ready 并执行 |
| 2 | 多节点无依赖 | 并发执行（受信号量限制） |
| 3 | 缺失依赖 ID | 阻断，Plan → waiting_human(feedback_required) |
| 4 | 自依赖 | 阻断，Plan → waiting_human(feedback_required) |
| 5 | 三节点环 | 阻断，Plan → waiting_human(feedback_required) |
| 6 | 存在冗余边 | 传递约简后再调度 |
| 7 | 上游失败 + block 策略 | 下游 blocked_by_failure |
| 8 | 上游重试成功 | 下游恢复到 ready |
| 9 | skip + 唯一硬依赖 | 下游 skipped |
| 10 | skip + 弱依赖 | 下游仍可 ready |
| 11 | skip + 无法判定强弱 | 默认 skipped + 审计日志 |
| 12 | AI 审核 approve → 人工驳回 | Secretary 重生成 → 重走 AI review |
| 13 | AI 审核超 2 轮 → 人工反馈 | Secretary 重生成 → 重走 AI review |
