# Frontend 总览（次基线）

状态：`观察`（按你的要求与后端分开）

## 1. 定位

前端 SPA 用于承载当前可视化工作流。

本目录职责：
- 记录当前前端真实行为与接口调用。
- 标注可保留交互设计与剩余契约债务。
- 不反向约束后端实现。

## 2. 主要视图

- `ChatView`：Team Leader 对话、会话切换、会话事件回放、从文件创建 issue。
- `RunView`：Run 只读监控（列表、状态/结论、事件流、GitHub 关联）。
- `BoardView`：issue 看板视图。
- `A2AChatView`：A2A 任务发送/取消与状态查看。

## 3. 可保留设计（建议保留）

- 会话维度订阅：`subscribe_chat_session` / `unsubscribe_chat_session`
- WS 重连后自动补订阅当前会话
- `run_update` 解析 `acp.sessionUpdate` 展示增量信息
- `VITE_A2A_ENABLED` 控制 Chat/A2A 入口切换

## 4. 运行时环境变量（当前实现）

- `VITE_API_BASE_URL`：默认 `"/api/v1"`
- `VITE_API_TOKEN`：默认空字符串
- `VITE_A2A_ENABLED`：默认开启；当值为 `false/0/off` 时关闭

## 5. 状态管理（Zustand）

- `projectsStore`：`projects/selectedProjectId/loading/error`
- `runsStore`：`RunsByProjectId/selectedRunId/loading/error`
- `chatStore`：`sessionsByProjectId/activeSessionId/loading/error`

## 6. 仅记录不固化

- `web/src/types/workflow.ts` 中仍存在旧 Run 状态语义，和后端 `status + conclusion` 双轴不完全一致。
- `issue` 与 `plan` 双命名别名仍在 API 层存在，属于迁移期兼容。
