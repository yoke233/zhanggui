# V2 API 规范（Issue / WorkflowProfile / WorkflowRun / Team Leader）

## 约定

- Base：`/api/v2`
- 鉴权：Bearer（按部署配置）
- 错误体：`{ "error": "...", "code": "...", "details": {} }`
- 所有写接口必须返回可追踪 ID（`issue_id` / `run_id` / `session_id`）
- 除 `/projects` 外，资源统一使用 `project_id` 关联所属项目（path/query/body）

## 项目

### 创建项目

`POST /projects`

```json
{
  "name": "demo",
  "repo_path": "D:/repo/demo",
  "default_branch": "main"
}
```

- `default_branch` 可选；为空时服务端自动检测仓库当前分支（fallback `main`）。

### 查询项目

- `GET /projects`
- `GET /projects/{projectID}`

响应包含 `default_branch` 字段，表示该项目所有 run 使用的 base branch。

## Team Leader 会话

### 发起/续接会话并触发编排

`POST /sessions`

```json
{
  "project_id": "proj-xxx",
  "message": "请拆分 issue 并给出执行建议",
  "session_id": "",
  "workflow_profile": "normal",
  "role": "team_leader"
}
```

说明：

- `session_id` 为空表示新会话；非空表示续接。
- `workflow_profile` 默认 `normal`。
- `role` 默认 `team_leader`，仅用于角色绑定，不等于流程档位。

### 取消会话中的活跃 run

`POST /sessions/{sessionID}/runs/cancel?project_id={projectID}`

### 查询会话 run 事件

`GET /sessions/{sessionID}/runs/events?project_id={projectID}`

返回关键字段：

- `run_id`
- `event_type`：`run_created | run_started | run_updated | run_waiting_review | run_completed | run_failed | run_timeout | run_cancelled`
- `update_type`
- `payload`
- `created_at`

## Issue

### 创建 issue（文本模式）

`POST /issues`

```json
{
  "project_id": "proj-xxx",
  "session_id": "sess-xxx",
  "input_mode": "text",
  "title": "auth-refactor",
  "body": "重构认证流程",
  "workflow_profile": "strict",
  "auto_merge": false
}
```

校验规则：

- `input_mode` 必须为 `text`。
- `title` 必填，去除首尾空白后长度必须在 `1..120`。
- `body` 必填，`file_paths` 禁止出现。
- 校验失败返回 `400 INVALID_ISSUE_INPUT_MODE`、`400 INVALID_ISSUE_TITLE` 或 `400 INVALID_ISSUE_BODY`。

### 基于文件创建 issue 并进入 review（文件模式）

`POST /issues/from-files`

```json
{
  "project_id": "proj-xxx",
  "session_id": "sess-xxx",
  "input_mode": "files",
  "title": "auth-refactor",
  "file_paths": ["docs/feature.md", "README.md"],
  "body": "可选补充说明",
  "workflow_profile": "normal",
  "auto_merge": false
}
```

校验规则：

- `input_mode` 必须为 `files`。
- `title` 必填，去除首尾空白后长度必须在 `1..120`。
- `file_paths` 至少 1 个，必须去重。
- `file_paths` 仅允许仓库内相对路径，禁止绝对路径和 `../` 越界路径。
- 文件模式创建后，issue 必须直接进入 `reviewing`，并初始化 `review_scope.files`。
- 校验失败返回 `400 INVALID_ISSUE_TITLE` 或 `400 INVALID_ISSUE_FILE_PATHS`。

### Issue 查询

- `GET /issues?project_id={projectID}`
- `GET /issues/{issueID}?project_id={projectID}`
- `GET /issues/{issueID}/reviews?project_id={projectID}`
- `GET /issues/{issueID}/changes?project_id={projectID}`
- `GET /issues/{issueID}/timeline?project_id={projectID}`

### Issue 动作

- `POST /issues/{issueID}/review`（body 带 `project_id`）
- `POST /issues/{issueID}/action`（body 带 `project_id`）
- `POST /issues/{issueID}/auto-merge`（body 带 `project_id`）

### Review Scope 显式变更（通过 issue action）

`POST /issues/{issueID}/action`

```json
{
  "project_id": "proj-xxx",
  "action": "replace_review_scope_files",
  "files": ["docs/feature.md", "internal/core/issue.go"],
  "reason": "补充实现涉及文件"
}
```

约束：

- 仅允许通过 action 改变 `review_scope.files`。
- 变更后必须写时间线事件 `review_scope_changed`。
- 非显式 action 产生的范围漂移必须视为服务端错误。

## WorkflowProfile

### 列出档位

`GET /workflow-profiles?project_id={projectID}`

### 查询单个档位

`GET /workflow-profiles/{profileID}?project_id={projectID}`

`profileID` 允许值：`normal | strict | fast_release`

## WorkflowRun

### 创建 run（通常由 Team Leader 调用）

`POST /runs`

```json
{
  "project_id": "proj-xxx",
  "issue_id": "issue-xxx",
  "workflow_profile": "normal",
  "session_id": "sess-xxx"
}
```

### 查询 run

- `GET /runs?project_id={projectID}`
- `GET /runs/{runID}?project_id={projectID}`

### Run 事件流

`GET /runs/{runID}/events?project_id={projectID}`

返回 `{ items: RunEvent[], total: int }`。每条 RunEvent 包含：

- `run_id` / `project_id` / `issue_id`
- `event_type`：与 EventBus 事件类型一致（如 `run_started`、`agent_output`、`auto_merged`）
- `stage`（可空）
- `agent`（可空）
- `data`：`map<string, string>` 自由键值
- `error`（可空）
- `created_at`

### 取消 run

`POST /runs/{runID}/cancel?project_id={projectID}`

## 时间线与观测

`GET /issues/{issueID}/timeline?project_id={projectID}&kinds=review,log,checkpoint,action`

时间线元素统一字段：

- `event_id`
- `kind`
- `created_at`
- `actor_type`
- `title`
- `body`
- `status`
- `refs`
- `meta`

推荐 `kind` 扩展值：

- `review_scope_initialized`
- `review_scope_changed`

## 断代约束

- 不再暴露 `/api/v1/projects/{projectID}/plans*` 与 `/tasks*`。
- 前端与脚本不得继续请求 `/api/v1/projects/{projectID}/chat/*`。
- 不提供旧字段别名或兼容路由。
- 对外文档统一 `issue / workflow_profile / workflow_run / Team Leader`。
