# PRD - Phase 3 质量与 PR/CI 自动化增强

版本：v1.0  
状态：Draft  
负责人：PM / Integrator / QA / Platform  
目标阶段：Phase 3

## 1. 背景与问题

Phase 2 已完成“可运行自动化”，但质量信号仍大量依赖人工搬运。  
Phase 3 的目标是在不改协议的前提下，自动接入 PR/CI/review 结果，提升质量闸门的实时性与可计算性。

## 2. 目标与非目标

目标：

- 自动读取 review 结论与 CI 状态。
- 自动写回 Issue 的质量证据并触发路由。
- 将 changes_requested / CI fail 自动回流到责任角色。

非目标：

- 不重写协作协议字段。
- 不做全链路发布平台替代。
- 不在本阶段扩展复杂审批模式（`all/quorum/staged` 可后续演进）。

## 3. 用户与场景

用户：

- PM：关注阶段质量达成率与返工成本。
- Reviewer/QA：关注判定是否被准确执行。
- Integrator：关注合并门槛和失败回流效率。
- Lead/Worker：关注失败定位与重试成本。

场景：

- 多 PR 并行，手工同步质量状态易滞后。
- 需要统一质量证据口径用于审计与复盘。

## 4. 范围（In Scope）

- 自动读取 PR review（approved/changes_requested）。
- 自动读取 CI checks（pass/fail）。
- 自动生成结构化质量回填 comment。
- 自动回流路由（Next + 标签建议）到相关角色。
- 可选：自动生成 release note/changelog 草稿。

## 5. 功能需求（PRD 级）

- FR-3-01：系统必须将 review/CI 映射到统一质量语义。
- FR-3-02：质量失败必须自动写回 Issue，且包含证据链接。
- FR-3-03：质量失败必须自动路由到责任角色。
- FR-3-04：质量通过后支持自动推进到可合并状态（不越过审批策略）。
- FR-3-05：质量事件必须可回放、可审计、可追溯到 IssueRef。

## 6. 验收标准（DoD）

- AC-3-01：PR review 与 CI 结果可自动回填到 Issue。
- AC-3-02：`changes_requested` 能自动触发回流，不依赖人工提醒。
- AC-3-03：CI fail 能自动带证据路由到对应角色。
- AC-3-04：质量通过后，Integrator 可依据自动信号完成合并闭环。
- AC-3-05：同一事件不会重复写回造成噪声。

## 7. 成功指标

- 指标 1：质量失败发现到责任人可见的时延下降 >= 60%。
- 指标 2：因“信息未同步”导致的返工比例下降 >= 40%。
- 指标 3：关闭 Issue 的质量证据完整率达到 100%。

## 8. 风险与缓解

- 风险：不同 forge 平台事件模型差异大。  
  缓解：保持 Issue/Event 抽象层，平台差异在 adapter 内解决。

- 风险：自动路由误判责任角色。  
  缓解：保留 `needs-human` 人工接管开关与审计记录。

- 风险：自动回填噪声过高。  
  缓解：引入幂等键与去重策略，按事件级别聚合写回。

## 9. 依赖

- `docs/operating-model/quality-gate.md`
- `docs/operating-model/outbox-backends.md`
- `docs/workflow/approval-policy.md`
- `docs/workflow/label-catalog.md`
