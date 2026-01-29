# 09 Go 本地单跑执行器开发计划（纯本地 + 沙箱 + 落盘 + zip）

> 本文件是一个 **Go（Golang）实现导向**的开发计划：把“纯本地单跑 + 沙箱执行 + 产物文件夹落盘 + 最后 zip 上传”的形态，拆成可交付、可验收的里程碑。  
> 约束：不引入复杂工作流引擎；不引入 RBAC/权限库；以文件系统边界与 manifest 白名单替代权限系统；日志用 slog；CLI 用 cobra；配置用 viper。  
> 注意：本文是计划与协议（MVP 优先），**不包含代码实现**。

---

## 0) 范围与非目标（先写清楚）

### 0.1 目标（必须）
- 单机执行：一次 `run` 完整跑完一个任务（Task Run），产物落盘到一个任务目录。
- 沙箱边界：沙箱内只允许读/写被授权的 workspace；任何写入必须落在任务目录的允许范围内。
- 产物协议：每个阶段写哪些文件、如何避免覆盖、如何验收（VERIFY 阶段强校验）。
- 结束打包：在沙箱外生成 `manifest` 白名单并打 zip（可选上传，但上传不作为 MVP 阶段必须实现）。

### 0.2 非目标（MVP 不做）
- 不做数据库、不做分布式调度、不做多机并发。
- 不做复杂权限/身份体系（RBAC/OPA 等）。
- 不做重型检索索引（先用目录遍历/rg 风格扫描；索引后置）。
- 不做“自动继续跑”工作流引擎（但状态必须可落盘，便于人工诊断/重跑）。

---

## 1) 总体形态（你要做出来的产品长什么样）

### 1.1 CLI（建议命令集）
- `run`：创建任务目录 → 沙箱执行 → VERIFY → PACK（→ 可选 UPLOAD）
- `inspect`：读取任务目录，打印当前状态机状态、失败原因、产物清单
- `pack`：对已有任务目录重新 VERIFY + 重新生成 manifest + 重新打包 zip（不进沙箱）
- `replay`（可选后置）：对已有任务目录重放/再验收（本质=inspect+verify）

### 1.2 配置来源（viper）
- 默认：`~/.taskctl/config.yaml`
- 覆盖：环境变量（如 `TASKCTL_SANDBOX_MODE=docker`）
- 覆盖：CLI flags（run/pack/inspect 各自 flags）

### 1.3 日志（slog）
- 统一字段：`run_id` / `task_id` / `step` / `attempt` / `sandbox_mode`
- 输出：stderr + `task_dir/logs/run.log`（建议；stdout 保持机器可读输出）
- 审计：`task_dir/logs/tool_audit.jsonl`（jsonl；写入动作可追溯，见 `docs/10_tool_gateway_acl.md`）
- 约束：日志与产物分离；VERIFY/PACK 不读取沙箱里的日志作为“证据”，只当调试信息。

---

## 2) 目录与产物规范（最小可执行）

> 约定：沙箱执行产物按 revision（rN）落在 `revs/rN/`（append-only）；每次 `VERIFY + PACK` 生成一个新的审计 Bundle（`pack_id`），落在 `packs/{pack_id}/`（不可变）。  
> 任务目录根路径由 `--base-dir` 控制；默认可以指向任意本地目录（实现默认 `./fs/taskctl/`）。

