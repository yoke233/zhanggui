# Requirement Flow Runbook（2026-03-21）

## 目的

这份 runbook 用于给本轮 Requirement -> Thread -> Proposal -> Initiative -> WorkItem Execution 留痕，覆盖：

- 本轮功能与提交落点
- 已实际执行的验证命令与结果
- 如何重跑“虚拟全流程”
- 如何重跑“thread 输出 workitem 关系组并继续执行”的完整链路
- 后续如果切换到真实 ACP，应如何启动、验证与留档

## 本轮变更范围

本轮主线提交如下：

| Commit | 说明 |
| --- | --- |
| `7a4c779` | `fix(proposal): validate proposal provenance` |
| `34db688` | `feat(requirement): add requirement analysis flow` |
| `d5532fe` | `fix(requirement): hide internal thread metadata` |
| `07b1e32` | `test(requirement): add virtual end-to-end requirement flow` |

核心能力落点：

- 后端新增 requirement 分析与建 thread API
  - `POST /requirements/analyze`
  - `POST /requirements/create-thread`
- 前端新增 Requirement 页面与入口
  - `/requirements/new`
- proposal/initiative 主链已打通
- 本轮补了一条不依赖真实 ACP 的虚拟全流程测试，覆盖：
  - Requirement Analyze
  - Requirement Create Thread
  - Thread meeting 启动与消息派发
  - Proposal Create / Submit / Approve
  - Initiative materialize
- 本轮继续补了一条更完整的集成测试，覆盖：
  - Proposal Reject / Revise / Resubmit
  - thread 产出 3 个带 `depends_on` 的 work item 关系组
  - Initiative Propose / Approve
  - root work item 自动提交、dependent work item 自动解锁
  - work item 内部 gate reject -> rework -> approve

## 代码落点

- 后端应用层
  - `internal/application/requirementapp/`
  - `internal/application/threadapp/service.go`
- 后端 HTTP
  - `internal/adapters/http/requirement.go`
  - `internal/adapters/http/requirement_test.go`
  - `internal/adapters/http/requirement_flow_test.go`
  - `internal/adapters/http/handler.go`
- 前端
  - `web/src/pages/RequirementPage.tsx`
  - `web/src/pages/RequirementPage.test.tsx`
  - `web/src/pages/DashboardPage.tsx`
  - `web/src/pages/ThreadsPage.tsx`
  - `web/src/pages/CreateProjectPage.tsx`

## 本轮已执行验证

### 2026-03-21 当前会话实际重跑

```powershell
go test ./internal/adapters/http -run TestAPI_RequirementToProposalToInitiativeFlow -count=1 -v
go test ./internal/adapters/http -run TestIntegration_RequirementToWorkItemExecutionFlow -count=1 -v
go test ./internal/adapters/http -count=1
```

结果：

- `TestAPI_RequirementToProposalToInitiativeFlow`：`PASS`
- `TestIntegration_RequirementToWorkItemExecutionFlow`：`PASS`
- `./internal/adapters/http`：`PASS`

说明：

- 第一条命令直接验证 Requirement -> Proposal -> Initiative 的稳定基线。
- 第二条命令验证 thread 收敛到 work item 关系组后，继续走 initiative 审批与 work item DAG 执行闭环。
- 第三条命令验证新增测试没有破坏现有 thread / websocket / proposal / requirement HTTP 行为。

### 同一任务链此前已通过的验证

以下命令由同一轮 harness 任务链在前序实现阶段已执行通过：

```powershell
go test ./internal/adapters/http ./internal/runtime/agent ./internal/application/... ./internal/adapters/store/sqlite -count=1
npm --prefix web test -- --run src/pages/RequirementPage.test.tsx src/pages/CreateProjectPage.test.tsx src/pages/ThreadsPage.test.tsx src/pages/DashboardPage.test.tsx
npm --prefix web run build
```

说明：

- 上述结果对应 Plan 3 实现和 review 修复后的验证留痕。
- 当前会话没有再次重跑前端命令，因为本轮新增提交仅涉及后端测试与文档。

## 虚拟全流程如何重跑

### 最小闭环

```powershell
go test ./internal/adapters/http -run TestAPI_RequirementToProposalToInitiativeFlow -count=1 -v
```

这条测试会在内存/临时环境中完成：

