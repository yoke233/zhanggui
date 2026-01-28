# 04 纸面演练：报告（report）+ PPT（ppt）

> 目的：验证“模块并行 + 渐进式交付 + 强协议流水线”能不落库跑通。

## 0) 假设需求
- 交付：管理层评审用《多 Agent 协作系统》报告 + 10 页 PPT
- 强调：可扩展、可治理、可追溯；不过度学术
- 禁止：承诺“完全自动化无需人工”

## 1) Planner 生成 must-answer（示例 10 条）
1) 为什么多 agent？
2) 并行单位与防爆炸？
3) 强协议 JSON→HTML 谁负责？如何防渲染器改语义？
4) 多交付物如何口径一致？
5) 合并如何防 token 爆炸与截断？
6) 失败如何恢复/降级？
7) 如何扩展新交付物/新渲染器？
8) 权限与审计怎么做？
9) MVP 最小闭环有哪些？
10) 最大风险与兜底？

## 2) MPU 拆分（并行执行）
每个 MPU 输出一个模块文件（无冲突）：
- report 模块：按章（02~08）
- ppt 模块：按页（slide_1~slide_10）
- quality 模块：覆盖度/一致性/损失检查（可并行）

## 3) 渐进式交付
默认每个 MPU 只交：
- summary.md（含 assigned_must_answer / assigned_outline_nodes（由程序下发））
组长只在需要合并时再要：
- cards.md（结构化卡片）

## 4) 组长合并策略（最小上下文）
输入只包含：
- Master IR（goal/constraints/outline/must-answer）
- summaries（或 cards）
输出：
- report_final.md
- ppt_ir.json

## 5) 强协议链
- ppt_ir.json → (Adapter) ppt_renderer_input.json → (Renderer) slides.html
若 adapter 发现无法压缩/缺字段：
- 产出 issue_list，交主编裁决（拆页/降级/回问）

## 6) rg 快筛建议
- 找覆盖 must-answer=3 的模块：rg "assigned_must_answer: .*3" teams/**/summary.md
- 找低置信度回炉：rg "agent_confidence（可选）: 0\.[0-5]" teams/**/summary.md
- 找可复用到 PPT 的句子：rg "reuse: .*ppt" teams/**/cards.md

## 7) 自动锚点（Markdown 内嵌 HTML Anchor）与追溯
合并器生成 `deliver/report.md` 时，对每个章节区块自动插入：

```md
<a id="block-deliver-report-2"></a>
<!--meta task=task-000123@r2 sources=task-000123@r2-->
```

之后任何地方都可稳定引用（跨文件）：
- `见：[第2章](deliver/report.md#block-deliver-report-2)`

注：锚点与 meta 由生成器写入，Agent 不参与；阅读视图可默认不显示注释。

