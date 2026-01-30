# 11 前端 AI 界面对接（AG-UI 协议：落库草案）

> 本文目标：把“前端 AI 界面”与 zhanggui 的对接方式先**落库**成可执行契约（但不保证一次写完）。  
> 当前定位：**对接规范的一部分**（先把 event/tools/interrupt-resume 的最小约束写清楚）。  
> 设计原则：UI 负责用户侧交互与外部动作触发；后端负责发起 run、产出事件流、落盘、审计、以及在工具结果回来后继续推进。
>
> v1 重要口径（必须）：**主线真相源以文件系统约定为准**（见 `FILE_STRUCTURE.md` 与 Bundle/ledger/evidence 规范）；**AG-UI 只是 UI 对接的事件承载协议**。  
> 实践上：系统状态快照/增量更新通过 AG-UI 的 `STATE_SNAPSHOT/STATE_DELTA`（必要时 `CUSTOM`）承载，payload 对齐本仓库的 `state.json` / thread state 形状（而不是强依赖某个外部资源模型）。

---

## 0) 总体目标（必须）

我们同时支持两种人机交互机制：
1) **Frontend Tools（前端工具）**：细粒度交互（确认、表单、选文件、预览审阅…），由 UI 执行并返回结果。
2) **Interrupt / Resume（中断/恢复）**：粗粒度“流程关口”（人审/复核/强审批），后端在中断点结束当前 run，等待 UI 触发下一次 run 继续。

> 约束：无论是否启用 Docker/VM 沙箱，**所有落盘写入必须走 Tool Gateway**（见 `docs/10_tool_gateway_acl.md`），以保证路径/动作/单写者/审计一致。

---

## 1) 组件分工（建议）

### 1.1 UI（前端）负责
- 渲染事件流（文本、步骤、活动、状态快照）
- 执行 `ui.*` 工具（用户确认/输入/选择/预览审阅/调度）
- 在需要时触发 interrupt 页面（审批页/复核页），并在用户操作后 resume

### 1.2 zhanggui（后端）负责
- 提供 `run(input) -> 事件流（SSE/WS）`
- 在 run 中触发 Tool Call（请求 UI 执行工具），并等待 Tool Result 回填
- 维护 run 状态与可恢复性（interrupt/resume、重连后 snapshot）
- 将“可写文件系统边界”统一收敛到 Tool Gateway，并落审计 `tool_audit.jsonl`

---

## 2) 对接形态（我们建议的最小落地）

### 2.1 传输：SSE 优先（最简单）
- 后端对 UI 输出：**单向 SSE**（事件序列）
- UI 对后端回传：通过 **HTTP POST** 提交 tool result / resume payload（避免在 SSE 上做反向通道）

### 2.2 “本地单跑”也适用
即使先不做完整 UI，也可以用 CLI/脚本模拟 UI：
- 后端输出事件 JSONL 到文件
- 人工/脚本构造 tool result，再调用 resume 或 tool-result endpoint 继续

### 2.3 最小 HTTP 端点（当前实现）
> 端点与 base path 都需要可配置，避免未来协议/路由调整导致大改。

默认：
- `GET /healthz`
- `POST /agui/run`：返回 SSE（事件流）
- `POST /agui/tool_result`：回填工具结果

运行参数（实现层面）：
- 监听地址/端口可配（例如 `127.0.0.1:8020`）
- `base_path` 可配（例如从 `/agui` 改成 `/ag-ui`）

---

## 3) 事件契约（Events Contract）

### 3.1 命名风格（本仓库固定口径）
本阶段先遵循 **AG-UI 的 wire format**（把它当做对外协议），因此：
- 事件 `type`：按 AG-UI 协议/SDK 的定义原样使用（例如 `RUN_STARTED`）。
- 字段命名：优先使用 AG-UI 常见写法（例如 `threadId/runId/messageId/toolCallId`）。

> 兼容策略（必须）：服务端 **入站接受多种别名**（camelCase/snake_case），但**出站尽量保持单一风格**，避免 UI 侧适配成本爆炸。  
> 说明：我们不在此阶段做“协议转换层”（避免复杂度），但会保留 `rawEvent` 与版本字段，为后续升级/转换留钩子。

### 3.2 SSE 包装（建议）
每个事件一条 SSE message，`data` 为完整 JSON：

```text
event: agui
data: {"type":"RUN_STARTED","threadId":"t1","runId":"r1", ...}

```

客户端必须以 JSON 内的 `type` 作为最终判定依据。

