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
	"github.com/yoke233/ai-workflow/internal/platform/config"
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
		"run":       "Run 执行默认值（超时、模板、重试）",
		"scheduler": "并发调度限制（全局 agent 数、项目 run 数）",
		"server":    "HTTP 服务器设置（监听地址、端口）",
		"github":    "GitHub 集成（App、Webhook、PR 自动化）",
		"store":     "持久化后端（SQLite 驱动和路径）",
		"context":   "上下文存储提供者设置",
		"log":       "日志配置（级别、文件轮转）",
		"runtime":   "运行时引擎配置（drivers、profiles、sandbox、mcp、prompts）",
	}

	if schema.Properties != nil {
		for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
			if desc, ok := topDescs[pair.Key]; ok {
				pair.Value.Description = desc
			}
		}
	}

	fieldDescs := map[string]map[string]string{
		"RunConfig": {
			"default_template": "默认 run 模板（standard / fast_release 等）",
			"global_timeout":   "单次 run 全局超时", "auto_infer_template": "是否自动推断模板",
			"max_total_retries": "最大重试次数",
		},
		"SchedulerConfig": {
			"max_global_agents": "全局最多同时运行的 agent 数",
			"max_project_runs":  "每个项目最多的并发 run 数",
		},
		"ServerConfig": {
			"host": "监听地址（127.0.0.1 = 仅本地）", "port": "监听端口",
		},
		"StoreConfig":   {"driver": "存储驱动（sqlite）", "path": "数据库文件路径（相对项目根目录）"},
		"ContextConfig": {"provider": "上下文提供者（context-sqlite / mock / 空=禁用）", "path": "SQLite 文件路径"},
		"LogConfig": {
			"level": "日志级别（debug / info / warn / error）", "file": "日志文件路径",
			"max_size_mb": "单个日志文件最大 MB", "max_age_days": "日志保留天数",
		},
		"CapabilitiesConfig": {"fs_read": "文件系统读权限", "fs_write": "文件系统写权限", "terminal": "终端执行权限"},
		"MCPConfig":          {"enabled": "是否启用 MCP 工具注入", "tools": "注入的 MCP 工具名列表"},
		"RuntimeConfig": {
			"mock_executor": "是否使用本地 stub 执行器",
			"collector":     "运行时元数据采集配置",
			"llm":           "LLM API provider 配置",
			"sandbox":       "运行时沙箱配置",
			"agents":        "运行时 driver 和 profile 配置",
			"mcp":           "运行时 MCP server 与绑定配置",
			"prompts":       "运行时提示词模板",
		},
		"RuntimeLLMConfig": {
			"default_config_id": "当前默认启用的 LLM 配置 ID",
			"configs":           "可维护的多条 LLM 配置项",
		},
		"RuntimeLLMEntryConfig": {
			"id":       "配置唯一标识",
			"type":     "LLM provider 类型（openai_chat_completion / openai_response / anthropic）",
			"base_url": "LLM API 根地址",
			"api_key":  "LLM API 密钥",
			"model":    "默认模型名",
		},
		"RuntimeDriverConfig": {
			"id": "driver 唯一标识", "launch_command": "启动命令", "launch_args": "启动参数",
			"env": "注入环境变量", "capabilities_max": "能力上限",
		},
		"RuntimeProfileConfig": {
			"id": "profile 唯一标识", "name": "展示名称", "driver": "引用的 driver ID",
			"llm_config_id": "引用的 LLM provider 配置 ID（位于 runtime.llm.configs）",
			"role":          "运行时角色", "capabilities": "能力标签", "actions_allowed": "允许动作",
			"prompt_template": "提示词模板名", "skills": "启用 skill 列表", "session": "会话复用配置", "mcp": "MCP 配置",
		},
		"RuntimeSessionConfig": {
			"reuse": "是否复用 session", "max_turns": "单 session 最大轮数", "idle_ttl": "空闲过期时间",
		},
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
