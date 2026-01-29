# 13 IGI v1：资源模型（Canonical）与对外承载（AG-UI）

> 归档说明：当前仓库主线已采用更轻量的 v1 落地（以 `FILE_STRUCTURE.md` 与 Bundle/ledger/evidence 为准），因此本文不再作为“必实现规范”。如未来需要引入 IGI 作为 canonical model，可把本文作为参考输入逐步迁移。

> 目标：把 zhanggui 的“世界定义 / 公司级架构协议”先落成 v1：**IGI（apiVersion: `igi.zhanggui.io/v1`）**。  
> 定位：IGI 是 **真相源（canonical model）**；AG-UI 是 **前端交互事件承载（transport/presentation）**。  
> 原则：v1 不做通用“协议互转引擎”，只规定 **承载方式** 与 **落盘/追溯**，为 v2/v3 扩展留 `ext` 端口。

---

## 0) IGI 与 AG-UI 的关系（先写死）

### 0.1 我们的选择（v1）
- **对 UI 的事件流**：继续使用 AG-UI（见 `docs/11_ag_ui_integration.md`）。  
- **系统内部状态/对象**：统一用 IGI 资源对象表达（本文件）。  
- **二者关系**：IGI 的资源快照/增量更新通过 AG-UI 的 `STATE_SNAPSHOT/STATE_DELTA`（以及必要时 `CUSTOM`）承载。

> 结果：对外只需要适配 AG-UI；对内所有系统/Agent 统一理解 IGI 资源模型，不会被 UI 协议绑死。

### 0.2 为什么不在 v1 做“AG-UI ↔ IGI 全量转换”
v1 只需要：
- **IGI → AG-UI（输出）**：把 IGI state 投影到 `STATE_*`/`ACTIVITY_*`。  
- **AG-UI → IGI（输入）**：把 tool_result/resume/user_message 规范化为 IGI 的命令/变更（通常是 `ChangeSet/Directive` 更新）。

通用互转引擎（多协议、多版本）属于 v2/v3；否则会在 v1 过早引入复杂度与兼容成本。

---

## 1) 资源外壳（K8s 风格，v1 统一口径）

### 1.1 通用结构（所有 kind 都一样的外壳）
```json
{
  "apiVersion": "igi.zhanggui.io/v1",
  "kind": "Thread",
  "metadata": {
    "id": "thr_01J...",
    "rid": "igi://org/acme/project/zhanggui/threads/thr_01J...",
    "scope": { "orgId": "acme", "projectId": "zhanggui", "caseId": "case-001" },
    "createdAt": "2026-01-29T03:20:00Z",
    "createdBy": { "actorType": "user", "actorId": "u-001", "display": "张三" },
    "updatedAt": "2026-01-29T03:21:00Z",
    "updatedBy": { "actorType": "system", "actorId": "zhanggui" },
    "labels": { "env": "local" },
    "annotations": {},
    "ext": {}
  },
  "spec": {},
  "status": {}
}
```

### 1.2 字段约束（必须）
- `apiVersion`：固定为 `igi.zhanggui.io/v1`（v2/v3 另起 version）。  
- `kind`：固定枚举（见 §2）。  
- `metadata.id`：该 kind 下全局唯一（推荐 UUIDv7；v1 只要求可作为目录名与键）。  
- `metadata.scope.orgId/projectId`：必须（避免“名字太简单导致归属不清”）。  
- `metadata.createdBy/updatedBy`：必须（审计与追溯）。  
- `metadata.ext`：预留扩展（禁止把未知字段塞进 spec/status 顶层）。

### 1.3 `rid`（资源标识符，建议但强烈推荐）
`rid` 用于跨系统引用/迁移/转换，v1 推荐 URI 形态：
- `igi://org/{orgId}/project/{projectId}/{pluralKind}/{id}`

> v1 允许 `rid` 为空（仅本地单机），但一旦对外/跨系统，就必须补齐。

---

## 2) v1 核心 kind 列表（先定最小集合）

> 说明：Meeting/Task 等更业务的对象可以后延；v1 先把“协作/输入/变更/执行/进度/打包”做成通用底座。

### 2.1 `Thread`（协作容器：共识/控制/进度）
**用途**：承载用户“思想状态（Directive）”、控制信号（pause/resume/cancel）、全员进度（progress board）、输入资料包（Artifact/Manifest）。

`spec`（建议最小字段）：
- `directiveRef`：当前生效的 `Directive` 引用（或直接内嵌 id）
- `policy`：输入限流（max_files/max_bytes/max_urls...）

`status`（建议最小字段）：
- `phase`: `RUNNING|PAUSE_REQUESTED|PAUSED|CANCELED`
- `activeRunId`：当前 active run（v1 建议同 thread 只允许 1 个 active run）
- `progress`：`{agents: {agentId: AgentStatus.status...}, updatedAt}`
- `lastChangeSetId`：最近一次变更包

### 2.2 `Directive`（用户思想状态/北极星）
**用途**：把“目标/约束/验收/优先级/决策”结构化同步给所有参与者，避免跑偏。

`spec`（建议）：
- `revision`（整数递增；并发冲突用它判定）
- `goal`（一句话/短段落）
- `constraints[]`
- `acceptance.must_answer[] / must_not[]`
- `priorities[]`
- `ext.freeform_md`（可选兜底：允许用户自由补充）

