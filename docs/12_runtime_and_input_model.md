# 12 运行时与输入模型（v1：协程 Agent + 可控暂停开关 + 输入落盘）

> 本文目标：把“并行 Agent（Go 协程）如何跑、如何暂停/变更、用户输入如何落盘与引用”先落成 **v1 可执行规范**。  
> 定位：总体性内容；会议（Meeting Mode）与具体任务流水线可后延，但需要共享同一套最小运行信息与输入协议。  
> 兼容性：对外事件承载采用 AG-UI（见 `docs/11_ag_ui_integration.md`）；系统内部真相源以文件系统约定为准（见 `FILE_STRUCTURE.md` 与 Bundle/ledger/evidence 规范）。本文聚焦 **运行时机制** 与 **落盘约束**。

---

## 0) 关键决策（v1 写死，后续可在变更记录中调整）

1) **Agent 在 Go 内部用协程（goroutine）表达**：v1 不引入“worker 间协议/网络通信”。  
2) **暂停/用户决策默认采用“可控开关：允许当前 step 收尾 → 统一中断（interrupt）→ 下次 resume 继续”**。  
3) **会议与任务先不强行收敛成同一种 Task**：可以两条线推进，但必须共享“Thread/Run/Artifact(Inputs)/Control”这套公共基座（见 §1）。  
4) **命名口径**：本文中的 Thread/Run/Inputs/ChangeSet 等概念，以 `FILE_STRUCTURE.md` 的目录与文件名为权威，避免未来实现演进导致语义漂移。

> 说明：以上三条是为了降低 v1 复杂度，同时保持可追溯与可演进。

---

## 1) 公共基座：Thread / Run / Workflow / Agent

### 1.1 Thread（线程/会话：公共铺位）
**Thread 是用户“持续对话/持续推进”的最小单位**。它是跨 run 的稳定容器，用来承载：
- 可恢复的公共状态（例如：当前 spec 版本号、已确认的约束、已接收的附件清单、未决变更）
- 控制信号（pause/resume/cancel 的“意图”与审计）

建议落盘（不入 git）：
- `fs/threads/{thread_id}/state.json`：公共状态快照（single-writer：系统）
- `fs/threads/{thread_id}/events/events.jsonl`：公共事件（append-only：系统）
- `fs/threads/{thread_id}/inputs/manifest.json`：输入清单（append-only 或 single-writer，见 §3）

> 为什么要 Thread：仅靠 `fs/runs/{run_id}` 会导致“中断恢复要遍历查找/状态分散”，且会议/任务并行时缺少统一协调点。

### 1.2 Run（一次运行：append-only 证据链）
Run 是一次执行实例（一次 `/run` 调用），主要用于：
- 输出对外事件流（AG-UI SSE）
- 记录本次 run 的状态与证据（run.json/state.json/events.jsonl）

落盘（已实现的约定见 `FILE_STRUCTURE.md`）：
- `fs/runs/{run_id}/run.json`
- `fs/runs/{run_id}/state.json`
- `fs/runs/{run_id}/events/events.jsonl`

### 1.3 Workflow（工作流/模式）
同一套运行基座支持多个 workflow，例如：
- `task`：执行任务、产物落盘、verify/pack…
- `meeting`：会议（可后延实现；见 `docs/06_meeting_mode.md`）

约束（v1 建议）：
- **同一 thread 同时只允许 1 个 active run**（避免 UI 对接/暂停广播复杂化）。  
  若未来要支持同 thread 多 run 并行，必须先补齐 Control 广播与冲突处理（见 §2.4）。

### 1.4 Agent（协程：内部并行单元）
v1 的 Agent 是运行时内部概念：每个 Agent 代表一个角色/能力（writer/coder/security/recorder…），以 goroutine 并行执行。

硬约束：
- **所有落盘写入必须走 Tool Gateway**（见 `docs/10_tool_gateway_acl.md`）
- Agent 不得绕过 Coordinator 直接写共享区（单写者/append-only 文件除外）

---

## 2) 可控暂停开关：允许当前 step 收尾 → 统一中断 → 下次继续

### 2.1 为什么选“收尾后暂停”而不是立刻杀死
用户输入“新需求/新文件/新方向”时，强行立即中止会带来：
- 产物半写入、审计不完整
- step 内部资源未释放（文件句柄/临时文件/锁）

因此 v1 默认：**先请求暂停（pause_requested），允许当前 step 收尾，在 step 边界进入一致状态后再中断**。

### 2.2 运行时状态机（v1 最小）
Thread 级（公共）控制意图：
- `RUNNING`：正常推进
- `PAUSE_REQUESTED`：已收到用户变更/暂停意图，等待各协程到达 safe point
- `PAUSED`：已完成收尾并进入暂停点（会对外呈现为 run interrupt）

Run 级（对外）呈现：
- 在到达暂停点时，当前 run 用 `RUN_FINISHED outcome=interrupt` 结束（AG-UI 机制）
- 下一次 `/run` 带 `resume.interruptId`（以及用户决策 payload）继续

