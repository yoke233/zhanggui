# 子计划 A：WorkItem 文档与规则收口

> 状态：已完成（2026-03-13）
> 创建日期：2026-03-13
> 适用方式：可直接交给新会话执行

## 目标

冻结命名规则，统一文档状态标记，阻止 spec 继续漂移。

## 允许改动范围

- `docs/spec`
- `docs/plan`

## 禁止改动范围

- `internal/*`
- `web/src/*`
- 任意业务代码

## 必须遵守的规则

1. 对外产品语义统一使用 `Work Item`
2. Public REST 目标统一为 `/api/work-items/*`
3. 当前实现仍允许内部使用 `Issue`
4. `Flow` 仅允许作为历史兼容或技术流程术语出现
5. 所有相关 spec 必须标状态头：
   - `现行`
   - `部分实现`
   - `草案`
   - `历史`

## 任务清单

1. 复查所有与 `Issue / WorkItem / Flow / Thread / ChatSession` 相关的 spec
2. 把“未来设计写成当前行为”的文档改成 `草案` 或 `部分实现`
3. 把“历史迁移记录”改成 `历史`
4. 在关键 spec 顶部增加“当前实现状态”段
5. 明确：
   - 前端主入口是 `/work-items`
   - 后端主 REST 目前仍是 `/issues`
   - `/flows` 仅为前端兼容 redirect
6. 更新总计划与子计划之间的引用关系

## 输出要求

- 文档内容必须区分“现状”和“目标”
- 不得把尚未实现的 `/api/work-items/*` 写成当前事实
- 不得把 `Flow` 写成当前主业务对象

## 完成标准

- 关键命名相关 spec 都有明确状态头
- 至少一份总规范文档明确约束后续命名方向
- 新人阅读文档后，不会再误以为当前主对象仍是 `Flow`

## 本次完成结果

- 已把 `docs/spec/naming-transition-thread-workitem.zh-CN.md` 定义为现行命名治理规范，并补充“当前实现状态”
- 已把 `docs/spec/thread-workitem-migration-guide.zh-CN.md` 明确标记为历史迁移记录
- 已补齐/规范关键相关 spec 的状态头：`design-issue-centric-model.md`、`thread-workitem-linking.zh-CN.md`、`execution-context-building.zh-CN.md`、`step-context-progressive-loading.zh-CN.md`、`spec-context-memory.md`、`ai-company-domain-model.zh-CN.md`
- 已修正 `thread-workitem-linking.zh-CN.md` 中与当前代码不一致的删除语义描述

## 建议验证

- 手动复读修改后的文档
- 确认没有自相矛盾的“现状描述”

## 新会话提示词

```text
请按 docs/plan/2026-03-13-workitem-docs-governance-subplan.md 执行。
只修改 docs/spec 和 docs/plan。
目标是冻结命名规则、补齐状态头、区分现状与草案，不要改任何业务代码。
```
