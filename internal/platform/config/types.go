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
	Run       RunConfig       `toml:"run"        yaml:"Run"`
	Scheduler SchedulerConfig `toml:"scheduler"  yaml:"scheduler"`
	Server    ServerConfig    `toml:"server"     yaml:"server"`
	GitHub    GitHubConfig    `toml:"github"     yaml:"github"`
	Store     StoreConfig     `toml:"store"      yaml:"store"`
	Context   ContextConfig   `toml:"context"    yaml:"context"`
	Log       LogConfig       `toml:"log"        yaml:"log"`
	Audit     AuditConfig     `toml:"audit"      yaml:"audit"`
	LLMFilter LLMFilterConfig `toml:"llm_filter" yaml:"llm_filter"`
	Runtime   RuntimeConfig   `toml:"runtime"    yaml:"runtime"`
}

type AuditConfig struct {
	Enabled        bool            `toml:"enabled"         yaml:"enabled"`
	FallbackDir    string          `toml:"fallback_dir"    yaml:"fallback_dir"`
	RetentionDays  int             `toml:"retention_days"  yaml:"retention_days"`
	RedactionLevel string          `toml:"redaction_level" yaml:"redaction_level"`
	OTLP           AuditOTLPConfig `toml:"otlp"            yaml:"otlp"`
}

type AuditOTLPConfig struct {
	Enabled  bool              `toml:"enabled" yaml:"enabled"`
	Endpoint string            `toml:"endpoint" yaml:"endpoint"`
	Headers  map[string]string `toml:"headers" yaml:"headers"`
}

type LLMFilterConfig struct {
	Enabled  bool   `toml:"enabled"  yaml:"enabled"`
	Provider string `toml:"provider" yaml:"provider"` // anthropic / openai / local
	Model    string `toml:"model"    yaml:"model"`
}

// RuntimeConfig holds configuration for the runtime engine.
type RuntimeConfig struct {
	// MockExecutor makes runtime step execution use an in-process stub instead of ACP agents.
	// Useful for smoke tests and environments without LLM credentials.
	MockExecutor   bool                        `toml:"mock_executor" yaml:"mock_executor" json:"mock_executor"`
	Collector      RuntimeCollectorConfig      `toml:"collector" yaml:"collector" json:"collector"`
	LLM            RuntimeLLMConfig            `toml:"llm" yaml:"llm" json:"llm"`
	Sandbox        RuntimeSandboxConfig        `toml:"sandbox"   yaml:"sandbox" json:"sandbox"`
	Agents         RuntimeAgentsConfig         `toml:"agents"    yaml:"agents" json:"agents"`
	MCP            RuntimeMCPConfig            `toml:"mcp"       yaml:"mcp" json:"mcp"`
	Prompts        RuntimePromptsConfig        `toml:"prompts"   yaml:"prompts" json:"prompts"`
	SessionManager RuntimeSessionManagerConfig `toml:"session_manager" yaml:"session_manager" json:"session_manager"`
	ExecutionProbe RuntimeExecutionProbeConfig `toml:"execution_probe" yaml:"execution_probe" json:"execution_probe"`
	Cron           RuntimeCronConfig           `toml:"cron"            yaml:"cron" json:"cron"`
	Inspection     RuntimeInspectionConfig     `toml:"inspection"      yaml:"inspection" json:"inspection"`
}

// RuntimeInspectionConfig configures the self-evolving inspection system.
type RuntimeInspectionConfig struct {
	Enabled   bool     `toml:"enabled"        yaml:"enabled" json:"enabled"`
	Interval  Duration `toml:"interval"       yaml:"interval" json:"interval"`             // how often to run (default "24h")
	LookbackH int      `toml:"lookback_hours" yaml:"lookback_hours" json:"lookback_hours"` // hours of data to inspect (default 24)
}

// RuntimeCronConfig configures the cron trigger for scheduled flows.
type RuntimeCronConfig struct {
	Enabled  bool     `toml:"enabled"  yaml:"enabled" json:"enabled"`
	Interval Duration `toml:"interval" yaml:"interval" json:"interval"` // scan interval (default "1m")
}

