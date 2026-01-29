# 提案：审计/验收证据链 v1（A 档：工程约束优先）

> 本提案面向 **fs 落盘**（单机/单写者）场景：先把“可复核的证据链”做对，再逐步增强检索与可观测性。  
> 约定：本提案不修改现有协议/实现，仅定义 v1 的目标形态与最小约束。

## 1. 目标与非目标

**目标**
- 让每个**审计单元（Bundle）**都能导出一份“证据包”，离线也可复核：**按哪套验收标准 → 得出什么结论 → 证据是什么**。
- 让任何关键动作都可追溯：**什么时候发生、谁触发、引用了哪些输入/产物**。
- 保持实现简单：以现有 `Gateway + tool_audit.jsonl` 为基础，不引入重系统。

**非目标（v1 不做）**
- 不做“不可抵赖/防内鬼”的密码学签名（hash 链、签名、时间戳服务属于 v2+）。
- 不把全量资源模型落盘成对象树（运行时只落必要子集；避免对象地狱）。
- 不引入外部数据库作为真相源（SQLite 仅作为可重建索引，放到后续）。

## 2. 三件套与责任边界

- `state.json`：**当前快照**（UI/快速读/断点恢复），允许覆盖写；不作为审计依据。
- `ledger/events.jsonl`：**验收账本**（append-only），所有“可审计事实”都落这里；这是 v1 的真相源。
- Trace（OTel）：**时序/性能透视**，可选；通过 `trace_id/span_id` 与账本互链，但不承担审计证明。

> 说明：仓库现有 `events/events.jsonl` 常被用于 SSE 回放/协议事件流（例如 AG-UI）。  
> v1 为避免语义混淆，新增 **`ledger/`** 专用于审计/验收；原 `events/` 维持现有用途。

## 3. 落盘布局（建议）

### 3.1 审计单元（Bundle）定义（v1 硬约束）

为避免“无限长总账”难以切片/归档/回放，v1 约定：
- **Bundle 是不可变边界**：除 `state.json`（快照，可覆盖）外，Bundle 内的 ledger/report/manifest/zip/证据文件均以 `create-only` 或 `append-only` 写入；不做覆盖写。
- **Bundle 有稳定 ID**：`bundle_id` 统一用 UUIDv7；不同类型可复用现有字段：
  - `zhanggui`：`bundle_id == run_id`
  - `taskctl`：`bundle_id == pack_id`
- **每个 Bundle 一份 ledger**：`ledger/events.jsonl` 仅记录该 Bundle 的审计/验收事实。

### 3.2 `zhanggui`（run 天然是 Bundle）

以 `fs/runs/{run_id}/` 为 Bundle 根（不入 git），建议结构：

```text
fs/runs/{run_id}/
  state.json                      # 快照（可覆盖）
  ledger/
    events.jsonl                  # 审计/验收账本（append-only）
  evidence/
    files/
      {sha256}                    # 内容寻址证据文件（create-only，可复用）
  verify/
    report.json                   # 验收报告（建议 create-only；或写入 evidence/files 后仅留指针）
  artifacts/
    manifest.json                 # 产物清单（路径→sha256/size）
  pack/
    evidence.zip                  # 证据包（用于归档/交付/复核）
  logs/
    tool_audit.jsonl              # 文件写入审计（必须，现有实现）
```

### 3.3 `taskctl`（以 pack_id 做 Bundle；提供 latest 指针）

`taskctl` 目录下同时存在两类东西：
- **工作区（可变）**：`revs/` 等用于产物生成与迭代。
- **审计 Bundle（不可变）**：每次 `VERIFY + PACK` 生成一个新的 `pack_id`，落在 `packs/{pack_id}/`。

建议结构：

```text
fs/taskctl/{task_id}/
  revs/
    r1/
      ...
  packs/
    {pack_id}/                    # Bundle 根（不可变）
      state.json                  # 本次打包快照（可选；可覆盖；不作为审计依据）
      ledger/events.jsonl         # 本次打包账本（append-only）
      evidence/files/{sha256}     # 本次打包证据库（create-only）
      verify/report.json          # 本次验收报告（create-only）
      artifacts/manifest.json     # 本次产物清单（create-only）
      pack/artifacts.zip          # 产物包（create-only）
      pack/evidence.zip           # 证据包（create-only）
      logs/tool_audit.jsonl       # 本次写入审计（append-only）
  pack/                           # latest 指针/快捷入口（可覆盖；不作为审计依据）
    latest.json                   # { "pack_id": "...", "created_at": "..." }
    artifacts.zip                 # 可选：最新产物包副本
    evidence.zip                  # 可选：最新证据包副本
    manifest.json                 # 可选：最新 manifest 副本
  verify/                         # latest 指针（可选）
    report.json                   # 可选：最新报告副本（审计引用仍走 sha256 ref）
```

