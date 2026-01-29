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