// RuntimeSessionManagerConfig configures the session manager mode.
type RuntimeSessionManagerConfig struct {
	// Mode selects the session manager implementation: "local" (default) or "nats".
	// Local mode runs agents in-process with no external dependencies.
	// NATS mode uses JetStream for crash-resilient, distributed execution.
	Mode string `toml:"mode" yaml:"mode" json:"mode"`

	// ServerID uniquely identifies this server instance in multi-server deployments.
	// Used as a prefix in prompt IDs to avoid collisions. Auto-generated if empty.
	ServerID string `toml:"server_id" yaml:"server_id" json:"server_id"`

	// NATS holds configuration for the NATS session manager (only used when Mode == "nats").
	NATS RuntimeNATSConfig `toml:"nats" yaml:"nats" json:"nats"`
}

// RuntimeExecutionProbeConfig configures watchdog-driven execution probes.
type RuntimeExecutionProbeConfig struct {
	Enabled     bool     `toml:"enabled" yaml:"enabled" json:"enabled"`
	Interval    Duration `toml:"interval" yaml:"interval" json:"interval"`
	After       Duration `toml:"after" yaml:"after" json:"after"`
	IdleAfter   Duration `toml:"idle_after" yaml:"idle_after" json:"idle_after"`
	Timeout     Duration `toml:"timeout" yaml:"timeout" json:"timeout"`
	MaxAttempts int      `toml:"max_attempts" yaml:"max_attempts" json:"max_attempts"`
}

// RuntimeNATSConfig configures the NATS connection and JetStream settings.
type RuntimeNATSConfig struct {
	// URL is the NATS server URL (e.g., "nats://localhost:4222").
	// The current implementation requires an external NATS server.
	URL string `toml:"url" yaml:"url" json:"url"`

	// Embedded is reserved for a future in-process NATS mode and is not wired yet.
	Embedded bool `toml:"embedded" yaml:"embedded" json:"embedded"`

	// EmbeddedDataDir is reserved for the future embedded NATS mode.
	EmbeddedDataDir string `toml:"embedded_data_dir" yaml:"embedded_data_dir" json:"embedded_data_dir"`

	// StreamPrefix is the NATS JetStream stream name prefix. Default: "aiworkflow".
	StreamPrefix string `toml:"stream_prefix" yaml:"stream_prefix" json:"stream_prefix"`
}

// RuntimeSandboxConfig configures per-ACP-process sandbox isolation.
type RuntimeSandboxConfig struct {
	Enabled  bool                 `toml:"enabled"  yaml:"enabled"`
	Provider string               `toml:"provider" yaml:"provider"`
	LiteBox  RuntimeLiteBoxConfig `toml:"litebox"  yaml:"litebox"`
	Docker   RuntimeDockerConfig  `toml:"docker"   yaml:"docker"`
}

type RuntimeLiteBoxConfig struct {
	BridgeCommand string   `toml:"bridge_command" yaml:"bridge_command"`
	BridgeArgs    []string `toml:"bridge_args"    yaml:"bridge_args"`
	RunnerPath    string   `toml:"runner_path"    yaml:"runner_path"`
	RunnerArgs    []string `toml:"runner_args"    yaml:"runner_args"`
}

type RuntimeDockerConfig struct {
	Command        string   `toml:"command"           yaml:"command"`
	Image          string   `toml:"image"             yaml:"image"`
	RunArgs        []string `toml:"run_args"          yaml:"run_args"`
	CPUs           string   `toml:"cpus"              yaml:"cpus"`
	Memory         string   `toml:"memory"            yaml:"memory"`
	MemorySwap     string   `toml:"memory_swap"       yaml:"memory_swap"`
	PidsLimit      string   `toml:"pids_limit"        yaml:"pids_limit"`
	Network        string   `toml:"network"           yaml:"network"`
	ReadOnlyRootFS bool     `toml:"read_only_rootfs"  yaml:"read_only_rootfs"`
	Tmpfs          []string `toml:"tmpfs"             yaml:"tmpfs"`
}

