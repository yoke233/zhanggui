# 10 Tool Gateway：文件写入 ACL + 单写者 + 审计（Stage 1 落地规范）

> 目标：把“谁能写哪里、怎么写、写了什么”从约定变成硬约束。  
> 适用范围：所有会落盘的 Agent/Runner/Assembler/Recorder 行为；包括会议系统与任务系统。  
> 落地形态：统一由 Tool Gateway 执行文件系统写入动作；策略由 Spec（MeetingSpec/TaskSpec）或运行参数生成；每次动作落审计（jsonl）。

---

## 0) 在定义协议前：先把边界写清楚（必须）

### 0.1 Gateway 的职责（必须）
Gateway 必须做到：
1) **路径边界**：所有写入必须落在允许前缀内；拒绝任何路径逃逸。
2) **动作边界**：所有写入必须显式声明动作类型（create/append/replace/mkdir/rename/delete）；默认 deny。
3) **写入语义边界**：append-only / single-writer 的规则必须被强制执行。
4) **审计**：每次动作必须写入 `tool_audit.jsonl`，包含 who/what/where/when/result/linkage（见 §4）。

### 0.2 Gateway 不负责什么（非目标）
- 不做 RBAC/用户体系；“身份”由上层（会议/任务 Spec）提供并写入审计。
- 不做复杂冲突合并；违规即拒绝并记录。
- 不依赖特定沙箱技术；即使有 Docker/VM，也仍要在外层做一次路径/动作校验。

---

## 1) 统一路径模型（必须）

### 1.1 统一根目录（root）
- Gateway 必须以一个 `root_dir` 作为边界（例如：`fs/meetings/{meeting_id}/` 或某个 `task_dir/`）。
- Gateway 只接受 **root 内的相对路径**（`rel_path`），并在内部生成 OS 绝对路径执行实际 I/O。

### 1.2 规范化（必须）
对任意 `rel_path`，Gateway 必须：
- 做 `Clean`（去掉 `.`、折叠多余分隔符）。
- 拒绝包含 `..` 片段的路径（防止路径逃逸）。
- 统一使用 `/` 作为策略匹配的分隔符（落审计也用 `/`，便于 `rg`）。

### 1.3 允许前缀匹配（必须）
- `allowed_write_prefixes` 是一组 **相对 root 的前缀**（以 `/` 分隔）。
- 当 `rel_path` 以任一前缀开头时，才允许写入。
- 约定：前缀以 `/` 结尾表示目录前缀；不以 `/` 结尾表示精确文件或前缀（实现可统一按“前缀字符串”处理，但必须在文档里固定口径）。

---

## 2) 写入动作模型（必须，默认 deny）

### 2.1 动作列表（必须）
- `create`：新建文件（若已存在则拒绝）
- `append`：追加到文件末尾（若不存在可选允许创建；但必须明确策略）
- `replace`：覆盖写（建议使用 atomic write：写临时文件 → rename）
- `mkdir`：创建目录（等价于 mkdir -p）
- `rename`：重命名（源/目标都要过 ACL 校验）
- `delete`：删除文件或空目录（默认建议禁用；需要显式允许）

### 2.2 默认策略（建议）
- 默认只允许：`create/append/replace/mkdir`
- 默认拒绝：`rename/delete`
- 任何动作被拒绝必须写审计（result=error + error_code）。

---

## 3) append-only / single-writer 规则（必须）

### 3.1 append-only（必须）
append-only 适用于“历史记录”与“证据链”文件，典型如：
- `shared/transcript.log`
- `shared/decisions.md`

**规则（必须）**
- 允许：`append`（以及可选 `create`，仅当文件不存在）
- 禁止：`replace`、`rename`、`delete`
- 违规：必须拒绝并写审计（error_code 建议为 `E_APPEND_ONLY_VIOLATION`）

### 3.2 single-writer（必须）
single-writer 适用于“共享指针/白板/队列”等允许覆盖写但禁止并行写的对象，典型如：
- `shared/whiteboard.md`
- `shared/hand_queue.json`
- `artifacts/**`（会后聚合产物）

single-writer 要同时解决两件事：
1) **谁有资格写**（角色约束：例如 only recorder）
2) **同一时刻只能一个写者**（逻辑锁）

**规则（必须）**
- 当 `rel_path` 落在 `single_writer_prefixes`（或 `single_writer_files`）内：
  - Actor 必须满足 `single_writer_roles` 之一（例如 `recorder`）。
  - Gateway 必须校验并持有锁（见 §3.3）。
