# 合并的 Markdown

> 生成时间: 2026-01-29 15:59:15 +08:00
> Root: D:\xyad\company

---

## 文件名：README.md

# Multi-Agent 协作系统工作区（精简版）

> 版本：Slim 0.1  
> 日期：2026-01-28  
> 目标：先冻结“可执行的最小规范”（`docs/**`），再按 `docs/08_development_plan.md` 逐阶段落地实现。当前已包含 Go MVP：`taskctl` + `zhanggui`（AG-UI SSE demo）。

## 你现在应该先看哪里
1. `FILE_STRUCTURE.md`（权威：项目文件结构 + 写入/权限；任务系统与会议系统统一存放）
2. `docs/01_minimal_kernel.md`（最关键：最小内核 + extensions + 渐进式交付）
3. `docs/02_planning_and_parallelism.md`（怎么拆任务并行、分身、配额与调度）
4. `docs/03_artifact_pipeline.md`（多交付物与强协议：Transformer/Adapter/Renderer/Verifier）
5. `docs/04_walkthrough_report_ppt.md`（一次完整纸面演练：报告 + PPT）
6. `docs/06_meeting_mode.md`（会议模式 v2：发言串行 + 思考并行、单写者、锚点协议）
7. `ROADMAP.md`（路线图：从 demo 到正式开发的里程碑）

## 文件清单
- `docs/00_scope_and_principles.md` —— 范围、原则、我们要解决的痛点
- `docs/01_minimal_kernel.md` —— Master IR 最小内核 + extensions + 动态验收 + 渐进式交付
- `docs/02_planning_and_parallelism.md` —— Delivery Plan、MPU 拆分、spawn、并行资源池与“谁能拿走”
- `docs/03_artifact_pipeline.md` —— 交付物类型、强协议节点、插件契约、口径一致性与 issue_list
- `docs/04_walkthrough_report_ppt.md` —— 纸面跑通：report + ppt（产物、目录、rg 筛选点）
- `docs/99_templates.md` —— 最少模板：summary/cards/full + issue_list + deliver_plan（可选用）

> 说明：这版刻意“少文档”。后续若要扩展，再拆分成更多章节。

新增：
- `docs/05_user_interaction.md`：用户中途指令处理硬流程（change_request/replan/terminate/major_restart）
- `docs/06_meeting_mode.md`：会议模式 v2（上下文工程版：规范 + 协议 + 文件写入约束）
- `docs/07_convergence_gates.md`：收敛门 Gate Node（把“会议式收敛”嵌入任务流水线）
- `docs/08_development_plan.md`：多阶段落地开发计划（语言无关）
- `docs/09_golang_development_plan.md`：Go 本地单跑执行器开发计划（沙箱 + 落盘 + zip）
- `docs/10_tool_gateway_acl.md`：Tool Gateway（写入 ACL + 单写者 + 审计）落地规范（Stage 1）
- `docs/11_ag_ui_integration.md`：前端 AI 界面对接（AG-UI：events/tools/interrupt-resume 草案）
- `docs/12_runtime_and_input_model.md`：运行时与输入模型（v1：协程 Agent + 可控暂停开关 + 输入落盘）
- `docs/13_igi_v1_resource_model.md`：IGI v1（`apiVersion: igi.zhanggui.io/v1`）：资源模型（真相源）与通过 AG-UI 承载的映射规则

文件系统存储区（运行态数据）：`fs/**`（目录结构以 `FILE_STRUCTURE.md` 为准）

---

## 文件名：AGENTS.md

# Repository Guidelines

## Project Structure & Module Organization
- `cmd/`: executable entrypoints (`taskctl`, `zhanggui`).
- `internal/`: Go implementation (AG-UI handler, IGI APIs, sandbox runners, CLI commands).
- `contracts/`: versioned integration contracts and JSON schemas (`contracts/igi/v1`, `contracts/ag_ui`).
- `docs/`: spec-first design docs (start with `docs/README.md` and `FILE_STRUCTURE.md`).
- `fs/`: runtime data (ignored by git); do not commit.

## Build, Test, and Development Commands
- `go test ./...`: run all tests.
- `go test ./... -run TestName -count=1`: run a focused test without cache.
- `go build ./cmd/taskctl`: build the `taskctl` CLI.
- `go build ./cmd/zhanggui`: build the `zhanggui` server.
- `go run ./cmd/zhanggui serve --print-endpoints`: start local AG-UI + IGI demo server.
- `go run ./cmd/taskctl --help`: explore `run`, `inspect`, and `pack`.

## Coding Style & Naming Conventions
- Format with `gofmt` (`go fmt ./...`) before pushing (Go indentation is tabs).
- Follow standard Go naming: `CamelCase` exports, `mixedCase` locals, `lowercase` packages.
- Keep new code under `internal/<area>/`; add a new `cmd/<tool>/main.go` only for new binaries.

## Testing Guidelines
- Tests live next to code as `*_test.go`; use `TestXxx` naming.
- Prefer black-box tests (`package foo_test`) when validating public behavior (see `internal/agui`).

## Commit & Pull Request Guidelines
- Git history is currently minimal; use clear, scoped messages (e.g., `feat(taskctl): add pack flag`).
- PRs should include: what/why, how to test, linked issue/doc section, and updates to `contracts/` or `docs/` when behavior/protocol changes.

## Security & Configuration Tips
- Never commit secrets or runtime outputs (`.env*`, `fs/`, logs).
- Config loads via `--config` or `config.yaml` from `.` / `~/.taskctl` / `~/.zhanggui`; env prefixes are `TASKCTL_` and `ZHANGGUI_`.

---

## 文件名：CHANGELOG.md

# Changelog

## meeting_v3
- 新增：docs/07_convergence_gates.md（把会议式收敛抽象为 Gate Node，嵌入任务流水线）
- 更新：docs/01_minimal_kernel.md 引入 Gate Node 概念
- 更新：docs/03_artifact_pipeline.md 补充 Gate 与流水线关系

---

## 文件名：FILE_STRUCTURE.md

# 文件结构（单项目：任务系统 + 会议系统）

> 目标：把“任务系统（case/tasks/deliver）”与“会议系统（meetings）”的**所有可追溯文件**放在同一个项目里，并且在**基于文件系统**的前提下，做到：
> - **不并行改同一份共享文件**（避免冲突、避免口径漂移）
> - 允许通过 **append / 新增文件** 的方式“叠加”产出（高并行、可回放）
> - 少数“可维护可编辑”的文件，只有少数角色/人员有权限

本文件是**唯一权威**的目录结构与写入权限约定；其他文档如有冲突，以此为准。

---

## 1) 顶层分区（强约束）

本项目分为以下几类内容（避免“运行态数据”和“实现代码/契约”混在一起）：

- `docs/**`：规范与设计文档（少数维护者可编辑；避免多人并行改同一文件）
- `contracts/**`：对接契约与 schema（JSON/YAML；版本化；用于前后端/工具对接；例如 `contracts/ag_ui/*`、`contracts/igi/v1/*`）
- `cmd/**`、`internal/**`、`go.mod`：实现代码（当前包含 Go MVP：`taskctl`）
- `fs/**`：文件系统存储区（运行态数据：case、task、meeting、run、archive；**不入 git**）

> 说明：`fs/` 下默认采用“**只追加/只新建**”的写策略；需要覆盖写的共享文件必须遵守单写者原则。  
> 另：`fs/**` 属于运行态数据，应通过 `.gitignore` 排除，避免污染仓库历史。

---

## 2) 目录结构（权威树）

```text
./
  README.md
  FILE_STRUCTURE.md
  CHANGELOG.md
  contracts/                        # 对接契约与 schema（版本化）
  cmd/                              # 可执行入口（当前：taskctl）
  internal/                         # Go 实现（可替换为其他语言实现）
  go.mod
  go.sum
  docs/                             # 规范文档（维护者可编辑）
  fs/                               # 文件系统存储区（运行态数据）
    cases/
      {case_id}/
        current.yaml                # 指针：当前 major（由系统/Planner 单写者维护）
        versions/
          v1/
            spec.md                 # Master IR / 需求与验收（可编辑：Planner/Editor）
            tasks/
              task-000123/
                spec.yaml           # TaskSpec（程序生成/维护）
                current.yaml        # 指针：当前 revision（程序维护）
                revs/
                  r1/
                    summary.md      # 必交（Agent 写；新建，不覆盖）
                    cards.md        # 按需（Agent 写；新建，不覆盖）
                    issues.md       # 可选（Agent/Verifier 写；新建，不覆盖）
                  r2/
                    ...
                review.md           # 审核记录（Editor/Verifier 追加或单写者维护）
            deliver/                # 最终交付物（Editor 单写者）
              report.md
              slides.md
              manifest.yaml
              coverage_map.yaml
              issue_list.md
            decisions/              # 决策日志（Planner/Editor 追加）
              decisions.log

    meetings/
      {meeting_id}/
        spec.yaml                   # MeetingSpec（程序生成/主持人维护）
        shared/                     # 共享区：单写者（Recorder/Moderator）
          transcript.log            # 追加式
          whiteboard.md             # 允许覆盖写（单写者）
          hand_queue.json           # 可选：举手队列/发言序列（单写者维护）
          decisions.md              # 追加式
          compaction/
            snap_0001.md
        agents/                     # 参与者私有区：各自写各自目录（append/new only）
          {agent_id}/
            inbox/                  # 单写者投递给该 agent（追加）
            outbox/
              turn_0001.md          # 该 agent 本轮产出（新建文件）
            vault/
              notes_*.md            # 私有草稿/证据片段（追加）
        events/                     # 可选：事件流事实层（JSONL 分片，无共享写）
          agent_{agent_id}.jsonl
          recorder.jsonl
        artifacts/                  # 会后产物：单写者聚合生成
          export_minutes.md
          action_items.yaml
          citations.yaml

    threads/
      {thread_id}/
        state.json                  # Thread 公共状态快照（single-writer：系统）
        events/
          events.jsonl              # Thread 公共事件（append-only）
        logs/
          tool_audit.jsonl          # 追加式审计日志（系统写；Tool Gateway 写）
        inputs/
          manifest.json             # 输入清单（系统维护；建议 single-writer + events 追加记录；对齐 IGI kind=ArtifactManifest）
          files/
            {sha256}                # 原始文件（不入 git；文件名=sha256 便于去重）
          snapshots/
            {input_id}.md           # URL 抓取/提取后的快照（可选）
        changesets/
          {changeset_id}.json        # 变更单（kind=ChangeSet；新建文件；可回放/追溯）
        control/
          state.json                # 暂停/恢复意图与一致性快照（single-writer：系统）
          events.jsonl              # 控制事件日志（append-only）

    runs/
      {run_id}/
        run.json                  # Run 元信息（对外协议/线程/父子 run；程序生成）
        state.json                # Run 状态（tool/interrupt 等；程序维护）
        events/
          events.jsonl            # 事件流落盘（append-only；可用于重连/回放）
        ledger/
          events.jsonl            # 审计/验收账本（append-only；与 events/ 分工，见 docs/proposals/audit_acceptance_ledger_v1.md）
        evidence/
          files/
            {sha256}              # 证据文件（create-only；内容寻址；不入 git）
        verify/
          report.json             # 验收报告（create-only；审计引用以 sha256 ref 为准）
        artifacts/
          manifest.json           # 产物清单（create-only；路径→sha256/size）
        pack/
          artifacts.zip           # 产物包（create-only；严格白名单）
          evidence.zip            # 证据包（create-only；默认嵌套包含 artifacts.zip）
        gate/
          {gate_id}/
            position_packets/
              position_packet_{agent_id}.md
            gate_decision.md        # 单写者（Moderator/Editor/Planner）
        logs/
          tool_audit.jsonl          # 追加式审计日志（系统写）

    taskctl/                         # Go MVP：本地单跑任务目录（不入 git；可随时清理）
      {task_id}/
        task.json                    # 任务元信息（输入、参数、沙箱配置、创建时间）
        state.json                   # 状态机落盘（step 状态、开始结束时间、错误摘要）
        logs/
          run.log
          tool_audit.jsonl           # 追加式审计日志（Tool Gateway 写）
        revs/
          r1/
            summary.md               # 最小必交（示例，可按任务类型改）
            issues.json              # 最小必交（无问题可空数组，但文件必须存在）
            artifacts/               # 该 rev 的附加产物（可选）
        packs/
          {pack_id}/                 # 审计单元（Bundle；不可变）
            ledger/
              events.jsonl           # 审计/验收账本（append-only）
            evidence/
              files/{sha256}         # 证据文件（create-only；内容寻址）
            verify/report.json       # 验收报告（create-only）
            artifacts/manifest.json  # 产物清单（create-only）
            pack/artifacts.zip       # 产物包（create-only）
            pack/evidence.zip        # 证据包（create-only；默认嵌套包含 artifacts.zip）
            logs/tool_audit.jsonl    # 追加式审计日志（append-only）
        pack/
          latest.json                # latest 指针（single-writer；不作为审计依据）
          artifacts.zip              # 可选：最新产物包副本（可覆盖）
          evidence.zip               # 可选：最新证据包副本（可覆盖）
          manifest.json              # 可选：最新 manifest 副本（可覆盖）
        verify/
          report.json                # 可选：最新报告副本（可覆盖；审计引用仍走 sha256 ref）

    archive/                        # 归档区：不可改（只追加/只新建）
      meetings/{meeting_id}/...
      cases/{case_id}/...
```

---

## 3) 写入/编辑权限（核心规则）

### 3.1 写入类型（统一术语）

- **append-only**：只允许追加/新建；禁止覆盖旧内容（通过新版本/新文件“叠加”）
- **single-writer**：允许覆盖写，但同一时间只能有一个写者（角色/人员固定）
- **maintainer-editable**：少数维护者可编辑；要求串行合并/改动（避免多人同时改同一文件）

### 3.2 路径级规则（最小可执行）

