# 一句话指令 → Issue DAG 设计

日期: 2026-03-09
状态: approved

## 1. 目标

用户在 ChatView 或 BoardView 输入一句话需求，Team Leader 自动分析并产出 Issue DAG 方案，前端渲染可视化 DAG 预览供用户编辑和确认，确认后批量创建 Issue 并按严格依赖调度执行。

## 2. 术语统一

- **Issue** = 系统唯一的工作单元概念（等同 v3 的 Task，不引入新模型）
- **DAG** = Issue 之间通过 `DependsOn`/`Blocks` 形成的有向无环图
- **Proposal** = Team Leader 产出的拆解方案（未创建 Issue 前的草稿）

## 3. 两个入口

### 入口 A：ChatView 对话

- 用户在 Chat 里发消息（如"帮我做一个用户注册系统"）
- Team Leader 作为 Chat Assistant 回复，产出结构化 DAG 方案
- 前端检测到 DAG 方案消息后，内嵌渲染 DAG 预览组件
- 用户在 Chat 内确认/编辑

### 入口 B：BoardView 快速输入框

- BoardView 顶部加一个输入框，placeholder: "描述你的需求..."
- 输入提交后，调用 Team Leader 拆解 API
- 弹出 DAG 预览面板，用户编辑后确认创建

两个入口最终都走同一个后端 API。

## 4. 拆解流程

```
用户输入一句话
  ↓
POST /api/v2/projects/{id}/decompose
  body: { "prompt": "帮我做一个用户注册系统" }
  ↓
Team Leader LLM 调用
  - System prompt: 分析需求，拆成独立可执行任务，指明依赖
  - 输出: 结构化 JSON（Issue 列表 + 依赖边）
  ↓
返回 DAG 方案（不创建 Issue）
  {
    "proposal_id": "prop-xxx",
    "issues": [
      { "temp_id": "A", "title": "设计数据库 schema", "body": "...", "depends_on": [] },
      { "temp_id": "B", "title": "实现注册 API", "body": "...", "depends_on": ["A"] },
      ...
    ]
  }
  ↓
前端渲染 DAG 预览（可视化图 + Issue 列表）
  - 用户可编辑：修改标题/描述、调整依赖、删除/新增节点
  ↓
用户点"确认创建"
  ↓
POST /api/v2/projects/{id}/decompose/confirm
  body: { "proposal_id": "prop-xxx", "issues": [...edited] }
  ↓
批量创建 Issue
  - 为每个节点创建 Issue，temp_id → 真实 issue_id
  - 设置 DependsOn/Blocks 关系
  - 全部置为 queued 状态
  - 写 TaskStep: created + queued
  ↓
Scheduler 按严格依赖调度
  - 无前置依赖的 Issue → ready → executing
  - 有前置依赖的 Issue → 等待前置全部 done → ready
```

## 5. API 设计

### 5.1 拆解（产出方案，不创建 Issue）

```
POST /api/v2/projects/{projectId}/decompose
Request:
{
  "prompt": "帮我做一个用户注册系统"
}
Response:
{
  "proposal_id": "prop-20260309-xxxx",
  "summary": "用户注册系统，包含数据库设计、后端API、前端页面和集成测试",
  "issues": [
    {
      "temp_id": "A",
      "title": "设计数据库 schema",
      "body": "设计用户表结构...\n\n验收标准：\n- users 表包含 id, email, password_hash...",
      "depends_on": [],
      "labels": ["backend", "database"]
    },
    {
      "temp_id": "B",
      "title": "实现注册 API",
      "body": "实现 POST /api/register...",
      "depends_on": ["A"],
      "labels": ["backend"]
    },
    {
      "temp_id": "C",
      "title": "实现登录 API",
      "body": "实现 POST /api/login...",
      "depends_on": ["A"],
      "labels": ["backend"]
    },
    {
      "temp_id": "D",
      "title": "前端注册页面",
      "body": "React 注册表单组件...",
      "depends_on": ["B"],
      "labels": ["frontend"]
    },
    {
      "temp_id": "E",
      "title": "集成测试",
      "body": "端到端测试注册流程...",
      "depends_on": ["B", "C", "D"],
      "labels": ["test"]
    }
  ]
}
```

### 5.2 确认创建

```
POST /api/v2/projects/{projectId}/decompose/confirm
Request:
{
  "proposal_id": "prop-20260309-xxxx",
  "issues": [
    {
      "temp_id": "A",
      "title": "设计数据库 schema（可被用户编辑过）",
      "body": "...",
      "depends_on": [],
      "labels": ["backend"],
      "template": "standard",
      "auto_merge": true
    }
  ]
}
Response:
{
  "created_issues": [
    { "temp_id": "A", "issue_id": "issue-20260309-abcd" },
    { "temp_id": "B", "issue_id": "issue-20260309-bcde" },
    ...
  ]
}
```

## 6. Team Leader 拆解 Prompt

