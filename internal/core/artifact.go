package core

import "time"

// Artifact is the unified deliverable of an Execution.
// Every Execution produces exactly one Artifact.
// result_markdown is the agent's natural language output.
// metadata is engine-extracted structured data (via small model in Collect phase).
type Artifact struct {
	ID             int64          `json:"id"`
	ExecutionID    int64          `json:"execution_id"`
	StepID         int64          `json:"step_id"`
	IssueID        int64          `json:"issue_id"`
	ResultMarkdown string         `json:"result_markdown"`    // agent's primary output (natural language)
	Metadata       map[string]any `json:"metadata,omitempty"` // engine-extracted structured data
	Assets         []Asset        `json:"assets,omitempty"`   // attachments
	CreatedAt      time.Time      `json:"created_at"`
}

// Asset is an attachment in an Artifact.
type Asset struct {
	Name      string `json:"name"`
	URI       string `json:"uri"`
	MediaType string `json:"media_type,omitempty"`
}