- `docs/**`：`maintainer-editable`（少数维护者；原则上不并行改同一文件）
- `fs/cases/**/versions/**/tasks/**/revs/**`：`append-only`（Agent 输出只能“新增 rN”，不覆盖）
- `fs/cases/**/versions/**/deliver/**`：`single-writer`（Editor/Assembler）
- `fs/cases/**/versions/**/spec.md`：`single-writer`（Planner/Editor）
- `fs/cases/**/current.yaml`、`fs/cases/**/versions/**/tasks/**/current.yaml`：`single-writer`（系统/调度中心）
- `fs/meetings/**/agents/**`：`append-only`（各 agent 只写自己目录；inbox 由单写者投递）
- `fs/meetings/**/shared/**`、`fs/meetings/**/artifacts/**`：`single-writer`（Recorder/Moderator）
- `fs/threads/**/events/**`、`fs/threads/**/control/events.jsonl`：`append-only`
- `fs/threads/**/logs/**`：`append-only`
- `fs/threads/**/state.json`、`fs/threads/**/inputs/manifest.json`、`fs/threads/**/control/state.json`：`single-writer`（系统/调度中心）
- `fs/runs/**/events/**`、`fs/runs/**/ledger/**`、`fs/runs/**/logs/**`：`append-only`
- `fs/runs/**/state.json`：`single-writer`（系统）
- `fs/runs/**/gate/**/gate_decision.md`：`single-writer`（Moderator/Editor/Planner）
- `fs/taskctl/**/revs/**`、`fs/taskctl/**/packs/**`：`append-only`（Bundle 不可变；pack/verify 允许维护 latest 指针）
- `fs/taskctl/**/state.json`、`fs/taskctl/**/pack/latest.json`：`single-writer`（系统）
- `fs/archive/**`：`append-only`（归档后视为只读）

---

## 4) 版本策略（避免“覆盖写”）

- **Major（vN）**：用户/方向级推翻重来 → 新建 `versions/v(N+1)/`，更新 `fs/cases/{case_id}/current.yaml`
- **Revision（rN）**：同一 task 的返工迭代 → 新建 `revs/r(N+1)/`，更新 `tasks/{task_id}/current.yaml`

> 关键点：正文产物尽量“只新建”，共享指针文件才允许单写者覆盖写。

---

## 5) 会议系统版本

- 会议系统以 **v2** 为当前版本（上下文工程 + 发言串行/思考并行 + 单写者共享区）。
- 规范入口：`docs/06_meeting_mode.md`

---

## 文件名：ROADMAP.md

# Roadmap（zhanggui：从 demo 到正式开发）

> 目的：给贡献者与团队一个“高层路线图”（产品/能力里程碑）。  
> 细节执行计划请看：`docs/08_development_plan.md`（语言无关）与 `docs/09_golang_development_plan.md`（Go）。
>
> 状态标记：`[ ]` 未处理 / `[/]` 进行中 / `[x]` 已完成

---

## v0.1（基础可追溯：已完成）

- [x] 规范入口与索引冻结（docs/ 与 FILE_STRUCTURE）
- [x] Tool Gateway（写入 ACL + 单写者 + 审计）
- [x] AG-UI 最小对接（SSE `/agui/run` + `/agui/tool_result` + interrupt/resume demo）
- [x] IGI v1（`igi.zhanggui.io/v1`）资源模型与 contracts（schemas）落库

---

## v0.2（Thread 协作控制面：正式开发第一阶段）

目标：UI 能看到“全局一致目标（Directive）+ 全员进度（Progress Board）+ 控制状态（Pause/Resume）”，并能断线恢复。

- [ ] 实现 `ThreadSnapshot` 组装与落盘（`fs/threads/{thread_id}`）
- [ ] 实现 Thread watch（SSE）：首包 `STATE_SNAPSHOT`，后续 `STATE_DELTA`（RFC6902）
- [ ] Run 与 Thread 关联：run start/finish 推进 `Thread.status.activeRunId/phase`
- [ ] 固化前端工具 schema（`contracts/ag_ui/tools.json` 补齐 args/result）

---

## v0.3（Materials Pack：CAS + ChangeSet + 可控暂停）

目标：用户上传/追加需求（文件/PDF/URL/大量资料）先入库再变更，系统能在 step 边界“收尾后暂停”，并可恢复继续。

- [ ] Artifact 入库（CAS：按 `sha256` 去重存储）
- [ ] ArtifactManifest 维护（集合清单，run 只引用，不塞二进制）
- [ ] ChangeSet 创建与落盘（引用 inputRefs，requestedControl=drain_step）
- [ ] 可控暂停开关：`PAUSE_REQUESTED -> PAUSED -> RESUME`
- [ ] 限流与裁决：超限必须 tool call 让用户裁决

---

## v0.4（任务执行闭环：TaskSpec/verify/pack 与 IGI 对齐）

目标：把执行面从 demo 升级到真实任务流水线（verify/pack/manifest），并把关键状态回灌到 Thread。

- [ ] 任务运行与 rev 产物协议对齐（`taskctl` 与 Thread/Run/Bundle 映射）
- [ ] Verifier 最小可用（协议校验 + issue_list）
- [ ] Packager 产出 Bundle（manifest 白名单 + zip）

---

## v0.5（Meeting Mode MVP：并行提案 + 串行收敛）

目标：把 `docs/06_meeting_mode.md` 的协议落地成可回放会议目录，并能输出 action_items 注入任务流。

- [ ] meeting 目录初始化与单写者 Recorder
- [ ] proposal/speak/whiteboard/compaction 最小闭环
- [ ] 输出三件套：export_minutes/action_items/citations
- [ ] action_items → task patch（PatchSpec v1）注入与审计

---

## v0.6（扩展：多系统接入与可观测性）

目标：在不破坏 IGI/AG-UI 契约的前提下，引入更多系统/agent，提升可观测与运维能力。

- [ ] 事件元信息增强（eventId/producer/actor/subject/correlation）从“可出现”升级为“必须”
- [ ] cursor/断点续传与回放工具（thread/run）
- [ ] 更强的安全边界与脱敏/保留策略（archive/retention）


---

## 文件名：contracts/igi/v1/README.md

# IGI v1 Schemas (`apiVersion: igi.zhanggui.io/v1`)

> 目标：固化 zhanggui 的“世界定义/公司级对象模型”最小契约（JSON Schema）。  
> 对外 UI 事件仍走 AG-UI；IGI 资源通过 AG-UI 的 `STATE_SNAPSHOT/STATE_DELTA` 承载（见 `docs/13_igi_v1_resource_model.md`）。

## 文件清单
- `common.schema.json`：通用定义（Resource Envelope / metadata / actor / scope）
- `thread.schema.json`
- `directive.schema.json`
- `changeset.schema.json`
- `artifact.schema.json`
- `artifact_manifest.schema.json`
- `agent_status.schema.json`
- `bundle.schema.json`
- `thread_snapshot.schema.json`

## 兼容性规则（v1 内必须遵守）
- 新字段优先加到 `metadata.ext/spec.ext/status.ext`
- 若要新增顶层字段：必须同步更新 schema，并保持向后兼容（新增 optional 字段）
- 破坏性变更（删字段/改语义/改枚举含义）必须升 `igi.zhanggui.io/v2`

---

## 文件名：docs/00_scope_and_principles.md

# 00 范围与原则（不落库、不写代码阶段）

## 我们要解决的现实问题
- 多 Agent 并行时，最容易“看起来并行，实际串行”，最后卡在组长/主编。
- 多交付物（报告 + PPT + 合同/脚本）会出现口径不一致、重复劳动、强协议输出难控（JSON→HTML/PPTX）。
- 上下文膨胀：组长合并时 token 爆炸、跑偏、输出截断。
- 权限与隔离：不同 agent 不应互写文件；渲染器/工具不应篡改语义。

## 本阶段的边界
- 不做数据库设计、不做后端实现。
- 只讨论：最小规范、交互协议、产物格式、并行/调度策略。
- 一切以“文件系统约定 + 工具网关边界”作为落地载体（轻量）。

## 核心设计原则（精简版）
1. **最小内核**：固定少量 Core 字段，其他全部进 extensions（命名空间）。
2. **渐进式加载**：技能与产物都按需展开，不把全文塞进上下文。
3. **模块化交付**：每个执行单元（MPU）输出独立文件，天然无冲突。
4. **强协议边界**：Adapter/Renderer 只做转换/表现，不做语义决策；做不了就出 issue_list。
5. **并行当资源**：全局并行配额可回收再分配；谁能拿走由调度策略决定（不是抢）。

## 额外收敛约定（本轮讨论结论）
- **位置锚点使用 HTML Anchor**：在 Markdown 中插入 `<a id=...></a>` + `<!--meta ...-->`，由生成器在区块前自动写入，支持稳定跳转与追溯。
- **两级版本**：Major(vN) 表示用户/VP级“推翻重来”；Revision(rN) 表示同一 task 内返工迭代。
- **目录扁平化**：team/agent 等治理信息放在 TaskSpec（spec.yaml）里，不用目录层级表达。

## 最小工具网关/沙盒/审计（系统边界，不可自由发挥）
即便不落库，也必须把“能做什么/不能做什么”写死在工具网关层。

最小要求：
- **ACL（写权限）**：每个 task 只能写 `TaskSpec.outputs.*` 指定路径（或允许前缀）；禁止写其他文件。
- **配额**：每个 role/task 有 token/工具调用/并行数上限；超限必须降级或请求裁决。
- **审计字段**（至少记录到文件或日志）：
  - who: agent_id / role / team_id
  - what: tool_name + args 摘要（脱敏）
  - where: 读写的路径 / 外部连接域名
  - when: timestamp
  - result: success/fail + error
- **沙盒阶段**（最小三档）：local → container → vm（逐步强化隔离），不影响上层契约。

## X) 最小 Tool Gateway / 审计 / 索引边界（必须）
为避免实现者自由发挥导致越权/不可追溯，本节为硬性系统边界。

### X.1 写入 ACL（硬限制）
- Agent/合并器任何写入必须经过 Tool Gateway
- Tool Gateway 必须根据 TaskSpec 的 `outputs` 与 `policy.allowed_prefixes` 校验路径
- 校验失败：直接拒绝写入（不得自动改写路径）

### X.2 配额与并行（硬限制）
每个 run 必须同时满足：
- `max_parallel_units`（全局并行上限）
- `per_agent_max_parallel_subtasks`（单 agent 可开分身上限）
- `tool_calls_budget` / `token_budget`（到达即降级或停止）

### X.3 审计字段（硬要求）
每次 tool 调用必须记录：
- who: `agent_id`, `role`
- what: tool_name, action
- where: file_path（若写文件）
- when: timestamp
- result: ok/error + error_code
- linkage: `run_id`, `task_id`, `rev`

### X.4 检索索引边界（硬要求）
- 索引 corpus 只能来自 TaskSpec.retrieval.corpus_globs 匹配的文件
- `deliver/**` 永不纳入索引（禁止自引用闭环）
- 跨 case/version 检索默认禁止（除非显式配置 allowlist）


## 会议模式（可插拔）

- 会议不是默认路径；仅在需要澄清/裁决/对齐时触发。
- 会议产物也是文件（whiteboard/decision/transcript），可回灌 TaskSpec，且同样遵循“锚点 meta 由程序生成、Agent 不编字段”。

---

## 文件名：docs/01_minimal_kernel.md

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

---

## 文件名：docs/02_planning_and_parallelism.md

# 02 计划与并行（Delivery Plan + MPU + spawn + 调度配额）

## 1) Delivery Plan：让“动态组团”可执行
最小字段：
- case_id / goal / deliverables[]
- teams[]（可选；分叉才需要多个）
- roles[]（每个 Team 的角色实例）
- quality[]（校验节点）
- budgets（可选）

## 2) MPU：并行最小单元（Minimum Parallel Unit）
一个 MPU 必须满足：
- 单一目标（一句话说明）
- 输入边界清晰（读哪些文件/节点）
- 输出是一个文件（模块文件）
- 与其他 MPU 弱依赖（可最后合并）

## 3) spawn（分身）制度化：允许，但有硬规则
允许 spawn 的条件：
- 子任务可模块化输出（独立文件）
- 子任务之间弱依赖
- 工具/权限需要隔离
禁止 spawn：
- 本质是单线程裁决（需要统一决策）
- 会写同一文件/同一资源

建议限制：
- spawn 深度 ≤ 2
- 每 agent 同时 spawn ≤ 3~5

## 4) 并行度作为“资源池”（Global Concurrency Pool）
- GLOBAL_MAX：全局并行上限（例如 10）
- TEAM_MAX：每 Team 上限（防某路线吃光）
- AGENT_MAX：每角色/agent 上限（防无限分身）

slot 以 Lease（租约）形式发放，可续租、可回收。
运行单元 DONE/CANCELLED → slot 回收 → 再分配。

## 5) “谁能拿走”并行资源？
建议默认：**中心化分配**
- 角色/agent 只能 request_slots(n, tasks, reason)
- 调度中心按优先级/关键路径/截止时间/公平性分配

可选加速通道：主编/监督者可对某交付物倾斜预算（拨款），但仍由调度中心执行发放。

## 6) 抢占（Preemption）策略（轻量）
仅对可重跑/低价值/卡死任务抢占：
- non_preemptible / soft_preemptible / hard_preemptible 三档
做不了就降级：出 issue_list 或把任务切小重跑。

## 7) 文件布局建议（扁平化）
为了避免目录爆炸：不再用多层 team/agent 目录表达治理，而是把治理字段放进 TaskSpec。

推荐（单 case）：
```
fs/cases/{case_id}/
  current.yaml                 # 指向当前 vN（程序/Planner 单写者维护）
  versions/
    v2/
      spec.md                  # Master IR / 需求与验收（Planner/Editor 单写者）
      tasks/
        task-000123/
          spec.yaml
          current.yaml         # 指向当前 rN（程序维护）
          revs/
            r1/ (summary.md, cards.md, issues.md)
            r2/ ...
          review.md
      deliver/
        report.md
        slides.md
```

## 8) 版本两级：Major(vN) vs Revision(rN)
- **Major(vN)**：用户/VP层面推翻重来 → 新建 `vN+1/`，更新 `fs/cases/{case_id}/current.yaml`
- **Revision(rN)**：组长/审核打回返工 → 同一 task 下新建 `revs/rN+1/`，不覆盖旧版本
读取最新：通过 `task/current.yaml` 指针定位当前 rN。

## 9) TaskSpec 引用（进一步减负）
Agent 产物文件只写：
- `task_spec_ref: ../../tasks/task-000123/spec.yaml`（或 task_id）
其余元字段全部由程序/调度中心在 spec.yaml 中维护。

## 10) 从需求到计划：Team Builder / Role Selector 的“契约”
目标：把“动态组团”从口号变成可实现的输入输出；实现者按契约做即可，不自由发挥。

### 输入（来自用户需求 + Master IR）
```yaml
goal: "..."
deliverables: [report, ppt]           # 需要产出哪些交付物
constraints:
  time: "..."
  budget: "..."
  tech_boundary: ["..."]
acceptance:
  must_answer: [1,2,3]
  must_not: ["不要引入新事实", "不要改动范围"]
context_refs:
  - global/requirement.md
  - global/master_ir.yaml
```

