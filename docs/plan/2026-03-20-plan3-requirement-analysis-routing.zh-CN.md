# Plan 3：需求分析与智能路由

> 状态：草案
>
> 创建日期：2026-03-20
>
> 依赖：Plan 1（Initiative 实体）、Plan 2（Thread Proposal 流程）
>
> 被依赖：无

## 1. 目标

让用户只需提交一句需求描述，AI 即可自动完成：

1. 分析需求涉及哪些项目（单项目 / 跨项目）
2. 创建 Thread 并关联相关项目上下文
3. 邀请合适的 Agent 参与讨论
4. 启动讨论流程，最终收敛为 Proposal → Initiative

当前现状：

- Project 有 `Description` 和 `Metadata` 字段，但内容通常不够丰富
- Thread 创建时可以手动关联 ThreadContextRef，但没有自动匹配逻辑
- Agent 邀请是手动的，没有基于能力自动匹配
- 没有"需求入口"的概念——用户必须自己判断需求属于哪个项目

## 2. 核心判断

1. **需求分析是 Agent 逻辑，不是硬编码规则**。项目匹配应该由 LLM 基于项目描述来判断，而不是关键词匹配。
2. **项目元数据是基础**。AI 路由的质量取决于项目描述的质量，因此必须先增强项目元数据。
3. **自动化程度分级**。第一阶段做"AI 建议 + 人类确认"，不做全自动。
4. **复用已有 Thread 基础设施**。不引入新的讨论容器，只是自动化 Thread 的创建和配置。

## 3. 项目元数据增强

### 3.1 Project 模型扩展

当前 Project 只有 `Name`, `Kind`, `Description`, `Metadata`。需要在 Metadata 中约定标准化字段：

```go
// 建议的 Metadata 标准字段（约定，非强制）
// 存储在 Project.Metadata map[string]string 中

const (
    ProjectMetaScope       = "scope"        // 项目职责范围描述
    ProjectMetaTechStack   = "tech_stack"   // 技术栈，逗号分隔
    ProjectMetaKeywords    = "keywords"     // 关键词，逗号分隔
    ProjectMetaOwner       = "owner"        // 项目负责人
    ProjectMetaDependsOn   = "depends_on"   // 依赖的其他项目名，逗号分隔
    ProjectMetaAgentHints  = "agent_hints"  // 推荐的 Agent profile ID，逗号分隔
)
```

### 3.2 不修改 Project 结构体

Project.Metadata 已经是 `map[string]string`，足够承载这些信息。只需：

- 在 UI 上提供引导式填写
- 在 Agent prompt 中说明这些字段的含义

### 3.3 前端项目设置增强

在项目编辑页面增加结构化字段填写：

- 项目职责范围（textarea）
- 技术栈（tag input）
- 关键词（tag input）
- 负责人
- 关联的 Agent Profiles（多选）

这些字段存入 `Project.Metadata`，不修改数据库 schema。

## 4. 需求入口

### 4.1 新增 RequirementInput API

```
POST /api/requirements/analyze
```

Request:
```json
{
    "description": "用户提交的需求描述",
    "context": "可选的补充上下文"
}
```

Response:
```json
{
    "analysis": {
        "summary": "需求摘要",
        "type": "single_project | cross_project | new_project",
        "matched_projects": [
            {
                "project_id": 1,
                "project_name": "backend-api",
                "relevance": "high",
                "reason": "需求涉及 API 接口变更",
                "suggested_scope": "新增 /api/xxx 端点"
            }
        ],
        "suggested_agents": [
            {
                "profile_id": "arch-reviewer",
                "reason": "涉及架构变更，需要架构评审"
            }
        ],
        "complexity": "low | medium | high",
        "suggested_meeting_mode": "direct | concurrent | group_chat",
        "risks": ["可能影响现有 API 兼容性"]
    },
    "suggested_thread": {
        "title": "讨论：xxx 需求",
        "context_refs": [
            {"project_id": 1, "access": "read"},
            {"project_id": 2, "access": "read"}
        ],
        "agents": ["arch-reviewer", "backend-dev"],
        "meeting_mode": "group_chat",
        "meeting_max_rounds": 6
    }
}
```

### 4.2 确认并创建 Thread

```
POST /api/requirements/create-thread
```

Request:
```json
{
    "description": "原始需求描述",
    "analysis_id": "可选，引用之前的分析结果",
    "thread_config": {
        "title": "讨论：xxx 需求",
        "context_refs": [
            {"project_id": 1, "access": "read"},
            {"project_id": 2, "access": "read"}
        ],
        "agents": ["arch-reviewer", "backend-dev"],
        "meeting_mode": "group_chat",
        "meeting_max_rounds": 6
    }
}
```

这个接口会：
1. 创建 Thread
2. 添加 ThreadContextRef（关联项目）
3. 邀请 Agent
4. 发送初始消息（包含需求描述 + 分析结果）
5. 启动讨论

### 4.3 两步流程 vs 一步流程

- **两步流程**（推荐）：先 analyze → 用户确认/修改 → 再 create-thread
- **一步流程**（快捷）：直接 create-thread，跳过人工确认

第一阶段实现两步流程，保证用户有确认机会。

