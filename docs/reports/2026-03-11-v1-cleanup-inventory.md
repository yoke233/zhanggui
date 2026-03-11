# v1 清理台账（首轮盘点）

日期：2026-03-11

## 目标口径

本台账按当前你的决策记录：

- 后续主线按“当前仓库里的 `internal/v2/*` + `/api/v2` + 新页面壳”推进。
- `v1` 指当前仍挂在 `/api/v1`、旧 issue/run/chat 模型、以及围绕它形成的兼容层与遗留资产。
- `docs/other/v3/*` 目前视为历史/另案设想，不作为这次清理目标实现依据。

## 结论摘要

当前项目不是“仓库里残留一些旧文件”，而是存在四层交织：

1. 运行时双栈：主进程同时启动旧 runtime 和 `v2` runtime，并同时挂 `/api/v1` 与 `/api/v2`。
2. 配置兼容：`v2` agent/profile 仍可从 `v1` 的 `agents.profiles + roles` 推导。
3. 前端双模型：新 `pages/` 路由壳已上线，但大量真实 API 集成仍停留在旧 `apiClient.ts` / `views/*` / `v2/*` 旧视图体系。
4. 文档口径冲突：README、CLAUDE、`docs/other/v3` 对 “v1/v2/v3” 的命名语义互相冲突。

## 分层盘点

### A. 后端运行时与接口

| 项目 | 当前状态 | 判断 | 建议动作 |
|---|---|---|---|
| `cmd/ai-flow/server.go` | 主启动流程先起旧 runtime，再额外挂 `bootstrapV2(...)` | 双栈核心入口 | 最后清；先作为总控点，后续逐步把旧依赖摘掉 |
| `cmd/ai-flow/v2_bootstrap.go` | `v2` 运行时单独建库、事件总线、调度器、handler | 目标主线 | 保留，后续作为唯一目标运行时 |
| `internal/web/server.go` | 同时挂 `/api/v1` 和 `/api/v2` | 双栈出口 | 待迁移完成后收敛为 `/api/v2` 主入口 |
| `internal/web/handlers_v3.go` | 实际是 `/api/v1` 路由注册函数 `registerV1Routes` | 旧接口主注册点，且文件名误导 | 优先重命名/拆分，避免 “v3 文件注册 v1 路由” 的命名污染 |
| `internal/teamleader/*` | 含 legacy review path、compatibility interface、legacy field names | 兼容壳仍在运行链路 | 第二波处理；先明确默认流量是否还落旧评审链 |
| `internal/mcpserver/*`、`internal/teamleader/mcp_tools.go` | 仍默认拼接 `/api/v1/mcp`、`/api/v1/admin/ops/*` | 活跃依赖 | 迁移前不能删，需先改内部 URL 生成策略 |
| `src-tauri/src/main.rs` | 同时暴露 `api_v1_base_url` 和 `api_v2_base_url`，WS/A2A 仍走 `/api/v1` | 桌面壳仍双栈 | 后续需要统一 desktop bootstrap 契约 |

### B. 配置与协议

| 项目 | 当前状态 | 判断 | 建议动作 |
|---|---|---|---|
| `internal/config/defaults.toml` | 明写“`v2.agents` 为空时从 v1 推导” | `v2` 对 `v1` 的关键兼容依赖 | 第二波处理；先补齐纯 v2 配置样例与加载路径 |
| `internal/configruntime/materialize.go` | `BuildV2Agents` 支持从旧 `roles` / `agents` 转换 | 兼容桥核心 | 后续要么删除 fallback，要么下沉成一次性迁移工具 |
| `internal/config/types.go` | 含 legacy YAML dual-format 支持 | 老配置兼容层 | 可放到后段清理，先统计真实使用情况 |
| `configs/prompts/team_leader.tmpl` | 仍写死 `"contract_version": "v1"` | 旧协议标记仍活跃 | 第一波修正项，至少先改命名和说明 |

### C. 前端代码

| 项目 | 当前状态 | 判断 | 建议动作 |
|---|---|---|---|
| `web/src/App.tsx` | 已切到新的 `pages/` + `react-router` | 当前 UI 外壳 | 保留 |
| `web/src/pages/*` | 多数页面仍是静态 mock 数据，未真正接 API | 新壳未落业务 | 这是下一阶段迁移重点 |
| `web/src/lib/apiClient.ts` | 完整绑定 `/api/v1` 的 issue/run/chat/repo/admin API | 旧业务 client 仍完整活着 | 不能直接删；先找活跃 import，再逐页迁移到新 client 或新数据层 |
| `web/src/lib/apiClientV2.ts` | 绑定 `/api/v2` Flow/Step/Execution 模型 | 目标 client | 保留，并逐步扩展覆盖缺口 |
| `web/src/v2/*` | 旧版 V2 页面体系仍在，且有真实 `apiClientV2` 使用 | 过渡实现，不是纯归档 | 先评估是否把其中真实数据逻辑搬到 `pages/`，再决定删 |
| `web/src/v3/*` | 旧版 Issue 模型页面仍在 | 遗留可执行源码 | 若无现网引用，可在后期整体归档或删除 |
| `web/src/archive/legacy/*`、`web/src/_archived/*` | 大量历史副本仍在仓库 | 纯历史/半历史混杂 | 可优先做一轮“只保留一份归档”的瘦身 |
| `web/src/stores/*` | 名称仍偏旧模型，如 `projectsStore` / `runsStore` / `chatStore` | 迁移未完成 | 先确认是否被 `pages/` 使用，再决定保留或重构 |

