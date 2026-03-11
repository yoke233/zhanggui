# V3 前端后端接口覆盖清单

更新时间：2026-03-09

本文档记录当前 `web/src/v3/**` 与 v3 主入口 [App.tsx](D:/project/ai-workflow/.worktrees/web/web/src/App.tsx) 的后端接口接入情况，目的是明确：

- 哪些接口已经被 v3 页面实际使用
- 哪些接口只完成了部分接入
- 哪些接口仍未暴露到 v3 UI

结论先行：

- v3 五个主页面的核心主流程已经接上后端
- 当前不是“所有后端接口都已经前端化”
- 目前状态更准确地说是“主流程已接，低频控制项仍有缺口”

## 1. 总览页

实现文件：

- [CommandCenterView.tsx](D:/project/ai-workflow/.worktrees/web/web/src/views/CommandCenterView.tsx)
- [OverviewView.tsx](D:/project/ai-workflow/.worktrees/web/web/src/v3/views/OverviewView.tsx)

已接入接口：

- `getStats`
- `listIssues`
- `listRuns`

当前用途：

- 拉取全局统计卡片
- 展示 Issue 焦点摘要
- 展示 Run 健康摘要

状态判断：

- 已接入核心只读接口
- 没有额外动作型接口

## 2. 项目 / Issue 工作台

实现文件：

- [IssuesView.tsx](D:/project/ai-workflow/.worktrees/web/web/src/v3/views/IssuesView.tsx)

已接入接口：

- `listIssues`
- `decompose`
- `confirmDecompose`
- `getIssueDag`
- `listIssueTimeline`

当前用途：

- 拉取 Issue 队列
- 基于一句话需求生成 Proposal DAG
- 确认 Proposal 并批量创建 Issue
- 展示当前焦点 Issue 的 DAG 摘要
- 展示当前焦点 Issue 的时间线

状态判断：

- 已覆盖 v3 设计稿要求的主流程
- 当前仍偏重“拆解与确认”
- 还没有把更多 Issue 深层操作统一展开到 v3 页面

## 3. 会话 / 线程工作区

实现文件：

- [SessionsView.tsx](D:/project/ai-workflow/.worktrees/web/web/src/v3/views/SessionsView.tsx)

已接入接口：

- `listChats`
- `listChatRunEvents`
- `createChat`
- `listAgents`

当前用途：

- 拉取会话收件箱
- 展示当前会话的消息/事件流
- 创建新会话
- 拉取 Agent 列表并作为新建会话时的可选项

部分接入但未完整前端化的接口：

- `getChatEventGroup`
- `getSessionCommands`
- `getSessionConfigOptions`
- `setSessionConfigOption`
- `cancelChat`

说明：

- 这些接口在旧会话页体系里已有更完整使用
- 但在当前 v3 会话页中，还没有全部做成独立的控制区或高级面板

状态判断：

- 核心浏览与创建链路已接入
- 高级控制、事件组钻取、会话配置仍未完全铺开

## 4. Run 详情与事件流

实现文件：

- [RunsView.tsx](D:/project/ai-workflow/.worktrees/web/web/src/v3/views/RunsView.tsx)

已接入接口：

- `listRuns`
- `listRunEvents`
- `getRunCheckpoints`

当前用途：

- 拉取 Run 列表
- 展示 Run 事件时间线
- 展示检查点、阶段状态与错误信息

部分接入但未完整前端化的接口：

- `wakeStageSession`
- `getStageSessionStatus`
- `promptStageSession`

说明：

- 当前 v3 Run 页已经具备设计稿要求的三栏主信息结构
- 但右侧“阶段会话控制”仍然主要是只读信息和诊断摘要
- 尚未把阶段唤醒、会话状态轮询、prompt 注入做成可操作区

状态判断：

- Run 的主读链路已接入
- Run 的低频控制链路未完全前端化

## 5. 协议 / 审计 / 运维控制台

实现文件：

- [OpsView.tsx](D:/project/ai-workflow/.worktrees/web/web/src/v3/views/OpsView.tsx)

已接入接口：

- `createProjectCreateRequest`
- `getProjectCreateRequest`
- `listWorkflowProfiles`
- `listAdminAuditLog`
- `forceIssueReady`
- `forceIssueUnblock`
- `sendSystemEvent`

当前用途：

- 创建项目
- 轮询项目创建状态
- 查看 workflow profiles
- 查看审计记录
- 执行 `force ready`
- 执行 `force unblock`
- 发送系统事件

状态判断：

- 该页是当前 v3 中后端动作接口接入最完整的一页
- 已承担项目初始化、审计查看和高权限操作收口职责

## 6. App 层项目上下文与实时刷新

实现文件：

- [App.tsx](D:/project/ai-workflow/.worktrees/web/web/src/App.tsx)

已接入接口 / 通道：

- `listProjects`
- WebSocket `subscribe("*")` 增量刷新

当前用途：

- 拉取项目列表
- 维护当前项目上下文
- 在 Issue / Run 相关事件到达时触发页面刷新

状态判断：

- 已完成 v3 主入口级接入

## 7. 当前未在 v3 页面完整暴露的重点接口

以下接口不是“后端没有”，而是“当前 v3 UI 还没有完整接进去或只在旧页面存在”：

- `getChatEventGroup`
- `getSessionCommands`
- `getSessionConfigOptions`
- `setSessionConfigOption`
- `cancelChat`
- `wakeStageSession`
- `getStageSessionStatus`
- `promptStageSession`

## 8. 汇总结论

按页面看：

- 总览页：核心只读接口已接
- 项目 / Issue 页：拆解与 DAG 主流程已接
- 会话页：核心浏览与创建已接，高级控制未全接
- Run 页：核心读链路已接，阶段控制未全接
- 运维页：创建、审计、高权限动作已较完整接入
- App 层：项目上下文与 WebSocket 刷新已接

按整体判断：

- 当前可以说“v3 主页面已经对接上后端核心接口”
- 不能说“v3 前端已经把所有后端接口全部接完”

## 9. 后续补齐建议

优先级建议如下：

1. 会话页补齐高级控制
   - `getChatEventGroup`
   - `getSessionCommands`
   - `getSessionConfigOptions`
   - `setSessionConfigOption`
   - `cancelChat`

2. Run 页补齐阶段会话控制
   - `getStageSessionStatus`
   - `wakeStageSession`
   - `promptStageSession`

3. 在文档更新时保持“按页面归属”而不是“按 API 文件归属”
   - 这样更适合 UI 设计复刻和联调排查
