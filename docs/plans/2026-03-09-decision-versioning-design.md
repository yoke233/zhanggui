# Decision 版本化设计

日期: 2026-03-09
状态: draft

## 1. 目标

为系统中每个 AI 决策建立结构化记录，包含 prompt/model/reasoning/output，实现决策可追溯。当 Agent 行为异常时，能够回溯到具体的 prompt 和模型版本。

### 设计原则

- **独立表，显式 schema** — Decision 是核心资产，值得专用表和强类型字段
- **与 TaskStep 通过 RefID 关联** — TaskStep 记"发生了什么"，Decision 记"为什么这么决策"，职责分离
- **渐进覆盖** — 先覆盖 3 个关键决策点（审查/分解/Stage），后续扩展
- **不影响性能** — 决策记录是写入的副作用，不增加 LLM 调用

## 2. 现状分析

### 系统中的 5 个决策点

| 决策点 | 位置 | 当前记录 | 缺失 |
|--------|------|---------|------|
| 审查决策 | review.go `decideSession()` | ReviewRecord (verdict/score) | prompt、model、reasoning |
| 分解决策 | decompose_handler.go | TaskStep (note only) | 全部 |
| Stage 执行 | executor_stages.go | run_events (prompt/done) | model、template 版本 |
| Chat 助手 | handlers_chat.go | 无 | 全部 |
| 权限决策 | acp_handler.go | 无 | 策略选择记录 |

## 3. 数据模型

### 3.1 Decision 结构

```go
// internal/core/decision.go
type Decision struct {
    ID              string    `json:"id"`               // dec-YYYYMMDD-HHMMSS-xxxxxxxx
    IssueID         string    `json:"issue_id"`         // 关联 Issue
    RunID           string    `json:"run_id,omitempty"`  // 关联 Run（可选）
    StageID         StageID   `json:"stage_id,omitempty"`// 关联 Stage（可选）
    AgentID         string    `json:"agent_id"`         // 决策执行者
    Type            string    `json:"type"`             // 决策类型枚举
    // 输入
    PromptHash      string    `json:"prompt_hash"`      // prompt 的 SHA256 前 16 hex
    PromptPreview   string    `json:"prompt_preview"`   // prompt 前 500 字符预览
    Model           string    `json:"model"`            // 模型 ID
    Template        string    `json:"template"`         // prompt 模板名
    TemplateVersion string    `json:"template_version"` // 模板版本
    InputTokens     int       `json:"input_tokens"`     // 输入 token 数
    // 输出
    Action          string    `json:"action"`           // 决策结果动作
    Reasoning       string    `json:"reasoning"`        // AI 推理过程
    Confidence      float64   `json:"confidence"`       // 置信度 0-1
    OutputTokens    int       `json:"output_tokens"`    // 输出 token 数
    OutputData      string    `json:"output_data"`      // 结构化输出 JSON
    // 元数据
    DurationMs      int64     `json:"duration_ms"`      // 决策耗时
    CreatedAt       time.Time `json:"created_at"`
}
```

### 3.2 Decision Type 枚举

```go
const (
    DecisionTypeReview     = "review"      // 审查决策
    DecisionTypeDecompose  = "decompose"   // 任务分解
    DecisionTypeStage      = "stage"       // Stage 执行
    DecisionTypeChat       = "chat"        // Chat 助手
    DecisionTypePermission = "permission"  // 权限决策
)
```

### 3.3 与 TaskStep 的关联

TaskStep 通过 `RefID`/`RefType` 关联 Decision：

```go
TaskStep{
    Action:  StepReviewApproved,
    RefID:   "dec-20260309-120000-abcd1234",  // 指向 Decision.ID
    RefType: "decision",
}
```

一个 TaskStep 可以关联 0 或 1 个 Decision。一个 Decision 对应 1 个 TaskStep。

## 4. 数据库表

```sql
CREATE TABLE decisions (
    id               TEXT PRIMARY KEY,
    issue_id         TEXT NOT NULL,
    run_id           TEXT,
    stage_id         TEXT,
    agent_id         TEXT NOT NULL,
    type             TEXT NOT NULL,
    prompt_hash      TEXT NOT NULL,
    prompt_preview   TEXT NOT NULL DEFAULT '',
    model            TEXT NOT NULL DEFAULT '',
    template         TEXT NOT NULL DEFAULT '',
    template_version TEXT NOT NULL DEFAULT '',
    input_tokens     INTEGER NOT NULL DEFAULT 0,
    action           TEXT NOT NULL,
    reasoning        TEXT NOT NULL DEFAULT '',
    confidence       REAL NOT NULL DEFAULT 0,
    output_tokens    INTEGER NOT NULL DEFAULT 0,
    output_data      TEXT NOT NULL DEFAULT '{}',
    duration_ms      INTEGER NOT NULL DEFAULT 0,
    created_at       DATETIME NOT NULL,
    FOREIGN KEY (issue_id) REFERENCES issues(id)
);
CREATE INDEX idx_decisions_issue ON decisions(issue_id, created_at);
CREATE INDEX idx_decisions_type  ON decisions(type, created_at);
CREATE INDEX idx_decisions_model ON decisions(model);
```

## 5. Store 接口

```go
// core/store.go 新增
SaveDecision(d *Decision) error
GetDecision(id string) (*Decision, error)
ListDecisions(issueID string) ([]Decision, error)
```

