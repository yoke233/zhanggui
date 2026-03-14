# Tauri 桌面版（Go 后端 + React 前端）

> 状态：现行
> 最后按代码核对：2026-03-14

本仓库的桌面版采用：

- **Tauri（Rust 壳）**：加载 `web/dist` 的静态资源
- **Go 后端（cmd/ai-flow）**：作为 sidecar 在桌面应用启动时自动运行
- **前端自动注入 Token/BaseURL**：首次启动无需手动拼 `?token=...`

## 当前实现摘要

- Tauri 启动时会拉起 Go sidecar，并在 Rust 侧提供 `desktop_bootstrap` 命令。
- `desktop_bootstrap` 当前返回 `token`、`api_v1_base_url`、`api_base_url`、`ws_base_url`。
- 前端当前主工作台实际统一消费 `api_base_url` 作为 HTTP 与 WebSocket 基址。
- `api_v1_base_url` 与 `ws_base_url` 目前仍作为兼容字段返回，但主工作台并未优先使用它们。

## 目录与关键文件

- `src-tauri/tauri.conf.json`：Tauri v2 配置（devUrl / frontendDist / bundle.externalBin 等）
- `src-tauri/src/main.rs`：启动 Go sidecar、提供 `desktop_bootstrap` 命令（给前端取 token/baseUrl）
- `scripts/tauri/build-sidecar.mjs`：将 `cmd/ai-flow` 编译成 Tauri sidecar（二进制输出到 `src-tauri/binaries/`）
- `web/src/lib/desktopBridge.ts`：前端在 Tauri 环境下调用 `desktop_bootstrap`
- `src-tauri/capabilities/default.json`：Tauri v2 capabilities（当前仅启用 `core:default`）
- `src-tauri/icons/*`：应用图标（由 `tauri icon` 生成）

## 运行时行为

- 启动时：Tauri 先选择一个空闲端口，然后拉起 `ai-flow server --port <port>`。
- 数据目录：sidecar 会使用 `AI_WORKFLOW_DATA_DIR`（由 Tauri 指向 app_data_dir 下的 `ai-workflow/`）存放 `config.toml` 与 `secrets.toml`。
- 认证：前端通过 `desktop_bootstrap` 读取 `secrets.toml` 中的 `tokens.admin.token`，并保存到 `localStorage`。
- API/WS：当前桌面端会同时注入：
  - `api_v1_base_url = http://127.0.0.1:<port>/api/v1`
  - `api_base_url = http://127.0.0.1:<port>/api`
  - `ws_base_url = http://127.0.0.1:<port>/api/v1`
- 但当前前端主工作台统一使用 `api_base_url`，`createWsClient()` 会自行拼接到 `${api_base_url}/ws`。

## 开发与构建

Windows：

```powershell
npm install
npm run tauri:icons
npm run tauri:dev
```

构建：

```powershell
npm install
npm run tauri:build
```

## 先决条件（Windows 常见坑）

- Rust 工具链（stable、MSVC）
- Microsoft Edge WebView2 Runtime（通常系统自带/可安装）
