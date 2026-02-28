# Agent 与 Runtime 插件层 — 设计文档

## 概述

Agent 和 Runtime 是两个独立的插件槽位。Agent 插件封装 AI 工具的调用方式（prompt 怎么传、输出怎么解析），Runtime 插件封装执行环境（直接子进程、tmux 会话、Docker 容器）。两者解耦，任意组合。

## 一、Agent 插件接口

### AgentPlugin 接口

```go
type AgentPlugin interface {
    Plugin
    // 根据执行选项构建 CLI 启动命令
    BuildCommand(opts ExecOpts) ([]string, error)
    // 为指定 Agent 的 stdout 创建流式解析器
    NewStreamParser(r io.Reader) StreamParser
    // 检查 Agent 活跃状态（基于 session 文件等信号）
    DetectActivity(sessionDir string) (ActivityStatus, error)
}

type StreamParser interface {
    // 逐个返回解析出的事件，EOF 时返回 nil, io.EOF
    Next() (*StreamEvent, error)
}

type ActivityStatus struct {
    State       string    // "generating" | "tool_running" | "idle" | "finished" | "error"
    LastActive  time.Time
    CurrentTool string    // 当前正在执行的工具名（如有）
    TokensUsed  int       // 已消耗 token（如可获取）
}
```

## 二、Runtime 插件接口

Runtime 决定 Agent 在哪里运行。P0 阶段只需 process（直接子进程），后续可扩展。

```go
type RuntimePlugin interface {
    Plugin
    // 创建一个执行环境，返回 session handle
    Create(ctx context.Context, opts RuntimeOpts) (*Session, error)
    // 向执行中的 session 发送消息（用于人工 inject）
    Send(sessionID string, message string) error
    // 获取 session 状态
    Status(sessionID string) (SessionStatus, error)
    // 终止 session
    Kill(sessionID string) error
    // 恢复 session（崩溃恢复）
    Restore(sessionID string) (*Session, error)
}

type RuntimeOpts struct {
    WorkDir  string            // 工作目录
    Env      map[string]string // 环境变量
    Command  []string          // 启动命令（由 Agent 插件构造）
}

type Session struct {
    ID     string
    Stdin  io.WriteCloser
    Stdout io.Reader
    Stderr io.Reader
    Wait   func() error // 阻塞直到进程退出
}

type SessionStatus struct {
    ID       string
    State    string  // "running" | "exited" | "crashed"
    ExitCode int
    PID      int
}
```

### Runtime 实现

| 实现 | 场景 | 特点 |
|---|---|---|
| `runtime-process` | P0 默认 | 直接 `os/exec`，最简单，进程退出即结束 |
| `runtime-tmux` | 需要持久会话 | Agent 崩溃可恢复，支持 `ao send` 注入，人工可 attach 查看 |
| `runtime-docker` | 强隔离需求 | 容器级隔离，mount worktree 目录，解决之前讨论的沙盒问题 |

### Agent × Runtime 的协作

```
Pipeline Engine
  → cmd := Agent.BuildCommand(opts)                  // Agent 构建启动命令
  → session := Runtime.Create(ctx, RuntimeOpts{Command: cmd})  // Runtime 启动进程
  → parser := Agent.NewStreamParser(session.Stdout)   // Agent 创建解析器
  → for { event, err := parser.Next() }              // 逐事件消费
  → Runtime.Status() 监控进程状态
  → Activity Detection 补充判断 Agent 是否卡住
```

这种分离意味着：同一个 Claude Code Agent 插件可以跑在 process 里（本地开发），也可以跑在 Docker 里（团队部署），代码不需要改。

### ExecOpts 字段说明

| 字段 | 类型 | 用途 |
|---|---|---|
| Prompt | string | 发给 Agent 的指令 |
| WorkDir | string | 子进程工作目录（项目路径或 worktree 路径） |
| AllowedTools | []string | 限制 Agent 可用的工具（仅 Claude 支持） |
| MaxTurns | int | 最大交互轮次（仅 Claude 支持） |
| Timeout | time.Duration | 执行超时 |
| Env | map[string]string | 额外环境变量 |
| AppendContext | string | 追加到 prompt 前的上下文（重试/反馈信息） |

### Result 字段

| 字段 | 类型 | 用途 |
|---|---|---|
| Output | string | Agent 的完整文本输出 |
| Structured | json.RawMessage | 如果输出是 JSON，保存解析后的原始 JSON |
| ExitCode | int | 子进程退出码 |
| Duration | time.Duration | 执行耗时 |
| TokensUsed | int | Token 消耗（如果 Agent 输出中包含） |

