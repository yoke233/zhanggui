# 分布式部署规范

> **Status:** 设计阶段，未实现。
>
> 本文定义 ai-workflow 的分布式部署架构：TL 在本地与 Human 交互，开发团队在云端容器化运行。

---

## 1. 部署拓扑

控制面在云端，与 Worker 同进程。本地 TL 通过 WebSocket 接收事件、通过 REST API 发送命令。

```
本地（开发者机器）                        云端（Docker/K8s）
──────────────                         ─────────────────
ai-flow local-tl                       ai-flow-server 容器
├── TL Agent (Claude ACP)              ├── Web/API + WS Hub
├── Human 对话                          ├── TL Manager (standby/active)
├── WebSocket ← 接收事件                ├── DepScheduler
├── REST API → 发送命令                 ├── Executor → ACP Agents (子进程)
└── Heartbeat 保活                      ├── EventBus (进程内)
                                        ├── Store (SQLite, volume 挂载)
                                        └── OpenViking (可选 sidecar)

                                        Volumes:
                                          /data/db     → SQLite 数据库
                                          /data/repos  → bare clone 缓存 + worktrees
                                          /data/logs   → 日志
```

### 单容器策略

ACP agents 通过 `exec.Command()` 以 stdio 管道运行为服务器子进程。拆分 Worker 到独立容器需替换 stdio 传输为网络协议——改动量极大。

当前阶段：**Worker 在服务器容器内作为子进程运行**。单实例并发上限由容器资源和 `scheduler.max_global_agents` 决定。

未来扩展路径：引入任务队列（NATS/Redis），Worker 解耦为独立容器，Executor 改为网络派发。

---

## 2. 容器化

### 2.1 镜像构建

多阶段构建：Go builder → Node.js runtime（ACP agents 通过 `npx` 启动）+ git。

```dockerfile
FROM golang:1.23-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /ai-flow ./cmd/ai-flow

FROM node:22-bookworm-slim AS runtime
RUN apt-get update && apt-get install -y git ca-certificates curl && rm -rf /var/lib/apt/lists/*
# 预装 ACP agent 包，避免首次运行时下载
RUN npx -y @zed-industries/claude-agent-acp@latest --version || true
RUN npx -y @zed-industries/codex-acp@latest --version || true

COPY --from=builder /ai-flow /usr/local/bin/ai-flow
COPY configs/ /app/configs/

WORKDIR /app
VOLUME ["/data/db", "/data/repos", "/data/logs"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

EXPOSE 8080
ENTRYPOINT ["ai-flow"]
CMD ["server", "--port", "8080"]
```

### 2.2 健康检查

| 探针 | 端点 | 检查内容 |
|------|------|---------|
| Liveness | `GET /health` | HTTP 服务存活（已有） |
| Readiness | `GET /api/v3/ready`（新增） | Store 可访问 + Scheduler 已启动 + IssueManager 已启动 |

### 2.3 崩溃恢复

已有机制，容器化天然兼容：

1. **启动时** `RecoverActiveRuns()` — 找到 in_progress Run，标记中断 checkpoint 为 failed，清理 worktree，重新入队
2. **启动时** `RecoverExecutingIssues()` — 找到活跃 Issue，重新调度
3. **优雅关闭** — SIGTERM → 10s 超时依次关闭 HTTP → IssueManager → Scheduler

### 2.4 容器策略

- Docker Compose: `restart: unless-stopped`
- K8s: `restartPolicy: Always` + PVC 挂载 `/data`
- SQLite WAL 模式保证崩溃安全写入
- 数据库路径必须在 volume 挂载上（`/data/db/data.db`）

---

## 3. Cloud Workspace

Worker 在云端需要 git clone。设计 bare clone 缓存 + per-run worktree，保证 default_branch 始终干净最新。

### 3.1 存储结构

```
/data/repos/
├── cache/                              # 裸克隆缓存（每个远程 repo 一份）
│   └── github.com/
│       └── owner/
│           └── repo.git/               # git clone --bare
└── worktrees/                          # 每次 Run 独立 worktree
    ├── run-abc123/
    └── run-def456/
```

### 3.2 新插件 `workspace-cloud`

实现 `core.WorkspacePlugin` 接口。

**Setup 流程**（每次 Run 开始时）：

```
1. 解析 Project.RemoteURL → 定位 bare cache 路径
2. bare cache 不存在？
   → git clone --bare <remote_url> <cache_path>
3. bare cache 已存在？
   → git -C <cache> fetch --all --prune
4. 创建 worktree：
   git -C <cache> worktree add /data/repos/worktrees/<run-id> \
     -b ai-flow/<run-id> origin/<default_branch>
5. 返回 { BranchName, WorktreePath, BaseBranch }
```

