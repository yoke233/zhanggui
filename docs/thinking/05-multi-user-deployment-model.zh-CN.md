# 多用户多 Project 部署模型

> **相关**: A2A token 权限分层设计，见 [04-A2A 对外接口与权限](04-a2a-external-access-design.zh-CN.md)。

## 核心推导

**Git repo 是唯一的协调原子单位。** 不管多少个系统在写代码，最终都汇入同一棵 git 树。

多套系统独立操作同一个 repo = 多个脑子同时操作一个身体。
A 不知道 B 正在改哪个文件，冲突发生时两边都不知道对方的意图，没有人能做出"先合 A 再让 B rebase"的决策。

**所以：一个 repo = 一个 TL = 一个 ai-workflow 实例的管辖范围。**

但一个实例可以管多个 repo（多个 Project）。跨 repo 无代码冲突，协调的是时序依赖，不是合并顺序。

## 当前系统能力（已验证）

| 能力 | 现状 |
|------|------|
| 多 Project | ✅ 一个实例支持多 project，每个 project 对应一个 repo |
| Scheduler | ✅ 全局单个，有 per-project 并发限制 |
| 子 Issue 跨 Project | ❌ `decompose_handler.go` 强制继承父 issue 的 ProjectID |
| 用户识别 | ❌ 单个全局 token，不知道"谁发的" |
| A2A 多 project | ✅ 支持显式指定 project_id |

## 三个独立问题

多用户、多 repo、多团队看起来是一个大问题，实际是三个互不依赖的小问题。

### 问题 1：跨 Project 分解

**本质**：一个 epic 的子 issue 能不能分到不同 repo？

**答案**：`DecomposeSpec` 加 `ProjectID` 字段。`decompose_handler` 创建子 issue 时用 `spec.ProjectID` 替代 `parent.ProjectID`。

`ChildCompletionHandler` 的 `WHERE parent_id=?` 查询不区分 project，天然支持跨 project 子 issue 完成追踪。

这跟多用户、多实例没有关系。单用户单实例也需要这个能力。

### 问题 2：多用户识别

**本质**：怎么知道"谁发的请求"和"他能操作哪些 project"？

**答案**：每个 token 同时携带身份 + 角色 + project 范围，不建用户系统。Token 模型与 [04-A2A 权限设计](04-a2a-external-access-design.zh-CN.md) 共用同一结构：

```yaml
a2a:
  tokens:
    - token: "tok_alice"
      submitter: "alice"             # 谁（审计溯源）
      role: admin                    # 能做什么（见 04 的 roles 定义）
      projects: ["frontend", "backend"]  # 能碰哪些 project
    - token: "tok_bob"
      submitter: "bob"
      role: orchestrator
      projects: ["backend"]
    - token: "tok_ci"
      submitter: "ci"
      role: orchestrator
      projects: ["*"]
```

鉴权流程：`BearerAuthMiddleware` 解析 token → 得到 submitter/role/projects → `resolveProjectScope` 检查 project 权限 → role 决定可用操作和数据可见性。

**Issue 溯源**（纯审计）：
```go
SubmittedBy string `json:"submitted_by"` // "alice" / "github:webhook" / "ci"
```

不自建 GitHub 权限镜像。GitHub token 已回答"谁能访问什么"，再做一套有同步延迟且违反 KISS。

**Token 统一鉴权**：A2A 和 Web API 共用同一套 token + `BearerAuthMiddleware`，不分两套认证体系。浏览器用户如需 Dashboard，做一个 token 输入页（同 Jupyter Notebook 模式），不做 OAuth/注册/密码。

**Token 管理**：config YAML 手动维护，不做管理界面。触发条件：token 数量超 20 个或需要用户自助申请时再考虑。

### 问题 3：是否需要多实例

**本质**：什么时候一个实例不够用？

**答案：只有一个理由 — 信任边界。**

| 场景 | 需要多实例吗 | 原因 |
|------|-------------|------|
| 同一团队，前端+后端 repo | **不需要** | 一个实例多 project，TL 有全局视图 |
| 同一公司，不同部门 | **不需要** | ProjectID 逻辑隔离 + token scope 控制权限 |
| 不同公司共用一个 repo | **需要** | 信任边界不同，不能共享 token |
| 团队要求独立部署/运维 | **需要** | 部署节奏独立是组织约束 |