### 输出（Delivery Plan 片段：可直接喂给调度中心）
```yaml
teams:
  - team_id: team_a
    intent: "主方案（稳健）"
  - team_id: team_b
    intent: "备选方案（激进/更快/更省成本）"
roles:
  # 角色不是“固定编队”，而是按交付物链路与风险点最少集
  - role: planner_editor
    count: 1
    owns: ["master_ir", "outline", "acceptance_gate"]
  - role: domain_writer
    count: 2
    owns: ["cards", "sections"]
  - role: ppt_transformer
    count: 1
    owns: ["ppt_ir"]
  - role: verifier
    count: 1
    owns: ["coverage_map", "issue_list"]
quality:
  - gate: "must_answer_coverage"
  - gate: "no_new_facts"
  - gate: "cross_deliverable_consistency"
budgets:
  max_parallel: 10
  per_role_parallel_cap:
    domain_writer: 3
    ppt_transformer: 1
```

### 选择器（收益 > 成本）的最小启发式
- **需要多个 Team 的信号**：方向不确定/有明显分叉/代价差异大/需要对比（最多 3 个 Team）。
- **需要新增角色的信号**：出现强协议输出（JSON schema）→ 必有 Adapter/Verifier；涉及安全/权限 → 必有 Tool Gateway/审计。
- **避免过度编队**：能用“Verifier + cards”解决的，不要再加“二次专家审查”。

### 最小例子：报告 + PPT
- Planner/Editor：产出 Master IR + 章节/页大纲（锚点）
- Writers：按大纲节点并行产出 cards/sections（一个节点一个文件）
- PPT Transformer/Adapter：把 Master IR 投影为 PPT_IR → renderer_input.json
- Verifier：覆盖度 + 一致性 + 长度/丢失风险，必要时生成 issue_list

## X) 检索优先（BM25/向量）与 Cards 触发规则（写死，不模糊）

本系统默认采用 **“检索找材料 +（必要时）Cards 结构化合并”** 的策略：
- 检索（BM25/可选向量）解决 **“找得到”**
- Cards 解决 **“合并不跑偏 + 可验收 + 可追溯”**
- 任何情况下，都必须保留 **引用协议（manifest/引用块）**，否则后续无法审计与回放

### X.1 TaskSpec 必填字段（与检索/合并相关）
在 `tasks/<task_id>/spec.yaml` 中，必须包含以下字段（缺一不可）：

```yaml
artifacts_required: ["summary"]         # 默认只要 summary
retrieval:
  enabled: true
  mode: "bm25"                          # bm25 | hybrid
  top_k: 25                             # 合并器检索返回片段数量
  chunking:
    unit: "md_block"                    # md_block(按标题/段落块) | paragraph
    max_chars: 1200                     # 单 chunk 最大字符数（硬上限）
    overlap_chars: 120                  # chunk 重叠字符数
  corpus_globs:                         # 只在本 case/version 内检索（禁止跨 case）
    - "tasks/**/revs/**/summary.md"
    - "tasks/**/revs/**/full.md"
  deny_globs:                           # 明确禁止纳入索引的路径
    - "deliver/**"
    - "log/**"
cards_policy:
  required_when:                        # 触发 Cards 的硬规则（满足任一则必须产 Cards）
    - "parallel_teams>=2"
    - "must_answer_count>=6"
    - "has_tradeoffs=true"
    - "needs_comparison_matrix=true"
    - "conflict_detected=true"          # 检索结果出现互斥结论/相反建议（由 verifier 标注）
    - "evidence_required=true"          # 需要明确证据链（合同/合规/投研等）
  optional_when:
    - "parallel_teams==1 && must_answer_count<=5 && has_tradeoffs=false"
citation_policy:
  manifest_required: true
  cite_fields: ["task_id","rev","file","chunk_id","start_line","end_line","sha256"]
```

> 解释：  
> - `mode=bm25`：只使用 BM25（默认，先落地）；  
> - `mode=hybrid`：BM25 + 向量召回（需要时再开），但引用协议不变；  
> - `deny_globs` 硬限制：交付物与日志永不入索引，避免“合并器拿交付物当证据”自循环。

### X.2 Cards 是否需要：最终判定规则（完全确定）
- 若 `cards_policy.required_when` 任一条件为真：`artifacts_required` 必须包含 `"cards"`
- 否则：`artifacts_required` 仅包含 `"summary"`（可选 `"full"`，见下条）
- 若任务属于“内容长且可能被摘抄”场景（报告/论文/脚本）：建议加 `"full"`，但不是强制

### X.3 检索输出格式（合并器/主编必须按此消费）
检索服务/本地脚本输出必须是严格结构化对象（JSON/YAML 皆可），每个命中片段必须包含：

- `task_id`, `rev`
- `file`
- `chunk_id`（稳定：`<file>#<n>` 或 hash）
- `start_line`, `end_line`
- `score_bm25`（若 hybrid 再加 `score_vec`）
- `text`（命中正文片段，允许截断但必须可回溯定位）

合并器不得把“检索命中片段”当最终内容直接改写为新事实；任何新增断言必须能回指到某个 `chunk_id`。

### X.4 Cards 最小字段（如果触发 Cards，必须包含这些）
Cards 文件中每张卡必须包含（缺一不可）：

- `claim`：一句主张（不可超过 200 字）
- `evidence`：引用列表（至少 1 条，使用 citation_policy 的字段）
- `conditions`：适用条件/边界（至少 1 条）
- `tradeoffs`：代价/风险（至少 1 条；没有则写 `tradeoffs: none`）
- `links_to_outline`：绑定到 `assigned_outline_nodes`（至少 1 个 node）
- `confidence`：0~1（可选，但建议提供）

### X.5 “检索替代 Cards”的边界（写死）
允许“只用检索 + summary”而不产 Cards 的前提同时满足：
1) `parallel_teams==1`  
2) `must_answer_count<=5`  
3) `has_tradeoffs=false && needs_comparison_matrix=false`  
4) verifier 未标记 `conflict_detected`  
否则必须产 Cards。

## Y) Planner/PM：从需求到 Team/MPU 的决策契约（写死）

本系统区分两层“并行”：
- **Team 并行（宏观分支）**：不同方案/路线/交付路径并行探索（上限通常 3）
- **MPU 并行（微观单元）**：同一方案下，按大纲节点/必须回答拆分的执行单元（可更高并行）

### Y.1 谁决定派几个 Team？
唯一决策者：**PM/Planner（同一角色的两个面向）**
- Planner 负责生成 Master IR、Delivery Plan、TaskSpec
- PM 负责对用户汇报、处理中途指令、裁决分叉、最终签字

> 实现上可以是一个 Agent（带不同 sub-role prompt），也可以是两个 Agent，但决策权必须集中，不可让 Team 自行增殖。

### Y.2 派 Team 的硬规则（不靠模型随意判断）
默认：`team_count=1`。只有满足以下条件才允许派生额外 Team（每条是硬触发）：

- `has_competing_strategies=true`（至少两条路线在成本/风险/收益上显著不同）
- `decision_is_directional=true`（方向性选择，不做会影响整体交付）
- `user_explicitly_requests_options=true`（用户明确要多方案对比）
- `uncertainty_high=true` 且 `time_budget_allows_parallel=true`

并行 Team 上限：
- `max_parallel_teams` 默认 **3**（与我们此前讨论一致）
- 若达到上限且仍需分叉：必须 terminate 一个 Team 或请求用户裁决（参见 `05_user_interaction.md`）

### Y.3 分叉与资源分配（写死）
- 新 Team 必须有 `fork_reason` 和 `fork_point`（写入 decisions/forks）
- 每个 Team 初始配额默认均分；可由 PM/Planner 明确倾斜
- Team 结束或被 terminate 后，其配额回收至资源池，由 scheduler 再分配

### Y.4 Master IR 到 TaskSpec 的生成链路（回答“TaskSpec 谁生成”）
1. PM/Planner 生成 `spec.md`（Master IR：goal/constraints/deliverables/must-answer/outline）
2. Planner 生成 `Delivery Plan`（teams/roles/quality/budgets）
3. Planner 为每个 MPU 生成 `tasks/<task_id>/spec.yaml`
4. 若 Team 内需要更细拆分：**只能由 Planner** 生成子 TaskSpec（Team Lead 只能提出建议，不得自行创建）

### Y.5 主编（Editor）是谁？
- **Editor=合并器决策者**：负责 Normalize/Assemble 的“最后口径”
- 默认由 PM/Planner 兼任（小规模）；当并行 Team >=2 或交付物 >=2 时，建议拆分为独立 Editor
- Verifier 不等于 Editor：Verifier 只做规则检查与出 issue_list，不做业务裁决


## 7) 会议模式（可插拔拓扑）
- 会议是一个 Topology Plugin：用于“决策收敛/分歧处理/信息补全”，不替代 MPU 产出流水线。
- 会议的并行：Think 并行占用 slots；Speak 串行不等人。
- 触发点与产物格式见：`docs/06_meeting_mode.md`。

---

## 文件名：docs/03_artifact_pipeline.md

# 03 多交付物与强协议流水线（Artifact Pipeline + 插件契约）

## 1) 为什么要分 Master IR / View IR
- Master IR：全局口径与动态大纲的“真相源”（少字段，灵活）
- View IR：面向具体交付物的投影（PPT_IR / REPORT_IR / CONTRACT_IR）
这样一个需求可以有多个不同大纲（PPT 一页一页；报告一章一章），但口径来自同一 Master IR。

## 2) 强协议节点：JSON→HTML（PPT）谁负责？
结论：由插件链负责，主编负责语义签字。

- Transformer：Master IR → PPT_IR（语义页结构）
- Adapter：PPT_IR → renderer_input.json（严格 schema）
- Renderer：renderer_input.json → slides.html（表现层）
- Verifier：覆盖度/一致性/格式/损失校验

## 3) 插件契约（必须守边界）
### Transformer
- 允许：重组/映射/裁剪
- 禁止：引入新事实/新假设（除非 issue_list 请求裁决）
- 必产出：coverage_map + transform_log

### Adapter
- 责任：schema 校验、默认值、长度限制处理、字段归一化
- 禁止：改变语义；必须提示“损失风险”
- 必产出：transform_log

### Renderer
- 仅表现层生成；不得做内容决策
- 建议产出：渲染摘要（页数/失败原因）

### Verifier
- 输出：verify_report + issue_list
- 关注：must-answer 覆盖、report/ppt 口径一致、压缩丢失

## 4) issue_list：统一“做不了/有冲突”的反馈
字段建议：
- severity: blocker/warn/info
- where: transform/adapter/render/verify
- what: 问题描述
- options: 可选策略（裁剪/拆分/回问用户/降级）
- need_decision_by: Editor/User/Planner
- suggested_patch: 可选

> 重要：强协议输出的最终节点可以是“PPT 生成器/渲染器”，但**交付责任仍在主编**（语义签字）。

## 5) 自动锚点与可追溯（Markdown 内嵌 HTML Anchor，不靠 Agent）
- 最终交付物（report/ppt）由生成器拼装时，**自动在每个区块前插入 HTML Anchor**，实现稳定跳转。
- 紧跟一行 `<!--meta ...-->` 记录 `task@rev` 与 sources，保证可追溯，不要求 Agent 写任何标记。
- 组长阅读时，anchor/注释默认不可见；需要定位时直接使用 `path#id`。

区块模板：
```md
<a id="block-deliver-report-2"></a>
<!--meta task=task-000123@r2 sources=task-000123@r2-->
## 2. 并行与资源池
...
```

引用示例：
```md
见：[报告第2章](deliver/report.md#block-deliver-report-2)
```

## 6) 合并器协议（Normalize → Assemble → Verify）
目标：让“主编合并”可实现、可验收、可回滚，避免合并器随意引入新事实。

### 6.1 Cards 最小字段（可合并协议）
每张卡至少包含：
- `card_id`（程序生成或合并器补齐）
- `links_to_outline`（绑定到大纲节点，如 report:2 / ppt:s05）
- `claim`（主张/结论）
- `evidence`（依据：引用/数据来源/推理链简述）
- `conditions`（适用条件/假设）
- `tradeoffs`（取舍/副作用）
- `confidence`（可选：0~1，自评）

### 6.2 阶段产物（合并过程必须留下脚印）
- `deliver/manifest.md`：本次交付引用了哪些 `task@rev`、哪些 `card_id`
- `deliver/coverage_map.yaml`：must-answer 与 outline 节点的覆盖映射（Verifier/合并器产出）
- `deliver/issue_list.md`：无法解决/需裁决的事项（blocker 优先）

### 6.3 规则：防止“合并器引入新事实”
- 合并器只能“重排/裁剪/归并/改写表达”，**不得新增事实**；
- 若必须新增（例如为了连贯补充一句假设），必须写入 `issue_list` 并标注 `need_decision_by`；
- Verifier 对比 cards 与最终 deliver：出现“无来源句子”即报警。

### 6.4 绑定到位置锚点（Anchor + meta）
- 每个交付区块前的 `<a id=...></a>` + `<!--meta ...-->` 由生成器写；
- `meta` 的 `sources=` 应从 manifest 自动生成（task@rev + card_id 可选）。

## X) 合并器协议（必须遵守，不得自由发挥）

合并器（Editor/Assembler）只做三件事：**归一化 → 组装 → 验证**。任何偏离都会导致不可验收或不可追溯。

### X.1 Normalize（归一化）
输入：各任务产物（summary / optional cards / optional full）与检索命中列表。  
输出：`deliver/manifest.(md|yaml)` 的草稿与候选引用集合。

硬规则：
- 不得改写事实：Normalize 只允许“摘取/重排/去重/格式统一”
- 任何句子若新增断言（新事实/新数字/新因果），必须标记为 `NEW_ASSERTION` 并进入 `issue_list`
- 任何引用必须携带 citation_policy 规定字段（见下）

### X.2 Assemble（组装）
输入：Normalize 的候选集合 + `spec.md` 的结构（outline/slide 计划）。  
输出：最终交付物（例：`deliver/report.md`, `deliver/slides.md`）与 `coverage_map.yaml`。

硬规则：
- 交付物每个章节/slide 区块前必须插入锚点与 meta（生成器写）：
  ```md
  <a id="block-deliver-report-2"></a>
  <!--meta task=task-000123@r2 sources=task-000120@r1,task-000121@r1-->
  ```