#### `pack/latest.json`（taskctl latest 指针：最小 schema）

`pack/latest.json` 用于“快速定位最新 Bundle”，允许覆盖写、可重建、**不作为审计依据**（审计以 `packs/{pack_id}/ledger/events.jsonl` 为准）。

建议最小结构：

```json
{
  "schema_version": 1,
  "task_id": "0195d8a2-4c3b-7f12-8a3b-123456789abc",
  "pack_id": "0195d8a2-4c3b-7f13-8a3b-123456789abc",
  "rev": "r1",
  "created_at": "2026-01-29T12:00:00Z",
  "paths": {
    "bundle_root": "packs/0195d8a2-4c3b-7f13-8a3b-123456789abc/",
    "evidence_zip": "packs/0195d8a2-4c3b-7f13-8a3b-123456789abc/pack/evidence.zip",
    "artifacts_zip": "packs/0195d8a2-4c3b-7f13-8a3b-123456789abc/pack/artifacts.zip"
  }
}
```

## 4. Ledger 事件规范（`ledger/events.jsonl`）

### 4.1 事件 Envelope（v1 固定字段）
每行一个 JSON 对象，字段使用 `snake_case`：

v1 约定：所有**系统生成的运行时 ID**（如 `bundle_id/thread_id/run_id/task_id/intent_id/pack_id/event_id/change_set_id/...`）统一使用 **UUIDv7**（小写、标准连字符格式）。

- `schema_version`: `1`
- `ts`: RFC3339Nano（例如 `2026-01-29T12:34:56.789123456Z`）
- `seq`: `uint64`，同一 `events.jsonl` 内单调递增且不回退
- `event_id`: UUIDv7（可排序）
- `event_type`: 枚举（见 §4.3）
- `actor`: `{ "type": "system|agent|human", "id": "...", "role": "..." }`
- `correlation`: `{ "bundle_id", "thread_id"?, "run_id"?, "task_id"?, "rev"?, "intent_id"?, "pack_id"? }`
- `refs`: `[]`（证据/输入/产物引用，见 §4.2）
- `payload`: `{}`（按 `event_type` 扩展；v1 允许为空）
- `trace`: `{ "trace_id"?, "span_id"? }`（可选，仅用于跳转）

约束：
- `correlation.bundle_id` **必填**（与落盘 Bundle 根目录一一对应）。
- **大内容不进账本**：正文/文件一律走 `refs` 指向的证据文件。
- **脱敏**：`payload` 与 `refs.summary` 禁止写入 secrets/PII；必要时只写摘要/哈希。

### 4.2 `refs[]`（证据链核心）
`refs[]` 用于把“结论”绑定到“证据”。每个 ref 建议字段：

- `kind`: `criteria|input|artifact|report|approval|external`
- `id`: 稳定标识（推荐 `sha256:{hex}`；external 可用 `url:{...}`）
- `path`: 相对路径（**相对 Bundle 根目录**；`/` 分隔，便于 `rg -n`）
- `sha256`: 若 `path` 指向本地文件则必须填写
- `size`: 可选
- `summary`: 可选（短摘要，必须脱敏）

> v1 推荐将结构化证据（report/approval/criteria 快照等）写入 `evidence/files/{sha256}`，然后用 ref 绑定。

### 4.3 事件类型（v1 最小集合）
目标是覆盖“验收闭环 + 人工审批”，不追求细粒度全事件化。

- `BUNDLE_CREATED`：创建 Bundle 时；`payload` 记录版本信息（tool/spec/pack/protocol）。
- `STEP_STARTED` / `STEP_FINISHED`：`payload.step`（如 `SANDBOX_RUN|VERIFY|PACK`）+ `outcome`。
- `CRITERIA_SNAPSHOTTED`：把 `docs/**` 的验收标准快照写入证据库；ref 指向快照文件（sha256）。
- `VERIFY_REPORT_WRITTEN`：验收报告生成；ref 指向 `verify/report.json`（或 evidence/files）。
- `APPROVAL_REQUESTED`：请求人工审批；ref 指向相关报告/材料；`payload.approval_id` 必填。
- `APPROVAL_GRANTED` / `APPROVAL_DENIED`：审批结论；ref 指向审批记录（建议也是 evidence/files）。
- `ARTIFACT_MANIFEST_WRITTEN`：产物清单写出；ref 指向 `artifacts/manifest.json`（或 evidence/files）。
- `EVIDENCE_PACK_CREATED`：`pack/evidence.zip` 生成；ref 指向 zip（sha256）。

