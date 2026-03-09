package config

import (
	"fmt"
	"time"
)

// Duration wraps time.Duration with TOML-friendly text marshaling.
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", string(b), err)
	}
	d.Duration = v
	return nil
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

type Config struct {
	Agents     AgentsConfig     `toml:"agents"       yaml:"agents"`
	Roles      []RoleConfig     `toml:"roles"        yaml:"roles"`
	RoleBinds  RoleBindings     `toml:"role_bindings" yaml:"role_bindings"`
	Run        RunConfig        `toml:"run"          yaml:"Run"`
	Scheduler  SchedulerConfig  `toml:"scheduler"    yaml:"scheduler"`
	TeamLeader TeamLeaderConfig `toml:"team_leader"  yaml:"team_leader"`
	A2A        A2AConfig        `toml:"a2a"          yaml:"a2a"`
	Server     ServerConfig     `toml:"server"       yaml:"server"`
	GitHub     GitHubConfig     `toml:"github"       yaml:"github"`
	Store      StoreConfig      `toml:"store"        yaml:"store"`
	Context    ContextConfig    `toml:"context"      yaml:"context"`
	Log        LogConfig        `toml:"log"          yaml:"log"`
	LLMFilter  LLMFilterConfig  `toml:"llm_filter"   yaml:"llm_filter"`
}

type LLMFilterConfig struct {
	Enabled  bool   `toml:"enabled"  yaml:"enabled"`
	Provider string `toml:"provider" yaml:"provider"` // anthropic / openai / local
	Model    string `toml:"model"    yaml:"model"`
}

type AgentsConfig struct {
	Claude   *AgentConfig         `toml:"claude"   yaml:"claude"`
	Codex    *AgentConfig         `toml:"codex"    yaml:"codex"`
	OpenSpec *AgentConfig         `toml:"openspec" yaml:"openspec"`
	Profiles []AgentProfileConfig `toml:"profiles" yaml:"-"`
}

type AgentConfig struct {
	Plugin          *string             `toml:"plugin"           yaml:"plugin"`
	Binary          *string             `toml:"binary"           yaml:"binary"`
	MaxTurns        *int                `toml:"default_max_turns" yaml:"default_max_turns"`
	DefaultTools    *[]string           `toml:"default_tools"    yaml:"default_tools"`
	Model           *string             `toml:"model"            yaml:"model"`
	Reasoning       *string             `toml:"reasoning"        yaml:"reasoning"`
	Sandbox         *string             `toml:"sandbox"          yaml:"sandbox"`
	Approval        *string             `toml:"approval"         yaml:"approval"`
	CapabilitiesMax *CapabilitiesConfig `toml:"capabilities_max" yaml:"capabilities_max"`
}

type RunConfig struct {
	DefaultTemplate   string   `toml:"default_template"   yaml:"default_template"`
	GlobalTimeout     Duration `toml:"global_timeout"     yaml:"global_timeout"`
	AutoInferTemplate bool     `toml:"auto_infer_template" yaml:"auto_infer_template"`
	MaxTotalRetries   int      `toml:"max_total_retries"  yaml:"max_total_retries"`
}

type SchedulerConfig struct {
	MaxGlobalAgents int            `toml:"max_global_agents" yaml:"max_global_agents"`
	MaxProjectRuns  int            `toml:"max_project_runs"  yaml:"max_project_Runs"`
	Watchdog        WatchdogConfig `toml:"watchdog"          yaml:"watchdog"`
}

type WatchdogConfig struct {
	Enabled       bool     `toml:"enabled"         yaml:"enabled"`
	Interval      Duration `toml:"interval"        yaml:"interval"`
	StuckRunTTL   Duration `toml:"stuck_run_ttl"   yaml:"stuck_run_ttl"`
	StuckMergeTTL Duration `toml:"stuck_merge_ttl" yaml:"stuck_merge_ttl"`
	QueueStaleTTL Duration `toml:"queue_stale_ttl" yaml:"queue_stale_ttl"`
}

type TeamLeaderConfig struct {
	ReviewGatePlugin   string                   `toml:"review_gate_plugin"  yaml:"review_gate_plugin"`
	ReviewOrchestrator ReviewOrchestratorConfig `toml:"review_orchestrator" yaml:"review_orchestrator"`
	DAGScheduler       DAGSchedulerConfig       `toml:"dag_scheduler"       yaml:"dag_scheduler"`
}

type ReviewOrchestratorConfig struct {
	MaxRounds int `toml:"max_rounds" yaml:"max_rounds"`
}

