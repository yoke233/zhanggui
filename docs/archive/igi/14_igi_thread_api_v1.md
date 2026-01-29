# 14 IGI v1：Thread API（Snapshot + Watch/Subscribe，协议先行）

> 归档说明：当前仓库主线已采用更轻量的 v1 落地（以 `FILE_STRUCTURE.md` 与 Bundle/ledger/evidence 为准），因此本文不再作为“必实现规范”。如未来需要引入 IGI，可把本文作为参考输入。

> 本文目标：定义 **Thread 级协作 API**（不实现，先定协议）：  
> - `ThreadSnapshot`：UI/系统一次性拿到“全局一致目标 + 输入资料包 + 变更包 + 全员进度”的状态形状  
> - `watch/subscribe`：持续订阅 thread 的状态增量（pause/change/directive/progress）  
>
> 口径：IGI（`apiVersion: igi.zhanggui.io/v1`）是 **真相源**（见 `docs/archive/igi/13_igi_v1_resource_model.md`）；对外事件承载优先复用 AG-UI（见 `docs/11_ag_ui_integration.md`）。

---

## 0) 设计目标（v1）

1) **用户“思想状态”自上而下同步**：任何参与者/Agent/UI 都应以同一份 `Directive` 为准。  
2) **用户能看到全员进度**：thread 的进度面板是第一类状态，而不是跑完才看日志。  
3) **追加需求/暂停包含复杂输入**：图片/PDF/URL/大量文件必须先入库为 `Artifact/ArtifactManifest`，再以引用进入 `ChangeSet`；run 里只消费引用。  
4) **不绑定实现**：HTTP path、SSE event name 可配置；客户端必须依赖 JSON 字段（`apiVersion/kind/type`），忽略未知字段（向前兼容）。

---

## 1) API Base Path（建议）

我们采用 k8s 风格的 group/version 路径（建议但不强制）：

```text
/apis/igi.zhanggui.io/v1
```

> 实现侧必须提供 `base_path` 配置（类似现有 `/agui`），避免未来迁移/网关改路由导致大改。

---

## 2) ThreadSnapshot（核心：状态形状）

### 2.1 `ThreadSnapshot` 的用途
`ThreadSnapshot` 是一个 **聚合视图**（view），用于让 UI/系统一次性得到：
- 当前 `Thread`（控制状态、active run、进度面板）
- 当前 `Directive`（用户思想状态）
- 当前 `ArtifactManifest`（资料包清单；可截断）
- 最近 `ChangeSet`（追加需求/变更包；可截断）
- 其它可选资源（例如：最近 Run 列表摘要）

> 注意：这不是“列出所有 Artifact”。Artifact 数量可能很大；v1 snapshot 只应包含 manifest/计数/分页信息。

### 2.2 JSON 结构（v1 固定）
`ThreadSnapshot` 本身也采用 IGI 资源外壳（便于版本化与扩展）：

```json
{
  "apiVersion": "igi.zhanggui.io/v1",
  "kind": "ThreadSnapshot",
  "metadata": {
    "id": "thr_01J...",
    "scope": { "orgId": "acme", "projectId": "zhanggui" },
    "createdAt": "2026-01-29T03:30:00Z",
    "createdBy": { "actorType": "system", "actorId": "zhanggui" },
    "ext": { "sequence": 42, "snapshotAt": "2026-01-29T03:30:00Z" }
  },
  "spec": {
    "thread": { "...": "Thread resource" },
    "directive": { "...": "Directive resource (optional)" },
    "artifactManifest": { "...": "ArtifactManifest (optional)" },
    "recentChangeSets": [{ "...": "ChangeSet (optional)" }],
    "paging": {
      "recentChangeSets": { "truncated": true, "nextCursor": "..." },
      "artifacts": { "truncated": true, "nextCursor": "..." }
    },
    "ext": {}
  },
  "status": { "ext": {} }
}
```

### 2.3 Patch 路径约定（STATE_DELTA 必须遵守）
Thread watch 的增量更新用 RFC6902 JSON Patch，路径一律以 snapshot 为根：

- `Thread` 控制状态：`/spec/thread/status/phase`
- active run：`/spec/thread/status/activeRunId`
- 进度面板：`/spec/thread/status/progress/agents/{agentId}/pct` 等
- 当前 directive：`/spec/directive`（整体替换）或更细粒度 patch（可选）
- 最近 changesets：`/spec/recentChangeSets`（整体替换，v1 简化）
- paging：`/spec/paging/*`

> v1 建议：对 `directive/recentChangeSets/artifactManifest` 以“整体替换”为主，避免早期过度细粒度 patch 引入合并复杂度。

---

## 3) Watch/Subscribe（SSE：持续订阅 thread 状态）

### 3.1 Endpoint（建议）
```text
GET /apis/igi.zhanggui.io/v1/threads/{threadId}/watch
```

Query（v1 建议）：
- `cursor`：从某个事件/序列号恢复（断线续传）
- `heartbeat_seconds`：心跳间隔（默认 15）