- `id` 命名必须使用：`block-<scope>-<deliverable>-<node>`
- 区块内容只允许来自：
  1) 任务产物（summary/cards/full）的可回溯片段
  2) 用户输入的原始需求/补充信息
- 合并器不得把“交付物旧版本”当来源（禁止自引用闭环）

### X.3 Verify（验证）
输入：交付物 + manifest + coverage_map + issue_list  
输出：`deliver/issue_list.md`（若无问题可为空）与“通过/不通过”结论（供调度中心决定是否返工）。

必须检查：
- must-answer 覆盖：每条 must-answer 都必须在 coverage_map 中有 >=1 个证据引用
- 冲突检测：同一 must-answer 下若出现互斥结论，必须进 issue_list 并触发 cards_policy.required_when: conflict_detected=true
- 引用可回放：manifest 中每条引用都能定位到原文件的 `start_line~end_line`

---

## Y) 合并阶段必须产物（固定三件套）

### Y.1 manifest（引用清单，必须有）
路径：`deliver/manifest.yaml`（或 .md，但字段必须等价）

每条引用必须包含（缺一不可）：
- `task_id`
- `rev`
- `file`
- `chunk_id`
- `start_line`, `end_line`
- `sha256`（对引用片段文本计算）
- `used_in`：被用于哪个区块（例：`block-deliver-report-2`）

### Y.2 coverage_map（验收映射，必须有）
路径：`deliver/coverage_map.yaml`

字段要求：
- `must_answer_id` → `evidence_refs[]`（每个 evidence_ref 指向 manifest 的一条引用）
- 同时记录 `status: covered|missing|conflicted`

### Y.3 issue_list（问题单，必须有）
路径：`deliver/issue_list.md`

issue 最小字段：
- `severity: blocker|warn|info`
- `where: normalize|assemble|verify|source`
- `what`
- `evidence`（若相关）
- `action`（建议修复方式）

## Z) 失败处理与回滚（必须，写死）

### Z.1 issue_list 出现 blocker 后怎么走？
若 `deliver/issue_list.md` 中存在 `severity=blocker`：
1. Verify 结论必须为 **NOT_PASS**
2. PM/Planner 必须选择以下之一并记录 decision（不可跳过）：
   - `request_rework`：打回到相关 task，创建新 Revision `r+1`
   - `terminate_branch`：若 blocker 来源于某个 Team/分支，则 terminate 该 Team
   - `major_restart`：若 blocker 表示需求/方向错误或用户推翻 → 升级 Major（v+1）

### Z.2 回滚到 r1 还是继续修？
规则：
- **不覆盖旧 revision**：永远创建新 `revs/r(N+1)/`
- “回滚”只是一种读取视角（把 current 指向旧 rev），但最终修复仍应产生新 rev
- 由 PM/Planner 决策（必要时回问用户）

### Z.3 Team 失败怎么处理？
- Team 内连续两次 NOT_PASS 或成本超预算 → PM/Planner 可 terminate_team
- 被 terminate 的 Team 产物进入 archived 只读，供复盘与证据引用（不得删）

### Z.4 自动降级策略（允许，但必须可审计）
当资源耗尽（token/tool_calls/时间）：
- 允许从 `full` 降级为 `summary-only` 交付
- 允许降低 top_k 或关闭向量，仅保留 BM25
但必须：
- 在 decisions 中记录 `degrade_reason` 与影响范围
- issue_list 记录“降级导致的缺口”


## 7) 会议模式的注入点（可选，但建议）
- 当出现 fork/blocker/Verify 不通过，可触发会议模式做“快速收敛”。
- 会议产物（decision/action_items）写回 decisions 与 TaskSpec，避免靠主编脑补。
- 详见：`docs/06_meeting_mode.md`。


## 3.x 会议产物作为上游输入

- 会议（06_meeting_mode.md）产出的 whiteboard/decision/action_items 可作为 TaskSpec.master 或 TaskSpec.patch 的输入。
- 会议 transcript 默认不进入执行上下文，仅用于审计/引用验证（按需检索）。


---

## 补充：Gate Node 与流水线的关系

- Gate 不产出大段正文，它产出 **decision + TaskSpec 更新**。
- 触发条件（写死）：分叉影响交付 / Verifier blocker / 口径冲突 / 高风险定稿 / 用户大版本打回。
- 执行模式：并行提交 position_packet → Moderator 单写 gate_decision。

详见：`docs/07_convergence_gates.md`。

---

## 文件名：docs/04_walkthrough_report_ppt.md

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


---

## 文件名：docs/05_user_interaction.md

# 05 用户中途指令处理（硬规则）

> 目标：不中断并行执行的前提下，将用户指令“吸收进系统”，并且可审计、可回滚、可裁决。
>
> 术语对齐（v1）：本文的 `change_request` 在 IGI 资源模型中对应 `kind=ChangeSet`（`apiVersion: igi.zhanggui.io/v1`）。  
> 用户输入的文件/图片/PDF/URL 等应先入库为 `Artifact/ArtifactManifest` 再被引用，避免把二进制或海量内容直接塞进上下文（见 `docs/12_runtime_and_input_model.md`、`docs/13_igi_v1_resource_model.md`）。

---

## 1) 指令分类（必须先分类）
用户消息进入系统后，PM/Planner 必须在写入日志前完成分类（不可跳过）：

| 类型 | 典型例子 | 对正在跑的 MPU/Team 的影响 | 默认动作 |
|---|---|---|---|
| 补充信息 | “我们用 MySQL 5.7” | 不改变目标，仅补充约束 | 写入 `spec.md#constraints`，广播相关 Team |
| 询问进度 | “现在到哪了？” | 无 | 返回 status（从 progress/manifest 推导） |
| 追加需求 | “顺便支持导出 PDF” | 可能改变交付物/范围 | 进入 `change_request` 流程（见 §3） |
| 方向调整 | “移动端先不做了” | 会废弃部分工作 | 触发 `replan`（见 §4） |
| 方案选择 | “就用方案 B” | 砍掉分支 | 触发 `terminate_team`（见 §5） |
| 推翻重来 | “全部作废，重做一版” | 新 Major | 触发 `major_restart`（见 §6） |
| 质量/格式要求 | “PPT 必须 10 页以内” | 改验收与产出格式 | 更新 TaskSpec/transformer 约束 |

---

## 2) 统一写入：interaction_log（必须）
任何用户消息必须写入（文件或 DB）并带上以下字段（缺一不可）：

- `interaction_id`
- `timestamp`
- `raw_user_message`
- `classified_type`
- `pm_understanding`（PM/Planner 对意图的单句复述）
- `action_taken`（触发了哪些系统动作）
- `affected_major`（vN）
- `affected_tasks`（task_id 列表）
- `affected_teams`（team_id 列表）

---

## 3) change_request（追加需求）的硬流程
触发条件：classified_type = 追加需求

流程：
1. PM/Planner 生成 `change_impact`（必须包含：新增交付物/改动范围/新增 must-answer/预计新增 MPU）
2. 若影响满足任一：`deliverables_changed=true` 或 `scope_changed=true` → 必须升级 Major（见 §6）
3. 否则：仅产生新 task（Revision 内追加），并写入 `spec.md#delta`

硬规则：
- 不允许“直接塞进正在写的段落”而不记录变更。
- 变更后必须重新跑 Verify（coverage_map/issue_list）。

---

## 4) replan（方向调整）的硬流程
触发条件：classified_type = 方向调整

流程：
1. PM/Planner 评估已有产物可复用性（可复用：保留；不可复用：deprecated）
2. 更新 `spec.md#constraints/#deliverables/#must_answer`（delta 方式记录）
3. 对受影响的 tasks：
   - 若仅变更输出格式 → 更新 transformer/adapter 约束，保持 task
   - 若目标/结论方向变更 → 创建新 task，旧 task 标记 deprecated（Revision 内）

---

## 5) terminate_team（用户选方案/砍分支）
触发条件：classified_type = 方案选择

规则：
- `teams` 并行上限受 `max_parallel_teams` 控制（默认 3）
- 被砍 team 必须：
  - 标记 `terminated`
  - 保留已产出（summary/cost/notes）进入 archived（只读）
  - 释放配额回资源池（scheduler 做）

---

## 6) major_restart（用户推翻重来）
触发条件：classified_type = 推翻重来 或 change_request 导致 scope/deliverables 变化

规则（写死）：
- 创建新目录 `fs/cases/{case_id}/versions/v(N+1)/`
- 复制上一版的 `spec.md` 到新版本，作为历史基线，新增一节 `# restart_reason`
- 新版本必须重新生成所有 TaskSpec（旧 TaskSpec 不复用，只作为参考）
- 更新 `fs/cases/{case_id}/current.yaml` 指向 `v(N+1)`

---

## 7) 何时必须“回问用户”（不要模型自由发挥）
若满足任一，PM/Planner 必须回问用户并暂停新任务派发：
- 用户指令自相矛盾（例如：既要“只要方案A”又要“方案B细化”）
- 影响范围不清（无法判断是否要升级 Major）
- 验收标准缺失（必须回答的问题 must-answer 无法列出）

---

## 文件名：docs/06_meeting_mode.md

# 06 会议模式（Meeting Mode v2：上下文工程版）

> 本文是会议模式（Meeting Mode v2）的**权威规范入口**。  
> 目标：在不牺牲可控性/可追溯性的前提下，实现“发言串行 + 思考并行”的会议协作，并与 TaskSpec / Artifact Pipeline / Gate Node 无缝复用。  
> 关键约束：**共享文件单写者**（避免并行写入冲突）；参与者只提交提案，不直接改共享区。

---

## 0) 在定义协议前：先把边界写清楚（必须）

### 0.1 本文解决什么（范围）
- 把“会议”定义为一个可插拔拓扑：当出现分歧/不确定/阻塞时，以最小回合完成裁决与信息补全。
- 明确：会议的**文件结构**、**写入权限**、**提案/发言/收敛**的协议、以及会议结束后的**输出物**如何注入任务流。

### 0.2 本文不解决什么（非目标）
- 不规定具体 UI（聊天/网页/命令行均可）。
- 不规定具体实现语言/框架/数据库（默认仍为文件系统）。
- 不把会议变成“产出大段正文”的主路径；会议产物以 decision / action_items / brief 为主。

### 0.3 规范用语（为了可执行）
本文使用以下关键词：
- **必须**：不满足即视为协议违规（应被工具网关/Verifier 拒绝或报错）。
- **禁止**：出现即视为越权或不可验收。
- **建议**：默认策略；可以改，但要在 MeetingSpec/decision 中记录理由。

### 0.4 核心不变式（必须始终成立）
1) **单写者共享区**：`fs/meetings/{meeting_id}/shared/**` 仅允许 Recorder 写入。  
2) **可追溯**：进入 whiteboard/decision 的结论必须能回链到提案与 sources（会议内或外部）。  
3) **可控上下文**：运行时上下文装配只加载“必要片段”（白板 + 最近发言 + 必要证据），而不是全文 transcript。  
4) **不靠 Agent 自由发挥元字段**：锚点 id / meta 字段尽量由生成器或 Recorder 统一生成；参与者只填内容字段。

> 落地计划见：`docs/08_development_plan.md`（语言无关、多阶段）。

---

## 1) 会议在流程中的位置（触发条件）

会议不是默认步骤，仅在以下触发条件出现时介入：

- **任务之前**（planning / clarification）
  - 需求不清晰、存在多条路线、必须明确 `acceptance.must_answer`
- **任务过程中**（fork / blocker）
  - 出现方向分叉（fork_detected）、阻塞（blocker_issue）、验证失败（verifier_conflict）
- **任务之后**（review / retro）
  - 验收争议、用户打回大版本、需要沉淀可复用结论

默认原则：**不开会；满足触发条件才开会；能用 Gate Node 解决的优先 Gate**（见 `docs/07_convergence_gates.md`）。

---

## 2) 上下文分层（会议记忆模型）

为控制 token 与避免角色混淆，会议上下文分三层：

### Layer A：工作草稿（Agent 私有，短命）
- `fs/meetings/{meeting_id}/agents/{agent_id}/vault/scratchpad.md`（可选）
- 内容：草稿/待发言点/局部推理；**不进入共享结论**

### Layer B：会议共享记录（全员可读）
- `fs/meetings/{meeting_id}/shared/transcript.log`（append-only）
- `fs/meetings/{meeting_id}/shared/whiteboard.md`（single-writer，可覆盖写）
- `fs/meetings/{meeting_id}/shared/hand_queue.json`（可选，single-writer）
- `fs/meetings/{meeting_id}/shared/decisions.md`（append-only，可选）
- `fs/meetings/{meeting_id}/shared/compaction/snap_0001.md`（可选，append-only 新文件）

> 说明：**“只保留最近 N 条发言”是上下文装配策略，不是文件截断策略。**  
> 文件本身保持 append-only；过长时由 Recorder 产出 compaction snapshot，运行时只读取 snapshot + 最近窗口。

### Layer C：归档结论（跨会议可检索）
- `fs/archive/meetings/{meeting_id}/decision.md`
- `fs/archive/meetings/{meeting_id}/action_items.md`
- `fs/archive/meetings/{meeting_id}/meeting_brief.md`

---

## 3) 会议拓扑：Think 并行 / Speak 串行 / Settle 串行

- **Think（并行）**：参与者围绕 focus 产出 Proposal Block（短、结构化、可追溯）。
- **Speak（串行）**：Moderator 依据 hand queue 选择发言者；Recorder 记录到 transcript。
- **Settle（串行）**：Recorder 将可采纳内容写入 whiteboard（Settled Block）。

> 并行只发生在“思考/提案生成”；共享区写入永远由单写者完成（见第 4 节）。

---

## 4) 文件与权限（强约束）

### 4.1 允许写入的路径（必须）

**参与者 Agent（architect/cost/security/writer 等）**
- ✅ 允许写：`fs/meetings/{meeting_id}/agents/{agent_id}/**`（如需落盘）
- ✅ 建议：不落盘，直接把提案内容返回给 Recorder 统一写入共享区
- ❌ 禁止写：`fs/meetings/{meeting_id}/shared/**`、`fs/archive/**`、`fs/cases/**`、`deliver/**`

**Recorder（共享区唯一写者）**
- ✅ 允许写：`fs/meetings/{meeting_id}/shared/**`、`fs/meetings/{meeting_id}/artifacts/**`
- ✅ 允许写：`fs/archive/meetings/{meeting_id}/**`（会议结束归档阶段）
- ❌ 禁止写：非授权的 case/task 目录（除非 action_items 明确要求且走 Tool Gateway）