type DAGSchedulerConfig struct {
	MaxConcurrentTasks int `toml:"max_concurrent_tasks" yaml:"max_concurrent_tasks"`
}

type A2AConfig struct {
	Enabled bool   `toml:"enabled" yaml:"enabled"`
	Token   string `toml:"token"   yaml:"token"`
	Version string `toml:"version" yaml:"version"`
}

type ServerConfig struct {
	Host string `toml:"host" yaml:"host"`
	Port int    `toml:"port" yaml:"port"`
}

type GitHubConfig struct {
	Enabled             bool              `toml:"enabled"              yaml:"enabled"`
	Token               string            `toml:"token"                yaml:"token"`
	AppID               int64             `toml:"app_id"               yaml:"app_id"`
	PrivateKeyPath      string            `toml:"private_key_path"     yaml:"private_key_path"`
	InstallationID      int64             `toml:"installation_id"      yaml:"installation_id"`
	Owner               string            `toml:"owner"                yaml:"owner"`
	Repo                string            `toml:"repo"                 yaml:"repo"`
	WebhookSecret       string            `toml:"webhook_secret"       yaml:"webhook_secret"`
	WebhookEnabled      bool              `toml:"webhook_enabled"      yaml:"webhook_enabled"`
	PREnabled           bool              `toml:"pr_enabled"           yaml:"pr_enabled"`
	LabelMapping        map[string]string `toml:"label_mapping"        yaml:"label_mapping"`
	AuthorizedUsernames []string          `toml:"authorized_usernames" yaml:"authorized_usernames"`
	AutoTrigger         bool              `toml:"auto_trigger"         yaml:"auto_trigger"`
	AllowPATFallback    bool              `toml:"allow_pat_fallback"   yaml:"allow_pat_fallback"`
	PR                  GitHubPRConfig    `toml:"pr"                   yaml:"pr"`
}

type GitHubPRConfig struct {
	AutoCreate   bool     `toml:"auto_create"   yaml:"auto_create"`
	Draft        bool     `toml:"draft"         yaml:"draft"`
	AutoMerge    bool     `toml:"auto_merge"    yaml:"auto_merge"`
	Reviewers    []string `toml:"reviewers"     yaml:"reviewers"`
	Labels       []string `toml:"labels"        yaml:"labels"`
	BranchPrefix string   `toml:"branch_prefix" yaml:"branch_prefix"`
}

type StoreConfig struct {
	Driver string `toml:"driver" yaml:"driver"`
	Path   string `toml:"path"   yaml:"path"`
}

type ContextConfig struct {
	Provider string `toml:"provider" yaml:"provider"`
	Path     string `toml:"path"     yaml:"path"`
}

type LogConfig struct {
	Level      string `toml:"level"        yaml:"level"`
	File       string `toml:"file"         yaml:"file"`
	MaxSizeMB  int    `toml:"max_size_mb"  yaml:"max_size_mb"`
	MaxAgeDays int    `toml:"max_age_days" yaml:"max_age_days"`
}

// ConfigLayer 表示可选覆盖层。nil 字段表示"未设置"，用于多层配置继承合并。
type ConfigLayer struct {
	Agents     *AgentsLayer       `toml:"agents"        yaml:"agents"`
	Roles      *[]RoleConfig      `toml:"roles"         yaml:"roles"`
	RoleBinds  *RoleBindingsLayer `toml:"role_bindings"  yaml:"role_bindings"`
	Run        *RunLayer          `toml:"run"            yaml:"Run"`
	Scheduler  *SchedulerLayer    `toml:"scheduler"      yaml:"scheduler"`
	TeamLeader *TeamLeaderLayer   `toml:"team_leader"    yaml:"team_leader"`
	A2A        *A2ALayer          `toml:"a2a"            yaml:"a2a"`
	Server     *ServerLayer       `toml:"server"         yaml:"server"`
	GitHub     *GitHubLayer       `toml:"github"         yaml:"github"`
	Store      *StoreLayer        `toml:"store"          yaml:"store"`
	Context    *ContextLayer      `toml:"context"        yaml:"context"`
	Log        *LogLayer          `toml:"log"            yaml:"log"`
}

type AgentsLayer struct {
	Claude   *AgentConfig          `toml:"claude"   yaml:"claude"`
	Codex    *AgentConfig          `toml:"codex"    yaml:"codex"`
	OpenSpec *AgentConfig          `toml:"openspec" yaml:"openspec"`
	Profiles *[]AgentProfileConfig `toml:"profiles" yaml:"-"`
}

