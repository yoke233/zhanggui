# Codex Sandbox 配置实战笔记（ai-workflow）

更新时间：2026-03-05

## 1. 目标与结论

本笔记的目标是让 Codex 具备以下行为：

1. 可执行命令（自动执行，不弹审批）。
2. 网络请求可用。
3. 写入范围尽量限制在当前项目目录。
4. 尽量避免 Go/Node/Codex 在 `~/.cache`、`~/.npm`、`/tmp` 等目录产生副作用。

本仓库已落地的方案是：

1. 在项目内新增 `.ai-workflow/config.yaml` 作为启动配置覆盖。
2. 通过 `codex-acp` 的 `-c key=value` 注入 Codex CLI 的 sandbox 与环境策略。
3. 将 `HOME/TMPDIR/NPM/GO/CODEX_HOME` 指向项目内目录（`.ai-workflow/*`）。

## 2. 如何使用

### 2.1 准备目录

在仓库根目录执行（可重复执行）：

```bash
mkdir -p \
  .ai-workflow/home \
  .ai-workflow/tmp \
  .ai-workflow/npm-cache \
  .ai-workflow/xdg-cache \
  .ai-workflow/go-cache \
  .ai-workflow/go-mod-cache \
  .ai-workflow/codex-home
```

### 2.2 使用项目内配置

确保文件存在：

- `.ai-workflow/config.yaml`

`ai-flow` 启动时会优先读取该路径（不存在才回落默认配置）。

### 2.3 启动与运行

按你原来的方式启动服务或任务即可，例如：

```bash
go run ./cmd/ai-flow server --port 8080
```

如果本机 Go 全局缓存目录权限有问题，可临时把 Go 缓存也指向项目内（仅当前命令生效）：

```bash
GOCACHE=$PWD/.ai-workflow/go-cache \
GOMODCACHE=$PWD/.ai-workflow/go-mod-cache \
go run ./cmd/ai-flow server --port 8080
```

## 3. 核心配置说明（为什么这么配）

`codex` agent 里关键项如下：

1. `sandbox_mode="workspace-write"`  
   在工作区内允许读写与执行。
2. `approval_policy="never"`  
   自动执行，不弹审批。
3. `sandbox_workspace_write.network_access=true`  
   workspace-write 模式下放开出网。
4. `sandbox_workspace_write.exclude_slash_tmp=true`  
   不把 `/tmp` 自动当作可写根。
5. `sandbox_workspace_write.exclude_tmpdir_env_var=true`  
   不把 `$TMPDIR` 自动当作可写根。
6. `sandbox_workspace_write.writable_roots=[]`  
   不额外追加其他可写根。
7. `shell_environment_policy.inherit="core"` + `shell_environment_policy.set.*`  
   控制子进程继承环境，并显式重定向缓存/临时目录到项目内。

## 4. 边界与风险（必须知道）

当前 `ai-workflow` 的 ACP 终端执行路径是宿主进程直接 `exec.Command(...)`。  
它会做 `cwd` 作用域检查，但不是单独再套一层内核级文件系统沙箱。

这代表：

1. 常见副作用目录已通过环境变量重定向做了强收敛。
2. 但若命令显式写绝对路径，最终是否被阻断仍取决于外层运行环境策略。

如果要“硬保证仅能写当前目录”，需要在执行层增加系统级沙箱/容器隔离，而不只靠配置。

## 5. 引用（参考来源）

下面是本笔记依赖的权威来源，优先官方文档与上游源码：

1. OpenAI Codex - Advanced Config（`-c` 覆盖、shell environment policy）  
   https://developers.openai.com/codex/config-advanced
2. OpenAI Codex - Config Reference（`sandbox_workspace_write.*`、`approval_policy`、`shell_environment_policy.*`）  
   https://developers.openai.com/codex/config-reference
3. OpenAI Codex - Security（workspace-write 默认包含当前目录与临时目录如 `/tmp`）  
   https://developers.openai.com/codex/security
4. OpenAI Codex 开源仓库中的配置入口说明（文档迁移到 developers 站点）  
   https://raw.githubusercontent.com/openai/codex/main/docs/config.md
5. zed-industries/codex-acp README（ACP 适配器定位与安装方式）  
   https://raw.githubusercontent.com/zed-industries/codex-acp/main/README.md
6. zed-industries/codex-acp main（CLI 解析 `CliConfigOverrides` 并传入 run_main）  
   https://raw.githubusercontent.com/zed-industries/codex-acp/main/src/main.rs
7. zed-industries/codex-acp lib（`parse_overrides` + `Config::load_with_cli_overrides...`）  
   https://raw.githubusercontent.com/zed-industries/codex-acp/main/src/lib.rs

## 6. “引用是什么 / 怎么写”

在 Markdown 里，“引用”通常有两种：

1. 链接引用：`[标题](https://example.com)`
2. 块引用：`> 这里是摘录`

建议工程文档优先使用“链接引用”，并标注你引用的结论点。  
例如：

```md
参考 OpenAI Config Reference，`sandbox_workspace_write.network_access` 用于控制 workspace-write 的出网能力：
https://developers.openai.com/codex/config-reference
```

## 7. 本仓库对应落地点

1. 项目配置文件：`.ai-workflow/config.yaml`
2. 配置加载入口：`cmd/ai-flow/commands.go` 的 `loadBootstrapConfig()`
3. ACP 终端执行入口：`internal/teamleader/acp_handler.go` 的 `CreateTerminal()`
4. 文件作用域限制：`internal/teamleader/acp_handler.go` 的 `normalizePathInScope()` / `normalizeDirInScope()`