**Cleanup 流程**：

```
git -C <cache> worktree remove /data/repos/worktrees/<run-id> --force
```

**干净保证**：
- 每次 Run 前 `git fetch --all --prune` 拉取最新
- worktree 从 `origin/<default_branch>` HEAD 新建，保证无脏状态
- Run 结束后 worktree 被删除，不累积

### 3.3 Project 模型扩展

```go
type Project struct {
    // ... 现有字段 ...
    RemoteURL string `json:"remote_url,omitempty"` // git 远程 URL（云端模式）
}
```

选择逻辑：
- `RemoteURL` 有值 + `workspace-cloud` 插件 → bare cache 流程
- `RepoPath` 有值 + `workspace-worktree` 插件 → 现有本地流程

### 3.4 定期清理

后台 goroutine 每 6 小时：
- 移除已终态 Run 的残留 worktree
- `git gc` 压缩 bare cache
- 移除 30 天未访问的 bare cache

---

## 4. TL 本地/云端混合

本地 TL 优先。本地运行时覆盖云端 TL；本地不在时云端 TL 接管。

### 4.1 所有权状态机

```
                  register(local)
  ┌──────────┐ ────────────────→ ┌──────────┐
  │ CLOUD_TL │                   │ LOCAL_TL │
  │ (active) │ ←──────────────── │ (active) │
  └──────────┘  grace 超时        └────┬─────┘
       ▲        (heartbeat 断了)       │ heartbeat (10s)
       │                               ▼
       │        显式 deregister   ┌──────────┐
       └──────────────────────── │ LOCAL_TL │
                                  └──────────┘

切换规则：
  Heartbeat 丢失 → 30s 宽限期 → 回退 CLOUD_TL
  显式 deregister → 立即回退 CLOUD_TL
  宽限期内 heartbeat 恢复 → 取消回退，继续 LOCAL_TL
```

### 4.2 TL 注册 API（新增）

| 方法 | 端点 | 说明 |
|------|------|------|
| POST | `/api/v3/tl/register` | 本地 TL 注册为主控 |
| DELETE | `/api/v3/tl/register` | 本地 TL 注销 |
| GET | `/api/v3/tl/status` | 查询当前 TL 归属（cloud/local） |
| POST | `/api/v3/tl/heartbeat` | 心跳保活 |

注册 payload：

```json
{
  "client_id": "local-tl-<uuid>",
  "capabilities": ["issue_review", "decompose", "triage"],
  "priority": 100
}
```

### 4.3 TLProxy 组件

新增 `internal/teamleader/tl_proxy.go`，包装现有 TL Manager 的事件反应逻辑。

```go
type TLProxy struct {
    mode         TLMode          // "cloud" | "local"
    localClient  *TLLocalClient  // nil when cloud-only
    cloudTL      *Manager        // always present
    gracePeriod  time.Duration   // default 30s
    heartbeatTTL time.Duration   // default 15s
}
```

行为：
- `mode=cloud`：云端 TL 处理所有决策（auto-approve、auto-dispatch）
- `mode=local`：暂停云端 TL 的自动决策，需要决策的事件推送给本地 TL 处理
- DepScheduler 始终在云端运行，不受 mode 影响

### 4.4 本地 TL 工作方式

```bash
ai-flow local-tl --server https://cloud:8080 --token xxx
```

流程：
1. WebSocket 连接 `/api/v1/ws` → 订阅所有事件
2. `POST /api/v3/tl/register` → 声明主控权，获取当前状态快照
3. 收到事件（如 `issue_reviewing`）→ TL Agent 决策 → REST API 执行
4. 每 10s 发 heartbeat
5. `Ctrl+C` 退出 → `DELETE /api/v3/tl/register` → 云端立即接管

### 4.5 状态一致性

- 所有决策通过 REST API 持久化到云端 SQLite
- 本地和云端 TL 操作同一数据库（本地 via REST，云端 via 直接调用）
- 切换时只丢失"正在思考中但未提交"的决策，已提交的不受影响
- `action_required` 状态的 Run：如果本地 TL 连接 → 推送给它处理；如果不在 → 云端 TL 按策略处理或排队

---

## 5. 事件流

### Worker → 本地 TL（复用现有 WebSocket Hub）

