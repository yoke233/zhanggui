# 2026-03-14 Thread Deliverable & Asset Store 设计方案

## 1. 背景与动机

### 1.1 当前问题

Chat Session（单聊）创建 worktree 来隔离文件操作，但 Thread（多人讨论）没有。
经过讨论，确定 Thread 的定位是**跨项目讨论空间**，不应拥有文件修改权限：

- Thread 可能涉及多个项目，给它绑定单一 worktree 语义不对。
- Thread 中的 agent 职责是**思考和产出结构化内容**，不是写代码。
- 代码修改应通过 Thread → Work Item 流转后，在 Work Item 的执行环境（带 worktree）中完成。

但当前 Thread → Work Item 的内容传递非常薄——只传 `thread.Summary` 或手动填的 body 字符串。
讨论中产生的决策、方案、截图、架构图等全部丢失。

### 1.2 目标

1. Thread 讨论能产出**结构化的内容物**（ThreadDeliverable），包含文本和附件资产。
2. ThreadDeliverable 能完整流转到 Work Item，成为 Action 执行的上下文。
3. 附件资产（图片、PDF 等）的存储后端可插拔——本地开发用文件系统，远程协作用 S3。

## 2. 设计概览

```
Thread 讨论
  │
  ├── ThreadDeliverable #1 (kind="spec", assets=[arch.png])
  ├── ThreadDeliverable #2 (kind="decision")
  │
  └── 创建 Work Item ──→ WorkItem.Body ← deliverable.Content
                          WorkItem.Metadata ← source_deliverable_id
                            │
                            ↓
                          Action.Input (引擎组装 briefing 时拉取 deliverable + assets)
                            │
                            ↓
                          Run → Deliverable (执行产出，已有概念)
```

## 3. 领域模型

### 3.1 ThreadDeliverable

```go
// internal/core/thread_deliverable.go

// ThreadDeliverable 是 Thread 讨论的结构化产出。
// 与 Run 的 Deliverable（代码执行结果）不同，这是讨论的结论性内容。
type ThreadDeliverable struct {
    ID        int64          `json:"id"`
    ThreadID  int64          `json:"thread_id"`
    Kind      string         `json:"kind"`       // "spec", "decision", "action_plan", "review", "notes"
    Title     string         `json:"title"`
    Content   string         `json:"content"`     // markdown 正文
    Assets    []Asset        `json:"assets,omitempty"` // 复用已有 Asset 类型
    Metadata  map[string]any `json:"metadata,omitempty"`
    CreatedBy string         `json:"created_by"`  // user ID 或 agent profile ID
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
}
```

**Kind 枚举说明：**

| Kind          | 用途                     | 典型流转目标           |
|---------------|--------------------------|------------------------|
| `spec`        | 需求规格 / 技术方案       | → WorkItem.Body        |
| `decision`    | 讨论决策记录              | → WorkItem.Metadata    |
| `action_plan` | 拆分后的执行计划          | → 多个 WorkItem        |
| `review`      | 评审意见                  | → 关联 WorkItem 的备注 |
| `notes`       | 一般性笔记                | 存档，不流转            |

### 3.2 AssetStore 接口

```go
// internal/core/asset_store.go

// AssetStore 抽象资产文件的存储后端。
// DB 只存 Asset 元数据（name, uri, media_type），实际文件由 AssetStore 管理。
type AssetStore interface {
    // Upload 存储资产文件，返回统一 URI。
    Upload(ctx context.Context, scope AssetScope, name string, r io.Reader, mediaType string) (uri string, err error)

    // Open 按 URI 读取资产文件。
    Open(ctx context.Context, uri string) (io.ReadCloser, error)

    // Delete 按 URI 删除资产文件。
    Delete(ctx context.Context, uri string) error
}

// AssetScope 标识资产所属的聚合根。
type AssetScope struct {
    Kind string // "thread-deliverable", "run-deliverable"
    ID   int64  // deliverable ID
}
```

### 3.3 URI 约定

DB 中存储的 URI 使用统一的逻辑路径，不暴露存储后端细节：

```
assets://thread-deliverables/{threadID}/{deliverableID}/{filename}
assets://run-deliverables/{runID}/{deliverableID}/{filename}
```

AssetStore 实现负责将逻辑 URI 映射到实际存储位置：

| 后端  | 逻辑 URI                                           | 实际位置                                                     |
|-------|-----------------------------------------------------|--------------------------------------------------------------|
| Local | `assets://thread-deliverables/42/1/arch.png`        | `$DATA_DIR/assets/thread-deliverables/42/1/arch.png`         |
| S3    | `assets://thread-deliverables/42/1/arch.png`        | `s3://bucket/prefix/thread-deliverables/42/1/arch.png`       |

## 4. 存储层实现

### 4.1 LocalAssetStore

