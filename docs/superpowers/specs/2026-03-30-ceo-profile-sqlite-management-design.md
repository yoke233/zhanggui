# CEO Profile Sqlite 管理设计稿

> 日期：2026-03-30
> 状态：草案
> 类型：设计文档
> 范围：CEO profile 管理、profile 持久化收口、前端 onboarding

---

## 1. 背景

当前系统已经完成第一阶段 CEO chat orchestration MVP，核心方向为：

- `CEO = chat profile + ceo-manage skill + orchestrate task CLI`
- 默认只使用 `lead` 作为执行 profile
- 优先任务编排，不默认开启 thread

但现有 `profile` 管理仍有两个问题：

1. `profile` 同时存在于 `config.toml` 和 sqlite，两边都有持久化语义
2. CEO 虽然已经具备调度能力，但还没有一套稳定的 profile 管理动作面

新的目标是：

- 将 `profile` 完整收口到 sqlite
- 让 CEO 可以通过 CLI 管理 profile
- 保留 `config.toml` 只承载全局运行配置
- 在前端为“当前只有 CEO”场景提供清晰 onboarding

---

## 2. 设计目标

本设计的目标如下：

1. 将 `profile` 变为 sqlite 的唯一事实源
2. `config.toml` 不再持有 `runtime.agents.profiles`
3. 系统启动时只幂等 seed `ceo`
4. CEO 通过专用 CLI 完成 profile 的创建、查看、修改基础字段、增删 skill
5. 新 profile 默认通过 `--from ceo` 模板化创建
6. 前端在“只有 CEO”时做引导，但不负责 seed `ceo`

---

## 3. 非目标

本阶段明确不做以下内容：

- 不把 `drivers / llm / sandbox` 迁入 sqlite
- 不开放 `actions_allowed / capabilities` 的 profile 管理
- 不开放 `session / mcp` 的 profile 管理
- 不支持多种 profile 模板体系
- 不自动 seed `lead`
- 不由前端负责创建 `ceo`
- 不把 profile 管理改造成复杂组织系统

---

## 4. 核心决策

### 4.1 profile 的唯一事实源改为 sqlite

调整后职责边界如下：

```text
config.toml
  ├─ runtime.agents.drivers
  ├─ runtime.llm
  ├─ runtime.sandbox
  └─ 其他全局运行配置

sqlite
  └─ agent_profiles
       ├─ id / name / role
       ├─ driver_id / llm_config_id
       ├─ prompt_template
       └─ skills
```

运行时解析 profile 时：

- 先从 sqlite 读取 profile 基础数据
- 再通过 `config.toml` 中的 driver / llm 配置进行 materialize

这意味着：

- “人是什么” 在 sqlite
- “系统怎么跑” 在 `config.toml`

### 4.2 启动时只 seed `ceo`

启动逻辑改为：

1. 检查 sqlite 中是否存在 `ceo`
2. 如果不存在，则写入默认 `ceo`
3. 如果存在，则不覆盖、不重置

明确不再自动 seed `lead`。

这样可以保证：

- 系统始终有一个管理入口
- 默认执行角色不被偷偷补回
- 当系统里没有其他 profile 时，由 `ceo` 自己创建新的 profile

### 4.3 新 profile 统一从 `ceo` 模板创建

第一阶段只支持一种创建方式：

```text
ai-flow profile create --from ceo ...
```

创建逻辑不是“把 `ceo` 结构体全量复制一份”，而是一个 **白名单模板投影**：

- 只允许从 `ceo` 这个模板来源创建
- 但不会继承 `ceo` 的管理型运行语义
- 创建时分两部分处理：

1. **可配置字段**
   - `id`
   - `name`
   - `role`
   - `driver_id`
   - `llm_config_id`
   - `prompt_template`

2. **系统固定字段**
   - `actions_allowed`
   - `capabilities`
   - `session`
   - `mcp`

第二部分不从 sqlite 中当前 `ceo` 记录直接继承，而是由系统内置的
`from ceo` 模板投影规则生成。第一阶段这些字段不对 CEO 开放编辑，
也不允许通过复制把 `ceo-manage` 一类管理行为带到新 profile 上。

这样做的目的不是让新 profile 继承 CEO 身份，而是：

- 保证新 profile 一开始就是一份结构完整、可运行的 profile
- 降低 CEO “招人”时的字段负担

### 4.4 前端只做 onboarding，不做 seed

`ceo` 的 seed 必须由后端负责，前端不承担初始化职责。

