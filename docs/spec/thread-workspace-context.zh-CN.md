# Thread Workspace 与上下文引用模型规格

> 状态：草案
>
> 最后按代码核对：2026-03-14
>
> 对应实现：尚无；当前 `thread_session_pool.go:191` 启动 ACP 时 cwd 为空字符串
>
> 补充边界说明：Thread 主模型见 `thread-agent-runtime.zh-CN.md`；Thread-WorkItem 关联见 `thread-workitem-linking.zh-CN.md`；项目资源绑定见 `spec-unified-resource-model.zh-CN.md`

## 概述

Thread 是不绑定项目的全局讨论容器。当前 Thread agent 启动 ACP session 时没有 cwd（`acphandler.NewACPHandler("", "", nil)`），导致 agent 无法执行文件读写和终端命令，本质上只是"讨论型会话"。

本规格解决两个问题：

1. **Thread agent 缺少工作空间**——每个 Thread 获得独立的持久化工作目录，agent 在其中自由读写，保留跨会话的工作产物
2. **Thread 需要访问项目资源但不应绑定项目**——通过轻量的上下文引用（Context Ref）将项目资源只读挂载到 Thread，不改变 Thread 的全局视角定位

设计原则：

- Thread 主模型不添加 `project_id`
- 不引入独立的权限授权实体（ThreadGrant）——引用即授权，上下文引用自身携带访问级别
- 不引入独立的执行任务实体（ExecutionSession）——ACP session 配置即执行上下文
- 一次操作一个主 cwd，不做多目录自由漫游

## 架构模型

```
{dataDir}/
  threads/
    {threadID}/
      workspace/                ← agent 的 cwd（可读写）
        .context.json           ← 平台自动维护的上下文描述
        ...agent 工作产物...
      mounts/                   ← 只读项目视图（路径映射，非物理复制）
        {project-slug}/         → Project ResourceBinding 解析出的实际路径
      archive/
        2026-03-14/             ← 每日归档快照
```

```
Thread
  ├── ThreadMember (human / agent)
  ├── ThreadMessage
  │     └── metadata.resource_refs[]   ← 消息级资源引用
  ├── ThreadWorkItemLink               ← 已有，长期关联
  └── thread_context_refs              ← 新增，轻量上下文引用
        ├── ProjectA (read)
        └── ProjectB (check)

Agent ACP Session
  cwd = {dataDir}/threads/{threadID}/workspace/
  路径映射:
    mounts/{slug}/** → 项目实际路径（ACP handler 层拦截转译）
```

## 数据模型

### thread_context_refs

```sql
CREATE TABLE thread_context_refs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id   INTEGER NOT NULL,
    project_id  INTEGER NOT NULL,
    access      TEXT    NOT NULL DEFAULT 'read',  -- read | check | write
    note        TEXT    NOT NULL DEFAULT '',       -- 引用说明
    granted_by  TEXT    NOT NULL DEFAULT '',       -- 谁引用的（user_id）
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at  DATETIME,                          -- 可选过期时间
    UNIQUE(thread_id, project_id)
);
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `thread_id` | int64 | 关联的 Thread ID |
| `project_id` | int64 | 引用的 Project ID |
| `access` | string | 访问级别：`read`、`check`、`write` |
| `note` | string | 引用说明，如"审核方案文档" |
| `granted_by` | string | 创建引用的用户 ID（谁分享谁负责） |
| `expires_at` | datetime | 可选；为空表示随 Thread 生命周期 |

### Go 领域类型

```go
// ContextAccess 定义 Thread 对项目资源的访问级别。
type ContextAccess string

const (
    ContextAccessRead  ContextAccess = "read"   // 读文件、看 diff
    ContextAccessCheck ContextAccess = "check"  // read + 执行项目定义的检查命令
    ContextAccessWrite ContextAccess = "write"  // check + 在项目范围内写文件
)

