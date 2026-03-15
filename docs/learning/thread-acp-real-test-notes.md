# Thread Task Real ACP 测试心得

> codex-acp@0.9.5 全流程集成测试经验、踩坑记录、测试架构设计
> 日期: 2026-03-15

## 测试目标

验证 ThreadSessionPool 通过统一 `acpclient.Bootstrap` 启动真实 codex-acp agent 的全流程：
InviteAgent → boot prompt → SendMessage → 多轮对话 → RemoveAgent（含 progress summary）

## 测试架构设计

### 两层测试策略

1. **Mock 层** (`thread_session_pool_lifecycle_test.go`)
   - 用 `io.Pipe()` + `NewWithIO` + `fakeACPServer` goroutine 模拟 ACP agent
   - 通过 `bootstrapFn` 注入点替换 `acpclient.Bootstrap`
   - 9 个测试用例，覆盖全部分支：lifecycle、multi-turn、token tracking、boot failure、resume from paused、cleanup、idempotent invite、context budget warning
   - 每个测试 < 1s，适合 CI

2. **Real 层** (`thread_session_pool_real_test.go`)
   - `//go:build real` + `AI_WORKFLOW_REAL_THREAD_ACP=1` 环境变量守门
   - 真实启动 `npx -y @zed-industries/codex-acp@0.9.5`
   - 验证端到端文件 I/O、多轮对话记忆、graceful leave summary

### bootstrapFn 注入模式

**Why:** `ThreadSessionPool.bootSession` 直接调用 `acpclient.Bootstrap`，无法在 mock 测试中替换。

**How:** 在 struct 上加 `bootstrapFn` 字段（同 `ACPSessionPool.createSessionFn` 模式）：
```go
type ThreadSessionPool struct {
    bootstrapFn func(context.Context, acpclient.BootstrapConfig) (*acpclient.BootstrapResult, error)
}
```
生产代码用 nil（fallback 到 `acpclient.Bootstrap`），测试注入 mock。

### fakeACPServer 设计

用 `io.Pipe()` 创建双向管道，`acpclient.NewWithIO` 创建无进程的 client，goroutine 运行 fake server：

```
[client.PromptText] --writes--> [clientW pipe] --reads--> [serverR → fakeACPServer]
[fakeACPServer]     --writes--> [serverW pipe] --reads--> [clientR → client]
```

fake server 只需处理 `session/prompt` 方法：解析 prompt 文本、记录、返回配置的 reply + usage。

## 踩坑记录

### 1. ContentBlock JSON 序列化格式（重要）

**问题：** `acpproto.ContentBlock{Text: &ContentBlockText{Text: "hello"}}` 序列化为：
```json
{"text":"hello","type":"text"}    ← 实际（flat string）
```
而不是预期的：
```json
{"text":{"text":"hello"}}          ← 错误假设（nested object）
```

**影响：** fake server 用 nested struct 解析，prompt 文本全部为空字符串。测试看似通过（有 prompt 记录），但断言 "boot prompt should contain thread title" 失败。

**修复：** fake server 改为 flat string 解析：
```go
// 错误
Prompt []struct { Text *struct{ Text string } `json:"text"` }

// 正确
Prompt []struct { Text string `json:"text"` }
```

**教训：** 写 fake server 解析逻辑前，先用一个 debug test 打印实际 JSON 格式，不要猜 SDK 的序列化行为。

### 2. codex-acp 不返回 token usage

`codex-acp@0.9.5` 的 `PromptResponse.usage` 中 `inputTokens`/`outputTokens` 均为 0。
这不是 bug — codex 底层用 OpenAI API，但 ACP 协议层面没有暴露 usage。

**处理：** real test 中用 `t.Log("NOTE: ...")` 记录而非 `t.Error` 断言。mock test 中 fake server 返回非零值来测试 tracking 逻辑。

### 3. Windows 上的 stderr 编码

codex-acp 的 stderr 输出带 ANSI 颜色码，在 Windows 控制台显示为乱码（如 `[2m`, `[31m`）。
这是正常的，不影响功能，只影响日志可读性。

### 4. io.Pipe 与 scanner.Bytes()

`bufio.Scanner.Bytes()` 返回的 slice 在下次 `Scan()` 后失效。
在 fake server 中同一次 Scan 迭代内传递给 handlePrompt 是安全的，因为 handlePrompt 在 return 前完成全部解析。

### 5. AgentProfile 不需要手动设 Capabilities

`EffectiveCapabilities()` 从 `ActionsAllowed` 或 role 默认值推导 ACP 能力。
`RoleWorker` 默认包含 `fs_write` + `terminal`，无需显式设置 `CapabilityConfig`。

## 关键数据点

| 指标 | 值 |
|---|---|
| codex-acp 冷启动（npx 首次下载） | ~30-60s |
| codex-acp 热启动（已缓存） | ~2-3s |
| Initialize + NewSession | < 1s |
| 单次 Prompt 响应 | ~5-15s |
| 完整 lifecycle 测试（2 turns + remove） | ~40s |
| 文件 I/O 测试（1 turn + verify） | ~37s |

## Real Test 验证清单

| 步骤 | 验证内容 | 结果 |
|---|---|---|
| Bootstrap | npx 启动 codex-acp，Initialize + NewSession | ✅ |
| Boot prompt | 包含 thread title、context | ✅ |
| SendMessage | agent 执行文件创建任务并回复 "DONE" | ✅ |
| 多轮对话 | follow-up 问题，agent 正确回忆之前创建的文件内容 | ✅ |
| 文件 I/O | 验证文件确实写入磁盘 workspace 目录 | ✅ |
| RemoveAgent | graceful leave 返回 progress summary | ✅ |
| 状态机 | booting → active → paused | ✅ |
| Token 追踪 | turns 计数正确，token 值为 0（codex 特性） | ✅ |

## 运行方式

```bash
# Mock 测试（CI 常规，< 10s）
go test ./internal/runtime/agent/... -run TestThreadPool -timeout 60s

# Real 测试（手动，需 OPENAI_API_KEY）
AI_WORKFLOW_REAL_THREAD_ACP=1 go test -tags real -run TestReal_ThreadPool -v -timeout 300s ./internal/runtime/agent/...

# 只跑文件 I/O 测试
AI_WORKFLOW_REAL_THREAD_ACP=1 go test -tags real -run TestReal_ThreadPoolFileIO -v -timeout 300s ./internal/runtime/agent/...
```

## 文件位置

| 文件 | 说明 |
|---|---|
| `internal/runtime/agent/thread_session_pool_lifecycle_test.go` | Mock 测试（9 cases） |
| `internal/runtime/agent/thread_session_pool_real_test.go` | Real 测试（codex-acp） |
| `internal/runtime/agent/thread_session_pool.go` | bootstrapFn 注入点 |
| `internal/adapters/agent/acpclient/client.go` | PromptText 便捷方法 |
| `internal/adapters/agent/acpclient/bootstrap.go` | 统一 Bootstrap 入口 |