前端在用户进入系统时应做以下事情：

1. 拉取 profile 列表
2. 只有在 **profile 列表请求成功** 且结果 **恰好只有一个 `ceo`**
   时，才判定为 onboarding 场景
3. 展示 onboarding 卡片
4. 告诉用户现在可以：
   - 直接和 CEO 对话
   - 创建第一个执行型 profile

如果 profile 列表请求失败，则前端必须进入错误态，而不是误判成
“当前只有 CEO”。

推荐前端提示文案语义：

- 当前系统已初始化 CEO
- 你可以直接把目标交给 CEO
- 如果需要执行人，可以先创建一个新的 profile

---

## 5. CLI 设计

### 5.1 命令范围

新增独立 profile 管理命名空间：

```text
ai-flow profile list
ai-flow profile get --id <profile_id>
ai-flow profile create --from ceo --id <id> --name <name> --role <role> --driver <driver_id> --llm <llm_config_id> --prompt <prompt_template>
ai-flow profile set-base --id <id> [--name ...] [--role ...] [--driver ...] [--llm ...] [--prompt ...]
ai-flow profile add-skill --id <id> --skill <skill_name>
ai-flow profile remove-skill --id <id> --skill <skill_name>
ai-flow profile delete --id <id>
```

CEO skill 通过这些 CLI 管理 profile，而不是直接写 `curl` 或直接操作数据库。

### 5.2 第一阶段允许管理的字段

本阶段仅允许 CEO 管理以下 4 类字段：

1. `id / name / role`
2. `driver_id / llm_config_id`
3. `prompt_template`
4. `skills`

明确不开放：

- `actions_allowed / capabilities`
- `session / mcp`

### 5.3 命令职责

`profile list`

- 列出 sqlite 中全部 profile

`profile get`

- 查看单个 profile 详情

`profile create --from ceo`

- 从 `ceo` 模板复制出一个新 profile

`profile set-base`

- 修改新 profile 的基础字段
- 只允许改 `name / role / driver / llm / prompt`

`profile add-skill`

- 向 profile 追加一个 skill

`profile remove-skill`

- 从 profile 中移除一个 skill

`profile delete`

- 删除 profile
- 但禁止删除 `ceo`

---

## 6. 数据流设计

### 6.1 现状问题

当前 profile 更新路径带有“双写”特征：

- registry / store 更新 sqlite
- HTTP profile 管理同时尝试同步回 `config.toml`

这会带来三个问题：

1. sqlite 和 `config.toml` 容易出现语义漂移
2. profile 的真实来源不清晰
3. CEO 的 CLI 管理路径无法稳定收口

### 6.2 改造后的单源数据流

改造后采用如下路径：

```text
CLI / HTTP
  -> AgentRegistry / Profile 管理服务
  -> sqlite.agent_profiles

运行时 Resolve
  -> 从 sqlite 取 profile
  -> 从 config.toml 取 driver / llm
  -> materialize 成可运行 profile
```

关键变化：

- profile 的 CRUD 只落 sqlite
- 不再调用 `CreateProfileConfig / UpdateProfileConfig / DeleteProfileConfig`
- `configruntime.SyncRegistry` 不再承担“把 config.toml profile 同步到 sqlite”的职责
- 必须显式重构或移除 `bootstrap_runtime.go -> SyncRegistry` 这条 profile
  回填链路，否则 sqlite 单源会继续被 `config.toml` 覆盖或删改

建议收口方式：

- `SyncRegistry` 不再同步 profile
- profile 初始化与 seed 改为独立启动逻辑
- runtime reload 只处理 driver / llm / sandbox 等全局运行配置

---

## 7. 启动与迁移规则

### 7.1 启动规则

启动时对 profile 采用如下规则：

1. 不从 `defaults.toml` 批量导入 profile
2. 不从 `config.toml` 回填 profile
3. 只检查 sqlite 中是否存在 `ceo`
4. 若不存在，则 seed `ceo`

这里的前提是：

- profile 的 seed 与启动检查不再依赖 `SyncRegistry`
- runtime reload 不得再把 `snap.Profiles` 写回 sqlite

### 7.2 迁移策略

采用受控迁移：

- 第一阶段的“sqlite 单源 + 只 seed `ceo`”行为默认只面向 **新 data dir**
  或明确知道当前环境尚未依赖历史 config profile 的场景
