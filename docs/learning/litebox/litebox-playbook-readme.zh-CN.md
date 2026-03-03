# LiteBox 学习与落地手册（面向 Go 多平台项目）

> 目标：把 LiteBox 的调用方式、权限模型、目录注入方式、Node 启动方式、
> 以及 Go 接收标准输出的做法整理为一份可执行的工程资料。

## 1. 你最关心的结论（先看）

1. LiteBox 当前主打场景是：
   - 在 Linux 上沙箱运行 Linux 程序。
   - 在 Windows x86_64 上运行 Linux 程序。
2. macOS（你说的 `ma`）目前没有官方 runner/platform 支持。
3. 目录挂载（`--mount` / bind mount）当前没有 CLI 能力，主路径是：
   - 先把文件树打成 `.tar`。
   - 再用 `--initial-files` 注入初始 rootfs。
4. `--insert-file` 参数虽然存在，但当前实现里是 `unimplemented!`，不能依赖。
5. Go 集成建议：通过 `os/exec` 调用 runner，可同时支持：
   - 实时透传 stdout/stderr。
   - 缓冲捕获 stdout/stderr（用于日志与结果解析）。

## 2. 工程目录

本手册配套代码位于：`D:\litebox-playbook`

- `scripts/build-runner.windows.ps1`
  - Windows 构建 `litebox_runner_linux_on_windows_userland`。
- `scripts/run-static-hello.windows.ps1`
  - 在 Windows 上运行静态 Linux ELF 示例。
- `scripts/new-rootfs-tar.windows.ps1`
  - 把目录打包为 `--initial-files` 需要的 tar。
- `go/liteboxcli/runner.go`
  - Go 调用封装（参数校验、构造参数、执行、采集输出）。
- `go/cmd/run-example/main.go`
  - Go 端演示入口。
- `docs/runner-parameters.zh-CN.md`
  - 参数全量清单与说明。

## 3. 平台支持边界

1. Windows 侧 runner：`litebox_runner_linux_on_windows_userland`
   - 代码中显式限制：仅 `target_os = "windows"` 且 `target_arch = "x86_64"`。
2. Linux 侧 runner：`litebox_runner_linux_userland`
   - 用于在 Linux host 上运行 Linux 程序。
3. macOS
   - 当前仓库没有 `target_os = "macos"` 的 runner/platform 路径。

## 4. 调用方式（高频）

### 4.1 Windows 最小调用（静态 ELF）

```powershell
# 假设已构建 runner
D:\tmp\litebox-research\target\release\litebox_runner_linux_on_windows_userland.exe \
  D:\tmp\litebox-research\litebox_runner_linux_on_windows_userland\tests\test-bins\hello_world_static
```

说明：
- 静态 ELF 一般不依赖共享库，可不传 `--initial-files`。
- 若程序依赖动态库，通常需要先准备 rootfs tar，再传 `--initial-files`。

### 4.2 Linux 最小调用

```bash
./target/release/litebox_runner_linux_userland /path/to/linux/program
```

### 4.3 带 rootfs 的调用（通用思路）

```text
runner --unstable --initial-files <rootfs.tar> <program> [args...]
```

## 5. 权限模型（你问的“怎么授予权限”）

1. Windows runner
   - 基于 Windows 文件属性推导 LiteBox 模式位。
   - 只读文件映射为 `r-x`，可写文件/目录映射为 `rwx`。
   - 统一使用 UID `1000`（Windows 无 Unix UID 等价）。
2. Linux runner
   - 读取目标程序和祖先目录元数据，把 mode/uid带入初始 FS。
   - 会确保 `/tmp` 具备可写执行权限。

结论：
- 当前没有独立“权限策略配置文件”接口。
- 权限主要来自宿主文件属性 + runner 内置逻辑。

## 6. 目录挂载（你问的“怎么挂载目录”）

当前状态：
1. 没有 `--mount` / `--bind` / `--volume` 这类参数。
2. 推荐路径：目录先打包成 tar，再用 `--initial-files` 注入。
3. `--insert-file` 目前未实现，不可作为生产路径。

## 7. 启动 Node 程序（你问的“怎么启动 node”）

已验证的官方思路：
1. 准备 Linux 版 `node` 可执行文件。
2. 把脚本（比如 `hello_world.js`）放进 rootfs（如 `/out/hello_world.js`）。
3. 运行：`runner ... node /out/hello_world.js`。

如果使用重写链路，常见做法是：
- 先用 `litebox_syscall_rewriter` 处理目标二进制/依赖。
- 再由 runner 执行重写后的程序。

## 8. Go 怎么接收 STDOUT（你问的“STD out”）

两种模式：
1. 透传模式
   - `cmd.Stdout = os.Stdout`
   - `cmd.Stderr = os.Stderr`
   - 适合在线观察运行日志。
2. 捕获模式
   - 绑定 `bytes.Buffer` 到 `Stdout/Stderr`。
   - 执行后读取 `buffer.String()`。
   - 适合结构化记录、后处理、上报。

配套实现见：`go/liteboxcli/runner.go`。

## 9. 推荐落地路径（Go 项目）

1. 把 LiteBox 视作“外部执行引擎”，不要先做 FFI。
2. 先用本仓库提供的 Go 封装稳定 CLI 参数和错误语义。
3. 按操作系统拆分 runner 路径：
   - Windows: `litebox_runner_linux_on_windows_userland.exe`
   - Linux: `litebox_runner_linux_userland`
4. 统一产出执行结果对象：`exit_code/stdout/stderr/args/duration`。

## 10. 参考来源（官方链接）

1. 根 README（项目定位与支持方向）
   - https://github.com/microsoft/litebox/blob/main/README.md
2. Windows runner 平台限制与参数
   - https://github.com/microsoft/litebox/blob/main/litebox_runner_linux_on_windows_userland/src/main.rs
   - https://github.com/microsoft/litebox/blob/main/litebox_runner_linux_on_windows_userland/src/lib.rs
3. Linux runner 参数与 `--initial-files`
   - https://github.com/microsoft/litebox/blob/main/litebox_runner_linux_userland/src/lib.rs
4. Node 与 stdout 示例（测试代码）
   - https://github.com/microsoft/litebox/blob/main/litebox_runner_linux_userland/tests/run.rs
5. Node bench 示例命令
   - https://github.com/microsoft/litebox/blob/main/dev_bench/src/main.rs

---

如果你后续决定补一层服务化封装（比如 gRPC/HTTP 调度 runner），
建议先保留 `CLI + Go exec` 这条路径作为 fallback，降低跨平台耦合风险。

## 11. Windows 运行稳定性补充（2026-03-03 实测）

1. 在 Windows runner 上直接跑部分 ELF（包括 hello_world_static）可能出现退出码 `0xC0000005`（`3221225477`）。
2. 对应可行规避：默认追加 `--unstable --rewrite-syscalls`。
3. 本目录脚本 `scripts/run-static-hello.windows.ps1` 已默认启用以上两个参数；如需回归对比，可传：

```powershell
-EnableUnstable:$false -EnableRewriteSyscalls:$false
```

4. 若启用 `--rewrite-syscalls` 后出现 panic（例如 `Unsupported madvise behavior NoHugePage`），说明是目标程序与当前 LiteBox 能力边界冲突，需要：
   - 更换更“朴素”的目标二进制，或
   - 在 LiteBox 源码侧补行为兼容（而不是 ACP 协议层）。