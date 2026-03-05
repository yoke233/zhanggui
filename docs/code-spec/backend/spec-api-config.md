# Backend API 规范（代码事实版）

状态：`保留`（以可用路由为准）

## 1. 路由分层

- `/`：系统健康检查入口
- `/api/v1`：写操作与控制面
- `/api/v2`：只读查询面（issue/run/profile）
- `/webhook`：GitHub webhook 入口

## 2. 系统与发现接口

- `GET /health`
- `GET /api/v1/health`
- `GET /api/v1/stats`
- `GET /.well-known/agent-card.json`
- `POST /api/v1/a2a`

说明：
- `POST /api/v1/a2a` 由 A2A token 独立鉴权，不等同于 `server.auth_enabled` 的 Bearer 鉴权开关。

## 3. `/api/v1`（主写路径）

### Project
- `GET /api/v1/projects`
- `POST /api/v1/projects`
- `POST /api/v1/projects/create-requests`
- `GET /api/v1/projects/create-requests/{id}`
- `GET /api/v1/projects/{id}`

### Repo
- `GET /api/v1/projects/{projectID}/repo/tree?dir=...`
- `GET /api/v1/projects/{projectID}/repo/status`
- `GET /api/v1/projects/{projectID}/repo/diff?file=...`

### Chat（Team Leader 会话入口）
- `GET /api/v1/projects/{projectID}/chat`
- `POST /api/v1/projects/{projectID}/chat`
- `POST /api/v1/projects/{projectID}/chat/{sessionID}/cancel`
- `GET /api/v1/projects/{projectID}/chat/{sessionID}/events`
- `GET /api/v1/projects/{projectID}/chat/{sessionID}`
- `DELETE /api/v1/projects/{projectID}/chat/{sessionID}`

### Issue（当前写入口）
- `POST /api/v1/projects/{projectID}/issues`
- `POST /api/v1/projects/{projectID}/issues/from-files`
- `GET /api/v1/projects/{projectID}/issues`
- `GET /api/v1/projects/{projectID}/issues/{id}`
- `GET /api/v1/projects/{projectID}/issues/{id}/dag`
- `GET /api/v1/projects/{projectID}/issues/{id}/reviews`
- `GET /api/v1/projects/{projectID}/issues/{id}/changes`
- `GET /api/v1/projects/{projectID}/issues/{id}/timeline`
- `POST /api/v1/projects/{projectID}/issues/{id}/review`
- `POST /api/v1/projects/{projectID}/issues/{id}/action`
- `POST /api/v1/projects/{projectID}/issues/{id}/auto-merge`

### Admin
- `POST /api/v1/admin/ops/force-ready`
- `POST /api/v1/admin/ops/force-unblock`
- `POST /api/v1/admin/ops/replay-delivery`
- `GET /api/v1/admin/audit-log`

### WS
- `GET /api/v1/ws`

## 4. `/api/v2`（当前读路径）

- `GET /api/v2/issues?project_id=...`
- `GET /api/v2/issues/{id}`
- `GET /api/v2/workflow-profiles`
- `GET /api/v2/workflow-profiles/{type}`
- `GET /api/v2/runs?project_id=...`
- `GET /api/v2/runs/{id}`
- `GET /api/v2/runs/{id}/events`

## 5. 前端调用现状（后端视角）

前端当前调用面：
- issue/run 查询：走 `/api/v2/*`
- issue/chat/repo/admin 写调用：走 `/api/v1/*`
- A2A：走 `/api/v1/a2a`
- 实时事件：走 `/api/v1/ws`

前端中仍存在历史方法别名（`createPlan/listPlans`），但本质调用 issue 接口。

## 6. 明确不纳入（当前未落地或无路由）

- `/api/v2/sessions/*`（不存在）
- `POST /api/v2/issues` / `POST /api/v2/runs`（不存在）
- `POST /api/v1/projects/{projectID}/issues/{id}/actions`（错误复数路径）
- `PATCH /api/v1/projects/{projectID}/issues/{id}/auto-merge`（错误方法）

说明：
- 上述接口可在未来重构时重新设计，但当前不得写入“已实现规范”。
