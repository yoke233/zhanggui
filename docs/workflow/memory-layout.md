# 记忆布局（项目级共享 + 角色子目录，基于 memsearch ccplugin 思路）

## 目标

我们希望解决的是“多角色协作的连续性问题”，而不是把系统做成一个重型知识库。

因此记忆层的目标是：

- 透明：人可以直接打开 Markdown 看见 AI 记住了什么
- 可控：可以编辑/删除/迁移，不被黑盒数据库绑架
- 低成本：默认自动注入，不引入 MCP 工具定义常驻上下文的开销
- 可分工：项目级共享记忆 + 角色私有记忆隔离，避免互相污染

其中“自动注入、Markdown 真源、索引可重建”的设计，参考 memsearch ccplugin 的思路（自动注入 top-k、Markdown 作为真源、索引仅作缓存）。

## 三层检索，但本项目不额外存 L3

为了兼顾“默认轻量”和“按需深入”，推荐三层检索路径：

- L1（自动层）：每次工作前自动注入 top-k 相关记忆片段（短预览 + 指针）。
- L2（按需层）：需要细节时再展开对应 Markdown 章节（完整段落 + 元数据）。
- L3（原始层）：需要追溯原始对话/执行细节时，从 session 系统取回 transcript/日志。

本次讨论的明确选择：

- 不额外存 L3（不再复制一份 JSONL/observations 到记忆目录）。
- 原因：session 体系已保存原始记录；记忆目录只存“可读、可维护、可迁移”的 Markdown。

## 目录结构（推荐）

一个项目一个 memory root；每个角色一个子目录。

memory root 的位置不强制，但需要可配置（建议写进 `<outbox_repo>/workflow.toml`，见下文）。

推荐布局如下：

```text
<memory_root>/
  shared/
    CONSTITUTION.md        # 项目“宪法/硬约束”（尽量少改，review 严格）
    DECISIONS.md           # 已 Accepted 的关键决策（含 IssueRef/PRRef）
    CONTRACTS.md           # contracts@版本引用、破坏性变更约定、生成策略摘要
    RUNBOOK.md             # 常用命令/排障套路（必须可执行）

  roles/
    architect/
      MEMORY.md            # 该角色长期稳定记忆（接口设计偏好、裁决原则…）
      2026-02-14.md        # 每日/每 session 摘要（可追加）
    backend/
      MEMORY.md
      2026-02-14.md
    frontend/
      MEMORY.md
      2026-02-14.md
    qa/
      MEMORY.md
      2026-02-14.md
    integrator/
      MEMORY.md
      2026-02-14.md
    recorder/
      MEMORY.md
      2026-02-14.md

  index/                   # 语义索引缓存（可删除，可重建）
```

说明：

- `shared/` 放“跨角色都应一致的事实”，并且应该可被 review/盖章。
- `roles/<role>/` 放“该角色的工作偏好、踩坑经验、历史摘要”，不要求每个角色都存在；由 `roles.enabled` 决定。
- `index/` 是缓存，不作为真源。真源永远是 Markdown。

## 单写者规则（避免记忆分叉）

记忆最容易出问题的点是：多个 Worker 同时写“结论”，最后谁都说不清哪个可信。

推荐硬规则：

- 共享记忆（`shared/`）只允许 Lead/Recorder 写入（Single Writer）。
- Worker 不允许直接写共享记忆；Worker 只能在 Issue 里提案/给证据，由 Lead 盖章后落盘。
- 角色记忆（`roles/<role>/`）也尽量保持单写者：该 role 的 Lead 写。

## 写入策略（什么时候写，写什么）

建议“写入触发点”不要太多，否则会变成噪音。

推荐触发点：

- Decision Accepted：当 issue 上的关键提案被 `/accept` 并打上 `decision:accepted` 后，把结论写入 `shared/DECISIONS.md`。
- 合并关键 PR：当 contracts / breaking change / 大重构合并后，写入 `shared/CONTRACTS.md` 或 `shared/RUNBOOK.md`。
- 每日摘要：Lead 在一天结束（或一个阶段完成）时，往 `roles/<role>/<YYYY-MM-DD>.md` 追加“今天做了什么、结论是什么、遗留风险是什么”。

不推荐写入：

- 未盖章的争议讨论
- 临时猜测、没有证据的推断
- 很快会过时的实现细节（除非是 runbook 级别的排障步骤）

## 检索与注入策略（默认自动，按需展开）

核心原则：默认返回“短预览 + 指针”，避免一次注入太长导致 token 浪费。

建议注入格式（L1）：

- top-k（例如 3）条语义检索结果
- 每条最多 200-300 字预览
- 必须包含指针：`file path + line/section` 或稳定 anchor

按需展开（L2）：

- 当 Worker/Lead 明确请求“展开第 N 条”时，再把对应章节全文插入上下文

L3（原始记录）：

- 当需要追责/复现/找细节时，从 session 系统取 transcript/日志片段
- 不把 transcript 再写回 memory root（除非做“脱敏后的 runbook”）

## 在 `workflow.toml` 中声明（建议）

为了让不同项目动态决定“记忆根目录在哪、是否启用角色隔离”，建议在 `<outbox_repo>/workflow.toml` 增加可选段：

```toml
[memory]
root = "D:\\workspace\\_goclaw_memory\\my-project"
layout = "project+roles" # 预留：project-only | project+roles
auto_inject_topk = 3
```

约定：

- `root` 是项目级（不是某个 repo 私有），多 repo 项目也共用一个 root。
- 不同 role 的 Lead 在该 root 下写各自子目录，避免互相覆盖。

## 与 goclaw 现状的兼容思路（非强制）

当前 goclaw 已有 `workspace/memory/YYYY-MM-DD.md` 与 `workspace/memory/MEMORY.md` 的模式。

向“项目级 + 角色子目录”演进时，可以：

- 保持 Markdown 真源不变（仍是 `.md` 文件）
- 将 memory root 从 `workspace/memory` 抽象为可配置路径
- 在 root 下增加 `roles/<role>/`，并让 Lead 使用自己的 role 目录
- 语义索引保持可重建（例如现有 memsearch/search manager 都可以对目录递归索引）