### D. 测试、脚本、文档

| 项目 | 当前状态 | 判断 | 建议动作 |
|---|---|---|---|
| `scripts/dev.sh` | 仍默认把 `VITE_API_BASE_URL` 指向 `/api/v1` | 本地开发入口仍偏旧 | 第一波修正项 |
| `scripts/test/v2-smoke.ps1`、`v2-pr-flow-smoke.ps1` | 已有 `/api/v2` 冒烟 | 目标链路验证基础 | 保留并扩充 |
| `internal/web/*_test.go` | 大量测试仍直接打 `/api/v1/*` | 活跃测试依赖 | 不能直接删；需先建立对应 `/api/v2` 测试面 |
| `README.md` | 把 `/api/v1` 称为 “V2 API 主链路” | 最容易误导协作 | 第一波修正项 |
| `CLAUDE.md` | 仍描述 `VITE_UI_VERSION` 和旧前端代际关系 | 过时协作文档 | 第一波修正项 |
| `docs/other/v3/*` | 明确说仓库里真正要推进的是 v3，不是 v2 | 与当前决策冲突 | 先标注“历史方案，不作为现行主线” |

## 优先级分组

### P0：先改，不改会持续制造误判

- `README.md`
- `CLAUDE.md`
- `configs/prompts/team_leader.tmpl`
- `internal/web/handlers_v3.go` 的命名
- `scripts/dev.sh`

目标：先统一语言，避免团队继续把 `/api/v1` 当“V2 主链路”。

### P1：先迁再删的活跃依赖

- `internal/web/server.go` 下的 `/api/v1` 路由组
- `web/src/lib/apiClient.ts`
- `internal/teamleader/*` 的 legacy review compatibility
- `internal/configruntime/materialize.go` 的 v1 -> v2 agent/profile fallback
- MCP/A2A/桌面壳里硬编码的 `/api/v1/*`

目标：这部分都在运行链路上，必须先找替代路径，不能粗暴删除。

### P2：可较早瘦身的历史资产

- `web/src/_archived/*`
- `web/src/archive/legacy/*`
- 未被新入口引用的 `web/src/v3/*`
- 重复测试副本
- 历史计划/设计中明显过时的版本切换描述

目标：降低认知噪音，减少后续误引用。

## 建议的三波执行法

### Wave 1：统一命名与协作文档

输出：

- README 改成“当前双栈状态 + 目标收敛方向”
- 明确 `/api/v1 = legacy`，`/api/v2 = target`
- 给 `docs/other/v3/*` 加历史方案标识
- 修正 `team_leader.tmpl` 中的旧 `contract_version` 口径

风险低，收益高，建议立刻做。

### Wave 2：前端先完成真实迁移

输出：

- 新 `pages/` 真正接上 `/api/v2`
- 把 `web/src/v2/*` 里仍有价值的数据逻辑搬入 `pages/`
- 清空现网入口对 `apiClient.ts` 的直接依赖

完成标志：

- 非归档目录中，不再有活跃入口 import `createApiClient`
- `pages/` 不再依赖 mock data 作为主显示来源

### Wave 3：后端收敛与兼容层拆除

输出：

- `/api/v2` 覆盖现有业务主能力
- `/api/v1` 仅保留明确兼容白名单，或整体下线
- 删除 v1 -> v2 配置推导 fallback
- 清理 legacy review path / legacy token / legacy auth 注释与结构

完成标志：

- 主启动流程不再依赖旧 runtime
- 内部 URL 生成不再默认写 `/api/v1/*`

## 建议的下一步

下一步直接进入 Wave 1，比继续讨论更划算。建议我马上做这一波最小清理：

1. 修正 `README.md` 的版本口径。
2. 修正 `CLAUDE.md` 中过时的 UI 版本说明。
3. 把 `internal/web/handlers_v3.go` 改成更准确的命名。
4. 标注 `docs/other/v3/*` 为历史设计，不作为当前实施主线。

## 关键证据

- `cmd/ai-flow/server.go`
- `cmd/ai-flow/v2_bootstrap.go`
- `internal/web/server.go`
- `internal/web/handlers_v3.go`
- `internal/config/defaults.toml`
- `internal/configruntime/materialize.go`
- `configs/prompts/team_leader.tmpl`
- `web/src/App.tsx`
- `web/src/pages/DashboardPage.tsx`
- `web/src/pages/ProjectsPage.tsx`
- `web/src/lib/apiClient.ts`
- `web/src/lib/apiClientV2.ts`
- `web/src/v2/AppV2.tsx`
- `web/src/v3/views/OverviewView.tsx`
- `README.md`
- `CLAUDE.md`
- `docs/other/v3/README.md`