### 4.2 写入类型（必须）
- `shared/transcript.log`：append-only（禁止覆盖写；内容必须按 Speak Block 追加，见 §5.2）
- `shared/decisions.md`：append-only（如存在）
- `shared/whiteboard.md`：single-writer（可覆盖写）
- `shared/hand_queue.json`：single-writer（可覆盖写）
- `shared/compaction/*.md`：append-only（只新增文件）

---

## 5) 资产与锚点协议（统一可追溯语法）

会议中所有可引用片段统一使用：**HTML anchor + HTML meta 注释**（rg 可定位、生成器可解析）。

### 5.1 Proposal Block（提案块，参与者产出）

**命名规则（必须）**
- `prop-{meeting_id}-{agent_id}-r{round}-{seq}`  
- `round` 从 1 开始；`seq` 建议 3 位补零（001, 002...）。

**最小结构（必须）**
```md
<a id="prop-{meeting_id}-{agent_id}-r{round}-{seq}"></a>
<!--meta
type: proposal
meeting_id: mtg-000001
agent_id: a03
round: 2
intent: rebut|propose|question|evidence|synthesize
confidence: 0.72
needs_sources: true
-->
**point**：一句话观点/结论  
**why**：核心理由（2~5条）  
**evidence_refs**：S2,S5（可空）  
**conditions**：适用前提/边界  
**tradeoffs**：取舍（成本/质量/风险/时间）  
**ask**：需要主持人裁决的问题（可空）
```

### 5.2 Speak Block（发言记录块，Recorder 记录）

**命名规则（必须）**
- `spk-{meeting_id}-r{round}-{seq}`

**最小结构（必须）**
```md
<a id="spk-{meeting_id}-r{round}-{seq}"></a>
<!--meta
type: speak
meeting_id: mtg-000001
round: 2
speaker: a03
intent: propose|rebut|question|summary
from_prop: prop-mtg-000001-a03-r2-001
ts: 2026-01-28T09:00:00+08:00
-->
内容：……（允许被自动截断；截断必须记录在 meta 中，例如 truncated=true）
```

### 5.3 Settled Block（白板收敛块，Recorder 维护）

whiteboard 只写“已收敛”的结论，并绑定来源提案：

```md
<a id="wb-{meeting_id}-{seq}"></a>
<!--meta
type: settled
meeting_id: mtg-000001
from_props: [prop-mtg-000001-a03-r2-001, prop-mtg-000001-a01-r2-004]
sources: [S2,S5]
-->
结论：选择批处理作为当前版本，保留流处理升级接口。
```

---

## 6) 会议协议（一步一步）

> 本节把会议当作“可执行的状态机”，每一步都有输入/输出与责任人。

### Step 1：创建 MeetingSpec（程序/Moderator）
必须生成 `fs/meetings/{meeting_id}/spec.yaml`，并写入：会议类型、参与者、context_refs、limits、outputs、policy。

### Step 2：初始化目录（程序）
必须创建：`shared/`、`agents/`、`artifacts/`（以及可选的 `events/`、`shared/compaction/`）。

### Step 3：打开 Round（Moderator 指令，Recorder 落盘）
Moderator 给出本轮 `focus`（一句话问题）；Recorder 必须将其写入 `whiteboard.md` 顶部（或 decisions.md），并标注 round 编号。

### Step 4：Think 并行产出提案（参与者）
参与者必须在时限内提交 Proposal Block（短、结构化）。
- 建议通过消息提交给 Recorder；如需落盘，则写入 `agents/{agent_id}/outbox/turn_XXXX.md`。

### Step 5：举手与排队（可选，Recorder 单写）
若启用 `hand_queue.json`：参与者只“请求发言”；Recorder 负责更新队列。

### Step 6：Speak 串行发言（Moderator 选择，Recorder 记录）
Moderator 从队列中挑选 1 人发言；Recorder 将发言写入 `shared/transcript.log`（Speak Block）。

### Step 7：Settle 收敛（Recorder 单写）
Recorder 将达成一致/可裁决的内容写入 `shared/whiteboard.md`（Settled Block），并回链到提案锚点与 sources。

### Step 8：必要时 Compaction（Recorder）
当 transcript 过长或出现重要阶段性结论：Recorder 新建 `shared/compaction/snap_000N.md`，摘要历史发言并列出覆盖到的 spk id 范围。

### Step 9：结束会议并产出三件套（Recorder）
会议结束必须产出：
1) `artifacts/export_minutes.md`（会议纪要，可引用锚点）
2) `artifacts/action_items.yaml`（可执行变更：TaskSpec patch / 新任务建议）
3) `artifacts/citations.yaml`（sources 清单与外部引用）

### Step 10：归档（程序/Recorder）
将最小可复用输入写入：`fs/archive/meetings/{meeting_id}/meeting_brief.md` + `decision.md` + `action_items.md`（可由 `action_items.yaml` 渲染生成）。

---

## 7) MeetingSpec（spec.yaml）协议（最小可执行）

> MeetingSpec 是会议的“唯一真相源”，用于约束写入 ACL、上下文装配范围与输出路径。

**必填字段（必须）**
```yaml
schema_version: 1
meeting_id: mtg-000001
type: planning   # planning/fork/blocker/review
topic: "数据处理架构选型"
participants:
  - { role: moderator, agent_id: pm }
  - { role: recorder, agent_id: recorder-01 }
context_refs:
  - { path: fs/cases/{case_id}/versions/v2/spec.md }
limits:
  think_timeout_s: 12
  speak_max_chars: 600
  max_rounds: 5
  max_minutes: 15
outputs:
  transcript_path: shared/transcript.log
  whiteboard_path: shared/whiteboard.md
  queue_path: shared/hand_queue.json
  decisions_path: shared/decisions.md
  minutes_path: artifacts/export_minutes.md
  action_items_path: artifacts/action_items.yaml
  citations_path: artifacts/citations.yaml
policy:
  # 约定：policy/outputs 中的路径，均以 “spec.yaml 所在目录（meeting root）” 为基准。
  # allowed_write_prefixes 负责把写入边界圈定在 meeting root 内；角色级规则见 §4 与 `docs/10_tool_gateway_acl.md`。
  allowed_write_prefixes: [""]
  append_only_files: ["shared/transcript.log","shared/decisions.md"]
  single_writer_prefixes: ["shared/","artifacts/"]
  single_writer_roles: ["recorder"]
  lock_file: "shared/.writer.lock"
```

**可选字段（建议）**
- `focus`：本次会议首轮 focus（一句话）
- `must_answer_refs`：受影响的 must-answer 列表（引用 task/case 的锚点）
- `evidence_policy`：是否允许外部资料、是否需要 citations 完整度
- `queue_policy`：队列优先级公式与 intents 列表

---

### 7.1 hand_queue.json（可选，single-writer）

> 用途：把“举手/排队”从自由群聊变成可控队列；参与者只提出请求，Recorder 负责更新与选中记录。

**最小结构（必须）**
```json
{
  "schema_version": 1,
  "meeting_id": "mtg-000001",
  "round": 2,
  "queue": [
    {
      "agent_id": "a03",
      "intent": "question",
      "requested_at": "2026-01-28T09:00:00+08:00",
      "note": "只写一句话摘要（可选）"
    }
  ]
}
```

**规则（必须）**
- 只能由 Recorder 覆盖写（single-writer）。
- 每次选中发言者后，Recorder 必须在 `shared/transcript.log` 写入对应 Speak Block（`from_prop` 可为空）。

---

## 8) 会议输出（注入任务流）

会议结束至少要能回答两件事：
1) **裁决了什么**（decision）  
2) **下一步怎么执行**（action_items）

因此会议关闭时必须产出两类输出：

### 8.1 会议内输出（fs/meetings/{meeting_id}/artifacts，必须）
- `export_minutes.md`：会议纪要（可引用 prop/spk/wb 锚点）
- `action_items.yaml`：可执行变更（TaskSpec patch / 新任务 / 请求用户裁决）
- `citations.yaml`：sources 清单（会议内外证据的索引）

### 8.2 归档输出（fs/archive/meetings/{meeting_id}，必须）
- `meeting_brief.md`：后续 Planner/Editor 的最小上下文（替代全文 transcript）
- `decision.md`：最终裁决（必须回链到 wb/prop/source）
- `action_items.md`：从 `action_items.yaml` 导出的可读版（必须；用于审计/复盘）

> 注意：Planner/Editor 默认只读 brief/decision/action_items；全文 transcript 仅用于审计与争议回放。

### 8.3 action_items.yaml（最小 schema，必须）

```yaml
schema_version: 1
meeting_id: mtg-000001
generated_at: 2026-01-28T09:30:00+08:00
items:
  - item_id: ai-0001
    kind: task_patch          # task_patch|new_task|terminate_team|major_restart_request|ask_user
    summary: "把验收 must_answer 补齐到 10 条"
    need_approval_by: planner # planner|editor|user
    priority: P0              # P0|P1|P2
    rationale:
      from_wb: wb-mtg-000001-001
      from_props: [prop-mtg-000001-a03-r2-001]
    patch:                    # kind=task_patch 时必填（其余 kind 可省略）
      patch_spec_version: 1
      target:
        file: fs/cases/{case_id}/versions/v2/spec.md
        selector:
          kind: md_heading    # md_anchor|md_heading
          value: "acceptance.must_answer"
      ops:
        - op: md_append_lines # md_append_lines|md_replace_section|md_insert_after
          lines:
            - "- 11) ……"
            - "- 12) ……"
```

**规则（必须）**
- `items[].rationale` 必须能回链到 whiteboard/props（至少一个）。
- 任何涉及“改 case/spec”的 patch，必须声明 `need_approval_by`，并在决策链路中可审计。

#### 8.3.1 PatchSpec v1（语言无关，必须）

> 目的：把“会议结论要改哪些内容”表达成**可实现、可审计、可拒绝**的补丁；具体实现语言/工具不在本文范围。

**通用规则（必须）**
- Patch 必须是 **atomic**：任一 op 失败 → 整个 patch 失败，不得部分成功。
- Patch 不得隐式创建路径：selector/目标缺失必须失败，并产出 issue_list 或转为 `ask_user`。
- Patch 应支持“安全前置条件”（可选但建议）：例如 section/base hash 不一致则拒绝，避免静默覆盖。

**target.selector（必须）**
- `kind=md_heading`：`value` 必须与目标 Markdown 中的标题文本**完全一致**（不含 `#`）。选中范围为：
  - 从该标题行开始，到**下一个同级或更高层级标题**之前（若没有则到文件末尾）。
- `kind=md_anchor`：`value` 是 `<a id="..."></a>` 中的 `id`（不含 `#`）。选中范围为：
  - 从该 anchor 行开始，找到其后第一个标题行作为 section 起点；section 终止规则同 `md_heading`。
  - 若 anchor 后找不到标题行 → 失败（防止把整文件当 section）。

**ops（必须）**
- `md_append_lines`：把 `lines[]` 追加到选中 section 的末尾（在 section 结束边界之前）。
- `md_insert_after`：把 `lines[]` 插入到 selector 定位点之后（对 `md_anchor` 最常用）。
- `md_replace_section`：替换选中 section 的正文；可选字段：
  - `keep_heading: true|false`（默认 true；保留标题行，仅替换正文）

**安全前置条件（建议）**
```yaml
preconditions:
  section_sha256: "..."  # 以目标 section 原文计算；不一致则拒绝
```

### 8.4 citations.yaml（最小 schema，必须）

```yaml
schema_version: 1
meeting_id: mtg-000001
sources:
  - id: S1
    kind: file     # file|url|note|meeting_anchor
    ref: "fs/cases/{case_id}/versions/v2/spec.md#constraints"
    title: "当前版本约束"
    captured_at: 2026-01-28T09:10:00+08:00
  - id: S2
    kind: url
    ref: "https://example.com/spec"
    title: "外部规范（示例）"
```

**规则（必须）**
- `sources[].id` 必须全局唯一（同一 meeting 内）。
- Proposal 中的 `evidence_refs` 必须引用 `sources[].id`；若 `needs_sources=true` 则 `evidence_refs` 不得为空（否则必须转为 issue/ask_user）。

### 8.5 decision.md（归档，最小模板，必须）

路径：`fs/archive/meetings/{meeting_id}/decision.md`

```md
# Decision — {topic}

<a id="dec-{meeting_id}-{seq}"></a>
<!--meta
type: decision
meeting_id: mtg-000001
decision_id: dec-001
ts: 2026-01-28T09:30:00+08:00
from_wb: wb-mtg-000001-001
from_props: [prop-mtg-000001-a03-r2-001]
sources: [S1,S2]
-->

decision: choose_a            # choose_a|choose_b|keep_parallel|defer_to_user|no_decision
summary: "一句话裁决结果"
rationale:
  - "理由1（应可回链到 from_props/sources）"
  - "理由2"
action_items:
  - ai-0001
open_questions:
  - "仍需用户确认的点（如有）"
dissent:
  - agent_id: a01
    note: "异议摘要（可选）"
```

**规则（必须）**
- `decision` 必须能回链到 `from_wb`（至少一个 wb）。
- `action_items` 必须引用 `action_items.yaml` 的 `items[].item_id`。

### 8.6 meeting_brief.md（归档，最小模板，必须）

路径：`fs/archive/meetings/{meeting_id}/meeting_brief.md`

```md
# Meeting Brief — {topic}

<a id="brief-{meeting_id}"></a>
<!--meta
type: meeting_brief
meeting_id: mtg-000001
ts: 2026-01-28T09:30:00+08:00
-->

## 1) 一句话结论
- …

## 2) 本次 focus（问题）
- …

## 3) 已收敛结论（指向白板/决策）
- wb: wb-mtg-000001-001 → dec: dec-001

## 4) 下一步（行动项）
- ai-0001：…（need_approval_by=planner）

## 5) 关键背景（仅保留必要上下文）
- …
```

**规则（必须）**
- meeting_brief 必须在不读全文 transcript 的情况下，支持 Planner/Editor 继续工作（只保留最小必要上下文）。

### 8.7 action_items.md（归档，渲染规则，必须）

路径：`fs/archive/meetings/{meeting_id}/action_items.md`

**规则（必须）**
- 必须由 `fs/meetings/{meeting_id}/artifacts/action_items.yaml` 渲染生成（不可人工改写语义）。
- 每条必须包含：`item_id`、`kind`、`summary`、`need_approval_by`、`priority`、以及 `rationale.from_wb`（或 from_props）。