// RuntimePromptsConfig stores user-maintained prompt templates for runtime runtime behaviors.
// These prompts are used as incremental messages when reusing ACP sessions, so they
// should be concise to preserve prompt caching and reuse existing context.
type RuntimePromptsConfig struct {
	ReworkFollowup        string                         `toml:"rework_followup"        yaml:"rework_followup"`
	ContinueFollowup      string                         `toml:"continue_followup"      yaml:"continue_followup"`
	PRImplementObjective  string                         `toml:"pr_implement_objective" yaml:"pr_implement_objective"`
	PRGateObjective       string                         `toml:"pr_gate_objective"      yaml:"pr_gate_objective"`
	PRMergeReworkFeedback string                         `toml:"pr_merge_rework_feedback" yaml:"pr_merge_rework_feedback"`
	PRProviders           RuntimePRPromptProvidersConfig `toml:"pr_providers" yaml:"pr_providers"`
}

type RuntimePRPromptProvidersConfig struct {
	GitHub RuntimePRProviderPromptConfig `toml:"github" yaml:"github"`
	CodeUp RuntimePRProviderPromptConfig `toml:"codeup" yaml:"codeup"`
	GitLab RuntimePRProviderPromptConfig `toml:"gitlab" yaml:"gitlab"`
}

type RuntimePRProviderPromptConfig struct {
	ImplementObjective  string                          `toml:"implement_objective" yaml:"implement_objective"`
	GateObjective       string                          `toml:"gate_objective" yaml:"gate_objective"`
	MergeReworkFeedback string                          `toml:"merge_rework_feedback" yaml:"merge_rework_feedback"`
	MergeStates         RuntimePRMergeStatePromptConfig `toml:"merge_states" yaml:"merge_states"`
}

type RuntimePRMergeStatePromptConfig struct {
	Default  string `toml:"default" yaml:"default"`
	Dirty    string `toml:"dirty" yaml:"dirty"`
	Blocked  string `toml:"blocked" yaml:"blocked"`
	Behind   string `toml:"behind" yaml:"behind"`
	Unstable string `toml:"unstable" yaml:"unstable"`
	Draft    string `toml:"draft" yaml:"draft"`
}

// RuntimeAgentsConfig defines agent drivers and profiles for the runtime engine.
type RuntimeAgentsConfig struct {
	Drivers  []RuntimeDriverConfig  `toml:"drivers"  yaml:"drivers" json:"drivers"`
	Profiles []RuntimeProfileConfig `toml:"profiles" yaml:"profiles" json:"profiles"`
}

// RuntimeMCPConfig defines MCP servers and per-profile bindings for the runtime engine.
type RuntimeMCPConfig struct {
	Servers         []RuntimeMCPServerConfig         `toml:"servers"          yaml:"servers" json:"servers"`
	ProfileBindings []RuntimeMCPProfileBindingConfig `toml:"profile_bindings" yaml:"profile_bindings" json:"profile_bindings"`
}

type RuntimeMCPServerConfig struct {
	ID            string            `toml:"id"              yaml:"id" json:"id"`
	Name          string            `toml:"name"            yaml:"name" json:"name"`
	Kind          string            `toml:"kind"            yaml:"kind" json:"kind"`
	Transport     string            `toml:"transport"       yaml:"transport" json:"transport"`
	Endpoint      string            `toml:"endpoint"        yaml:"endpoint" json:"endpoint"`
	Command       string            `toml:"command"         yaml:"command" json:"command"`
	Args          []string          `toml:"args"            yaml:"args" json:"args"`
	Env           map[string]string `toml:"env"             yaml:"env" json:"env"`
	AuthSecretRef string            `toml:"auth_secret_ref" yaml:"auth_secret_ref" json:"auth_secret_ref"`
	Enabled       bool              `toml:"enabled"         yaml:"enabled" json:"enabled"`
}

type RuntimeMCPProfileBindingConfig struct {
	Profile  string   `toml:"profile"   yaml:"profile" json:"profile"`
	Server   string   `toml:"server"    yaml:"server" json:"server"`
	Enabled  bool     `toml:"enabled"   yaml:"enabled" json:"enabled"`
	ToolMode string   `toml:"tool_mode" yaml:"tool_mode" json:"tool_mode"`
	Tools    []string `toml:"tools"     yaml:"tools" json:"tools"`
}

