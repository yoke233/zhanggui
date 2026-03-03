# LiteBox 团队交接手册（测试同学版）

## 1. 目标

用最少步骤验证三件事：

1. Windows runner 能否稳定拉起基础 ELF。
2. `litebox-acp` 是否能保证 ACP 的 `stdout` 行协议不被污染。
3. 出现失败时，能否快速判断是“参数问题、协议问题还是程序兼容性问题”。

## 2. 前置条件

1. 已有 LiteBox 仓库：`D:\tmp\litebox-research`
2. 已构建 runner：`target\release\litebox_runner_linux_on_windows_userland.exe`
3. 当前项目路径：`D:\project\ai-workflow`

## 3. 最短验证链路

### 3.1 验证 runner（推荐先跑）

```powershell
pwsh -NoProfile -File D:\litebox-playbook\scripts\run-static-hello.windows.ps1 `
  -RepoRoot D:\tmp\litebox-research `
  -CaptureOutput
```

预期：

1. 参数里包含 `--unstable --rewrite-syscalls`
2. `STDOUT` 包含 `hello world.`
3. 退出码 `0`

### 3.2 验证 ACP 桥接命令

```powershell
Set-Location -LiteralPath D:\project\ai-workflow
go test ./cmd/litebox-acp -count=1
go build -o D:\tmp\litebox-acp.exe .\cmd\litebox-acp
```

预期：

1. `go test` 通过
2. 可生成 `D:\tmp\litebox-acp.exe`

## 4. 与 acp-smoke 联调（不改 acp-smoke）

```powershell
Set-Location -LiteralPath D:\project\ai-workflow

# 先准备一个 Linux 目标程序（示例）
$env:CGO_ENABLED='0'
$env:GOOS='linux'
$env:GOARCH='amd64'
go build -o D:\tmp\acp-fake-linux .\internal\acpclient\testdata\fake_agent.go
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue

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

说明：

1. 该链路现在主要用于验证协议层/桥接层。
2. 如果目标程序触发 LiteBox 未实现行为（例如 `NoHugePage`），会在 runner 内部失败，这属于程序兼容性边界，不是 ACP 协议栈错误。

## 5. 故障分流（速查）

1. 退出码 `3221225477`（`0xC0000005`）：
   - 优先确认是否已带 `--unstable --rewrite-syscalls`
2. `invalid json-rpc message ... System information.`：
   - 说明 runner banner 进了协议通道，应走 `litebox-acp` 过滤层
3. `Unsupported madvise behavior NoHugePage`：
   - 目标二进制触发了当前 LiteBox 未实现行为，需更换目标程序或做 LiteBox 侧兼容补丁
4. `initialize EOF` 且 runner 已异常退出：
   - 先排 runner 自身错误，再看 ACP

## 6. 交付物位置

1. 桥接命令：
   - `cmd/litebox-acp/main.go`
2. 桥接测试：
   - `cmd/litebox-acp/main_test.go`
3. 研究索引与复盘：
   - `docs/learning/litebox/README.zh-CN.md`
   - `docs/learning/litebox/research-notes.zh-CN.md`