---

## 9) 最小实现契约（给工程落地）

必须写死的两条契约（不满足即不可验收）：
1) Proposal/Speak/Settled 的**最小字段 + 锚点命名规则**（rg 可定位、解析器可解析）
2) Recorder 单写者 + **ACL 路径白名单**（共享区与归档区只允许 Recorder 写）

---

## 文件名：docs/07_convergence_gates.md

# 07 收敛门 Convergence Gate：把“会议式收敛”嵌入任务流水线（不牺牲吞吐）

> 目的：任务系统保持“并行产出 + 合并收敛”的高吞吐；仅在必要时引入“会议式回合”作为 **收敛门（Gate Node）** 解决分歧与口径一致。

---

## 7.1 结论判定（强约束）

- **不建议**把整个任务流改成会议式轮次（会显著降低吞吐）。
- **推荐**把会议抽象为一种通用节点：**Convergence Gate（收敛门）**，在少数关键点触发，用极短回合完成裁决，再回到并行生产。

---

## 7.2 三类节点（统一到同一条 TaskSpec 语义）

任务流水线只需要三类节点即可覆盖绝大多数场景：

1) **Work Node（默认）**  
- 目标：并行生产内容（每个 Agent 写自己的文件，互不冲突）。  
- 产物：Summary / Full / 可选 Cards（按触发规则）。

2) **Gate Node（收敛门 / 微型会议）**  
- 目标：裁决分歧、锁定口径、更新计划（不是产出大段正文）。  
- 产物：`gate_decision.md`（单写者：Moderator/Editor/Planner）  
- 约束：最多 1~2 轮，输入极少，输出极硬。

3) **Review Node（Verifier/QA）**  
- 目标：质量与一致性检查；失败触发返工或 Gate。  
- 产物：`issue_list.md` / `verification_report.md`

> 说明：会议模式（06_meeting_mode.md）是一种“泛化的 Gate Node”，适用于复杂、开放、争议大的场景；但在任务流水线中默认使用“微型 Gate”。

---

## 7.3 Gate Node 的输入/输出契约（最小可执行）

### 输入（程序下发，不靠 Agent 自由扩写）
- `focus`: 当前必须收敛的问题（一个句子）
- `options`: 允许的选项列表（A/B/C...）
- `must_answer`: 受影响的 must-answer 列表（引用其锚点）
- `evidence_refs`: 允许引用的证据范围（文件路径 + 锚点）
- `constraints`: 时间/成本/合规/风格等硬约束
- `round_budget`: 轮次与长度上限（默认 1 轮；必要时 2 轮）

### 输出（唯一产物：gate_decision）
`gate_decision.md` 必须包含：

- `decision`: 选项（choose_a / choose_b / keep_parallel / defer_to_user）
- `rationale`: 3~5 条理由（必须可追溯到 evidence_refs）
- `updates`: 对 TaskSpec 的更新（新增/修改 must-answer、约束、分工、并行度）
- `action_items`: 下一步要补的证据/任务（生成新的 Work Nodes 或返工）
- `dissent`: 可选异议记录（用于审计与后续复盘）

---

## 7.4 Gate Node 的“发言串行 + 思考并行”但不拖慢

### 7.4.1 “投递包”代替自由群聊
Gate 不做自由聊天，参与者仅提交短包（每人一个文件）：

- `position_packet_<agent>.md`（很短）
  - 立场（支持 A/B/并行）
  - 关键证据锚点（只引用，不复制长文）
  - 风险点（最多 3 条）
  - 需要的裁决点（如有）

Moderator 只读这些短包 + 证据锚点，然后单写 `gate_decision.md`。

### 7.4.2 回合限制（防“会议化拖慢”）
- 默认 **1 轮**：所有人并行投递 position_packet → Moderator 裁决  
- 仅在证据不足时进入 **第 2 轮**：补证据/反驳（仍是短包）  
- 超过 2 轮必须升级为完整会议模式（06_meeting_mode.md）

---

## 7.5 何时触发 Gate（程序规则：写死，避免模型分裂）

以下任一条件满足即触发 Gate Node：

1) **Fork 影响交付**：出现方向性分叉，且会影响成本/时间/交付物结构  
2) **Verifier blocker**：Review Node 给出 `severity=blocker`  
3) **口径冲突**：同一 must-answer 下出现互斥 claim（可用本地检索判定）  
4) **高风险输出进入定稿**：合同/财务/对外材料在“final 前”必须过 Gate  
5) **用户打回（大版本）**：用户否决方向，需要重新确立口径（见 05_user_interaction.md）

---

## 7.6 与“并行产出 + 合并器”的关系（关键回答）

- **并行产出**负责“生产内容”
- **Gate**负责“裁决与更新计划”
- **合并器**负责“组装成 IR/交付物”
- **Verifier**负责“质量与一致性兜底”

> 直观理解：Gate 是控制面；Work+Merge 是数据面。控制面少用但必须硬，数据面高吞吐。

---

## 7.7 目录与写权限（避免并行写入冲突）

- position_packet：各 Agent 写各自文件（并行安全）
- gate_decision：**只允许 Moderator 单写**
- TaskSpec 更新：由程序写入（或由 Moderator 写“patch 提案”，程序应用）

推荐路径：

- `runs/<run_id>/gate/<gate_id>/position_packets/position_packet_<agent>.md`
- `runs/<run_id>/gate/<gate_id>/gate_decision.md`

---

## 7.8 示例：报告+PPT 中的 Gate 放在哪里（最小例）

- Gate#1（早期）：确认报告结构与 PPT 口径是否一致（锁定 must-answer 与 outline）
- Work：并行撰写报告章节、并行准备 PPT 节点素材
- Review：检查事实引用与覆盖率
- Gate#2（定稿前）：对外口径/数据一致性裁决
- Merge/Transform：组装报告 → 生成 PPT IR → 渲染


---

## 文件名：docs/08_development_plan.md

# 08 多阶段落地开发计划（语言无关）

> 目标：把本仓库的“最小规范”落地成可运行系统，但**本文件不绑定任何语言/框架/存储**；只定义阶段、输出物、验收标准与决策点。  
> 适用范围：Meeting Mode v2（`docs/06_meeting_mode.md`）+ 与 Task/Gate 的集成。  
> 原则：每个阶段都必须能独立验收、可回滚、可审计。
>
> 状态标记：`[ ]` 未处理 / `[ / ]` 进行中 / `[x]` 已完成（注意：`[ / ]` 中间无空格时显示更紧凑，本文用 `[/]`）。

---

## 0) 总体策略（先写清楚）

### 0.1 交付优先级（建议）
1) **ACL + 单写者**（先把越权与冲突写死）  
2) **会议最小闭环**（能开会→能收敛→能输出 action_items）  
3) **注入任务流**（action_items → TaskSpec patch / 新任务）  
4) **检索与引用**（sources/citations/回放）  
5) **质量门与审计**（Verifier/coverage/日志）

### 0.2 统一“Definition of Done”（必须）
一个阶段完成，必须同时满足：
- 有明确输出物（文件/目录/协议样例）
- 有可执行的验收清单（手工或脚本均可）
- 不破坏上游协议（协议版本变更必须记录）
- 失败路径可解释（出 issue_list / decision）

---

## 1) Stage 0：规范冻结与命名统一（文档阶段）

- [x] Stage 0（整体）

**目标**
- 让“协议”与“索引/入口”一致可用，避免引用断裂与口径漂移。

**输出物（必须）**
- [x] `docs/06_meeting_mode.md`：会议模式 v2 规范（含 step-by-step 协议）
- [x] `docs/99_templates.md`：模板与 MeetingSpec 最小字段对齐
- [x] `docs/README.md`、`README.md`：入口与索引不再引用不存在文件

**验收标准（必须）**
- [x] `README.md`、`docs/README.md`、`FILE_STRUCTURE.md` 的会议入口统一指向 `docs/06_meeting_mode.md`
- [x] `docs/README.md` 中每个条目都存在对应文件

---

## 2) Stage 1：文件系统与写入 ACL（Tool Gateway 级）

- [x] Stage 1（整体）

**目标**
- 把“谁能写哪里、怎么写”从约定变成硬约束。

**工作项（建议顺序）**
- [x] 定义 Path ACL 规则：基于 `MeetingSpec.policy.allowed_write_prefixes` 与 append-only/single-writer 列表
- [x] 定义写入动作模型：`create|append|replace|mkdir|rename|delete`（默认 deny）
- [x] 定义共享区单写者：Recorder 身份认证与锁（逻辑锁即可）
- [x] 定义审计字段：who/what/where/when/result/linkage（见 `docs/00_scope_and_principles.md`）

**输出物（必须）**
- [x] 一份可实现的 ACL 规则说明（可以是文档或配置样例）：`docs/10_tool_gateway_acl.md`
- [x] 一套“违规示例 → 必须被拒绝”的用例清单（越权写 shared/、覆盖 transcript、写 deliver 等）

**验收标准（必须）**
- [x] 任意非 Recorder 角色对 `shared/**` 的写入会被拒绝并产出审计记录
- [x] append-only 文件的覆盖写会被拒绝

---

## 2.5) Stage 1.5：前端 AI 界面对接（AG-UI）

- [/] Stage 1.5（整体）

**目标**
- 把“UI 交互 + interrupt/resume”变成可执行契约：run 事件流 + 前端工具 + 中断恢复。
- 会议系统（Stage 2）可后延；先确保 UI 能驱动任务执行与产物落盘。

**工作项（建议顺序）**
- [x] 落库对接规范草案：`docs/11_ag_ui_integration.md`
- [x] 落库 IGI v1 资源模型（真相源）与承载规则：`docs/13_igi_v1_resource_model.md`
- [x] 定义 Thread 协作 API（Snapshot + Watch/Subscribe，协议先行）：`docs/14_igi_thread_api_v1.md`
- [x] 固化 IGI v1 contracts：`contracts/igi/v1/**`（Resource Envelope + Thread/Directive/ChangeSet/Artifact/Manifest...）
- [/] 固定事件契约：事件命名风格、必须字段、SSE 格式、重连 snapshot 规则
- [ ] 固定工具契约：`ui.*` 工具清单 + args/result schema（建议落到 `contracts/` 并版本化）
- [x] 定义 Thread 公共铺位（`fs/threads/{thread_id}`）：公共 state / inputs manifest / events（见 `docs/12_runtime_and_input_model.md`）
- [x] 定义“可控暂停开关”：允许当前 step 收尾 → 统一 interrupt → 下次 resume（含 change_request 落盘与追溯）
- [x] 定义大输入/附件处理策略：图片/PDF/URL/大量文件的入库与限流（max_files/bytes/url），run 只消费引用
- [x] 定义“run ↔ fs/落盘”的映射（run_id/thread_id 与 task_dir 的关系、事件日志 JSONL）
- [x] 定义 interrupt/resume 的落盘要求（interrupt payload 与 user decision 必须可追溯）

**输出物（必须）**
- [/] 一份可实现的 API 约定（/run + tool_result + resume 的最小集合）
- [x] 一份最小 demo 事件流样例（含 tool call 与 interrupt）

**验收标准（必须）**
- [ ] UI 侧可用 tool call 完成一次“确认/表单”交互，并让后端继续跑完 run
- [ ] 可在 interrupt 点挂起并 resume（下一次 run 可继续推进且可追溯）
- [ ] 用户输入（文件/URL 等）可先入库并在 run 中以引用形式消费（不把二进制塞进 state/run 元信息）

---

## 2.6) Stage 1.6：IGI Thread 协作控制面（实现 MVP）

- [x] Stage 1.6（整体）

**目标**
- 将“ThreadSnapshot + watch/subscribe”的协议落到可运行实现：UI 能看到全局一致状态（Directive/Progress/Controls），并能断线重连。

**工作项（建议顺序）**
- [x] 实现 Thread 资源的落盘与单写者更新：`fs/threads/{thread_id}/state.json`
- [x] 实现 Thread 事件日志（append-only）：`fs/threads/{thread_id}/events/events.jsonl`（至少记录：actor + subject + patch）
- [x] 实现 ThreadSnapshot 组装器（view）：`Thread + Directive + ArtifactManifest(可空) + recentChangeSets(可空) + progress`
- [x] 提供 `GET /apis/igi.zhanggui.io/v1/threads/{threadId}/snapshot`（非流式）
- [x] 提供 `GET /apis/igi.zhanggui.io/v1/threads/{threadId}/watch`（SSE）：首包 `STATE_SNAPSHOT`，后续 `STATE_DELTA`
- [x] 将 `/agui/run` 与 Thread 关联：run 启动/结束时更新 `Thread.status.activeRunId` 与 `phase`

**输出物（必须）**
- [x] 一套可回放的 thread 目录样例（真实 run 或示例数据均可）：`fs/threads/{thread_id}/...`
- [x] 一份最小端到端演示脚本/步骤（不要求 UI）：能订阅 watch，看见 progress 与 phase 变化（见 `docs/14_igi_thread_api_v1.md` §7）

**验收标准（必须）**
- [x] 订阅 watch 后能收到 `STATE_SNAPSHOT`（包含 `kind=ThreadSnapshot`）
- [x] 产生一次状态变化（progress/phase）后能收到至少一条 `STATE_DELTA`
- [x] 断线重连时可通过再发 snapshot 恢复（cursor 可后置，但必须可恢复）

---

## 2.7) Stage 1.7：Materials Pack（CAS）+ ChangeSet + 可控暂停（实现 MVP）

- [ ] Stage 1.7（整体）

**目标**
- 将“复杂用户输入（文件/图片/PDF/URL/大量资料）+ 追加需求”纳入系统：先入库（Artifact/Manifest），再变更（ChangeSet），触发可控暂停并通过 resume 继续。

**工作项（建议顺序）**
- [ ] 实现 Artifact 入库（CAS）：按 `sha256` 去重存储（本地 `fs/threads/{thread_id}/inputs/files/{sha256}`）
- [ ] 实现 ArtifactManifest 维护：更新清单而不是把内容塞进 run/state
- [ ] 实现 ChangeSet 创建与落盘（包含 inputRefs 与 requestedControl=drain_step）
- [ ] 实现“可控暂停开关”（线程级）：ChangeSet → `Thread.phase=PAUSE_REQUESTED` → run 在 step 边界收尾并 interrupt
- [ ] 实现 resume 后的 intake 步骤（最小）：记录 decision、更新 directive/constraints（可只落盘不改任务流）
- [ ] 限流策略生效：max_files/max_bytes/max_urls 超限时必须触发 tool call 让用户裁决（不允许静默丢弃）