// RuntimeDriverConfig defines an ACP agent driver (process launch configuration).
type RuntimeDriverConfig struct {
	ID              string             `toml:"id"               yaml:"id" json:"id"`
	LaunchCommand   string             `toml:"launch_command"   yaml:"launch_command" json:"launch_command"`
	LaunchArgs      []string           `toml:"launch_args"      yaml:"launch_args" json:"launch_args"`
	SandboxArgs     []string           `toml:"sandbox_args"     yaml:"sandbox_args" json:"sandbox_args"`
	Env             map[string]string  `toml:"env"              yaml:"env" json:"env"`
	CapabilitiesMax CapabilitiesConfig `toml:"capabilities_max" yaml:"capabilities_max" json:"capabilities_max"`
}

// RuntimeProfileConfig defines an agent profile (role instance) for the runtime engine.
type RuntimeProfileConfig struct {
	ID             string               `toml:"id"              yaml:"id" json:"id"`
	Name           string               `toml:"name"            yaml:"name" json:"name"`
	Driver         string               `toml:"driver"          yaml:"driver" json:"driver"`
	Role           string               `toml:"role"            yaml:"role" json:"role"`
	Capabilities   []string             `toml:"capabilities"    yaml:"capabilities" json:"capabilities"`
	ActionsAllowed []string             `toml:"actions_allowed" yaml:"actions_allowed" json:"actions_allowed"`
	PromptTemplate string               `toml:"prompt_template" yaml:"prompt_template" json:"prompt_template"`
	Skills         []string             `toml:"skills"          yaml:"skills" json:"skills"`
	Session        RuntimeSessionConfig `toml:"session"         yaml:"session" json:"session"`
	MCP            MCPConfig            `toml:"mcp"             yaml:"mcp" json:"mcp"`
}

// RuntimeSessionConfig configures session management for a runtime profile.
type RuntimeSessionConfig struct {
	Reuse              bool     `toml:"reuse"               yaml:"reuse" json:"reuse"`
	MaxTurns           int      `toml:"max_turns"           yaml:"max_turns" json:"max_turns"`
	IdleTTL            Duration `toml:"idle_ttl"            yaml:"idle_ttl" json:"idle_ttl"`
	ThreadBootTemplate string   `toml:"thread_boot_template" yaml:"thread_boot_template" json:"thread_boot_template,omitempty"`
	MaxContextTokens   int64    `toml:"max_context_tokens"  yaml:"max_context_tokens" json:"max_context_tokens,omitempty"`
	ContextWarnRatio   float64  `toml:"context_warn_ratio"  yaml:"context_warn_ratio" json:"context_warn_ratio,omitempty"`
}

// RuntimeCollectorConfig configures the runtime metadata collector.
type RuntimeCollectorConfig struct {
	// MaxRetries controls how many additional attempts the collector makes
	// for transient OpenAI API errors. 0 means no retry.
	MaxRetries int `toml:"max_retries" yaml:"max_retries"`
}

// RuntimeLLMConfig stores editable LLM provider endpoints in config.toml.
type RuntimeLLMConfig struct {
	DefaultConfigID string                  `toml:"default_config_id" yaml:"default_config_id" json:"default_config_id"`
	Configs         []RuntimeLLMEntryConfig `toml:"configs"           yaml:"configs" json:"configs"`
}

type RuntimeLLMEntryConfig struct {
	ID                   string  `toml:"id"                     yaml:"id" json:"id"`
	Type                 string  `toml:"type"                   yaml:"type" json:"type"`
	BaseURL              string  `toml:"base_url"               yaml:"base_url" json:"base_url"`
	APIKey               string  `toml:"api_key"                yaml:"api_key" json:"api_key"`
	Model                string  `toml:"model"                  yaml:"model" json:"model"`
	Temperature          float64 `toml:"temperature"            yaml:"temperature" json:"temperature"`
	MaxOutputTokens      int64   `toml:"max_output_tokens"      yaml:"max_output_tokens" json:"max_output_tokens"`
	ReasoningEffort      string  `toml:"reasoning_effort"       yaml:"reasoning_effort" json:"reasoning_effort"`
	ThinkingBudgetTokens int64   `toml:"thinking_budget_tokens" yaml:"thinking_budget_tokens" json:"thinking_budget_tokens"`
}

