# D2 Gate 门禁设计

> 日期: 2026-03-09
>
> 目标: 用可配置的 Gate 链替代固定的 ReviewGate 插件选择，支持 auto/owner_review/peer_review/vote 四种策略，可串联多道门禁。

## 一、动机

当前系统的审查是硬编码的两阶段流程：

```
Issue → SubmitForReview → TwoPhaseReview(Phase1需求审+Phase2依赖分析) → approve/reject
```

问题：
1. **不可配置** — 审查策略绑死在 WorkflowProfileType 上（normal/strict/fast_release），无法按 Issue 粒度定制
2. **无门禁串联** — 只有一道审查关卡，无法表达"先跑自动检查，再人工审核"
3. **决策不可追溯** — 审查决策没有关联 Decision 记录，无法回溯 AI 判断过程
4. **前置条件缺失** — 没有 acceptance_criteria 概念，审查标准模糊

## 二、核心设计

### 2.1 Gate 模型

```go
// internal/core/gate.go

type GateType string

const (
    GateTypeAuto        GateType = "auto"         // AI 自动判定
    GateTypeOwnerReview GateType = "owner_review"  // 主持人/TL 判定
    GateTypePeerReview  GateType = "peer_review"   // 参与者互审
    GateTypeVote        GateType = "vote"          // 投票表决
)

type GateFallback string

const (
    GateFallbackEscalate  GateFallback = "escalate"   // 升级给人
    GateFallbackForcePass GateFallback = "force_pass"  // 强制通过
    GateFallbackAbort     GateFallback = "abort"        // 终止
)

type GateStatus string

const (
    GateStatusPending  GateStatus = "pending"
    GateStatusPassed   GateStatus = "passed"
    GateStatusFailed   GateStatus = "failed"
    GateStatusSkipped  GateStatus = "skipped"
)

// Gate defines a single checkpoint in the review pipeline.
type Gate struct {
    Name        string       `json:"name"`                   // "单测通过", "代码审阅"
    Type        GateType     `json:"type"`                   // auto / owner_review / peer_review / vote
    Rules       string       `json:"rules"`                  // 自然语言规则描述，写进 prompt
    MaxAttempts int          `json:"max_attempts,omitempty"` // 最多检查次数，0=不限
    Fallback    GateFallback `json:"fallback,omitempty"`     // 超过 max_attempts 后的处理
}
```

### 2.2 Gate 在 Issue 上的位置

不在 Issue 结构体上加 `Gates` 字段。原因：
- Gate 配置应该来自 **WorkflowProfile / 模板**，而非每个 Issue 手动指定
- 减少 Issue 模型膨胀

而是在 WorkflowProfile 层面定义默认 Gate 链：

```go
// internal/core/workflow_profile.go — 扩展

type WorkflowProfile struct {
    Type       WorkflowProfileType `json:"type"`
    SLAMinutes int                 `json:"sla_minutes"`
    Gates      []Gate              `json:"gates,omitempty"` // 新增: 门禁链
}
```

### 2.3 预置 Profile 的默认 Gate 链

```go
var defaultProfileGates = map[WorkflowProfileType][]Gate{
    WorkflowProfileNormal: {
        {Name: "demand_review", Type: GateTypeAuto, Rules: "需求完整性和可行性检查", MaxAttempts: 2, Fallback: GateFallbackEscalate},
    },
    WorkflowProfileStrict: {
        {Name: "demand_review", Type: GateTypeAuto, Rules: "需求完整性和可行性检查", MaxAttempts: 2, Fallback: GateFallbackEscalate},
        {Name: "peer_review", Type: GateTypePeerReview, Rules: "代码和方案质量互审", MaxAttempts: 3, Fallback: GateFallbackEscalate},
    },
    WorkflowProfileFastRelease: {
        {Name: "auto_pass", Type: GateTypeAuto, Rules: "快速通过，仅检查基本格式", MaxAttempts: 1, Fallback: GateFallbackForcePass},
    },
}
```

### 2.4 GateCheck 记录

每次门禁检查产生一条持久化记录：