### 2.1 任务目录结构（必须）
```text
{base_dir}/
  {task_id}/
    task.json                # 任务元信息（输入、参数、沙箱配置、创建时间）
    state.json               # 状态机落盘（step 状态、开始结束时间、错误摘要）
    logs/
      run.log
      tool_audit.jsonl       # 追加式审计日志（Tool Gateway 写）
    revs/
      r1/
        summary.md           # 最小必交（示例，可按你的任务类型改）
        issues.json          # 最小必交（无问题可空数组，但文件必须存在）
        artifacts/           # 该 rev 的附加产物（可选）
    packs/
      {pack_id}/             # 审计单元（Bundle；不可变；详见 docs/proposals/audit_acceptance_ledger_v1.md）
        ledger/events.jsonl  # 审计/验收账本（append-only）
        evidence/files/...   # 证据库（create-only；内容寻址）
        verify/report.json   # 验收报告（create-only）
        artifacts/manifest.json # 产物清单（create-only；路径→sha256/size）
        pack/artifacts.zip   # 产物包（create-only；严格白名单）
        pack/evidence.zip    # 证据包（create-only；默认嵌套包含 artifacts.zip）
        logs/tool_audit.jsonl # 本次打包写入审计（append-only）
    pack/                    # latest 指针/快捷入口（可覆盖；不作为审计依据）
      latest.json            # { pack_id, task_id, rev, created_at, paths... }
      artifacts.zip          # 可选：最新产物包副本
      evidence.zip           # 可选：最新证据包副本
      manifest.json          # 可选：最新 manifest 副本
    verify/                  # latest 指针（可选）
      report.json            # 可选：最新报告副本（审计引用仍走 sha256 ref）
```

### 2.2 `task.json`（最小 schema，必须）
```json
{
  "schema_version": 1,
  "task_id": "0195d8a2-4c3b-7f12-8a3b-123456789abc",
  "run_id": "0195d8a2-4c3b-7f13-8a3b-9876543210fe",
  "created_at": "2026-01-28T09:00:00+08:00",
  "tool_version": "0.1.0",
  "sandbox": {
    "mode": "docker",
    "image": "your-image:latest",
    "network": "none",
    "timeout_seconds": 900
  },
  "workspace": {
    "input_ro_paths": ["D:/data/input"],
    "output_rw_path": "{task_dir}/revs/r1"
  },
  "params": {
    "entrypoint": ["your-binary", "arg1"]
  }
}
```

### 2.3 `state.json`（最小 schema，必须）
```json
{
  "schema_version": 1,
  "task_id": "0195d8a2-4c3b-7f12-8a3b-123456789abc",
  "run_id": "0195d8a2-4c3b-7f13-8a3b-9876543210fe",
  "status": "RUNNING",
  "current_step": "SANDBOX_RUN",
  "steps": [
    { "name": "INIT", "status": "DONE", "started_at": "...", "ended_at": "..." },
    { "name": "SANDBOX_RUN", "status": "RUNNING", "started_at": "..." }
  ],
  "last_error": {
    "code": "E_SANDBOX_TIMEOUT",
    "message": "sandbox run timed out",
    "hint": "increase timeout_seconds or reduce workload",
    "occurred_at": "..."
  }
}
```

### 2.4 `issues.json`（最小 schema，必须）
```json
{
  "schema_version": 1,
  "task_id": "0195d8a2-4c3b-7f12-8a3b-123456789abc",
  "rev": "r1",
  "issues": [
    {
      "severity": "blocker",
      "where": "verify",
      "what": "missing required file summary.md",
      "action": "produce summary.md in rev folder"
    }
  ]
}
```

### 2.5 `artifacts/manifest.json`（白名单，PACK 阶段生成，必须）
```json
{
  "schema_version": 1,
  "task_id": "0195d8a2-4c3b-7f12-8a3b-123456789abc",
  "rev": "r1",
  "generated_at": "2026-01-28T09:30:00+08:00",
  "files": [
    {
      "path": "revs/r1/summary.md",
      "sha256": "...",
      "size": 1234
    },
    {
      "path": "revs/r1/issues.json",
      "sha256": "...",
      "size": 456
    }
  ]
}
```

**manifest 规则（必须）**
- `path` 必须是相对 `{task_dir}` 的相对路径，禁止绝对路径。
- 生成 manifest 时必须拒绝：
  - 路径逃逸（`..` 等）
  - 符号链接指向任务目录外（如 OS 支持）
  - 不在允许前缀内的文件（默认只允许 `revs/{rev}/**` + `task.json` + `state.json`）

