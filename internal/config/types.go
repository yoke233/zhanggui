package config

import "time"

type Config struct {
	Agents    AgentsConfig    `yaml:"agents"`
	Roles     []RoleConfig    `yaml:"roles"`
	RoleBinds RoleBindings    `yaml:"role_bindings"`
Runtime   RuntimeConfig   `yaml:"runtime"`
	Pipeline  PipelineConfig  `yaml:"pipeline"`
	Scheduler SchedulerConfig `yaml:"scheduler"`
	Secretary SecretaryConfig `yaml:"secretary"`
	Server    ServerConfig    `yaml:"server"`
	GitHub    GitHubConfig    `yaml:"github"`
	Store     StoreConfig     `yaml:"store"`
	Log       LogConfig       `yaml:"log"`
}

type AgentsConfig struct {
	Claude   *AgentConfig         `yaml:"claude"`
	Codex    *AgentConfig         `yaml:"codex"`
	OpenSpec *AgentConfig         `yaml:"openspec"`
	Profiles []AgentProfileConfig `yaml:"-"`
}

type AgentConfig struct {
	Plugin          *string             `yaml:"plugin"`
	Binary          *string             `yaml:"binary"`
	MaxTurns        *int                `yaml:"default_max_turns"`
	DefaultTools    *[]string           `yaml:"default_tools"`
	Model           *string             `yaml:"model"`
	Reasoning       *string             `yaml:"reasoning"`
	Sandbox         *string             `yaml:"sandbox"`
	Approval        *string             `yaml:"approval"`
	CapabilitiesMax *CapabilitiesConfig `yaml:"capabilities_max"`
}

type PipelineConfig struct {
	DefaultTemplate   string        `yaml:"default_template"`
	GlobalTimeout     time.Duration `yaml:"global_timeout"`
	AutoInferTemplate bool          `yaml:"auto_infer_template"`
	MaxTotalRetries   int           `yaml:"max_total_retries"`
}

type RuntimeConfig struct {
	Driver string `yaml:"driver"`
}

type SchedulerConfig struct {
	MaxGlobalAgents     int `yaml:"max_global_agents"`
	MaxProjectPipelines int `yaml:"max_project_pipelines"`
}

type SecretaryConfig struct {
	ReviewGatePlugin   string                   `yaml:"review_gate_plugin"`
	ReviewOrchestrator ReviewOrchestratorConfig `yaml:"review_orchestrator"`
	DAGScheduler       DAGSchedulerConfig       `yaml:"dag_scheduler"`
}

type ReviewOrchestratorConfig struct {
	MaxRounds int `yaml:"max_rounds"`
}

type DAGSchedulerConfig struct {
	MaxConcurrentTasks int `yaml:"max_concurrent_tasks"`
}

type ServerConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	AuthEnabled bool   `yaml:"auth_enabled"`
	AuthToken   string `yaml:"auth_token"`
}

type GitHubConfig struct {
	Enabled             bool              `yaml:"enabled"`
	Token               string            `yaml:"token"`
	AppID               int64             `yaml:"app_id"`
	PrivateKeyPath      string            `yaml:"private_key_path"`
	InstallationID      int64             `yaml:"installation_id"`
	Owner               string            `yaml:"owner"`
	Repo                string            `yaml:"repo"`
	WebhookSecret       string            `yaml:"webhook_secret"`
	WebhookEnabled      bool              `yaml:"webhook_enabled"`
	PREnabled           bool              `yaml:"pr_enabled"`
	LabelMapping        map[string]string `yaml:"label_mapping"`
	AuthorizedUsernames []string          `yaml:"authorized_usernames"`
	AutoTrigger         bool              `yaml:"auto_trigger"`
	AllowPATFallback    bool              `yaml:"allow_pat_fallback"`
	PR                  GitHubPRConfig    `yaml:"pr"`
}

type GitHubPRConfig struct {
	AutoCreate   bool     `yaml:"auto_create"`
	Draft        bool     `yaml:"draft"`
	AutoMerge    bool     `yaml:"auto_merge"`
	Reviewers    []string `yaml:"reviewers"`
	Labels       []string `yaml:"labels"`
	BranchPrefix string   `yaml:"branch_prefix"`
}

type StoreConfig struct {
	Driver string `yaml:"driver"`
	Path   string `yaml:"path"`
}