```go
// internal/adapters/storage/asset/local.go

type LocalAssetStore struct {
    baseDir string // $AI_WORKFLOW_DATA_DIR/assets
}

func (s *LocalAssetStore) Upload(ctx context.Context, scope AssetScope, name string, r io.Reader, mediaType string) (string, error) {
    // 1. 拼接路径: baseDir / scope.Kind / scope.ID / name
    // 2. 创建目录 (os.MkdirAll)
    // 3. 写入文件 (atomic write via temp + rename)
    // 4. 返回 "assets://{scope.Kind}/{scope.ID}/{name}"
}

func (s *LocalAssetStore) Open(ctx context.Context, uri string) (io.ReadCloser, error) {
    // 1. 解析 assets:// URI → 本地路径
    // 2. os.Open
}

func (s *LocalAssetStore) Delete(ctx context.Context, uri string) error {
    // 1. 解析 URI → 本地路径
    // 2. os.Remove
}
```

### 4.2 S3AssetStore

```go
// internal/adapters/storage/asset/s3.go

type S3AssetStore struct {
    client *s3.Client
    bucket string
    prefix string // e.g. "assets/"
}

func (s *S3AssetStore) Upload(ctx context.Context, scope AssetScope, name string, r io.Reader, mediaType string) (string, error) {
    // 1. 拼接 key: prefix / scope.Kind / scope.ID / name
    // 2. s3.PutObject
    // 3. 返回 "assets://{scope.Kind}/{scope.ID}/{name}" (同样的逻辑 URI)
}

func (s *S3AssetStore) Open(ctx context.Context, uri string) (io.ReadCloser, error) {
    // 1. 解析 URI → S3 key
    // 2. s3.GetObject
}
```

### 4.3 配置

```toml
# .ai-workflow/config.toml
[storage.assets]
backend = "local"          # "local" | "s3"

[storage.assets.s3]
bucket = "my-workflow"
region = "us-east-1"
prefix = "assets/"
# endpoint = "..."        # 可选，MinIO 等兼容存储
```

### 4.4 工厂

```go
// internal/adapters/storage/asset/factory.go

func NewAssetStore(cfg config.AssetStorageConfig, dataDir string) (core.AssetStore, error) {
    switch cfg.Backend {
    case "local", "":
        return &LocalAssetStore{baseDir: filepath.Join(dataDir, "assets")}, nil
    case "s3":
        return newS3AssetStore(cfg.S3)
    default:
        return nil, fmt.Errorf("unknown asset storage backend %q", cfg.Backend)
    }
}
```

## 5. DB Schema

### 5.1 thread_deliverables 表

```sql
CREATE TABLE thread_deliverables (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id   INTEGER NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    kind        TEXT    NOT NULL DEFAULT 'notes',
    title       TEXT    NOT NULL,
    content     TEXT    NOT NULL DEFAULT '',
    metadata    TEXT,           -- JSON
    created_by  TEXT    NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_thread_deliverables_thread ON thread_deliverables(thread_id);
```

### 5.2 deliverable_assets 表

统一存储 ThreadDeliverable 和 Run Deliverable 的资产引用：

```sql
CREATE TABLE deliverable_assets (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    deliverable_type TEXT    NOT NULL,  -- "thread" | "run"
    deliverable_id   INTEGER NOT NULL,
    name             TEXT    NOT NULL,
    uri              TEXT    NOT NULL,  -- assets://... 逻辑 URI
    media_type       TEXT,
    size_bytes       INTEGER,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_deliverable_assets_ref ON deliverable_assets(deliverable_type, deliverable_id);
```

> **说明：** 当前 Run 的 Deliverable 把 `Assets []Asset` 内联为 JSON 存在 deliverables 表的字段里。
> 新表 `deliverable_assets` 将 asset 独立出来，方便查询和级联删除。
> 存量 Run Deliverable 的 assets 可以保持 JSON 内联不迁移，新数据走新表。

## 6. ThreadStore 接口扩展

```go
// 追加到 core/thread.go 的 ThreadStore interface

// ThreadDeliverable CRUD
CreateThreadDeliverable(ctx context.Context, d *ThreadDeliverable) (int64, error)
GetThreadDeliverable(ctx context.Context, id int64) (*ThreadDeliverable, error)
ListThreadDeliverables(ctx context.Context, threadID int64) ([]*ThreadDeliverable, error)
UpdateThreadDeliverable(ctx context.Context, d *ThreadDeliverable) error
DeleteThreadDeliverable(ctx context.Context, id int64) error

// Deliverable Assets (供 AssetStore 配合使用)
AddDeliverableAsset(ctx context.Context, deliverableType string, deliverableID int64, asset *Asset) (int64, error)
ListDeliverableAssets(ctx context.Context, deliverableType string, deliverableID int64) ([]Asset, error)
DeleteDeliverableAsset(ctx context.Context, assetID int64) error
```

## 7. HTTP API

### 7.1 ThreadDeliverable 端点

```
POST   /threads/{threadID}/deliverables                    创建 deliverable
GET    /threads/{threadID}/deliverables                    列出 deliverables
GET    /threads/{threadID}/deliverables/{deliverableID}    获取单个
PUT    /threads/{threadID}/deliverables/{deliverableID}    更新
DELETE /threads/{threadID}/deliverables/{deliverableID}    删除（级联删 assets）
```

### 7.2 Asset 上传/下载端点