type RunConfig struct {
	DefaultTemplate   string   `toml:"default_template"   yaml:"default_template"`
	GlobalTimeout     Duration `toml:"global_timeout"     yaml:"global_timeout"`
	AutoInferTemplate bool     `toml:"auto_infer_template" yaml:"auto_infer_template"`
	MaxTotalRetries   int      `toml:"max_total_retries"  yaml:"max_total_retries"`
}

type SchedulerConfig struct {
	MaxGlobalAgents int            `toml:"max_global_agents" yaml:"max_global_agents"`
	MaxProjectRuns  int            `toml:"max_project_runs"  yaml:"max_project_runs"`
	Watchdog        WatchdogConfig `toml:"watchdog"          yaml:"watchdog"`
}

type WatchdogConfig struct {
	Enabled       bool     `toml:"enabled"         yaml:"enabled"`
	Interval      Duration `toml:"interval"        yaml:"interval"`
	StuckRunTTL   Duration `toml:"stuck_run_ttl"   yaml:"stuck_run_ttl"`
	StuckMergeTTL Duration `toml:"stuck_merge_ttl" yaml:"stuck_merge_ttl"`
	QueueStaleTTL Duration `toml:"queue_stale_ttl" yaml:"queue_stale_ttl"`
}

type ServerConfig struct {
	Host         string `toml:"host"          yaml:"host"`
	Port         int    `toml:"port"          yaml:"port"`
	AuthRequired *bool  `toml:"auth_required" yaml:"auth_required"`
}

// IsAuthRequired returns whether token authentication is enabled.
// Defaults to true if not explicitly set.
func (s ServerConfig) IsAuthRequired() bool {
	if s.AuthRequired == nil {
		return true
	}
	return *s.AuthRequired
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
	Run       *RunLayer       `toml:"run"       yaml:"Run"`
	Scheduler *SchedulerLayer `toml:"scheduler" yaml:"scheduler"`
	Server    *ServerLayer    `toml:"server"    yaml:"server"`
	GitHub    *GitHubLayer    `toml:"github"    yaml:"github"`
	Store     *StoreLayer     `toml:"store"     yaml:"store"`
	Context   *ContextLayer   `toml:"context"   yaml:"context"`
	Log       *LogLayer       `toml:"log"       yaml:"log"`
	Audit     *AuditLayer     `toml:"audit"     yaml:"audit"`
	Runtime   *RuntimeLayer   `toml:"runtime"   yaml:"runtime"`
}

type AuditLayer struct {
	Enabled        *bool           `toml:"enabled" yaml:"enabled"`
	FallbackDir    *string         `toml:"fallback_dir" yaml:"fallback_dir"`
	RetentionDays  *int            `toml:"retention_days" yaml:"retention_days"`
	RedactionLevel *string         `toml:"redaction_level" yaml:"redaction_level"`
	OTLP           *AuditOTLPLayer `toml:"otlp" yaml:"otlp"`
}

type AuditOTLPLayer struct {
	Enabled  *bool              `toml:"enabled" yaml:"enabled"`
	Endpoint *string            `toml:"endpoint" yaml:"endpoint"`
	Headers  *map[string]string `toml:"headers" yaml:"headers"`
}

type RuntimeLayer struct {
	Collector      *RuntimeCollectorLayer      `toml:"collector"       yaml:"collector"`
	LLM            *RuntimeLLMLayer            `toml:"llm"             yaml:"llm"`
	Sandbox        *RuntimeSandboxLayer        `toml:"sandbox"         yaml:"sandbox"`
	Agents         *RuntimeAgentsLayerCfg      `toml:"agents"          yaml:"agents"`
	MCP            *RuntimeMCPLayer            `toml:"mcp"             yaml:"mcp"`
	Prompts        *RuntimePromptsLayer        `toml:"prompts"         yaml:"prompts"`
	SessionManager *RuntimeSessionManagerLayer `toml:"session_manager" yaml:"session_manager"`
	ExecutionProbe *RuntimeExecutionProbeLayer `toml:"execution_probe" yaml:"execution_probe"`
	Cron           *RuntimeCronLayer           `toml:"cron"            yaml:"cron"`
}

