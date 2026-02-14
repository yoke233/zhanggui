# Delivery Control (交付层控制)

目标：让“有了 Spec”之后，交付可以稳定推进并闭环，尤其是多 repo、多角色、多 worker 并行时。

本层关注的不是“写代码技巧”，而是：

- 队列与节奏：先做什么、什么时候交付
- 路由与闭环：谁负责、谁合并、谁验收
- 证据回填：Issue 能回放发生过什么

## 角色与职责边界

### Project Manager（项目经理）

负责管理 Lead 的队列与节奏：

- 优先级排序（P0/P1/P2）
- 里程碑与交付节奏
- 资源冲突（同一模块多人抢占、跨仓依赖）
- 风险/阻塞升级（需要管理层或外部团队介入）

PM 管：

- 做什么
- 何时交付
- 谁负责（指派到 Lead/角色）

PM 不管：

- 怎么实现
- 怎么合并
- 具体修哪个文件

### Lead（Tech Lead / Integrator）

负责管理 Worker 的路由与闭环：

- 拆任务（把 Spec 拆成可独立交付的子 Issue）
- 派工（spawn worker 到正确 repo/workdir）
- 阻塞处理（DependsOn/BlockedBy/needs-human）
- 证据回填（PR/commit/CI/test）
- 合并/发布收敛（由 Integrator/Lead 负责最终落地）

关键原则：

- PM 管 Lead 是对的：PM 不应直接指挥 Worker，否则指挥链混乱。
- Lead 的核心职责是“标准化写回”，而不是“把所有实现都自己写完”。

## 真源与接口（Single Source of Truth）

交付层有三个事实真源：

1. 配置真源：`<outbox_repo>/.agents/workflow.toml`
2. 协作真源：Issue（`IssueRef`）
3. 代码真源：PR/commit（以及 CI/test 证据）

其它聊天记录、口头承诺都不算真源。

## 结构化写回：Lead 单写者（推荐）

为了避免 worker 输出不规范导致 Outbox 记录分叉，推荐：

- Worker 只回传“原始素材”（PR 链接、命令结果、日志、疑问）
- Lead/Integrator/Recorder 负责把素材转换为固定模板 comment 并写回 Outbox

见：

- `docs/workflow/lead-worker.md`
- `docs/workflow/outbox-and-mailbox-skill.md`

## 状态与标签（Lean 模式）

为了能“真正开工”，我们把条件分成 Hard/Soft：

- Hard：assignee（claim 真源）、IssueRef、PR/commit Evidence、Close 闭环
- Soft：`state:*` 标签（推荐补齐以保持队列清晰，但 Phase 1 不应阻塞开工）

详细协议见：

- `docs/workflow/v1.1.md`
- `docs/workflow/issue-protocol.md`
