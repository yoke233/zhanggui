package sqlite

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// JSONField stores structured values in TEXT columns.
type JSONField[T any] struct {
	Data T
}

func (j JSONField[T]) Value() (driver.Value, error) {
	b, err := json.Marshal(j.Data)
	if err != nil {
		return nil, err
	}
	s := string(b)
	if s == "null" || s == "{}" || s == "[]" {
		return nil, nil
	}
	return s, nil
}

func (j *JSONField[T]) Scan(value any) error {
	if value == nil {
		var zero T
		j.Data = zero
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case string:
		bytes = []byte(v)
	case []byte:
		bytes = v
	default:
		return fmt.Errorf("unsupported type for JSONField: %T", value)
	}
	return json.Unmarshal(bytes, &j.Data)
}

type ProjectModel struct {
	ID          int64                        `gorm:"column:id;primaryKey;autoIncrement"`
	Name        string                       `gorm:"column:name;not null"`
	Kind        string                       `gorm:"column:kind;not null"`
	Description string                       `gorm:"column:description;not null"`
	Metadata    JSONField[map[string]string] `gorm:"column:metadata;type:text"`
	CreatedAt   time.Time                    `gorm:"column:created_at"`
	UpdatedAt   time.Time                    `gorm:"column:updated_at"`
}

func (ProjectModel) TableName() string { return "projects" }

type ResourceBindingModel struct {
	ID        int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	ProjectID int64                     `gorm:"column:project_id;not null"`
	Kind      string                    `gorm:"column:kind;not null"`
	URI       string                    `gorm:"column:uri;not null"`
	Config    JSONField[map[string]any] `gorm:"column:config;type:text"`
	Label     string                    `gorm:"column:label;not null"`
	CreatedAt time.Time                 `gorm:"column:created_at"`
}

func (ResourceBindingModel) TableName() string { return "resource_bindings" }

type IssueModel struct {
	ID                int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	ProjectID         *int64                    `gorm:"column:project_id"`
	ResourceBindingID *int64                    `gorm:"column:resource_binding_id"`
	Title             string                    `gorm:"column:title;not null"`
	Body              string                    `gorm:"column:body;not null"`
	Status            string                    `gorm:"column:status;not null"`
	Priority          string                    `gorm:"column:priority;not null"`
	Labels            JSONField[[]string]       `gorm:"column:labels;type:text"`
	DependsOn         JSONField[[]int64]        `gorm:"column:depends_on;type:text"`
	Metadata          JSONField[map[string]any] `gorm:"column:metadata;type:text"`
	ArchivedAt        *time.Time                `gorm:"column:archived_at"`
	CreatedAt         time.Time                 `gorm:"column:created_at"`
	UpdatedAt         time.Time                 `gorm:"column:updated_at"`
}

func (IssueModel) TableName() string { return "issues" }

type StepModel struct {
	ID                   int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	IssueID              int64                     `gorm:"column:issue_id;not null"`
	Name                 string                    `gorm:"column:name;not null"`
	Description          string                    `gorm:"column:description;not null"`
	Type                 string                    `gorm:"column:type;not null"`
	Status               string                    `gorm:"column:status;not null"`
	Position             int                       `gorm:"column:position;not null"`
	AgentRole            string                    `gorm:"column:agent_role"`
	RequiredCapabilities JSONField[[]string]       `gorm:"column:required_capabilities;type:text"`
	AcceptanceCriteria   JSONField[[]string]       `gorm:"column:acceptance_criteria;type:text"`
	TimeoutMs            int64                     `gorm:"column:timeout_ms"`
	Config               JSONField[map[string]any] `gorm:"column:config;type:text"`
	MaxRetries           int                       `gorm:"column:max_retries"`
	RetryCount           int                       `gorm:"column:retry_count"`
	CreatedAt            time.Time                 `gorm:"column:created_at"`
	UpdatedAt            time.Time                 `gorm:"column:updated_at"`
}

func (StepModel) TableName() string { return "steps" }

