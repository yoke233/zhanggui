# 系统缺失项审查报告

> 审查日期: 2026-03-13
> 范围: 代码实现层面的缺失，不包括文档

---

## 一、未实现 / 半实现的功能

### 1. GitHub Skill 导入 — 返回 501
- **位置**: `internal/adapters/http/skills.go:171`
- **现状**: 接口已注册，但处理函数直接返回 `501 Not Implemented`
- **影响**: 前端 `ImportGitHubDialog` 组件已就绪，但调用会失败

### 2. Planning 模块 — 实现完整但依赖 LLM 配置
- **契约**: `internal/application/planning/contracts.go`
- **实现**: `internal/adapters/planning/llm/dag_generator.go`（含测试）
- **接线**: `internal/platform/bootstrap/bootstrap_api.go:38-40`
- **现状**: 功能完整，但需要配置 LLM client；未配置时 HTTP 返回 503
- **注意**: `application/planning/` 目录仅有数据结构定义，业务逻辑在 adapter 层实现，
  这偏离了分层架构约定（应用层逻辑不应下沉到 adapter 层）

### 3. Chat 应用层 — 仅有契约定义
- **位置**: `internal/application/chat/contracts.go`
- **现状**: 只有接口定义，聊天服务的业务逻辑分散在 adapter 层
- **影响**: 应用层缺乏对聊天业务的统一编排

### 4. NATS Embedded 模式 — 预留未实现
- **位置**: `RuntimeNATSConfig.Embedded` / `EmbeddedDataDir`
- **现状**: 配置字段存在，但未接线，分布式模式必须依赖外部 NATS 服务器
- **影响**: 部署复杂度高，无法开箱即用分布式执行

### 5. Skill 描述缺失
- **位置**: `internal/skills/skillset.go:64`
- **现状**: 描述字段硬编码为 `"TODO"`

---

## 二、CI/CD 完全缺失

| 缺失项 | 说明 |
|--------|------|
| GitHub Actions | 无 `.github/workflows/` 目录 |
| GitLab CI | 无 `.gitlab-ci.yml` |
| 任何 CI 配置 | 测试完全依赖手动运行 PowerShell 脚本 |

**影响**: 无自动化构建、测试、部署流水线；代码合入无质量门禁。

---

## 三、测试覆盖缺口

### 后端 (Go)
- 242 个源文件，94 个测试文件（约 39% 文件覆盖率）
- **完全无测试的包**:
  - `adapters/llm/` — LLM 客户端
  - `adapters/llmconfig/` — LLM 配置服务
  - `adapters/mcp/` — MCP 分析服务器
  - `application/chat/` — 聊天服务
  - `application/planning/` — 规划服务
  - `application/runtime/` — 会话管理
  - `platform/appdata/` — 数据路径工具
  - `skills/builtin/` — 内建技能

### 前端 (React)
- 23 个页面，仅 3 个有测试（13% 页面覆盖率）
- **无测试的关键页面**: DashboardPage, ProjectsPage, AgentsPage, AnalyticsPage, LoginPage, SkillsPage 等 20 个

---

## 四、前端缺失项

### 1. 无全局错误边界 (Error Boundary)
- **现状**: 各页面各自 try-catch，无顶层 React Error Boundary
- **影响**: 未捕获的渲染异常会导致整个应用白屏

### 2. 无 API 请求重试策略
- **现状**: `apiClient.ts` 对瞬时失败（网络抖动、502/503）无自动重试
- **影响**: 网络不稳定时用户体验差

### 3. WebSocket 断线原因不透明
- **现状**: 重连时只有状态回调，不传递断开原因
- **影响**: 用户无法区分网络问题还是服务端主动断开

### 4. 状态管理偏薄
- **现状**: 仅 2 个 Zustand store（notification、settings），其余状态散落在组件内
- **影响**: 跨页面状态共享困难，如全局加载态、用户信息等无集中管理

---

## 五、基础设施 & 运维缺失