### StreamEvent 字段

| 字段 | 类型 | 说明 |
|---|---|---|
| Type | string | `text` / `tool_call` / `tool_result` / `error` / `done` |
| Content | string | 文本内容 |
| ToolName | string | 调用的工具名（如 Bash、Read） |
| ToolInput | string | 工具输入参数 |
| Timestamp | time.Time | 事件时间 |

## 三、Claude Code Driver

### 调用方式

使用 `claude -p` 非交互模式，核心参数：

```
claude -p "{prompt}"
  --output-format stream-json    # 流式 NDJSON 输出
  --max-turns {n}                # 限制交互轮次
  --allowedTools "{tools}"       # 限制可用工具
  --model {model}                # 可选，覆盖默认模型
```

### 输出解析

`stream-json` 格式为逐行 JSON（NDJSON），每行一个事件对象。需要关注的字段：

- `type: "assistant"` + `content[].type: "text"` → 文本输出
- `type: "assistant"` + `content[].type: "tool_use"` → 工具调用
- `type: "result"` → 最终结果，包含 `result` 字段

解析规则：
- 逐行读取 stdout，每行尝试 JSON 解析
- 解析失败的行跳过（可能是非 JSON 的 stderr 混入）
- 文本事件实时广播到 EventBus
- 工具调用事件记录但不广播详情（避免信息过载）
- `result` 事件提取最终输出

> **注意**：以上 stream-json 事件 schema 是简化描述。实际 Claude CLI 的 stream-json 输出包含更多事件类型和嵌套结构（如 `system`、`progress` 等）。实现时需要对照当前安装的 Claude CLI 版本验证实际输出格式，建议先用 `claude -p "hello" --output-format stream-json 2>/dev/null` 采样真实输出。

### AllowedTools 配置规则

不同阶段需要不同的权限：

| 阶段 | AllowedTools | 原因 |
|---|---|---|
| requirements | Read(*) | 需要读取项目结构和现有代码来理解上下文 |
| spec_gen | Bash(ls *), Bash(openspec *), Read(*), Write(*) | 需要读代码、调用 openspec CLI、写 spec 文件 |
| spec_review | Read(*), Edit(*) | 读取 + 修改 spec |
| code_review | Read(*), Bash(git diff *), Bash(git log *), Bash(git show *) | 只读 + git 信息 |
| implement | Bash(npm *), Bash(go *), Bash(make *), Bash(git *), Bash(cat *), Bash(ls *), Bash(mkdir *), Bash(cp *), Bash(cd *), Bash(find *), Read(*), Write(*), Edit(*) | 构建/测试/git 操作，按项目语言栈配置 |
| fixup | 同 implement | 同 implement |

安全规则：
- **永远不给 `Bash(*)`** — 这等于无限制 shell 访问，违背安全原则
- 每个阶段只开放该阶段需要的命令前缀
- implement/fixup 阶段的 Bash 权限按项目语言栈配置（Go 项目给 `Bash(go *)`, Node 项目给 `Bash(npm *)` 等）
- 项目配置中可覆盖默认 AllowedTools，适应不同技术栈
- 显式禁止的危险模式：`Bash(rm -rf *)`, `Bash(chmod *)`, `Bash(sudo *)`, `Bash(curl * | sh)` — 即使给了前缀也不匹配
- Review 阶段只给读权限，防止 AI 直接改代码
- AllowedTools 支持通配符 `*` 做前缀匹配，注意空格：`Bash(git diff *)` 匹配 `git diff --staged` 但不匹配 `git diffstat`

项目级 AllowedTools 覆盖示例：

```yaml
# .ai-workflow/config.yaml
agents:
  claude:
    stage_tools:
      implement:
        - "Bash(npm *)"
        - "Bash(npx *)"
        - "Bash(node *)"
        - "Bash(git *)"
        - "Read(*)"
        - "Write(*)"
        - "Edit(*)"
```

### Claude 特有注意事项

- `-p` 模式下 slash command（如 /commit）不可用，需要用自然语言描述任务
- 长任务建议设置 `--max-turns 20-50`，防止无限循环
- 如果输出中包含 "I need permission to use" 字样，说明 AllowedTools 配置不足
- 会话不跨调用保持，每次 `-p` 都是全新上下文

## 四、Codex CLI Driver

