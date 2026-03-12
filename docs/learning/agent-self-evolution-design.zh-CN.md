# Agent 自进化系统设计讨论

> 日期：2026-03-13（v2，整合 Cron Trigger 发现）
> 背景：Harness Engineering 研究 → ai-workflow 对齐分析 → 自进化方向探索
> 状态：设计草案，待讨论

---

## 一、问题定义

当前 ai-workflow 的 Agent 是"无记忆的劳动者"——每次执行拿到 Briefing 就干活，干完就忘。即使同一个 Profile 的 Agent 在 Gate 被拒绝了 10 次（都是因为"忘了写测试"），第 11 次它还是可能犯同样的错。

**核心需求：**
- 项目级的架构规范和编码规则 → 存在项目目录下，通过 Briefing 注入
- Profile 级的经验积累 → Agent 自动分析执行历史，生成/优化 Skill，让自己"进步"

第一个是**静态知识注入**（已在对齐讨论中设计）。第二个是本文要解决的**动态自进化闭环**。

---

## 二、现有基础盘点

### 已有数据（足够丰富）

| 数据源 | 内容 | 位置 |
|--------|------|------|
| **Execution 记录** | 每次执行的成功/失败、耗时、重试次数 | `executions` 表 |
| **Artifact 元数据** | summary, files_changed, tests_passed, verdict | `artifacts` 表 |
| **Gate rework_history** | 每次被拒的原因、PR 状态、修复建议 | `steps.config` JSON |
| **UsageRecord** | token 消耗、执行时长、缓存命中率 | `usage_records` 表 |
| **AnalyticsStore** | 瓶颈步骤、错误分类、失败率排名 | 6 个聚合查询 |
| **ExecutionProbe** | 卡顿诊断（alive/blocked/hung/dead） | `execution_probes` 表 |
| **StepSignal** | Agent 主动信号（need_help, blocked） | `step_signals` 表 |

### 已有 Skill 系统

- **结构**：目录 + `SKILL.md`（YAML frontmatter: name/description/assign_when/version）
- **安装**：符号链接到 Agent 的 skills 目录
- **配置**：`AgentProfile.Skills = ["skill-name"]`
- **特点**：静态的，人工编写，人工分配

### 已有 Cron Trigger 系统

`internal/application/cron/` 提供了完整的定时任务框架：

- **Cron 表达式引擎**：标准 5-field 格式（分 时 日 月 周），支持 `*/n`、范围、列表
- **Issue 模板机制**：任何 Issue 可通过 `Metadata["cron_template"]="true"` 标记为模板
- **自动克隆执行**：触发时克隆 Issue + Steps，提交到 IssueScheduler
- **并发控制**：`cron_max_instances` 限制同时运行的实例数
- **API 支持**：`POST/DELETE /api/issues/:id/cron` 动态启停
- **生命周期**：bootstrap 阶段启动后台 goroutine，context cancel 时优雅退出

这意味着 Reflection Flow 的定时触发**不需要任何新代码**——直接创建 Reflection Issue 并标记为 cron 模板。

### 缺失的闭环

```
执行 → 数据积累 → [???] → Skill 生成/更新 → 注入下次执行
                    ↑ 这里是空白（Cron 可以定时触发，但"分析→生成→验证→部署"需要设计）
```

---

## 三、设计方案：Reflection Flow（反思流）

### 3.1 核心思路

**把 Agent 自进化实现为一种特殊的 Issue**——"Reflection Issue"。完全复用现有的 Issue/Step/Gate/Execution 机制，不需要新的执行路径。

```
┌─────────────────────────────────────────────────────┐
│                  Reflection Flow                     │
│                                                      │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐         │
│  │ Analyze  │──→│ Generate │──→│ Validate │         │
│  │ (exec)   │   │ (exec)   │   │ (gate)   │         │
│  └──────────┘   └──────────┘   └──────────┘         │
│       ↑                              │               │
│       │         Gate Reject          │               │
│       └──────────────────────────────┘               │
│                                      ↓ Gate Pass     │
│                              ┌──────────┐            │
│                              │  Deploy  │            │
│                              │ (builtin)│            │
│                              └──────────┘            │
└─────────────────────────────────────────────────────┘
```

