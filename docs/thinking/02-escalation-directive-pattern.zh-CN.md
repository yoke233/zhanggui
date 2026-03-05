# Escalation/Directive 模式：通用层级决策协议

> **前置**: 本文从 [01-PR/Merge 流程](01-pr-merge-flow.zh-CN.md) 的 TL Triage 模式泛化而来。
> **后续**: 如何用 A2A 协议表达本模式，见 [03-A2A 协议映射](03-a2a-escalation-mapping.zh-CN.md)。

## 核心抽象

每个 Agent 只需要两个通道：

```
        ┌─────────┐
        │  上级    │ (agent 不知道这是谁)
        └────┬────┘
     Directive↓   ↑Escalation
        ┌────┴────┐
        │  Agent  │
        └────┬────┘
     Directive↓   ↑Escalation
        ┌────┴────┐
        │  下级    │ (可能有，可能没有)
        └─────────┘
```

**整个系统只有两种消息**：
- **Escalation（上报）** — "我遇到了我处理不了的事"
- **Directive（指令）** — "这是你应该做的"

## 数据结构

```go
type Escalation struct {
    ID          string
    FromRole    string            // "coder", "reviewer", "team-leader"
    IssueID     string
    ProjectID   string
    Category    string            // "merge_conflict", "test_failure", "beyond_scope"
    Summary     string            // 发生了什么
    Analysis    string            // agent 自己的判断
    Suggestions []Suggestion      // agent 认为应该怎么做
    Priority    string            // agent 评估的紧急程度
    Context     map[string]any    // 附加数据
}

type Suggestion struct {
    Action      string
    Description string
    Confidence  float64
}

type Directive struct {
    ID            string
    EscalationID  string          // 响应哪个上报
    ToRole        string
    IssueID       string
    Action        string          // "retry", "modify", "abort", "reassign"
    Instructions  string          // 自然语言指导
    Parameters    map[string]any
    Reason        string          // 审计用
}
```

## 层级路由

Agent 不做路由，**系统根据配置路由**：

```yaml
# 最小团队
chain:
  coder: team-leader
  reviewer: team-leader
  team-leader: human

# 大型组织
chain:
  coder: team-leader
  team-leader: engineering-manager
  engineering-manager: vp-engineering
  vp-engineering: human
```

加一层管理只需改配置，不改代码。

## 分形冒泡

```
Coder 遇到冲突 → escalate
  → 系统查 chain: coder → team-leader
  → TL 分析后发现是跨项目依赖，自己也搞不定
  → TL escalate
  → 系统查 chain: team-leader → vp-engineering
  → VP 读两个项目上下文，下发两个 directive
  → 分别传回各自的 TL → 传回各自的 Coder
```

每一级用完全相同的协议。Agent 不知道上级是 AI 还是人类。

## 触发场景

| 事件 | TL 典型决策 |
|------|------------|
| EventIssueMergeConflict | 打回 coder rebase |
| EventRunFailed | 分析日志，指导 coder 修复 |
| EventIssueFailed（子 issue） | 判断影响范围，跳过/重试/升级 |
| Review reject 3 次 | 介入重写 spec 或升级人类 |

## 人类收件箱

人类收件箱 = **TL Triage 的升级出口**，不是独立系统。

```go
type InboxItem struct {
    ID          string
    IssueID     string
    ProjectID   string
    Type        string     // "merge_conflict" / "repeated_failure" / ...
    TLSummary   string     // TL 对问题的总结
    TLAnalysis  string     // TL 的分析
    Suggestions []string   // TL 的建议（人类选一个即可）
    Priority    string
    Status      string     // "pending" / "resolved"
}
```

人类从"读日志找问题做判断"变成"读 TL 的分析选一个选项"。
