# 统一资源模型设计

> 状态：草案
>
> 日期：2026-03-14
>
> 前序：同名 v2 讨论稿（当前仓库未保留原文）
>
> 目标：聚焦文件资源整理，不含检索与全文索引

## 1. 问题

当前系统有三条并行的文件管理路径，彼此不互通：

| 路径 | 实体 | 存储位置 | Provider 参与 |
|---|---|---|---|
| 项目资源 | `ResourceBinding` | `resource_bindings` 表 | Registry 分发 |
| 工作项附件 | `ResourceBinding(kind=attachment)` | 同上，但 HTTP handler 直接操盘磁盘 | 不走 Provider |
| 执行产出 | `Run.ResultAssets []Asset` | `executions` 表内联 JSON | 无 |
| 消息附件 | 不存在 | — | — |

核心问题：**`ResourceBinding` 这个名字和实体同时承担了三件不同的事**：

1. 指向一个**路径空间**（git 仓库、S3 bucket、本地目录）
2. 记录一个**具体文件**（上传的附件、执行产出）
3. 充当 Action 与存储后端之间的**绑定关系**

路径空间和具体文件是本质不同的东西——Git 仓库不是一个文件，它是一个包含无数文件的空间。上传的 PNG 截图才是一个文件。把它们塞进同一个 `ResourceBinding` 是当前混乱的根源。

## 2. 命名决策

### 不再叫 ResourceBinding

| 旧名 | 问题 | 新名 | 语义 |
|---|---|---|---|
| `ResourceBinding` | 身兼三职 | 拆分为两个实体 | — |
| — | — | **`ResourceSpace`** | 外部路径空间（git repo、S3 bucket、本地目录） |
| — | — | **`Resource`** | 一个具体的文件/对象 |
| `Asset` | 脱离体系的内联结构 | 废弃，并入 `Resource` | — |
| `ActionResource` | 混淆声明与事实 | **`ActionIODecl`** | Action 的 I/O 声明（执行前定义） |
| `attachmentResponse` | 平行于主模型的 DTO | 废弃，用 `Resource` 投影 | — |

### 命名原则

- **`ResourceSpace`** = 一个可以按路径读写的空间。它不是文件，是文件的容器。
- **`Resource`** = 一个具体的文件对象。有大小、有类型、可以下载。简短、直接、无歧义。
- **`ActionIODecl`** = Action 的输入输出声明。"Decl" 明确它是声明而非事实。执行产生的实际文件记录在 `Resource` 上，归属于 `Run`。

## 3. 核心模型

### 3.1 ResourceSpace — 路径空间

表示一个外部的、可按路径寻址的存储空间。它是 **Provider 的操作对象**。

```go
type ResourceSpace struct {
    ID        int64          `json:"id"`
    ProjectID int64          `json:"project_id"`
    Kind      string         `json:"kind"`       // "git" | "local_fs" | "s3" | "http" | "webdav"
    RootURI   string         `json:"root_uri"`   // 空间根路径，Provider 按此定位
    Role      string         `json:"role"`       // "primary_repo" | "shared_drive" | "media_store" | "reference"
    Label     string         `json:"label,omitempty"`
    Config    map[string]any `json:"config,omitempty"` // branch, credentials, prefix...
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
}
```

**关键决策**：

- `ProjectID` 直接放在 ResourceSpace 上，不用单独的 Link 表。原因：当前系统不需要跨项目共享空间，一张表更简单。如果未来需要共享，再拆出 `ProjectResourceSpaceLink`。
- `RootURI` 是唯一定位真相源。`Config` 存放 provider 特定的解析细节（如 git branch、S3 region），不作为定位依据。

**典型实例**：

```
{Kind: "git",      RootURI: "https://github.com/org/repo.git",  Role: "primary_repo"}
{Kind: "local_fs", RootURI: "/data/shared-drive",               Role: "shared_drive"}
{Kind: "s3",       RootURI: "s3://media-bucket/project-42",     Role: "media_store"}
{Kind: "http",     RootURI: "https://docs.example.com",         Role: "reference"}
```

### 3.2 Resource — 文件/对象

表示一个具体的文件。有实际内容、有大小、可以下载。