type ExecutionModel struct {
	ID               int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	StepID           int64                     `gorm:"column:step_id;not null"`
	IssueID          int64                     `gorm:"column:issue_id;not null"`
	Status           string                    `gorm:"column:status;not null"`
	AgentID          string                    `gorm:"column:agent_id"`
	AgentContextID   *int64                    `gorm:"column:agent_context_id"`
	BriefingSnapshot string                    `gorm:"column:briefing_snapshot"`
	ArtifactID       *int64                    `gorm:"column:artifact_id"`
	Input            JSONField[map[string]any] `gorm:"column:input;type:text"`
	Output           JSONField[map[string]any] `gorm:"column:output;type:text"`
	ErrorMessage     string                    `gorm:"column:error_message"`
	ErrorKind        string                    `gorm:"column:error_kind"`
	Attempt          int                       `gorm:"column:attempt"`
	StartedAt        *time.Time                `gorm:"column:started_at"`
	FinishedAt       *time.Time                `gorm:"column:finished_at"`
	CreatedAt        time.Time                 `gorm:"column:created_at"`
}

func (ExecutionModel) TableName() string { return "executions" }

type ArtifactModel struct {
	ID             int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	ExecutionID    int64                     `gorm:"column:execution_id;not null"`
	StepID         int64                     `gorm:"column:step_id;not null"`
	IssueID        int64                     `gorm:"column:issue_id;not null"`
	ResultMarkdown string                    `gorm:"column:result_markdown;not null"`
	Metadata       JSONField[map[string]any] `gorm:"column:metadata;type:text"`
	Assets         JSONField[[]core.Asset]   `gorm:"column:assets;type:text"`
	CreatedAt      time.Time                 `gorm:"column:created_at"`
}

func (ArtifactModel) TableName() string { return "artifacts" }

type BriefingModel struct {
	ID          int64                        `gorm:"column:id;primaryKey;autoIncrement"`
	StepID      int64                        `gorm:"column:step_id;not null"`
	Objective   string                       `gorm:"column:objective;not null"`
	ContextRefs JSONField[[]core.ContextRef] `gorm:"column:context_refs;type:text"`
	Constraints JSONField[[]string]          `gorm:"column:constraints;type:text"`
	CreatedAt   time.Time                    `gorm:"column:created_at"`
}

func (BriefingModel) TableName() string { return "briefings" }

type AgentContextModel struct {
	ID               int64      `gorm:"column:id;primaryKey;autoIncrement"`
	AgentID          string     `gorm:"column:agent_id;not null"`
	IssueID          int64      `gorm:"column:issue_id;not null"`
	SystemPrompt     string     `gorm:"column:system_prompt"`
	SessionID        string     `gorm:"column:session_id"`
	Summary          string     `gorm:"column:summary"`
	TurnCount        int        `gorm:"column:turn_count"`
	WorkerID         string     `gorm:"column:worker_id;not null"`
	WorkerLastSeenAt *time.Time `gorm:"column:worker_last_seen_at"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
}

func (AgentContextModel) TableName() string { return "agent_contexts" }

type EventModel struct {
	ID        int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	Type      string                    `gorm:"column:type;not null"`
	IssueID   *int64                    `gorm:"column:issue_id"`
	StepID    *int64                    `gorm:"column:step_id"`
	ExecID    *int64                    `gorm:"column:exec_id"`
	Data      JSONField[map[string]any] `gorm:"column:data;type:text"`
	Timestamp time.Time                 `gorm:"column:timestamp"`
}

func (EventModel) TableName() string { return "events" }

type ExecutionProbeModel struct {
	ID             int64      `gorm:"column:id;primaryKey;autoIncrement"`
	ExecutionID    int64      `gorm:"column:execution_id;not null"`
	IssueID        int64      `gorm:"column:issue_id;not null"`
	StepID         int64      `gorm:"column:step_id;not null"`
	AgentContextID *int64     `gorm:"column:agent_context_id"`
	SessionID      string     `gorm:"column:session_id;not null"`
	OwnerID        string     `gorm:"column:owner_id;not null"`
	TriggerSource  string     `gorm:"column:trigger_source;not null"`
	Question       string     `gorm:"column:question;not null"`
	Status         string     `gorm:"column:status;not null"`
	Verdict        string     `gorm:"column:verdict;not null"`
	ReplyText      string     `gorm:"column:reply_text;not null"`
	Error          string     `gorm:"column:error;not null"`
	SentAt         *time.Time `gorm:"column:sent_at"`
	AnsweredAt     *time.Time `gorm:"column:answered_at"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
}

func (ExecutionProbeModel) TableName() string { return "execution_probes" }