type RunLayer struct {
	DefaultTemplate   *string   `toml:"default_template"   yaml:"default_template"`
	GlobalTimeout     *Duration `toml:"global_timeout"     yaml:"global_timeout"`
	AutoInferTemplate *bool     `toml:"auto_infer_template" yaml:"auto_infer_template"`
	MaxTotalRetries   *int      `toml:"max_total_retries"  yaml:"max_total_retries"`
}

type SchedulerLayer struct {
	MaxGlobalAgents *int           `toml:"max_global_agents" yaml:"max_global_agents"`
	MaxProjectRuns  *int           `toml:"max_project_runs"  yaml:"max_project_Runs"`
	Watchdog        *WatchdogLayer `toml:"watchdog"          yaml:"watchdog"`
}

type WatchdogLayer struct {
	Enabled       *bool     `toml:"enabled"         yaml:"enabled"`
	Interval      *Duration `toml:"interval"        yaml:"interval"`
	StuckRunTTL   *Duration `toml:"stuck_run_ttl"   yaml:"stuck_run_ttl"`
	StuckMergeTTL *Duration `toml:"stuck_merge_ttl" yaml:"stuck_merge_ttl"`
	QueueStaleTTL *Duration `toml:"queue_stale_ttl" yaml:"queue_stale_ttl"`
}

type TeamLeaderLayer struct {
	ReviewGatePlugin   *string                  `toml:"review_gate_plugin"  yaml:"review_gate_plugin"`
	ReviewOrchestrator *ReviewOrchestratorLayer `toml:"review_orchestrator" yaml:"review_orchestrator"`
	DAGScheduler       *DAGSchedulerLayer       `toml:"dag_scheduler"       yaml:"dag_scheduler"`
}

type ReviewOrchestratorLayer struct {
	MaxRounds *int `toml:"max_rounds" yaml:"max_rounds"`
}

type DAGSchedulerLayer struct {
	MaxConcurrentTasks *int `toml:"max_concurrent_tasks" yaml:"max_concurrent_tasks"`
}

type A2ALayer struct {
	Enabled *bool   `toml:"enabled" yaml:"enabled"`
	Token   *string `toml:"token"   yaml:"token"`
	Version *string `toml:"version" yaml:"version"`
}

type ServerLayer struct {
	Host *string `toml:"host" yaml:"host"`
	Port *int    `toml:"port" yaml:"port"`
}

type GitHubLayer struct {
	Enabled             *bool              `toml:"enabled"              yaml:"enabled"`
	Token               *string            `toml:"token"                yaml:"token"`
	AppID               *int64             `toml:"app_id"               yaml:"app_id"`
	PrivateKeyPath      *string            `toml:"private_key_path"     yaml:"private_key_path"`
	InstallationID      *int64             `toml:"installation_id"      yaml:"installation_id"`
	Owner               *string            `toml:"owner"                yaml:"owner"`
	Repo                *string            `toml:"repo"                 yaml:"repo"`
	WebhookSecret       *string            `toml:"webhook_secret"       yaml:"webhook_secret"`
	WebhookEnabled      *bool              `toml:"webhook_enabled"      yaml:"webhook_enabled"`
	PREnabled           *bool              `toml:"pr_enabled"           yaml:"pr_enabled"`
	LabelMapping        *map[string]string `toml:"label_mapping"        yaml:"label_mapping"`
	AuthorizedUsernames *[]string          `toml:"authorized_usernames" yaml:"authorized_usernames"`
	AutoTrigger         *bool              `toml:"auto_trigger"         yaml:"auto_trigger"`
	AllowPATFallback    *bool              `toml:"allow_pat_fallback"   yaml:"allow_pat_fallback"`
	PR                  *GitHubPRLayer     `toml:"pr"                   yaml:"pr"`
}

type GitHubPRLayer struct {
	AutoCreate   *bool     `toml:"auto_create"   yaml:"auto_create"`
	Draft        *bool     `toml:"draft"         yaml:"draft"`
	AutoMerge    *bool     `toml:"auto_merge"    yaml:"auto_merge"`
	Reviewers    *[]string `toml:"reviewers"     yaml:"reviewers"`
	Labels       *[]string `toml:"labels"        yaml:"labels"`
	BranchPrefix *string   `toml:"branch_prefix" yaml:"branch_prefix"`
}

type StoreLayer struct {
	Driver *string `toml:"driver" yaml:"driver"`
	Path   *string `toml:"path"   yaml:"path"`
}

type ContextLayer struct {
	Provider *string `toml:"provider" yaml:"provider"`
	Path     *string `toml:"path"     yaml:"path"`
}