**输出物（必须）**
- [ ] 一份材料包样例（包含至少 1 个 PDF 或图片 + 1 个 URL ref）：`fs/threads/{thread_id}/inputs/**`
- [ ] 一份变更单样例：`ChangeSet`（可在 thread events 中回放到创建原因与引用）

**验收标准（必须）**
- [ ] 用户上传文件后可生成 `Artifact(digest)` 并出现在 `ArtifactManifest`
- [ ] 创建 ChangeSet 后，watch 能看到 `Thread.phase=PAUSE_REQUESTED`
- [ ] 当前 run 能在安全点 interrupt，且下一次 resume 能继续推进并留下可追溯记录

---

## 3) Stage 2：会议最小闭环（MVP）

- [ ] Stage 2（整体）

**目标**
- 在不依赖 UI 的情况下，跑通一次会议：focus → proposal → speak → settle → outputs。

**必须支持的协议动作**
- [ ] 创建 `fs/meetings/{meeting_id}/spec.yaml`
- [ ] 初始化目录（shared/agents/artifacts）
- [ ] 写入 whiteboard 初始 focus
- [ ] 采集 proposals（至少能形成 Proposal Block）
- [ ] 记录 speaks（形成 Speak Block，写入 transcript.log）
- [ ] 写入 settled 结论（whiteboard）
- [ ] 生成三件套：`export_minutes.md` + `action_items.yaml` + `citations.yaml`

**输出物（必须）**
- [ ] 一次可回放的会议目录样例（可用真实 run 或示例数据）
- [ ] 三件套文件内容至少包含：锚点回链、sources 占位、下一步可执行项

**验收标准（必须）**
- [ ] 每条 whiteboard 结论都有 from_props + sources（允许 sources 为空但必须显式标注）
- [ ] meeting_brief 能在不读全文 transcript 的情况下让 Planner 继续派发任务

---

## 4) Stage 3：注入任务流（action_items → TaskSpec）

- [ ] Stage 3（整体）

**目标**
- 让会议成为“控制面”：不产长正文，但能更新 TaskSpec/计划与触发下一轮工作。

**工作项（建议顺序）**
- [ ] 定义 `action_items.yaml` 的最小 schema（新增 must_answer / 更新 constraints / 创建新 task / 触发 Gate）
- [ ] 定义 patch 应用规则：谁可批准、如何审计、失败如何回滚
- [ ] 定义与 Gate Node 的衔接：meeting 输出可触发 Gate 或直接创建 Work Nodes

**输出物（必须）**
- [ ] `action_items.yaml` schema（文档形式即可）
- [x] `PatchSpec v1`：task_patch 的补丁协议（见 `docs/06_meeting_mode.md` 的 `8.3.1`）
- [ ] 一组 action_items 示例（至少覆盖：update_constraints / add_task / terminate_team / major_restart 提议）

**验收标准（必须）**
- [ ] 任何对 case/spec 的修改都必须可追溯到 meeting_id + decision 段落
- [ ] 不能“静默改写”已有约束；必须记录 delta 与理由

---

## 5) Stage 4：检索与引用（可选但强烈建议）

- [ ] Stage 4（整体）

**目标**
- 让会议结论可被后续会议/任务复用；避免“结论漂移”。

**工作项（建议）**
- [ ] sources 编号规范（S1/S2...）与 citations.yaml 结构
- [ ] meeting_brief/decision 的可检索字段（固定行/front-matter）
- [ ] transcript/whiteboard 的 compaction 策略与快照索引

**输出物（建议）**
- [ ] `artifacts/citations.yaml` 示例与字段说明
- [ ] `shared/compaction/snap_0001.md` 示例（包含覆盖的 spk id 范围）

**验收标准（建议）**
- [ ] 给定某个结论，能定位到对应 proposal/speak/source 的锚点与文件路径

---

## 6) Stage 5：质量门与审计（Verifier）

- [ ] Stage 5（整体）

**目标**
- 让“协议是否被遵守”可自动检查，减少人工审查负担。

**工作项（建议）**
- [ ] 锚点唯一性检查（prop/spk/wb 不重复）
- [ ] whiteboard 结论必须回链（from_props）
- [ ] ACL/append-only/single-writer 违规检测
- [ ] action_items 的可执行性校验（字段缺失/非法路径/越权动作）

**输出物（必须）**
- [ ] 一份 Verifier 检查清单（可以是文档/伪代码/规则表）
- [ ] issue_list 的格式与严重级别定义（blocker/warn/info）

**验收标准（必须）**
- [ ] 违规即产出 issue_list（blocker），并阻止进入归档/注入阶段

---

## 7) Stage 6：安全与保留策略（上线前）

- [ ] Stage 6（整体）

**目标**
- 控制敏感信息与仓库膨胀风险，确保可长期运行。

**工作项（建议）**
- [ ] 对 transcript 的脱敏/标注策略（哪些字段禁止落盘）
- [ ] 归档策略（哪些进入 `fs/archive/**`，哪些只保留 brief）
- [x] `.gitignore` 策略：运行态数据默认不应污染仓库（`fs/**` 不入 git；如需要持久化，必须明确规则）

**验收标准（必须）**
- [ ] 满足最小合规：敏感信息不可被默认写入可检索层
- [ ] 满足可运维：会议目录增长可控（有归档与压缩策略）

---

## 文件名：docs/09_golang_development_plan.md

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

## 10) IGI Thread 协作控制面（Go 实现计划，正式开发入口）

> 说明：从“demo 能跑”进入“正式开发”时，建议优先落地 Thread 协作控制面。  
> 协议入口：`docs/13_igi_v1_resource_model.md`、`docs/14_igi_thread_api_v1.md`；Schema：`contracts/igi/v1/**`。

### T0（半天）：数据模型与落盘骨架
**交付**
- `fs/threads/{thread_id}/state.json`（ThreadSnapshot 或至少 Thread）落盘与更新（single-writer：系统）
- `fs/threads/{thread_id}/events/events.jsonl`（append-only）写入（至少记录：actor + subject + patch）
**验收**
- 给定 thread_id，能创建目录并写入一份最小快照；重复写入不破坏 append-only 约束

### T1（1 天）：Thread Snapshot + Watch（SSE）
**交付**
- `GET /apis/igi.zhanggui.io/v1/threads/{threadId}/snapshot`
- `GET /apis/igi.zhanggui.io/v1/threads/{threadId}/watch`（首包 `STATE_SNAPSHOT`，后续 `STATE_DELTA`）
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

---

## 文件名：docs/10_tool_gateway_acl.md

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

---

## 文件名：docs/11_ag_ui_integration.md

# 11 前端 AI 界面对接（AG-UI 协议：落库草案）

> 本文目标：把“前端 AI 界面”与 zhanggui 的对接方式先**落库**成可执行契约（但不保证一次写完）。  
> 当前定位：**对接规范的一部分**（先把 event/tools/interrupt-resume 的最小约束写清楚）。  
> 设计原则：UI 负责用户侧交互与外部动作触发；后端负责发起 run、产出事件流、落盘、审计、以及在工具结果回来后继续推进。
>
> v1 重要口径（必须）：**IGI（`apiVersion: igi.zhanggui.io/v1`）是系统内部“世界定义/真相源”**（见 `docs/13_igi_v1_resource_model.md`）；**AG-UI 只是 UI 对接的事件承载协议**。  
> 实践上：IGI 的资源快照/增量更新通过 AG-UI 的 `STATE_SNAPSHOT/STATE_DELTA`（以及必要时 `CUSTOM`）承载。

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
> 说明：我们不在此阶段做“协议转换层”（避免复杂度），但会保留 `raw_event` 与版本字段，为后续升级/转换留钩子。

### 3.2 SSE 包装（建议）
每个事件一条 SSE message，`data` 为完整 JSON：

```text
event: agui
data: {"type":"RUN_STARTED","thread_id":"t1","run_id":"r1", ...}

```

客户端必须以 JSON 内的 `type` 作为最终判定依据。

### 3.3 通用事件外壳（最小字段）
每个事件对象至少包含：
- `type`：事件类型
- `timestamp`：RFC3339 时间戳（建议；用于排序与审计）
- `runId`：运行 ID（建议；用于重连与追溯）
- `threadId`：线程/会话 ID（建议；用于 UI 会话归并）

> 兼容字段：`raw_event` 可保留上游原始事件（透传/调试）。

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
  "thread_id": "t1",
  "run_id": "r1",
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
  "thread_id": "t1",
  "run_id": "r2",
  "resume": {
    "interrupt_id": "int-0001",
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
- **版本演进**：协议可能变化；本阶段不做转换层，但必须保留 `raw_event` 与 `protocol_version` 等字段以便追溯与迁移。

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

---

## 文件名：docs/12_runtime_and_input_model.md

# 12 运行时与输入模型（v1：协程 Agent + 可控暂停开关 + 输入落盘）

> 本文目标：把“并行 Agent（Go 协程）如何跑、如何暂停/变更、用户输入如何落盘与引用”先落成 **v1 可执行规范**。  
> 定位：总体性内容；会议（Meeting Mode）与具体任务流水线可后延，但需要共享同一套最小运行信息与输入协议。  
> 兼容性：对外事件承载采用 AG-UI（见 `docs/11_ag_ui_integration.md`）；系统内部“世界定义/真相源”采用 IGI 资源模型（`apiVersion: igi.zhanggui.io/v1`，见 `docs/13_igi_v1_resource_model.md`）。本文聚焦 **运行时机制** 与 **落盘约束**。

---

## 0) 关键决策（v1 写死，后续可在变更记录中调整）

1) **Agent 在 Go 内部用协程（goroutine）表达**：v1 不引入“worker 间协议/网络通信”。  
2) **暂停/用户决策默认采用“可控开关：允许当前 step 收尾 → 统一中断（interrupt）→ 下次 resume 继续”**。  
3) **会议与任务先不强行收敛成同一种 Task**：可以两条线推进，但必须共享“Thread/Run/Artifact(Inputs)/Control”这套公共基座（见 §1）。  
4) **命名口径**：本文中的 Thread/Run/Inputs/change_request 等概念，最终都应落到 IGI 的 kind（`Thread/Run/Artifact/ArtifactManifest/ChangeSet/...`），避免未来多系统接入时语义漂移。

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
> IGI 对齐建议：  
> - `inputs/files/{sha256}` 可视为 `kind=Artifact` 的本地存储形态（内容寻址）。  
> - `inputs/manifest.json` 可视为 `kind=ArtifactManifest` 的本地快照形态（集合清单）。  
> - “用户追加需求/暂停变更”应落为 `kind=ChangeSet`，而不是只停留在聊天文本。

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

---

## 文件名：docs/13_igi_v1_resource_model.md

# 13 IGI v1：资源模型（Canonical）与对外承载（AG-UI）

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

v1 必须在 `contracts/igi/v1/` 固化 JSON Schema（最小集合）：
- 资源外壳（Resource Envelope）
- `Thread/Directive/ChangeSet/Artifact/ArtifactManifest/AgentStatus/Bundle`
- `ThreadSnapshot`（Thread 协作视图：snapshot + watch/subscribe；见 `docs/14_igi_thread_api_v1.md`）

并遵循兼容性规则：
- v1 内新增字段只能加到 `metadata.ext/spec.ext/status.ext` 或新增 optional 字段
- 破坏性变更必须升 `igi.zhanggui.io/v2`

---

## 文件名：docs/14_igi_thread_api_v1.md

# 14 IGI v1：Thread API（Snapshot + Watch/Subscribe，协议先行）

> 本文目标：定义 **Thread 级协作 API**（不实现，先定协议）：  
> - `ThreadSnapshot`：UI/系统一次性拿到“全局一致目标 + 输入资料包 + 变更包 + 全员进度”的状态形状  
> - `watch/subscribe`：持续订阅 thread 的状态增量（pause/change/directive/progress）  
>
> 口径：IGI（`apiVersion: igi.zhanggui.io/v1`）是 **真相源**（见 `docs/13_igi_v1_resource_model.md`）；对外事件承载优先复用 AG-UI（见 `docs/11_ag_ui_integration.md`）。

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

---

## 文件名：docs/15_round_based_delivery.md

# 15 回合制交付（Round-based Delivery）与 Profile（统一代码/文稿任务）

> 本文目标：把“文本任务 vs 代码任务”统一抽象到同一条主线：**回合制交付**。  
> 本文不是实现细节，而是用于：对齐术语、对齐状态机、对齐 UI 展示与后续扩展方向。  
>
> 与本仓库关系：  
> - 真相源：IGI（`apiVersion: igi.zhanggui.io/v1`，见 `docs/13_igi_v1_resource_model.md`）  
> - 对外承载：AG-UI（见 `docs/11_ag_ui_integration.md`）  
> - Thread 协作 API：`ThreadSnapshot + watch/subscribe`（见 `docs/14_igi_thread_api_v1.md`）

---

## 0) 结论（先讲清楚）

1) **代码任务与文稿任务不需要两套系统**：它们都是“从立意到交付的一串回合（Rounds）”。  
2) 差异不在流程，而在“门槛/制度”：验收更严格、回合更多、Gate 更重。  
3) 因此我们引入两个概念：
- **Round（回合）**：一次推进循环（派发→执行→收集→验收→裁决）。  
- **Profile（画像/制度参数）**：不同任务类型的规则集合（产物要求、验收规则、Gate 策略、interrupt 策略）。

> v1 实践：Round 可直接映射为一次 `Run`（AG-UI run lifecycle），Profile 先作为配置/契约存在（不要求运行态实现）。

---

## 1) 统一主线（七步链）

建议对外统一叙事链路（便于 UI 展示与团队沟通）：

> 立意 → 立契 → 立据 → 立行 → 立验 → 立交 → 立账

对应到 IGI/协议概念（v1 映射）：
- 立意：`Thread`（稳定锚点）+ `Directive`（当前“思想状态/目标/约束”，可迭代 revision）
- 立契：`ChangeSet`（追加需求/变更包）与（后续）任务侧 TaskSpec/patch
- 立据：`Artifact` + `ArtifactManifest`（资料包/CAS）
- 立行：`Run`（一次执行 = 一回合）+ `Thread.status.progress`
- 立验：Verifier/Gate（先产出 issues/decision，再决定下一回合）
- 立交：`Bundle`（pack 输出）
- 立账：`events.jsonl` + `tool_audit.jsonl`（事实层/审计层）

---

## 2) Round（回合）定义（v1：映射为 Run）