```go
type Resource struct {
    ID        int64          `json:"id"`
    ProjectID int64          `json:"project_id"` // 所有资源都归属项目（上下文）

    // ── 归属范围（最多设一个，都不设 = 项目级通用资源）──
    WorkItemID *int64        `json:"work_item_id,omitempty"` // 工作项输入/附件
    RunID      *int64        `json:"run_id,omitempty"`       // 执行产出
    MessageID  *int64        `json:"message_id,omitempty"`   // 消息附件

    // ── 资源定位 ──
    StorageKind string       `json:"storage_kind"`  // "local" | "s3" | "oss" | ...
    URI         string       `json:"uri"`            // 实际存储位置（磁盘路径、对象 URL）
    Role        string       `json:"role"`           // "input" | "output" | "attachment" | "reference"

    // ── 文件元数据 ──
    FileName  string         `json:"file_name"`             // 原始文件名
    MimeType  string         `json:"mime_type,omitempty"`
    SizeBytes int64          `json:"size_bytes,omitempty"`
    Checksum  string         `json:"checksum,omitempty"`    // 可选，用于校验

    // ── 通用 ──
    Metadata  map[string]any `json:"metadata,omitempty"`    // 扩展字段
    CreatedAt time.Time      `json:"created_at"`
}
```

**关键决策**：

- **直接 FK 而非多态**：用 `WorkItemID` / `RunID` / `MessageID` 三个可空外键表示归属，不用 `OwnerType + OwnerID` 多态模式。原因：归属类型是有限且已知的（只有 4 种），直接 FK 查询更快、有参照完整性、SQLite 友好。
- **`StorageKind` 而非 `Kind`**：Resource 的 Kind 描述的是存储方式（"这个文件存在哪"），不是资源类型。和 ResourceSpace 的 Kind（"这个空间是什么"）是不同概念，用不同字段名避免混淆。
- **`FileName` 独立字段**：原来用 `Label` 兼做文件名，语义模糊。文件名是文件的固有属性。
- **不可变**：Resource 创建后不可修改（文件不会变），只能删除重建。没有 `UpdatedAt`。
- **不做引用计数**：当前阶段删除 Resource 直接删除物理文件。未来需要 dedup 时再加。

### 3.3 ActionIODecl — Action I/O 声明

声明一个 Action 执行时需要读取或写出的资源。它是**编排层的契约**，不是实际文件记录。

```go
type ActionIODecl struct {
    ID          int64          `json:"id"`
    ActionID    int64          `json:"action_id"`
    Direction   IODirection    `json:"direction"`   // "input" | "output"
    SpaceID     *int64         `json:"space_id,omitempty"`     // 从哪个空间读/写
    ResourceID  *int64         `json:"resource_id,omitempty"`  // 直接引用某个已有文件
    Path        string         `json:"path"`                   // 空间内的相对路径
    MediaType   string         `json:"media_type,omitempty"`
    Description string         `json:"description,omitempty"`
    Required    bool           `json:"required"`
    CreatedAt   time.Time      `json:"created_at"`
}

type IODirection string

const (
    IOInput  IODirection = "input"
    IOOutput IODirection = "output"
)
```

**两种输入模式**：

| 模式 | 字段 | 场景 |
|---|---|---|
| 空间路径 | `SpaceID` + `Path` | "从 Git 仓库读 `src/main.go`" |
| 直接引用 | `ResourceID` | "使用工作项附件里上传的 spec.md" |

**输出始终归 Run**：ActionIODecl 只声明 "期望输出到哪里"。实际产出记录在 `Resource{RunID: runID}` 上。

## 4. 每个模块的资源形态

### 4.1 Project 级 — 路径空间

```
用户操作：配置项目的代码仓库、共享存储
模型实体：ResourceSpace
生命周期：跟随项目，手动管理
```

```
Project
  └── ResourceSpace[]
      ├── {Kind: "git",      Role: "primary_repo",  RootURI: "..."}
      ├── {Kind: "s3",       Role: "media_store",   RootURI: "..."}
      └── {Kind: "local_fs", Role: "shared_drive",  RootURI: "..."}
```

项目级不产生 `Resource`（文件），它提供的是空间——Action 在运行时从空间里读文件、往空间里写文件。

### 4.2 WorkItem 级 — 需求输入

```
用户操作：上传需求文档、设计图、参考资料
模型实体：Resource{WorkItemID: xxx}
生命周期：跟随工作项，用户可删除
```

```
WorkItem #42
  ├── Resource{Role: "input",     FileName: "需求文档.md",   MimeType: "text/markdown"}
  ├── Resource{Role: "input",     FileName: "UI设计.png",    MimeType: "image/png"}
  └── Resource{Role: "reference", FileName: "API文档.pdf",   MimeType: "application/pdf"}
```

