# 规格一致性：把“对齐”做成机制

## 本次讨论的共识

我们要避免的主要问题：文档/规格不一致导致产出不一致。

为此采用的原则：

- 单一真源（Single Source of Truth）：同一类规格只认一个权威位置
- 单写者（Single Writer）：关键规格（尤其 contracts/proto）只允许少数 owner 合并
- 机制化校验（Executable Checks）：能用 CI/测试拦住的漂移，不靠人肉 review

## 建议的“真源划分”

- 接口契约：`contracts` repo 的 proto（唯一真源）
- subagent 工具能力：代码中的 tool schema（例如 `agent/tools/...`）为真源
- 运行时加载规则：以 `docs/requirements/subagent-layering-and-agents-dir.md` 为约定文档
- 流程约定：`docs/workflow/`（本目录）

## 曾观察到的漂移风险（示例，已修复）

- `docs/subagent.md` 的 `sessions_spawn` 参数表曾与代码能力不一致：
  - 代码 `agent/tools/subagent_spawn_tool.go` 支持 `task_id`、`repo_dir`、`mcp_config_path`
  - 文档曾遗漏上述字段（已在 2026-02-14 补齐）

这类漂移会直接导致不同角色对“工具能力”的理解不一致。

备注：

- 即使 V1 工作流不使用 `task_id` 作为协作主线，工具能力本身仍支持该参数；因此文档仍应与代码能力对齐，避免“有人以为不能用/有人以为能用”的理解分叉。

## 漂移治理（两条路二选一）

### 路线 A：代码真源，文档生成

- 以 `SubagentSpawnTool.Parameters()` 的 schema 为真源
- 自动生成 `docs/subagent.md` 的参数表片段

优点：不会反向漂移；缺点：需要补一段生成脚本/流程。

### 路线 B：文档真源，测试断言

- 保持 `docs/subagent.md` 手写
- 增加测试：读取文档中的参数列表，与 `Parameters()` 对比，不一致则测试失败

优点：改动小；缺点：需要维护“解析文档”的规则。

## contracts 一致性的机制建议

- `CODEOWNERS` 锁定 proto/生成配置目录
- `buf lint` + `buf breaking` 作为强制检查
- PR 必须引用 `contracts@<sha|tag>`，实现 repo 只能引用不复制