### 3.3 通用事件外壳（最小字段）
每个事件对象至少包含：
- `type`：事件类型
- `timestamp`：UNIX 毫秒时间戳（number；建议；用于排序与审计）
- `runId`：运行 ID（建议；用于重连与追溯）
- `threadId`：线程/会话 ID（建议；用于 UI 会话归并）

> 兼容字段：`rawEvent` 可保留上游原始事件（透传/调试）。

---

## 4) 事件类型清单（本仓库最小子集）

> 说明：以下是我们当前需要支持的“最小子集”。其余草案/扩展事件（reasoning/meta 等）先不作为强依赖契约。

### 4.1 Lifecycle（运行生命周期）
- `RUN_STARTED`
- `STEP_STARTED`
- `STEP_FINISHED`
- `RUN_FINISHED`
- `RUN_ERROR`

### 4.2 Text（文本流）
- `TEXT_MESSAGE_START`
- `TEXT_MESSAGE_CONTENT`
- `TEXT_MESSAGE_END`
- `TEXT_MESSAGE_CHUNK`（便利事件：可展开为 start/content/end）

### 4.3 Tool Call（工具调用）
- `TOOL_CALL_START`
- `TOOL_CALL_ARGS`
- `TOOL_CALL_END`
- `TOOL_CALL_RESULT`
- `TOOL_CALL_CHUNK`（便利事件：可展开为 start/args/end）

### 4.4 State（状态同步）
- `STATE_SNAPSHOT`
- `STATE_DELTA`（RFC6902 JSON Patch）

### 4.5 Activity（A2UI 载荷）
- `activity_message`：用于承载 A2UI 消息流（前端渲染协议）
  - `content.spec`：固定 `a2ui`
  - `content.version`：按 A2UI 实际版本（例如 `0.9` / `1.0`）
  - `content.messages`：A2UI envelope 列表（`createSurface` / `updateComponents` / `updateDataModel` / `deleteSurface`）

> 说明：A2UI 作为 UI 渲染协议，不新增独立 SSE 端点；通过 AG-UI 的活动类事件透传给前端。
- `MESSAGES_SNAPSHOT`

### 4.5 Activity（结构化活动提示）
- `ACTIVITY_SNAPSHOT`
- `ACTIVITY_DELTA`（RFC6902 JSON Patch）

### 4.6 Special（扩展/透传）
- `RAW`
- `CUSTOM`

---

## 5) Interrupt / Resume（中断/恢复）

### 5.1 中断（interrupt）
当 run 需要进入“人审/复核/强审批”时：
- 后端以 `RUN_FINISHED` 结束当前 run，但带 `outcome: "interrupt"` 与 `interrupt` 载荷。
- UI 进入审批页/复核页；用户完成后，再发起下一次 run（resume）。

`RUN_FINISHED`（扩展字段示意）：
```json
{
  "type": "RUN_FINISHED",
  "threadId": "t1",
  "runId": "r1",
  "outcome": "interrupt",
  "interrupt": {
    "id": "int-0001",
    "reason": "human_approval",
    "payload": { "title": "请审批：是否发布", "risk_level": "high" }
  }
}
```

### 5.2 恢复（resume）
恢复通过“开启下一次 run”实现：在 `RunAgentInput` 增加 `resume` 字段：
```json
{
  "threadId": "t1",
  "runId": "r2",
  "resume": {
    "interruptId": "int-0001",
    "payload": { "verdict": "approve", "comment": "可以发布" }
  }
}
```

> 约束：resume payload 必须可落盘（JSON），用于后续追溯。

---

## 6) Frontend Tools（UI 工具）模型

### 6.1 设计原则（必须）
- **任何需要用户参与/确认/输入/选择/查看/决定/触发外部动作** → 一律建模为 `ui.*` 工具，由 UI 执行。
- 后端只负责：发出 tool call、等待 tool result、落盘与继续执行。
- 工具结果必须是 JSON 可序列化内容（对象/数组/字符串均可；若对接方限制类型，需约定 stringify）。

### 6.2 推荐的最小工具清单（先落库）
1) `ui.confirm`：确认/拒绝
2) `ui.form`：渲染表单，回传 JSON
3) `ui.pick_file`：选择文件/目录（回传 handle/path/metadata）
4) `ui.open_url`：打开链接/跳转页面
5) `ui.notify`：通知（toast/桌面）
6) `ui.review_artifacts`：预览并给 verdict（pass/revise）+ comment
7) `ui.schedule`：设置定时/日程
8) `ui.provide_secret`：输入 secret（仅在 UI 侧保存/系统 keychain；后端不落明文）

