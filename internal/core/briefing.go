package core

import "time"

// Briefing is the task sheet assembled by the engine for an Agent.
// It is not a prompt — it is structured input material for prompt construction.
type Briefing struct {
	ID          int64        `json:"id"`
	StepID      int64        `json:"step_id"`
	Objective   string       `json:"objective"`              // what to do
	ContextRefs []ContextRef `json:"context_refs,omitempty"` // accessible context references
	Constraints []string     `json:"constraints,omitempty"`  // restrictions
	CreatedAt   time.Time    `json:"created_at"`
}

// ContextRefType classifies the kind of context reference.
type ContextRefType string

const (
	CtxIssueSummary     ContextRefType = "issue_summary"
	CtxProjectBrief     ContextRefType = "project_brief"
	CtxUpstreamArtifact ContextRefType = "upstream_artifact"
	CtxAgentMemory      ContextRefType = "agent_memory"
	CtxFeatureManifest  ContextRefType = "feature_manifest"
)

// ContextRef points to a piece of context the Agent can access.
type ContextRef struct {
	Type   ContextRefType `json:"type"`
	RefID  int64          `json:"ref_id"`
	Label  string         `json:"label,omitempty"`
	Inline string         `json:"inline,omitempty"` // pre-rendered content (optional)
}