type LogConfig struct {
	Level      string `yaml:"level"`
	File       string `yaml:"file"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxAgeDays int    `yaml:"max_age_days"`
}

// ConfigLayer 表示可选覆盖层。nil 字段表示“未设置”，用于多层配置继承合并。
type ConfigLayer struct {
	Agents    *AgentsLayer       `yaml:"agents"`
	Roles     *[]RoleConfig      `yaml:"roles"`
	RoleBinds *RoleBindingsLayer `yaml:"role_bindings"`
Runtime   *RuntimeLayer      `yaml:"runtime"`
	Pipeline  *PipelineLayer     `yaml:"pipeline"`
	Scheduler *SchedulerLayer    `yaml:"scheduler"`
	Secretary *SecretaryLayer    `yaml:"secretary"`
	Server    *ServerLayer       `yaml:"server"`
	GitHub    *GitHubLayer       `yaml:"github"`
	Store     *StoreLayer        `yaml:"store"`
	Log       *LogLayer          `yaml:"log"`
}

type AgentsLayer struct {
	Claude   *AgentConfig          `yaml:"claude"`
	Codex    *AgentConfig          `yaml:"codex"`
	OpenSpec *AgentConfig          `yaml:"openspec"`
	Profiles *[]AgentProfileConfig `yaml:"-"`
}

type PipelineLayer struct {
	DefaultTemplate   *string        `yaml:"default_template"`
	GlobalTimeout     *time.Duration `yaml:"global_timeout"`
	AutoInferTemplate *bool          `yaml:"auto_infer_template"`
	MaxTotalRetries   *int           `yaml:"max_total_retries"`
}

type RuntimeLayer struct {
	Driver *string `yaml:"driver"`
}

type SchedulerLayer struct {
	MaxGlobalAgents     *int `yaml:"max_global_agents"`
	MaxProjectPipelines *int `yaml:"max_project_pipelines"`
}

type SecretaryLayer struct {
	ReviewGatePlugin   *string                  `yaml:"review_gate_plugin"`
	ReviewOrchestrator *ReviewOrchestratorLayer `yaml:"review_orchestrator"`
	DAGScheduler       *DAGSchedulerLayer       `yaml:"dag_scheduler"`
}

type ReviewOrchestratorLayer struct {
	MaxRounds *int `yaml:"max_rounds"`
}

type DAGSchedulerLayer struct {
	MaxConcurrentTasks *int `yaml:"max_concurrent_tasks"`
}

type ServerLayer struct {
	Host        *string `yaml:"host"`
	Port        *int    `yaml:"port"`
	AuthEnabled *bool   `yaml:"auth_enabled"`
	AuthToken   *string `yaml:"auth_token"`
}

type GitHubLayer struct {
	Enabled             *bool              `yaml:"enabled"`
	Token               *string            `yaml:"token"`
	AppID               *int64             `yaml:"app_id"`
	PrivateKeyPath      *string            `yaml:"private_key_path"`
	InstallationID      *int64             `yaml:"installation_id"`
	Owner               *string            `yaml:"owner"`
	Repo                *string            `yaml:"repo"`
	WebhookSecret       *string            `yaml:"webhook_secret"`
	WebhookEnabled      *bool              `yaml:"webhook_enabled"`
	PREnabled           *bool              `yaml:"pr_enabled"`
	LabelMapping        *map[string]string `yaml:"label_mapping"`
	AuthorizedUsernames *[]string          `yaml:"authorized_usernames"`
	AutoTrigger         *bool              `yaml:"auto_trigger"`
	AllowPATFallback    *bool              `yaml:"allow_pat_fallback"`
	PR                  *GitHubPRLayer     `yaml:"pr"`
}

type GitHubPRLayer struct {
	AutoCreate   *bool     `yaml:"auto_create"`
	Draft        *bool     `yaml:"draft"`
	AutoMerge    *bool     `yaml:"auto_merge"`
	Reviewers    *[]string `yaml:"reviewers"`
	Labels       *[]string `yaml:"labels"`
	BranchPrefix *string   `yaml:"branch_prefix"`
}

type StoreLayer struct {
	Driver *string `yaml:"driver"`
	Path   *string `yaml:"path"`
}

type LogLayer struct {
	Level      *string `yaml:"level"`
	File       *string `yaml:"file"`
	MaxSizeMB  *int    `yaml:"max_size_mb"`
	MaxAgeDays *int    `yaml:"max_age_days"`
}

type CapabilitiesConfig struct {
	FSRead   bool `yaml:"fs_read"`
	FSWrite  bool `yaml:"fs_write"`
	Terminal bool `yaml:"terminal"`
}

type AgentProfileConfig struct {
	Name            string             `yaml:"name"`
	LaunchCommand   string             `yaml:"launch_command"`
	LaunchArgs      []string           `yaml:"launch_args"`
	Env             map[string]string  `yaml:"env"`
	CapabilitiesMax CapabilitiesConfig `yaml:"capabilities_max"`
}

type RoleConfig struct {
	Name             string             `yaml:"name"`
	Agent            string             `yaml:"agent"`
	PromptTemplate   string             `yaml:"prompt_template"`
	Capabilities     CapabilitiesConfig `yaml:"capabilities"`
	Session          SessionConfig      `yaml:"session"`
	PermissionPolicy []PermissionRule   `yaml:"permission_policy"`
	MCP              MCPConfig          `yaml:"mcp"`
}

type SessionConfig struct {
	Reuse             bool `yaml:"reuse"`
	PreferLoadSession bool `yaml:"prefer_load_session"`
	ResetPrompt       bool `yaml:"reset_prompt"`
	MaxTurns          int  `yaml:"max_turns"`
}

type PermissionRule struct {
	Pattern string `yaml:"pattern"`
	Action  string `yaml:"action"`
	Scope   string `yaml:"scope"`
}

type MCPConfig struct {
	Enabled bool     `yaml:"enabled"`
	Tools   []string `yaml:"tools"`
}

type RoleBindings struct {
	Secretary          SingleRoleBinding    `yaml:"secretary"`
	Pipeline           PipelineRoleBindings `yaml:"pipeline"`
	ReviewOrchestrator ReviewRoleBindings   `yaml:"review_orchestrator"`
	PlanParser         SingleRoleBinding    `yaml:"plan_parser"`
}

type SingleRoleBinding struct {
	Role string `yaml:"role"`
}

type PipelineRoleBindings struct {
	StageRoles map[string]string `yaml:"stage_roles"`
}

type ReviewRoleBindings struct {
	Reviewers  map[string]string `yaml:"reviewers"`
	Aggregator string            `yaml:"aggregator"`
}

type RoleBindingsLayer struct {
	Secretary          *SingleRoleBindingLayer    `yaml:"secretary"`
	Pipeline           *PipelineRoleBindingsLayer `yaml:"pipeline"`
	ReviewOrchestrator *ReviewRoleBindingsLayer   `yaml:"review_orchestrator"`
	PlanParser         *SingleRoleBindingLayer    `yaml:"plan_parser"`
}

type SingleRoleBindingLayer struct {
	Role *string `yaml:"role"`
}

type PipelineRoleBindingsLayer struct {
	StageRoles *map[string]string `yaml:"stage_roles"`
}

type ReviewRoleBindingsLayer struct {
	Reviewers  *map[string]string `yaml:"reviewers"`
	Aggregator *string            `yaml:"aggregator"`
}

func (c *AgentsConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type legacy struct {
		Claude   *AgentConfig `yaml:"claude"`
		Codex    *AgentConfig `yaml:"codex"`
		OpenSpec *AgentConfig `yaml:"openspec"`
	}

	// yaml.v3 passes *yaml.Node here, decode through closure.
	var list []AgentProfileConfig
	if err := unmarshal(&list); err == nil {
		c.Profiles = cloneAgentProfiles(list)
		return nil
	}

	var l legacy
	if err := unmarshal(&l); err != nil {
		return err
	}
	c.Claude = l.Claude
	c.Codex = l.Codex
	c.OpenSpec = l.OpenSpec
	return nil
}

func (c *AgentsLayer) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type legacy struct {
		Claude   *AgentConfig `yaml:"claude"`
		Codex    *AgentConfig `yaml:"codex"`
		OpenSpec *AgentConfig `yaml:"openspec"`
	}

	var list []AgentProfileConfig
	if err := unmarshal(&list); err == nil {
		cloned := cloneAgentProfiles(list)
		c.Profiles = &cloned
		return nil
	}

	var l legacy
	if err := unmarshal(&l); err != nil {
		return err
	}
	c.Claude = l.Claude
	c.Codex = l.Codex
	c.OpenSpec = l.OpenSpec
	return nil
}

func cloneAgentProfiles(in []AgentProfileConfig) []AgentProfileConfig {
	if in == nil {
		return nil
	}
	out := make([]AgentProfileConfig, len(in))
	for i := range in {
		out[i] = in[i]
		if in[i].LaunchArgs != nil {
			out[i].LaunchArgs = append([]string(nil), in[i].LaunchArgs...)
		}
		if in[i].Env != nil {
			out[i].Env = make(map[string]string, len(in[i].Env))
			for k, v := range in[i].Env {
				out[i].Env[k] = v
			}
		}
	}
	return out
}
