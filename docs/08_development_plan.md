# 08 多阶段落地开发计划（语言无关）

> 目标：把本仓库的“最小规范”落地成可运行系统，但**本文件不绑定任何语言/框架/存储**；只定义阶段、输出物、验收标准与决策点。  
> 适用范围：Meeting Mode v2（`docs/06_meeting_mode.md`）+ 与 Task/Gate 的集成。  
> 原则：每个阶段都必须能独立验收、可回滚、可审计。

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

**目标**
- 让“协议”与“索引/入口”一致可用，避免引用断裂与口径漂移。

**输出物（必须）**
- `docs/06_meeting_mode.md`：会议模式 v2 规范（含 step-by-step 协议）
- `docs/99_templates.md`：模板与 MeetingSpec 最小字段对齐
- `docs/README.md`、`README.md`：入口与索引不再引用不存在文件

**验收标准（必须）**
- `README.md`、`docs/README.md`、`FILE_STRUCTURE.md` 的会议入口统一指向 `docs/06_meeting_mode.md`
- `docs/README.md` 中每个条目都存在对应文件

---

## 2) Stage 1：文件系统与写入 ACL（Tool Gateway 级）

**目标**
- 把“谁能写哪里、怎么写”从约定变成硬约束。

**工作项（建议顺序）**
1) 定义 Path ACL 规则：基于 `MeetingSpec.policy.allowed_write_prefixes` 与 append-only/single-writer 列表
2) 定义写入动作模型：`create|append|replace|mkdir|rename|delete`（默认 deny）
3) 定义共享区单写者：Recorder 身份认证与锁（逻辑锁即可）
4) 定义审计字段：who/what/where/when/result/linkage（见 `docs/00_scope_and_principles.md`）

**输出物（必须）**
- 一份可实现的 ACL 规则说明（可以是文档或配置样例）
- 一套“违规示例 → 必须被拒绝”的用例清单（越权写 shared/、覆盖 transcript、写 deliver 等）

**验收标准（必须）**
- 任意非 Recorder 角色对 `shared/**` 的写入会被拒绝并产出审计记录
- append-only 文件的覆盖写会被拒绝

---

## 3) Stage 2：会议最小闭环（MVP）

**目标**
- 在不依赖 UI 的情况下，跑通一次会议：focus → proposal → speak → settle → outputs。

**必须支持的协议动作**
1) 创建 `fs/meetings/{meeting_id}/spec.yaml`
2) 初始化目录（shared/agents/artifacts）
3) 写入 whiteboard 初始 focus
4) 采集 proposals（至少能形成 Proposal Block）
5) 记录 speaks（形成 Speak Block，写入 transcript.log）
6) 写入 settled 结论（whiteboard）
7) 生成三件套：`export_minutes.md` + `action_items.yaml` + `citations.yaml`

**输出物（必须）**
- 一次可回放的会议目录样例（可用真实 run 或示例数据）
- 三件套文件内容至少包含：锚点回链、sources 占位、下一步可执行项

**验收标准（必须）**
- 每条 whiteboard 结论都有 from_props + sources（允许 sources 为空但必须显式标注）
- meeting_brief 能在不读全文 transcript 的情况下让 Planner 继续派发任务

---

## 4) Stage 3：注入任务流（action_items → TaskSpec）

**目标**
- 让会议成为“控制面”：不产长正文，但能更新 TaskSpec/计划与触发下一轮工作。

**工作项（建议顺序）**
1) 定义 `action_items.yaml` 的最小 schema（新增 must_answer / 更新 constraints / 创建新 task / 触发 Gate）
2) 定义 patch 应用规则：谁可批准、如何审计、失败如何回滚
3) 定义与 Gate Node 的衔接：meeting 输出可触发 Gate 或直接创建 Work Nodes

**输出物（必须）**
- `action_items.yaml` schema（文档形式即可）
- `PatchSpec v1`：task_patch 的补丁协议（见 `docs/06_meeting_mode.md` 的 `8.3.1`）
- 一组 action_items 示例（至少覆盖：update_constraints / add_task / terminate_team / major_restart 提议）

**验收标准（必须）**
- 任何对 case/spec 的修改都必须可追溯到 meeting_id + decision 段落
- 不能“静默改写”已有约束；必须记录 delta 与理由

---

## 5) Stage 4：检索与引用（可选但强烈建议）

**目标**
- 让会议结论可被后续会议/任务复用；避免“结论漂移”。

**工作项（建议）**
- sources 编号规范（S1/S2...）与 citations.yaml 结构
- meeting_brief/decision 的可检索字段（固定行/front-matter）
- transcript/whiteboard 的 compaction 策略与快照索引

**输出物（建议）**
- `artifacts/citations.yaml` 示例与字段说明
- `shared/compaction/snap_0001.md` 示例（包含覆盖的 spk id 范围）

**验收标准（建议）**
- 给定某个结论，能定位到对应 proposal/speak/source 的锚点与文件路径

---

## 6) Stage 5：质量门与审计（Verifier）

**目标**
- 让“协议是否被遵守”可自动检查，减少人工审查负担。

**工作项（建议）**
- 锚点唯一性检查（prop/spk/wb 不重复）
- whiteboard 结论必须回链（from_props）
- ACL/append-only/single-writer 违规检测
- action_items 的可执行性校验（字段缺失/非法路径/越权动作）

**输出物（必须）**
- 一份 Verifier 检查清单（可以是文档/伪代码/规则表）
- issue_list 的格式与严重级别定义（blocker/warn/info）

**验收标准（必须）**
- 违规即产出 issue_list（blocker），并阻止进入归档/注入阶段

---

## 7) Stage 6：安全与保留策略（上线前）

**目标**
- 控制敏感信息与仓库膨胀风险，确保可长期运行。

**工作项（建议）**
- 对 transcript 的脱敏/标注策略（哪些字段禁止落盘）
- 归档策略（哪些进入 `fs/archive/**`，哪些只保留 brief）
- `.gitignore` 策略：运行态数据默认不应污染仓库（如需要持久化，必须明确规则）

**验收标准（必须）**
- 满足最小合规：敏感信息不可被默认写入可检索层
- 满足可运维：会议目录增长可控（有归档与压缩策略）
