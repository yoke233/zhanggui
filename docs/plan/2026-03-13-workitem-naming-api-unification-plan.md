# WorkItem 命名与 API 契约统一计划

> 状态：待执行
> 创建日期：2026-03-13
> 关联规范：
> - `docs/spec/naming-transition-thread-workitem.zh-CN.md`
> - `docs/spec/design-issue-centric-model.md`
> - `docs/spec/thread-workitem-migration-guide.zh-CN.md`

## 背景

当前系统已经基本形成 issue-centric 主线，但仓库里仍同时存在多套命名和入口：

- 前端主入口使用 `/work-items`
- 后端主 REST 仍使用 `/issues`
- 前端兼容保留 `/flows` redirect
- 内部仍存在 `FlowScheduler`、`PRFlowPrompts`、`flow_*` 等历史命名

这会带来以下问题：

- 新功能开发时容易混淆“用户语义”和“内部实现语义”
- 文档容易把“现状”和“未来设计”写混
- 前后端、测试、集成脚本的命名成本持续增加
- `Flow` 历史命名仍在污染新的 public surface

本计划用于推动一次有边界的全量收口。

## 目标

本计划的目标是：

1. 对外产品语义统一为 `Work Item`
2. Public REST API 统一收口到 `/api/work-items/*`
3. 明确 `Issue` 是短中期内部实现名，避免仓库短期失稳
4. 将 `Flow` 限定为历史兼容层或纯执行流程术语
5. 建立文档状态管理规则，避免 spec 再次漂移

非目标：

- 不在第一阶段强制重命名数据库表
- 不在第一阶段强制把 `Step -> Action`、`Execution -> Run`、`Artifact -> Deliverable` 推成主 API 名
- 不把 Thread 和 ChatSession 合并成同一个概念

## 统一规则

### 命名规则

- 产品/UI：统一使用 `Work Item`
- Public REST：统一以 `/api/work-items/*` 为目标主契约
- 内部领域模型：短期保留 `Issue`
- 执行流程语义：使用 `workflow` 或 `execution pipeline`
- `Flow`：仅保留为兼容层/历史术语，不得继续扩散

### 迁移纪律

- 禁止新增 `/api/flows/*`
- 禁止新增 `Flow*` 页面、组件、公开类型名
- 新功能默认不得再把 `Issue` 暴露为用户可见主语义
- `/api/issues/*` 在兼容期内保留，但不再承载新的命名方向

## 总体策略

采用“先外后内、先契约后实现、先兼容后删除”的迁移顺序：

1. 先冻结规则
2. 再统一 public surface
3. 再收口前端和后端主实现命名
4. 最后再评估内部核心模型和持久层是否需要彻底改名

## 分阶段计划

## Phase 0：冻结规则与边界

### 目标

先把“应该改到哪里”定清楚，避免边改边摇摆。

### 范围

- 术语表
- Public API 目标路径
- `Issue` / `WorkItem` / `Flow` 的边界定义
- spec 文档状态规范

### 主要任务

- 更新 `docs/spec/naming-transition-thread-workitem.zh-CN.md`
- 统一相关 spec 的状态头
- 输出迁移边界说明和禁止新增项

### 交付物

- 正式命名策略 spec
- 本计划文档

### 完成标准

- 团队内对以下结论没有歧义：
  - 用户看到的是 `Work Item`
  - Public API 目标是 `/api/work-items/*`
  - 内部短期仍可保留 `Issue`
  - `Flow` 不再作为主业务对象名新增

### 风险

- 规则不够硬，后续执行时继续回摆

## Phase 1：统一 Public Surface

### 目标

先把用户、文档、前端路由、外部 API 设计语言统一。

### 范围

- 前端页面文案
- 导航与标题
- API 路径设计
- 外部文档与示例

### 主要任务

- 确认 `/work-items` 为唯一主页面入口
- 为后端设计 `/api/work-items/*` 主路由
- 将 `/api/issues/*` 定义为兼容 alias
- 清理新文档中的 `Flow` 主对象表述

### 交付物

- `/api/work-items/*` 路由设计清单
- 前端/文档中的统一术语

### 完成标准

- 新文档、新页面、新 API 设计不再把主对象称为 `Issue` 或 `Flow`
- `/flows` 不再出现在任何新 public design 中

### 风险

- 只改表面，不改实际 client / handler，会形成“看起来统一、实现仍混乱”的中间态

## Phase 2：前端内部收口

### 目标

让前端仓库内部命名与对外语义一致。

### 范围

- 页面组件
- hooks / store / route params
- i18n
- 前端类型与 API client 命名

### 主要任务

- `CreateFlowPage` -> `CreateWorkItemPage`
- `FlowsPage` -> `WorkItemsPage`
- `FlowDetailPage` -> `WorkItemDetailPage`
- 清理仍引用 `/flows/:id/...` 的 i18n 文案
- API client 主方法切到 `workItems`

### 交付物

