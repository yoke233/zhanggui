# Meeting Mode（会议模式：加速收敛，不制造第二真源）

目标：在“需求刚来、信息不全、角色多、分歧大”的情况下，用最小同步成本收敛到可执行结论。

核心原则：

- 单一真源：会议材料与结论必须落到同一个 `IssueRef`（Issue Spec 区块或 `SpecRef` 指向的 `spec.md`）。
- 并行输入：允许多角色异步并行提交材料与观点，不要求先对齐再提交。
- 单写者收敛：最终会议结论只能由一个角色写回（推荐 `PM/Recorder`），避免多份“会议纪要”分叉。
- 同步只处理冲突点：能异步解决的问题不占用会议时间。

本模式复用既有结构（不新增新对象）：

- 协作真源：Issue（Outbox）
- 交付真源：PR/commit + 证据
- 配置真源：`<outbox_repo>/workflow.toml`
- 结构化写回：`docs/workflow/templates/comment.md`

## 1) 什么时候进入会议模式

满足任意条件建议进入会议模式（否则优先异步评审）：

- 涉及 3 个以上角色（需求方/PM/设计/交互/架构/研发/QA）且存在冲突点
- 验收口径/数据口径无法在异步中 1 轮内说清
- 存在重大风险：breaking contracts、合规/安全、迁移/回滚、跨仓依赖
- 时效强：需要当天定方向才能继续推进

会议模式不等于“全员大会”。它强调：在最少人参与下做出必要决策。

## 2) 会议类型（轻协议，而不是统一模板）

把会议按“目的”分型，避免所有会议都变慢：

- `Triage`（10-15m）：只决定“要不要做/谁决策/下一步怎么走”
- `Decision`（15-30m）：只解决 1-3 个必须当场决定的点
- `Critique`（30-45m）：设计/交互评审，只产出修改建议与风险
- `Working Session`（60-90m）：少数人动手产出草案（spec/原型/接口），不是对齐会

每个会议只做一种类型，不混用。

## 3) 会前材料：谁来 summary、谁必须预读

### 3.1 材料的真源位置

会议材料不要求都写成规范文档，但必须可追溯并能被链接：

- 需求材料：Issue Spec 区块或 `SpecRef` 指向的 `spec.md`
- 设计材料：链接到原型/图片/文件（在 Issue 中贴链接或附件）
- 架构材料：最多 2 个方案的要点与 tradeoff（写在 Issue comment 或 ADR 草案链接）

禁止：同一类材料同时在多个地方维护不同版本。

### 3.2 Summary 责任人（必须明确）

建议固定一个 `PreRead Summarizer`（会前摘要人）：

- 默认：`PM`（需求会）或 `Recorder`
- 备选：`Lead`（技术会）

职责：

- 在会前把“材料清单 + 冲突点 + 需要决策的问题”写成一条结构化 comment（见 5.1）。
- 不是重写材料，而是做索引与冲突聚合。

### 3.3 参会人是否必须预读

规则建议按角色分层：

- `Decider`（做最终决策的人）：必须预读
- `Presenter`（要在会上讲的人）：必须预读 + 提前准备
- `Contributor`（提供专业输入的人，如架构/交互）：必须预读与准备 1 页以内立场
- `Observer`：不强制预读，尽量不进会；需要进会则只听不占用决策时间

务实建议：

- 允许“未预读不能发言影响决策”（由主持人控场），否则会议一定变慢。

## 4) 并行沟通：如何在开会前先把输入并行跑起来

### 4.1 并行输入的做法（默认）

在同一个 `IssueRef` 下，按泳道并行追加 comment：

- `lane:需求`（BA/需求方）
- `lane:产品`（PM）
- `lane:设计/交互`（设计师/交互师）
- `lane:架构`（架构师）

每个 lane 只需要回答本专业最关键的 3-5 条，不求完美。

### 4.2 冲突升级规则（什么时候必须同步）

当出现以下情况之一，升级到 `Decision` 会议：

- 需求 AC 冲突（两种口径无法共存）
- 设计交互与技术约束冲突（不可实现或成本不可接受）
- 数据/指标口径冲突（上线后无法验收）
- breaking change 是否允许无法异步达成一致

## 5) 会议的最小输出（不要死模板）

### 5.1 会前：PreRead Summary（必须有）

由 `PreRead Summarizer` 在 Issue 下写一条 comment，最小包含：

- `材料清单`：链接列表（SpecRef/原型/方案）
- `冲突点`：1-5 条（会中必须解决）
- `待决策点`：1-3 条（明确谁是 Decider）
- `建议默认选项`：没有强反对就按默认通过（加速）

建议用 `docs/workflow/templates/comment.md`，并约定：

- `Action: proposal`
- `Status: todo`
- `Trigger: meeting:preread:<YYYY-MM-DD>`

### 5.2 会中：时间盒与决策点限制（强烈建议）

- 每次会议最多处理 3 个决策点
- 超出则进入 Parking Lot，不在会内硬聊

### 5.3 会后：Meeting Summary（必须有）

由 `PM/Recorder` 单写者写回同一个 Issue：

- `Decisions`：今天生效的决定（逐条）
- `Next`：下一步（Owner + 截止时间 + 产物）
- `OpenQuestions`：未决事项（Owner + 截止时间 + 触发条件）

建议同样使用 comment 模板，并约定：

- `Action: accept|proposal|update`（视是否盖章）
- `Trigger: meeting:summary:<YYYY-MM-DD>`

## 6) 与审批/盖章的关系（避免“讨论当决定”）

会议总结不等于“已决定”。生效规则：

- 需要盖章的事项（接口、验收口径、breaking、合并顺序等）：
  - 会后必须由 Approver 执行 `/accept` 并落盘 `decision:accepted`
- 未盖章前只视为 `decision:proposed`，实现不得据此大改

审批策略见：

- `docs/workflow/approval-policy.md`

## 7) 进入交付层后的并行（PR 来了怎么并行审查）

会议模式解决“做什么/怎么验收/关键决策”。进入交付后：

- QA 与 Reviewer 可以并行
- 若都打回，必须由 `lead-integrator` 汇总为唯一 `FixList vN` 再派回（避免多头指挥）

护栏细节见：

- `docs/workflow/guardrails.md`
