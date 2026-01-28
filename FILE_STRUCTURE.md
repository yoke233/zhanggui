# 文件结构（单项目：任务系统 + 会议系统）

> 目标：把“任务系统（case/tasks/deliver）”与“会议系统（meetings）”的**所有可追溯文件**放在同一个项目里，并且在**基于文件系统**的前提下，做到：
> - **不并行改同一份共享文件**（避免冲突、避免口径漂移）
> - 允许通过 **append / 新增文件** 的方式“叠加”产出（高并行、可回放）
> - 少数“可维护可编辑”的文件，只有少数角色/人员有权限

本文件是**唯一权威**的目录结构与写入权限约定；其他文档如有冲突，以此为准。

---

## 1) 顶层分区（强约束）

本项目只分两类内容：

- `docs/**`：规范与设计文档（少数维护者可编辑；避免多人并行改同一文件）
- `fs/**`：文件系统存储区（运行态数据：case、task、meeting、run、archive）

> 说明：`fs/` 下默认采用“**只追加/只新建**”的写策略；需要覆盖写的共享文件必须遵守单写者原则。

---

## 2) 目录结构（权威树）

```text
./
  README.md
  FILE_STRUCTURE.md
  CHANGELOG.md
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

    runs/
      {run_id}/
        gate/
          {gate_id}/
            position_packets/
              position_packet_{agent_id}.md
            gate_decision.md        # 单写者（Moderator/Editor/Planner）
        logs/
          tool_audit.jsonl          # 追加式审计日志（系统写）

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
- `fs/runs/**`：默认 `append-only`；`gate_decision.md` 为 `single-writer`
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
