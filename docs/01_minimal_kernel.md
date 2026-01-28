# 01 最小内核（Minimal Kernel）与渐进式交付（Progressive Delivery）

## 1) Master IR：最小 Core（建议 8 个）
Core 只保证“系统能跑、能追溯、能一致”，不限制每个需求的大纲/内容形态。

- goal：1~3 句目标
- constraints：硬约束（受众/语气/长度/禁止项/期限…）
- deliverables：交付物清单（type + endpoints + priority + notes）
- outline：动态大纲（自由树结构，节点最少 id/title/children）
- key_points：关键要点（可空）
- risks：风险/不确定点（可空）
- sources：引用索引（可空）
- open_questions：待用户补充（可空）

## 2) extensions：可扩展命名空间（避免改 Core）
- extensions 是字典：key=namespace，value=任意结构（JSON/YAML）。
- 建议命名：artifact:ppt / artifact:report / domain:legal / org:xxx 等。
- 只有对应插件理解其结构；调度系统不依赖内部字段。

## 3) 动态验收（Must-answer Questions）
每次任务由 Planner 生成临时验收清单：
- acceptance.must_answer[]
- acceptance.must_not[]
- acceptance.format_rules[]
Verifier 只针对本次 acceptance 校验，避免硬模板化。

## 4) 渐进式交付（把“渐进加载”用到产物）
组员交付分三层：默认只交 Summary，按需再交 Cards/Full。

- Summary（必交）：150~300 字 + 要点 + 覆盖映射
- Cards（按需）：可被组长直接合并成 IR 的卡片集合
- Full（少用）：只有需要引用/争议/细节时才读取

### 为什么这能解决组长 token 爆炸
- 组长默认只读 Summary（快筛 + 决策）
- 需要合并才要 Cards（结构化、短、可 rg）
- 极少读 Full（降低跑偏概率）

## 5) 轻量可检索（rg 优先）
在 Summary/Cards 中固定少量可筛字段（front-matter 或固定行）：
- assigned_outline_nodes: [...]
- assigned_must_answer: [...]
- tags: [...]
- confidence: 0.xx
- reuse: [report,ppt,...]
这让组长用 rg 快速定位可用内容，而不扫全文。


## 6) 位置锚点 DSL（Markdown 内嵌 HTML Anchor，区块前置）
目标：稳定“跨文件/同文件跳转”，且携带可追溯 meta；锚点与 meta **由生成器自动写入**，Agent 不参与。

**区块前置模板：**
```md
<a id="block-deliver-report-2"></a>
<!--meta task=task-000123@r2 sources=task-000120@r1,task-000121@r1-->

## 2. 并行与资源池
...正文...
```

**跳转引用（跨文件也稳定）：**
```md
见：[报告第2章](deliver/report.md#block-deliver-report-2)
```

约定：
- `id` 命名：`block-<scope>-<deliverable>-<node>`（例：`block-deliver-report-2`、`block-deliver-ppt-s05`）
- `id` 不带版本号（位置稳定）；版本/来源写在 `<!--meta ...-->` 中（可变、可追溯）
- 组长阅读时，anchor 与注释默认不显示；需要追溯时可用 rg 查 `meta task=...`。


## Gate Node（收敛门）

为避免把整个任务流“会议化”导致吞吐下降，本体系将会议抽象为可插拔节点：**Gate Node**。
- 默认流水线：Work（并行产出）→ Merge（合并）→ Verify（校验）
- 仅在分歧/高风险/用户打回等场景触发 Gate，用 1~2 轮完成裁决，然后回到并行生产。

详见：`docs/07_convergence_gates.md`。