## 6. 各决策点的记录规范

### 6.1 审查决策

```go
// review.go — runReviewSession 完成后
decision := &core.Decision{
    ID:            core.NewDecisionID(),
    IssueID:       issue.ID,
    AgentID:       reviewerName,
    Type:          core.DecisionTypeReview,
    PromptHash:    core.PromptHash(reviewPrompt),
    PromptPreview: core.TruncateString(reviewPrompt, 500),
    Model:         modelID,  // 从 ACP session 或 reviewer 获取
    Template:      "review",
    Action:        "approve",  // approve / fix / escalate
    Reasoning:     verdict.Summary,
    Confidence:    float64(score) / 100.0,
    OutputData:    `{"score":85,"issues_count":0}`,
    DurationMs:    elapsed.Milliseconds(),
    CreatedAt:     time.Now(),
}
store.SaveDecision(decision)

// TaskStep 关联
recordTaskStep(issue, StepReviewApproved, reviewerName, note)
// RefID = decision.ID, RefType = "decision"
```

### 6.2 分解决策

```go
// decompose_handler.go / decompose_planner.go
decision := &core.Decision{
    ID:            core.NewDecisionID(),
    IssueID:       parentIssue.ID,
    AgentID:       "team_leader",
    Type:          core.DecisionTypeDecompose,
    PromptHash:    core.PromptHash(systemPrompt + userPrompt),
    PromptPreview: core.TruncateString(userPrompt, 500),
    Model:         modelID,
    Template:      "decompose",
    Action:        "decompose",
    Reasoning:     proposal.Summary,
    OutputData:    `{"children_count":5}`,
    DurationMs:    elapsed.Milliseconds(),
    CreatedAt:     time.Now(),
}
```

### 6.3 Stage 执行

```go
// executor.go — stage 完成后
decision := &core.Decision{
    ID:            core.NewDecisionID(),
    IssueID:       run.IssueID,
    RunID:         run.ID,
    StageID:       stage.Name,
    AgentID:       agentName,
    Type:          core.DecisionTypeStage,
    PromptHash:    core.PromptHash(prompt),
    PromptPreview: core.TruncateString(prompt, 500),
    Model:         modelID,
    Template:      string(stage.Name),
    Action:        "completed",  // completed / failed
    OutputTokens:  tokenCount,
    DurationMs:    elapsed.Milliseconds(),
    CreatedAt:     time.Now(),
}
```

## 7. API

```
GET /api/v3/issues/{issueId}/decisions
  → 返回该 Issue 的所有 Decision

GET /api/v3/decisions/{id}
  → 返回单个 Decision 详情
```

Timeline API 不变，TaskStep 通过 RefID 关联 Decision，前端按需懒加载。

## 8. 前端展示

IssueFlowTree 中，当 TaskStep 的 `ref_type == "decision"` 时，点击展开显示 Decision 详情：

```
▼ ✅ reviewing                                        10:05
    ├── 决策: approve
    ├── 模型: claude-sonnet-4-20250514
    ├── 评分: 85 (confidence: 0.9)
    ├── 推理: "代码结构清晰，测试覆盖率 85%"
    ├── 耗时: 3.2s
    └── prompt: sha256:abcd1234 (500 tokens)
```

## 9. 三层数据架构更新

```
TaskStep (业务事实层)
  ├── Issue 状态变迁（~15 种 action）
  └── Run 关键节点（~7 种 action）
          │
          ├── decisions (决策追溯层)  ← 新增
          │     └── prompt_hash / model / reasoning / action
          │
          ├── run_events (执行追溯层)
          │     └── prompt / agent_message / tool_call / ...
          │
          └── review_records (审核细节)
                └── reviewer / verdict / issues / fixes
```

- **TaskStep** — 记"发生了什么"（业务转折点）
- **Decision** — 记"为什么这么决策"（AI 推理追溯）
- **run_events** — 记"agent 具体做了什么"（执行细节）
- **review_records** — 记"怎么审的"（审核细节）
- 通过 `issue_id` / `run_id` / `ref_id` 互相关联

## 10. 改造范围

### 新增

- `internal/core/decision.go` — Decision 模型 + ID 生成 + PromptHash
- `store-sqlite` migration V11 — decisions 表
- `store-sqlite` SaveDecision/GetDecision/ListDecisions 实现
- API handler: `GET /api/v3/issues/{id}/decisions`

### 改造

- `internal/teamleader/review.go` — 审查完成时写 Decision
- `internal/teamleader/decompose_handler.go` — 分解完成时写 Decision
- `internal/engine/executor.go` — Stage 完成时写 Decision
- `internal/teamleader/scheduler_dispatch.go` — recordTaskStep 支持 RefID/RefType 填写
- `web/src/components/IssueFlowTree.tsx` — 展示 Decision 详情

### 不变

- TaskStep 模型和 schema（只使用已有 RefID/RefType 字段）
- run_events 表
- review_records 表
- Timeline API

## 11. 未来演进

- **retrieval_trace** — 记录"AI 看到了什么上下文"（v3 OpenViking 设计）
- **DecisionValidator** — 对 Decision 做硬规则校验，防止 AI 做出不合理决策
- **Decision 统计** — 按 model/type/action 聚合分析决策质量
- **Gate 集成** — gate_check 的每次检查产生一条 Decision 记录
