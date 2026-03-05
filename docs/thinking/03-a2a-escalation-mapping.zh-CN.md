# A2A 协议与 Escalation 模式的映射

> **前置**: [02-Escalation/Directive 模式](02-escalation-directive-pattern.zh-CN.md) 定义了层级决策协议。
> **后续**: 基于本映射的对外权限设计，见 [04-A2A 对外接口与权限](04-a2a-external-access-design.zh-CN.md)。

## A2A 已有的原语

| A2A 原语 | 我们的概念 |
|---------|----------|
| `INPUT_REQUIRED` | Escalation — Agent 说"我需要帮助" |
| Client 发后续 Message（同 taskId） | Directive — 上级给出决策 |
| `contextId` | Issue 上下文 — escalation chain 共享 |
| Agent Card + skills | 角色能力声明 |
| Task 不可变性 | Run 模型 — retry 创建新 Run |
| Opaque Execution | 每层不知道上下级内部实现 |

## A2A 没有的（我们要建的）

1. **层级关系** — A2A 是 Client↔Server 对等模型，无上下级链
2. **自动路由** — A2A 的 Client 要自己知道跟谁通信，无 chain 配置
3. **逐级冒泡** — `INPUT_REQUIRED` 只回直接调用者，不自动向上传递
4. **多级决策** — 无"我的上级也搞不定，继续往上报"的模式

## 架构分层

```
┌──────────────────────────────────────────────┐
│  应用层: Chain of Responsibility（我们新建的） │
│  - chain 配置（谁的上级是谁）                  │
│  - EscalationRouter（监听 INPUT_REQUIRED，    │
│    查 chain，路由到正确的 supervisor）          │
│  - 人类终端适配器（chain 终点 = human 时       │
│    转换为 inbox + 通知）                      │
├──────────────────────────────────────────────┤
│  传输层: A2A Protocol（已有的标准）             │
│  - Task lifecycle                            │
│  - INPUT_REQUIRED（中断等待输入）              │
│  - SendMessage（传递 Directive）              │
│  - contextId（关联上下文）                     │
│  - Agent Card（能力声明）                      │
└──────────────────────────────────────────────┘
```

## 具体映射

```
Escalation
  = A2A Task 转为 INPUT_REQUIRED
  + escalation 元数据放在 Task.status.message.parts 里（structured data Part）

Directive
  = A2A Client 向 INPUT_REQUIRED 的 Task 发 SendMessage
  + directive 内容放在 Message.parts 里

Chain 路由
  = A2A 之上的应用层逻辑
  = 收到 INPUT_REQUIRED 时，EscalationRouter 查 chain 配置
  → 找到 supervisor → 转发为 SendMessage
  → supervisor 返回决策 → 转换为 SendMessage 发回原 agent
```

## 分形冒泡用 A2A 表达

```
Coder Agent → Task INPUT_REQUIRED
  → EscalationRouter 查 chain: coder → team-leader
  → 创建新 A2A Task 发给 TL
  → TL 也搞不定 → Task INPUT_REQUIRED
  → EscalationRouter 查 chain: team-leader → human
  → 创建 InboxItem（supervisor = human）
  → Human 响应 → SendMessage → TL → SendMessage → Coder
```

## 结论

A2A 给了正确的底层原语（INPUT_REQUIRED + SendMessage + contextId）。
我们不需要发明新的通信协议，直接复用 A2A 的 Task 生命周期和 Message 传递。
只需在应用层加 chain 配置 + EscalationRouter + 人类终端适配器。
