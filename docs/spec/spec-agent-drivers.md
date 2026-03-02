# Agent 通信层（ACP 统一版）— 设计文档

## 概述

AI Workflow 的所有 Agent 交互统一使用 ACP（JSON-RPC 2.0 over stdio）。

本系统只维护两类配置对象：

- `Agent Profile`：定义如何启动 Agent 进程（命令、参数、环境变量、能力上限）
- `Role Profile`：定义业务角色如何使用 Agent（prompt、会话策略、权限策略、MCP 工具）

业务层（Pipeline、Secretary、Review Orchestrator、Plan Parser）只绑定 `role_id`，不直接绑定具体 Agent 二进制。

## 一、核心对象

### 1.1 Agent Profile

```go
type AgentProfile struct {
    ID              string
    LaunchCommand   string
    LaunchArgs      []string
    Env             map[string]string
    CapabilitiesMax ClientCapabilities
}
```

### 1.2 Role Profile

```go
type RoleProfile struct {
    ID               string
    AgentID          string
    PromptTemplate   string
    SessionPolicy    SessionPolicy
    Capabilities     ClientCapabilities
    PermissionPolicy []PermissionRule
    MCPTools         []string
}
```

### 1.3 Role Binding

```go
type RoleBindings struct {
    Secretary struct {
        Role string
    }
    Pipeline struct {
        StageRoles map[string]string // stage -> role_id
    }
    ReviewOrchestrator struct {
        Reviewers  map[string]string // completeness/dependency/feasibility -> role_id
        Aggregator string            // role_id
    }
    PlanParser struct {
        Role string
    }
}
```

### 1.4 约束

- `role.capabilities` 必须是 `agent.capabilities_max` 的子集
- Session 池 key 必须包含 `role_id`，禁止不同角色共享同一 session
- 任意角色复用失败时，统一回退链路：`LoadSession -> NewSession`
- 角色默认可预置：`secretary`、`worker`、`reviewer`、`aggregator`、`plan_parser`

## 二、ACP Client 设计

建议目录：

```text
internal/acpclient/
  client.go      # Client 主结构与会话方法
  protocol.go    # ACP message/request/response/event 类型
  handler.go     # 工具回调接口（fs/permission/terminal）
  transport.go   # stdio JSON-RPC 收发与请求关联
```

核心对象：

```go
type Client struct {
    // launch config
    // json-rpc transport
    // session registry
    // request tracker
    // callback handler
}
```

## 三、Client API 契约

```go
func New(cfg LaunchConfig, h Handler, opts ...Option) (*Client, error)
func (c *Client) Initialize(ctx context.Context, caps ClientCapabilities) error
func (c *Client) NewSession(ctx context.Context, req NewSessionRequest) (SessionInfo, error)
func (c *Client) LoadSession(ctx context.Context, req LoadSessionRequest) (SessionInfo, error)
func (c *Client) Prompt(ctx context.Context, req PromptRequest) (*PromptResult, error)
func (c *Client) Cancel(ctx context.Context, req CancelRequest) error
func (c *Client) Close(ctx context.Context) error
```

语义：

- `Initialize`：会话前必调，协商能力门禁
- `NewSession`：创建会话并绑定 `cwd`、可选 `mcpServers`
- `LoadSession`：恢复已有会话（不支持或不存在应返回明确错误码）
- `Prompt`：单轮调用，流式接收 `session/update`
- `Cancel`：取消进行中的请求，幂等
- `Close`：回收 Client 下全部会话和进程资源，可重复调用

数据模型建议：

```go
type NewSessionRequest struct {
    CWD        string
    MCPServers []MCPServerConfig
    Metadata   map[string]string
}

type PromptRequest struct {
    SessionID string
    Prompt    string
    Metadata  map[string]string // 建议注入 role_id / stage / pipeline_id / plan_id
}

type PromptResult struct {
    RequestID  string
    Text       string
    Usage      TokenUsage
    StopReason string
}
```

## 四、Handler 设计

