package core

import "time"

// DAGTemplate is a reusable template for generating WorkItem DAGs.
// It stores a snapshot of actions (without runtime state) so users can
// quickly create new work items from proven patterns.
type DAGTemplate struct {
	ID          int64               `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	ProjectID   *int64              `json:"project_id,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	Metadata    map[string]string   `json:"metadata,omitempty"`
	Actions     []DAGTemplateAction `json:"actions"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// DAGTemplateAction is an action blueprint inside a DAGTemplate.
// DependsOn uses action names (not IDs) so the template is self-contained.
type DAGTemplateAction struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description,omitempty"`
	Type                 string   `json:"type"` // exec | gate | plan
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