### 3.2 四个步骤

#### Step 1: Analyze（分析）

**类型**: `exec`，由专门的 `reflector` 角色执行

**输入（通过 Briefing 注入）**：
- 目标 Profile 的最近 N 次执行摘要（从 AnalyticsStore 查询）
- 该 Profile 关联的所有 Gate rework_history
- 该 Profile 的 ErrorBreakdown 和 StepBottleneck
- 该 Profile 当前的 Skills 内容（读取 SKILL.md 文件）
- 该 Profile 的 UsageRecord 聚合（token 效率趋势）

**输出（Artifact）**：
```json
{
  "patterns": [
    {
      "type": "recurring_failure",
      "description": "Gate 审查 7/10 次因为缺少单元测试被拒绝",
      "evidence": ["rework #3: no tests", "rework #5: tests missing", ...],
      "frequency": 0.7,
      "impact": "high"
    },
    {
      "type": "efficiency_issue",
      "description": "文件编辑操作平均执行 3.2 轮才收敛，疑似反复尝试",
      "evidence": ["exec #12: 5 turns", "exec #15: 4 turns"],
      "frequency": 0.5,
      "impact": "medium"
    }
  ],
  "current_skills_assessment": {
    "effective": ["skill-go-conventions"],
    "missing": ["testing-patterns", "error-handling-conventions"],
    "outdated": []
  }
}
```

#### Step 2: Generate（生成）

**类型**: `exec`，由 `reflector` 角色执行

**输入**：Step 1 的 Artifact（模式分析结果）+ 当前 Skills 内容

**输出**：新的或更新的 SKILL.md 文件内容，作为 Artifact 的 Assets 附件

```markdown
---
name: testing-discipline
description: 确保所有代码修改都附带相应的单元测试
assign_when: Agent 角色为 worker 且步骤涉及代码实现
version: 1
source: reflection  # 标记为自动生成
generated_from:
  profile: worker-go
  analyzed_executions: 47
  generated_at: 2026-03-12T14:30:00Z
---

# Testing Discipline

## 核心规则
在提交任何代码修改前，必须：
1. 为新增的公开函数编写表驱动测试
2. 运行 `go test ./...` 确认通过
3. 在 SignalComplete 的 payload 中报告测试覆盖率

## 常见陷阱（从历史执行中学到）
- 不要只写 happy path 测试，Gate 审查员会检查边界条件
- 修改已有函数时，先运行现有测试确认没有破坏
- 测试文件放在同包下（`_test.go`），不要放到单独的 test 目录

## 反模式
- ❌ 先提交代码再补测试（Gate 会拒绝）
- ❌ 删除失败的测试来"修复"问题
- ❌ 只测试自己新写的代码，忽略回归
```

#### Step 3: Validate（验证）

**类型**: `gate`，由 `gate` 角色执行（或人工审批）

**验证维度**：
1. **格式合规**：SKILL.md 符合 `ValidateSkillMD()` 规则
2. **内容质量**：规则具体可执行，不是空洞的"写好代码"
3. **无冲突**：不与已有 Skills 矛盾
4. **证据充分**：每条规则都有执行历史的数据支撑

**Gate 拒绝 → 回到 Step 2 重新生成**（利用现有 Gate 反馈机制）

#### Step 4: Deploy（部署）

**类型**: `exec`，`builtin` 执行器

**行为**：
1. 将 Artifact Assets 中的 SKILL.md 写入 `<skillsRoot>/<skill-name>/SKILL.md`
2. 更新 Profile 的 Skills 列表（如果是新 Skill）
3. 调用 `EnsureProfileSkills()` 重新链接
4. 记录部署事件（用于后续效果追踪）

---

### 3.3 触发机制（复用已有 Cron Trigger）

系统已有完整的 Cron 框架（`internal/application/cron/`），Reflection Issue 直接作为 **Cron 模板 Issue** 运行，**零新增触发代码**。

**实现方式：** 创建一个 Reflection Issue，标记为 cron 模板：