**替代方案对比**：

```
旧：ResourceBinding{Kind: "attachment", Config: {"mime_type": "...", "size": 2048}}
新：Resource{StorageKind: "local", MimeType: "...", SizeBytes: 2048, Role: "input"}
```

不再有 `Kind="attachment"` 这个杂交概念。上传的文件就是一个 `Resource`，存在本地磁盘上（`StorageKind="local"`），归属于某个 WorkItem（`WorkItemID` 设置），作用是输入（`Role="input"`）。

### 4.3 Run 级 — 执行产出

```
产出方式：Agent 执行完成后，引擎收集产出文件
模型实体：Resource{RunID: xxx}
生命周期：跟随 Run，可查询历史
```

```
Run #7 (Action: "write-article", WorkItem: #42)
  ├── Resource{Role: "output", FileName: "report.md",     MimeType: "text/markdown"}
  ├── Resource{Role: "output", FileName: "analysis.csv",  MimeType: "text/csv"}
  └── Resource{Role: "output", FileName: "diagram.svg",   MimeType: "image/svg+xml"}
```

**替代方案对比**：

```
旧：Run.ResultAssets = []{Name: "report.md", URI: "/path/...", MediaType: "text/markdown"}
新：Resource{RunID: 7, FileName: "report.md", URI: "/data/outputs/runs/7/report.md", ...}
```

从内联 JSON 变为独立记录。可以按 Run 查询、按项目汇总、有完整元数据。

**Run.ResultMarkdown 保留**：作为执行摘要文本，不是文件。它和 Resource 互补——摘要看 ResultMarkdown，文件看 Resource 列表。

### 4.4 Message 级 — 对话附件

```
用户操作：在 Thread 聊天中发送/接收文件
模型实体：Resource{MessageID: xxx}
生命周期：跟随消息
```

```
ThreadMessage #99 (Thread #5, sender: user)
  ├── Resource{Role: "attachment", FileName: "screenshot.png",  MimeType: "image/png"}
  └── Resource{Role: "attachment", FileName: "error-log.txt",   MimeType: "text/plain"}

ThreadMessage #102 (Thread #5, sender: agent)
  └── Resource{Role: "output",     FileName: "fix-patch.diff",  MimeType: "text/x-diff"}
```

**当前缺口**：ThreadMessage 完全没有文件支持。统一后，消息附件和工作项附件复用同一套 Resource + 上传/下载逻辑，只是归属范围不同（`MessageID` vs `WorkItemID`）。

## 5. 全景关系图

```
Project
  │
  ├── ResourceSpace[]  ← 路径空间（git / S3 / local_fs / ...）
  │     │
  │     └── ActionIODecl 引用 ← "从这个空间的这个路径读/写"
  │
  ├── WorkItem
  │     │
  │     ├── Resource[]{Role: input}     ← 用户上传的需求文件
  │     │     │
  │     │     └── ActionIODecl 引用     ← "使用这个已有文件作为输入"
  │     │
  │     ├── Action[]
  │     │     └── ActionIODecl[]        ← I/O 声明（编排层契约）
  │     │           ├── SpaceID + Path  ← 空间路径模式
  │     │           └── ResourceID      ← 直接引用模式
  │     │
  │     └── Run[]
  │           └── Resource[]{Role: output}  ← 执行产出文件
  │
  └── Thread
        └── Message[]
              └── Resource[]{Role: attachment | output}  ← 消息附件
```

## 6. Provider 层调整

### 6.1 SpaceProvider — 空间操作

对应 ResourceSpace，负责从路径空间读写文件。替代原来的 `ResourceProvider`。

```go
type SpaceProvider interface {
    Kind() string
    Fetch(ctx context.Context, space *ResourceSpace, path string, destDir string) (localPath string, err error)
    Deposit(ctx context.Context, space *ResourceSpace, path string, localPath string) error
}
```

实现：`LocalFSSpaceProvider`、`HTTPSpaceProvider`（已有）、`GitSpaceProvider`、`S3SpaceProvider`（待补）。

### 6.2 FileStore — 文件存储

对应 Resource 的物理存储，负责存取具体文件。这是内部基础设施，不对外暴露。

```go
type FileStore interface {
    Save(ctx context.Context, fileName string, r io.Reader) (uri string, size int64, err error)
    Open(ctx context.Context, uri string) (io.ReadCloser, error)
    Delete(ctx context.Context, uri string) error
}
```

