# 子计划 C：WorkItem API Alias 收口

> 状态：待执行
> 创建日期：2026-03-13
> 适用方式：可直接交给新会话执行

## 目标

在不重构内部 `Issue` 核心模型的前提下，为系统补齐 `/api/work-items/*` 主契约。

## 允许改动范围

- `internal/adapters/http`
- `web/src/lib/apiClient.ts`
- 相关 HTTP / client 测试

## 禁止改动范围

- `internal/core` 中 `Issue` 的彻底重命名
- 数据库表名
- payload 字段全面 rename

## 关键约束

1. `/api/work-items/*` 是新增主契约
2. `/api/issues/*` 必须继续可用
3. 两套路径行为必须一致
4. 本阶段不要求把响应字段从 `issue_*` 全改成 `work_item_*`
5. 本阶段不删除旧路由

## 任务清单

1. 在 HTTP 路由层新增 `/work-items/*` alias
2. 复用现有 `/issues/*` handler 逻辑
3. 前端 `apiClient` 主调用切到 `/work-items/*`
4. 保留 `issues` 兼容方法或兼容路径
5. 为 alias 补测试，确保：
   - CRUD 一致
   - `run/cancel/archive/steps/generate-steps/events` 一致
   - `threads` 反查 work item 的行为不受影响

## 输出要求

- 优先做 alias，不要先做深度重构
- 不要顺手把内部 service / store 一起大改

## 完成标准

- 前端默认走 `/api/work-items/*`
- `/api/issues/*` 仍保持兼容
- 测试能证明双路由同语义

## 建议验证

- `go test ./internal/adapters/http/...`
- `npm --prefix web test -- apiClient`
- 受影响集成测试

## 新会话提示词

```text
请按 docs/plan/2026-03-13-workitem-api-alias-subplan.md 执行。
本次只完成 /api/work-items alias，不做内部 Issue 模型重命名，不改数据库表名。
要求前端 apiClient 默认切到 /work-items，同时保留 /issues 兼容。
```
