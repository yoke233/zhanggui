# 标签分类与治理

版本：v1.0  
状态：Draft  
Owner：Lead / PM

## 1. 目标

- 用有限、稳定的标签集合支撑路由、状态机和质量判定。
- 明确“哪些标签会触发自动化，哪些只是信息展示”。
- 避免不同项目自行发明同义标签导致监听混乱。

## 2. 标签前缀

- `kind:*`：任务类型
- `state:*`：状态机
- `to:*`：路由目标（谁应关注）
- `prio:*`：优先级
- `review:*`：质量审查结论
- `decision:*`：业务/集成最终结论
- 控制标签（无前缀）：`needs-human`、`autoflow:off`

## 3. 最小必备标签（V1）

- `kind:task`
- `state:todo`, `state:doing`, `state:blocked`, `state:review`, `state:done`
- `to:<role>`（如 `to:backend`, `to:frontend`, `to:qa`, `to:integrator`）
- `prio:p0` ~ `prio:p3`
- `review:approved`, `review:changes_requested`
- `decision:accepted`, `decision:rejected`
- `needs-human`, `autoflow:off`

## 4. 监听规则

- 自动化监听只对 `to:*` 与 `state:*` 生效。
- `review:*` 与 `decision:*` 不触发执行，仅触发闸门判定。
- 未在 `roles.enabled` 启用的角色，即使出现 `to:<role>` 也不派发。

## 5. 状态迁移（统一语义）

推荐迁移：

1. `state:todo -> state:doing`：被 claim 并开始执行。
2. `state:doing -> state:review`：提交证据待审。
3. `state:review -> state:done`：审批通过。
4. `state:* -> state:blocked`：出现阻塞。
5. `state:blocked -> state:todo`：阻塞解除后恢复排队。

## 6. blocked 解除规则

统一规则：

- 若 `auto_unblock_when_dependency_closed=true`，依赖关闭时自动 `blocked -> todo`。
- 若配置为 `false`，则需显式 `/unblock` 或等效结构化操作。
- 两种行为由同一个配置项决定，不允许并存双重判定。

## 7. 治理规则

- 标签定义真源：`<outbox_repo>/workflow.toml` 的 `[labels]` 段。
- 本文与 `workflow.toml` 不一致时，以 `workflow.toml` 为准并同步修文档。
- 新增标签前必须写明：目的、触发方、监听方、是否影响状态机。

