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
- `contracts/**`：对接契约与 schema（JSON/YAML；版本化；用于前后端/工具对接；例如 `contracts/ag_ui/*`）
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
          manifest.json             # 输入清单（系统维护；建议 single-writer + events 追加记录）
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

## 6) README 归档（迁移自 README.md）

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
- `docs/archive/igi/README.md`：IGI 草案（已归档；当前主线 v1 不要求实现）

文件系统存储区（运行态数据）：`fs/**`（目录结构以 `FILE_STRUCTURE.md` 为准）

## 构建与测试（建议在 WSL 跑 -race）

### 依赖（WSL）
- Go（版本以 `go.mod` 为准）
- 启用 `-race` 需要 C 编译器：`gcc`/`clang`（Ubuntu 可用 `sudo apt-get install -y build-essential`）

### 一键测试脚本（WSL）
```bash
set -euo pipefail

go test ./...
go test -count=1 ./...            # 关闭测试缓存（排查用）
go test -race ./...               # 并发/调度相关建议必跑
```

### 常用命令
- `go build ./cmd/taskctl`
- `go build ./cmd/zhanggui`
- `go test ./... -run TestName -count=1`
- `go run ./cmd/taskctl run --sandbox-mode local --workflow demo04 --approval-policy always`
- `go run ./cmd/taskctl approve grant <task_dir>`
