package core

import "time"

// Deliverable is the unified output of a Run.
// Every Run produces exactly one Deliverable.
// ResultMarkdown is the agent's natural language output.
// Metadata is engine-extracted structured data (via small model in Collect phase).
type Deliverable struct {
	ID             int64          `json:"id"`
	RunID          int64          `json:"run_id"`
	ActionID       int64          `json:"action_id"`
	WorkItemID     int64          `json:"work_item_id"`
	ResultMarkdown string         `json:"result_markdown"`    // agent's primary output (natural language)
	Metadata       map[string]any `json:"metadata,omitempty"` // engine-extracted structured data
	Assets         []Asset        `json:"assets,omitempty"`   // attachments
	CreatedAt      time.Time      `json:"created_at"`
}

// Asset is an attachment in a Deliverable.
type Asset struct {
	Name      string `json:"name"`
	URI       string `json:"uri"`
	MediaType string `json:"media_type,omitempty"`
}
