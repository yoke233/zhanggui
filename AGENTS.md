# Repository Guidelines

## 项目结构与模块组织
- `cmd/ai-flow`：主程序入口（CLI + server）。
- `cmd/acp-smoke`：协议/集成冒烟入口。
- `internal/`：核心业务代码，按职责拆分为 `core`、`engine`、`secretary`、`web`、`github`、`plugins`、`git`、`tui` 等包。
- `web/`：前端（Vite + React + TypeScript），主要目录为 `src/views`、`src/components`、`src/lib`、`src/stores`、`src/types`。
- `configs/`：默认配置与提示词模板；`scripts/test/`：一键测试脚本；`docs/`：规格、计划与学习记录。

## 构建、测试与本地开发命令
- 后端启动：`go run ./cmd/ai-flow server --port 8080`
- 前端安装依赖：`npm --prefix web install`
- 前端开发：`npm --prefix web run dev -- --strictPort`
- 后端全量测试：`pwsh -NoProfile -File .\scripts\test\backend-all.ps1`
- 前端单测：`pwsh -NoProfile -File .\scripts\test\frontend-unit.ps1`
- 前端构建验证：`pwsh -NoProfile -File .\scripts\test\frontend-build.ps1`
- 端到端集成回归：`pwsh -NoProfile -File .\scripts\test\p3-integration.ps1`

## 编码风格与命名规范
- Go 代码必须可通过 `gofmt`；包名使用小写短词，文件名沿用现有 `snake_case` 风格。
- TypeScript/React 保持现有风格：2 空格缩进、双引号、语句分号、按功能分层组织。
- 新增 API/事件/模型时，优先在 `internal/core` 定义领域对象，再向 `engine`、`web`、`plugins` 扩展。

## 测试规范
- Go 测试文件命名：`*_test.go`；前端测试命名：`*.test.ts` / `*.test.tsx`。
- 修改后至少运行受影响模块测试；涉及跨层改动时运行 `p3-integration.ps1`。
- Web 交互改动建议补充组件测试（`web/src/**`）并覆盖关键状态分支。

## 提交与 Pull Request 规范
- 提交信息遵循 Conventional Commits：`feat(scope): ...`、`fix: ...`、`test(scope): ...`、`chore: ...`。
- PR 需包含：变更摘要、影响范围、测试命令与结果、回滚方式。
- 涉及 UI 变更时附截图或录屏；涉及 GitHub 集成时说明所用配置与模拟数据。

## 安全与配置建议
- 严禁提交密钥、令牌和本地绝对路径。
- 通过环境变量注入敏感配置（如 `AI_WORKFLOW_CHAT_PROVIDER`、`VITE_API_TOKEN`），默认值仅用于本地开发。

## 协作与提交流程建议
- 开始改动前先检索：优先用 `rg -n 'pattern' internal web` 定位，再打开文件做最小修改。
- 提交前建议执行最小闭环：后端改动跑 `backend-all.ps1`，前端改动跑 `frontend-unit.ps1` + `frontend-build.ps1`。
- 涉及接口或事件字段变更时，同步更新 `web/src/types` 与相关 handler 测试，避免前后端契约漂移。
- 评审说明尽量使用“变更点 + 风险点 + 验证证据”三段式，便于快速合并与回归排查。
