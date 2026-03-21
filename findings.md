# Findings

## 2026-03-21

- 路由层当前只暴露了 `RequirementPage`、`ThreadsPage`、`ThreadDetailPage`、`WorkItemsPage`、`WorkItemDetailPage`，没有 `Proposal` 或 `Initiative` 页面入口。
- `RequirementPage` 已支持 `analyzeRequirement` 和 `createThreadFromRequirement`，并允许选择项目、agents、meeting mode、meeting rounds。见 `web/src/pages/RequirementPage.tsx:79`、`web/src/pages/RequirementPage.tsx:126`。
- `ThreadsPage` 仍是“创建 thread + 发首条消息”的轻入口，另有“从需求创建”跳转，但不承接 proposal/initiative 审批。见 `web/src/pages/ThreadsPage.tsx:71`、`web/src/pages/ThreadsPage.tsx:108`、`web/src/pages/ThreadsPage.tsx:150`。
- `ThreadDetailPage` 已支持发消息、邀请 agent、文件引用、summary、从 thread 创建 work item、链接既有 work item、task group 管理，但没有 proposal/initiative 列表、详情或审批操作。见 `web/src/pages/ThreadDetailPage.tsx:1224`、`web/src/pages/ThreadDetailPage.tsx:1360`、`web/src/pages/ThreadDetailPage.tsx:1378`、`web/src/pages/ThreadDetailPage.tsx:1402`、`web/src/pages/ThreadDetailPage.tsx:1426`、`web/src/pages/ThreadDetailPage.tsx:1437`、`web/src/pages/ThreadDetailPage.tsx:1555`、`web/src/pages/ThreadDetailPage.tsx:1576`。
- `ThreadDetailsPanel` 组件已有 summary 和 “Create from Summary / Link existing” 交互稿，但当前未被任何页面引用，说明 thread 详情侧边信息面存在未接线的 UI 资产。见 `web/src/components/threads/ThreadDetailsPanel.tsx:46`；全局未搜到引用。
- `WorkItemDetailPage` 已展示来源 thread、依赖 work items，并支持 run/cancel/edit，但没有来源 proposal / initiative 信息，也没有 gate 决策 UI。见 `web/src/pages/WorkItemDetailPage.tsx:248`、`web/src/pages/WorkItemDetailPage.tsx:278`、`web/src/pages/WorkItemDetailPage.tsx:328`。
- 前端 `apiClient` 已接 `requirements/*`、`threads/*`、`work-items/*`，但没有任何 `proposal` 和 `initiative` 类型与 API 方法；`apiV2.ts` 也没有相应结构体。这是主链 UI 断点。见 `web/src/lib/apiClient.ts` 中已存在 `/requirements/create-thread`、`/threads/*`、`/work-items/*`，但无 `/proposals/*`、`/initiatives/*`；`web/src/types/apiV2.ts` 同样缺失 `Proposal` / `Initiative` 类型。
- 后端实际上已经提供完整的 `proposal` / `initiative` 路由，因此缺口主要在前端接入而不是后端能力。见 `internal/adapters/http/proposal.go:39`、`internal/adapters/http/initiative.go:43`。
- 已完成 Phase 1 契约层补齐：`web/src/types/apiV2.ts` 新增 `ThreadProposal`、`ProposalWorkItemDraft`、`Initiative`、`InitiativeDetail`、`InitiativeItem`、`ThreadInitiativeLink` 及请求体类型；`web/src/lib/apiClient.ts` 新增 `/threads/{id}/proposals`、`/proposals/*`、`/initiatives/*` 的访问方法；`web/src/lib/apiClient.test.ts` 已覆盖 proposal / initiative 路由命中。
- 已完成 Phase 2 第一轮实现：`ThreadDetailPage` 现在会加载 thread proposals，并在右侧 sidebar 提供 `New Proposal`、草案编辑、draft 明细、`Submit/Approve/Reject/Revise` 动作；`ThreadDetailPage.test.tsx` 已覆盖创建 proposal、编辑保存和提交审批。