```json
// Issue.Metadata
{
  "cron_template": "true",
  "cron_enabled": "true",
  "cron_schedule": "0 2 * * 0",          // 每周日凌晨 2 点
  "cron_max_instances": "1",             // 同时最多 1 个实例
  "reflection_target_profile": "worker-go"  // 自定义字段：反思目标
}
```

**Cron Trigger 已有的能力（直接复用）：**

| 能力 | 已有实现 | Reflection 如何用 |
|------|---------|-------------------|
| 表达式解析 | 标准 5-field cron（`*/n`, 范围, 列表） | `"0 2 * * 0"` 每周日凌晨 |
| 自动克隆 Issue + Steps | `cloneAndSubmit()` | 每次触发克隆出新实例 |
| 并发限制 | `cron_max_instances` | 限制为 1，防止重叠分析 |
| 幂等追踪 | `cron_last_triggered` | 自动记录上次触发时间 |
| 提交调度 | `scheduler.Submit()` | 无缝进入现有执行流 |
| API 控制 | `POST /api/issues/:id/cron` | 动态启用/禁用 |

**补充触发方式（可选，Phase 4）：**

除了 cron 定时触发外，可在 `IssueEngine.finalizeStep()` 中加入事件驱动触发：

```go
// 在 step 完成后检查是否需要提前触发 Reflection
func (e *IssueEngine) checkReflectionTrigger(ctx context.Context, profileID string) {
    // 查询最近 N 次执行的失败率
    stats := e.analytics.ErrorBreakdown(ctx, AnalyticsFilter{ProfileID: profileID, Since: last7Days})
    failRate := float64(stats.FailCount) / float64(stats.TotalCount)
    if failRate > 0.3 {
        // 失败率超过 30%，提前触发 Reflection
        e.scheduler.Submit(reflectionIssueID)
    }
}
```

但这是 Phase 4 的增强——Phase 1-3 仅用 cron 就足够了。

---

### 3.4 数据组装（Reflection Briefing）

这是整个设计的关键——如何把执行历史压缩成 Agent 可理解的分析材料。

新增一个专用的 BriefingBuilder 扩展：

```go
type ReflectionDataAssembler struct {
    store     Store
    analytics AnalyticsStore
    usage     UsageStore
    skillRoot string
}

func (r *ReflectionDataAssembler) AssembleForProfile(
    ctx context.Context, profileID string, since time.Time,
) (*ReflectionData, error) {

    data := &ReflectionData{}

    // 1. 执行统计概览
    data.ExecutionStats = r.analytics.IssueBottleneckSteps(ctx, AnalyticsFilter{
        ProfileID: profileID, Since: since,
    })

    // 2. 错误模式
    data.ErrorBreakdown = r.analytics.ErrorBreakdown(ctx, AnalyticsFilter{
        ProfileID: profileID, Since: since,
    })

    // 3. 最近失败详情（含 ErrorMessage）
    data.RecentFailures = r.analytics.RecentFailures(ctx, AnalyticsFilter{
        ProfileID: profileID, Since: since, Limit: 20,
    })

    // 4. Gate 拒绝历史（从 Step.Config 中提取）
    data.GateRejections = r.collectGateRejections(ctx, profileID, since)

    // 5. Token 效率趋势
    data.UsageTrend = r.usage.UsageByProfile(ctx, profileID)

    // 6. 当前 Skills 内容
    data.CurrentSkills = r.readCurrentSkills(profileID)

    return data, nil
}
```

**输出格式（注入 Briefing 的 Markdown）**：