### 调用方式

使用 `codex exec` 非交互模式：

```
codex exec "{prompt}"
  --sandbox workspace-write        # 沙箱权限（允许写工作区文件）
  --json                           # JSON 输出（可选）
  -m gpt-5.3-codex                 # 模型
  -c model_reasoning_effort=high   # 推理强度
  -a never                         # 自动批准所有操作（等价于 --ask-for-approval never）
```

### 与 Claude Driver 的关键差异

| 维度 | Claude Code | Codex CLI |
|---|---|---|
| 非交互标志 | `-p` | `exec` 子命令 |
| 输出格式 | `--output-format stream-json` | `--json` 或纯文本 stdout |
| 权限控制 | `--allowedTools` 细粒度 | `--sandbox` 三档（read-only / workspace-write / full-auto） |
| 模型指定 | `--model` | `-m model_name` |
| 进程行为 | 执行完自动退出 | 执行完自动退出 |

### Sandbox 选择规则

| 阶段 | Sandbox | 原因 |
|---|---|---|
| implement | workspace-write | 需要写代码和运行测试 |
| fixup | workspace-write | 需要修改文件 |
| code_review | read-only | 只看不改 |

### Codex --json 事件映射

Codex `--json` 输出的事件类型到统一 StreamEvent 的映射：

| Codex 事件 | StreamEvent.Type | 说明 |
|---|---|---|
| `message` | `text` | Agent 文本输出 |
| `function_call` | `tool_call` | 工具调用 |
| `function_call_output` | `tool_result` | 工具执行结果 |
| `error` | `error` | 错误信息 |
| EOF / 进程退出 | `done` | 执行完毕 |

> **注意**：以上映射基于 Codex CLI 当前版本推断，实现时需用 `codex exec "hello" --json` 验证实际输出格式。

### Codex 特有注意事项

- `codex exec` 结果在 stdout，直接捕获即可
- `-a never`（或 `--ask-for-approval never`）在自动化场景必须设置，否则会卡住等用户输入
- `--full-auto` — 等价于 `--sandbox full-auto -a never`，最大自由度
- `--ephemeral` — 不保存对话历史
- `-C <DIR>` — 指定工作目录
- Codex 有自己的 AGENTS.md 和 prompts 目录机制，如果项目中有这些文件会自动加载
- OpenSpec 初始化时如果选了 codex 工具，会在 `~/.codex/prompts/` 生成 prompt 文件，Codex 会自动识别

## 五、OpenSpec Driver

### 设计决策

OpenSpec CLI 本身只是一个脚手架工具（init、update、instructions 等），真正的 spec 生成需要 AI 参与（/opsx:ff 命令在 AI 对话中执行）。因此：

**OpenSpec Driver 不直接调用 openspec CLI，而是通过 Claude Driver 间接驱动。**

### 执行流程

```
OpenSpecDriver.GenerateSpec(changeName)
  │
  ├── 1. 调用 openspec instructions --change {name} --json
  │      获取当前阶段应该生成什么 artifact
  │
  ├── 2. 构造 prompt，包含：
  │      - OpenSpec 的 AGENTS.md 内容（如果存在）
  │      - 当前 change 目录的已有文件
  │      - 用户的需求描述
  │      - 明确指令：生成 proposal → specs → design → tasks
  │
  └── 3. 通过 Claude Driver 执行
         claude -p "{构造好的 prompt}" --allowedTools "Bash(openspec *),Bash(ls *),Read(*),Write(*)"
```

### 可直接调用 openspec CLI 的场景

以下操作不需要 AI，可以直接调用：

| 命令 | 场景 |
|---|---|
| `openspec instructions --change {name} --json` | 获取下一步指引 |
| `openspec templates --json` | 获取模板路径 |
| `openspec init --tools claude,codex --force` | 初始化新项目 |

### Spec 审核的 Prompt 构造规则

审核阶段让 Claude 检查以下文件，prompt 应包含明确的检查清单：

```
审核 openspec/changes/{name}/ 下的所有文件：
1. proposal.md — 需求描述是否清晰、边界是否明确
2. specs/ — 技术规格是否覆盖所有需求点、是否有遗漏
3. design.md — 技术方案是否合理、是否考虑了边界情况
4. tasks.md — 任务拆分是否合理、颗粒度是否适中、顺序是否正确

检查项：
- 各文件之间是否一致（proposal 说的和 tasks 做的是否对应）
- 是否有遗漏的需求点
- 技术方案是否可行
- 任务是否可以被 AI 独立执行（描述是否足够清晰）

如有问题直接修复文件。最后输出 JSON：
{"status": "approved" | "fixed", "issues_found": [...], "fixes_applied": [...]}
```

