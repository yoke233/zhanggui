# 命名迁移规范：Thread / WorkItem / Action

> 本文档定义系统对外术语升级的映射矩阵、兼容策略与淘汰周期。
>
> 状态：现行
>
> 最后按代码核对：2026-03-29
>
> 当前实现状态：本文中的命名治理规则已基本生效。前端主入口与后端 Public REST 已经切到 `workitem/action/run` 主命名；旧 `/issues/*`、`/flows/*` 已退出当前工作台；剩余兼容层主要存在于持久化表名（`issues` / `steps` / `executions`）和少量历史 helper / request struct。
>
> 重要说明：本文现在更适合作为“现行收口规则 + 剩余兼容层说明”阅读，而不是“未来迁移计划”。

## 决策摘要

本文建议并约束后续演进方向如下：

1. 对外产品语义统一使用 `Work Item` / `Action` / `Run`。
2. 对外 Public REST API 统一为 `/api/work-items/*` 与 `/api/actions/*`。
3. 内部公共领域实现统一以 `WorkItem` / `Action` / `Run` 表达；持久化层短期保留 `issues` / `steps` / `executions` 历史命名。
4. `Flow` 降级为历史兼容/技术执行术语，不再作为主业务对象名称继续扩散。
5. `ChatSession` 与 `Thread` 明确分离，不做合并命名。

换句话说：

- 用户看到的是 `Work Item` / `Action` / `Run`
- 新 API 目标是 `work-items` / `actions`
- 内部领域主名已经是 `WorkItem` / `Action` / `Run`
- `Flow` 只能留在兼容层，不能再进入新设计、新接口、新文档主叙述

## 当前现状与目标状态

### 当前现状（按代码）

- 前端主页面路由：`/work-items`
- 旧 `/issues/*`、`/flows/*` 已退出当前前端工作台
- 后端主 REST 路由：`/work-items/*`、`/actions/*`
- 内部核心领域对象：`WorkItem` / `Action` / `Run`
- Thread 已独立建模并拥有自己的 REST / WebSocket 协议

### 目标状态（本规范要求）

- 产品/UI：统一称 `Work Item` / `Action` / `Run`
- Public REST：统一以 `/api/work-items/*`、`/api/actions/*` 为主
- 内部领域实现：继续以 `WorkItem` / `Action` / `Run` 作为公共主名；仅持久化/历史 helper 保留旧命名
- `Flow`：只允许作为历史兼容名或纯技术执行流程语义存在

## 分层命名规则

| 层级 | 统一名称 | 当前实现 | 规则 |
|------|----------|----------|------|
| 产品/UI | `Work Item` | 已基本落地 | 新页面、新文案、新交互统一使用 `Work Item` |
| Public REST API | `work-items` | 已落地 | 新增主接口继续沿用 `/api/work-items/*` |
| 内部领域模型 | `WorkItem` | 已落地 | 持久化表名与部分兼容代码仍保留旧命名 |
| 执行流程语义 | `workflow` / `execution pipeline` | 部分混用 `flow` | 用于描述“步骤推进过程”，不是业务对象名 |
| 历史兼容词 | `Flow` | 仍大量存在 | 不得在新功能和新 spec 中继续作为主对象名扩散 |

## 命名映射矩阵

| 内部 Go struct / 表名 | API 外部名 | UI 显示名 | 说明 |
|----------------------|-----------|----------|------|
| `Issue` | `WorkItem` | Work Item | 对外统一用 WorkItem；`Issue` 只剩持久化/历史残留 |
| `Step` | `Action` | Action | 对外统一用 Action；`Step` 只剩持久化/历史残留 |
| `Execution` | `Run` | Run | 对外统一用 Run；`Execution` 只剩持久化/历史残留 |
| `Artifact` | `Artifact` | Deliverable / Artifact | UI 文案可逐步转 Deliverable；API/模型短期不强制改名 |
| `ChatSession` | `ChatSession` | Chat | **不映射为 Thread**；保持 1:1 direct chat 概念 |
| `Thread`（新增） | `Thread` | Thread | 独立领域实体，多 AI + 多 human 共享讨论 |

## ChatSession 保持 direct chat 概念，不映射为 Thread

`ChatSession` 与 `Thread` 是两个并列的交互概念：

