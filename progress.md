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
- 当前进入 `task-020`：review Proposal 操作面并执行 build 验证。