```go
type Handler interface {
    HandleReadFile(ctx context.Context, req ReadFileRequest) (ReadFileResult, error)
    HandleWriteFile(ctx context.Context, req WriteFileRequest) (WriteFileResult, error)

    HandleRequestPermission(ctx context.Context, req PermissionRequest) (PermissionDecision, error)

    HandleTerminalCreate(ctx context.Context, req TerminalCreateRequest) (TerminalCreateResult, error)
    HandleTerminalWrite(ctx context.Context, req TerminalWriteRequest) (TerminalWriteResult, error)
    HandleTerminalRead(ctx context.Context, req TerminalReadRequest) (TerminalReadResult, error)
    HandleTerminalResize(ctx context.Context, req TerminalResizeRequest) (TerminalResizeResult, error)
    HandleTerminalClose(ctx context.Context, req TerminalCloseRequest) (TerminalCloseResult, error)
}
```

回调责任：

- `HandleReadFile`：路径归一化、scope 校验、读取审计
- `HandleWriteFile`：路径归一化、scope 校验、写盘、记录变更、触发 `secretary_files_changed`
- `HandleRequestPermission`：按角色权限策略执行 allow/deny/ask 并审计
- `HandleTerminal*`：终端实例生命周期与 I/O 转发

## 五、会话策略

标准链路：

```text
AcquireSession -> Prompt -> 处理 session/update -> stopReason=end_turn -> ReleaseSession
```

建议规则：

- Pipeline 粒度建议 `1 Pipeline = 1 ACPClient`
- 可并存多会话（`worker`、`reviewer` 等），按角色分池复用
- 会话获取顺序：内存命中 -> `LoadSession` -> `NewSession`
- 复用失败（session 丢失、传输断连、权限不匹配）必须自动新建会话

默认角色策略：

| 角色 | 默认策略 | 说明 |
|---|---|---|
| `secretary` | 复用 + 优先 LoadSession | 支持多轮对话与项目重开恢复 |
| `worker` | 复用 | 减少 implement/fixup 上下文重建 |
| `reviewer` | 复用 + reset_prompt | 保持审查连续性并降低漂移 |
| `aggregator` | 复用 + reset_prompt | 保持多轮聚合上下文 |
| `plan_parser` | 一次性 | 单一转换任务，结束即回收 |

## 六、权限模型

统一采用 ACP 三层权限：

1. Capability Gate（`Initialize`）：声明并协商能力
2. Scope（`NewSession/LoadSession` 的 `cwd`）：限定文件和命令边界
3. Runtime Permission（`session/request_permission`）：高风险操作逐次授权

安全约束：

- 所有路径先规范化，再进行 `cwd` 边界校验
- 未协商成功的能力不得调用
- 权限决策必须记录审计日志

## 七、调用方规范

- Pipeline Stage：按 `role_bindings.pipeline.stage_roles` 取 `role_id` 执行 ACP Prompt
- Secretary：按 `role_bindings.secretary.role` 启动持久会话
- Review Orchestrator：reviewers/aggregator 均按 `role_bindings.review_orchestrator` 执行
- Plan Parser：按 `role_bindings.plan_parser.role` 执行一次性调用

以上调用方统一只依赖 `role_id`，由 RoleResolver 解算到 `AgentProfile + RoleProfile`。

## 八、Prompt 模板

模板内容与变量体系保持不变，仅传递方式统一为 `PromptRequest.prompt` 字段。

模板文件：

- `configs/prompts/requirements.tmpl`
- `configs/prompts/implement.tmpl`
- `configs/prompts/code_review.tmpl`
- `configs/prompts/fixup.tmpl`
- `configs/prompts/e2e_test.tmpl`
- `configs/prompts/secretary_system.tmpl`
- `configs/prompts/plan_parser.tmpl`
- `configs/prompts/review_completeness.tmpl`
- `configs/prompts/review_dependency.tmpl`
- `configs/prompts/review_feasibility.tmpl`
- `configs/prompts/review_aggregator.tmpl`

## 九、验收标准

- `internal/acpclient` 提供完整 API：`New/Initialize/NewSession/LoadSession/Prompt/Cancel/Close`
- 配置层完整支持 `agents + roles + role_bindings`
- Pipeline、Secretary、Review Orchestrator、Plan Parser 均按 `role_id` 调用 ACP
- 文件变更事实来源为 `HandleWriteFile`，不依赖 stdout 推断
- 全链路具备 `LoadSession -> NewSession` 恢复能力
