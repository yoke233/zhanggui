# 16 IGI v1：Inputs（CAS）与 ChangeSet API（本地单机实现）

> 归档说明：当前仓库主线已采用更轻量的 v1 落地（以 `FILE_STRUCTURE.md` 与 Bundle/ledger/evidence 为准），因此本文不再作为“必实现规范”。如未来需要引入 IGI，可把本文作为参考输入。

> 本文目标：把 Stage 1.7 的“复杂输入入库（CAS）+ 变更单（ChangeSet）+ 线程暂停请求（PAUSE_REQUESTED）”落成 **可调用的 v1 API**。  
> 口径：IGI（`apiVersion: igi.zhanggui.io/v1`）是 **真相源**；UI/前端事件承载仍走 AG-UI（见 `docs/11_ag_ui_integration.md`、`docs/archive/igi/14_igi_thread_api_v1.md`）。  
> 兼容：path 与 SSE event name 均可配置；客户端必须以 JSON 字段（`apiVersion/kind/type`）判定语义并忽略未知字段。

---

## 0) Base Path

默认 base path：

```text
/apis/igi.zhanggui.io/v1
```

---

## 1) Inputs：入库与清单（CAS + ArtifactManifest）

### 1.1 设计原则（v1）

- **文件内容入库为 CAS**：按 `sha256` 去重，落盘路径固定为：`fs/threads/{thread_id}/inputs/files/{sha256}`。  
- **输入清单是资源**：`fs/threads/{thread_id}/inputs/manifest.json` 保存 `kind=ArtifactManifest`（single-writer：系统）。  
- **run 不携带二进制**：run/state 中只出现引用（digest/ref），不塞文件内容。

### 1.2 上传文件（multipart）

```text
POST {base}/threads/{threadId}/inputs/upload
Content-Type: multipart/form-data
```

表单字段：
- `file`（必填）：上传文件
- `title`（可选）：显示标题
- `role`（可选）：用途角色（如 `context|attachment|reference`）
- `notes`（可选）：备注
- `required`（可选）：`true|false`
- `source`（可选）：`user_upload|ui_pick_file|system_generated`（默认 `user_upload`）

响应（示意）：

```json
{
  "ok": true,
  "digest": "sha256:...",
  "storedPath": "inputs/files/<sha256>",
  "sizeBytes": 123,
  "mime": "application/pdf",
  "artifactManifest": { "apiVersion":"igi.zhanggui.io/v1","kind":"ArtifactManifest", "...": "..." }
}
```

### 1.3 追加 URL 引用（JSON）

```text
POST {base}/threads/{threadId}/inputs/url
Content-Type: application/json
```

请求（示意）：

```json
{
  "url": "https://example.com",
  "title": "索引页",
  "role": "reference",
  "notes": "先不抓取正文，只做可追溯引用",
  "required": false,
  "source": "user_url"
}
```

响应：返回更新后的 `artifactManifest`。

### 1.4 读取 inputs manifest（调试用）

```text
GET {base}/threads/{threadId}/inputs/manifest
```

返回：`kind=ArtifactManifest`。

---

## 2) ChangeSet：追加需求/变更包 + 暂停请求

### 2.1 创建 ChangeSet（JSON）

```text
POST {base}/threads/{threadId}/changesets
Content-Type: application/json
```

请求（最小字段）：
- `message`（必填）：用户变更描述
- `inputRefs`（可选）：引用 inputs（通常引用 `digest/ref`）
- `requestedControl`（可选）：默认 `{mode:"drain_step",reason:"change_request"}`

请求（示意）：

```json
{
  "message": "新增约束：交付件必须包含 X；并补充这份 PDF 作为依据",
  "inputRefs": [
    { "kind": "artifact", "ref": "inputs/files/<sha256>", "digest": "sha256:<sha256>" },
    { "kind": "url", "ref": "https://example.com" }
  ],
  "requestedControl": { "mode": "drain_step", "reason": "change_request" }
}
```

行为（v1）：
1) 写入变更单文件：`fs/threads/{threadId}/changesets/{changeSetId}.json`（新建文件，可追溯）
2) 更新 `ThreadSnapshot`：
   - `Thread.status.lastChangeSetId = {changeSetId}`
   - `spec.recentChangeSets` 头插入（v1 仅保留最近 20 条）
   - 若 `requestedControl.mode=drain_step`：`Thread.status.phase = PAUSE_REQUESTED`
3) thread watch 广播 `STATE_DELTA`（整体替换 `recentChangeSets`；并更新 phase）

响应（示意）：

```json
{
  "ok": true,
  "changeSet": { "apiVersion":"igi.zhanggui.io/v1","kind":"ChangeSet", "...": "..." }
}
```

### 2.2 读取 ChangeSet（调试用）

```text
GET {base}/threads/{threadId}/changesets/{changeSetId}
```

### 2.3 列出最近 ChangeSet（调试用）

```text
GET {base}/threads/{threadId}/changesets
```

---

## 3) 最小端到端演示（pwsh；不要求 UI）

> 前提：Windows 上请使用 `curl.exe`（不要用 PowerShell 的 `curl` alias）。

### 3.1 启动服务

```powershell
go run .\cmd\zhanggui\main.go serve `
  --http-addr 127.0.0.1 `
  --http-port 8020 `
  --runs-dir fs/runs `
  --threads-dir fs/threads `
  --igi-base-path /apis/igi.zhanggui.io/v1 `
  --igi-event-name igi
```

### 3.2 创建 thread（最小 run：`workflow=ping`）

```powershell
curl.exe -N -X POST "http://127.0.0.1:8020/agui/run" `
  -H "Content-Type: application/json" `
  -d "{\"threadId\":\"thread-demo-1\",\"runId\":\"run-demo-1\",\"workflow\":\"ping\"}"
```

### 3.3 订阅 thread watch

```powershell
curl.exe -N "http://127.0.0.1:8020/apis/igi.zhanggui.io/v1/threads/thread-demo-1/watch"
```

### 3.4 上传一个文件（示例：PDF）

```powershell
curl.exe -sS -X POST "http://127.0.0.1:8020/apis/igi.zhanggui.io/v1/threads/thread-demo-1/inputs/upload" `
  -F "file=@C:\\path\\to\\demo.pdf" `
  -F "title=demo.pdf" `
  -F "role=attachment" | Out-String
```

### 3.5 追加一个 URL ref

```powershell
curl.exe -sS -X POST "http://127.0.0.1:8020/apis/igi.zhanggui.io/v1/threads/thread-demo-1/inputs/url" `
  -H "Content-Type: application/json" `
  -d "{\"url\":\"https://example.com\",\"title\":\"example\",\"role\":\"reference\"}" | Out-String
```

### 3.6 创建 ChangeSet，并观察 watch 中的 `PAUSE_REQUESTED`

```powershell
curl.exe -sS -X POST "http://127.0.0.1:8020/apis/igi.zhanggui.io/v1/threads/thread-demo-1/changesets" `
  -H "Content-Type: application/json" `
  -d "{\"message\":\"追加需求：请暂停并应用新资料\",\"requestedControl\":{\"mode\":\"drain_step\",\"reason\":\"change_request\"}}" | Out-String
```

watch 侧应出现：
- `STATE_DELTA`：`/spec/thread/status/phase = PAUSE_REQUESTED`
