# Usecase Layer

该目录用于放置应用用例编排逻辑（输入输出 DTO、事务边界、调用领域接口）。

当前阶段仅完成项目初始化，不包含具体业务用例实现。

## 约定

- 用例入口函数统一采用 `func (u *UseCase) Handle(ctx context.Context, ...)` 风格。
- 所有对外接口与下游依赖调用必须传递同一个 `ctx`，禁止在链路中重新创建 `context.Background()`。
- 结构化日志字段（如 `issue_ref`、`run_id`、`trace_id`、`span_id`）通过 `ctx` 传递，保证日志和链路追踪可关联。
