# LiteBox Runner 参数全量说明（中文）

## 1. Windows Runner

可执行文件：`litebox_runner_linux_on_windows_userland.exe`

### 1.1 位置参数

1. `program_and_arguments...`（必填）
   - 含义：要运行的程序及其参数。
   - 示例：
     - `C:\path\to\hello_world_static`
     - `C:\path\to\node /out/hello_world.js`

### 1.2 可选参数

1. `--env <K=V>`（可重复）
   - 含义：向目标程序注入环境变量。
   - 示例：`--env HOME=/`

2. `--forward-env`
   - 含义：把宿主进程当前环境变量转发给目标程序。
   - 注意：可能引入不可预期变量，生产环境建议白名单。

3. `-Z, --unstable`
   - 含义：开启不稳定选项的总开关。
   - 需要它的参数：`--insert-file` / `--initial-files` / `--rewrite-syscalls`。

4. `--insert-file <PATH>`（可重复，当前未实现）
   - 当前状态：参数存在，但 `run()` 中为 `unimplemented!`。
   - 建议：不要在生产方案依赖此参数。

5. `--initial-files <PATH_TO_TAR>`
   - 含义：把 tar 文件中的内容作为初始只读文件系统层注入。
   - 约束：必须是 `.tar` 后缀。

6. `--rewrite-syscalls`
   - 含义：执行前对 ELF 做 syscall 重写（便于拦截）。
   - 注意：通常与 `-Z --unstable` 配合。

### 1.3 Windows runner 不支持的 Linux runner 参数

1. `--interception-backend`
2. `--tun-device-name`

---

## 2. Linux Runner

可执行文件：`litebox_runner_linux_userland`

### 2.1 位置参数

1. `program_and_arguments...`（必填）
   - 含义：要运行的 Linux 程序和参数。

### 2.2 可选参数

1. `--env <K=V>`（可重复）
2. `--forward-env`
3. `-Z, --unstable`
4. `--insert-file <PATH>`（可重复，当前未实现）
5. `--initial-files <PATH_TO_TAR>`（`.tar`）
6. `--rewrite-syscalls`
7. `--interception-backend <seccomp|rewriter>`
   - 默认：`seccomp`
   - `rewriter`：依赖重写路径。
8. `--tun-device-name <NAME>`
   - 含义：连接指定 TUN 设备。

---

## 3. 关于“目录挂载”

当前 CLI 未提供以下能力：
1. `--mount`
2. `--bind`
3. `--volume`

可行替代：
1. 把目录打包为 tar。
2. 用 `--initial-files <tar>` 注入。

---

## 4. 常见参数组合

### 4.1 Windows 运行静态 ELF

```powershell
litebox_runner_linux_on_windows_userland.exe \
  D:\tmp\litebox-research\litebox_runner_linux_on_windows_userland\tests\test-bins\hello_world_static
```

### 4.2 Windows 运行动态程序（带 rootfs）

```powershell
litebox_runner_linux_on_windows_userland.exe \
  --unstable \
  --env LD_LIBRARY_PATH=/lib64:/lib32:/lib \
  --initial-files D:\path\rootfs.tar \
  D:\path\program
```

### 4.3 Linux 使用 rewriter 后端

```bash
./litebox_runner_linux_userland \
  --unstable \
  --interception-backend rewriter \
  --initial-files ./rootfs.tar \
  /path/to/program
```

---

## 5. 关键限制（务必知晓）

1. Windows runner 当前限制为 `Windows x86_64`。
2. macOS 目前没有官方 runner/platform 支持。
3. `--insert-file` 现阶段不可用。
4. 使用 `--forward-env` 时要控制敏感变量泄露风险。

---

## 6. Windows 实测兼容建议（2026-03-03）

1. 若出现退出码 `3221225477`（`0xC0000005`），优先尝试：
   - `--unstable --rewrite-syscalls`
2. 该组合在 `hello_world_static` 上可把崩溃转为正常退出（本地实测）。
3. 若进一步出现 `Unsupported madvise behavior NoHugePage`，通常是目标程序触发了当前未实现的行为，不属于参数拼接错误。