1. 创建 backend/frontend 两个 project 与 resource space
2. 用 stub LLM 产出 requirement analysis
3. 调 `/requirements/create-thread` 建出 group_chat thread
4. 通过 stub thread runtime 模拟 3 个 agent 接力发言并收敛
5. 创建 proposal
6. submit proposal
7. approve proposal
8. 校验 initiative 与 2 个 initiative items 已物化

### 完整 work item 关系组闭环

```powershell
go test ./internal/adapters/http -run TestIntegration_RequirementToWorkItemExecutionFlow -count=1 -v
```

这条测试会在临时 SQLite + 内存 bus + scheduler 环境中完成：

1. 创建 backend/frontend/qa 三个 project 与 resource space
2. 用 stub LLM 产出 requirement analysis，并创建 `group_chat` thread
3. 通过 stub thread runtime 模拟 3 个 agent 发言收敛
4. 创建 proposal
5. submit proposal
6. reject proposal
7. revise proposal，并把草案改成 3 个 work item 的依赖链
8. resubmit + approve proposal，物化 initiative 与 work item 关系组
9. 为 3 个 work item 补内部 steps
10. propose + approve initiative
11. 自动执行 root work item，随后自动解锁 dependent work item
12. 在前端 work item 内模拟 gate reject -> rework -> approve
13. 校验 3 个 work item 全部 done，initiative 最终收敛为 done

### 扩展回归

```powershell
go test ./internal/adapters/http -count=1
```

适用场景：

- 改动了 requirement API
- 改动了 thread message routing / meeting mode
- 改动了 proposal / initiative HTTP 主链

## 这条虚拟全流程当前验证了什么

当前 `TestAPI_RequirementToProposalToInitiativeFlow` 已覆盖以下证据点：

- requirement 分析结果中：
  - `matched_projects`
  - `suggested_agents`
  - `suggested_meeting_mode`
- requirement 建 thread 时：
  - thread metadata 正确写入 `meeting_mode=group_chat`
  - `meeting_max_rounds=6`
  - `skip_default_context_refs` 不泄漏到最终 thread metadata
  - context refs 仅落选中的两个项目
- thread runtime 启动时：
  - 3 个 agent 均被 Invite
  - 会议模式实际走 `PromptAgent`，不是直接 `SendMessage`
  - group_chat 产生 agent 发言与 system summary
- proposal 主链：
  - create
  - submit
  - approve
- initiative 物化：
  - 状态为 `draft`
  - 生成 2 个 items

当前 `TestIntegration_RequirementToWorkItemExecutionFlow` 额外覆盖以下证据点：

- proposal 审核返工：
  - reject
  - revise
  - replace drafts
  - resubmit
- thread 讨论输出模型：
  - 最终不是 `taskgroup`
  - 而是 3 个 `work_item_drafts`
  - `depends_on` 形成 `backend -> frontend -> qa` 关系链
- initiative 执行链：
  - `propose`
  - `approve`
  - root work item 自动进入调度
  - dependent work item 先处于 `accepted`，依赖完成后自动排队执行
- work item 内部执行链：
  - 前端 work item 发生 gate reject
  - 触发 rework 后再次执行
  - gate 最终 approve
- thread 留痕：
  - `meeting_summary`
  - `proposal_rejected`
  - `proposal_revised`
  - `proposal_merged`

## 如果后面要切到真实 ACP，怎么跑

本项目已经有三类真实 ACP 入口，建议按下面顺序使用。

### 1. 先验证 ACP 基础连通与 JSON trace

用途：

- 确认 ACP agent 能启动
- 确认 initialize / session/new / session/prompt 基本协议通了
- 给后续排查保留原始 trace

命令：

```powershell
go test -tags probe ./cmd/acp-probe -run TestCaptureRealACPJSONTranscripts -v -timeout 600s
```

参考：

- `docs/reports/acp-real-traces.md`
- `cmd/acp-probe/real_trace_capture_test.go`

产物：

- `docs/reports/artifacts/acp-real-traces/*.json`

### 2. 验证 ThreadSessionPool 真实 ACP 生命周期

用途：

- 验证 InviteAgent / WaitAgentReady / SendMessage / RemoveAgent 全链路
- 验证真实 ACP 是否能进行多轮对话与文件 I/O

前置：

- 已安装 Node.js / `npx`
- `OPENAI_API_KEY` 已设置
- 设置环境变量 `AI_WORKFLOW_REAL_THREAD_ACP=1`

命令：

