# Web 产品面现状总览

> 状态：现行
>
> 最后按代码核对：2026-04-03
>
> 适用范围：本文描述当前 `web/` 前端已经落地的页面面、
> 契约面与事件消费面，不描述未来设计。

## 一句话结论

当前前端已经不是“WorkItem + Thread 的几个页面”，而是一个完整工作台：

- 通用工作台
- 监控域
- 运行时域

其中 `ChatSession`、`Thread`、`WorkItem` 是三条并行主线。

## 技术栈与运行壳

当前前端基于：

- React 18
- Vite
- TypeScript
- `react-router-dom`
- `zustand`
- `i18next`
- Wails desktop bridge

运行壳由 `WorkbenchContext` 统一提供：

- token 获取与登录态探测
- 当前项目选择
- API Client 注入
- WebSocket Client 注入
- 桌面端 Wails `GetBootstrap()` 适配

## 当前页面域

### 1. 通用工作台

当前主路由包括：

- `/`
- `/chat`
- `/work-items`
- `/work-items/new`
- `/work-items/:workItemId`
- `/threads`
- `/threads/:threadId`
- `/initiatives/:initiativeId`
- `/requirements/new`
- `/projects`
- `/projects/new`
- `/projects/:projectId/git-tags`
- `/projects/:projectId/manifest`
- `/settings`
- `/runs/:runId`

### 2. 监控域

当前监控域挂在 `/monitoring/*` 下：

- `/monitoring/dashboard`
- `/monitoring/analytics`
- `/monitoring/usage`
- `/monitoring/inspections`
- `/monitoring/scheduled-tasks`

旧入口如 `/dashboard`、`/analytics`、`/usage` 目前只是 redirect。

### 3. 运行时域

当前运行时域挂在 `/runtime/*` 下：

- `/runtime/agents`
- `/runtime/skills`
- `/runtime/templates`
- `/runtime/sandbox`

旧入口如 `/agents`、`/skills`、`/templates`、`/sandbox`
当前也是 redirect。

## 三条主交互线

### ChatSession

`ChatSession` 当前代表 direct chat 入口，不与 Thread 合并。

当前已落地能力包括：

- 会话列表与详情
- 流式输出消费
- profile / driver 选择
- 权限请求展示
- 会话取消、关闭、重命名、归档
- Create PR / Refresh PR

当前 chat 侧既消费 REST，也消费 WebSocket 事件。

### Thread

`Thread` 当前代表多人/多 agent 协作容器。

当前已落地能力包括：

- Thread 创建、列表、详情、更新、删除
- 消息发送与消息列表
- `@agent` 定向发送
- `mention_only` / `broadcast` / `auto` 路由模式展示
- 参与者管理
- agent 邀请与移除
- Thread 与 WorkItem 关联
- 从 Thread 直接创建 WorkItem
- context refs 管理
- 附件上传/下载
- workspace / project / attachment 文件搜索
- proposal / initiative 展示与审批操作

### WorkItem

当前 WorkItem 页面已经全面切到 `/work-items` 主入口。

当前已落地能力包括：

- WorkItem 列表
- WorkItem 创建
- WorkItem 详情与更新
- WorkItem Inbox 视图，可按处理人查看待审核 / 待返工 / 待上级处理项
- Inbox 内直接 approve / reject / unblock 待处理 Action
- 运行与取消
- 自动生成标题
- 自动生成 Action
- 上传附件
- 反查关联 Thread
- 展示来源 Thread、关联 Thread、依赖 WorkItem
- 展示 WorkItem deliverables，并显式采纳 `final deliverable`
- 从 DAG Template 生成 WorkItem

## 监控与配置面

### 监控面

当前前端已具备以下监控/观察页面：

- Dashboard
- Analytics
- Usage
- Inspection
- Scheduled Tasks

这表示当前产品面已经包含运行观察与运营分析，不再只是执行面 UI。

### 配置面

当前前端已具备以下运行时配置页面：

- Agents：driver / profile / LLM config 组合管理
- Skills：技能 CRUD 与 GitHub import
- Templates：DAG 模板管理与实例化
- Sandbox：沙箱支持配置

## 当前 REST 契约事实

主 API 客户端位于 `web/src/lib/apiClient.ts`。

当前前端真实消费的主 REST 面包括：

- `/work-items`
- `/work-items/pending`
- `/work-items/*/deliverables`
- `/work-items/*/final-deliverable`
- `/threads`
- `/threads/*/deliverables`
- `/chat`
- `/projects`
- `/requirements`
- `/threads/*/proposals`
- `/proposals`
- `/initiatives`
- `/templates`
- `/skills`
- `/analytics`
- `/inspections`
- `/notifications`
- `/agents/drivers`
- `/agents/profiles`
- `/themes`

补充事实：

- 前端主 API client 已以 `/work-items` 为主，不再主用 `/issues`
- WorkItem Inbox 当前通过 `GET /work-items/pending?profile_id=...` 拉取
- WorkItem 详情页当前通过 `final_deliverable_id` + deliverables 列表展示“当前最终结果”
- Thread 文件搜索支持 `source=attachment|project|workspace|all`
- Thread 成员统一模型是 `ThreadMember`

## 当前 WebSocket 事件面

当前前端明确消费的事件面至少包括：

- chat 会话流式事件
- thread 事件
- notification 事件
- system event

从交互模型看：

- `chat.send` 继续用于 direct chat
- `thread.send` 用于 Thread 协作消息
- 两者不是 alias，而是两套独立事件链路

## 类型层现状

当前前端主契约类型以 `web/src/types/apiV2.ts` 为主。

当前事实：

- `apiV2.ts` 是主页面与主 API client 的 barrel export，真实定义分布在 `api-v2/*`
- 前端协作侧直接使用 `ThreadMember`
- `ThreadAgentSessionStatus` 只是状态类型，不代表独立 session 实体
- WorkItem 主术语已经对齐，但部分旧 `issue` 语义仍保留在兼容层

## Wails 桌面端现状

当前桌面端已实现。

实际工作台使用上：

- 前端通过 Wails binding `DesktopApp.GetBootstrap()` 获取 `token`
- 当前主工作台仍统一依赖默认同源 `api_base_url=/api`
- WebSocket 基址仍由 `/api + /ws` 推导，而不是桌面端单独注入

## 当前阅读方式建议

如果你想快速理解前端现状，建议按这个顺序看：

1. `web/src/App.tsx`
2. `web/src/contexts/WorkbenchContext.tsx`
3. `web/src/lib/apiClient.ts`
4. `web/src/lib/wsClient.ts`
5. `web/src/pages/ChatPage.tsx`
6. `web/src/pages/ThreadDetailPage.tsx`
7. `web/src/pages/WorkItemDetailPage.tsx`

## 与其他 spec 的关系

本文是“产品面与契约面总览”，建议与以下专题配合阅读：

1. `naming-transition-thread-workitem.zh-CN.md`
2. `thread-agent-runtime.zh-CN.md`
3. `thread-workitem-linking.zh-CN.md`
4. `thread-plan-review-chain.zh-CN.md`
5. `tauri-desktop.md`
