package core

import "time"

// StageID uniquely identifies a Run stage.
type StageID string

const (
	StageRequirements StageID = "requirements"
	StageSetup        StageID = "setup"
	StageImplement    StageID = "implement"
	StageReview       StageID = "review"
	StageFixup        StageID = "fixup"
	StageTest         StageID = "test"
	StageMerge        StageID = "merge"
	StageCleanup      StageID = "cleanup"
)

var knownStageIDs = map[StageID]struct{}{
	StageRequirements: {},
	StageSetup:        {},
	StageImplement:    {},
	StageReview:       {},
	StageFixup:        {},
	StageTest:         {},
	StageMerge:        {},
	StageCleanup:      {},
}

func IsKnownStage(stage StageID) bool {
	_, ok := knownStageIDs[stage]
	return ok
}

// OnFailure defines what happens when a stage fails.
type OnFailure string

const (
	OnFailureRetry OnFailure = "retry"
	OnFailureSkip  OnFailure = "skip"
	OnFailureAbort OnFailure = "abort"
	OnFailureHuman OnFailure = "human"
)

// StageConfig holds the declarative configuration for a single Run stage.
type StageConfig struct {
	Name             StageID       `yaml:"name" json:"name"`
	Agent            string        `yaml:"agent" json:"agent"`
	Role             string        `yaml:"role" json:"role"`
	PromptTemplate   string        `yaml:"prompt_template" json:"prompt_template"`
	Timeout          time.Duration `yaml:"timeout" json:"timeout"`
	MaxRetries       int           `yaml:"max_retries" json:"max_retries"`
	RequireHuman     bool          `yaml:"require_human" json:"require_human"`
	OnFailure        OnFailure     `yaml:"on_failure" json:"on_failure"`
	ReuseSessionFrom StageID       `yaml:"reuse_session_from" json:"reuse_session_from,omitempty"`
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
	RunID      string            `json:"run_id"`
	StageName  StageID           `json:"stage_name"`
	Status     CheckpointStatus  `json:"status"`
	Artifacts  map[string]string `json:"artifacts"`
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt time.Time         `json:"finished_at"`
	AgentUsed  string            `json:"agent_used"`
	TokensUsed int               `json:"tokens_used"`
	RetryCount int               `json:"retry_count"`
	Error      string            `json:"error,omitempty"`
}
