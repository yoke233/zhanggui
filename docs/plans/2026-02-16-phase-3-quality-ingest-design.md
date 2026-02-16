# Phase 3 Quality Ingest Design

日期：2026-02-16  
范围：Phase 3 第一批（本地可跑，sqlite outbox）

## 目标

- 将外部质量事件（review/ci）映射为统一质量语义并写回 Issue。
- 质量失败自动路由到责任角色（通过结构化 `Next`）。
- 事件具备幂等去重、可回放、可审计。
- 保守模式：不自动推进 `state:*`、不改 assignee、不自动 close。

## 约束

- 仅实现 sqlite outbox 路径，不接 GitHub/GitLab API。
- 不改变既有 Issue 协议字段；新增能力通过结构化 comment + 审计表实现。
- 与 Phase 2.8 模型兼容（subscriber/comment-only + single writer）。

## 方案

### 1. 数据层

新增表 `quality_events`：

- `idempotency_key`：唯一键（去重真源）
- `issue_id`：关联 Issue
- `source` / `external_event_id`：来源追踪
- `category` / `result`：统一质量语义输入
- `actor` / `summary` / `evidence_json` / `payload_json`：审计与回放
- `ingested_at`：落盘时间

### 2. 用例层

新增 `IngestQualityEvent`：

- 校验事件合法性（`review` + `approved/changes_requested`；`ci` + `pass/fail`）
- 失败事件强制 evidence（满足 FR-3-02）
- 生成幂等键（用户指定或系统派生）
- 先写审计表，重复键直接返回 duplicate（不重复写 comment）
- 事件映射为结构化 comment（`Summary` marker + `Tests.Evidence` + `Next`）
- 路由规则：
  - `review:changes_requested` / `ci:fail` -> `@<责任角色>`（基于 routing labels 推断）
  - 通过 -> `@integrator` 收敛

### 3. CLI

在 `outbox` 下新增质量子命令：

- `outbox quality ingest`：导入质量事件
- `outbox quality list`：按 Issue 查看审计记录

## 与 PRD 对齐

- FR-3-01：通过 `category/result -> marker/result_code` 映射实现。
- FR-3-02：失败事件要求 evidence 并写入结构化 comment。
- FR-3-03：失败事件自动生成责任角色 `Next`。
- FR-3-04：本批不自动推进状态，保留 integrator 人工收敛（符合保守模式）。
- FR-3-05：`quality_events` + 幂等键 + IssueRef 关联满足回放与审计。

## 非目标（本批）

- 不接 forge webhook / API。
- 不自动合并、不开关审批策略。
- 不自动维护 `review:*` / `qa:*` 标签，仅输出建议语义与证据。
