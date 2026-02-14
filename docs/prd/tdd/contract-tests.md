# 协议契约测试规格（Contract Tests）

版本：v1.0  
状态：Draft  
负责人：Architect / Lead / QA  
目标：冻结关键字段语义，防止实现漂移

## 1. 覆盖对象

- `IssueRef`（协作主键）
- `run_id`（执行尝试主键）
- `assignee`（claim 真源）
- `issue_ref`（WorkOrder/WorkResult 契约字段）
- `result_code`（失败原因码）

## 2. IssueRef 契约

### [CT-REF-001] GitHub IssueRef 格式校验

输入：

- `owner/repo#123`

期望：

- 校验通过
- 可解析 `owner`、`repo`、`number`

### [CT-REF-002] GitLab IssueRef 格式校验

输入：

- `group/project#456`

期望：

- 校验通过
- 可解析 `group`、`project`、`iid`

### [CT-REF-003] SQLite IssueRef 格式校验

输入：

- `local#12`

期望：

- 校验通过
- 可解析 `issue_id=12`

### [CT-REF-004] 禁止平台内部 ID 充当 IssueRef

输入：

- GitHub REST `id`
- GraphQL `node_id`
- GitLab 全局 `id`

期望：

- 校验失败
- 返回明确错误信息

## 3. run_id 契约

### [CT-RUN-001] run_id 格式

输入：

- `2026-02-14-backend-0001`

期望：

- 校验通过
- 可解析 date/role/seq

### [CT-RUN-002] 同一 Issue 只允许一个 active_run_id

Given:

- 当前 active run = `...0002`

When:

- 收到 `...0001` 结果

Then:

- 视为 stale run
- 不得覆盖当前状态

## 4. assignee 契约

### [CT-CLAIM-001] Claim 真源

Given:

- 有 `/claim` 文本
- 但 assignee 未设置

Then:

- claim 视为失败

### [CT-CLAIM-002] assignee 设置成功即 claim 生效

Given:

- assignee 已设置

Then:

- claim 生效
- 允许进入开工判断

## 5. WorkOrder/WorkResult 契约

### [CT-WORK-001] WorkOrder 必填字段

必须包含：

- `issue_ref`
- `run_id`
- `role`
- `repo_dir`

任一缺失：

- 校验失败

### [CT-WORK-002] WorkResult 回显字段

必须回显：

- `issue_ref`（与 WorkOrder 一致）
- `run_id`（与 WorkOrder 一致）

任一不一致：

- 结果标记无效，不自动写回

### [CT-WORK-003] Changes 与 Tests 下限

要求：

- Changes 至少有 PR 或 commit 一个
- Tests 必须存在（可为 `n/a`，但必须显式）

缺失任一：

- 不允许进入 done/close

## 6. result_code 契约

### [CT-CODE-001] 枚举内通过

输入：

- `dep_unresolved` / `test_failed` / `stale_run` 等

期望：

- 校验通过

### [CT-CODE-002] 枚举外拒绝

输入：

- 任意未定义字符串

期望：

- 校验失败
- 返回可选枚举列表

## 7. 通过标准

- 上述所有契约测试通过。
- 任一关键字段漂移都能在测试层暴露。