### 2.6 `pack/latest.json`（latest 指针；可覆盖，不作为审计依据）
`pack/latest.json` 只用于“快速定位最新 pack_id”，允许覆盖写、可重建。审计复核以 `packs/{pack_id}/ledger/events.jsonl` + `refs.sha256` 为准。

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

---

## 3) 状态机（最小 4 steps）与转移表

### 3.1 Step 列表（MVP）
- `INIT`：创建任务目录 + 写 task.json/state.json + 选择本次 rev（例如 r1）
- `SANDBOX_RUN`：在沙箱内运行，产出写入 `revs/rN/`（append-only/new-file）
- `VERIFY`：在沙箱外验收（schema 校验、必要文件齐全、路径白名单、越界检测）
- `PACK`：生成 `pack_id` Bundle（ledger/report/manifest）+ 产物 zip + 证据包（默认嵌套包含 artifacts.zip）
- `UPLOAD`（可选后置）：上传 zip（不在 MVP 必须范围内）

### 3.2 状态转移（必须写死）
```text
INIT -> SANDBOX_RUN -> VERIFY -> PACK -> (UPLOAD)

任一步 FAIL：
  - 写 state.json（status=FAILED，last_error 填充）
  - 写 issues.json（若已进入 VERIFY/PACK）
  - 不允许进入 PACK/UPLOAD
```

### 3.3 幂等原则（必须）
- `INIT`：若任务目录已存在且包含 task.json/state.json → 拒绝覆盖（要求新 task_id 或显式 `--force`，MVP 可不提供 --force）。
- `SANDBOX_RUN`：禁止覆盖已有 `revs/rN/`；重跑必须生成 `r(N+1)`（或要求清理目录）。
- `VERIFY`/`PACK`：允许重复执行；每次必须生成新的 `pack_id`（不可变 Bundle 写入 `packs/{pack_id}/`）；`pack/latest.json` 可覆盖更新为最新。

---

## 4) 沙箱与边界（“不需要权限库”的前提）

### 4.1 核心原则（必须）
- **沙箱内可写路径**必须只映射到 `{task_dir}/revs/rN/`（或更小），其他一律只读或不挂载。
- 所有写入都要做路径归一化与前缀校验（即使沙箱已限制，也要在外层再做一次）。
- PACK 阶段在沙箱外做，并且只打包 manifest 白名单。

### 4.2 Go 侧抽象（建议）
- `SandboxRunner` 接口：
  - 输入：task.json（含 image/timeout/mounts/entrypoint）
  - 输出：exit_code、stdout/stderr（可选）、耗时、失败分类
- 两个实现（建议）：
  - `DockerRunner`（默认）：通过 `docker run ...` 做隔离（最贴近“workspace 映射”）
  - `LocalRunner`（开发模式）：不隔离，但仍强制写入路径检查（仅用于调试）

### 4.3 路径逃逸防护（必须）
- 所有用户/配置/沙箱返回的路径都必须走：
  - `Clean` + `Abs` + `Rel`（相对 task_dir）三段校验
- 任何写入必须满足：
  - `rel` 不以 `..` 开头
  - `rel` 不包含 `..` 片段
  - `rel` 前缀在允许集合内（默认 `revs/{rev}/`）

---

## 5) VERIFY（强校验清单，MVP 必须）

### 5.1 必要文件（可配置，但 MVP 固定）
- `revs/{rev}/summary.md`（必须存在）
- `revs/{rev}/issues.json`（必须存在；允许 issues 为空数组）

### 5.2 结构校验（必须）
- `issues.json`：JSON 可解析，字段齐全，severity 只允许 `blocker|warn|info`
- `task.json`/`state.json`：schema_version 正确，task_id/run_id 一致

