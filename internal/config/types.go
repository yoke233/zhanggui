package config

import "time"

type Config struct {
	Agents    AgentsConfig    `yaml:"agents"`
	Spec      SpecConfig      `yaml:"spec"`
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
	Claude   *AgentConfig `yaml:"claude"`
	Codex    *AgentConfig `yaml:"codex"`
	OpenSpec *AgentConfig `yaml:"openspec"`
}

type AgentConfig struct {
	Plugin       *string   `yaml:"plugin"`
	Binary       *string   `yaml:"binary"`
	MaxTurns     *int      `yaml:"default_max_turns"`
	DefaultTools *[]string `yaml:"default_tools"`
	Model        *string   `yaml:"model"`
	Reasoning    *string   `yaml:"reasoning"`
	Sandbox      *string   `yaml:"sandbox"`
	Approval     *string   `yaml:"approval"`
}

type SpecConfig struct {
	Enabled   bool               `yaml:"enabled"`
	Provider  string             `yaml:"provider"`
	OnFailure string             `yaml:"on_failure"`
	OpenSpec  SpecOpenSpecConfig `yaml:"openspec"`
}

type SpecOpenSpecConfig struct {
	Binary string `yaml:"binary"`
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
	ReviewGatePlugin string             `yaml:"review_gate_plugin"`
	ReviewPanel      ReviewPanelConfig  `yaml:"review_panel"`
	DAGScheduler     DAGSchedulerConfig `yaml:"dag_scheduler"`
}

type ReviewPanelConfig struct {
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
	Enabled        bool   `yaml:"enabled"`
	Token          string `yaml:"token"`
	AppID          int64  `yaml:"app_id"`
	PrivateKeyPath string `yaml:"private_key_path"`
	InstallationID int64  `yaml:"installation_id"`
	WebhookSecret  string `yaml:"webhook_secret"`
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
	Agents    *AgentsLayer    `yaml:"agents"`
	Spec      *SpecLayer      `yaml:"spec"`
	Runtime   *RuntimeLayer   `yaml:"runtime"`
	Pipeline  *PipelineLayer  `yaml:"pipeline"`
	Scheduler *SchedulerLayer `yaml:"scheduler"`
	Secretary *SecretaryLayer `yaml:"secretary"`
	Server    *ServerLayer    `yaml:"server"`
	GitHub    *GitHubLayer    `yaml:"github"`
	Store     *StoreLayer     `yaml:"store"`
	Log       *LogLayer       `yaml:"log"`
}

type AgentsLayer struct {
	Claude   *AgentConfig `yaml:"claude"`
	Codex    *AgentConfig `yaml:"codex"`
	OpenSpec *AgentConfig `yaml:"openspec"`
}

type SpecLayer struct {
	Enabled   *bool              `yaml:"enabled"`
	Provider  *string            `yaml:"provider"`
	OnFailure *string            `yaml:"on_failure"`
	OpenSpec  *SpecOpenSpecLayer `yaml:"openspec"`
}

type SpecOpenSpecLayer struct {
	Binary *string `yaml:"binary"`
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
	ReviewGatePlugin *string            `yaml:"review_gate_plugin"`
	ReviewPanel      *ReviewPanelLayer  `yaml:"review_panel"`
	DAGScheduler     *DAGSchedulerLayer `yaml:"dag_scheduler"`
}

type ReviewPanelLayer struct {
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
	Enabled        *bool   `yaml:"enabled"`
	Token          *string `yaml:"token"`
	AppID          *int64  `yaml:"app_id"`
	PrivateKeyPath *string `yaml:"private_key_path"`
	InstallationID *int64  `yaml:"installation_id"`
	WebhookSecret  *string `yaml:"webhook_secret"`
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
