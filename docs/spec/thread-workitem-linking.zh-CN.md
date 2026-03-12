# Thread-WorkItem 关联模型规格

## 概述

Thread（多人讨论容器）与 WorkItem（Issue，执行主线）之间通过显式链接表 `thread_work_item_links` 建立双向关联。

## 关联表结构

```sql
CREATE TABLE thread_work_item_links (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id   INTEGER NOT NULL REFERENCES threads(id),
    work_item_id INTEGER NOT NULL REFERENCES issues(id),
    relation_type TEXT NOT NULL DEFAULT 'related',  -- 'related' | 'drives' | 'blocks'
    is_primary  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(thread_id, work_item_id)
);

CREATE INDEX idx_twil_thread ON thread_work_item_links(thread_id);
CREATE INDEX idx_twil_work_item ON thread_work_item_links(work_item_id);
```

## 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | 自增主键 |
| `thread_id` | int64 | 关联的 Thread ID（FK → threads.id） |
| `work_item_id` | int64 | 关联的 WorkItem/Issue ID（FK → issues.id） |
| `relation_type` | string | 关系类型：`related`（默认）、`drives`、`blocks` |
| `is_primary` | bool | 是否为主关联。每个 Thread 最多一条 `is_primary=true` 的 link |
| `created_at` | datetime | 创建时间 |

## 主关联规则

- 每个 Thread 最多有一条 `is_primary = true` 的 link
- 设置新的 primary link 时，旧 primary 自动降级为 `is_primary = false`
- 主关联用于 UI 默认展示：Thread 页面优先显示 primary WorkItem

## 删除清理策略

采用**显式清理**（非 CASCADE）：

1. **删除 Thread 时**：应用层在删除 thread 前，先删除该 thread 的所有 `thread_work_item_links` 记录
2. **删除 WorkItem/Issue 时**：应用层在删除 issue 前，先删除该 issue 的所有 `thread_work_item_links` 记录
3. **理由**：显式清理允许在删除前做审计日志或通知，避免静默 CASCADE 丢失关联信息

## API 端点

| Method | Path | 说明 |
|--------|------|------|
| `POST` | `/threads/{threadID}/links/work-items` | 创建 thread-workitem 关联 |
| `GET` | `/threads/{threadID}/work-items` | 查询 thread 关联的 work items |
| `DELETE` | `/threads/{threadID}/links/work-items/{workItemID}` | 删除指定关联 |
| `GET` | `/issues/{issueID}/threads` | 查询 work item 关联的 threads |

## 约束

- `UNIQUE(thread_id, work_item_id)` 防止重复关联
- thread_id 和 work_item_id 必须引用已存在的记录
- relation_type 限定为 `related`、`drives`、`blocks` 三个值