> v1 允许先只落关键事件，后续再补充：工具调用、输入上传、变更集等更细颗粒事件。

**关于“先打包后审批”（B 方案）**
- v1 允许 `APPROVAL_*` 发生在 `EVIDENCE_PACK_CREATED` 之后：此时已生成的 `pack/evidence.zip` **不会回写**，因此不保证包含后续审批记录。
- 审计真相仍以 `ledger/events.jsonl` + `evidence/files/{sha256}` 为准；需要单文件自包含时，升级到 A 方案（将 `evidence.zip` 生成延后到审批完成）或提供“重新导出 evidence.zip”能力。

### 4.4 示例：taskctl 一次打包的 ledger（JSONL，8 行）

下例展示一次 `VERIFY + PACK`（Bundle=`pack_id`）的最小账本事件流（每行一个 JSON）：

```jsonl
{"schema_version":1,"ts":"2026-01-29T12:00:00.000000000Z","seq":1,"event_id":"0195d8a2-4c3b-7f20-8a3b-000000000001","event_type":"BUNDLE_CREATED","actor":{"type":"system","id":"taskctl","role":"system"},"correlation":{"bundle_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","pack_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","task_id":"0195d8a2-4c3b-7f12-8a3b-123456789abc","rev":"r1"},"refs":[],"payload":{"tool_version":"0.1.0"}}
{"schema_version":1,"ts":"2026-01-29T12:00:00.010000000Z","seq":2,"event_id":"0195d8a2-4c3b-7f20-8a3b-000000000002","event_type":"STEP_STARTED","actor":{"type":"system","id":"taskctl","role":"system"},"correlation":{"bundle_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","pack_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","task_id":"0195d8a2-4c3b-7f12-8a3b-123456789abc","rev":"r1"},"refs":[],"payload":{"step":"VERIFY"}}
{"schema_version":1,"ts":"2026-01-29T12:00:00.020000000Z","seq":3,"event_id":"0195d8a2-4c3b-7f20-8a3b-000000000003","event_type":"CRITERIA_SNAPSHOTTED","actor":{"type":"system","id":"taskctl","role":"system"},"correlation":{"bundle_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","pack_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","task_id":"0195d8a2-4c3b-7f12-8a3b-123456789abc","rev":"r1"},"refs":[{"kind":"criteria","id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","path":"evidence/files/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sha256":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}],"payload":{"criteria_id":"docs.acceptance.v1","criteria_version":"0.1.0"}}
{"schema_version":1,"ts":"2026-01-29T12:00:00.030000000Z","seq":4,"event_id":"0195d8a2-4c3b-7f20-8a3b-000000000004","event_type":"VERIFY_REPORT_WRITTEN","actor":{"type":"system","id":"taskctl","role":"system"},"correlation":{"bundle_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","pack_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","task_id":"0195d8a2-4c3b-7f12-8a3b-123456789abc","rev":"r1"},"refs":[{"kind":"report","id":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","path":"verify/report.json","sha256":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}],"payload":{"summary":{"passed":5,"failed":0,"blocker":0}}}
{"schema_version":1,"ts":"2026-01-29T12:00:00.040000000Z","seq":5,"event_id":"0195d8a2-4c3b-7f20-8a3b-000000000005","event_type":"STEP_FINISHED","actor":{"type":"system","id":"taskctl","role":"system"},"correlation":{"bundle_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","pack_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","task_id":"0195d8a2-4c3b-7f12-8a3b-123456789abc","rev":"r1"},"refs":[],"payload":{"step":"VERIFY","outcome":"pass"}}
{"schema_version":1,"ts":"2026-01-29T12:00:00.050000000Z","seq":6,"event_id":"0195d8a2-4c3b-7f20-8a3b-000000000006","event_type":"STEP_STARTED","actor":{"type":"system","id":"taskctl","role":"system"},"correlation":{"bundle_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","pack_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","task_id":"0195d8a2-4c3b-7f12-8a3b-123456789abc","rev":"r1"},"refs":[],"payload":{"step":"PACK"}}
{"schema_version":1,"ts":"2026-01-29T12:00:00.060000000Z","seq":7,"event_id":"0195d8a2-4c3b-7f20-8a3b-000000000007","event_type":"ARTIFACT_MANIFEST_WRITTEN","actor":{"type":"system","id":"taskctl","role":"system"},"correlation":{"bundle_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","pack_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","task_id":"0195d8a2-4c3b-7f12-8a3b-123456789abc","rev":"r1"},"refs":[{"kind":"artifact","id":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","path":"artifacts/manifest.json","sha256":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}],"payload":{}}
{"schema_version":1,"ts":"2026-01-29T12:00:00.070000000Z","seq":8,"event_id":"0195d8a2-4c3b-7f20-8a3b-000000000008","event_type":"EVIDENCE_PACK_CREATED","actor":{"type":"system","id":"taskctl","role":"system"},"correlation":{"bundle_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","pack_id":"0195d8a2-4c3b-7f13-8a3b-123456789abc","task_id":"0195d8a2-4c3b-7f12-8a3b-123456789abc","rev":"r1"},"refs":[{"kind":"artifact","id":"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","path":"pack/artifacts.zip","sha256":"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"},{"kind":"artifact","id":"sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","path":"pack/evidence.zip","sha256":"sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"}],"payload":{"layout":"nested"}}
```

