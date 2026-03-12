package core

import "time"

// DAGTemplate is a reusable template for generating Issue DAGs.
// It stores a snapshot of steps (without runtime state) so users can
// quickly create new issues from proven patterns.
type DAGTemplate struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	ProjectID   *int64            `json:"project_id,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Steps       []DAGTemplateStep `json:"steps"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// DAGTemplateStep is a step blueprint inside a DAGTemplate.
// DependsOn uses step names (not IDs) so the template is self-contained.
type DAGTemplateStep struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description,omitempty"`
	Type                 string   `json:"type"` // exec | gate | composite
	DependsOn            []string `json:"depends_on,omitempty"`
	AgentRole            string   `json:"agent_role,omitempty"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
	AcceptanceCriteria   []string `json:"acceptance_criteria,omitempty"`
	ProfileID            string   `json:"profile_id,omitempty"` // optional: pre-assigned agent profile
}

// DAGTemplateFilter constrains DAGTemplate queries.
type DAGTemplateFilter struct {
	ProjectID *int64
	Tag       string
	Search    string // partial match on name/description
	Limit     int
	Offset    int
}