```go
// internal/core/gate.go

// GateCheck records one attempt at passing a gate.
type GateCheck struct {
    ID         string     `json:"id"`          // gc-YYYYMMDD-HHMMSS-xxxxxxxx
    IssueID    string     `json:"issue_id"`
    GateName   string     `json:"gate_name"`   // 对应 Gate.Name
    GateType   GateType   `json:"gate_type"`
    Attempt    int        `json:"attempt"`      // 第几次尝试
    Status     GateStatus `json:"status"`       // pending/passed/failed/skipped
    Reason     string     `json:"reason"`       // 通过/失败原因
    DecisionID string     `json:"decision_id,omitempty"` // 关联 Decision 记录
    CheckedBy  string     `json:"checked_by"`   // agent ID 或 "human"
    CreatedAt  time.Time  `json:"created_at"`
}

func NewGateCheckID() string {
    return fmt.Sprintf("gc-%s-%s", time.Now().Format("20060102-150405"), randomHex(4))
}
```

### 2.5 与 Decision 的关联

每次 `auto` 类型的 gate_check 产生一条 Decision 记录：

```go
Decision{
    Type:          "gate_check",
    IssueID:       issueID,
    Action:        "pass" / "fail" / "continue",
    Reasoning:     "...",               // AI 判断理由
    PromptHash:    PromptHash(prompt),
    PromptPreview: TruncateString(prompt, 500),
}
```

GateCheck.DecisionID 指向该 Decision.ID，实现审计追溯。

### 2.6 与 TaskStep 的关联

门禁检查产生 TaskStep：

```go
// 新增 action
StepGateCheck  TaskStepAction = "gate_check"   // 不改变 Issue.Status
StepGatePassed TaskStepAction = "gate_passed"   // 不改变 Issue.Status
StepGateFailed TaskStepAction = "gate_failed"   // 不改变 Issue.Status
```

这些是 **run-level actions**（不改变 Issue.Status），与现有 `StepRunCreated` 等同级。

TaskStep 的 `Note` 字段记录 gate_name，`RefID` 指向 GateCheck.ID。

## 三、执行引擎

### 3.1 GateRunner 接口

```go
// internal/core/gate.go

// GateRunner evaluates a single gate for an issue.
type GateRunner interface {
    Check(ctx context.Context, issue *Issue, gate Gate, attempt int) (*GateCheck, error)
}
```

### 3.2 四种策略实现

```
internal/teamleader/gate_runner.go

AutoGateRunner      — 调用现有 DemandReviewer，用 gate.Rules 增强 prompt
OwnerReviewRunner   — 标记 pending，等待人工 API 调用 approve/reject
PeerReviewRunner    — 标记 pending，等待所有 participants 投出 verdict
VoteGateRunner      — 标记 pending，计票达到阈值后决策
```

**第一阶段只实现 `AutoGateRunner` 和 `OwnerReviewRunner`**，peer_review 和 vote 留占位。

### 3.3 GateChain 编排

```go
// internal/teamleader/gate_chain.go

type GateChain struct {
    Store   GateStore
    Runners map[GateType]GateRunner
}

// Run executes gates sequentially. Returns the first failing gate or nil if all pass.
func (c *GateChain) Run(ctx context.Context, issue *Issue, gates []Gate) (*GateCheck, error) {
    for _, gate := range gates {
        for attempt := 1; ; attempt++ {
            check, err := c.Runners[gate.Type].Check(ctx, issue, gate, attempt)
            if err != nil {
                return nil, err
            }
            c.Store.SaveGateCheck(check)
            // 记录 TaskStep

            if check.Status == GateStatusPassed {
                break // 下一道门禁
            }
            if check.Status == GateStatusFailed {
                if gate.MaxAttempts > 0 && attempt >= gate.MaxAttempts {
                    return c.applyFallback(gate, check)
                }
                // 继续重试（auto 类型会触发 fixup）
                continue
            }
            if check.Status == GateStatusPending {
                return check, nil // 等待人工介入
            }
        }
    }
    return nil, nil // 全部通过
}
```

### 3.4 与 Manager 的集成

替换 `submitIssues()` 中的 3 级优先级选择：

```go
// Before:
switch {
case m.twoPhaseReview != nil:  m.twoPhaseReview.SubmitForReview()
case m.reviewSubmitter != nil: m.reviewSubmitter.Submit()
case m.reviewGate != nil:      m.reviewGate.Submit()
}

// After:
if m.gateChain != nil {
    gates := m.resolveGates(issues)     // 从 profile 或 issue 获取 gate 配置
    result, err := m.gateChain.Run(ctx, issue, gates)
    // ...
} else {
    // fallback: 兼容旧 ReviewGate 接口
}
```

## 四、数据持久化

### 4.1 SQLite 迁移