- 对已经依赖 `config.toml` 中 legacy profile 的老环境，不做静默自动切换
- 本阶段不做“自动批量导入 legacy profile 到 sqlite”
- 老环境切换到 sqlite 单源应通过后续单独迁移动作完成

这样可以避免：

- 老用户在无感升级后突然失去原有 `lead` 或其他 profile
- 新逻辑和旧环境的 profile 心智混在一起
- 为了兼容 legacy profile 而把“只 seed `ceo`”语义冲淡

---

## 8. API 行为调整

现有 `/agents/profiles` API 可以保留，但语义需调整：

- `GET /agents/profiles`
  - 读取 sqlite
- `POST /agents/profiles`
  - 写 sqlite
- `PUT /agents/profiles/{profileID}`
  - 写 sqlite
- `DELETE /agents/profiles/{profileID}`
  - 写 sqlite

必须移除“顺带同步回 `config.toml`”的逻辑。
但必须保留现有 runtime `driver / llm` 解析与兼容性校验能力，
不能因为删除 config 回写而把 profile 校验一起删掉。

这样可确保：

- Web
- CLI
- CEO skill

看到的是同一份 profile 数据。

---

## 9. 校验与保护规则

第一阶段必须保证以下规则：

### 9.1 创建规则

- `create --from ceo` 时，必须存在 `ceo`
- `driver_id` 必须能在运行配置中解析到
- `llm_config_id` 必须能在运行配置中解析到

### 9.2 skill 规则

- `add-skill` 前必须校验 skill 是否存在
- 重复添加 skill 应幂等返回
- 删除不存在的 skill 可安全返回“未变更”

### 9.3 删除规则

- 禁止删除 `ceo`
- 删除不存在的 profile 应返回 `not found`

### 9.4 运行时规则

- 即使系统里没有任何执行 profile，也不能影响 `ceo` 登录和管理能力
- 当没有可执行人时，前端 onboarding 和 CEO 对话都应明确提示“需要先创建一个 profile”

---

## 10. 测试策略

至少覆盖以下测试：

### 10.1 sqlite 单源测试

- profile CRUD 只依赖 sqlite
- profile 更新后不再回写 `config.toml`

### 10.2 seed 测试

- 空库启动只 seed `ceo`
- 启动不会自动补 `lead`
- 已存在 `ceo` 时不覆盖

### 10.3 CLI 测试

- `profile list`
- `profile get`
- `profile create --from ceo`
- `profile set-base`
- `profile add-skill`
- `profile remove-skill`
- `profile delete`

### 10.4 保护规则测试

- 禁止删除 `ceo`
- 非法 `driver_id` 创建失败
- 非法 `llm_config_id` 创建失败
- 非法 `skill` 添加失败

### 10.5 前端 / API 行为测试

- 只有在 profile 列表请求成功且结果恰好只有 `ceo` 时，
  才识别为 onboarding 状态
- profile 列表请求失败时应进入错误态，而不是 onboarding
- `/agents/profiles` 返回 sqlite 中的 profile 数据

---

## 11. 验收标准

本设计落地后，应满足以下验收标准：

1. `profile` 不再依赖 `config.toml` 作为持久化来源
2. 空库启动后系统里只有 `ceo`
3. 用户进入前端时能清楚知道当前可以：
   - 和 CEO 对话
   - 创建第一个 profile
4. CEO 能通过 CLI：
   - 创建新 profile
   - 修改基础字段
   - 添加 skill
   - 删除 skill
5. 删除 `ceo` 会被明确拒绝
6. 当没有其他 profile 时，系统仍可正常工作，只是提示需要先创建执行 profile

---

## 12. 推荐实现顺序

建议按以下顺序实施：

1. 切断 profile 到 `config.toml` 的回写链路
2. 明确 sqlite 成为唯一事实源
3. 增加“只 seed `ceo`”启动逻辑
4. 新增 `ai-flow profile ...` CLI
5. 调整 `/agents/profiles` 行为为只读写 sqlite
6. 增加前端 onboarding
7. 补齐测试并做单条场景验收

---

## 13. 总结

本设计的核心不是“再加一套 profile 功能”，而是把 profile 这条线彻底收口：

- 数据源收口到 sqlite
- 管理入口收口到 CEO CLI / API
- 初始化职责收口到后端 seed
- 用户认知收口到前端 onboarding

这样之后，CEO 才真正具备“先创建组织成员，再继续分派任务”的能力，而且不会再被
`config.toml` 和 sqlite 的双源语义拖乱。
