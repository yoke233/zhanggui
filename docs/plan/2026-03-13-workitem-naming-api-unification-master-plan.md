# WorkItem 命名与 API 契约统一总计划

> 状态：进行中（子计划 A 已完成，其余待执行）
> 创建日期：2026-03-13
> 适用方式：总控 / 分会话并行执行 / 进度追踪

## 目标

本计划用于统一当前系统的命名和 Public API 契约，解决以下历史混用问题：

- 前端主入口是 `/work-items`
- 后端主 REST 仍是 `/issues`
- 前端仍保留 `/flows` 兼容 redirect
- 内部仍存在 `FlowScheduler`、`PRFlowPrompts`、`flow_*` 等历史命名

统一目标如下：

1. 对外产品语义统一为 `Work Item`
2. Public REST API 目标统一为 `/api/work-items/*`
3. 内部短期继续保留 `Issue` 作为实现名
4. `Flow` 降级为历史兼容/技术执行术语，不再作为主业务对象名称继续扩散
5. Thread 与 ChatSession 保持独立概念

## 执行原则

- 先统一对外语义，再统一 API 契约，再渐进清理内部实现
- 第一波不动数据库表名
- 第一波不做 payload 字段全面 rename
- 第一波不把 `Step -> Action`、`Execution -> Run`、`Artifact -> Deliverable` 推成主 API 名
- 并行可以做，但必须按目录和责任边界拆分

## 关联规范

- `docs/spec/naming-transition-thread-workitem.zh-CN.md`
- `docs/spec/design-issue-centric-model.md`
- `docs/spec/thread-workitem-migration-guide.zh-CN.md`

## 计划结构

### 总体路线

- Phase 0：冻结规则与边界
- Phase 1：统一 Public Surface
- Phase 2：前端内部收口
- Phase 3：后端 API alias 收口
- Phase 4：内部领域与 exported symbol 渐进改名
- Phase 5：Flow 兼容层清理
- Phase 6：删除兼容层

### 推荐并行拆分

以下 3 条可独立开会话并行执行：

1. 文档与规则线
2. 前端收口线
3. 后端 API alias 线

以下 1 条建议等前三条稳定后再启动：

4. 内部命名清理线

## 子计划文件

### 子计划 A：文档与规则线

文件：

- `docs/plan/2026-03-13-workitem-docs-governance-subplan.md`

当前进度：

- 已完成（2026-03-13）

负责范围：

- `docs/spec`
- `docs/plan`

任务目标：

- 统一命名规则
- 补齐 spec 状态头
- 明确“现状 / 草案 / 历史 / 部分实现”边界
- 输出迁移纪律和禁止新增项

### 子计划 B：前端收口线

文件：

- `docs/plan/2026-03-13-workitem-frontend-subplan.md`

负责范围：

- `web/src`

任务目标：

- 页面/组件/i18n 去 `Flow`
- 前端内部变量和组件名切到 `workItem`
- 路由仍保持 `/work-items` 主入口
- 暂时允许继续调用 `/issues` API

### 子计划 C：后端 API alias 线

文件：

- `docs/plan/2026-03-13-workitem-api-alias-subplan.md`

负责范围：

- `internal/adapters/http`
- `web/src/lib/apiClient.ts`
- 相关 HTTP / client 测试

任务目标：

- 增加 `/api/work-items/*` alias
- 复用现有 `/issues/*` 逻辑
- 前端 client 切主路由
- `/api/issues/*` 保留兼容

### 子计划 D：内部命名清理线

文件：

- `docs/plan/2026-03-13-workitem-internal-cleanup-subplan.md`

负责范围：

- `internal/core`
- `internal/application/flow`
- 非 public surface 的误导性命名

任务目标：

- 清理高误导 exported symbol
- 逐步减少 `Flow*` 和强语义 `Issue*` 对认知的污染
- 不把数据库表 rename 绑在第一波执行里

## 推荐执行顺序

### 第一批

- 子计划 A：文档与规则线
- 子计划 B：前端收口线
- 子计划 C：后端 API alias 线

### 第二批

- 子计划 D：内部命名清理线

## 每条线的硬边界

### A 线不得做

- 不改业务代码
- 不改 API 契约实现

### B 线不得做

- 不改后端路由注册
- 不改 `internal/*`
- 不擅自切换 `/issues` 到 `/work-items` API，如果 C 线尚未完成

### C 线不得做

- 不改数据库表名
- 不重命名核心 `Issue` 模型
- 不做 payload 字段全面 rename

### D 线不得做

- 不删除兼容路由
- 不做数据库 schema rename，除非另行批准

## 验收标准

### 第一阶段验收

- 文档规则冻结
- 前端主语义统一为 `Work Item`
- 后端已提供 `/api/work-items/*` alias
- 前端 client 默认走 `/api/work-items/*`
- `/api/issues/*` 仍可工作

### 最终验收

- 新功能、新文档、新接口不再扩散 `Flow`
- `Work Item` 成为统一对外语义
- `Flow` 只留必要兼容层

## 建议的新会话启动方式

如果要在新会话中直接执行，请明确指定子计划文件，而不是只说“按总计划做”。

推荐示例：

```text
请按 docs/plan/2026-03-13-workitem-frontend-subplan.md 执行。
只做该文件规定范围内的改动，不要越界到后端和数据库层。
```

```text
请按 docs/plan/2026-03-13-workitem-api-alias-subplan.md 执行。
本次只完成 /api/work-items alias，不做内部 Issue 模型重命名。
```

## 备注

如果后续需要进一步拆分，可继续把子计划 B 和 C 再细分成多个子文件，但目前这一层已经足够支持并行推进。
