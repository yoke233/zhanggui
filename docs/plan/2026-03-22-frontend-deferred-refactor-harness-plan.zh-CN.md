# 2026-03-22 前端 deferred refactor harness 计划

## 背景

这批重构已经确认要做，但不适合塞进当前实现任务里直接展开：

1. `ThreadDetailPage.tsx` 拆分
2. `ChatPage.tsx` 拆分
3. `ThreadSidebar.tsx` prop drilling / context 收敛
4. Store 大接口拆分
5. 全局 Error Boundary
6. 消息列表虚拟化

本次目标不是直接开做，而是把它们整理成可恢复、可分阶段推进、可验证的 `harness` backlog。

## 当前基线

2026-03-22 仓库内前端现状：

- `web/src/pages/ThreadDetailPage.tsx` 约 `2478` 行
- `web/src/pages/ChatPage.tsx` 约 `1433` 行
- `web/src/components/threads/ThreadSidebar.tsx` 约 `1253` 行
- `ThreadSidebar` 当前只在 `ThreadDetailPage` 中被直接渲染
- `ThreadSidebarProps` 已经同时承载 members / proposals / work items / files / info 五组职责
- Chat 消息主链路是 `ChatPage -> ChatMainPanel -> MessageFeedView -> entries.map(...)`
- Thread 消息主链路是 `ThreadDetailPage -> ThreadMessageList -> messages.map(...)`
- `web/src/main.tsx` 当前只有 `React.StrictMode`，没有根级 Error Boundary

这些点说明：

1. 页面拆分应该先于 store/interface 收敛，否则边界会反复重画
2. `ThreadSidebar` 适合做局部 context，而不是先引入新的全局 store
3. 消息列表虚拟化应该落在列表容器层，而不是直接改消息项组件
4. Error Boundary 是独立改造项，可以并行于页面拆分推进

## 执行策略

### Phase 0: 基线校准

先冻结当前文件边界、状态入口、渲染区块和验证命令，避免后续多人并行时出现“拆分目标不断变化”。

对应 harness：

- `task-001`

完成标准：

- 大文件的职责边界、候选拆分点、依赖顺序写入本计划
- 后续任务不再临时扩 scope

### Phase 1: 页面壳层拆分

先拆页面壳和纯展示区块，后拆数据与副作用。

对应 harness：

- `task-002`
- `task-003`

拆分原则：

1. 页面文件只保留路由接线、顶层状态编排、少量桥接逻辑
2. 纯展示组件先拆，副作用 hook 后拆
3. 不在这一阶段引入新的跨页面抽象

### Phase 2: Sidebar / Store 边界收敛

在页面壳层稳定后，再处理 `ThreadSidebar` 的 prop drilling 和大接口问题。

对应 harness：

- `task-004`
- `task-005`

执行原则：

1. 先建立 `ThreadSidebar` 局部 context，收敛高频透传字段
2. 再把 Thread / Chat 页面中的大接口按业务域拆成更小的 selector / action 边界
3. 尽量避免一次性把局部状态提升成全局 store

### Phase 3: 稳定性与性能

这一阶段处理与功能边界正交的系统性改造。

对应 harness：

- `task-006`
- `task-007`
- `task-008`

执行原则：

1. `RootErrorBoundary` 先放在 `web/src/main.tsx`
2. 列表容器先统一接口，再接虚拟化
3. 虚拟化先覆盖 Chat 主消息流，再评估是否扩到 Thread 详情时间线

## 任务顺序建议

建议按以下顺序推进：

1. `task-001` 校准基线
2. `task-002` 拆 `ThreadDetailPage`
3. `task-003` 拆 `ChatPage`
4. `task-004` 收敛 `ThreadSidebar` prop drilling
5. `task-006` 接 `RootErrorBoundary`
6. `task-007` 抽离消息列表容器
7. `task-008` 接入虚拟化
8. `task-005` 拆大接口状态边界
9. `task-009` 做长会话性能回归与跨层验收

这样排的原因：

1. 页面拆分先落地，才能看清哪些 state/interface 真的值得提升或复用
2. Error Boundary 与页面拆分正交，不必阻塞
3. 虚拟化必须建立在稳定的列表容器边界之上
4. 大接口拆分放在较后阶段，避免一边抽组件一边改状态协议

## 验证策略

所有 harness 任务都已经写入 `harness-tasks.json`，每个任务至少包含一个客观验证命令。

当前统一采用项目内既有脚本：

- `.\scripts\test\frontend-build.ps1`
- `.\scripts\test\frontend-unit.ps1`
- `.\scripts\test\frontend-e2e.ps1`
- `.\scripts\test\suite-p3.ps1`

约束：

1. 页面/组件拆分至少跑 `frontend-unit.ps1` + `frontend-build.ps1`
2. 虚拟化落地必须补 `frontend-e2e.ps1`
3. 大接口拆分或跨层状态改动，最终必须过 `suite-p3.ps1`

## 风险边界

主要风险：

1. 页面拆分时把副作用和纯展示一起迁移，导致回归面过大
2. `ThreadSidebar` context 收敛不当，反而掩盖依赖关系
3. 消息虚拟化破坏现有“自动滚到底部 / 向上加载更多 / 流式追加”行为
4. Error Boundary fallback 设计过粗，导致问题被吞掉而不是显式暴露

控制方式：

1. 每个 harness session 最多推进 2 个任务
2. 先拆边界，再抽公共层，再做性能优化
3. 回归失败时严格回滚到 `started_at_commit`

## 下一步

当前 backlog 已完成初始化。后续真正开做时，建议直接从：

1. `/harness run`
2. 或手动把 `task-001` 设为 `in_progress` 后按 `harness` 协议执行

不要跳过 `task-001`，否则后面的页面拆分和 store/interface 收敛会很容易重新发散。
