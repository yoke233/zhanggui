# 统一资源模型现状规格

> 状态：部分实现
>
> 最后按代码核对：2026-03-17
>
> 对应实现：
> - `internal/core/unified_resource.go`
> - `internal/adapters/store/sqlite/unified_resource_models.go`
> - `internal/adapters/store/sqlite/unified_resource_migration.go`
> - `internal/adapters/http/action_resource.go`
> - `internal/adapters/http/handler.go`

## 一句话结论

统一资源模型已经不再是纯设计稿。

当前已经落地的主线是：

- `ResourceSpace`：项目级路径空间
- `Resource`：具体文件对象
- `ActionIODecl`：Action 输入输出声明
- SQLite 表与迁移逻辑
- HTTP API 与前端类型

但它仍是“部分实现”，因为旧模型仍在仓库中共存，且并非所有旧接口都已彻底删除。

## 当前为什么需要这套模型

旧实现把不同语义塞在一起：

- 项目资源空间
- 工作项附件
- Run 产出
- Action I/O 绑定

当前统一资源模型的目标，就是把这些概念拆清：

- 空间是空间
- 文件是文件
- I/O 声明是声明
- 执行事实是执行事实

这套拆分在代码里已经开始真实承担职责。

## 当前主模型

### ResourceSpace

`ResourceSpace` 当前表示项目级外部路径空间。

核心字段包括：

- `id`
- `project_id`
- `kind`
- `root_uri`
- `role`
- `label`
- `config`
- `created_at`
- `updated_at`

当前 `kind` 常量已包含：

- `git`
- `local_fs`
- `s3`
- `http`
- `webdav`

当前它已被用于：

- 项目级空间配置
- Thread workspace 项目根路径解析
- Action I/O 输入输出解析

### Resource

`Resource` 当前表示具体文件对象。

核心字段包括：

- `id`
- `project_id`
- `work_item_id`
- `run_id`
- `message_id`
- `storage_kind`
- `uri`
- `role`
- `file_name`
- `mime_type`
- `size_bytes`
- `checksum`
- `metadata`
- `created_at`

当前它已经能表达：

- WorkItem 文件
- Run 产出
- Message 附件

这意味着旧文档中“消息附件还不存在”的结论已经过时。

### ActionIODecl

`ActionIODecl` 当前表示 Action 的输入输出声明。

核心字段包括：

- `id`
- `action_id`
- `direction`
- `space_id`
- `resource_id`
- `path`
- `media_type`
- `description`
- `required`
- `created_at`

当前它已经支持两种引用方式：

- `space_id + path`
- `resource_id`

## 当前已落地能力

### Store 与数据库表

当前 SQLite 已经存在：

- `resource_spaces`
- `resources`
- `action_io_decls`

并且已经有：

- 模型定义
- CRUD store
- schema 注册
- 测试

### 迁移逻辑

当前迁移逻辑已经存在，说明统一资源模型不是“先写 spec 再说”，
而是已开始承接旧事实源。

当前迁移覆盖：

- `resource_bindings` -> `resource_spaces`
- `resource_bindings(kind=attachment)` -> `resources`
- `action_resources` -> `action_io_decls`
- `executions.result_assets` -> `resources(run_id)`

因此“统一资源模型已进入现状实现”现在是准确表述。

### HTTP API

当前已实现端点包括：

| Method | Path | 说明 |
|------|------|------|
| `POST` | `/projects/{projectID}/spaces` | 创建空间 |
| `GET` | `/projects/{projectID}/spaces` | 列出空间 |
| `GET` | `/spaces/{spaceID}` | 获取空间 |
| `PUT` | `/spaces/{spaceID}` | 更新空间 |
| `DELETE` | `/spaces/{spaceID}` | 删除空间 |
| `POST` | `/actions/{actionID}/io-decls` | 创建 I/O 声明 |
| `GET` | `/actions/{actionID}/io-decls` | 列出 I/O 声明 |
| `DELETE` | `/io-decls/{declID}` | 删除 I/O 声明 |
| `POST` | `/work-items/{id}/resources` | 上传 WorkItem 文件 |
| `GET` | `/work-items/{id}/resources` | 列出 WorkItem 文件 |
| `POST` | `/messages/{id}/resources` | 上传消息附件 |
| `GET` | `/messages/{id}/resources` | 列出消息附件 |
| `GET` | `/runs/{id}/resources` | 列出 Run 产出 |
| `GET` | `/resources/{id}` | 获取资源元数据 |
| `GET` | `/resources/{id}/download` | 下载资源 |
| `DELETE` | `/resources/{id}` | 删除资源 |

### 前端契约

前端当前已经存在：

- `ResourceSpace` 类型
- `Resource` 类型
- `ActionIODecl` 类型
- 对应 API client 方法

说明统一资源模型已经不是后端自嗨设计，而是前后端共同消费的现状能力。

## 当前运行时关系

当前可以把统一资源模型理解成：

```text
Project
  -> ResourceSpace[]

WorkItem
  -> Resource[]

Run
  -> Resource[]

ThreadMessage
  -> Resource[]

Action
  -> ActionIODecl[]
    -> ResourceSpace or Resource
```

这条主线已经真实存在于代码中。

## 与旧模型的关系

当前旧模型并没有完全消失。

仓库仍然保留：

- `ResourceBinding`
- `ActionResource`
- `executions.result_assets` 的历史兼容迁移来源

因此当前正确说法应该是：

- 统一资源模型已经落地
- 但旧模型仍处于兼容和迁移尾声

而不是：

- “旧模型已经彻底删除”

## 当前边界

以下内容已经成立：

- 三个核心模型已建立
- 三张主表已建立
- CRUD 与迁移已存在
- 统一资源端点已存在
- Thread workspace 已开始依赖 `ResourceSpace`
- Action I/O resolver 已开始依赖 `ActionIODecl`

以下内容不应写成“已经完全收口”：

- 旧名和旧类型还没有全量清理
- `ResourceBinding` 仍存在于部分兼容代码与说明中
- 并不是所有历史接口都已在仓库中彻底移除

## 当前推荐表述

描述现状时，应优先使用：

- `ResourceSpace`
- `Resource`
- `ActionIODecl`

只有在说明迁移历史时，才提及：

- `ResourceBinding`
- `ActionResource`
- `result_assets`

## 推荐搭配阅读

1. `backend-current-architecture.zh-CN.md`
2. `thread-workspace-context.zh-CN.md`
3. `execution-context-building.zh-CN.md`
4. `naming-transition-thread-workitem.zh-CN.md`