### 5.3 安全校验（必须）
- 产物文件必须全部落在 `revs/{rev}/`（或白名单允许的路径内）
- 不允许把输入目录（input_ro_paths）中的文件复制到 pack 白名单（除非显式 allowlist，后置）

### 5.4 审计/验收产出（v1 建议）
为建立可复核证据链（Bundle 化），每次 `VERIFY + PACK` 建议额外产出：
- `packs/{pack_id}/ledger/events.jsonl`：审计/验收账本（append-only；记录关键事实与 refs）
- `packs/{pack_id}/evidence/files/{sha256}`：冻结验收标准与结构化证据（create-only；内容寻址）
- `packs/{pack_id}/verify/report.json`：验收报告（create-only；引用 criteria 快照与证据 refs）

验收标准来源建议固定为 `docs/proposals/acceptance_criteria_v1.yaml`，但**每次打包必须快照冻结**（以 sha256 ref 绑定），避免 docs 变更破坏审计复核（详见 `docs/proposals/audit_acceptance_ledger_v1.md`）。

---

## 6) PACK（白名单打包规则，MVP 必须）

### 6.1 manifest 生成（必须）
- 只枚举允许打包的路径前缀（默认 `revs/{rev}/` + 必要的元文件）
- 对每个文件计算 sha256/size（用于上传后校验与审计）

### 6.2 zip 打包（必须）
- zip 内路径必须与 manifest.path 一致（相对路径）
- zip 生成必须拒绝：
  - manifest 中缺失的文件
  - manifest 外的文件被打入 zip（必须不可能发生）

### 6.3 Bundle 与证据包（v1 建议）
每次 PACK 必须生成新的 `pack_id`，并将产物落在 `packs/{pack_id}/`（不可变）：
- `packs/{pack_id}/artifacts/manifest.json`
- `packs/{pack_id}/pack/artifacts.zip`
- `packs/{pack_id}/pack/evidence.zip`（默认嵌套包含 `pack/artifacts.zip`，不展开）

完成后更新 `pack/latest.json` 指向最新 `pack_id`（latest 允许覆盖写，但不作为审计依据）。

---

## 7) Go 工程结构（建议落地组织）

```text
cmd/
  taskctl/
    main.go                 # cobra root
internal/
  cli/                      # cobra 子命令：run/inspect/pack
  config/                   # viper 加载与默认值
  taskdir/                  # 任务目录创建、路径校验、rev 生成
  state/                    # state.json 读写、状态机转移
  sandbox/                  # SandboxRunner + DockerRunner/LocalRunner
  verify/                   # VERIFY 规则集合
  manifest/                 # manifest 生成与校验
  pack/                     # zip 打包
  logging/                  # slog 规范化（字段、文件输出）
```

> 说明：MVP 不要求强 DDD/Clean Architecture，但必须把 “路径校验/manifest/verify” 独立出来，避免被 CLI 逻辑污染。

---

## 8) 里程碑（按 1~2 天 MVP 排）

### M0（半天）：脚手架与命令骨架
**交付**
- cobra 子命令：`run`/`inspect`/`pack` 空实现（只解析参数）
- viper 配置加载（config file + env + flags）
- slog 统一日志字段（stderr + 文件；stdout 保持机器可读）
**验收**
- `run --help` 等输出稳定
- `inspect` 能读取并打印一个 task_dir（即使字段不全也给出友好错误）

### M1（半天）：任务目录与状态机落盘
**交付**
- `task.json`/`state.json` 初始化与更新（进入每 step 必写）
- `revs/r1/` 创建策略（禁止覆盖）
**验收**
- 任何阶段崩溃后，`state.json` 能定位到 `current_step` 与 `last_error`