### 1. 无 docker-compose.yml
- **现状**: 有 Dockerfile 但无编排文件
- **影响**: 多容器场景（如外挂 NATS、数据库卷挂载）需手动 docker run

### 2. 无 Makefile / 任务编排
- **现状**: 构建、测试、部署全靠分散的 PowerShell/bash 脚本
- **影响**: 新人上手成本高，无统一入口命令

### 3. 数据库迁移无版本管理
- **现状**: GORM AutoMigrate，无显式迁移文件
- **影响**: 生产环境 schema 变更不可追溯、不可回滚；多实例升级有风险

### 4. 无 `.env.example`（根目录）
- **现状**: 仅 `web/.env.example` 存在，后端环境变量无模板
- **影响**: 新开发者不知道需要配哪些环境变量

### 5. 无 Prometheus/Grafana 指标
- **现状**: OpenTelemetry 依赖已引入但默认禁用，无 metrics endpoint
- **影响**: 生产环境无法监控 QPS、延迟、错误率等关键指标

### 6. 健康检查不完整
- **现状**: 仅 `GET /health` 返回 `{"status":"ok"}`
- **缺失**: 无 readiness probe（检查 DB 连接、NATS 连接）、无 liveness probe 分离
- **影响**: Kubernetes 部署时无法正确判断服务就绪状态

---

## 六、安全 & 代码质量

### 1. CORS 默认允许所有来源
- **位置**: `internal/adapters/http/server/middleware.go`
- **现状**: `allowedOrigins` 为空时允许所有 Origin
- **影响**: 生产环境存在 CSRF 风险

### 2. 无 Go 静态分析配置
- **现状**: 无 `.golangci.yml`，仅依赖 `gofmt`
- **影响**: 无法自动捕获常见 bug（空指针、资源泄漏、错误忽略等）

### 3. 无 Prettier 配置
- **现状**: 前端仅有 ESLint，无代码格式化工具配置
- **影响**: 代码风格不一致

### 4. 无 `.editorconfig`
- **影响**: 不同编辑器的缩进、换行符等设置不统一

---

## 七、功能性缺口汇总（按优先级排序）

| 优先级 | 缺失项 | 类别 | 建议 |
|--------|--------|------|------|
| **P0** | CI/CD 流水线 | 基础设施 | 添加 GitHub Actions: lint + test + build + docker |
| **P0** | 数据库迁移版本管理 | 基础设施 | 引入 golang-migrate 或 atlas |
| **P0** | 全局 Error Boundary | 前端 | 添加 React Error Boundary 组件 |
| **P1** | CORS 默认策略收紧 | 安全 | 生产环境强制配置 allowedOrigins |
| **P1** | golangci-lint 配置 | 代码质量 | 添加 `.golangci.yml` 并集成到 CI |
| **P1** | 健康检查增强 | 运维 | 添加 /readyz、/livez，检查 DB/NATS 连接 |
| **P1** | Prometheus metrics | 运维 | 启用 OTEL metrics exporter 或添加 /metrics |
| **P2** | GitHub Skill 导入实现 | 功能 | 完成 501 存根的实际实现 |
| **P3** | Planning 模块分层重构 | 架构 | 将 adapter 层业务逻辑上移到 application 层 |
| **P2** | docker-compose.yml | 基础设施 | 编排 app + nats + volume |
| **P2** | 后端测试覆盖 | 质量 | 优先补 llm/mcp/chat 包的测试 |
| **P2** | 前端测试覆盖 | 质量 | 优先补 Dashboard/Login/Projects 页面测试 |
| **P2** | API 重试策略 | 前端 | apiClient 添加指数退避重试 |
| **P3** | Makefile | 开发体验 | 统一 build/test/dev 入口 |
| **P3** | .env.example | 开发体验 | 根目录添加后端环境变量模板 |
| **P3** | NATS Embedded 模式 | 功能 | 实现内嵌 NATS，简化单机部署 |
| **P3** | Prettier + EditorConfig | 代码质量 | 统一代码格式 |
| **P3** | 前端状态管理增强 | 前端 | 添加 auth store、全局 loading store |