type LogLayer struct {
	Level      *string `toml:"level"        yaml:"level"`
	File       *string `toml:"file"         yaml:"file"`
	MaxSizeMB  *int    `toml:"max_size_mb"  yaml:"max_size_mb"`
	MaxAgeDays *int    `toml:"max_age_days" yaml:"max_age_days"`
}

type CapabilitiesConfig struct {
	FSRead   bool `toml:"fs_read"   yaml:"fs_read"`
	FSWrite  bool `toml:"fs_write"  yaml:"fs_write"`
	Terminal bool `toml:"terminal"  yaml:"terminal"`
}

type AgentProfileConfig struct {
	Name            string             `toml:"name"             yaml:"name"`
	LaunchCommand   string             `toml:"launch_command"   yaml:"launch_command"`
	LaunchArgs      []string           `toml:"launch_args"      yaml:"launch_args"`
	Env             map[string]string  `toml:"env"              yaml:"env"`
	CapabilitiesMax CapabilitiesConfig `toml:"capabilities_max" yaml:"capabilities_max"`
}

type RoleConfig struct {
	Name             string             `toml:"name"              yaml:"name"`
	Agent            string             `toml:"agent"             yaml:"agent"`
	PromptTemplate   string             `toml:"prompt_template"   yaml:"prompt_template"`
	Capabilities     CapabilitiesConfig `toml:"capabilities"      yaml:"capabilities"`
	Session          SessionConfig      `toml:"session"           yaml:"session"`
	PermissionPolicy []PermissionRule   `toml:"permission_policy" yaml:"permission_policy"`
	MCP              MCPConfig          `toml:"mcp"               yaml:"mcp"`
}

type SessionConfig struct {
	Reuse             bool `toml:"reuse"               yaml:"reuse"`
	PreferLoadSession bool `toml:"prefer_load_session" yaml:"prefer_load_session"`
	ResetPrompt       bool `toml:"reset_prompt"        yaml:"reset_prompt"`
	MaxTurns          int  `toml:"max_turns"           yaml:"max_turns"`
}

type PermissionRule struct {
	Pattern string `toml:"pattern" yaml:"pattern"`
	Action  string `toml:"action"  yaml:"action"`
	Scope   string `toml:"scope"   yaml:"scope"`
}

type MCPConfig struct {
	Enabled bool     `toml:"enabled" yaml:"enabled"`
	Tools   []string `toml:"tools"   yaml:"tools"`
}

type RoleBindings struct {
	TeamLeader         SingleRoleBinding  `toml:"team_leader"         yaml:"team_leader"`
	Run                RunRoleBindings    `toml:"run"                 yaml:"Run"`
	ReviewOrchestrator ReviewRoleBindings `toml:"review_orchestrator" yaml:"review_orchestrator"`
	PlanParser         SingleRoleBinding  `toml:"plan_parser"         yaml:"plan_parser"`
}

type SingleRoleBinding struct {
	Role string `toml:"role" yaml:"role"`
}

type RunRoleBindings struct {
	StageRoles map[string]string `toml:"stage_roles" yaml:"stage_roles"`
}

type ReviewRoleBindings struct {
	Reviewers  map[string]string `toml:"reviewers"  yaml:"reviewers"`
	Aggregator string            `toml:"aggregator" yaml:"aggregator"`
}

type RoleBindingsLayer struct {
	TeamLeader         *SingleRoleBindingLayer  `toml:"team_leader"         yaml:"team_leader"`
	Run                *RunRoleBindingsLayer    `toml:"run"                 yaml:"Run"`
	ReviewOrchestrator *ReviewRoleBindingsLayer `toml:"review_orchestrator" yaml:"review_orchestrator"`
	PlanParser         *SingleRoleBindingLayer  `toml:"plan_parser"         yaml:"plan_parser"`
}

type SingleRoleBindingLayer struct {
	Role *string `toml:"role" yaml:"role"`
}

type RunRoleBindingsLayer struct {
	StageRoles *map[string]string `toml:"stage_roles" yaml:"stage_roles"`
}

type ReviewRoleBindingsLayer struct {
	Reviewers  *map[string]string `toml:"reviewers"  yaml:"reviewers"`
	Aggregator *string            `toml:"aggregator" yaml:"aggregator"`
}

// UnmarshalYAML supports legacy YAML dual-format for agents (list vs map).
func (c *AgentsConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type legacy struct {
		Claude   *AgentConfig `yaml:"claude"`
		Codex    *AgentConfig `yaml:"codex"`
		OpenSpec *AgentConfig `yaml:"openspec"`
	}

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

// UnmarshalYAML supports legacy YAML dual-format for agents layer.
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