- 前端页面与组件命名统一
- 前端 API 调用主语义统一

### 完成标准

- 前端主路径、主组件、主变量名统一使用 `workItem`
- 前端仓库内不再新增 `Flow*` 页面/组件名

### 风险

- 改名范围大，容易影响路由、测试快照和 import 链

## Phase 3：后端 API 与 handler 收口

### 目标

把后端对外接口和 handler 层命名统一到 `work-items`。

### 范围

- HTTP 路由注册
- handler 命名
- request / response DTO
- OpenAPI / API client 对齐

### 主要任务

- 增加 `/api/work-items/*` 路由
- 复用现有 `/issues/*` 处理逻辑
- 逐步将 handler 名从 `*Issue*` 收口到 `*WorkItem*`
- 为 `/api/issues/*` 保留兼容入口

### 交付物

- 新主路由 `/api/work-items/*`
- 兼容路由 `/api/issues/*`

### 完成标准

- 前端默认调用 `/api/work-items/*`
- `/api/issues/*` 可继续工作，但已降级为兼容入口

### 风险

- 双路由共存阶段容易产生测试重复和行为漂移

## Phase 4：内部领域与服务层渐进改名

### 目标

逐步减少 `Issue` 对新开发者的认知负担，但不以破坏稳定为代价。

### 范围

- core/service/exported symbols
- scheduler / engine / prompts
- 日志、事件、metrics

### 主要任务

- 评估 `IssueEngine`、`IssueScheduler` 是否改名
- 优先替换误导性最强的 exported symbol
- `PRFlowPrompts`、`FlowScheduler` 等名称逐步收口

### 交付物

- 一批新的中性命名 exported symbol
- 旧 symbol 的兼容封装或迁移层

### 完成标准

- 新主 service / exported API 不再以 `Flow` 命名
- 内部是否继续保留 `Issue` 成为明确可接受的过渡状态

### 风险

- 如果一步改太深，容易连锁影响 engine、scheduler、测试和集成逻辑

## Phase 5：Flow 兼容层清理

### 目标

把历史 `Flow` 语义从 public surface 和高误导位置移走。

### 范围

- 兼容 redirect
- 历史页面名/文件名
- 历史文案
- 历史 spec

### 主要任务

- 清理高误导 `flow_*` 文件名和文案
- 降低 `/flows` redirect 的可见性
- 移除仍把 `Flow` 当主对象的 spec 叙述

### 交付物

- `Flow` 仅剩必要兼容层

### 完成标准

- 新开发者从主路径已看不到 `Flow` 是“主业务对象”的错觉

### 风险

- 某些历史测试、脚本、书签仍依赖旧路由和旧名称

## Phase 6：删除兼容层

### 目标

在兼容周期结束后，去掉旧世界入口。

### 范围

- `/api/issues/*`
- `/flows` redirect
- 历史 alias
- 旧文档入口

### 前提条件

- 前端和脚本都已迁移到 `/api/work-items/*`
- 文档和测试已完全切换
- 至少经历一轮稳定发布

### 主要任务

- 移除旧路由
- 移除 deprecated wrapper
- 清理历史 spec 和迁移说明

### 完成标准

- 对外只剩 `Work Item` 与 `/api/work-items/*`
- `Flow` 兼容层退出主线

### 风险

- 如果生态中仍有外部调用依赖 `/issues/*`，会造成破坏性变更

## 风险清单

### 高风险

- 同时改路由、handler、DTO、前端 client、测试，导致跨层回归
- 如果进一步改数据库表名，迁移复杂度显著上升

### 中风险

- 双路由兼容期里出现行为不一致
- 前端 import/组件改名导致隐藏编译错误

### 低风险

- 文档状态头与术语修订
- i18n 文案收口

## 验证策略

每一阶段结束后至少验证以下内容：

- 前端路由行为正确
- API client 命中预期路径
- 关键 CRUD / run / cancel / step / thread linking 正常
- 兼容路径在过渡期仍可工作
- spec 与代码现状一致

建议验证命令按改动面执行：

- 前端：`npm --prefix web test`、`npm --prefix web run typecheck`
- 后端：`go test ./internal/adapters/http/...`、`go test ./internal/application/flow/...`
- 跨层：针对 thread/work-item 路由和 issue/work-item alias 增补集成测试

## 建议的第一批实施范围

如果现在开始第一波开发，建议只做以下内容：

1. 完成 Phase 0
2. 完成 Phase 1 的契约冻结
3. 开始 Phase 2 的前端去 `Flow`
4. 开始 Phase 3 的 `/api/work-items` alias

暂缓：

- 数据库表 rename
- `Issue` 全量重命名为 `WorkItem`
- payload 字段全面从 `issue_*` 切到 `work_item_*`
- 错误码体系全量替换

## 一句话结论

这次迁移的正确路径不是“一次性把所有旧名删光”，而是：

**先统一对外语义，再统一 API 契约，再渐进清理内部实现，最后删除兼容层。**