type AgentDriverModel struct {
	ID            string                       `gorm:"column:id;primaryKey"`
	LaunchCommand string                       `gorm:"column:launch_command;not null"`
	LaunchArgs    JSONField[[]string]          `gorm:"column:launch_args;type:text"`
	Env           JSONField[map[string]string] `gorm:"column:env;type:text"`
	CapFSRead     bool                         `gorm:"column:cap_fs_read;not null"`
	CapFSWrite    bool                         `gorm:"column:cap_fs_write;not null"`
	CapTerminal   bool                         `gorm:"column:cap_terminal;not null"`
	CreatedAt     time.Time                    `gorm:"column:created_at"`
	UpdatedAt     time.Time                    `gorm:"column:updated_at"`
}

func (AgentDriverModel) TableName() string { return "agent_drivers" }

type AgentProfileModel struct {
	ID               string                   `gorm:"column:id;primaryKey"`
	Name             string                   `gorm:"column:name;not null"`
	DriverID         string                   `gorm:"column:driver_id;not null"`
	Role             string                   `gorm:"column:role;not null"`
	Capabilities     JSONField[[]string]      `gorm:"column:capabilities;type:text"`
	ActionsAllowed   JSONField[[]core.Action] `gorm:"column:actions_allowed;type:text"`
	PromptTemplate   string                   `gorm:"column:prompt_template;not null"`
	Skills           JSONField[[]string]      `gorm:"column:skills;type:text"`
	SessionReuse     bool                     `gorm:"column:session_reuse;not null"`
	SessionMaxTurns  int                      `gorm:"column:session_max_turns;not null"`
	SessionIdleTTLMs int64                    `gorm:"column:session_idle_ttl_ms;not null"`
	MCPEnabled       bool                     `gorm:"column:mcp_enabled;not null"`
	MCPTools         JSONField[[]string]      `gorm:"column:mcp_tools;type:text"`
	CreatedAt        time.Time                `gorm:"column:created_at"`
	UpdatedAt        time.Time                `gorm:"column:updated_at"`
}

func (AgentProfileModel) TableName() string { return "agent_profiles" }

type DAGTemplateModel struct {
	ID          int64                             `gorm:"column:id;primaryKey;autoIncrement"`
	Name        string                            `gorm:"column:name;not null"`
	Description string                            `gorm:"column:description;not null"`
	ProjectID   *int64                            `gorm:"column:project_id"`
	Tags        JSONField[[]string]               `gorm:"column:tags;type:text"`
	Metadata    JSONField[map[string]string]      `gorm:"column:metadata;type:text"`
	Steps       JSONField[[]core.DAGTemplateStep] `gorm:"column:steps;type:text"`
	CreatedAt   time.Time                         `gorm:"column:created_at"`
	UpdatedAt   time.Time                         `gorm:"column:updated_at"`
}

func (DAGTemplateModel) TableName() string { return "dag_templates" }

