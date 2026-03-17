# Thread Workspace 与上下文引用现状规格

> 状态：部分实现
>
> 最后按代码核对：2026-03-17
>
> 当前实现状态：`thread_context_refs`、Thread 目录布局、
> `.context.json` 同步、项目挂载 alias、附件收录、Thread agent 启动时
> 使用 Thread 目录作为工作区都已落地；但早期文档中的
> `workspace/` / `mounts/` / `archive/` 分层、`focus` 字段和每日归档并未按原稿全部实现。

## 一句话结论

Thread 现在已经有真实的文件工作区，不再只是逻辑上下文概念。

当前实现主线是：

- 每个 Thread 在 `dataDir/threads/{threadID}` 下拥有自己的目录
- 平台维护 `.context.json`
- 项目访问通过 `thread_context_refs` + `projects/{slug}` alias 暴露
- 用户上传附件通过 `attachments/` 暴露
- Thread agent 启动时直接以 Thread 目录作为 cwd / workspace 根

## 当前目录布局

当前真实目录布局来自 `internal/threadctx/workspace.go`。

已明确创建的目录/文件包括：

```text
{dataDir}/threads/{threadID}/
├── projects/       <- 项目 alias 目录
├── attachments/    <- Thread 附件目录
└── .context.json   <- 平台同步生成
```

补充说明：

- 当前代码里没有单独创建 `workspace/` 子目录
- 也没有实现文档草案中的 `archive/{date}` 日归档结构
- 早期草案里提到的 `mounts/` 最终落到了 `projects/` 目录命名

因此任何把现状写成 `workspace/ + mounts/ + archive/` 固定结构的文档
都需要降级为“历史方案”，不能再当现状。

## `.context.json` 当前作用

平台会基于当前 Thread 状态同步 `.context.json`。

当前已写入的信息包括：

- `thread_id`
- `workspace`
- `mounts`
- `members`
- `attachments`
- `updated_at`

其中：

- `workspace` 当前为 `"."`
- `mounts` 的 path 会指向 `projects/{slug}`
- `attachments` 会指向 `attachments/{filename}`

这意味着 agent 可以通过读取 `.context.json` 发现：

- 现在有哪些项目上下文被引用
- 每个项目对应哪个 alias
- 当前有哪些附件可用
- 当前有哪些成员参与 Thread

## 上下文引用模型

当前表为：

- `thread_context_refs`

核心字段包括：

- `thread_id`
- `project_id`
- `access`
- `note`
- `granted_by`
- `created_at`
- `expires_at`

当前核心语义仍成立：

- 引用即授权
- Thread 本身不直接绑定单一 `project_id`
- 权限粒度由 `ContextAccess` 控制

当前访问级别仍为：

- `read`
- `check`
- `write`

## 挂载解析现状

当前项目挂载并不是“复制项目文件”。

实际逻辑是：

1. 从 `thread_context_refs` 找到 `project_id`
2. 读取该 Project 的 `ResourceSpace`
3. 优先解析可落地的 `local_fs` 或 `git` 工作目录
4. 为该 project 生成一个 slug
5. 在 `threads/{threadID}/projects/{slug}` 下创建 alias 目录
6. 在 `.context.json` 中登记 `projects/{slug}` 路径

当前重点：

- 当前挂载别名目录名是 `projects/{slug}`
- Windows 下会优先尝试创建 junction
- 如果 junction 不可用，会退化为普通目录占位

## 与 ResourceSpace 的关系

当前 Thread workspace 不再依赖旧文档里的 `ResourceBinding` 主叙事，
而是已经优先从 `ResourceSpace` 解析项目根路径。

当前实际行为：

- `local_fs` 类型：直接使用 `RootURI`
- `git` 类型：优先解析非远端的 `RootURI`，或读取 `Config.clone_dir`
- 可从 `ResourceSpace.Config` 中读取 `check_commands`

这意味着 Thread workspace 与统一资源模型已经接上了第一条真实链路。

## 附件现状

当前 Thread 附件是现行能力，不再是规划项。

系统会：

- 将附件放到 `attachments/`
- 把附件信息同步进 `.context.json`
- 让 Thread agent 可通过上下文看到这些附件

因此当前 Thread workspace 实际上已经同时承载：

- 项目引用
- 用户上传附件
- agent 运行目录

## 权限语义现状

当前文档仍可保留三档访问语义：

- `read`
- `check`
- `write`

但要明确边界：

- access 字段和上下文信息已经落地
- 更完整的“所有文件操作都由本文档中的统一权限判定函数拦截”
  这一层并没有按草案全文一比一实现

换句话说：

- “访问级别模型存在”是现状
- “所有权限语义都已严格闭环”不是现状

## 当前 agent 工作区事实

Thread agent runtime 启动时，已经会为 agent 准备 Thread workspace 作用域。

当前已落地行为包括：

- 创建 Thread 目录布局
- 同步 `.context.json`
- 将 Thread 工作目录注入到 runtime
- 按项目 alias 与附件目录暴露上下文

因此现在应当把 Thread workspace 理解为：

“围绕 Thread 根目录组织的工作区”，而不是
“必须有独立 `workspace/` 子目录的设计草图”。

## 当前未落地或不应按现状表述的内容

以下内容应明确视为草案遗留，不应写成现状：

- `Thread.Metadata.focus`
- 固定 `workspace/` 子目录
- 固定 `mounts/` 子目录
- 每日归档 `archive/{date}`
- “所有命令白名单与路径权限都已完全闭环”

这些内容可以继续作为未来增强方向，但不能再冒充当前代码行为。

## API 现状

当前已实现端点：

| Method | Path | 说明 |
|------|------|------|
| `POST` | `/threads/{threadID}/context-refs` | 创建项目上下文引用 |
| `GET` | `/threads/{threadID}/context-refs` | 列出引用 |
| `PATCH` | `/threads/{threadID}/context-refs/{refID}` | 更新 access / note |
| `DELETE` | `/threads/{threadID}/context-refs/{refID}` | 删除引用 |

与之直接相关的现行能力还包括：

- `/threads/{threadID}/attachments`
- `/threads/{threadID}/files`

说明 Thread workspace 当前已经不仅仅是“引用表”，还包含文件入口。

## 推荐搭配阅读

1. `thread-agent-runtime.zh-CN.md`
2. `thread-task-dag.zh-CN.md`
3. `spec-unified-resource-model.zh-CN.md`
4. `thread-workitem-linking.zh-CN.md`
