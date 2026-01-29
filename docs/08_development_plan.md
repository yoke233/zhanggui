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
- [x] （归档）IGI 草案与资源模型/API 参考：`docs/archive/igi/README.md`
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

## 2.6) Stage 1.6：Thread 协作控制面（实现 MVP）

- [x] Stage 1.6（整体）

**目标**
- 将“ThreadSnapshot + watch/subscribe”的协议落到可运行实现：UI 能看到全局一致状态（Directive/Progress/Controls），并能断线重连。

**工作项（建议顺序）**
- [x] 实现 Thread 资源的落盘与单写者更新：`fs/threads/{thread_id}/state.json`
- [x] 实现 Thread 事件日志（append-only）：`fs/threads/{thread_id}/events/events.jsonl`（至少记录：actor + subject + patch）
- [x] 实现 ThreadSnapshot 组装器（view）：`Thread + Directive + ArtifactManifest(可空) + recentChangeSets(可空) + progress`
- [x] 提供 `GET {threads_base}/threads/{threadId}/snapshot`（非流式；base 可配置，建议默认 `/threads`）
- [x] 提供 `GET {threads_base}/threads/{threadId}/watch`（SSE）：首包 `STATE_SNAPSHOT`，后续 `STATE_DELTA`
- [x] 将 `/agui/run` 与 Thread 关联：run 启动/结束时更新 `Thread.status.activeRunId` 与 `phase`

**输出物（必须）**
- [x] 一套可回放的 thread 目录样例（真实 run 或示例数据均可）：`fs/threads/{thread_id}/...`
- [x] 一份最小端到端演示脚本/步骤（不要求 UI）：能订阅 watch，看见 progress 与 phase 变化

**验收标准（必须）**
- [x] 订阅 watch 后能收到 `STATE_SNAPSHOT`
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