```powershell
$env:AI_WORKFLOW_REAL_THREAD_ACP="1"
go test -tags real -run TestReal_ThreadPoolFullLifecycle -v -timeout 300s ./internal/runtime/agent/...
go test -tags real -run TestReal_ThreadPoolFileIO -v -timeout 300s ./internal/runtime/agent/...
```

参考：

- `internal/runtime/agent/thread_session_pool_real_test.go`
- `docs/learning/thread-acp-real-test-notes.md`

真实 ACP 启动命令证据：

- Codex ACP
  - `npx -y @zed-industries/codex-acp`
- Claude ACP
  - `npx -y @zed-industries/claude-agent-acp`

说明：

- `thread_session_pool_real_test.go` 当前默认用的是 codex ACP 真实进程。
- 冷启动可能要 30-60 秒，热启动通常 2-3 秒。

### 3. 验证 HTTP 层 Thread Task + 真实 ACP

用途：

- 从 HTTP 层触发真实 agent 任务，验证服务端 handler 到 ACP runtime 的整链联调

前置：

- `.ai-workflow/config.toml` 中有可用 agent profile
- 设置环境变量 `AI_WORKFLOW_REAL_THREAD_TASK=1`

命令：

```powershell
$env:AI_WORKFLOW_REAL_THREAD_TASK="1"
go test -tags real -run TestReal_ThreadTask_WithACP -timeout 120s ./internal/adapters/http/...
```

参考：

- `internal/adapters/http/thread_task_real_test.go`

## 如果要手动跑服务，再接真实 ACP

### 启动后端

```powershell
go run ./cmd/ai-flow server --port 8080
```

### 启动前端

```powershell
npm --prefix web install
npm --prefix web run dev -- --strictPort
```

### 手动 ACP 侧准备

如果走 Codex ACP，确保：

- `OPENAI_API_KEY` 已设置
- `npx` 可用
- agent profile 的 driver 指向 ACP 启动命令

常见启动形式：

```powershell
npx -y @zed-industries/codex-acp
npx -y @zed-industries/claude-agent-acp
```

### 推荐手动验证顺序

1. 先跑 `acp-probe` 采样 trace，确认 ACP 本身可用
2. 再跑 `TestReal_ThreadPoolFullLifecycle`
3. 再跑 `TestReal_ThreadTask_WithACP`
4. 最后再从 Web 或 API 手动点 requirement -> create thread -> proposal -> initiative 主链

## 建议的真实 requirement 全流程演练方式

如果后续要把当前“虚拟 requirement 全流程”升级为“真实 ACP 全流程”，建议按下面的顺序扩展，而不是一步到位：

1. 保留当前 `TestAPI_RequirementToProposalToInitiativeFlow` 作为稳定基线
2. 单独新增 real build tag 的 requirement flow test
3. 只把 thread runtime 替换成真实 ACP，LLM analyze 仍先用 stub
4. 待 thread runtime 稳定后，再考虑把 requirement analyze 的 LLM 也切成真实依赖

这样做的好处：

- 可以先把“线程调度 / ACP 启动 / 多 agent 接力”稳定下来
- 出问题时更容易判断是 ACP runtime、HTTP handler、还是 analyze LLM 在抖动

## 留痕位置

- 任务状态：
  - `harness-tasks.json`
  - `harness-progress.txt`
- 业务提交：
  - `7a4c779`
  - `34db688`
  - `d5532fe`
  - `07b1e32`
- ACP 真实 trace：
  - `docs/reports/acp-real-traces.md`
  - `docs/reports/artifacts/acp-real-traces/*.json`
- 本轮 runbook：
  - `docs/runbooks/2026-03-21-requirement-flow-runbook.zh-CN.md`

## 当前结论

截至 2026-03-21，本轮 requirement 三阶段能力已经满足：

- 虚拟全流程可稳定重跑
- thread meeting 的 `group_chat` 路径已被真实测试证据覆盖
- proposal -> initiative 物化闭环已通过
- 后续切真实 ACP 的运行入口、环境变量、命令和留痕路径已明确

## 2026-03-21 真实 ACP 全流程复跑结果

### 最终成功留痕

最终成功的一轮真实联调使用以下隔离环境：

- Root: `C:\Users\yoke\AppData\Local\Temp\ai-flow-real-20260321-102745`
- Summary: `C:\Users\yoke\AppData\Local\Temp\ai-flow-real-20260321-102745\artifacts\SUMMARY.md`
- Artifacts 目录: `C:\Users\yoke\AppData\Local\Temp\ai-flow-real-20260321-102745\artifacts`
- Port: `18084`