### 3.2 SSE 事件承载（复用 AG-UI 的类型）
Thread watch 的 SSE `data` 必须是 **AG-UI 事件对象**（兼容 UI 事件处理器），其中：
- 首条必须发送 `STATE_SNAPSHOT`，`snapshot` 字段为 `ThreadSnapshot`
- 后续发送 `STATE_DELTA`（或在必要时再次发 `STATE_SNAPSHOT` 重新同步）

示例：
```text
event: igi
data: {"type":"STATE_SNAPSHOT","timestamp":"...","snapshot":{...ThreadSnapshot...}}

event: igi
data: {"type":"STATE_DELTA","timestamp":"...","delta":[{"op":"replace","path":"/spec/thread/status/phase","value":"PAUSE_REQUESTED"}]}
```

> 兼容：SSE event name 可为 `igi`/`agui`，客户端必须以 JSON 内 `type` 判定语义。

### 3.3 事件元信息（建议在 v1 就预留）
为满足多系统/多 Agent 的追溯与去重，thread watch 的事件建议携带（字段存在即用，不存在也不得报错）：
- `eventId`：全局唯一
- `producer`：`{service, instanceId}`
- `actor`：`{actorType, actorId}`
- `subject`：`{apiVersion, kind, id}`（通常为 `Thread`）
- `correlation`：`{threadId, runId?, changeSetId?}`

> 这些字段不属于 AG-UI 的强制字段，但属于 IGI 的长期演进需求；v1 先“可出现”，v2 可升级为“必须”。

---

## 4) Snapshot 获取接口（非流式）

### 4.1 Endpoint（建议）
```text
GET /apis/igi.zhanggui.io/v1/threads/{threadId}/snapshot
```

Response：
- `200`：返回 `ThreadSnapshot` JSON
- `404`：thread 不存在

---

## 5) 典型流程（v1：追加需求 → 可控暂停 → 继续）

### 5.1 追加需求包含大输入
1) UI/系统先把文件/URL 入库为 `Artifact`，更新 `ArtifactManifest`
2) 生成 `ChangeSet`（引用 artifact/manifest）
3) thread watch 广播：
   - `STATE_DELTA`: `Thread.status.phase=PAUSE_REQUESTED`
   - （可选）`ACTIVITY_*`: 展示“正在收尾暂停”
4) 当前 run 在 step 边界收尾后用 `RUN_FINISHED outcome=interrupt` 结束（AG-UI run stream）
5) 用户确认后 resume：下一次 `/agui/run` 带 `resume.payload` 指向 `changeSetId`（或携带 decision）

> v1 关键点：输入先入库（Artifact），变更以 ChangeSet 表达；run 里只处理引用。

---

## 6) v1 明确不做（避免过早复杂化）

- 不做跨服务的通用 watch 框架（先本地单机/单服务）
- 不做细粒度的多 writer 合并（directive/changeset 冲突先用 revision + ask_user）
- 不在 v1 规定 artifact 的上传传输协议细节（multipart/分片/断点续传）——只规定落盘与 digest/manifest 语义

---

## 7) 最小端到端演示（pwsh；不要求 UI）

> 前提：Windows 上请使用 `curl.exe`（不要用 PowerShell 的 `curl` alias）。

### 7.1 启动服务
```powershell
go run .\cmd\zhanggui\main.go serve `
  --http-addr 127.0.0.1 `
  --http-port 8020 `
  --runs-dir fs/runs `
  --threads-dir fs/threads `
  --igi-base-path /apis/igi.zhanggui.io/v1 `
  --igi-event-name igi
```

### 7.2 创建一个 thread（用最小 run：`workflow=ping`）
```powershell
curl.exe -N -X POST "http://127.0.0.1:8020/agui/run" `
  -H "Content-Type: application/json" `
  -d "{\"threadId\":\"thread-demo-1\",\"runId\":\"run-demo-1\",\"workflow\":\"ping\"}"
```

此时会生成（运行态数据，不入 git）：
- `fs/threads/thread-demo-1/state.json`
- `fs/threads/thread-demo-1/events/events.jsonl`
- `fs/threads/thread-demo-1/logs/tool_audit.jsonl`

### 7.3 订阅 thread watch（观察 STATE_SNAPSHOT + STATE_DELTA）
```powershell
curl.exe -N "http://127.0.0.1:8020/apis/igi.zhanggui.io/v1/threads/thread-demo-1/watch"
```

新开一个终端触发一次 run（会看到 activeRunId/phase 的 delta）：
```powershell
curl.exe -N -X POST "http://127.0.0.1:8020/agui/run" `
  -H "Content-Type: application/json" `
  -d "{\"threadId\":\"thread-demo-1\",\"runId\":\"run-demo-2\",\"workflow\":\"ping\"}"
```

### 7.4 读取 snapshot（非流式）
```powershell
curl.exe "http://127.0.0.1:8020/apis/igi.zhanggui.io/v1/threads/thread-demo-1/snapshot"
```
