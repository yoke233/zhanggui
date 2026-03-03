# LiteBox + ACP 联调复盘与心得（2026-03-03）

## 1. 目标与范围

本次研究目标：

1. 验证 `Windows runner` 在本机是否可稳定运行。
2. 评估 `agent-acp` 是否可通过 LiteBox 承载（不改 `cmd/acp-smoke`）。
3. 在本仓库沉淀可复现命令、故障结论、工程化建议。

不在本次范围：

1. 修改 LiteBox 上游源码并提交 PR。
2. 完整实现 Go 运行时在 LiteBox Windows 平台的兼容修复。

## 2. 环境与版本

1. 日期：`2026-03-03`
2. OS：Windows（pwsh）
3. LiteBox 仓库：`D:\tmp\litebox-research`
4. Runner：`target\release\litebox_runner_linux_on_windows_userland.exe`
5. 当前项目：`D:\project\ai-workflow`

## 3. 关键复现结论

### 3.1 现象 A：不带重写参数时，常见 `0xC0000005`

命令（最小复现）：

```powershell
D:\tmp\litebox-research\target\release\litebox_runner_linux_on_windows_userland.exe `
  D:\tmp\litebox-research\litebox_runner_linux_on_windows_userland\tests\test-bins\hello_world_static
```

现象：

1. 有 banner 输出：`System information ...`
2. 退出码常见：`3221225477`（`0xC0000005`，访问冲突）

### 3.2 现象 B：加 `--unstable --rewrite-syscalls` 后可恢复

命令：

```powershell
D:\tmp\litebox-research\target\release\litebox_runner_linux_on_windows_userland.exe `
  --unstable `
  --rewrite-syscalls `
  D:\tmp\litebox-research\litebox_runner_linux_on_windows_userland\tests\test-bins\hello_world_static
```

结果：

1. 输出包含 `hello world.`
2. 退出码 `0`

### 3.3 现象 C：ACP 协议被 runner banner 污染

`ACP` 要求 `stdout` 按行是 JSON-RPC。  
但 Windows runner 启动会先打印 `System information.` 三行文本，导致 `acp-smoke` 在 `initialize` 前 JSON 解码失败。

## 4. 工程化处理

### 4.1 已完成：新增 LiteBox ACP 桥接器

新增命令：

1. [cmd/litebox-acp/main.go](/D:/project/ai-workflow/cmd/litebox-acp/main.go)
2. [cmd/litebox-acp/main_test.go](/D:/project/ai-workflow/cmd/litebox-acp/main_test.go)

行为：

1. 启动 runner 子进程。
2. 过滤 runner `stdout` 的非 JSON 行（输出到 `stderr` 带前缀）。
3. 仅透传 JSON-RPC 行到 `stdout`，保证 ACP 行协议。

验证：

1. `go test ./cmd/litebox-acp -count=1` 通过。
2. `go build ./cmd/litebox-acp` 通过。

### 4.2 已完成：playbook Windows 脚本默认启用重写

已更新：

1. `D:\litebox-playbook\scripts\run-static-hello.windows.ps1`
2. `D:\litebox-playbook\docs\quick-start.zh-CN.md`
3. `D:\litebox-playbook\docs\runner-parameters.zh-CN.md`
4. `D:\litebox-playbook\README.zh-CN.md`

默认追加：

1. `--unstable`
2. `--rewrite-syscalls`

并保留了可手动关闭开关（用于对比回归）。

## 5. 仍待处理的兼容边界

当把 Go 编译出的 Linux 二进制（例如 fake ACP agent）放进 LiteBox Windows runner 时，虽可绕过 `0xC0000005`，但进一步命中：

1. `Unsupported madvise behavior NoHugePage`
2. 该问题来自 LiteBox 当前行为边界，而非 ACP 协议层。

换句话说：

1. `litebox-acp` 已解决“stdout 协议污染”问题。
2. 目标程序能否跑通，还取决于程序自身是否触发 LiteBox 尚未支持的系统行为。

## 6. 建议落地策略

1. Windows 下 runner 调用默认使用 `--unstable --rewrite-syscalls`。
2. ACP 链路统一经 `cmd/litebox-acp`，避免 runner banner 破坏 JSON-RPC。
3. 对执行目标分级：
   - P0：先用静态、行为简单的二进制验证链路。
   - P1：再逐步引入复杂 runtime（如 Go/Node）并收集触发点。
4. 将“程序兼容性”与“协议兼容性”分开排查，避免混淆。

## 7. 心得体会

1. 协议层失败不一定是协议实现问题，常见是“传输层被污染”（本次就是 runner banner）。
2. 先做“可观测性改造”比盲修快：把 stdout/stderr、退出码、参数原样打印，定位速度明显提升。
3. 在不稳定平台上，优先做“可回退的小桥接层”（`litebox-acp`），比大改核心链路风险更低。
4. 对跨系统执行器（Windows host + Linux guest）要有心理预期：参数、ABI、runtime 行为差异会比普通本地执行大得多。
5. 文档和脚本要同步更新，否则测试同学会在旧命令上反复踩同一个坑。

## 8. 复现命令清单

### 8.1 playbook 脚本（已默认带重写）

```powershell
pwsh -NoProfile -File D:\litebox-playbook\scripts\run-static-hello.windows.ps1 `
  -RepoRoot D:\tmp\litebox-research `
  -CaptureOutput
```

### 8.2 通过桥接器跑 ACP smoke（不改 acp-smoke）

```powershell
go build -o D:\tmp\litebox-acp.exe .\cmd\litebox-acp

go run .\cmd\acp-smoke `
  -agent-cmd D:\tmp\litebox-acp.exe `
  -agent-arg -runner `
  -agent-arg D:\tmp\litebox-research\target\release\litebox_runner_linux_on_windows_userland.exe `
  -agent-arg -runner-arg `
  -agent-arg --unstable `
  -agent-arg -runner-arg `
  -agent-arg --rewrite-syscalls `
  -agent-arg -program `
  -agent-arg D:\tmp\acp-fake-linux `
  -cwd D:\project\ai-workflow `
  -prompt "请回复：ACP_GO_OK" `
  -timeout 90s
```

