# GitHub PAT（push/PR/merge）冒烟结果（2026-03-10）

## 结论

- `commit_pat` 可用于 **HTTPS git push**（创建 `main`、推送 feature 分支均成功）。
- 使用同一 `commit_pat` 调用 GitHub REST 创建 PR：`POST /repos/{owner}/{repo}/pulls` 返回 **403**（`Resource not accessible by personal access token`）。
- 但使用 `merge_pat`（单 token 模式）可跑通：**push → create PR → merge PR**（PR #1 已 squash merge 成功，并已删除远程分支）。

## 建议（让 PR/merge 流程可用）

为 `commit_pat` / `merge_pat` 重新配置（或重发）Fine-grained PAT 权限：

- Repository access：包含目标仓库 `yoke233/test-workflow`
- Permissions（至少）：
  - **Contents: Read and write**（推送分支/提交）
  - **Pull requests: Read and write**（创建 PR、更新 PR、merge PR）
  - 视实现需要：**Metadata: Read-only**（默认就有）

若仓库开启了 Branch protection / Required checks，merge 还需要满足对应规则（CI 通过、review 状态等）。

## 复现脚本

- `scripts/test/github-pat-smoke.ps1`
  - 单 token（全部用 merge_pat）：`pwsh -NoProfile -File .\\scripts\\test\\github-pat-smoke.ps1 -UseMergePatOnly`