### 2.3 “暂停”如何通知到所有 Agent（v1：不需要广播协议）
因为 Agent 在同一进程内：
- Coordinator 持有一个 **共享控制对象**（例如：`control`，内部用 channel/cond）
- 每个 Agent 在 **step 边界** 或 **可中断点**调用 `control.Checkpoint(step)`：
  - 若处于 `RUNNING` → 继续
  - 若处于 `PAUSE_REQUESTED` → 完成本 step 收尾 → 上报 `ACK_PAUSED` → 阻塞等待 `RESUME`

> 这满足你要的：允许当前 step 收尾 + 通知所有人 + 所有人都进入一致暂停点。

### 2.4 未来扩展端口（先定义，不要求 v1 实现）
若未来要支持同 thread 多 run 并行（例如任务执行与会议 UI 同时开），需要补：
- Thread Control SSE/WS 订阅（threadId → subscribers fanout）
- ACK 机制与超时策略（未 ACK 的协程如何处理）
- 冲突策略（两个 run 同时改 Thread state 的合并规则）

在协议层建议使用 AG-UI 的 `CUSTOM` / `RAW` 事件承载控制面扩展，避免自造新的顶层 event type（见 `docs/11_ag_ui_integration.md`）。

---

## 3) 用户输入（文本/URL/文件/图片/PDF）如何处理（必须落盘、可引用、可控规模）

### 3.1 总原则（v1 写死）
1) **用户输入不能直接“当作上下文一段文字”就喂给系统**：必须先被归档与归一化。  
2) **二进制输入（图片/PDF/压缩包等）不得直接进入 run.json/state.json**：只能以引用（ref）出现。  
3) **输入必须有 manifest**：可追溯、可去重、可限流、可审计。

### 3.2 统一输入清单：`inputs/manifest.json`
每个 Thread 维护一个输入清单（建议落在 Thread 目录，便于跨 run 复用）：

字段建议（v1 最小）：
- `input_id`：系统生成（稳定引用）
- `kind`：`text|url|file`
- `mime`：可选（`application/pdf`、`image/png`…）
- `title`：可选（UI 提供）
- `source`：`user_upload|user_url|ui_pick_file|system_generated`
- `sha256`：文件类必须有（用于去重与校验）
- `size_bytes`：文件类必须有
- `stored_path`：相对 `fs/threads/{thread_id}/inputs/` 的路径（文件类）
- `created_at`
- `notes`：可选（例如“这份 PDF 是合同扫描件”）

> manifest 的写入策略：v1 可采用 single-writer（系统覆盖写），但必须记录变更历史（建议另有 events.jsonl 追加记录）。
>
> 落盘对齐建议（v1）：
> - `inputs/files/{sha256}`：内容寻址的 CAS（create-only），用于去重与复核。
> - `inputs/manifest.json`：输入清单快照（single-writer 或 append-only + compaction），用于 UI 展示与快速定位。
> - “用户追加需求/暂停变更”必须落盘为 ChangeSet（变更单）并写入 Thread 事件流（append-only），而不是只停留在聊天文本。

### 3.3 URL 输入（必须分两层）
URL 不能等价于内容。v1 约定分两层：
- `url_ref`：仅记录 URL + metadata（永远可追溯）
- `fetched_snapshot`：若系统确实抓取了内容，再生成快照文件（HTML/Markdown/纯文本），并写入 manifest

这样可以避免“内容漂移导致的不可复现”。

### 3.4 文件/图片/PDF（必须先入库再引用）
推荐流程（适配 Web UI 与本地 UI）：
1) UI 获取文件（upload 或 pick）后，先把文件写入 `fs/threads/{thread_id}/inputs/files/`（或由后端接收并写入）
2) 系统计算 `sha256/size/mime`，写入 `inputs/manifest.json`
3) run 只引用 `input_id`（或 `sha256`），不携带二进制

### 3.5 “用户上传很多文件/很大文件”怎么处理（v1 需要硬上限）
必须有可配置限制（建议写入 Thread state）：
- `max_files_per_thread`
- `max_total_bytes_per_thread`
- `max_file_bytes`
- `max_urls_per_thread`

处理策略（v1 建议）：
- 超限时：必须通过 `ui.confirm/ui.form` 让用户做选择（保留哪些/是否先压缩/是否只上传索引页）
- 默认不做全文 OCR/全文解析：先只入库 + 元数据；需要解析时再触发后续 step（可被中断/审批）

---

## 4) 会议与任务的关系（v1 建议：拆线推进，共用基座）

### 4.1 会议是不是特殊 task？
概念上是“特殊 workflow”，但 v1 不必强行塞进 task 的 rev/pack/verify 体系里：
- 会议更像 **控制面/裁决面**：产出 decision、action_items、patch
- 任务更像 **执行面/产物面**：按 TaskSpec 产出 deliverables

### 4.2 v1 推荐做法
- 会议与任务分别实现各自的 workflow（便于聚焦与简化）
- 共用：
  - Thread 级公共状态与输入清单（§1、§3）
  - Tool Gateway（写入 ACL + 审计）
  - AG-UI 对接（事件流 + tool + interrupt/resume）

> 后续若要“收敛到一起”，可以把 meeting 变成 `task.type=meeting` 或统一抽象成 `Workflow` 接口；但这是 v2/v3 的事，不阻塞 v1。
