# Progress

## 2026-03-21

- 创建规划文件，开始梳理前端界面操作补齐计划。
- 已提交文档说明：`83faae9 docs(spec): capture thread plan review chain`
- 已完成现状盘点：Requirement/Thread/WorkItem 页面已接；Proposal/Initiative 页面、类型、API 未接。
- 已补正式计划文档：`docs/plan/2026-03-21-frontend-thread-proposal-initiative-plan.zh-CN.md`
- 已完成 `task-017`：补齐 `web/src/types/apiV2.ts` 与 `web/src/lib/apiClient.ts` 的 Proposal / Initiative 契约，并新增 `apiClient` 路由测试。
- 已通过验证：`npm --prefix web test -- --run src/lib/apiClient.test.ts`
- 已完成 `task-018` review：未发现需要修复的 correctness 问题；再次通过 `npm --prefix web test -- --run src/lib/apiClient.test.ts` 与 `pwsh -NoProfile -File .\\scripts\\test\\frontend-build.ps1`。
- 已完成 `task-019`：在线程页右侧 sidebar 增加 Proposal 区，支持创建、编辑、提交、审批与驳回/返修动作。
- 已通过验证：`npm --prefix web test -- --run src/pages/ThreadDetailPage.test.tsx src/lib/apiClient.test.ts`
- 已完成 `task-020`：补 Proposal 输入校验，拦截非法 `source_message_id`，并再次通过 ThreadDetailPage/apiClient 测试与前端 build。
- 已完成 `task-021`：新增 `web/src/pages/InitiativeDetailPage.tsx`，接入 `/initiatives/:initiativeId` 路由，并在线程 proposal 卡片上增加 initiative 跳转入口。
- 已新增测试：`web/src/pages/InitiativeDetailPage.test.tsx`，覆盖详情加载、审批动作、跨路由切换时审批表单刷新。
- review 阶段发现并修复一个状态串页问题：同一组件实例在不同 initiative 间切换时，右侧审批表单会残留前一个 initiative 的 reviewer / note。
- 已通过验证：
  - `npm --prefix web test -- --run src/pages/InitiativeDetailPage.test.tsx src/pages/ThreadDetailPage.test.tsx src/pages/WorkItemDetailPage.test.tsx src/lib/apiClient.test.ts`
  - `pwsh -NoProfile -File .\\scripts\\test\\frontend-build.ps1`
- 当前前端补齐任务 `task-017` ~ `task-022` 已全部完成。