type RuntimeSessionManagerLayer struct {
	Mode     *string           `toml:"mode"      yaml:"mode"`
	ServerID *string           `toml:"server_id" yaml:"server_id"`
	NATS     *RuntimeNATSLayer `toml:"nats"      yaml:"nats"`
}

type RuntimeExecutionProbeLayer struct {
	Enabled     *bool     `toml:"enabled" yaml:"enabled"`
	Interval    *Duration `toml:"interval" yaml:"interval"`
	After       *Duration `toml:"after" yaml:"after"`
	IdleAfter   *Duration `toml:"idle_after" yaml:"idle_after"`
	Timeout     *Duration `toml:"timeout" yaml:"timeout"`
	MaxAttempts *int      `toml:"max_attempts" yaml:"max_attempts"`
}

type RuntimeCronLayer struct {
	Enabled  *bool     `toml:"enabled" yaml:"enabled"`
	Interval *Duration `toml:"interval" yaml:"interval"`
}

type RuntimeNATSLayer struct {
	URL             *string `toml:"url"               yaml:"url"`
	Embedded        *bool   `toml:"embedded"          yaml:"embedded"`
	EmbeddedDataDir *string `toml:"embedded_data_dir" yaml:"embedded_data_dir"`
	StreamPrefix    *string `toml:"stream_prefix"     yaml:"stream_prefix"`
}

type RuntimeSandboxLayer struct {
	Enabled  *bool                `toml:"enabled"  yaml:"enabled"`
	Provider *string              `toml:"provider" yaml:"provider"`
	LiteBox  *RuntimeLiteBoxLayer `toml:"litebox"  yaml:"litebox"`
	Docker   *RuntimeDockerLayer  `toml:"docker"   yaml:"docker"`
}

type RuntimeLiteBoxLayer struct {
	BridgeCommand *string   `toml:"bridge_command" yaml:"bridge_command"`
	BridgeArgs    *[]string `toml:"bridge_args"    yaml:"bridge_args"`
	RunnerPath    *string   `toml:"runner_path"    yaml:"runner_path"`
	RunnerArgs    *[]string `toml:"runner_args"    yaml:"runner_args"`
}

type RuntimeDockerLayer struct {
	Command        *string   `toml:"command" yaml:"command"`
	Image          *string   `toml:"image" yaml:"image"`
	RunArgs        *[]string `toml:"run_args" yaml:"run_args"`
	CPUs           *string   `toml:"cpus" yaml:"cpus"`
	Memory         *string   `toml:"memory" yaml:"memory"`
	MemorySwap     *string   `toml:"memory_swap" yaml:"memory_swap"`
	PidsLimit      *string   `toml:"pids_limit" yaml:"pids_limit"`
	Network        *string   `toml:"network" yaml:"network"`
	ReadOnlyRootFS *bool     `toml:"read_only_rootfs" yaml:"read_only_rootfs"`
	Tmpfs          *[]string `toml:"tmpfs" yaml:"tmpfs"`
}

type RuntimePromptsLayer struct {
	ReworkFollowup        *string                        `toml:"rework_followup"         yaml:"rework_followup"`
	ContinueFollowup      *string                        `toml:"continue_followup"       yaml:"continue_followup"`
	PRImplementObjective  *string                        `toml:"pr_implement_objective"  yaml:"pr_implement_objective"`
	PRGateObjective       *string                        `toml:"pr_gate_objective"       yaml:"pr_gate_objective"`
	PRMergeReworkFeedback *string                        `toml:"pr_merge_rework_feedback" yaml:"pr_merge_rework_feedback"`
	PRProviders           *RuntimePRPromptProvidersLayer `toml:"pr_providers" yaml:"pr_providers"`
}

