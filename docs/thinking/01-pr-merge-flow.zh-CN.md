# PR/Merge 流程设计

## 问题

当前 Issue 生命周期中 `executing → done` 之间缺少合并阶段。
多个子 Issue（epic 分解后）并行执行时，先合并的改动会导致后续 PR 冲突。

## 核心洞察

1. **Issue done = 代码合入 main**，不是 coder 写完代码
2. **PR 创建不会冲突，合并才会** — GitHub 允许带冲突创建 PR
3. **内部已审核过，PR 只是集成机制** — 不需要 GitHub PR review

## 状态机扩展

新增 `merging` 状态：

```
executing → [RunDone] → merging → [合并成功] → done
                            ↓ [冲突/失败]
                        TL Triage
                            ├→ 打回 coder（→ executing，新 Run）
                            ├→ TL 调整后重试
                            └→ 升级人类（→ human inbox）
```

## 冲突解决流程

```
Child #101 done → PR → merge 成功 (first, no conflict)
Child #102 done → PR → merge 冲突
  → EventIssueMergeConflict
  → TL Triage Handler：
      读取 #102 PR 内容 + #101 已合并变更 + 父 #100 spec
      决策：retry + 给 coder 指示 "rebase main, 解决 handler.go 冲突"
  → issue #102 → executing（新 Run，注入 TL 指示）
  → coder rebase + 解决冲突 + 提交
  → 再次 merge → 成功 → done
```

## MergeHandler 拆分

当前 AutoMergeHandler 做了 test + PR + merge。拆为：
- **MergeHandler** — test gate + PR 创建 + merge 尝试（成功发 EventIssueMerged，冲突发 EventIssueMergeConflict）
- **TL Triage Handler** — 所有异常的统一决策入口

## 冲突解决后的 re-review

TL 可设 `SkipReview=true`：仅冲突解决（无逻辑变更）跳过全量审核。
如有实质逻辑变更，走正常 review。

---

> **后续推导**: TL Triage 作为统一异常处理入口的思路，在 [02-Escalation/Directive 模式](02-escalation-directive-pattern.zh-CN.md) 中被泛化为所有 Agent 通用的层级决策协议。
