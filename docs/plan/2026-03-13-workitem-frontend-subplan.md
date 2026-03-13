# 子计划 B：WorkItem 前端收口

> 状态：待执行
> 创建日期：2026-03-13
> 适用方式：可直接交给新会话执行

## 目标

统一前端内部命名与用户语义，消除 `Flow*` 页面/组件命名对认知的污染。

## 允许改动范围

- `web/src`

## 禁止改动范围

- `internal/*`
- 数据库 schema
- 后端路由注册

## 关键约束

1. 用户可见语义统一为 `Work Item`
2. 前端主路由保持 `/work-items`
3. 如果后端 alias 尚未完成，本线允许继续调用 `/issues` API
4. 不得新增新的 `Flow*` 页面、组件、变量名

## 重点清理对象

- `CreateFlowPage`
- `FlowsPage`
- `FlowDetailPage`
- 路由参数里的 `flowId`
- 仍引用 `/flows/:id/...` 的 i18n 文案
- 以 `Flow` 作为主语义的组件标题、导航、按钮文本

## 任务清单

1. 页面/组件命名切到 `WorkItem`
2. 变量名、props 名、hooks 名尽量切到 `workItem`
3. 清理 i18n 中的 `/flows` 历史文案
4. 保留 `/flows` 和 `/issues` 的前端 redirect，但不让它们继续污染主实现
5. 保证前端编译和单测可通过

## 输出要求

- 以最小可控改动完成语义收口
- 不要为了命名统一而连带重构无关 UI 逻辑

## 完成标准

- 前端主页面和主要组件不再使用 `Flow` 作为主对象名
- 新人从前端代码主路径可以明确看到主语义是 `workItem`

## 建议验证

- `npm --prefix web run typecheck`
- 受影响前端单测
- 路由跳转行为自检

## 新会话提示词

```text
请按 docs/plan/2026-03-13-workitem-frontend-subplan.md 执行。
只改 web/src。
目标是去掉前端主实现中的 Flow 命名，统一到 WorkItem。
如果后端 alias 尚未完成，可以继续兼容调用 /issues API，不要越界改后端。
```
