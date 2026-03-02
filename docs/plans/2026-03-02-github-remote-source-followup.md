# 2026-03-02 Git Remote Source Follow-up

## 决策记录

- 用户新增要求：`github_clone` 不再强制拆分 `owner/repo` 两段输入。
- 目标输入模型：前端优先支持单一 `remote_url`（可为 SSH 或 HTTPS），后端按远程地址直接 clone。
- 远程源不限定 GitHub，未来可能传入其他 Git 平台地址。

## 本轮执行策略

- 本轮先完成回归测试与联调验证，不在当前提交内继续扩展 `remote_url` 功能实现。
- `remote_url` 单输入模式作为后续开发项，在下一轮需求确认后落地。

