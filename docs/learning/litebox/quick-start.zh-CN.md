# Quick Start（交给测试同学直接跑）

> 说明：本目录在当前会话里只做了代码与脚本编写，未执行真实编译。
> 你可以把以下步骤发给测试同学直接验证。

## Windows 路径

1. 构建 runner

```powershell
pwsh -NoProfile -File D:\litebox-playbook\scripts\build-runner.windows.ps1 `
  -RepoRoot D:\tmp\litebox-research `
  -Configuration Release
```

2. 运行静态示例并捕获输出（脚本默认追加 --unstable --rewrite-syscalls）

```powershell
pwsh -NoProfile -File D:\litebox-playbook\scripts\run-static-hello.windows.ps1 `
  -RepoRoot D:\tmp\litebox-research `
  -CaptureOutput
```

3. 一键串行（可选）

```powershell
pwsh -NoProfile -File D:\litebox-playbook\scripts\run-example-end-to-end.windows.ps1 `
  -RepoRoot D:\tmp\litebox-research `
  -Build `
  -CaptureOutput
```

## Linux 路径

1. 构建 runner

```bash
bash D:/litebox-playbook/scripts/build-runner.linux.sh /path/to/litebox release
```

2. 运行程序

```bash
bash D:/litebox-playbook/scripts/run-static-hello.linux.sh \
  /path/to/litebox/target/release/litebox_runner_linux_userland \
  /path/to/your/linux/program
```

## Go 演示入口

```bash
cd D:/litebox-playbook/go
# 可选：go mod tidy
go run ./cmd/run-example \
  -runner D:/tmp/litebox-research/target/release/litebox_runner_linux_on_windows_userland.exe \
  -program D:/tmp/litebox-research/litebox_runner_linux_on_windows_userland/tests/test-bins/hello_world_static \
  -capture-output=true
```

可重复参数示例：

```bash
go run ./cmd/run-example \
  -runner ... \
  -program ... \
  -env HOME=/ \
  -env LD_LIBRARY_PATH=/lib64:/lib32:/lib \
  -arg --version
```