type RuntimePRPromptProvidersLayer struct {
	GitHub *RuntimePRProviderPromptLayer `toml:"github" yaml:"github"`
	CodeUp *RuntimePRProviderPromptLayer `toml:"codeup" yaml:"codeup"`
	GitLab *RuntimePRProviderPromptLayer `toml:"gitlab" yaml:"gitlab"`
}

type RuntimePRProviderPromptLayer struct {
	ImplementObjective  *string                         `toml:"implement_objective" yaml:"implement_objective"`
	GateObjective       *string                         `toml:"gate_objective" yaml:"gate_objective"`
	MergeReworkFeedback *string                         `toml:"merge_rework_feedback" yaml:"merge_rework_feedback"`
	MergeStates         *RuntimePRMergeStatePromptLayer `toml:"merge_states" yaml:"merge_states"`
}

type RuntimePRMergeStatePromptLayer struct {
	Default  *string `toml:"default" yaml:"default"`
	Dirty    *string `toml:"dirty" yaml:"dirty"`
	Blocked  *string `toml:"blocked" yaml:"blocked"`
	Behind   *string `toml:"behind" yaml:"behind"`
	Unstable *string `toml:"unstable" yaml:"unstable"`
	Draft    *string `toml:"draft" yaml:"draft"`
}

type RuntimeAgentsLayerCfg struct {
	Drivers  *[]RuntimeDriverConfig  `toml:"drivers"  yaml:"drivers"`
	Profiles *[]RuntimeProfileConfig `toml:"profiles" yaml:"profiles"`
}

type RuntimeMCPLayer struct {
	Servers         *[]RuntimeMCPServerConfig         `toml:"servers"          yaml:"servers"`
	ProfileBindings *[]RuntimeMCPProfileBindingConfig `toml:"profile_bindings" yaml:"profile_bindings"`
}

type RuntimeCollectorLayer struct {
	MaxRetries *int `toml:"max_retries" yaml:"max_retries"`
}

type RuntimeLLMLayer struct {
	DefaultConfigID *string                  `toml:"default_config_id" yaml:"default_config_id"`
	Configs         *[]RuntimeLLMEntryConfig `toml:"configs"           yaml:"configs"`
}

type RunLayer struct {
	DefaultTemplate   *string   `toml:"default_template"   yaml:"default_template"`
	GlobalTimeout     *Duration `toml:"global_timeout"     yaml:"global_timeout"`
	AutoInferTemplate *bool     `toml:"auto_infer_template" yaml:"auto_infer_template"`
	MaxTotalRetries   *int      `toml:"max_total_retries"  yaml:"max_total_retries"`
}

type SchedulerLayer struct {
	MaxGlobalAgents *int           `toml:"max_global_agents" yaml:"max_global_agents"`
	MaxProjectRuns  *int           `toml:"max_project_runs"  yaml:"max_project_runs"`
	Watchdog        *WatchdogLayer `toml:"watchdog"          yaml:"watchdog"`
}

type WatchdogLayer struct {
	Enabled       *bool     `toml:"enabled"         yaml:"enabled"`
	Interval      *Duration `toml:"interval"        yaml:"interval"`
	StuckRunTTL   *Duration `toml:"stuck_run_ttl"   yaml:"stuck_run_ttl"`
	StuckMergeTTL *Duration `toml:"stuck_merge_ttl" yaml:"stuck_merge_ttl"`
	QueueStaleTTL *Duration `toml:"queue_stale_ttl" yaml:"queue_stale_ttl"`
}

type ServerLayer struct {
	Host         *string `toml:"host"          yaml:"host"`
	Port         *int    `toml:"port"          yaml:"port"`
	AuthRequired *bool   `toml:"auth_required" yaml:"auth_required"`
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
	FSRead   bool `toml:"fs_read"   yaml:"fs_read" json:"fs_read"`
	FSWrite  bool `toml:"fs_write"  yaml:"fs_write" json:"fs_write"`
	Terminal bool `toml:"terminal"  yaml:"terminal" json:"terminal"`
}

type MCPConfig struct {
	Enabled bool     `toml:"enabled" yaml:"enabled" json:"enabled"`
	Tools   []string `toml:"tools"   yaml:"tools" json:"tools"`
}
