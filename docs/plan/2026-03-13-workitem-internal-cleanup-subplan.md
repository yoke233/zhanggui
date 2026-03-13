# 子计划 D：WorkItem 内部命名清理

> 状态：待执行
> 创建日期：2026-03-13
> 适用方式：建议在 A/B/C 稳定后启动

## 目标

在 Public Surface 已统一后，逐步减少内部高误导历史命名对开发认知的污染。

## 允许改动范围

- `internal/core`
- `internal/application/flow`
- 其他非 public surface 的命名清理点

## 禁止改动范围

- 数据库表 rename
- 持久层 schema 破坏性变更
- 删除 `/api/issues/*` 兼容层

## 关键约束

1. 这是“内部认知收口”阶段，不是“数据库大迁移”阶段
2. 优先改 exported symbol 和误导性最强的名称
3. 不要把全部历史名一次性铲平

## 优先清理对象

- `FlowScheduler`
- `PRFlowPrompts`
- `flow_pr_bootstrap.go`
- 仍把 `Flow` 写成主业务对象的 exported symbol / 注释 / helper

## 任务清单

1. 建立内部命名替换清单
2. 优先替换最误导的 exported symbol
3. 对需要保留兼容的名称，提供 wrapper 或 deprecated 注释
4. 保证 engine / scheduler / prompt 相关测试可通过

## 输出要求

- 每次只处理一组低耦合命名
- 不把目录级/数据库级 rename 绑进同一波

## 完成标准

- 新开发者阅读内部主线代码时，不会再轻易把 `Flow` 误当成当前主业务对象

## 建议验证

- `go test ./internal/application/flow/...`
- 受影响模块测试

## 新会话提示词

```text
请按 docs/plan/2026-03-13-workitem-internal-cleanup-subplan.md 执行。
这次只做内部命名清理，优先处理 FlowScheduler、PRFlowPrompts、flow_pr_bootstrap 这类高误导名称。
不要改数据库表名，不要删除 /issues 兼容层。
```