- `ChatSession`：1 AI + 1 human 的 direct chat，保留现有 `/chat` API 与 `chat.send` WebSocket 协议
- `Thread`：多 AI + 多 human 的共享讨论容器，新增 `/threads` API 与 `thread.send` WebSocket 协议

两者不共享主键、时间线或 runtime session。

## HTTP 路由兼容策略

当前现状分两层：

- 对外页面主入口：`/work-items`
- 对外 REST 主入口：`/api/work-items/*`
- 兼容层：主要剩在内部 `issue` / `step` 命名与持久化表名残留

目标分层：

- 对外主契约：`/api/work-items/*`
- 兼容契约：不再新增 `/api/issues/*`
- 历史页面路由：`/issues/*`、`/flows/*` 已退出当前工作台，不得再恢复为现行入口

### 主规则

1. 新增 API 能力应优先设计为 `/api/work-items/*`
2. 不再新增 `/api/issues/*` 兼容路由
3. 禁止新增 `/api/flows/*` 路由
4. 任何新文档都不得把 `/flows` 写成现行工作对象入口

### 新增路由

| 路由 | 说明 |
|------|------|
| `GET /threads` | Thread 列表 |
| `POST /threads` | 创建 Thread |
| `GET /threads/{id}` | Thread 详情 |
| `PUT /threads/{id}` | 更新 Thread |
| `DELETE /threads/{id}` | 删除 Thread |

说明：截至 2026-03-14，Thread 路由本身已经稳定，不在本次命名收口中变更。

### 现行主路由

| 路由 | 说明 |
|------|------|
| `GET /work-items` | Work Item 列表 |
| `POST /work-items` | 创建 Work Item |
| `GET /work-items/{id}` | Work Item 详情 |
| `PUT /work-items/{id}` | 更新 Work Item |
| `DELETE /work-items/{id}` | 删除 Work Item |

### 仍保留的现行兼容入口

| 路由 | 说明 |
|------|------|
| `GET /chat/sessions` | ChatSession 列表（保留） |
| `POST /chat` | 已废弃；当前返回 `410 Gone`，发送消息应改用 `chat.send` WebSocket |

### 当前未落地或已退出的历史入口

| 入口 | 当前状态 |
|------|----------|
| `POST /chat/sessions/{id}/crystallize-thread` | 当前代码中未落地，不应写成现行能力 |
| `/issues/*` | 已退出当前前端工作台 |
| `/flows/*` | 已退出当前前端工作台 |

### 兼容周期

- **Phase 1（已完成）**：前端以 `/work-items` 为主
- **Phase 2（已完成）**：后端 Public REST 切到 `/work-items`
- **Phase 3（当前）**：继续清理内部 `issue` / `flow` 兼容命名，但不强行改动表名
- **Phase 4（未来）**：视需要决定是否继续缩减 issue-named alias 方法与类型别名

## WebSocket 协议兼容策略

### 新增消息类型

| 消息类型 | 说明 |
|---------|------|
| `thread.send` | 向 Thread 发送消息（payload 包含 `thread_id`） |
| `subscribe_thread` | 订阅 Thread 事件流 |
| `unsubscribe_thread` | 取消订阅 Thread 事件流 |

### 保留消息类型

| 消息类型 | 说明 |
|---------|------|
| `chat.send` | 向 ChatSession 发送消息（保留 `session_id` 语义） |
| `subscribe_chat_session` | 订阅 ChatSession（保留） |

### 关键区别

- `thread.send` 的 payload 使用 `thread_id`，不使用 `session_id`
- `chat.send` 的 payload 继续使用 `session_id`
- 两者不互为 alias，各走独立的处理链路

## JSON Payload 与对象字段策略

### 主键字段

- 主对象主键统一继续使用 `id`
- 不建议引入 `issue_id` / `work_item_id` 双主键并存
- Thread 主对象继续使用 `id`

### 关联字段

当前策略：

- `/work-items` 当前已经落地，并尽量复用现有 JSON 结构
- `/threads` 主对象继续返回通用主键字段 `id`
- Thread 子资源（如 message、participant、agent session、work item link）按现有模型返回 `id` 与 `thread_id`

目标策略：

- 当 `/work-items` REST alias 落地后，路径语义切到 `work-items`
- 但响应体不强制把所有 `issue_*` 字段立即改成 `work_item_*`
- 是否改字段名，应另开一次专门迁移；不要把“路由命名收口”和“payload 字段全面重命名”绑定在同一波进行