```
你是技术项目主管。用户给了一个需求，请分解成多个独立可执行的任务。

规则：
1. 每个任务应该是一个独立的代码变更，可以独立开发和测试
2. 明确任务之间的依赖关系（哪个任务必须先完成）
3. 尽量让无依赖的任务可以并行执行
4. 每个任务给出清晰的标题和描述，描述中包含验收标准
5. 输出纯 JSON，不要其他文字

输出格式：
{
  "summary": "方案概述（一句话）",
  "issues": [
    {
      "temp_id": "A",
      "title": "任务标题",
      "body": "任务描述，包含验收标准",
      "depends_on": [],
      "labels": ["backend"/"frontend"/"test"/...]
    }
  ]
}
```

## 7. DAG 可视化组件

```
┌──────────────────────────────────────────────────┐
│  确认创建需求方案                        [创建] [取消] │
├──────────────────────────────────────────────────┤
│                                                  │
│   ┌─────────────┐                                │
│   │ A. DB Schema │                               │
│   └──────┬──────┘                                │
│          │                                       │
│    ┌─────┴─────┐                                 │
│    ▼           ▼                                 │
│ ┌──────┐  ┌──────┐                               │
│ │B. 注册│  │C. 登录│                               │
│ │  API │  │  API │                               │
│ └──┬───┘  └──┬───┘                               │
│    │         │                                   │
│    ▼         │                                   │
│ ┌──────┐    │                                    │
│ │D. 前端│    │                                    │
│ │ 注册页│    │                                    │
│ └──┬───┘    │                                    │
│    │        │                                    │
│    └───┬────┘                                    │
│        ▼                                         │
│   ┌─────────┐                                    │
│   │E. 集成  │                                    │
│   │  测试   │                                    │
│   └─────────┘                                    │
│                                                  │
├──────────────────────────────────────────────────┤
│  Issue 列表（可点击编辑标题、描述、依赖）             │
│  ☐ A. 设计数据库 schema        依赖: 无            │
│  ☐ B. 实现注册 API             依赖: A             │
│  ☐ C. 实现登录 API             依赖: A             │
│  ☐ D. 前端注册页面             依赖: B             │
│  ☐ E. 集成测试                 依赖: B, C, D       │
└──────────────────────────────────────────────────┘
```

交互行为：
- DAG 图节点可点击选中，高亮该节点及其依赖链
- Issue 列表每项可展开编辑标题、描述、依赖
- 可拖拽调整依赖关系（或下拉选择）
- 可删除节点（自动移除相关依赖边）
- 可新增节点
- "创建"按钮提交前做 DAG 校验（检测环、孤立节点等）

## 8. 调度策略：严格依赖

- 前置 Issue 必须全部 `done` 后，后续 Issue 才从 `queued → ready`
- 任一前置 `failed` → 根据 `FailPolicy` 决定：
  - `block`（默认）：后续 Issue 也标记 `failed`
  - `skip`：跳过失败的前置，后续继续
  - `human`：等待人工决策
- 现有 `scheduler_dispatch.go` 的 `markReadyByProfileQueueLocked()` 已有依赖检查逻辑，需加强为严格 done 检查

## 9. ChatView 集成

Team Leader 作为 Chat Assistant 回复时，消息内容包含结构化 DAG 数据：

```json
{
  "type": "dag_proposal",
  "proposal_id": "prop-xxx",
  "summary": "...",
  "issues": [...]
}
```

前端 ChatView 检测到消息 data 中有 `type: "dag_proposal"` 时，渲染 DagPreview 组件而非普通文本。用户在 Chat 内点"确认创建"后调用 confirm API。

## 10. 改造范围

### 新增

- `internal/teamleader/decompose_planner.go` — DAG 拆解逻辑（调用 LLM，输出 Proposal）
- `internal/web/handlers_decompose.go` — decompose / confirm API handler
- `web/src/components/DagPreview.tsx` — DAG 可视化预览 + 编辑组件
- `web/src/components/QuickInput.tsx` — BoardView 顶部输入框组件

### 改造

- `web/src/views/BoardView.tsx` — 集成 QuickInput + DagPreview 弹窗
- `web/src/views/ChatView.tsx` — 检测 dag_proposal 消息并渲染 DagPreview
- `internal/teamleader/scheduler_dispatch.go` — 加强依赖检查（严格 done 校验）
- `internal/teamleader/manager.go` — 批量创建 Issue + 设置依赖关系

### 不变

- Issue 模型（DependsOn/Blocks 字段已有）
- Run 执行引擎
- Pipeline 阶段逻辑
- TaskStep 事件溯源（直接复用，每个 Issue 创建时写 TaskStep）

## 11. 未来演进

- **Agent 自动确认**：Gate type 从 human 改为 auto/agent，Supervisor Agent 审批 DAG 方案
- **动态 DAG 调整**：执行中 Agent 发现新依赖时，可以向 Team Leader 请求调整 DAG
- **递归分解**：DAG 中的某个 Issue 本身也可以再次被分解为子 DAG（利用 Issue.ParentID）