```
ACP Agent (子进程)
  → stageEventBridge.HandleSessionUpdate()
  → EventBus.Publish(core.Event)
  → hub.BroadcastCoreEvent()
  → WebSocket
  → 本地 TL
```

不需要新的消息中间件。

### 本地 TL → 云端（复用现有 REST API）

```
本地 TL
  → POST /api/v3/issues/{id}/action
  → Manager.ApplyIssueAction()
  → Store.SaveIssue()
  → EventBus.Publish()
  → DepScheduler.OnEvent()
  → 调度下一步
```

不需要新的命令协议。

### 事件不丢失

- WebSocket Hub 已有 chat session 事件缓存（最多 32 条/session）
- 本地 TL 断连期间的事件，重连后通过 REST API 查询补齐（`GET /api/v3/events`）
- 宽限期内事件在 TLProxy 中 buffer，切换完成后 replay

---

## 6. 配置

### 6.1 新增配置段

```yaml
deployment:
  mode: cloud                    # local（默认，单机）| cloud（容器化）
  workspace:
    plugin: cloud                # worktree（本地）| cloud（容器）
    cache_root: /data/repos/cache
    worktree_root: /data/repos/worktrees
  tl_hybrid:
    enabled: true                # 是否允许本地 TL 注册
    grace_period: 30s            # heartbeat 丢失后的宽限期
    heartbeat_ttl: 15s           # heartbeat 超时判定
```

### 6.2 环境变量

```bash
AI_WORKFLOW_DEPLOYMENT_MODE=cloud
AI_WORKFLOW_WORKSPACE_PLUGIN=cloud
AI_WORKFLOW_WORKSPACE_CACHE_ROOT=/data/repos/cache
AI_WORKFLOW_WORKSPACE_WORKTREE_ROOT=/data/repos/worktrees
AI_WORKFLOW_DB_PATH=/data/db/data.db
AI_WORKFLOW_AUTH_TOKEN=<secret>
AI_WORKFLOW_TL_HYBRID_ENABLED=true
ANTHROPIC_API_KEY=<key>
```

### 6.3 云端 server 配置

```yaml
server:
  host: "0.0.0.0"               # 云端需监听所有接口
  port: 8080
  auth_enabled: true             # 云端必须开启认证
  auth_token: "${AI_WORKFLOW_AUTH_TOKEN}"
```

---

## 7. 实现阶段

| Phase | 内容 | 依赖 |
|-------|------|------|
| **1: 容器化基础** | Dockerfile, docker-compose.yml, readiness probe, `DeploymentConfig` 类型, 验证 SQLite WAL | 无 |
| **2: Cloud Workspace** | `Project.RemoteURL` 字段 + DB 迁移, `workspace-cloud` 插件, factory 注册, `--remote-url` CLI | Phase 1 |
| **3: TL 混合** | `tl_proxy.go` 状态机, TL 注册 API (4 端点), `ai-flow local-tl` CLI, heartbeat | Phase 1 |
| **4: 生产加固** | bare cache 定期 GC, 集成测试, K8s manifests, 运维文档 | Phase 1-3 |

---

## 8. 关键设计决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 单容器 vs 微服务 | 单容器 | ACP agents 是 stdio 子进程，拆分需替换传输协议，改动过大 |
| SQLite vs Postgres | SQLite（当前） | `core.Store` 已是抽象接口，未来可换。单实例 SQLite + WAL 足够 |
| Bare clone vs full clone | Bare clone | 节省磁盘，worktree 从 bare cache 创建更快 |
| TL 状态在哪 | 云端 SQLite | 两个 TL 操作同一数据库，切换不丢已提交状态 |
| 新消息中间件 | 不引入 | WebSocket Hub + REST API 已满足事件双向流转 |

---

## 9. 关键文件（实现时参考）

| 文件 | 改动 |
|------|------|
| `internal/config/types.go` | 新增 `DeploymentConfig`, `WorkspaceDeployment`, `TLHybridConfig` |
| `internal/core/project.go` | 新增 `RemoteURL` 字段 |
| `internal/plugins/workspace-cloud/cloud.go` | 新插件 |
| `internal/plugins/factory/factory.go` | 注册 `workspace-cloud`，按配置选择 |
| `internal/teamleader/tl_proxy.go` | 新组件 |
| `internal/web/handlers_v3.go` | TL 注册 API（4 端点） |
| `cmd/ai-flow/commands.go` | `local-tl` 子命令 + TLProxy 集成 |
| `Dockerfile` | 新文件 |
| `docker-compose.yml` | 新文件 |