## 5. 需求分析 Agent

### 5.1 分析逻辑

不新建独立 Agent 进程，而是使用现有 LLM 调用能力。分析步骤：

1. 获取所有 Project 列表（含 Description + Metadata）
2. 获取所有 AgentProfile 列表（含 role、capabilities）
3. 构建 prompt，让 LLM 分析：
   - 需求涉及哪些项目
   - 需求复杂度
   - 推荐的讨论模式
   - 推荐参与的 Agent
4. 解析 LLM 返回的结构化结果

### 5.2 Prompt 模板

```
你是一个需求分析助手。以下是系统中的项目列表：

{{range .Projects}}
## {{.Name}} ({{.Kind}})
- 描述：{{.Description}}
- 职责范围：{{.Metadata.scope}}
- 技术栈：{{.Metadata.tech_stack}}
- 关键词：{{.Metadata.keywords}}
{{end}}

以下是可用的 Agent：

{{range .Profiles}}
## {{.ID}} ({{.Role}})
- 能力：{{.Capabilities}}
{{end}}

用户提交了以下需求：

"""
{{.Description}}
"""

请分析：
1. 这个需求涉及哪些项目？为什么？
2. 这是单项目需求还是跨项目需求？
3. 推荐哪些 Agent 参与讨论？
4. 需求复杂度如何？建议使用什么讨论模式？
5. 有什么风险需要注意？

请以 JSON 格式返回分析结果。
```

### 5.3 应用层

```go
// internal/application/requirementapp/service.go

type Service struct {
    projectStore core.ProjectStore    // 项目读取，不新增接口
    profileStore AgentProfileStore    // Agent 配置读取
    llm          LLMClient            // LLM 调用
    threadApp    *threadapp.Service   // 复用已有 Thread 服务
}

func (s *Service) Analyze(ctx context.Context, input AnalyzeInput) (*AnalysisResult, error)
func (s *Service) CreateThread(ctx context.Context, input CreateThreadInput) (*Thread, error)
```

## 6. 前端

### 6.1 新增需求提交页面

**RequirementPage** (`/requirements/new`)：

1. **需求输入区**
   - 大文本框输入需求描述
   - 可选：补充上下文
   - "分析"按钮

2. **分析结果展示区**（分析完成后显示）
   - 需求摘要
   - 匹配的项目列表（可勾选/取消）
   - 推荐的 Agent（可勾选/取消）
   - 讨论模式建议（可修改）
   - 风险提示

3. **确认创建区**
   - "创建讨论"按钮 → 跳转到 ThreadDetailPage
   - 用户可以在确认前修改任何建议

### 6.2 现有页面入口

- **DashboardPage**：新增"提交需求"快捷入口
- **ThreadsPage**：新增"从需求创建"按钮（区别于直接创建 Thread）

## 7. 完整用户旅程

```
用户在 Dashboard 点击"提交需求"
    ↓
输入需求描述："我需要给用户系统加一个两步验证功能"
    ↓
点击"分析"
    ↓
AI 返回分析结果：
  - 涉及项目：backend-api（认证模块）、frontend-web（登录页面）、infra（短信网关）
  - 类型：跨项目需求
  - 推荐 Agent：arch-reviewer（架构评审）、backend-dev、frontend-dev
  - 讨论模式：group_chat（多方协作）
  - 风险：需要第三方短信服务集成
    ↓
用户确认（或调整匹配结果）
    ↓
系统自动创建 Thread：
  - 关联 3 个项目上下文（read access）
  - 邀请 3 个 Agent
  - 设置 group_chat 模式
  - 发送初始消息（需求描述 + 分析背景）
    ↓
跳转到 ThreadDetailPage
    ↓
多 Agent group_chat 讨论
    ↓
Lead Agent 发起 Proposal（Plan 2）
  - 拆分为 3 个 WorkItem（分属 3 个项目）
  - 标注依赖关系（前端依赖后端 API、后端依赖 infra 短信网关）
    ↓
用户审批 Proposal
    ↓
自动生成 Initiative + 3 个 WorkItem（Plan 1）
    ↓
用户审批 Initiative → 开始执行
    ↓
调度器按依赖顺序执行：infra → backend → frontend
```

## 8. 实施阶段

### Phase 1：项目元数据增强

- 定义 Metadata 标准字段常量
- 前端项目设置页面增加结构化填写
- 不涉及后端 schema 变更

### Phase 2：需求分析 API

- `internal/application/requirementapp/service.go`
- LLM prompt 模板
- `/api/requirements/analyze` 端点
- `/api/requirements/create-thread` 端点

### Phase 3：前端需求提交页面

- RequirementPage 页面
- 分析结果展示与编辑
- Dashboard / ThreadsPage 入口

### Phase 4：智能推荐优化

- 基于历史数据优化匹配准确度
- 常见需求模式识别
- Agent 能力匹配优化

## 9. 明确不做的内容

1. 全自动需求处理（始终需要人类确认分析结果）
2. 需求优先级自动排序
3. 需求去重/合并
4. 自然语言需求解析为结构化 spec
5. 需求变更追踪
6. 基于历史数据的 ML 模型训练
