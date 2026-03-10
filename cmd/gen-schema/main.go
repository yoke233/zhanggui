// gen-schema generates a JSON Schema from the config.Config struct.
//
//	go run ./cmd/gen-schema > configs/config-schema.json
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/yoke233/ai-workflow/internal/config"
)

func main() {
	r := &jsonschema.Reflector{
		FieldNameTag:              "toml",
		AllowAdditionalProperties: true,
		Anonymous:                 true,
	}
	r.Mapper = func(t reflect.Type) *jsonschema.Schema {
		if t == reflect.TypeOf(config.Duration{}) {
			return &jsonschema.Schema{
				Type:        "string",
				Description: "持续时间字符串，如 \"2h\", \"30m\", \"5s\"",
				Examples:    []any{"2h", "30m", "10s", "500ms"},
			}
		}
		if t == reflect.TypeOf(time.Duration(0)) {
			return &jsonschema.Schema{Type: "string", Description: "持续时间字符串"}
		}
		return nil
	}

	schema := r.ReflectFromType(reflect.TypeOf(config.Config{}))
	schema.Title = "AI Workflow Config"
	schema.Description = "ai-workflow 编排器配置 (.ai-workflow/config.toml)"

	addDescriptions(schema)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(schema); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
}

func addDescriptions(schema *jsonschema.Schema) {
	topDescs := map[string]string{
		"agents":        "Agent 启动配置（ACP 启动命令、能力上限）",
		"roles":         "角色定义（绑定 agent、能力、提示词模板）",
		"role_bindings": "角色绑定（team_leader、run 阶段、review 分配）",
		"run":           "Run 执行默认值（超时、模板、重试）",
		"scheduler":     "并发调度限制（全局 agent 数、项目 run 数）",
		"team_leader":   "Team Leader 编排设置（审核门、DAG 调度器）",
		"a2a":           "Agent-to-Agent 协议设置",
		"server":        "HTTP 服务器设置（监听地址、端口、认证）",
		"github":        "GitHub 集成（App、Webhook、PR 自动化）",
		"store":         "持久化后端（SQLite 驱动和路径）",
		"context":       "上下文存储提供者设置",
		"log":           "日志配置（级别、文件轮转）",
	}

	if schema.Properties != nil {
		for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
			if desc, ok := topDescs[pair.Key]; ok {
				pair.Value.Description = desc
			}
		}
	}

	fieldDescs := map[string]map[string]string{
		"AgentProfileConfig": {
			"name": "Agent 标识符（唯一）", "launch_command": "启动命令（如 npx）",
			"launch_args": "启动参数列表", "env": "注入的环境变量",
			"capabilities_max": "能力上限（角色能力不能超出此上限）",
		},
		"RoleConfig": {
			"name": "角色名称（唯一）", "agent": "引用的 agent 名称",
			"prompt_template": "提示词模板名（对应 prompt_templates/{name}.tmpl）",
			"capabilities":    "角色能力（必须 ≤ agent capabilities_max）",
			"session":         "ACP 会话策略", "permission_policy": "文件/终端权限规则",
			"mcp": "MCP 工具注入配置",
		},
		"RunConfig": {
			"default_template": "默认 run 模板（standard / fast_release 等）",
			"global_timeout":   "单次 run 全局超时", "auto_infer_template": "是否自动推断模板",
			"max_total_retries": "最大重试次数",
		},
		"SchedulerConfig": {
			"max_global_agents": "全局最多同时运行的 agent 数",
			"max_project_runs":  "每个项目最多的并发 run 数",
		},
		"TeamLeaderConfig": {
			"review_gate_plugin":  "审核门插件（review-ai-panel / review-local / review-github-pr）",
			"review_orchestrator": "多 agent 审核编排设置",
			"dag_scheduler":       "DAG 任务调度器设置",
		},
		"A2AConfig": {
			"enabled": "是否启用 A2A 协议",
			"token":   "认证 token（自动生成到 secrets.yaml）",
			"version": "A2A 协议版本",
		},
		"ServerConfig": {
			"host": "监听地址（127.0.0.1 = 仅本地）", "port": "监听端口",
			"auth_enabled": "是否启用 API 认证", "auth_token": "API 认证 token",
		},
		"StoreConfig":   {"driver": "存储驱动（sqlite）", "path": "数据库文件路径（相对项目根目录）"},
		"ContextConfig": {"provider": "上下文提供者（context-sqlite / mock / 空=禁用）", "path": "SQLite 文件路径"},
		"LogConfig": {
			"level": "日志级别（debug / info / warn / error）", "file": "日志文件路径",
			"max_size_mb": "单个日志文件最大 MB", "max_age_days": "日志保留天数",
		},
		"SessionConfig": {
			"reuse": "是否复用 ACP session", "prefer_load_session": "崩溃恢复优先 LoadSession",
			"reset_prompt": "每次重发 system prompt", "max_turns": "单 session 最大交互轮数",
		},
		"MCPConfig":          {"enabled": "是否启用 MCP 工具注入", "tools": "注入的 MCP 工具名列表"},
		"CapabilitiesConfig": {"fs_read": "文件系统读权限", "fs_write": "文件系统写权限", "terminal": "终端执行权限"},
		"GitHubConfig": {
			"enabled": "是否启用 GitHub 集成", "token": "GitHub PAT（或通过 secrets.yaml）",
			"app_id": "GitHub App ID", "private_key_path": "GitHub App 私钥路径",
			"installation_id": "GitHub App 安装 ID", "owner": "仓库 owner", "repo": "仓库名",
			"webhook_secret": "Webhook 签名密钥", "webhook_enabled": "是否接收 Webhook",
			"pr_enabled": "是否启用 PR 管理", "auto_trigger": "Issue 创建时自动触发 run",
			"allow_pat_fallback": "App 认证失败时回退到 PAT", "pr": "PR 自动化设置",
		},
		"GitHubPRConfig": {
			"auto_create": "自动创建 PR", "draft": "创建为草稿 PR",
			"auto_merge": "自动合并", "reviewers": "指定 reviewer",
			"labels": "PR 标签", "branch_prefix": "分支名前缀",
		},
	}

	for typeName, descs := range fieldDescs {
		typeDef, ok := schema.Definitions[typeName]
		if !ok || typeDef.Properties == nil {
			continue
		}
		for prop := typeDef.Properties.Oldest(); prop != nil; prop = prop.Next() {
			if desc, ok := descs[prop.Key]; ok {
				prop.Value.Description = desc
			}
		}
	}
}
