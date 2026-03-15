# A2A E2E Smoke（Wave 3）

- 执行日期：2026-03-03
- 执行目录：`D:\project\ai-workflow\.worktrees\feat-a2a-global-rollout`
- 目标：验证 Wave 3 前端接入后，后端/前端/集成回归与 A2A 协议 smoke 均可通过。

## 回归结果

### 1) Backend 全量

命令：

```powershell
pwsh -NoProfile -File .\scripts\test\backend-all.ps1
```

结果：PASS（退出码 `0`）

摘要：
- `go test ./...` 通过。
- 所有有测试的包均为 `ok`。

### 2) Frontend 单测

命令：

```powershell
pwsh -NoProfile -File .\scripts\test\frontend-unit.ps1
```

结果：PASS（退出码 `0`）

摘要：
- Test Files: `15 passed`
- Tests: `86 passed`

### 3) Frontend 构建

命令：

```powershell
pwsh -NoProfile -File .\scripts\test\frontend-build.ps1
```

结果：PASS（退出码 `0`）

摘要：
- `tsc -b && vite build` 成功。
- 产物已生成：`dist/`。

### 4) P3 集成回归

命令：

```powershell
pwsh -NoProfile -File .\scripts\test\p3-integration.ps1
```

结果：PASS（退出码 `0`）

摘要：
- Backend 全量测试通过。
- P3.5 terminology gate 通过。
- GitHub 集成相关测试通过。
- Frontend 单测与构建均通过。

## A2A Smoke（Token 场景）

> 注：`18080` 端口在本机已被占用，改用 `19080`。

### 服务启动配置

```powershell
$env:AI_WORKFLOW_A2A_ENABLED='true'
$env:AI_WORKFLOW_A2A_TOKEN='wave3-a2a-token'
$env:AI_WORKFLOW_A2A_VERSION='0.3'
```

### 服务启动命令

```powershell
go run ./cmd/ai-flow server --port 19080
```

### Smoke 命令

```powershell
go run ./cmd/a2a-smoke `
  -card-base-url http://127.0.0.1:19080 `
  -a2a-version 0.3 `
  -token wave3-a2a-token `
  -project-id ai-workflow `
  -max-poll 1 `
  -allow-nonterminal `
  -timeout 180s
```

### 输出摘要

```text
rpc_url=http://127.0.0.1:19080/api/v1/a2a
card_protocol_version=0.3
send_result={"contextId":"","id":"plan-20260303-ed22f8ab","kind":"task","metadata":{"project_id":"ai-workflow"},"status":{"state":"input-required","timestamp":"2026-03-03T05:20:31Z"}}
task_result[1]={"contextId":"","id":"plan-20260303-ed22f8ab","kind":"task","metadata":{"project_id":"ai-workflow"},"status":{"state":"input-required","timestamp":"2026-03-03T05:20:31Z"}}
task_state=input-required
task_non_terminal=true
```

### 运行日志文件

- Server stdout: `.run/wave3-a2a-server-19080.out.log`
- Server stderr: `.run/wave3-a2a-server-19080.err.log`
- Smoke output: `.run/wave3-a2a-smoke-19080.out.log`

## 结论

- Wave 3 所需回归命令全部通过。
- A2A token 场景 smoke 通过，`message/send` 与 `tasks/get` 输出可解析且状态一致。
- 前端 A2A 接入采用独立入口（`A2AChatView`），默认 `VITE_A2A_ENABLED=false` 时 legacy `ChatView` 行为保持不变。
