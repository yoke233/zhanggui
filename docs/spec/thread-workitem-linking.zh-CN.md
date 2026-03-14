# Thread-WorkItem 关联模型规格

> 状态：部分实现
>
> 最后按代码核对：2026-03-14
>
> 当前实现状态：`thread_work_item_links` 表、Thread 侧关联 API 和按 WorkItem 反查 Thread 能力已存在；当前反查入口已经是 `/work-items/{id}/threads`。Store 层已提供按 Thread / WorkItem 清理关联的方法，但 handler 对父对象删除尚未形成统一清理协议；同时当前 GORM model 也没有把外键 / CASCADE 作为稳定契约显式声明。

## 概述

Thread（多人讨论容器）与 WorkItem（Issue，执行主线）之间通过显式链接表 `thread_work_item_links` 建立双向关联。

## 关联表结构

```sql
CREATE TABLE thread_work_item_links (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id   INTEGER NOT NULL,
    work_item_id INTEGER NOT NULL,
    relation_type TEXT NOT NULL DEFAULT 'related',  -- 'related' | 'drives' | 'blocks'
    is_primary  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(thread_id, work_item_id)
);
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

当前实现的真实情况更接近：

1. Store 已提供 `DeleteThreadWorkItemLinksByThread()` 与 `DeleteThreadWorkItemLinksByWorkItem()`
2. 但父对象删除 handler 当前没有统一调用它们
3. 当前 model 级 schema 也没有把 `ON DELETE CASCADE` 声明成现行契约

因此当前真实语义是：

- Thread / WorkItem 删除时的 link 清理还不是稳定契约
- 不能把“数据库外键自动兜底”写成现状
- 如果未来要收口为显式清理或 CASCADE，需要同步更新 store、handler、测试与本文

## API 端点

| Method | Path | 说明 |
|--------|------|------|
| `POST` | `/threads/{threadID}/links/work-items` | 创建 thread-workitem 关联 |
| `GET` | `/threads/{threadID}/work-items` | 查询 thread 关联的 work items |
| `DELETE` | `/threads/{threadID}/links/work-items/{workItemID}` | 删除指定关联 |
| `GET` | `/work-items/{issueID}/threads` | 查询 work item 关联的 threads |

## 约束

- `UNIQUE(thread_id, work_item_id)` 防止重复关联
- thread_id 和 work_item_id 必须引用已存在的记录
- relation_type 限定为 `related`、`drives`、`blocks` 三个值