### M2（1 天）：SANDBOX_RUN（先 LocalRunner，DockerRunner 后补）
**交付**
- `LocalRunner`：执行一个外部命令，限制输出目录为 `revs/r1/`
- 写入路径校验：任何越界写入被拒绝（至少在 pack/verify 阶段能检测并 fail）
**验收**
- 能产出最小产物：`summary.md` + `issues.json`
- 故意写越界文件会被 VERIFY 阻断

### M3（半天）：VERIFY + PACK（白名单）
**交付**
- VERIFY：必要文件、json 校验、路径白名单
- PACK：生成 `pack_id` Bundle（ledger/report/manifest）+ `pack/artifacts.zip` + `pack/evidence.zip`（默认嵌套包含 artifacts.zip）
**验收**
- 没有通过 VERIFY 时不生成 zip
- `pack/artifacts.zip` 内容完全等于 manifest.files

### M4（后置）：DockerRunner + UPLOAD
**交付**
- DockerRunner：workspace ro/rw 映射、timeout、资源限制（尽可能）
- UPLOAD：对接你们的上传端（HTTP/S3/自定义）
**验收**
- 沙箱内无法写入任务目录外
- 上传前后 sha256 校验一致

---

## 9) 风险清单（必须提前写）

- 路径逃逸：`..`、绝对路径、符号链接（需在 verify/pack 双重兜底）
- “一次运行”中断：必须保证 state 可诊断、revs 不覆盖
- 产物格式漂移：VERIFY 规则要足够硬（缺文件/字段直接 blocker）
- zip 泄露：只从 manifest 生成 zip，永不“遍历整个任务目录打包”

---

## 10) Thread 协作控制面（Go 实现计划，正式开发入口）

> 说明：从“demo 能跑”进入“正式开发”时，建议优先落地 Thread 协作控制面。  
> IGI 相关文档已归档：`docs/archive/igi/README.md`（当前主线 v1 不要求实现 IGI 资源模型）。

### T0（半天）：数据模型与落盘骨架
**交付**
- `fs/threads/{thread_id}/state.json`（ThreadSnapshot 或至少 Thread）落盘与更新（single-writer：系统）
- `fs/threads/{thread_id}/events/events.jsonl`（append-only）写入（至少记录：actor + subject + patch）
**验收**
- 给定 thread_id，能创建目录并写入一份最小快照；重复写入不破坏 append-only 约束

### T1（1 天）：Thread Snapshot + Watch（SSE）
**交付**
- `GET {threads_base}/threads/{threadId}/snapshot`（base 可配置，建议默认 `/threads`）
- `GET {threads_base}/threads/{threadId}/watch`（首包 `STATE_SNAPSHOT`，后续 `STATE_DELTA`）
- `/agui/run` 与 thread 关联：run start/finish 更新 `Thread.status.activeRunId/phase`
**验收**
- 一个终端订阅 watch，另一个触发 run 或写入进度更新，watch 能收到 delta

### T2（1 天）：Materials Pack（CAS）入库（Artifact/Manifest）
**交付**
- 文件入库：计算 `sha256` 去重，落在 `fs/threads/{thread_id}/inputs/files/{sha256}`
- 生成/更新 `Artifact` 与 `ArtifactManifest`（不把二进制塞进 run/state）
- 限流参数（max_files/max_bytes/max_urls）可配置并生效
**验收**
- 上传同一文件两次不会重复存储；manifest 中能看到引用

### T3（1 天）：ChangeSet + 可控暂停（drain_step）
**交付**
- 创建 ChangeSet（引用 inputRefs），将 `Thread.phase` 设置为 `PAUSE_REQUESTED`
- run 在 step 边界收尾后 interrupt；resume 后执行最小 intake（记录 decision + 更新 thread 状态）
**验收**
- watch 能看到 `PAUSE_REQUESTED -> PAUSED`；run 结束为 interrupt；resume 后继续并产出可追溯记录

> 后置：Meeting Mode（`docs/06_meeting_mode.md`）与 Task/Gate 的实装可放到 Thread/Inputs/ChangeSet 稳定之后。