> 后续会补：每个工具的 args/result JSON Schema（建议落到 `contracts/ag_ui/tools.json` 并版本化）。

---

## 7) 与文件系统（fs/）与 Tool Gateway 的对接（必须）

### 7.1 统一要求
- `fs/**` 为运行态数据目录，**不入 git**（见 `.gitignore` 与 `FILE_STRUCTURE.md`）。
- 后端落盘必须走 Tool Gateway：审计写入 `logs/tool_audit.jsonl`（jsonl）。

### 7.2 UI 工具与落盘的边界
- UI 返回的 `ui.pick_file` 结果（path/handle）不能直接被当作“可写路径”使用。
- 后端必须将其复制/导入到允许目录（例如某个 run/task 的 workspace），并通过 Tool Gateway 写入。
- 对“用户输入”（图片/PDF/URL/大量文件）必须先按统一输入清单入库，再在 run 中用引用消费；见 `docs/12_runtime_and_input_model.md`。

### 7.3 run 落盘目录（建议对齐 `FILE_STRUCTURE.md`）
AG-UI 的 run 事件建议落盘到：
- `fs/runs/{run_id}/run.json`
- `fs/runs/{run_id}/state.json`
- `fs/runs/{run_id}/events/events.jsonl`（append-only）
- `fs/runs/{run_id}/logs/tool_audit.jsonl`

---

## 8) zhanggui 的对接建议（下一步实现顺序）

> 会议（Meeting Mode）可以后延；先把 UI 对接跑起来，能驱动任务执行与产物落盘。

建议按以下顺序推进（对应 `docs/08_development_plan.md` 可新增 Stage 1.5）：
1) 建立 `run -> SSE events` 的最小服务（含重连 snapshot）
2) 落地 `ui.confirm/ui.form` 两个工具闭环（tool call → tool result → 继续 run）
3) 落地 interrupt/resume（审批页闭环）
4) 将 run 事件与 tool result 全量落盘（jsonl + snapshot），并与 Tool Gateway 审计关联（linkage）

---

## 9) 与本仓库设计规范的一致性评估（结论先行）

AG-UI 作为“对外协议（wire format）”，整体与本仓库的设计原则是**相容**的，但需要我们补齐两层边界：

**符合的部分（优势）**
- **渐进式加载**：事件流天然支持增量（delta/chunk），符合“只加载必要片段”的原则（见 `docs/01_minimal_kernel.md`）。
- **强协议节点**：Tool Call 把“需要用户/外部系统参与”的动作显式化，利于审计与可控性（见 `docs/03_artifact_pipeline.md`）。
- **可恢复**：interrupt/resume 让“人审关口”从隐式等待变成显式状态机边界。

**不覆盖的部分（需要我们补齐）**
- **文件系统边界/权限**：AG-UI 不关心落盘路径与写入语义；必须由 Tool Gateway 强制 ACL/append-only/single-writer，并落审计（见 `docs/10_tool_gateway_acl.md`）。
- **产物协议**：AG-UI 不定义任务产物格式；仍需按 `docs/08_development_plan.md`/`FILE_STRUCTURE.md` 的产物规范执行。
- **版本演进**：协议可能变化；本阶段不做转换层，但必须保留 `rawEvent` 与 `protocol`/`schema_version` 等字段以便追溯与迁移。

---

## 10) 最小 demo 事件流样例（当前实现：workflow=demo）

> 目的：让前后端能各自开工，不因“事件顺序/字段名”扯皮。  
> 注意：这只是 demo（会演示 tool call + interrupt/resume），不代表最终业务流程。

### 10.1 run#1：tool call（ui.form）→ interrupt
典型序列：
1) `RUN_STARTED`
2) `STEP_STARTED`（`COLLECT`）
3) `TEXT_MESSAGE_START/CONTENT/END`
4) `TOOL_CALL_START/ARGS/END`（`ui.form`）
5) `TOOL_CALL_RESULT`（UI 回填后出现）
6) `STEP_FINISHED`（`COLLECT`）
7) `STEP_STARTED/FINISHED`（`PROCESS`）
8) `RUN_FINISHED`（`outcome="interrupt"`，带 `interrupt.id`）

### 10.2 run#2：resume → success
典型序列：
1) `RUN_STARTED`
2) `STEP_STARTED/FINISHED`（`FINALIZE`）
3) `RUN_FINISHED`（`outcome="success"`）
