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
  allowed_write_prefixes: ["fs/meetings/mtg-000001/"]
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
