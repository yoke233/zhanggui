# 命名与标识规范

版本：v1.0  
状态：Draft  
Owner：Architect / PM

## 1. 目标

统一“对人字段”和“对机字段”的命名风格，减少 `IssueRef` 与 `run_id` 混用带来的理解成本。

## 2. 基本规则

- 文档模板（人读）字段：`PascalCase`
- 协议/接口（机读）字段：`snake_case`
- 协作主键统一语义：`IssueRef` / `issue_ref`
- 执行主键统一语义：`RunId` / `run_id`

说明：

- `IssueRef` 与 `issue_ref` 是同一语义的两种表示。
- `RunId` 与 `run_id` 是同一语义的两种表示。
- 禁止再引入 `outbox_issue`、`outbox_thread` 这类对象名。

## 3. 显式映射表

- `IssueRef` <-> `issue_ref`
- `RunId` <-> `run_id`
- `SpecRef` <-> `spec_ref`
- `ContractsRef` <-> `contracts_ref`
- `ReadUpTo` <-> `read_up_to`
- `BlockedBy` <-> `blocked_by`

## 4. IssueRef canonical 格式

- GitHub：`<owner>/<repo>#<number>`
- GitLab：`<group>/<project>#<iid>`
- SQLite：`local#<issue_id>`

禁止：

- 用 GitHub/GitLab 的内部 `id`/`node_id` 作为 `IssueRef`

## 5. RunId 格式

推荐格式：

- `<YYYY-MM-DD>-<role>-<seq>`
- 示例：`2026-02-14-backend-0001`

约束：

- 同一个 `IssueRef` 同时只能有一个 `active_run_id`
- 迟到结果若 `run_id != active_run_id` 必须丢弃

## 6. 过渡策略（解决写法别扭）

当前（V1）：

- 模板里已广泛使用 `IssueRef`
- 执行协议里已广泛使用 `run_id`

下一步（V1.1+，可选）：

- 在模板中增加 `RunId` 字段（可选），降低“大小写风格不一致”的体感。
- 解析层同时接受 `RunId` 与 `run_id`，写回时按“模板 PascalCase / 协议 snake_case”输出。

最终目标：

- 人读与机读风格各自一致，不再让同一层级出现混搭。

## 7. 验收检查

- 新增模板字段必须先在本文件登记映射关系。
- 新增协议字段必须定义 canonical 格式与校验规则。
- PR 审查时若出现未登记命名，要求补标准或改名。