**"我们有很多 repo"不是拆实例的理由** — 一个实例本来就支持多 project。
**"我们有很多用户"不是拆实例的理由** — 多个 A2A token 就够了。
**"我们想要团队隔离"不是拆实例的理由** — ProjectID + token scope 已经是逻辑隔离。

## 默认部署拓扑：单实例多 Project

覆盖 90% 场景：

```
Alice ──tok_alice──→ ┌──────────────────────────────────┐
                     │  ai-workflow 实例                  │
Bob   ──tok_bob────→ │                                    │
                     │  Project: frontend (repo: web-app) │
CI    ──tok_ci─────→ │  Project: backend  (repo: api)     │
                     │  Project: mobile   (repo: app)     │
                     │                                    │
                     │  TL 看到所有 project               │
                     │  Epic 可跨 project 分解子 issue    │
                     │  Scheduler 按 project 限制并发      │
                     └──────────────────────────────────┘
```

TL 拥有全局视图：
- 知道 Alice 的 issue #1 和 Bob 的 issue #3 都改了 `handler.go`
- 决定先合 #1，再让 #3 rebase
- Epic 拆出 frontend + backend 子 issue，追踪跨 project 完成状态

## 多实例场景（按需拆分）

### 跨组织共享 repo

```
Org Alpha 的 ai-workflow ──PR──→ repo X ←──PR── Org Beta / 外部开发者
                                   ↑
                           GitHub Webhook
                                   ↓
                        Alpha 的 TL 感知外部变更
```

无法强制对方用你的系统。通过 webhook 感知外部合并，TL 重新评估合并队列。

### 多实例 A2A 联通

当组织规模增长到单实例管不过来时：

```
实例 A (frontend 团队) ──A2A──→ 实例 B (backend 团队)
```

到那时 Escalation/Directive 协议（见 [02](02-escalation-directive-pattern.zh-CN.md)）和 A2A（见 [03](03-a2a-escalation-mapping.zh-CN.md)）已经在那里等着。不需要提前设计 PM 层级 — 当你遇到具体的协调瓶颈时，会清楚地知道哪里断裂了。

## 层级：不设计，让它涌现

每一层只做下一层做不了的事：

| 层级 | 协调什么 | 为什么下级做不了 |
|------|---------|----------------|
| Coder | 文件变更 | — |
| TL | 一个 repo 内的合并顺序 | Coder 不知道其他 Coder 在改什么 |
| （未来）PM | 多个 repo 间的时序依赖 | TL 不知道其他 repo 的进度 |
| Human | 战略优先级 | AI 不知道商业目标 |

现在不需要 PM 层。TL 管 3-5 个 project 时，它自然就是 PM。当 TL 管 10+ project 管不过来时，拆实例 + 加协调层。

## OpenViking 映射

```
account_id = 组织名（部署时配置）
user_id    = "system"（AI 系统是唯一 "user"）
agent_id   = 角色名（coder/reviewer/tl）
```

- 人类用户不出现在 OpenViking 里
- AI agent 记忆在组织内共享（特性不是缺陷 — Bob 的 coder 能复用 Alice 项目中学到的经验）
- agent 级隔离已实现（PR #120），account 级隔离已设计未实现
- 实现前：一个 OpenViking 实例 = 一个组织（部署隔离）

## 实施优先级

| 优先级 | 改动 | 工作量 |
|--------|------|--------|
| P0 | 跨 Project 分解：`DecomposeSpec.ProjectID` | 小 — 一个字段 + handler 逻辑 |
| P1 | 多用户识别：token → submitter + project scope | 中 — config 结构 + 权限检查 |
| P2 | 多实例联通：A2A client 能力 | 大 — 等到有实际需求再做 |

## 总结

| 场景 | 方案 | 原因 |
|------|------|------|
| 同团队多用户多 repo | 一个实例 + 多 project + A2A token | TL 全局视图，跨 project 分解 |
| 不同信任边界 | 各自部署，Git/A2A 联通 | 不能共享 token 和控制权 |
| 规模增长 | 拆实例 + Escalation/Directive 协议 | 按实际瓶颈拆，不提前设计 |