## 5. 验收标准：固定在 `docs/**`，但必须可冻结

v1 约定验收标准来源为仓库内文件（初始候选：`docs/proposals/acceptance_criteria_v1.yaml`），但**每次 verify 必须“冻结一份快照”**：

1) 读取 criteria 文件内容
2) 计算 `sha256`
3) 写入 `evidence/files/{sha256}`（create-only）
4) 写 `CRITERIA_SNAPSHOTTED` 事件引用该快照（以后复核按 sha256 找到“当时用的那套标准”）

## 6. 验收报告（`verify/report.json`）

报告必须能回答三个问题：**用哪套标准？每条标准结果如何？证据是什么？**

建议最小结构：

- `schema_version`: `1`
- `correlation`: 同 ledger
- `criteria`: `{ "id": "...", "sha256": "...", "path": "evidence/files/{sha256}" }`
- `results[]`：
  - `criteria_id`
  - `status`: `PASS|FAIL|SKIP`
  - `severity`: `blocker|warn|info`
  - `evidence_refs[]`: 直接复用与 ledger 一致的 ref 结构
  - `notes`: 可选（脱敏）
- `summary`: `{passed, failed, blocker}`

约束：
- `results[].evidence_refs[]` 必须可校验（本地文件必须带 sha256）。
- 推荐“只追加/只新建”：需要重新验收时生成新报告并写新事件，不覆盖旧报告。
- 为便于 UI/快速查看，可选维护一个“latest 指针/副本”（例如 `{task_root}/verify/report.json` 或 `{task_root}/pack/latest.json`），但**审计引用必须以 `refs.sha256` 为准**。

## 7. Evidence Pack（`pack/evidence.zip`）

**目的**：把“复核所需的一切”打成单文件，便于归档/交付/复现。

v1 建议包含（至少）：
- `ledger/events.jsonl`
- `logs/tool_audit.jsonl`
- `verify/report.json`（或 `evidence/files/...` 中对应的报告文件）
- `artifacts/manifest.json`
- `pack/artifacts.zip`（**默认嵌套包含，不展开**）
- `state.json`（可选：仅作为辅助，不作为审计依据）

生成后必须写 `EVIDENCE_PACK_CREATED` 事件，并对 zip 本身记录 `sha256`。
后续扩展（v2+）可增加 `--evidence-layout=expanded`：将 `artifacts.zip` 展开进 evidence.zip，但不得破坏 v1 的默认布局与兼容性。

## 8. A 档工程保障（v1 的“可信度来源”）

v1 不做 hash 链，因此可信度主要来自工程约束：

- **所有写入必须走 Gateway**：借助 `tool_audit.jsonl` 记录 who/what/where/result/linkage。
- **`ledger/events.jsonl` 强制 append-only**：Gateway policy 的 `AppendOnlyFiles` 必须包含它。
- **证据文件 create-only**：证据库（如 `evidence/files/{sha256}`）用 `create` 写入，复用时只引用不覆盖。
- **敏感信息隔离**：证据里只存脱敏摘要；原文/附件按需存入证据库并计算哈希。

## 9. 最小复核流程（给人/工具用）

给定一个 `{bundle_root}` 或 `pack/evidence.zip`：
1) 查 `ledger/events.jsonl`：找到 `VERIFY_REPORT_WRITTEN` 与 `EVIDENCE_PACK_CREATED`
2) 校验关键文件 sha256（报告、manifest、zip）
3) 按报告的 `criteria.sha256` 取出当时的标准快照，复跑或人工复核
4) 交叉检查 `logs/tool_audit.jsonl`：关键文件是否由允许角色写出、是否有失败/拒绝记录

## 10. 后续扩展（v2+）

- **B 档（hash 链）**：在 ledger 引入 `prev_hash/hash` 形成 tamper-evident 链。
- **SQLite 索引**：旁路生成 `events.db`（仅存关键列 + 文件偏移），可随时删除重建。
- **对外协议对齐（lite）**：把 `correlation/actor/refs/trace` 与对外 AG-UI（以及未来可能的其它协议）互映射。
