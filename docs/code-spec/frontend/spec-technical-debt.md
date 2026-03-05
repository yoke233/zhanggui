# Frontend 技术债清单（用于后续重构）

状态：`观察`

## 1. 类型系统漂移

- `web/src/types/workflow.ts` 的 `RunStatus/WorkflowRunStatus` 仍含旧状态语义。
- 与后端 `core.RunStatus/core.RunConclusion` 双轴模型不一致。

处理建议：
- 增加统一的前端适配层，把后端双轴状态映射到 UI 徽标，不在 domain type 中伪造单轴状态。

## 2. 命名与兼容层收敛

- `apiClient` 仍保留 `plan` 命名别名（迁移期可接受，长期应收敛）。
- `runsStore` 仍使用 `RunsByProjectId` / `RunId` 这类大写风格标识，和其余 TS 命名风格不一致。

处理建议：
- 继续将新能力收敛到 issue 命名主路径。
- 对 store 层进行一次命名标准化重构（保持行为不变）。

## 3. 规范归一建议

建议后续按两步走：
1. 先冻结现有 `legacy_alias`，禁止新增同类兼容入口。
2. 再把 `legacy_alias` 逐步收敛为 issue 命名的单一路径。

## 4. 不做的事（当前阶段）

- 不为了“看起来统一”而改写现有可用交互。
- 不把后端未实现接口提前写成前端必需契约。