type UsageRecordModel struct {
	ID               int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ExecutionID      int64     `gorm:"column:execution_id;not null"`
	IssueID          int64     `gorm:"column:issue_id;not null"`
	StepID           int64     `gorm:"column:step_id;not null"`
	ProjectID        *int64    `gorm:"column:project_id"`
	AgentID          string    `gorm:"column:agent_id;not null"`
	ProfileID        string    `gorm:"column:profile_id;not null"`
	ModelID          string    `gorm:"column:model_id;not null"`
	InputTokens      int64     `gorm:"column:input_tokens;not null"`
	OutputTokens     int64     `gorm:"column:output_tokens;not null"`
	CacheReadTokens  int64     `gorm:"column:cache_read_tokens;not null"`
	CacheWriteTokens int64     `gorm:"column:cache_write_tokens;not null"`
	ReasoningTokens  int64     `gorm:"column:reasoning_tokens;not null"`
	TotalTokens      int64     `gorm:"column:total_tokens;not null"`
	DurationMs       int64     `gorm:"column:duration_ms;not null"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (UsageRecordModel) TableName() string { return "usage_records" }

func projectModelFromCore(p *core.Project) *ProjectModel {
	if p == nil {
		return nil
	}
	return &ProjectModel{
		ID:          p.ID,
		Name:        p.Name,
		Kind:        string(p.Kind),
		Description: p.Description,
		Metadata:    JSONField[map[string]string]{Data: p.Metadata},
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func (m *ProjectModel) toCore() *core.Project {
	if m == nil {
		return nil
	}
	return &core.Project{
		ID:          m.ID,
		Name:        m.Name,
		Kind:        core.ProjectKind(m.Kind),
		Description: m.Description,
		Metadata:    m.Metadata.Data,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func resourceBindingModelFromCore(rb *core.ResourceBinding) *ResourceBindingModel {
	if rb == nil {
		return nil
	}
	return &ResourceBindingModel{
		ID:        rb.ID,
		ProjectID: rb.ProjectID,
		Kind:      rb.Kind,
		URI:       rb.URI,
		Config:    JSONField[map[string]any]{Data: rb.Config},
		Label:     rb.Label,
		CreatedAt: rb.CreatedAt,
	}
}

func (m *ResourceBindingModel) toCore() *core.ResourceBinding {
	if m == nil {
		return nil
	}
	return &core.ResourceBinding{
		ID:        m.ID,
		ProjectID: m.ProjectID,
		Kind:      m.Kind,
		URI:       m.URI,
		Config:    m.Config.Data,
		Label:     m.Label,
		CreatedAt: m.CreatedAt,
	}
}

func issueModelFromCore(issue *core.Issue) *IssueModel {
	if issue == nil {
		return nil
	}
	return &IssueModel{
		ID:                issue.ID,
		ProjectID:         issue.ProjectID,
		ResourceBindingID: issue.ResourceBindingID,
		Title:             issue.Title,
		Body:              issue.Body,
		Status:            string(issue.Status),
		Priority:          string(issue.Priority),
		Labels:            JSONField[[]string]{Data: issue.Labels},
		DependsOn:         JSONField[[]int64]{Data: issue.DependsOn},
		Metadata:          JSONField[map[string]any]{Data: issue.Metadata},
		ArchivedAt:        issue.ArchivedAt,
		CreatedAt:         issue.CreatedAt,
		UpdatedAt:         issue.UpdatedAt,
	}
}

func (m *IssueModel) toCore() *core.Issue {
	if m == nil {
		return nil
	}
	return &core.Issue{
		ID:                m.ID,
		ProjectID:         m.ProjectID,
		ResourceBindingID: m.ResourceBindingID,
		Title:             m.Title,
		Body:              m.Body,
		Status:            core.IssueStatus(m.Status),
		Priority:          core.IssuePriority(m.Priority),
		Labels:            m.Labels.Data,
		DependsOn:         m.DependsOn.Data,
		Metadata:          m.Metadata.Data,
		ArchivedAt:        m.ArchivedAt,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

func stepModelFromCore(step *core.Step) *StepModel {
	if step == nil {
		return nil
	}
	return &StepModel{
		ID:                   step.ID,
		IssueID:              step.IssueID,
		Name:                 step.Name,
		Description:          step.Description,
		Type:                 string(step.Type),
		Status:               string(step.Status),
		Position:             step.Position,
		AgentRole:            step.AgentRole,
		RequiredCapabilities: JSONField[[]string]{Data: step.RequiredCapabilities},
		AcceptanceCriteria:   JSONField[[]string]{Data: step.AcceptanceCriteria},
		TimeoutMs:            step.Timeout.Milliseconds(),
		Config:               JSONField[map[string]any]{Data: step.Config},
		MaxRetries:           step.MaxRetries,
		RetryCount:           step.RetryCount,
		CreatedAt:            step.CreatedAt,
		UpdatedAt:            step.UpdatedAt,
	}
}

func (m *StepModel) toCore() *core.Step {
	if m == nil {
		return nil
	}
	return &core.Step{
		ID:                   m.ID,
		IssueID:              m.IssueID,
		Name:                 m.Name,
		Description:          m.Description,
		Type:                 core.StepType(m.Type),
		Status:               core.StepStatus(m.Status),
		Position:             m.Position,
		AgentRole:            m.AgentRole,
		RequiredCapabilities: m.RequiredCapabilities.Data,
		AcceptanceCriteria:   m.AcceptanceCriteria.Data,
		Timeout:              time.Duration(m.TimeoutMs) * time.Millisecond,
		Config:               m.Config.Data,
		MaxRetries:           m.MaxRetries,
		RetryCount:           m.RetryCount,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}

func executionModelFromCore(exec *core.Execution) *ExecutionModel {
	if exec == nil {
		return nil
	}
	return &ExecutionModel{
		ID:               exec.ID,
		StepID:           exec.StepID,
		IssueID:          exec.IssueID,
		Status:           string(exec.Status),
		AgentID:          exec.AgentID,
		AgentContextID:   exec.AgentContextID,
		BriefingSnapshot: exec.BriefingSnapshot,
		ArtifactID:       exec.ArtifactID,
		Input:            JSONField[map[string]any]{Data: exec.Input},
		Output:           JSONField[map[string]any]{Data: exec.Output},
		ErrorMessage:     exec.ErrorMessage,
		ErrorKind:        string(exec.ErrorKind),
		Attempt:          exec.Attempt,
		StartedAt:        exec.StartedAt,
		FinishedAt:       exec.FinishedAt,
		CreatedAt:        exec.CreatedAt,
	}
}

func (m *ExecutionModel) toCore() *core.Execution {
	if m == nil {
		return nil
	}
	return &core.Execution{
		ID:               m.ID,
		StepID:           m.StepID,
		IssueID:          m.IssueID,
		Status:           core.ExecutionStatus(m.Status),
		AgentID:          m.AgentID,
		AgentContextID:   m.AgentContextID,
		BriefingSnapshot: m.BriefingSnapshot,
		ArtifactID:       m.ArtifactID,
		Input:            m.Input.Data,
		Output:           m.Output.Data,
		ErrorMessage:     m.ErrorMessage,
		ErrorKind:        core.ErrorKind(m.ErrorKind),
		Attempt:          m.Attempt,
		StartedAt:        m.StartedAt,
		FinishedAt:       m.FinishedAt,
		CreatedAt:        m.CreatedAt,
	}
}

func artifactModelFromCore(artifact *core.Artifact) *ArtifactModel {
	if artifact == nil {
		return nil
	}
	return &ArtifactModel{
		ID:             artifact.ID,
		ExecutionID:    artifact.ExecutionID,
		StepID:         artifact.StepID,
		IssueID:        artifact.IssueID,
		ResultMarkdown: artifact.ResultMarkdown,
		Metadata:       JSONField[map[string]any]{Data: artifact.Metadata},
		Assets:         JSONField[[]core.Asset]{Data: artifact.Assets},
		CreatedAt:      artifact.CreatedAt,
	}
}

func (m *ArtifactModel) toCore() *core.Artifact {
	if m == nil {
		return nil
	}
	return &core.Artifact{
		ID:             m.ID,
		ExecutionID:    m.ExecutionID,
		StepID:         m.StepID,
		IssueID:        m.IssueID,
		ResultMarkdown: m.ResultMarkdown,
		Metadata:       m.Metadata.Data,
		Assets:         m.Assets.Data,
		CreatedAt:      m.CreatedAt,
	}
}

func briefingModelFromCore(briefing *core.Briefing) *BriefingModel {
	if briefing == nil {
		return nil
	}
	return &BriefingModel{
		ID:          briefing.ID,
		StepID:      briefing.StepID,
		Objective:   briefing.Objective,
		ContextRefs: JSONField[[]core.ContextRef]{Data: briefing.ContextRefs},
		Constraints: JSONField[[]string]{Data: briefing.Constraints},
		CreatedAt:   briefing.CreatedAt,
	}
}

func (m *BriefingModel) toCore() *core.Briefing {
	if m == nil {
		return nil
	}
	return &core.Briefing{
		ID:          m.ID,
		StepID:      m.StepID,
		Objective:   m.Objective,
		ContextRefs: m.ContextRefs.Data,
		Constraints: m.Constraints.Data,
		CreatedAt:   m.CreatedAt,
	}
}

func agentContextModelFromCore(ac *core.AgentContext) *AgentContextModel {
	if ac == nil {
		return nil
	}
	return &AgentContextModel{
		ID:               ac.ID,
		AgentID:          ac.AgentID,
		IssueID:          ac.IssueID,
		SystemPrompt:     ac.SystemPrompt,
		SessionID:        ac.SessionID,
		Summary:          ac.Summary,
		TurnCount:        ac.TurnCount,
		WorkerID:         ac.WorkerID,
		WorkerLastSeenAt: ac.WorkerLastSeenAt,
		CreatedAt:        ac.CreatedAt,
		UpdatedAt:        ac.UpdatedAt,
	}
}

func (m *AgentContextModel) toCore() *core.AgentContext {
	if m == nil {
		return nil
	}
	return &core.AgentContext{
		ID:               m.ID,
		AgentID:          m.AgentID,
		IssueID:          m.IssueID,
		SystemPrompt:     m.SystemPrompt,
		SessionID:        m.SessionID,
		Summary:          m.Summary,
		TurnCount:        m.TurnCount,
		WorkerID:         m.WorkerID,
		WorkerLastSeenAt: m.WorkerLastSeenAt,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
}

func eventModelFromCore(event *core.Event) *EventModel {
	if event == nil {
		return nil
	}
	return &EventModel{
		ID:        event.ID,
		Type:      string(event.Type),
		IssueID:   int64PtrIfNonZero(event.IssueID),
		StepID:    int64PtrIfNonZero(event.StepID),
		ExecID:    int64PtrIfNonZero(event.ExecID),
		Data:      JSONField[map[string]any]{Data: event.Data},
		Timestamp: event.Timestamp,
	}
}

func (m *EventModel) toCore() *core.Event {
	if m == nil {
		return nil
	}
	event := &core.Event{
		ID:        m.ID,
		Type:      core.EventType(m.Type),
		Data:      m.Data.Data,
		Timestamp: m.Timestamp,
	}
	if m.IssueID != nil {
		event.IssueID = *m.IssueID
	}
	if m.StepID != nil {
		event.StepID = *m.StepID
	}
	if m.ExecID != nil {
		event.ExecID = *m.ExecID
	}
	return event
}

func int64PtrIfNonZero(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

func executionProbeModelFromCore(probe *core.ExecutionProbe) *ExecutionProbeModel {
	if probe == nil {
		return nil
	}
	return &ExecutionProbeModel{
		ID:             probe.ID,
		ExecutionID:    probe.ExecutionID,
		IssueID:        probe.IssueID,
		StepID:         probe.StepID,
		AgentContextID: probe.AgentContextID,
		SessionID:      probe.SessionID,
		OwnerID:        probe.OwnerID,
		TriggerSource:  string(probe.TriggerSource),
		Question:       probe.Question,
		Status:         string(probe.Status),
		Verdict:        string(probe.Verdict),
		ReplyText:      probe.ReplyText,
		Error:          probe.Error,
		SentAt:         probe.SentAt,
		AnsweredAt:     probe.AnsweredAt,
		CreatedAt:      probe.CreatedAt,
	}
}

func (m *ExecutionProbeModel) toCore() *core.ExecutionProbe {
	if m == nil {
		return nil
	}
	return &core.ExecutionProbe{
		ID:             m.ID,
		ExecutionID:    m.ExecutionID,
		IssueID:        m.IssueID,
		StepID:         m.StepID,
		AgentContextID: m.AgentContextID,
		SessionID:      m.SessionID,
		OwnerID:        m.OwnerID,
		TriggerSource:  core.ExecutionProbeTriggerSource(m.TriggerSource),
		Question:       m.Question,
		Status:         core.ExecutionProbeStatus(m.Status),
		Verdict:        core.ExecutionProbeVerdict(m.Verdict),
		ReplyText:      m.ReplyText,
		Error:          m.Error,
		SentAt:         m.SentAt,
		AnsweredAt:     m.AnsweredAt,
		CreatedAt:      m.CreatedAt,
	}
}

func agentDriverModelFromCore(d *core.AgentDriver) *AgentDriverModel {
	if d == nil {
		return nil
	}
	return &AgentDriverModel{
		ID:            d.ID,
		LaunchCommand: d.LaunchCommand,
		LaunchArgs:    JSONField[[]string]{Data: d.LaunchArgs},
		Env:           JSONField[map[string]string]{Data: d.Env},
		CapFSRead:     d.CapabilitiesMax.FSRead,
		CapFSWrite:    d.CapabilitiesMax.FSWrite,
		CapTerminal:   d.CapabilitiesMax.Terminal,
	}
}

func (m *AgentDriverModel) toCore() *core.AgentDriver {
	if m == nil {
		return nil
	}
	return &core.AgentDriver{
		ID:            m.ID,
		LaunchCommand: m.LaunchCommand,
		LaunchArgs:    m.LaunchArgs.Data,
		Env:           m.Env.Data,
		CapabilitiesMax: core.DriverCapabilities{
			FSRead:   m.CapFSRead,
			FSWrite:  m.CapFSWrite,
			Terminal: m.CapTerminal,
		},
	}
}

func agentProfileModelFromCore(p *core.AgentProfile) *AgentProfileModel {
	if p == nil {
		return nil
	}
	return &AgentProfileModel{
		ID:               p.ID,
		Name:             p.Name,
		DriverID:         p.DriverID,
		Role:             string(p.Role),
		Capabilities:     JSONField[[]string]{Data: p.Capabilities},
		ActionsAllowed:   JSONField[[]core.Action]{Data: p.ActionsAllowed},
		PromptTemplate:   p.PromptTemplate,
		Skills:           JSONField[[]string]{Data: p.Skills},
		SessionReuse:     p.Session.Reuse,
		SessionMaxTurns:  p.Session.MaxTurns,
		SessionIdleTTLMs: p.Session.IdleTTL.Milliseconds(),
		MCPEnabled:       p.MCP.Enabled,
		MCPTools:         JSONField[[]string]{Data: p.MCP.Tools},
	}
}

func (m *AgentProfileModel) toCore() *core.AgentProfile {
	if m == nil {
		return nil
	}
	return &core.AgentProfile{
		ID:             m.ID,
		Name:           m.Name,
		DriverID:       m.DriverID,
		Role:           core.AgentRole(m.Role),
		Capabilities:   m.Capabilities.Data,
		ActionsAllowed: m.ActionsAllowed.Data,
		PromptTemplate: m.PromptTemplate,
		Skills:         m.Skills.Data,
		Session: core.ProfileSession{
			Reuse:    m.SessionReuse,
			MaxTurns: m.SessionMaxTurns,
			IdleTTL:  time.Duration(m.SessionIdleTTLMs) * time.Millisecond,
		},
		MCP: core.ProfileMCP{
			Enabled: m.MCPEnabled,
			Tools:   m.MCPTools.Data,
		},
	}
}

func dagTemplateModelFromCore(t *core.DAGTemplate) *DAGTemplateModel {
	if t == nil {
		return nil
	}
	return &DAGTemplateModel{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		ProjectID:   t.ProjectID,
		Tags:        JSONField[[]string]{Data: t.Tags},
		Metadata:    JSONField[map[string]string]{Data: t.Metadata},
		Steps:       JSONField[[]core.DAGTemplateStep]{Data: t.Steps},
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func (m *DAGTemplateModel) toCore() *core.DAGTemplate {
	if m == nil {
		return nil
	}
	return &core.DAGTemplate{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		ProjectID:   m.ProjectID,
		Tags:        m.Tags.Data,
		Metadata:    m.Metadata.Data,
		Steps:       m.Steps.Data,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func usageRecordModelFromCore(r *core.UsageRecord) *UsageRecordModel {
	if r == nil {
		return nil
	}
	return &UsageRecordModel{
		ID:               r.ID,
		ExecutionID:      r.ExecutionID,
		IssueID:          r.IssueID,
		StepID:           r.StepID,
		ProjectID:        r.ProjectID,
		AgentID:          r.AgentID,
		ProfileID:        r.ProfileID,
		ModelID:          r.ModelID,
		InputTokens:      r.InputTokens,
		OutputTokens:     r.OutputTokens,
		CacheReadTokens:  r.CacheReadTokens,
		CacheWriteTokens: r.CacheWriteTokens,
		ReasoningTokens:  r.ReasoningTokens,
		TotalTokens:      r.TotalTokens,
		DurationMs:       r.DurationMs,
		CreatedAt:        r.CreatedAt,
	}
}

func (m *UsageRecordModel) toCore() *core.UsageRecord {
	if m == nil {
		return nil
	}
	return &core.UsageRecord{
		ID:               m.ID,
		ExecutionID:      m.ExecutionID,
		IssueID:          m.IssueID,
		StepID:           m.StepID,
		ProjectID:        m.ProjectID,
		AgentID:          m.AgentID,
		ProfileID:        m.ProfileID,
		ModelID:          m.ModelID,
		InputTokens:      m.InputTokens,
		OutputTokens:     m.OutputTokens,
		CacheReadTokens:  m.CacheReadTokens,
		CacheWriteTokens: m.CacheWriteTokens,
		ReasoningTokens:  m.ReasoningTokens,
		TotalTokens:      m.TotalTokens,
		DurationMs:       m.DurationMs,
		CreatedAt:        m.CreatedAt,
	}
}