第一阶段只实现 `LocalFileStore`（写入 `{dataDir}/files/{hash_prefix}/{filename}`）。未来可实现 S3FileStore 等。

**SpaceProvider vs FileStore 的区别**：

| | SpaceProvider | FileStore |
|---|---|---|
| 操作对象 | ResourceSpace | Resource |
| 寻址方式 | 空间 + 相对路径 | 内部 URI |
| 典型场景 | 从 Git 仓库拉代码 | 保存上传的附件 |
| 谁调用 | ActionIO 解析器 | 上传/产出收集逻辑 |
| 双向 | 是（Fetch + Deposit） | 是（Save + Open + Delete） |

## 7. API 设计

### 7.1 ResourceSpace API（项目级空间管理）

```
POST   /projects/{projectID}/spaces          创建空间
GET    /projects/{projectID}/spaces          列出空间
GET    /spaces/{spaceID}                     获取空间
PUT    /spaces/{spaceID}                     更新空间
DELETE /spaces/{spaceID}                     删除空间
```

### 7.2 Resource API（统一文件操作）

```
POST   /work-items/{id}/resources            上传工作项文件（multipart）
GET    /work-items/{id}/resources            列出工作项文件
POST   /messages/{id}/resources              上传消息附件（multipart）
GET    /messages/{id}/resources              列出消息附件
GET    /runs/{id}/resources                  列出执行产出
GET    /resources/{id}                       获取文件元数据
GET    /resources/{id}/download              下载文件内容
DELETE /resources/{id}                       删除文件
```

所有端点共享同一套 handler 逻辑，只是归属范围不同。

### 7.3 ActionIODecl API

```
POST   /actions/{actionID}/io-decls          创建 I/O 声明
GET    /actions/{actionID}/io-decls          列出 I/O 声明
DELETE /io-decls/{declID}                    删除声明
```

### 7.4 接口切换策略

| 旧端点 | 处理策略 |
|---|---|
| `POST /work-items/{id}/attachments` | 由 `POST /work-items/{id}/resources` 取代 |
| `GET /attachments/{id}` | 由 `GET /resources/{id}` 取代 |
| `GET /attachments/{id}/download` | 由 `GET /resources/{id}/download` 取代 |
| `GET /artifacts/{id}` | 由 `Resource{RunID}` 投影的新资源查询取代 |
| `GET /executions/{id}/artifacts` | 由基于 `resources` 的 Run 产出查询取代 |

原则：不为旧命名长期保留平行接口；迁移完成后直接删除旧端点。

## 8. 数据库 Schema

### 8.1 新增表

```sql
-- 路径空间
CREATE TABLE resource_spaces (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES projects(id),
    kind       TEXT    NOT NULL,  -- 'git' | 'local_fs' | 's3' | 'http' | 'webdav'
    root_uri   TEXT    NOT NULL,
    role       TEXT    NOT NULL DEFAULT '',
    label      TEXT    NOT NULL DEFAULT '',
    config     TEXT    NOT NULL DEFAULT '{}',  -- JSON
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_resource_spaces_project ON resource_spaces(project_id);

-- 文件/对象
CREATE TABLE resources (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id   INTEGER NOT NULL REFERENCES projects(id),
    work_item_id INTEGER REFERENCES issues(id),
    run_id       INTEGER REFERENCES executions(id),
    message_id   INTEGER REFERENCES thread_messages(id),
    CHECK (
        (CASE WHEN work_item_id IS NOT NULL THEN 1 ELSE 0 END) +
        (CASE WHEN run_id IS NOT NULL THEN 1 ELSE 0 END) +
        (CASE WHEN message_id IS NOT NULL THEN 1 ELSE 0 END) <= 1
    ),
    storage_kind TEXT    NOT NULL DEFAULT 'local',
    uri          TEXT    NOT NULL,
    role         TEXT    NOT NULL DEFAULT '',
    file_name    TEXT    NOT NULL DEFAULT '',
    mime_type    TEXT    NOT NULL DEFAULT '',
    size_bytes   INTEGER NOT NULL DEFAULT 0,
    checksum     TEXT    NOT NULL DEFAULT '',
    metadata     TEXT    NOT NULL DEFAULT '{}',  -- JSON
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_resources_work_item ON resources(work_item_id) WHERE work_item_id IS NOT NULL;
CREATE INDEX idx_resources_run       ON resources(run_id) WHERE run_id IS NOT NULL;
CREATE INDEX idx_resources_message   ON resources(message_id) WHERE message_id IS NOT NULL;
CREATE INDEX idx_resources_project   ON resources(project_id);

-- Action I/O 声明
CREATE TABLE action_io_decls (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    action_id   INTEGER NOT NULL REFERENCES steps(id),
    direction   TEXT    NOT NULL,  -- 'input' | 'output'
    space_id    INTEGER REFERENCES resource_spaces(id),
    resource_id INTEGER REFERENCES resources(id),
    path        TEXT    NOT NULL DEFAULT '',
    media_type  TEXT    NOT NULL DEFAULT '',
    description TEXT    NOT NULL DEFAULT '',
    required    BOOLEAN NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_action_io_decls_action ON action_io_decls(action_id, direction);
```

