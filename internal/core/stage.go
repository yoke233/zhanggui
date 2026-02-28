package core

import "time"

// StageID uniquely identifies a pipeline stage.
type StageID string

const (
	StageRequirements  StageID = "requirements"
	StageSpecGen       StageID = "spec_gen"
	StageSpecReview    StageID = "spec_review"
	StageWorktreeSetup StageID = "worktree_setup"
	StageImplement     StageID = "implement"
	StageCodeReview    StageID = "code_review"
	StageFixup         StageID = "fixup"
	StageE2ETest       StageID = "e2e_test"
	StageMerge         StageID = "merge"
	StageCleanup       StageID = "cleanup"
)

// OnFailure defines what happens when a stage fails.
type OnFailure string

const (
	OnFailureRetry OnFailure = "retry"
	OnFailureSkip  OnFailure = "skip"
	OnFailureAbort OnFailure = "abort"
	OnFailureHuman OnFailure = "human"
)

// StageConfig holds the declarative configuration for a single pipeline stage.
type StageConfig struct {
	Name           StageID       `json:"name"`
	Agent          string        `json:"agent"`
	PromptTemplate string        `json:"prompt_template"`
	Timeout        time.Duration `json:"timeout"`
	MaxRetries     int           `json:"max_retries"`
	RequireHuman   bool          `json:"require_human"`
	OnFailure      OnFailure     `json:"on_failure"`
	DependsOn      []StageID     `json:"depends_on,omitempty"`
	Condition      string        `json:"condition,omitempty"`
}

// CheckpointStatus represents the completion state of a stage checkpoint.
type CheckpointStatus string

const (
	CheckpointInProgress  CheckpointStatus = "in_progress"
	CheckpointSuccess     CheckpointStatus = "success"
	CheckpointFailed      CheckpointStatus = "failed"
	CheckpointSkipped     CheckpointStatus = "skipped"
	CheckpointInvalidated CheckpointStatus = "invalidated"
)

// Checkpoint records the execution state of a completed (or in-flight) stage.
type Checkpoint struct {
	PipelineID string            `json:"pipeline_id"`
	StageName  StageID           `json:"stage_name"`
	Status     CheckpointStatus  `json:"status"`
	Artifacts  map[string]string `json:"artifacts,omitempty"`
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt time.Time         `json:"finished_at,omitempty"`
	AgentUsed  string            `json:"agent_used,omitempty"`
	TokensUsed int               `json:"tokens_used,omitempty"`
	RetryCount int               `json:"retry_count"`
}