```markdown
# Reflection Data for Profile: worker-go

## 执行统计（最近 30 天）
- 总执行次数: 47
- 成功率: 68% (32/47)
- 平均执行时长: 4.2 分钟
- 平均重试次数: 1.3

## 错误分类
- transient: 8 次 (53%) — 网络超时、API 限流
- permanent: 4 次 (27%) — 编译错误、类型不匹配
- need_help: 3 次 (20%) — 需求不明确

## Gate 拒绝原因 TOP 5
1. "缺少单元测试" — 7 次
2. "未处理错误返回值" — 3 次
3. "不符合项目编码规范" — 2 次
4. "PR 合并冲突" — 2 次
5. "日志级别使用不当" — 1 次

## 最近 5 次失败详情
1. exec #234: "编译失败: undefined: slog.Error" (permanent)
2. exec #231: "测试超时: context deadline exceeded" (transient)
...

## 当前已装载 Skills
- skill-go-conventions (v2): Go 编码规范
- skill-git-workflow (v1): Git 工作流

## Token 消耗趋势
- 上周平均: 12,400 tokens/exec
- 本周平均: 15,200 tokens/exec (+22%)
- 缓存命中率: 34%
```

---

## 四、Skill 的分类体系

### 4.1 按来源分类

| 类型 | 来源 | 示例 | 生命周期 |
|------|------|------|---------|
| **Manual Skill** | 人工编写 | `skill-go-conventions` | 人工维护 |
| **Reflected Skill** | Reflection Flow 自动生成 | `skill-testing-discipline` | 自动迭代 |
| **Project Skill** | 项目目录下的 `AGENTS.md` | 架构规范、编码规则 | 随项目演进 |

### 4.2 Reflected Skill 的元数据扩展

在 SKILL.md 的 frontmatter 中增加反思来源信息：

```yaml
---
name: testing-discipline
description: ...
assign_when: ...
version: 3
source: reflection           # 标记来源
profile_origin: worker-go    # 哪个 Profile 生成的
evidence_window: 2026-02-10/2026-03-12  # 分析的时间窗口
execution_count: 47          # 基于多少次执行
effectiveness:               # 效果追踪（deploy 后回填）
  before_success_rate: 0.68
  after_success_rate: null    # 部署后待填
  measured_at: null
---
```

### 4.3 Skill 的跨 Profile 传播

好的 Skill 不应该局限于生成它的 Profile。设计传播机制：

```
Profile A 生成 Skill X（version 1）
  ↓ 效果验证：成功率从 68% → 85%
  ↓
Skill X 标记为 "proven"
  ↓
相同 Role 的其他 Profiles（B, C）自动继承
  ↓
各 Profile 可在自己的 Reflection 中进一步定制
```

配置：
```toml
[reflection]
skill_propagation = "same_role"  # none | same_role | all
```

---

## 五、效果追踪（Feedback on Feedback）

Skill 部署后需要追踪效果，否则可能越改越差。

### 5.1 A/B 对比

在 Skill 部署时记录基线指标：

```go
type SkillDeployment struct {
    SkillName    string
    ProfileID    string
    Version      int
    DeployedAt   time.Time

    // 部署前基线（从 AnalyticsStore 快照）
    BaselineSuccessRate float64
    BaselineAvgRetries  float64
    BaselineAvgTokens   int64

    // 部署后指标（定期更新）
    PostSuccessRate     *float64
    PostAvgRetries      *float64
    PostAvgTokens       *int64
    MeasuredAt          *time.Time
}
```

### 5.2 自动回滚

```go
// 在下一次 Reflection 触发时，检查上次部署的 Skill 效果
if deployment.PostSuccessRate != nil {
    delta := *deployment.PostSuccessRate - deployment.BaselineSuccessRate
    if delta < -0.1 { // 成功率下降超过 10%
        // 回滚：禁用该 Skill，恢复上一版本
        rollbackSkill(deployment.SkillName, deployment.Version-1)
        // 记录回滚事件
        publishEvent(EventSkillRolledBack, ...)
    }
}
```

---

## 六、Project Skills（项目级知识）

这是第一个问题——项目架构规范和编码规则的存储。

### 6.1 存储位置

```
project-repo/
├── .ai-workflow/
│   ├── config.toml          # 已有
│   ├── secrets.yaml         # 已有
│   └── skills/              # 新增：项目级 Skills
│       ├── arch-rules/
│       │   └── SKILL.md     # 架构规范
│       └── coding-style/
│           └── SKILL.md     # 编码风格
```

或者更简单——直接用一个文件：

```
project-repo/
├── AGENTS.md                # 项目级 Agent 指南（OpenAI/Anthropic 风格）
```

