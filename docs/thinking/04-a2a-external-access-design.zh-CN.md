# A2A 对外接口与权限设计

> **前置**: [03-A2A 协议映射](03-a2a-escalation-mapping.zh-CN.md) 确定了 A2A 原语与内部概念的对应关系。
> **相关**: 多实例部署下的权限边界，见 [05-多用户多 Project 部署](05-multi-user-deployment-model.zh-CN.md)。

## 定位

整个系统对外是 A2A 网络中的**一个 Agent** — 一个具备完整开发能力的团队。
内部有 TL、Coder、Reviewer、Decomposer，但外部只看到一个黑盒。

## 现有 A2A 实现

```
已有:
├── Agent Card — /.well-known/agent-card.json（skills 为空）
├── A2A Bridge — SendMessage→Issue, GetTask→状态, CancelTask→放弃
├── JSON-RPC — /api/v1/a2a，4 个方法
├── SSE — message/stream
├── Bearer Token — 单一认证
└── Issue→TaskState 映射
```

## 缺口

| 能力 | 缺口 |
|------|------|
| 跟进/反馈 | 无法对已有 Task 发后续 Message |
| INPUT_REQUIRED 回复 | reviewing 映射为 input-required，但外部无法回复 |
| Artifacts 输出 | PR URL、测试结果、代码变更等不返回 |
| 列表查询 | 无 tasks/list |
| Skills 声明 | Agent Card skills 为空 |
| 权限分层 | 单一 token，全有或全无 |
| Push 通知 | 无 webhook |

## 权限设计：Token = 身份 + 角色 + 范围

每个 token 同时携带三层信息（与 [05-多用户部署](05-multi-user-deployment-model.zh-CN.md) 共用同一模型）：

```yaml
a2a:
  tokens:
    - token: "tok_alice"
      submitter: "alice"           # 谁（审计溯源）
      role: admin                  # 能做什么（操作权限）
      projects: ["frontend", "backend"]  # 能碰哪些 project

    - token: "tok_bob"
      submitter: "bob"
      role: orchestrator
      projects: ["backend"]

    - token: "tok_ci"
      submitter: "ci"
      role: orchestrator
      projects: ["*"]

    - token: "tok_dashboard"
      submitter: "dashboard"
      role: viewer
      projects: ["*"]

  roles:
    orchestrator:  # 上级 agent（VP、其他团队 TL）、CI
      operations: [message/send, message/send:follow, tasks/get, tasks/list, tasks/cancel]
      data_scope: summary

    viewer:  # 监控/看板
      operations: [tasks/get, tasks/list]
      data_scope: summary

    human:  # 人类决策者
      operations: [message/send:follow, tasks/get, tasks/list]
      data_scope: full  # 看到 TL 分析、建议、完整上下文

    admin:  # 完全控制
      operations: ["*"]
      data_scope: full
```

## data_scope 控制可见性

同一个 `tasks/get`，不同 role 看到不同内容：

- **summary**: status + artifacts（PR URL 等）
- **full**: 上面 + escalation 上下文 + TL 分析 + 建议选项
- **internal**（admin only）: 上面 + 消息历史 + run 详情 + 子 issue 列表

## Extended Agent Card

A2A 原生支持认证后返回更详细的 Agent Card：

- **公开**: 基础 skills（develop）
- **orchestrator**: + directive、reprioritize
- **human**: + resolve_escalation

## Escalation → INPUT_REQUIRED → 外部回复

```
内部: Coder 冲突 → TL 也搞不定 → TL escalate(chain → human)
  ↓
系统: Issue → INPUT_REQUIRED
  ↓
A2A Task state = INPUT_REQUIRED，message.parts 包含结构化 escalation
  ↓
推送: Push notification / webhook 通知 human 端
  ↓
人类: message/send { taskId, parts: [{ data: { type: "directive", action: "retry", ... }}] }
  ↓
系统: → Directive → TL → Coder → 继续执行
```

## 改造方式

内部 EventBus/Handler 不动。在边界层加 A2A 适配：

```
外部 A2A Client ──→ A2A 权限层 ──→ A2ABridge ──→ EventBus/Manager
                         ↑                              │
                    A2A 响应 ←── TaskState 映射 ←────────┘
```

## 实现清单

| 文件 | 操作 | 说明 |
|------|------|------|
| web/a2a_auth.go | 新建 | Token→Role 解析、权限检查、data_scope 过滤 |
| web/handlers_a2a.go | 修改 | 加 tasks/list、follow-up 路由、ExtendedAgentCard |
| teamleader/a2a_bridge.go | 修改 | 加 HandleFollowUp、ListTasks、Artifact 输出 |
| web/handlers_a2a_protocol.go | 修改 | Artifact 序列化、结构化 Part |
| config/types.go | 修改 | A2A tokens/roles 配置结构 |
| configs/defaults.yaml | 修改 | A2A role 默认配置 |