## 六、流式输出解析器

### 统一解析策略

Claude 和 Codex 的流式输出格式不同，抽象一个 StreamParser：

```
StreamParser
  ├── ClaudeStreamParser   # 解析 stream-json NDJSON
  ├── CodexStreamParser    # 解析 codex --json 或纯文本
  └── PlainTextParser      # 兜底：按行读取
```

规则：
- Parser 从 io.Reader (stdout pipe) 逐行读取
- 每行尝试 JSON 解析，失败则作为纯文本处理
- 解析成功的事件转换为统一的 StreamEvent
- 所有事件带时间戳
- EOF 时发送 `done` 事件

### 输出缓存

- 所有流式输出同时写入一个 Buffer，用于最终 Result.Output
- Buffer 有大小上限（默认 10MB），超出后保留头尾各 5MB
- 流式事件和最终 Buffer 内容都持久化到 Store 的 log 表

## 七、Activity Detection（活跃检测）

### 为什么需要

单纯依赖 stdout 流式输出判断 Agent 状态不够可靠：Agent 可能在思考（无输出但在工作）、卡死（无输出也没在工作）、或者在等待工具执行（长时间无文本输出但有活动）。需要一个独立的检测机制补充判断。

借鉴来源：agent-orchestrator 的 `agent-claude-code` 插件直接读取 Claude Code 的 JSONL session 文件判断活跃状态，比靠 stdout 可靠得多。

### Claude Code 的 Session 文件

Claude Code 在 `~/.claude/projects/{project-hash}/` 下写入 JSONL 格式的 session 文件，每行一个事件。Agent 插件可以直接 tail 这些文件获取精确状态：

```
~/.claude/projects/{hash}/
  └── sessions/
      └── {session-id}.jsonl   ← 逐行追加的事件日志
```

> **实现细节**：Session 文件路径格式（`{project-hash}` 的计算方式、`sessions/` 子目录结构）是 Claude CLI 的内部实现，可能随版本变化。实现时应提供路径发现的 fallback 机制：优先按已知格式查找，找不到则退化到纯 stdout 检测。

每行 JSON 包含 `type` 字段，关键类型：

| type | 含义 | 映射到 ActivityStatus |
|---|---|---|
| `assistant` | AI 正在生成文本 | `generating` |
| `tool_use` | AI 调用了工具 | `tool_running` |
| `tool_result` | 工具执行完毕返回结果 | `generating`（等 AI 处理） |
| `result` | 本轮执行完毕 | `finished` |

### Codex 的 Session 文件

Codex 将 session 数据存储在 `~/.codex/sessions/` 下，格式类似。Agent 插件同样可以 tail 读取。

### 检测逻辑

```go
func (a *ClaudeAgent) DetectActivity(sessionDir string) (ActivityStatus, error) {
    // 1. 找到最新的 session JSONL 文件
    // 2. 读取最后 N 行
    // 3. 根据最后一条事件的 type 判断状态
    // 4. 结合时间戳判断是否卡住（最后事件超过 idle 阈值 → idle）
    //    idle 阈值可配置，默认 5min，hotfix 模板降为 2min
}
```

### 与 Pipeline Engine 的集成

Activity Detection 不是实时流的一部分，而是一个定时轮询信号：

- Executor 每 30 秒调用一次 `Agent.DetectActivity()`
- 如果连续 3 次返回 `idle` 且 stdout 无新输出 → 触发 Reactions 的 `pipeline_stuck` 事件
- 如果返回 `error` → 触发 `stage_failed` 事件
- 如果返回 `finished` 但进程未退出 → 可能卡在清理阶段，等待 60 秒后强制 kill

### Fallback

如果 session 文件不存在或不可读（比如新版 CLI 改了路径），退化到纯 stdout 检测：
- 有输出 → `generating`
- 无输出超过 idle 阈值（默认 5min）→ `idle`
- 进程退出 → `finished`

## 八、并发调度

### 资源模型

系统中的稀缺资源：
- **Claude API 额度** — 有速率限制
- **Codex API 额度** — 有速率限制
- **子进程数** — 每个 Agent 调用占一个进程
- **磁盘 I/O** — 多个 worktree 同时操作

### 调度策略

两级信号量控制：