`status`（建议）：
- `effectiveAt`
- `supersedes`（上一个 directive id）

### 2.3 `ChangeSet`（追加需求/变更包）
**用途**：把“用户追加需求/暂停时的输入”从聊天文本升级为可追溯变更单；后续所有系统只认 ChangeSet。

`spec`（建议）：
- `message`：用户原始描述
- `inputRefs[]`：引用 `Artifact`（digest/rid）或 URL 快照等
- `requestedControl`：`{mode: "drain_step", reason: "change_request"}`
- `proposedPatchRef?`：可选（来自会议/Planner 的 patch 提案）

`status`（建议）：
- `intakeStatus`: `PENDING|INDEXED|LIMITED|REJECTED|APPLIED`
- `decision`：`approve|reject|needs_more_info`

### 2.4 `Artifact`（内容寻址对象：CAS Blob）
**用途**：统一表示文件/图片/PDF/抓取快照等“内容本体”，以 digest 定位。

`spec`（建议）：
- `digest`：`sha256:<hex>`
- `sizeBytes`
- `mime`
- `title?`
- `source`：`user_upload|user_url|ui_pick_file|system_generated`
- `storedPath?`：仅本地模式使用（相对 thread inputs 目录）；对外系统不依赖此字段

### 2.5 `ArtifactManifest`（资料包清单）
**用途**：定义一个“可复现的资料集合”（一组 artifacts + 描述符），供所有 Agent/流程复用。

`spec`（建议）：
- `items[]`：`{artifactId|digest|rid, role, notes, required}`
- `generatedFrom`：引用 `ChangeSet` 或用户确认点

### 2.6 `Run`（一次执行实例）
**用途**：与 `fs/runs/{run_id}` 对齐，记录本次执行的状态、父子关系、以及对外事件流（AG-UI）证据链。

> 注意：Run 的对外事件仍走 AG-UI；IGI 的 Run 只是把其元信息与状态投影成资源对象，便于跨系统对齐。

### 2.7 `AgentStatus`（进度条目）
**用途**：让用户看到“所有人的进度”，并支持暂停/变更时的统一收尾。

`status`（建议）：
- `phase`: `idle|running|blocked|paused|done|error`
- `step`
- `pct`（0~100，可粗粒度）
- `activity`
- `updatedAt`
- `lastArtifactRef?`

### 2.8 `Bundle`（交付包：Pack 输出）
**用途**：表示最终可交付的打包产物（zip + manifest + 校验信息），与 `PACK` 阶段对齐。

---

## 3) 落盘映射（v1：先落本地文件系统）

目录权威见 `FILE_STRUCTURE.md`，v1 最少要求：
- `fs/threads/{thread_id}/state.json`：保存一个 `Thread` 资源快照（single-writer：系统）
- `fs/threads/{thread_id}/events/events.jsonl`：保存 IGI 资源变更事件（append-only：系统）
- `fs/threads/{thread_id}/inputs/**`：保存 `Artifact` 的本地存储（不入 git）
- `fs/runs/{run_id}/events/events.jsonl`：保存 AG-UI 事件流（append-only）

> v1 同时保留 IGI（canonical ledger）与 AG-UI（presentation log），未来可回放、可迁移、可转换。

---

## 4) 通过 AG-UI 承载 IGI（映射规则，v1 必须一致）

### 4.1 Thread 级状态同步（推荐）
- `STATE_SNAPSHOT.snapshot`：放一个对象 `{thread: Thread, directive: Directive, manifest?: ArtifactManifest, runs?: [...]}`  
- `STATE_DELTA.delta`：RFC6902 patch（例如更新 `thread.status.progress`、`thread.status.phase`）

> 这样 UI 只要实现 AG-UI 的 state 同步，就天然具备“思想状态同步 + 全员进度面板”。

### 4.2 变更/暂停（ChangeSet + drain_step）
当用户追加需求/上传大量资料：
1) 先把资料入库成 `Artifact`（digest），更新 `ArtifactManifest`
2) 再创建 `ChangeSet`
3) 更新 `Thread.status.phase = PAUSE_REQUESTED`
4) 当前 run 在 step 边界收尾后，用 `RUN_FINISHED outcome=interrupt` 结束（AG-UI）
5) 下一次 `/run` 的 `resume.payload` 指向 `ChangeSet`（或包含 decision），继续执行 intake/apply

### 4.3 `CUSTOM` 事件（仅在需要“命令语义”时使用）
v1 不强制，但建议保留：
- `CUSTOM.name = "igi.command"`
- `CUSTOM.value = { apiVersion, kind: "Command", ... }`

> 未来多系统/多 Agent 接入时，命令也可以变成一种资源对象（v2）。

---

## 5) 契约落库（contracts）

（历史记录）当时计划在 `contracts/igi/v1/` 固化 JSON Schema（最小集合）；当前主线已移除该目录，仅保留本文作为参考。
- 资源外壳（Resource Envelope）
- `Thread/Directive/ChangeSet/Artifact/ArtifactManifest/AgentStatus/Bundle`
- `ThreadSnapshot`（Thread 协作视图：snapshot + watch/subscribe；见 `docs/archive/igi/14_igi_thread_api_v1.md`）

并遵循兼容性规则：
- v1 内新增字段只能加到 `metadata.ext/spec.ext/status.ext` 或新增 optional 字段
- 破坏性变更必须升 `igi.zhanggui.io/v2`
