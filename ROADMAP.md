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

## v0.4（任务执行闭环：TaskSpec/verify/pack 与落盘契约对齐）

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

目标：在不破坏 **AG-UI 对接协议** 与 **落盘契约（FILE_STRUCTURE）** 的前提下，引入更多系统/agent，提升可观测与运维能力。

- [ ] 事件元信息增强（eventId/producer/actor/subject/correlation）从“可出现”升级为“必须”
- [ ] cursor/断点续传与回放工具（thread/run）
- [ ] 更强的安全边界与脱敏/保留策略（archive/retention）