### 6.2 注入机制

在 `BriefingBuilder.Build()` 中：

```go
// 从 workspace 读取项目级 skills
if ws := WorkspaceFromContext(ctx); ws != nil {
    agentsMD := filepath.Join(ws.Dir, "AGENTS.md")
    if content, err := os.ReadFile(agentsMD); err == nil {
        briefing.ContextRefs = append(briefing.ContextRefs, ContextRef{
            Type:   CtxProjectRules,
            Label:  "Project Engineering Rules",
            Inline: truncate(string(content), 2000),
        })
    }

    // 或者扫描 .ai-workflow/skills/ 目录
    projectSkillsDir := filepath.Join(ws.Dir, ".ai-workflow", "skills")
    // ...
}
```

### 6.3 与 Reflected Skills 的关系

```
优先级：Project Skills > Reflected Skills > Manual Skills

Project Skill：  "我们用 chi v5 做路由"        ← 项目事实
Reflected Skill："先写测试再写实现"              ← 从执行经验学到
Manual Skill：   "Go 函数命名用 camelCase"      ← 全局规范
```

冲突解决：项目级覆盖全局级。Reflection 生成的 Skill 如果与 Project Skill 矛盾，在 Validate Gate 中被拒绝。

---

## 七、完整数据流

```
                    ┌─────────────────────┐
                    │   正常任务执行       │
                    │  (Issue/Step/Exec)  │
                    └────────┬────────────┘
                             │
                    ┌────────▼────────────┐
                    │   数据积累           │
                    │  Execution records  │
                    │  Artifacts          │
                    │  Gate rework_history│
                    │  UsageRecords       │
                    │  Events             │
                    └────────┬────────────┘
                             │
                   触发条件满足？
                  (count/rate/cron)
                             │ yes
                    ┌────────▼────────────┐
                    │  Reflection Issue    │
                    │                      │
                    │  Step 1: Analyze     │◄── ReflectionDataAssembler
                    │  Step 2: Generate    │     查询 Analytics/Usage/
                    │  Step 3: Validate    │     rework_history
                    │  Step 4: Deploy      │
                    └────────┬────────────┘
                             │
                    ┌────────▼────────────┐
                    │  Skill 更新          │
                    │  SKILL.md 写入       │
                    │  Profile 链接更新    │
                    │  基线指标快照        │
                    └────────┬────────────┘
                             │
                    ┌────────▼────────────┐
                    │  下次任务执行        │
                    │  Skill 自动注入      │
                    │  Agent 使用新知识    │
                    └────────┬────────────┘
                             │
                    ┌────────▼────────────┐
                    │  效果追踪            │
                    │  对比 before/after   │
                    │  自动回滚 if 退步    │
                    └─────────────────────┘
```

---

## 八、实现路线图

### Phase 1: 静态知识注入（最快见效）

**改动量**: ~100 行 | **新增代码**: 0 个新包

1. 新增 `CtxProjectRules` ContextRefType
2. `BriefingBuilder.Build()` 中读取 workspace 的 `AGENTS.md`
3. 截断到 2000 chars 注入 Briefing

**效果**: Agent 立即具备项目架构意识。

### Phase 2: Reflection 数据组装

**改动量**: ~300 行 | **新增代码**: `internal/application/reflection/`

1. 实现 `ReflectionDataAssembler`（查询 Analytics + Usage + rework_history）
2. 渲染为 Markdown 格式的分析报告
3. 实现为特殊的 `CtxReflectionData` ContextRefType（Briefing 注入）

**效果**: 为 Agent 自分析提供数据基础。

### Phase 3: Reflection Flow

**改动量**: ~400 行（因复用 Cron，比原估减少 ~100 行）| **新增**: 1 个 builtin executor

1. 新增 `reflector` 角色和 Profile（config.toml 配置即可）
2. 创建 Reflection Issue 模板（4 步：Analyze → Generate → Validate → Deploy）
3. 标记为 Cron 模板（`cron_template=true`, `cron_schedule="0 2 * * 0"`）← **零新增触发代码**
4. Deploy 步骤作为新的 `builtin: "deploy_skill"` 执行器
5. `ReflectionDataAssembler` 挂载到 BriefingBuilder，识别 Reflection Issue 时自动注入