### 8.2 废弃表/字段（迁移后删除）

| 旧 | 新 | 迁移方式 |
|---|---|---|
| `resource_bindings` (kind != attachment) | `resource_spaces` | 按 Kind 映射 |
| `resource_bindings` (kind = attachment) | `resources` | WorkItemID + 文件元数据 |
| `action_resources` | `action_io_decls` | 字段改名 |
| `executions.result_assets` | `resources` (run_id) | JSON 展开为行 |

## 9. Store 接口

```go
// ResourceSpaceStore 持久化路径空间。
type ResourceSpaceStore interface {
    CreateResourceSpace(ctx context.Context, rs *ResourceSpace) (int64, error)
    GetResourceSpace(ctx context.Context, id int64) (*ResourceSpace, error)
    ListResourceSpaces(ctx context.Context, projectID int64) ([]*ResourceSpace, error)
    UpdateResourceSpace(ctx context.Context, rs *ResourceSpace) error
    DeleteResourceSpace(ctx context.Context, id int64) error
}

// ResourceStore 持久化文件/对象记录。
type ResourceStore interface {
    CreateResource(ctx context.Context, r *Resource) (int64, error)
    GetResource(ctx context.Context, id int64) (*Resource, error)
    ListResourcesByWorkItem(ctx context.Context, workItemID int64) ([]*Resource, error)
    ListResourcesByRun(ctx context.Context, runID int64) ([]*Resource, error)
    ListResourcesByMessage(ctx context.Context, messageID int64) ([]*Resource, error)
    DeleteResource(ctx context.Context, id int64) error
}

// ActionIODeclStore 持久化 Action I/O 声明。
type ActionIODeclStore interface {
    CreateActionIODecl(ctx context.Context, decl *ActionIODecl) (int64, error)
    GetActionIODecl(ctx context.Context, id int64) (*ActionIODecl, error)
    ListActionIODecls(ctx context.Context, actionID int64) ([]*ActionIODecl, error)
    ListActionIODeclsByDirection(ctx context.Context, actionID int64, dir IODirection) ([]*ActionIODecl, error)
    DeleteActionIODecl(ctx context.Context, id int64) error
}
```

## 10. 实施计划

### 实施原则

- **交付口径是全部完成**：下面的 Phase 只是实现顺序，不代表拆成多个长期独立交付物；本任务完成的标准是 5 个 Phase 全部落地。
- **统一事实源一次到位**：项目级路径空间、文件对象、Action I/O 声明都切到新模型，不为旧模型保留长期事实源地位。
- **接口切换以收口为目标**：迁移窗口应尽量短，不为旧命名长期保留平行 API。
- **实现顺序可以分阶段，验收必须整体完成**：允许先落表、再迁数据、再切接口、再清理旧模型，但最终验收按完整目标判断。

### Phase 1：核心模型与存储落地

- 新增 `ResourceSpace`、`Resource`、`ActionIODecl` 三个核心模型
- 新增 `resource_spaces`、`resources`、`action_io_decls` 三张表
- 新增 `ResourceSpaceStore`、`ResourceStore`、`ActionIODeclStore`
- 新增本地 `FileStore` 实现，作为附件与产出文件的统一物理存储
- 补齐基础约束，确保 `Resource` 单归属、`ActionIODecl` 引用关系明确

验收：三模型、三张表、三类 Store 与文件存储能力全部到位，可以承接后续迁移。

### Phase 2：旧数据与旧声明迁移

