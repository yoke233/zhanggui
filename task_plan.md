# 前端界面操作补齐计划

状态：in_progress
最后更新：2026-03-21

## 目标

基于当前 `Requirement -> Thread -> Proposal -> Initiative -> WorkItem` 主链，梳理并规划前端还缺哪些界面操作、页面入口、状态反馈与联动刷新，形成一份可执行的前端补齐计划。

## 阶段

| 阶段 | 状态 | 说明 |
|---|---|---|
| Phase 1 | completed | 盘点当前前端页面、关键入口与后端主链 |
| Phase 2 | completed | 识别缺失的用户操作、状态展示和跳转路径 |
| Phase 3 | completed | 形成分阶段实施计划、验证策略与风险清单 |

## 约束

- 本轮先做规划，不直接改前端代码。
- 只描述当前实现与明确缺口，不凭空扩展不存在的产品面。
- 优先覆盖真实主链：Requirement、Thread、Proposal、Initiative、WorkItem。

## 已知背景

- `thread` 计划后的审核不在会议内部自动完成，而是落在 `proposal` / `initiative` / `gate` 三层。
- 真实流程与回归测试已经打通。
- 用户希望开始规划“前端界面操作这些内容”的补齐方案。

## 当前执行

- `task-017` 已完成实现与验证：补齐 `Proposal / Initiative` 前端类型、`apiClient` 契约和路由命中测试。
- `task-018` 已完成 review：未发现需要修复的 correctness 问题，单测与 build 已通过。
- `task-019` 已完成实现与验证：在线程页侧栏补 proposal 列表、草案编辑、提交与审批动作。
- `task-020` 已完成 review/fix：补上 proposal 输入的数字校验与回归测试，前端 build 通过。
- 当前进入 `task-021`：新增 Initiative 详情页与审批入口。

## 本轮结论

- 当前前端已经覆盖 `Requirement -> Thread -> WorkItem` 的部分链路，但没有覆盖 `Proposal -> Initiative` 审批面。
- 已补一份正式计划文档：`docs/plan/2026-03-21-frontend-thread-proposal-initiative-plan.zh-CN.md`