最终状态：

- `thread 1`：真实 `group_chat` 已产出 agent 发言与 `meeting_summary`
- `proposal 1`：完整经过 `create -> submit -> reject -> revise -> update drafts -> submit -> approve`
- `initiative 1`：最终状态 `done`
- `work_item 1` backend：最终状态 `done`
- `work_item 2` frontend：最终状态 `done`

真实 thread 会议证据：

- `06-thread-messages.json` 中已有 3 条 agent 发言 + 1 条 `meeting_summary`
- `meeting_summary.stop_reason = backend-codex declared final`
- 真实发言顺序为：
  - 第 2 轮 `backend-codex`
  - 第 3 轮 `frontend-codex`
  - 第 4 轮 `backend-codex`

真实执行证据：

- `23-backend-runs.json`：backend step `status = succeeded`
- `24-frontend-runs.json`：frontend step `status = succeeded`
- backend 目录新增：`otp-plan.md`
- frontend 目录新增：`otp-ui-plan.md`

### 本轮真实环境修正点

第一次真实联调虽然把 `proposal -> initiative -> workitem execution` 跑通了，但 thread 会议阶段仍有两个真实问题：

1. Windows 上默认 `npx -y @zed-industries/codex-acp` 会拉到 `0.10.0`，但缺少 `@zed-industries/codex-acp-win32-x64` 可选平台包，thread / workitem 都无法启动真实 agent。
2. `group_chat` 会议路径里，agent session 刚 boot 就立即 `PromptAgent`，遇到慢启动/慢首轮时会触发恢复重入或首个 speaker 失败，导致 `meeting_summary` 只有 “no valid speech”。

本轮已验证有效的运行方式：

- driver 必须显式固定为：
  - `npx -y @zed-industries/codex-acp@0.9.5`
- 代码侧已补两类收敛：
  - thread boot prompt 超时拉长到 `120s`
  - `group_chat` 在真正 prompt 前等待 agent ready，并在单个 speaker 失败时跳过失败 speaker 继续轮转

对应代码落点：

- `internal/runtime/agent/thread_session_pool.go`
- `internal/adapters/http/thread_meeting.go`
- `internal/adapters/http/thread_ws_test.go`

### 如何按这次成功方式重跑真实流程

1. 启动 server，确保 `AI_WORKFLOW_DATA_DIR` 指向隔离目录。
2. 启动后先通过 API 把 `codex-acp` driver 改成：

```json
{
  "id": "codex-acp",
  "launch_command": "npx",
  "launch_args": ["-y", "@zed-industries/codex-acp@0.9.5"],
  "capabilities_max": {
    "fs_read": true,
    "fs_write": true,
    "terminal": true
  }
}
```

3. 再创建 profiles / projects / spaces，并调用：
   - `POST /api/requirements/analyze`
   - `POST /api/requirements/create-thread`
   - proposal / initiative / work-item 相关 API
4. 重点检查：
   - `artifacts/06-thread-messages.json`
   - `artifacts/23-backend-runs.json`
   - `artifacts/24-frontend-runs.json`
   - `artifacts/25-initiative-detail-final.json`

### 后续补充修复

后续又补了一轮 HTTP 集成回归稳定性修复，已收敛此前偶发的：

- `TestIntegration_RequirementToWorkItemExecutionFlow`
- `database is locked (5) (SQLITE_BUSY)`

本次收敛点：

1. `internal/application/proposalapp/service.go`
   - 移除了 `Approve` 事务里的冗余 `GetThread`，避免 SQLite 在并发写场景下出现读后写升级冲突。
2. `internal/application/flow/persister.go`
   - `Stop()` 现在会等待后台 goroutine 退出，避免测试清理阶段留下悬挂写库。
3. `internal/adapters/http/integration_test.go`
   - `setupIntegration` 清理时会显式等待 scheduler 退出，再停止 persister。

补充验证结果：

- `go test ./internal/adapters/http -run TestIntegration_RequirementToWorkItemExecutionFlow -count=10 -v`
- `go test ./internal/adapters/http -run 'TestAPI_WebSocket_ThreadSend_GroupChatMeeting|TestAPI_RequirementToProposalToInitiativeFlow|TestIntegration_RequirementToWorkItemExecutionFlow' -count=5`
- `go test ./internal/adapters/http -count=1`

以上均已通过。
