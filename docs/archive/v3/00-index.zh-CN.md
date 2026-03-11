# AI自主化工作流架构设计

> 来源：Notion MCP 抓取
>
> 页面：`AI自主化工作流架构设计`
>
> URL：`https://www.notion.so/31d4a9d94a3581b78323f49baf8c71ac`
>
> 抓取时间：`2026-03-09T00:15:37.571Z`

本项目设计一个AI自主化工作流系统，核心理念：**控制面极简，AI自主决策，接口隔离变化。**

本目录包含两份文档：
- **v3 主文档**：完整架构设计，包含领域模型、接口层、状态机、场景走查、已知问题与解决方案
- **补充文档**：v3之后的迭代，包含Session合并入Task、定时任务、标签、门禁、Dashboard等设计

## 目录链接

- [v3 主文档 — 完整架构设计](01-v3-main-architecture.zh-CN.md)
- [补充文档 — v3之后的迭代](02-v3-post-iterations.zh-CN.md)
- [上下文与Prompt缓存策略详细设计](03-context-prompt-cache.zh-CN.md)
- [OpenViking 与本系统的关联分析](04-openviking-analysis.zh-CN.md)
- [v3 交付物落地设计讨论 — Artifact、代码提交、文件上传与存储](08-artifact-delivery-and-storage.zh-CN.md)

## Notion 根页中列出的其他后续页面

- `v3.1 模型修订 — Thread引入 / Workspace降级 / 群聊支持`
- `设计反思 — 结构服务于Prompt质量`
- `OpenViking聊天存储设计 — Thread在OV中的落地方案`
- `Phase 0 代码骨架（Bun + TypeScript）`
