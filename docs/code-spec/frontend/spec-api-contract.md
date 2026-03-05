# Frontend API 契约现状（以调用代码为准）

状态：`观察`

## 1. 当前“可用主路径”

前端调用分为三类：
- REST API Client（`/api/v1` + `/api/v2`）
- WebSocket（`/api/v1/ws`）
- A2A JSON-RPC（`/api/v1/a2a`）

### 高频 REST 调用（当前有效）
- `GET /api/v2/issues`
- `GET /api/v2/issues/{id}`
- `GET /api/v2/runs`
- `GET /api/v2/runs/{id}`
- `GET /api/v2/runs/{id}/events`
- `GET /api/v2/workflow-profiles`
- `GET /api/v2/workflow-profiles/{type}`
- `POST /api/v1/projects/{projectID}/issues`
- `POST /api/v1/projects/{projectID}/issues/from-files`
- `POST /api/v1/projects/{projectID}/issues/{id}/review`
- `POST /api/v1/projects/{projectID}/issues/{id}/action`
- `POST /api/v1/projects/{projectID}/issues/{id}/auto-merge`
- `GET /api/v1/projects/{projectID}/chat*`
- `POST /api/v1/projects/{projectID}/chat*`
- `GET /api/v1/projects/{projectID}/repo/*`
- `GET /api/v1/admin/audit-log`

### WS 客户端消息（当前有效）
- `subscribe_run` / `unsubscribe_run`
- `subscribe_issue` / `unsubscribe_issue`
- `subscribe_chat_session` / `unsubscribe_chat_session`

## 2. 兼容别名（迁移期保留）

- `createPlan/listPlans/getPlanDag/...` 在前端是 issue 接口别名封装。
- `applyTaskAction` 目前等价调用 `POST /issues/{id}/action`。

建议：
- 新增页面与新代码统一使用 issue 命名。
- 别名仅作为存量兼容层，不再扩散。

## 3. 已清理的历史兼容层

以下 run 兼容方法已从前端代码移除，不再作为契约：
- `createRun`
- `getRunLogs`
- `getRunCheckpoints`
- `applyRunAction`

## 4. 规范建议

前端 API 契约应分为：
- `implemented`（可调用）
- `legacy_alias`（迁移保留）
- `removed_compat`（已下线，不可回归）
