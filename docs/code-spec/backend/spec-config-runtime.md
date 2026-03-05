# 配置加载规范（代码事实版）

状态：`保留`

## 1. 启动时配置来源

`ai-flow server` 的启动配置加载顺序：
1. 读取当前工作目录下 `.ai-workflow/config.yaml`
2. 若文件不存在：回退到内置 `Defaults()`
3. 在上述结果上应用环境变量覆盖（`ApplyEnvOverrides`）

关键事实：
- 启动阶段不会自动回退到 `~/.ai-workflow/config.yaml`。
- 缺省时会打印提示，建议用户执行 `ai-flow config init` 生成本地模板。

## 2. `config init` 行为

`ai-flow config init [--force]`：
- 在当前目录创建 `.ai-workflow/config.yaml`
- 模板优先来自 `configs/defaults.yaml`
- 若模板不存在，回退到运行时 `Defaults()` 序列化结果

## 3. 全局加载与项目层加载

`internal/config` 当前提供两种加载语义：
- `LoadGlobal(path)`：`Defaults -> 文件层 -> 环境变量 -> validate`
- `LoadProject(repoPath)`：读取 `<repoPath>/.ai-workflow/config.yaml`，文件不存在时返回空层（不报错）

## 4. 已实现的环境变量覆盖项

当前代码中显式支持：
- `AI_WORKFLOW_AGENTS_CLAUDE_BINARY`
- `AI_WORKFLOW_SERVER_PORT`
- `AI_WORKFLOW_SCHEDULER_MAX_GLOBAL_AGENTS`
- `AI_WORKFLOW_A2A_ENABLED`
- `AI_WORKFLOW_A2A_TOKEN`
- `AI_WORKFLOW_A2A_VERSION`
- `AI_WORKFLOW_GITHUB_TOKEN`

补充：
- `ai-flow mcp-serve` 还使用独立环境变量：
  - `AI_WORKFLOW_DB_PATH`（必需）
  - `AI_WORKFLOW_DEV_MODE`
  - `AI_WORKFLOW_SOURCE_ROOT`
  - `AI_WORKFLOW_SERVER_ADDR`

## 5. 默认路径（当前默认值）

- `store.path` 默认：`.ai-workflow/data.db`
- `log.file` 默认：`.ai-workflow/logs/app.log`

这两个默认值都落在当前项目目录内，而非用户 Home 目录。