### 错误码策略

- Thread 相关错误使用 `THREAD_*` 前缀（`THREAD_NOT_FOUND`, `CREATE_THREAD_FAILED`）
- Issue/WorkItem 相关错误继续使用 `ISSUE_*` 前缀
- ChatSession 相关错误继续使用 `CHAT_*` / `SESSION_*` 前缀

补充决策：

- 短期不要求把 `ISSUE_*` 全量改成 `WORK_ITEM_*`
- 新增错误码若属于 `/work-items` Public API，可优先采用中性命名，例如 `WORK_ITEM_NOT_FOUND`
- 旧错误码兼容保留

## Flow 兼容层边界

### 禁止继续新增

- 新页面名、组件名、路由名使用 `Flow`
- 新 Public API 使用 `/flows`
- 新 spec 把 `Flow` 写成当前主业务对象
- 新领域字段把 `flow_id` 作为主工作对象引用

### 应逐步替换

- `FlowScheduler`
- `PRFlowPrompts`
- `flow_pr_bootstrap.go`
- `CreateFlowPage` / `FlowsPage` / `FlowDetailPage`
- i18n 中仍引用 `/flows/:id/...` 的文案

### 可暂时保留

- `internal/application/flow` 这类包名
- 历史测试文件名中的 `flow_*`
- 少量内部错误名和 helper 名称

原则：

- 可以暂时保留旧包名
- 但不能再让旧包名继续污染新的 public surface

## 内部 Go struct 重命名策略

当前阶段（Wave 1-3）**不强制**重命名内部 Go struct 和数据库表名：

- `Issue` struct 和 `issues` 表保持不变
- `Step` struct 和 `steps` 表保持不变
- `Execution` struct 和 `executions` 表保持不变
- `Artifact` struct 和 `artifacts` 表保持不变

新增的 `Thread` 直接以新名称建模，不存在旧名遗留。

补充决策：

- `Issue` / `Step` / `Execution` 只在持久化层与历史 helper 中继续保留
- 不建议为了对齐术语而立刻重命名数据库表、store 接口和核心执行引擎
- 内部重命名应在 `/api/work-items` 主契约稳定后再评估

## 前端类型 alias 策略

当前策略已经收口为：

- 前端路由已切到 `/work-items`，对应实现见 `web/src/App.tsx`
- 前端主 API client 已使用 `/work-items`、`/actions`
- `Issue = WorkItem`、`Step = Action` 一类兼容 alias 已不再建议保留

补充说明：

- 旧 `issue` 语义主要残留在持久化命名和少量 request / handler 字段中
- 不建议继续引入 `Action = Step`、`Run = Execution`、`Deliverable = Artifact` 这类第二层 alias，除非确有明确产品收益

## spec 文档状态规范

今后所有相关 spec 顶部都应标记以下字段：

- `状态：现行 / 部分实现 / 草案 / 历史`
- `最后按代码核对：YYYY-MM-DD`

状态解释：

- `现行`：代码和接口基本已落地，可当真实契约阅读
- `部分实现`：部分已落地、部分仍为目标设计，必须显式区分
- `草案`：目标架构/未来方向，不代表当前行为
- `历史`：迁移记录或废弃方案，不作为现行依据

硬规则：

- 未来设计不能直接写成现在时
- 当前实现说明必须能落到代码或测试
- 迁移文档不能再冒充当前架构文档

## 迁移阶段建议

### Phase A：先定规则，不大改代码

- 统一对外术语为 `Work Item` / `Action` / `Run`
- 新文档和新页面禁止继续扩散 `Flow`
- spec 全部补状态头

### Phase B：Public REST 已切主

- 后端对外主路由已经是 `/api/work-items/*`、`/api/actions/*`
- 前端 API client 已经以 `/work-items`、`/actions` 为默认路径
- 现阶段重点不再是补 alias，而是说明兼容残留

### Phase C：前端与文案收口

- 页面/组件/i18n 去 `Flow`
- 不再把 `/issues/*`、`/flows/*` 当作现行工作台入口
- 变量名和导航统一到 `workItem`

### Phase D：内部命名渐进清理

- 优先替换最误导的 exported symbol
- 例如 `FlowScheduler`、`PRFlowPrompts`
- 包名是否要改，最后再评估
