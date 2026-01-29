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
- `archive/igi/README.md`：IGI 草案（已归档；当前主线 v1 不要求实现）

## 模板
- `99_templates.md`：最少模板（TaskSpec / Summary / Cards / issues / MeetingSpec 等）