- 违规：必须拒绝并写审计（error_code 建议为 `E_SINGLE_WRITER_VIOLATION` 或 `E_LOCK_NOT_HELD`）。

### 3.3 逻辑锁协议（必须，最小可执行）
> 目标：跨进程也能阻止并行写共享区（Windows/Linux 都可用）。

**锁文件位置（建议）**
- 对会议：`shared/.writer.lock`（位于 meeting 的 shared 目录内）
- 对任务：在允许覆盖写的共享目录内放置 `.writer.lock`（如有）

**获取锁（必须）**
- 使用 “创建即占有” 语义（`O_CREATE | O_EXCL`）创建锁文件；已存在则视为被占用。
- 锁文件内容（建议 JSON）至少包含：
  - `schema_version`
  - `lock_id`
  - `actor`（agent_id/role）
  - `acquired_at`
  - `purpose`（可选：meeting_id/task_id/rev）

**释放锁（建议）**
- 正常结束时删除锁文件；异常中断时可依赖人工清理或 TTL（TTL 属于后置增强）。

---

## 4) 审计（tool_audit.jsonl）（必须）

### 4.1 审计文件位置（建议）
- `.../logs/tool_audit.jsonl`

> 说明：`FILE_STRUCTURE.md` 已预留该文件名；实现时必须保持稳定，便于检索与归档。

### 4.2 审计记录 schema（最小必填）
每行一个 JSON 对象（jsonl），字段要求：

```json
{
  "schema_version": 1,
  "ts": "2026-01-28T09:00:00+08:00",
  "who": { "agent_id": "recorder-01", "role": "recorder" },
  "what": { "action": "append", "tool": "fs.write", "detail": "append transcript speak block" },
  "where": { "path": "shared/transcript.log" },
  "result": { "status": "ok", "error_code": "", "error": "" },
  "linkage": { "thread_id": "t1", "meeting_id": "mtg-000001", "task_id": "", "run_id": "", "rev": "" }
}
```

**约束（必须）**
- `path` 必须为相对 root 的 `/` 分隔路径（便于 `rg -n` 检索）。
- `detail` 必须脱敏（不得写入 secrets/PII；必要时只写摘要）。
- 失败也必须记录（status=error）。

---

## 5) 从 Spec 到 Gateway Policy 的映射（必须）

### 5.1 MeetingSpec → Policy（必须）
来自 `docs/06_meeting_mode.md` 的 MeetingSpec 关键字段：
- `policy.allowed_write_prefixes`
- `policy.append_only_files`
- （建议新增/扩展）`policy.single_writer_prefixes` 与 `policy.single_writer_roles`

映射规则：
- `allowed_write_prefixes` 直接作为 Policy 的写入前缀集合。
- `append_only_files` 作为 append-only 文件列表（相对 meeting root）。
- `shared/**`、`artifacts/**` 等 single-writer 目录应由 meeting type 固定生成，或由 policy 显式声明。

### 5.2 TaskSpec/RunSpec → Policy（建议）
任务执行目录（如 `{task_dir}/`）建议默认：
- `allowed_write_prefixes`：`["task.json","state.json","logs/","revs/","pack/"]`
- `append_only_files`：可为空（或将 `logs/tool_audit.jsonl` 视为 append-only）
- `single_writer_prefixes`：可为空（单进程场景可不启用锁）

---

## 6) 违规用例清单（必须被拒绝）

> 这些用例用于 Stage 1 验收：实现必须能稳定拒绝，并落审计记录。

### 6.1 路径逃逸（必须拒绝）
- `../secrets.txt`
- `shared/../../deliver/final.md`

### 6.2 越权写共享区（必须拒绝）
参与者（非 recorder）尝试：
- `replace shared/whiteboard.md`
- `append shared/transcript.log`
- `create artifacts/export_minutes.md`

### 6.3 append-only 覆盖写（必须拒绝）
Recorder 尝试：
- `replace shared/transcript.log`
- `delete shared/decisions.md`

### 6.4 single-writer 未持锁写入（必须拒绝）
Recorder 未获取锁时尝试：
- `replace shared/whiteboard.md`
- `replace shared/hand_queue.json`

### 6.5 rename/delete 默认禁用（建议拒绝）
任何角色尝试：
- `rename shared/whiteboard.md -> shared/whiteboard_old.md`
- `delete artifacts/export_minutes.md`