**效果**: 完整的自进化闭环。所有调度、克隆、并发控制直接由 Cron Trigger 提供。

### Phase 4: 效果追踪、事件驱动触发、跨 Profile 传播

**改动量**: ~300 行

1. SkillDeployment 记录 + 基线快照
2. 效果对比逻辑（下次 Reflection 时计算）
3. 自动回滚机制
4. 跨 Profile 传播（同 Role 自动继承 proven Skills）
5. （可选）事件驱动触发：失败率超阈值时提前触发 Reflection

**效果**: 确保自进化是正向的，不是"越改越差"。

---

## 九、关键设计决策

### Q1: 谁来做反思分析？

**方案 A**: 目标 Profile 自己分析自己
- 优点：自知之明，了解自己的上下文
- 缺点：可能有盲点，分析和执行用同一个模型

**方案 B**: 专门的 `reflector` Profile（推荐）
- 优点：职责分离，可以用更强的模型做分析
- 缺点：需要额外配置
- 实现：`reflector` 只有 `read_context` + `submit` 权限，不能写文件

**选择 B**：与"引擎管约束，Agent 管执行"一致——反思是一种"管理活动"，不应该由执行者自己做。

### Q2: Skill 粒度？

**方案 A**: 一个大 Skill 包含所有经验
- 缺点：难以追踪哪条规则有效

**方案 B**: 每个模式一个 Skill（推荐）
- 优点：可以独立追踪效果、独立回滚
- 缺点：数量可能膨胀

**选择 B + 上限**：每个 Profile 最多 10 个 Reflected Skills。超出时，按效果排序淘汰最差的。

### Q3: 验证环节是否需要人工？

**分级**：
- **低风险**（version bump，微调现有 Skill）→ 自动审批
- **高风险**（全新 Skill，或删除规则）→ 人工审批
- 配置 `reflection.auto_approve = "minor"` / `"all"` / `"none"`

### Q4: 如何防止 Skill 退化？

- 效果追踪 + 自动回滚（Section 5.2）
- Reflected Skill 的 version 单调递增，可随时回退
- 每条规则标注证据来源（execution ID），可追溯

---

## 十、与 Harness Engineering 的关系

这个设计本质上是 **Harness Engineering 的自进化层**：

| HE 概念 | 静态 Harness（现有） | 自进化 Harness（本方案） |
|---------|---------------------|------------------------|
| 架构约束 | capabilities, action 白名单 | Reflected Skills 中的约束规则 |
| 上下文工程 | Briefing + ContextRefs | AGENTS.md + Reflected Skills 注入 |
| 反馈回路 | Gate reject → rework | Reflection → Skill update → 效果追踪 |
| 熵管理 | （缺失） | Skill 淘汰 + 回滚 = 知识层面的 GC |

OpenAI 说"当 Agent 遇到困难时，我们将其视为信号"。本方案把这个信号从人工处理自动化为系统行为——**Agent 的困难自动转化为 Agent 的技能**。

---

## 十一、开放问题

1. **冷启动**：新 Profile 没有执行历史，Reflection 无数据可分析。需要初始的 Manual Skills 或从同 Role 的其他 Profile 继承。

2. **跨项目泛化**：Profile A 在项目 X 学到的经验，是否适用于项目 Y？需要区分"项目特定"和"通用"的 Skill。

3. **Skill 冲突**：项目 X 要求"用 logrus"，项目 Y 要求"用 slog"。Reflected Skill 如何处理？→ Project Skills 优先级最高，覆盖 Reflected Skills。

4. **评估指标偏差**：成功率不等于质量。Agent 可能学会"通过 Gate 的技巧"而非"写好代码"。需要多维度指标（成功率 + 重试次数 + token 效率 + 人工反馈）。

5. **Reflection 的 token 成本**：每次 Reflection 本身消耗 token。需要平衡反思频率和成本。建议初期 threshold 设高（50 次执行 / 7 天最快一次）。