```
POST   /threads/{threadID}/deliverables/{deliverableID}/assets    上传资产 (multipart/form-data)
GET    /assets/{uri...}                                           下载资产 (通用，按 URI 路由)
DELETE /threads/{threadID}/deliverables/{deliverableID}/assets/{assetID}  删除单个资产
```

上传请求示例：

```
POST /threads/42/deliverables/1/assets
Content-Type: multipart/form-data

file: <binary>
name: arch.png           (可选，默认取文件名)
media_type: image/png    (可选，默认 auto-detect)
```

响应：

```json
{
  "id": 7,
  "name": "arch.png",
  "uri": "assets://thread-deliverables/42/1/arch.png",
  "media_type": "image/png",
  "size_bytes": 204800
}
```

### 7.3 Thread → Work Item 流转增强

改造 `POST /threads/{threadID}/create-work-item`：

```json
// 请求 — 新增 deliverable_ids 字段
{
  "title": "实现 JWT 认证",
  "body": "",
  "project_id": 1,
  "deliverable_ids": [1, 2]    // ← 新增：指定哪些 deliverable 流转
}
```

处理逻辑：

1. 如果提供了 `deliverable_ids`：
   - 拉取对应 ThreadDeliverable 列表
   - 按 kind 组装 WorkItem.Body（spec → 正文，decision → 引用块）
   - Asset URI 写入 WorkItem.Metadata（`"source_assets": [...]`）
2. 如果没有提供，行为与当前一致（用 summary 或手动 body）

## 8. Thread → Work Item 内容组装规则

```go
func assembleWorkItemBody(deliverables []*ThreadDeliverable) string {
    // 按 kind 排序: spec > action_plan > decision > review > notes
    // 拼接 markdown:
    //
    // ## 需求规格
    // {spec deliverable content}
    //
    // ## 决策记录
    // > {decision deliverable content}
    //
    // ## 执行计划
    // {action_plan deliverable content}
    //
    // ---
    // _来源: Thread #42, Deliverable #1, #2_
}
```

Asset 流转到 Work Item 后，Action 的 agent 可以通过 `AssetStore.Open(uri)` 读取：

```
WorkItem.Metadata = {
    "source_thread_id": 42,
    "source_deliverable_ids": [1, 2],
    "source_assets": [
        {"name": "arch.png", "uri": "assets://thread-deliverables/42/1/arch.png", "media_type": "image/png"}
    ]
}
```

引擎在组装 Action.Input（briefing）时，将 asset 列表附到 briefing 末尾，
agent 可通过 MCP/ACP 工具读取这些 URI 对应的文件。

## 9. 实现路线

### Phase 1：基础模型与本地存储

1. `internal/core/thread_deliverable.go` — ThreadDeliverable 类型
2. `internal/core/asset_store.go` — AssetStore 接口 + AssetScope
3. `internal/adapters/storage/asset/local.go` — LocalAssetStore 实现
4. `internal/adapters/storage/asset/factory.go` — 工厂函数
5. DB migration — `thread_deliverables` + `deliverable_assets` 表
6. `internal/adapters/store/sqlite/thread_deliverable.go` — SQLite store 实现
7. `internal/adapters/http/thread_deliverable.go` — HTTP 端点
8. 测试

### Phase 2：流转增强

1. 改造 `createWorkItemFromThreadDataWithStore` — 支持 `deliverable_ids` 参数
2. `assembleWorkItemBody` — 内容组装函数
3. 引擎 briefing 组装 — Action.Input 中拉取关联的 assets
4. 前端 `web/src/types/apiV2.ts` — 同步 ThreadDeliverable 类型
5. 测试

### Phase 3：S3 支持

1. `internal/adapters/storage/asset/s3.go` — S3AssetStore 实现
2. `internal/support/config/` — 解析 `[storage.assets]` 配置
3. 集成测试（用 MinIO 或 LocalStack）

## 10. 不做的事

- **Thread 不创建 worktree** — 已确认 Thread 是讨论空间，不操作文件系统。
- **不迁移存量 Run Deliverable 的 assets** — 存量继续用 JSON 内联，新数据走新表。
- **不做资产版本管理** — 同名重传直接覆盖，简单可预期。
- **不做资产大小限制的硬编码** — 通过配置指定，默认 50MB。

## 11. 与现有概念的关系

```
┌─────────────────────────────────────────────────────────┐
│ Thread (讨论层，无 worktree)                             │
│   ├── ThreadMessage      讨论消息                        │
│   ├── ThreadDeliverable  讨论产出 ← 新增                 │
│   │     └── Asset        附件资产 (图片/PDF/截图)          │
│   └── ThreadWorkItemLink 流转关系                        │
│         └── WorkItem     执行单元 (有 worktree)           │
│               ├── Action         执行步骤                 │
│               │     └── Run      执行实例                 │
│               │           └── Deliverable  执行产出       │
│               │                 └── Asset  代码产物       │
│               └── ResourceBinding  项目资源绑定            │
└─────────────────────────────────────────────────────────┘

AssetStore (统一接口)
  ├── LocalAssetStore  ← 单机开发
  └── S3AssetStore     ← 远程协作
```