- 将 `resource_bindings(kind != attachment)` 迁移到 `resource_spaces`
- 将 `resource_bindings(kind = attachment)` 迁移到 `resources`
- 将 `action_resources` 迁移到 `action_io_decls`
- 将 `executions.result_assets` 展开迁移到 `resources`
- 完成迁移校验，确保旧事实记录能在新模型中无损表达

验收：旧数据全部进入新表，且可通过新模型完整读取。

### Phase 3：读写链路切换

- 项目级资源管理接口切到 `ResourceSpace`
- 工作项附件上传/下载/删除切到 `Resource`
- Run 产出查询切到 `Resource`
- Action 输入输出声明切到 `ActionIODecl`
- 新增消息附件上传、列表、下载接口，消息文件落到 `Resource{MessageID}`
- 前端统一收敛到 `ResourceSpace`、`Resource`、`ActionIODecl` 三套新类型与接口

验收：所有新增写入与所有业务读取都走新模型，消息附件已纳入统一资源体系。

### Phase 4：Provider 与编排层重命名收口

- Provider 改为操作 `ResourceSpace` 而非 `ResourceBinding`
- `ResourceResolver` 改为 `ActionIOResolver`
- 与此相关的命名、接口、调用路径全部完成收口，不再暴露旧的 binding/resource declaration 语义

验收：运行时编排层、Provider 层、Resolver 层全部切到 `ResourceSpace` 与 `ActionIODecl`。

### Phase 5：清理

- 删除旧表、旧类型、旧 API 端点
- 清理前端旧类型和临时迁移别名
- 更新文档与测试基线

验收：仓库中不再保留 `ResourceBinding`、`ActionResource`、`Asset(result_assets 事实源)`、旧附件 DTO 与旧资源端点。

### 实施顺序说明

- 上述 5 个 Phase 只是内部施工顺序，用于控制迁移风险和减少返工。
- 交付口径不是“完成前几个 Phase”，而是 **5 个 Phase 全部完成后统一验收**。
- 如果中途需要临时桥接代码，只允许作为迁移脚手架存在，并必须在 Phase 5 中一并删除。

### 风险控制

- **迁移窗口拖长**：旧新模型并存时间过长时，很容易重新形成双轨。
- **数据迁移遗漏**：项目资源、附件、Run 产出、消息附件、Action I/O 声明都需要有回填校验与抽样核对。
- **前后端契约不一致**：后端切到新模型时，前端类型与列表/下载/详情接口要同步收敛。
- **Provider / Resolver 收口不彻底**：如果只迁表不收命名和调用链，旧语义会继续残留。
- **约束不够导致脏数据**：`resources` 必须坚持单归属原则，避免同一条记录同时挂到多个 owner。

### 总体验收

- 项目级路径空间统一落在 `resource_spaces`
- 工作项附件、Run 产出、消息附件都统一落在 `resources`
- Action 输入输出声明统一落在 `action_io_decls`
- `resource_bindings`、`action_resources`、`executions.result_assets` 都不再作为事实源
- 旧附件 / artifact API 在切换完成后应直接删除，不保留长期兼容层
- 项目级资源管理、文件管理、Action I/O 声明、运行时编排层全部完成新命名收口
- 旧表、旧类型、旧 DTO、旧端点、临时桥接代码全部删除

## 11. 与 v2 计划的关系

本文档是 v2 计划的**简化实施版**。核心采纳了 v2 的两个关键洞察：

1. **路径空间和文件对象是不同的东西**（ResourceSpace vs Resource）
2. **I/O 声明和执行事实是不同的东西**（ActionIODecl vs Resource{RunID}）

相比 v2 做了以下简化：

| v2 方案 | 本方案 | 理由 |
|---|---|---|
| `ResourceRef` 多态引用表 | 直接 FK（WorkItemID/RunID/MessageID） | 归属类型有限且已知，直接 FK 更简单 |
| `DeliveryBatch` 交付批次 | 暂不引入 | 当前没有 "分批交付" 的业务需求 |
| `ObjectStore` 底层抽象 | `FileStore`（更聚焦） | 只做文件存取，不做通用对象存储 |
| `ProjectResourceSpaceLink` | `ResourceSpace.ProjectID` 直接 FK | 暂不需要跨项目共享空间 |
| `SourceType + SourceID` 溯源 | 直接 FK | 同上理由 |

当未来出现跨项目共享空间、分批交付、多后端对象存储等需求时，可以从本方案平滑演进到 v2 的完整形态。