### 2.1 回合循环（固定节奏）
一个回合建议包含以下阶段（可作为 run 的 stepName 口径）：
- `ROUND_START`
- `DISPATCH`（派发 work units/agents）
- `EXECUTE`（并行执行）
- `COLLECT`（收集产物/证据）
- `EVALUATE`（评审/验收：由 Profile 决定规则）
- `DECIDE`（裁决：继续/返工/暂停/结束）
- `ROUND_END`

### 2.2 v1 的最小落地方式
- Round 不需要新增 IGI kind 才能跑：**将每次推进做成一次 `Run`**。  
  - 优点：天然复用 AG-UI run lifecycle + interrupt/resume；日志与审计自然落到 `fs/runs/{run_id}`。  
  - 后续若要更强表达力，可在 v2 引入 `kind=Round`（不影响 v1）。

---

## 3) Profile（画像/制度参数）

### 3.1 为什么需要 Profile
Profile 用于把“任务类型差异”收敛到制度参数，避免出现两套流程：
- 文稿任务：回合少、验收偏格式/事实/引用一致性
- 代码任务：回合多、验收偏 lint/test/coverage/security/merge/gate

### 3.2 Profile 最小字段建议（契约层）
> v1 建议先以配置文件/contract 存在（后续可升级为 `kind=TaskProfile`）。

- `round_policy`：回合推进策略（并行数、checkpoint 频率、超时）
- `artifact_policy`：必须产出哪些 artifacts（summary/issues/patch/test_report/coverage...）
- `evaluation_policy`：验收规则集合（schema 校验、CI、静态分析、安全扫描等）
- `gate_policy`：Gate 的收敛策略（合并方式、审批人、阻断条件）
- `interrupt_policy`：哪些节点必须 interrupt（发布/合并/对外出货）
- `definition_of_done`：DoD（通过条件）

---

## 4) 与 ThreadSnapshot 的关系（UI 如何展示）

UI 不需要理解“内部执行细节”，只需要：
- 从 `ThreadSnapshot` 读 `Directive`（北极星）与 `ArtifactManifest`（资料包）
- 从 `ThreadSnapshot` 读 `Thread.status.progress`（全员进度）
- 从 `Run` 的事件流读“当前回合细节”（step/text/tool/interrupt）

建议 UI 分两层：
1) **Thread 面板（全局）**：北极星/资料包/进度/控制状态
2) **Run 面板（当前回合）**：本回合的 step timeline、日志、工具交互与 interrupt

---

## 5) v1 兼容性（不引入破坏性变更）

- 不要求新增 kind 即可落地：Round=Run、Profile=contract/config。  
- 协议扩展一律走 `ext` 或新增 optional 字段；破坏性变更升 `igi.zhanggui.io/v2`。  
- 现有 AG-UI demo 与 Tool Gateway 机制保持可用（无需重写）。


---

## 文件名：docs/16_igi_inputs_changesets_api_v1.md

# 16 IGI v1：Inputs（CAS）与 ChangeSet API（本地单机实现）

> 本文目标：把 Stage 1.7 的“复杂输入入库（CAS）+ 变更单（ChangeSet）+ 线程暂停请求（PAUSE_REQUESTED）”落成 **可调用的 v1 API**。  
> 口径：IGI（`apiVersion: igi.zhanggui.io/v1`）是 **真相源**；UI/前端事件承载仍走 AG-UI（见 `docs/11_ag_ui_integration.md`、`docs/14_igi_thread_api_v1.md`）。  
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


---

## 文件名：docs/99_templates.md

# 99 最少模板（可选用，不强制）

> 收敛结论：**元字段由程序下发**，Agent 产物文件只写 `task_spec_ref`（或 task_id）+ 正文。  
> 位置锚点：由生成器自动写入，在 Markdown 中插入 HTML Anchor（`<a id=...></a>`）+ 注释 meta，放在区块标题前。

---

## A) TaskSpec（程序生成，唯一真相源）

路径示例：`tasks/task-000123/spec.yaml`

```yaml
schema_version: 1
run_id: run-...
task_id: task-000123
team_id: team_a

agent:
  agent_id: a-writer-01
  role: writer

assignment:
  assigned_outline_nodes: [2, 4]
  assigned_must_answer: [2, 5]
  reuse_targets: [report, ppt]

refs:
  constraints_ref: ../spec.md#constraints
  must_not_ref: ../spec.md#acceptance.must_not

outputs:
  summary_path: revs/{rev}/summary.md
  cards_path: revs/{rev}/cards.md
  issues_path: revs/{rev}/issues.md

policy:
  write_policy: exact_paths_only
  allowed_prefixes: ["revs/{rev}/"]
```

### current.yaml（程序维护：读取最新修订）
路径示例：`tasks/task-000123/current.yaml`

```yaml
current_rev: r2
```

---

## B) Summary（必交，Agent写）

路径示例：`tasks/task-000123/revs/r2/summary.md`

```md
task_spec_ref: ../spec.yaml

一句话结论：
- ...

要点（5~10条）：
- ...

边界/假设：
- ...

风险/不确定点：
- ...

需要裁决（如有）：
- ...

可直接复用句子（可选）：
- ...
```

> 自评字段（可选）：Agent 可以在正文末尾加 `agent_confidence: 0.xx`。  
> 覆盖度/客观评分：由 Verifier/主编另行产出（例如 `deliver/manifest.md` 或 task 的 `review.md`）。

---

## C) Cards（按需，Agent写）

路径示例：`tasks/task-000123/revs/r2/cards.md`

```md
task_spec_ref: ../spec.yaml

# card_id: (可留空由程序补)
claim: ...
evidence: ...
conditions: ...
tradeoffs: ...
---
# card_id: ...
...
```

---

## D) issues（可选，Agent 或 Verifier 写）

路径示例：`tasks/task-000123/revs/r2/issues.md`

```md
task_spec_ref: ../spec.yaml

- severity: blocker|warn|info
  where: content|transform|adapter|render|verify
  what: ...
  options: ...
  need_decision_by: Editor|User|Planner
  suggested_patch: ...
```

---

## E) review（审核/打回统一记录）

路径示例：`tasks/task-000123/review.md`

```md
- on_rev: r1
  by: editor-01
  decision: changes_requested|approved
  comments:
    - ...
  requested_actions:
    - ...
```

---

## F) 位置锚点 DSL（生成器写，区块前置）

生成器在最终交付物（例：`deliver/report.md`）每个区块前插入：

```md
<a id="block-deliver-report-2"></a>
<!--meta task=task-000123@r2 sources=task-000120@r1,task-000121@r1-->
## 2. 并行与资源池
...
```

引用跳转（跨文件稳定）：

```md
见：[第2章](deliver/report.md#block-deliver-report-2)
```

约定：
- `id` 命名：`block-<scope>-<deliverable>-<node>`
- `id` 不带版本号；版本/来源写在 `<!--meta ...-->`
- 禁止全局 index 路由表；引用一律使用 `path#id` 或按文件+rg 查 meta。

---

## G) 检索输出（合并器消费，BM25 默认）
路径建议：`deliver/retrieval_hits.json`（或在运行时直接返回，不要求持久化）

每条命中必须包含：
```json
{
  "query": "并行上限 如何回收配额",
  "hits": [
    {
      "task_id": "task-000123",
      "rev": "r2",
      "file": "tasks/task-000123/revs/r2/summary.md",
      "chunk_id": "tasks/task-000123/revs/r2/summary.md#3",
      "start_line": 12,
      "end_line": 22,
      "score_bm25": 18.42,
      "text": "..."
    }
  ]
}
```

---

## H) manifest / coverage_map / issue_list（合并阶段三件套，必须）

### H.1 deliver/manifest.yaml
```yaml
refs:
  - ref_id: ref-0001
    used_in: block-deliver-report-2
    task_id: task-000123
    rev: r2
    file: tasks/task-000123/revs/r2/summary.md
    chunk_id: tasks/task-000123/revs/r2/summary.md#3
    start_line: 12
    end_line: 22
    sha256: "..."
```

### H.2 deliver/coverage_map.yaml
```yaml
must_answer:
  - id: 2
    status: covered
    evidence_refs: [ref-0001]
  - id: 5
    status: missing
    evidence_refs: []
```

### H.3 deliver/issue_list.md
```md
- severity: blocker
  where: verify
  what: "must-answer 5 未覆盖"
  action: "触发 Cards：让相关任务补充 tradeoffs/conditions，并产出可引用证据片段"
```


---

## M) MeetingSpec（程序生成）

路径示例：`fs/meetings/mtg-000001/spec.yaml`

```yaml
schema_version: 1
meeting_id: mtg-000001
type: planning   # planning/fork/blocker/review
topic: "数据处理架构选型"

participants:
  - { role: moderator, agent_id: pm }
  - { role: recorder, agent_id: recorder-01 }
  - { role: retriever, agent_id: retriever-01 }
  - { role: critic, agent_id: critic-01 }
  - { role: specialist, agent_id: architect-01 }

context_refs:
  - path: fs/cases/{case_id}/versions/v2/spec.md
  - path: fs/cases/{case_id}/versions/v2/tasks/task-000123/spec.yaml
  - path: fs/cases/{case_id}/versions/v2/tasks/task-000123/revs/r2/issues.md

limits:
  think_timeout_s: 12
  speak_max_chars: 600
  max_rounds: 5
  max_minutes: 15

queue_policy:
  priority_formula: "role_weight + intent_bonus + fairness"
  intents: [position, rebuttal, question, supplement, summary]

outputs:
  transcript_path: shared/transcript.log
  whiteboard_path: shared/whiteboard.md
  queue_path: shared/hand_queue.json
  decisions_path: shared/decisions.md
  minutes_path: artifacts/export_minutes.md
  action_items_path: artifacts/action_items.yaml
  citations_path: artifacts/citations.yaml

policy:
  append_only_files: ["shared/transcript.log","shared/decisions.md"]
  allowed_write_prefixes: [""]
  single_writer_prefixes: ["shared/","artifacts/"]
  single_writer_roles: ["recorder"]
  lock_file: "shared/.writer.lock"
  audit_fields: [who, what, where, when, result]
```

### Meeting 产物最小要求（Agent写正文，meta由程序写）
- `shared/transcript.log`：发言记录（追加式）
- `shared/whiteboard.md`：options/concerns/decision_draft（单写者维护）
- `shared/decisions.md`：决策清单（追加式）
- `shared/hand_queue.json`：（可选）举手/发言队列（单写者维护）
- `artifacts/export_minutes.md`：会后纪要（单写者聚合）
- `artifacts/action_items.yaml`：行动项（供任务系统接入）
- `artifacts/citations.yaml`：sources 清单（供审计/引用回链）

### Meeting 归档最小要求（必须）
- `fs/archive/meetings/{meeting_id}/meeting_brief.md`：最小上下文（供后续 Planner/Editor 复用）
- `fs/archive/meetings/{meeting_id}/decision.md`：最终裁决（必须回链 wb/prop/source）
- `fs/archive/meetings/{meeting_id}/action_items.md`：从 `artifacts/action_items.yaml` 渲染生成（审计/复盘用）

---

## 文件名：docs/proposals/audit_acceptance_ledger_v1.md

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
- 不把 IGI 的全量资源模型落盘成对象树（保留协议兼容，运行时只落必要子集）。
- 不引入外部数据库作为真相源（SQLite 仅作为可重建索引，放到后续）。

## 2. 三件套与责任边界

- `state.json`：**当前快照**（UI/快速读/断点恢复），允许覆盖写；不作为审计依据。
- `ledger/events.jsonl`：**验收账本**（append-only），所有“可审计事实”都落这里；这是 v1 的真相源。
- Trace（OTel）：**时序/性能透视**，可选；通过 `trace_id/span_id` 与账本互链，但不承担审计证明。

> 说明：仓库现有 `events/events.jsonl` 常被用于 SSE 回放/协议事件流（例如 AG-UI/IGI）。  
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
- **IGI-lite 对齐**：把 `correlation/actor/refs/trace` 与对外 IGI/AG-UI 协议互映射。

---

## 文件名：docs/README.md

# Docs 索引

入口建议：先读 `../FILE_STRUCTURE.md`，再按需读下面章节。

## 核心
- `00_scope_and_principles.md`：范围与原则（不落库、不写代码阶段）
- `01_minimal_kernel.md`：最小内核（Master IR）+ 渐进式交付
- `02_planning_and_parallelism.md`：计划与并行（MPU / spawn / 配额与调度）
- `03_artifact_pipeline.md`：多交付物与强协议流水线（Transformer/Adapter/Renderer/Verifier）
- `04_walkthrough_report_ppt.md`：纸面演练（报告 + PPT）
- `05_user_interaction.md`：用户中途指令处理硬流程
- `15_round_based_delivery.md`：回合制交付（Round-based Delivery）与 Profile（统一代码/文稿任务）

## 会议（v2 保留）
- `06_meeting_mode.md`：会议模式 v2（上下文工程 + 协议 + 单写者/锚点）
- `07_convergence_gates.md`：收敛门 Gate Node（把“会议式收敛”嵌入任务流水线）

## 落地
- `08_development_plan.md`：多阶段落地开发计划（语言无关）
- `09_golang_development_plan.md`：Go 本地单跑执行器开发计划（沙箱 + 落盘 + zip）
- `10_tool_gateway_acl.md`：Tool Gateway（写入 ACL + 单写者 + 审计）落地规范（Stage 1）
- `proposals/audit_acceptance_ledger_v1.md`：审计/验收证据链 v1（Bundle + ledger + evidence + evidence.zip）
- `proposals/acceptance_criteria_v1.yaml`：验收标准 v1（docs 固定来源；运行时需快照冻结）
- `11_ag_ui_integration.md`：前端 AI 界面对接（AG-UI：events/tools/interrupt-resume 草案）
- `12_runtime_and_input_model.md`：运行时与输入模型（v1：协程 Agent + 可控暂停开关 + 输入落盘）
- `13_igi_v1_resource_model.md`：IGI v1（`apiVersion: igi.zhanggui.io/v1`）：资源模型（真相源）与通过 AG-UI 承载的映射规则
- `14_igi_thread_api_v1.md`：IGI v1 Thread API（ThreadSnapshot + watch/subscribe；协议先行）
- `16_igi_inputs_changesets_api_v1.md`：IGI v1 Inputs（CAS）与 ChangeSet API（本地单机实现）

## 模板
- `99_templates.md`：最少模板（TaskSpec / Summary / Cards / issues / MeetingSpec 等）