```
全局 Agent 并发上限（默认 3）
  └── 每个项目的 Pipeline 并发上限（默认 2）
```

规则：
- 获取信号量时按 FIFO 排队
- 信号量在 Stage 开始前获取，Stage 结束后释放（不是 Pipeline 级别持有）
- 这意味着多个 Pipeline 可以交替执行各自的 Stage
- 如果一个 Pipeline 处于 waiting_human 状态，不占用信号量

### Agent 池化

同一类型的 Agent 不需要池化——每次调用都是启动一个新的子进程。但需要注意：
- Claude Code 的 `~/.claude.json` 是进程间共享的，并发读取无问题
- Codex 的 `~/.codex/config.toml` 同理
- 两个 Agent 同时操作同一个 worktree 会导致文件冲突，必须避免

**规则：同一个 worktree 同一时刻只能有一个 Agent 操作。** 调度器需要维护 worktree → Agent 的排他锁。

## 九、错误处理

### 错误分类

| 类型 | 示例 | 处理方式 |
|---|---|---|
| 进程启动失败 | CLI 二进制不存在 | 立即失败，不重试 |
| 超时 | Agent 执行超过 timeout | 杀进程，按 on_failure 处理 |
| 非零退出码 | Agent 内部报错 | 提取 stderr，按 on_failure 处理 |
| 输出解析失败 | JSON 格式异常 | 降级为纯文本输出，不影响流程 |
| API 限流 | 429 Too Many Requests | 指数退避重试（5s → 15s → 45s），最多 3 次；优先解析响应中的 `Retry-After` header |
| 上下文溢出 | prompt + 代码超过 context window | 提示用户拆分任务，不自动重试 |

### 错误信息传递

失败时的 error 信息需要结构化，以便注入到重试的 prompt 中：

```go
type AgentError struct {
    Agent    string    // "claude" / "codex"
    Stage    string    // 在哪个阶段失败
    ExitCode int
    Stderr   string    // 截取最后 2000 字符
    Duration time.Duration
}
```

### stderr 捕获

Agent Driver 需要同时捕获 stdout 和 stderr：

- stdout：流式解析为 StreamEvent，用于实时输出和最终 Result.Output
- stderr：单独写入一个 bytes.Buffer，不做流式解析
- stderr Buffer 上限 1MB，超出后只保留最后 1MB
- 正常退出时：stderr 内容记录到日志但不影响流程（可能包含 deprecation warning 等无害信息）
- 异常退出时：stderr 内容写入 AgentError.Stderr，注入到重试 prompt 和 Checkpoint.error_message
- stdout 和 stderr 分别通过 `cmd.StdoutPipe()` 和 `cmd.StderrPipe()` 获取，用独立 goroutine 读取避免死锁

### Windows 注意事项

- 进程树 kill：Windows 上 `cmd.Process.Kill()` 只杀主进程，不杀子进程树。需要使用 `taskkill /T /F /PID {pid}` 或在 Go 侧设置 `cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}` 以便通过 `GenerateConsoleCtrlEvent` 发送 Ctrl+C。
- stdout/stderr pipe 在 Windows 上使用命名管道，行为与 Unix 的 fd 一致，无需特殊处理。
- Session 文件路径使用 `filepath.Join()` 确保跨平台兼容。

## 十、Prompt 模板管理

### 模板存储

每个 Stage 对应一个 prompt 模板文件：

```
configs/prompts/
  ├── requirements.tmpl
  ├── spec_gen.tmpl
  ├── spec_review.tmpl
  ├── implement.tmpl
  ├── code_review.tmpl
  └── fixup.tmpl
```

### 模板变量

模板使用 Go text/template 语法，可用变量：

```
{{.ProjectName}}      — 项目名
{{.ChangeName}}       — 变更名
{{.RepoPath}}         — 仓库路径
{{.WorktreePath}}     — Worktree 路径
{{.Requirements}}     — 需求描述
{{.SpecPath}}         — Spec 文件目录
{{.TasksMD}}          — tasks.md 内容
{{.PreviousReview}}   — 上次 review 结果
{{.HumanFeedback}}    — 人工反馈
{{.RetryError}}       — 上次失败的错误信息
{{.RetryCount}}       — 当前重试次数
```

### 模板覆盖优先级

```
项目目录 .ai-workflow/prompts/{stage}.tmpl
  > 用户目录 ~/.ai-workflow/prompts/{stage}.tmpl
    > 内置默认模板
```
