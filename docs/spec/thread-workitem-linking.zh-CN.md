# Thread-WorkItem 关联模型规格

> 状态：部分实现
>
> 最后按代码核对：2026-03-13
>
> 当前实现状态：`thread_work_item_links` 表、Thread 侧关联 API 和按 WorkItem 反查 Thread 能力已存在；当前反查入口仍是 `/issues/{id}/threads`，还没有 `/api/work-items/*` alias；删除父对象时也还没有统一的应用层显式清理协议。

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

## 删除与一致性策略

当前实现不是“应用层显式清理”，也不是 `ON DELETE CASCADE`：

1. SQLite 连接当前开启 `PRAGMA foreign_keys=ON`
2. `thread_work_item_links` 的外键声明没有配置 `ON DELETE CASCADE`
3. 截至 2026-03-13，仓库内没有统一的“先删 link 再删 Thread/Issue”的应用层协议

因此当前真实语义是：

- 若父对象删除路径没有先移除关联，数据库会通过外键约束阻止删除，而不是静默级联删除
- 本文不应把“应用层显式清理”写成现状
- 如果未来要改成显式清理或 CASCADE，需要另开变更并同步更新测试与迁移说明

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