```sql
-- Migration V12: gates

CREATE TABLE IF NOT EXISTS gate_checks (
    id          TEXT PRIMARY KEY,
    issue_id    TEXT NOT NULL,
    gate_name   TEXT NOT NULL,
    gate_type   TEXT NOT NULL,
    attempt     INTEGER NOT NULL DEFAULT 1,
    status      TEXT NOT NULL DEFAULT 'pending',
    reason      TEXT DEFAULT '',
    decision_id TEXT DEFAULT '',
    checked_by  TEXT DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (issue_id) REFERENCES issues(id)
);

CREATE INDEX idx_gate_checks_issue ON gate_checks(issue_id);
```

### 4.2 Store 接口扩展

```go
// internal/core/store.go — 新增

SaveGateCheck(gc *GateCheck) error
GetGateChecks(issueID string) ([]GateCheck, error)
GetLatestGateCheck(issueID, gateName string) (*GateCheck, error)
```

## 五、API

### 5.1 查询门禁状态

```
GET /api/v3/issues/{issueId}/gates
```

响应：
```json
{
  "gates": [
    {
      "name": "demand_review",
      "type": "auto",
      "status": "passed",
      "attempts": 1,
      "checks": [
        {"id": "gc-...", "attempt": 1, "status": "passed", "reason": "..."}
      ]
    },
    {
      "name": "peer_review",
      "type": "peer_review",
      "status": "pending",
      "attempts": 0,
      "checks": []
    }
  ]
}
```

### 5.2 人工门禁操作

```
POST /api/v3/issues/{issueId}/gates/{gateName}/resolve
```

请求：
```json
{
  "action": "pass" | "fail",
  "reason": "符合需求"
}
```

用于 `owner_review` / `peer_review` / `vote` 类型的人工判定。

## 六、前端集成

### 6.1 IssueFlowTree 展示

在 IssueFlowTree 中展示 `gate_check` TaskStep：
- 图标区分 passed ✅ / failed ❌ / pending ⏳
- 点击展开显示 Reason 和关联 Decision

### 6.2 BoardView 门禁状态

Issue 卡片上显示 gate 进度条（如 "2/3 gates passed"）。

## 七、向后兼容

1. **ReviewGate 插件保留** — GateChain 作为新路径，ReviewGate 作为 fallback
2. **WorkflowProfile.Gates 为空时** — 使用 `defaultProfileGates` 映射，行为与当前完全一致
3. **review_records 表保留** — GateCheck 是新增表，不影响现有 ReviewRecord 流程
4. **渐进迁移** — 第一阶段 AutoGateRunner 内部复用 `TwoPhaseReview`，保证结果一致

## 八、实施分期

### Wave 1: 基础设施（D1 代码落地 + Gate 模型）

1. 创建 `internal/core/decision.go`（从计划文档落地代码）
2. 创建 `internal/core/gate.go`（Gate, GateCheck, GateRunner 接口）
3. SQLite 迁移 V12（gate_checks 表 + decisions 表）
4. Store 接口扩展 + SQLite 实现
5. TaskStep 新增 gate_check/gate_passed/gate_failed actions

### Wave 2: 执行引擎

6. `internal/teamleader/gate_runner.go` — AutoGateRunner（复用 DemandReviewer）
7. `internal/teamleader/gate_runner.go` — OwnerReviewRunner（标记 pending）
8. `internal/teamleader/gate_chain.go` — GateChain 编排器
9. WorkflowProfile 扩展 Gates 字段 + 默认 Gate 链
10. Manager 集成 GateChain，保留 ReviewGate fallback

### Wave 3: API + 前端

11. `internal/web/handlers_gate.go` — GET gates / POST resolve
12. 前端 IssueFlowTree 展示 gate_check 事件
13. BoardView Issue 卡片 gate 进度

### Wave 4: Decision 串联（可选，视时间）

14. AutoGateRunner 的每次 check 产生 Decision 记录
15. 前端 Decision 回溯查看

## 九、验收标准

1. `WorkflowProfile{Type: "normal"}` 默认产生一道 auto gate，行为与当前审查一致
2. `WorkflowProfile{Type: "strict"}` 产生两道 gate（auto + peer_review）
3. `WorkflowProfile{Gates: [...]}` 自定义 gate 链可正常执行
4. 每次 gate check 产生 TaskStep 记录，IssueFlowTree 可展示
5. owner_review 类型 gate 可通过 API 手动 resolve
6. 所有现有 review 测试继续通过（向后兼容）