// ThreadContextRef 表示 Thread 对一个项目上下文的轻量引用。
// 引用即授权：创建引用的行为本身就是对 Thread 参与者的临时授权。
type ThreadContextRef struct {
    ID        int64
    ThreadID  int64
    ProjectID int64
    Access    ContextAccess
    Note      string
    GrantedBy string
    CreatedAt time.Time
    ExpiresAt *time.Time
}
```

## Thread Workspace

### 创建时机

Thread 创建时同步创建 workspace 目录：

```
{dataDir}/threads/{threadID}/workspace/
```

### .context.json

平台自动生成并维护，agent 启动时读取即可了解当前可用上下文：

```json
{
  "thread_id": 456,
  "workspace": ".",
  "mounts": {
    "project-alpha": {
      "path": "../mounts/project-alpha",
      "project_id": 123,
      "access": "check",
      "check_commands": ["go test ./...", "npm test"]
    }
  },
  "archive": "../archive",
  "members": ["personA", "personC"],
  "updated_at": "2026-03-14T10:30:00Z"
}
```

`check_commands` 来自项目配置（见下文"检查命令白名单"），不在 Thread 层面单独配置。

### ACP Session 配置变更

当前 `thread_session_pool.go:191`：

```go
handler := acphandler.NewACPHandler("", "", nil)
```

变更为：

```go
workspaceDir := filepath.Join(dataDir, "threads", strconv.FormatInt(threadID, 10), "workspace")
handler := acphandler.NewACPHandler(workspaceDir, "", nil)
```

### 每日归档

每天凌晨（或 Thread 空闲时），将 workspace 增量快照到 archive：

```
{dataDir}/threads/{threadID}/archive/{YYYY-MM-DD}/
```

归档目录对 agent 只读。agent 可访问 `../archive/2026-03-13/` 查看历史版本。

归档策略：

- 默认保留 7 天（可通过项目配置调整）
- 空 workspace 不产生归档
- 归档使用增量复制，不是 git

## 上下文引用与项目挂载

### 引用即授权

不存在独立的 ThreadGrant 实体。`thread_context_refs` 的一行记录同时表达：

- **上下文**：这个 Thread 关联了哪个项目
- **授权**：Thread 参与者对该项目有什么级别的访问

创建引用 = 分享授权。`granted_by` 记录是谁做的，用于审计。

### 挂载机制

挂载不使用文件系统 symlink（Windows 兼容性差），而是 **ACP handler 层的路径映射**：

```
agent 请求的路径                    → 实际物理路径
workspace/plan.md                  → {dataDir}/threads/456/workspace/plan.md
mounts/project-alpha/src/main.go   → D:/projects/alpha/src/main.go
archive/2026-03-13/plan.md         → {dataDir}/threads/456/archive/2026-03-13/plan.md
```

ACP handler 持有挂载映射表（从 `thread_context_refs` + `ResourceBinding` 解析），拦截所有文件操作请求，做路径转译和权限检查。agent 不知道真实物理路径。

### Thread 焦点（Focus）

Thread.Metadata 中维护一个可变的 `focus` 字段，标识当前讨论焦点的项目：

```jsonc
// Thread.Metadata
{
  "focus": { "project_id": 123 }
}
```

- 对话中切换焦点：用户说"我们看一下 ProjectX"→ focus 更新
- focus 影响 UI 默认展示和 agent boot prompt 的上下文优先级
- focus 不影响权限——权限只看 `thread_context_refs`
- focus 为空时 Thread 处于纯讨论模式

## 访问级别与权限模型

### 三档访问级别

| 级别 | 文件读 | 文件写 | 终端命令 | 典型场景 |
|------|--------|--------|---------|---------|
| `read` | 项目文件只读 | 禁止 | 禁止 | 查看文件、diff、摘要 |
| `check` | 项目文件只读 | 禁止 | 仅项目白名单命令 | 审核 + 跑测试 |
| `write` | 项目文件只读 | 项目范围内允许 | 仅项目白名单命令 | 修改项目文件 |

注意：`write` 级别的文件写入受限于项目 ResourceBinding 范围，不是任意路径。

发布操作（push / merge / secrets）不在此模型中，走 SCM 插件自身的审批流程。

### 检查命令白名单

由项目配置定义，不在 Thread 层面单独配置：

```toml
# .ai-workflow/config.toml (项目级)
[review]
check_commands = [
    "go test ./...",
    "npm test",
    "scripts/check.sh",
]
```

当 `thread_context_ref.access = "check"` 或 `"write"` 时，agent 可执行该项目 `check_commands` 中列出的命令。不在白名单中的命令一律拒绝。

### 权限生效规则

最终有效权限 = `AgentProfile 上限` ∩ `ContextRef access`

```
Layer 1: AgentProfile                → capabilities_max + actions_allowed（已有）
Layer 2: ThreadContextRef.access     → read / check / write（本规格新增）
```

任何一层不允许，就不允许。ACP 的 `allow_once` / `reject` 作为运行时兜底交互，不算独立权限层。

### ACP Handler 路径权限判断

```go
func (h *Handler) checkThreadAccess(threadID int64, path string, op AccessOp) bool {
    // workspace/** → 自由读写
    if isUnderWorkspace(path) {
        return true
    }
    // archive/** → 只读
    if isUnderArchive(path) {
        return op == OpRead
    }
    // mounts/{project}/** → 查 context_ref
    if project, ok := resolveMount(path); ok {
        ref := getContextRef(threadID, project.ID)
        if ref == nil {
            return false
        }
        switch op {
        case OpRead:
            return true // 有 ref 就能读
        case OpWrite:
            return ref.Access == ContextAccessWrite
        case OpExec:
            return ref.Access >= ContextAccessCheck &&
                inCheckCommandWhitelist(cmd, project.ReviewConfig.CheckCommands)
        }
    }
    // 其他路径 → 拒绝
    return false
}
```

## 消息级资源引用

除了 `thread_context_refs` 表的项目级引用，消息也可携带轻量的资源引用：

```go
// ThreadMessage.Metadata 中的可选字段
type ResourceRef struct {
    ProjectID int64    `json:"project_id,omitempty"`
    Paths     []string `json:"paths,omitempty"`     // 具体文件路径
    Scripts   []string `json:"scripts,omitempty"`   // 建议执行的脚本
}
```

消息级引用是提示性的（给 agent 看的上下文），不创建 `thread_context_refs` 记录。真正的权限授权仍然只通过 `thread_context_refs` 表。

## API 端点

| Method | Path | 说明 |
|--------|------|------|
| `POST` | `/threads/{threadID}/context-refs` | 创建上下文引用（挂载项目） |
| `GET` | `/threads/{threadID}/context-refs` | 列出当前上下文引用 |
| `PATCH` | `/threads/{threadID}/context-refs/{refID}` | 更新访问级别 |
| `DELETE` | `/threads/{threadID}/context-refs/{refID}` | 移除上下文引用（卸载项目） |

### POST /threads/{threadID}/context-refs 请求体

```json
{
  "project_id": 123,
  "access": "check",
  "note": "审核方案文档，跑一下测试"
}
```

`granted_by` 从请求上下文中的用户身份自动填充。

### PATCH /threads/{threadID}/context-refs/{refID} 请求体

```json
{
  "access": "write"
}
```

仅允许升级或降级访问级别，不允许更改 project_id（需删除重建）。

## 协作流程示例

```
1. Thread#456 创建
   → 创建 {dataDir}/threads/456/workspace/
   → 生成 .context.json（空 mounts）

