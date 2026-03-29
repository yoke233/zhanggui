# 桌面版现状（Wails + Go 后端 + React 前端）

> 状态：现行
> 最后按代码核对：2026-03-29

> 文件名沿用 `tauri-desktop.md` 仅为历史兼容；当前仓库实际桌面壳实现已经是 Wails，而不是 `src-tauri/*` 方案。

## 当前实现摘要

当前桌面版采用：

- **Wails（Go 壳）**：内嵌 `web/dist` 静态资源并暴露前端绑定
- **Go 后端 runtime**：在桌面进程内通过 `BootstrapHTTPRuntime(Command: "desktop")` 启动
- **前端同源访问 API**：桌面模式下仍默认请求 `/api`，WebSocket 仍走 `/api/ws`
- **前端只注入 token**：`DesktopApp.GetBootstrap()` 当前只返回 `token`

这意味着当前桌面端不是“Rust sidecar + 单独 base URL 注入”模型，而是：

`Wails AssetServer + 内嵌前端 + 进程内 Go HTTP handler`

## 目录与关键文件

- `desktop.go`：Wails 应用入口，加载 `web/dist`
- `desktop_app.go`：稳定暴露 `DesktopApp` 绑定名，并转发给平台层实现
- `internal/platform/desktopapp/app.go`：桌面 runtime 生命周期、`GetBootstrap()`、`ServeHTTP()`
- `wails.json`：Wails v2 配置（前端目录、dev watcher、build 命令）
- `web/src/lib/desktopBridge.ts`：前端桌面探测与 `GetBootstrap()` 调用
- `web/src/wailsjs/*`：Wails 生成的前端绑定

## 运行时行为

- 启动时：Wails 进程内创建 `DesktopApp`，并在 `Startup()` 里调用 `BootstrapHTTPRuntime(Command: "desktop")`
- API 承载：`internal/platform/desktopapp/app.go` 中的 `ServeHTTP()` 会把 `/api` 与 `/api/ws` 请求转发给 runtime handler
- 认证：前端桌面模式下通过 `GetBootstrap()` 获取 admin token，并写入本地存储
- Base URL：前端没有从桌面端读取 `api_base_url` / `ws_base_url`；默认仍使用 `VITE_API_BASE_URL || "/api"`
- WebSocket：`createWsClient()` 会在 `/api` 基础上自动拼接 `/ws`

因此当前桌面模式下，前端和后端是“同进程、同源地址、显式注入 token”的关系。

## 当前返回给前端的 bootstrap 数据

当前 `DesktopApp.GetBootstrap()` 返回结构只有：

```json
{
  "token": "..."
}
```

当前并不会返回：

- `api_v1_base_url`
- `api_base_url`
- `ws_base_url`

任何仍把这些字段写成现状能力的文档，都已经落后于代码。

## 开发与构建

开发：

```powershell
npm install
npm run wails:dev
```

构建：

```powershell
npm install
npm run wails:build
```

## 与 Web 工作台的关系

- Web 与桌面端共用同一套 React 工作台代码
- `WorkbenchContext` 在桌面模式下只额外做一件事：调用 `fetchDesktopBootstrap()` 取 token
- 除 token 获取方式外，项目选择、鉴权检查、API Client、WS Client 的主逻辑都与浏览器模式保持一致

## 当前不应再写成现状的内容

以下描述已经不适用于当前仓库：

- `src-tauri/tauri.conf.json`
- Rust sidecar 拉起 `ai-flow server --port <port>`
- 桌面端返回 `api_v1_base_url` / `api_base_url` / `ws_base_url`
- Tauri capabilities / icons / `src-tauri/binaries/*`

如果未来再次切回 Tauri 或多进程 sidecar 方案，需要同步更新本文。