2. Agent 加入讨论，自由读写 workspace
   → cwd = threads/456/workspace/
   → agent 将讨论产出写入 workspace/plan.md

3. 用户: "挂载 ProjectAlpha，让 reviewerC 来审核，顺便跑测试"
   → POST /threads/456/context-refs { project_id: alpha, access: "check" }
   → 路径映射注册: mounts/project-alpha → alpha 的 ResourceBinding 路径
   → .context.json 更新

4. reviewerC 的 agent 启动
   → 读 .context.json → 知道有 project-alpha 可用，access=check
   → 读 workspace/plan.md → 看到之前 agent 的产出
   → 读 mounts/project-alpha/docs/arch.md → ACP 拦截 → 有 ref → 允许
   → 执行 go test ./... → ACP 拦截 → 在 check_commands 白名单 → 允许
   → 执行 rm -rf / → ACP 拦截 → 不在白名单 → 拒绝
   → 写审核意见到 workspace/review-notes.md → workspace 内 → 允许

5. 其他成员进入 Thread
   → 读 workspace/ → 看到 plan.md + review-notes.md

6. 凌晨归档
   → workspace/ 快照到 archive/2026-03-14/

7. 讨论结束
   → Thread 关闭或 ref 过期 → 挂载映射失效
   → workspace 保留（可查阅历史）
```

## 删除与清理策略

- Thread 删除时：先调用 `CleanupThread(threadID)` 释放 ACP session（已有），再清理 `thread_context_refs` 记录
- workspace 目录：Thread 删除后保留 N 天（可配置），之后由后台任务清理
- `thread_context_refs` 不设 `ON DELETE CASCADE`，通过 handler 显式清理（与 `thread_work_item_links` 策略一致）

## 与现有设计的关系

| 现有概念 | 本规格的关系 |
|---------|-------------|
| Thread 主模型 | 不修改；不添加 project_id |
| ThreadWorkItemLink | 保留；用于 Thread-WorkItem 长期关联 |
| thread_context_refs | 新增；用于项目上下文的轻量引用和权限 |
| Thread workspace | 新增；每个 Thread 的独立文件空间 |
| AgentProfile.capabilities_max | 保留；作为权限上限（Layer 1） |
| AgentProfile.actions_allowed | 保留；作为角色能力约束 |
| ResourceBinding | 保留；挂载时从中解析项目的实际 workspace 路径 |
| WorkItem 执行上下文 | 不影响；WorkItem 执行仍走 engine → workspace provider 路径 |
| ACP handler 权限检查 | 扩展；增加 workspace/mounts 路径判断和命令白名单 |
