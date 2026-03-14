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
	UpdatedAt time.Time                 `gorm:"column:updated_at"`
}

func (ResourceBindingModel) TableName() string { return "resource_bindings" }

type WorkItemModel struct {
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

func (WorkItemModel) TableName() string { return "issues" }

type WorkItemAttachmentModel struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"`
	IssueID   int64     `gorm:"column:issue_id;not null"`
	FileName  string    `gorm:"column:file_name;not null"`
	FilePath  string    `gorm:"column:file_path;not null"`
	MimeType  string    `gorm:"column:mime_type;not null"`
	Size      int64     `gorm:"column:size;not null"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (WorkItemAttachmentModel) TableName() string { return "work_item_attachments" }

func (m *WorkItemAttachmentModel) toCore() *core.WorkItemAttachment {
	return &core.WorkItemAttachment{
		ID:         m.ID,
		WorkItemID: m.IssueID,
		FileName:   m.FileName,
		FilePath:   m.FilePath,
		MimeType:   m.MimeType,
		Size:       m.Size,
		CreatedAt:  m.CreatedAt,
	}
}

func workItemAttachmentModelFromCore(a *core.WorkItemAttachment) *WorkItemAttachmentModel {
	return &WorkItemAttachmentModel{
		ID:        a.ID,
		IssueID:   a.WorkItemID,
		FileName:  a.FileName,
		FilePath:  a.FilePath,
		MimeType:  a.MimeType,
		Size:      a.Size,
		CreatedAt: a.CreatedAt,
	}
}

type ActionModel struct {
	ID                   int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	IssueID              int64                     `gorm:"column:issue_id;not null"`
	Name                 string                    `gorm:"column:name;not null"`
	Description          string                    `gorm:"column:description;not null"`
	Type                 string                    `gorm:"column:type;not null"`
	Status               string                    `gorm:"column:status;not null"`
	Position             int                       `gorm:"column:position;not null"`
	DependsOn            JSONField[[]int64]        `gorm:"column:depends_on;type:text"`
	Input                string                    `gorm:"column:input"`
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

func (ActionModel) TableName() string { return "steps" }

type RunModel struct {
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

func (RunModel) TableName() string { return "executions" }

type DeliverableModel struct {
	ID             int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	ExecutionID    int64                     `gorm:"column:execution_id;not null"`
	StepID         int64                     `gorm:"column:step_id;not null"`
	IssueID        int64                     `gorm:"column:issue_id;not null"`
	ResultMarkdown string                    `gorm:"column:result_markdown;not null"`
	Metadata       JSONField[map[string]any] `gorm:"column:metadata;type:text"`
	Assets         JSONField[[]core.Asset]   `gorm:"column:assets;type:text"`
	CreatedAt      time.Time                 `gorm:"column:created_at"`
}

func (DeliverableModel) TableName() string { return "artifacts" }

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

type RunProbeModel struct {
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

func (RunProbeModel) TableName() string { return "execution_probes" }

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
	ID               string                        `gorm:"column:id;primaryKey"`
	Name             string                        `gorm:"column:name;not null"`
	DriverID         string                        `gorm:"column:driver_id;not null"`
	Role             string                        `gorm:"column:role;not null"`
	Capabilities     JSONField[[]string]           `gorm:"column:capabilities;type:text"`
	ActionsAllowed   JSONField[[]core.AgentAction] `gorm:"column:actions_allowed;type:text"`
	PromptTemplate   string                        `gorm:"column:prompt_template;not null"`
	Skills           JSONField[[]string]           `gorm:"column:skills;type:text"`
	SessionReuse     bool                          `gorm:"column:session_reuse;not null"`
	SessionMaxTurns  int                           `gorm:"column:session_max_turns;not null"`
	SessionIdleTTLMs int64                         `gorm:"column:session_idle_ttl_ms;not null"`
	MCPEnabled       bool                          `gorm:"column:mcp_enabled;not null"`
	MCPTools         JSONField[[]string]           `gorm:"column:mcp_tools;type:text"`
	CreatedAt        time.Time                     `gorm:"column:created_at"`
	UpdatedAt        time.Time                     `gorm:"column:updated_at"`
}

func (AgentProfileModel) TableName() string { return "agent_profiles" }

type DAGTemplateModel struct {
	ID          int64                               `gorm:"column:id;primaryKey;autoIncrement"`
	Name        string                              `gorm:"column:name;not null"`
	Description string                              `gorm:"column:description;not null"`
	ProjectID   *int64                              `gorm:"column:project_id"`
	Tags        JSONField[[]string]                 `gorm:"column:tags;type:text"`
	Metadata    JSONField[map[string]string]        `gorm:"column:metadata;type:text"`
	Steps       JSONField[[]core.DAGTemplateAction] `gorm:"column:steps;type:text"`
	CreatedAt   time.Time                           `gorm:"column:created_at"`
	UpdatedAt   time.Time                           `gorm:"column:updated_at"`
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

type ToolCallAuditModel struct {
	ID             int64      `gorm:"column:id;primaryKey;autoIncrement"`
	IssueID        int64      `gorm:"column:issue_id;not null"`
	StepID         int64      `gorm:"column:step_id;not null"`
	ExecutionID    int64      `gorm:"column:execution_id;not null"`
	SessionID      string     `gorm:"column:session_id;not null"`
	ToolCallID     string     `gorm:"column:tool_call_id;not null"`
	ToolName       string     `gorm:"column:tool_name;not null"`
	Status         string     `gorm:"column:status;not null"`
	StartedAt      *time.Time `gorm:"column:started_at"`
	FinishedAt     *time.Time `gorm:"column:finished_at"`
	DurationMs     int64      `gorm:"column:duration_ms;not null"`
	ExitCode       *int       `gorm:"column:exit_code"`
	InputDigest    string     `gorm:"column:input_digest;not null"`
	OutputDigest   string     `gorm:"column:output_digest;not null"`
	StdoutDigest   string     `gorm:"column:stdout_digest;not null"`
	StderrDigest   string     `gorm:"column:stderr_digest;not null"`
	InputPreview   string     `gorm:"column:input_preview;not null"`
	OutputPreview  string     `gorm:"column:output_preview;not null"`
	StdoutPreview  string     `gorm:"column:stdout_preview;not null"`
	StderrPreview  string     `gorm:"column:stderr_preview;not null"`
	RedactionLevel string     `gorm:"column:redaction_level;not null"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
}

func (ToolCallAuditModel) TableName() string { return "tool_call_audits" }

// ── Thread ──

type ThreadModel struct {
	ID        int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	Title     string                    `gorm:"column:title;not null"`
	Status    string                    `gorm:"column:status;not null"`
	OwnerID   string                    `gorm:"column:owner_id;not null"`
	Summary   string                    `gorm:"column:summary;not null"`
	Metadata  JSONField[map[string]any] `gorm:"column:metadata;type:text"`
	CreatedAt time.Time                 `gorm:"column:created_at"`
	UpdatedAt time.Time                 `gorm:"column:updated_at"`
}

func (ThreadModel) TableName() string { return "threads" }

func threadModelFromCore(t *core.Thread) *ThreadModel {
	if t == nil {
		return nil
	}
	return &ThreadModel{
		ID:        t.ID,
		Title:     t.Title,
		Status:    string(t.Status),
		OwnerID:   t.OwnerID,
		Summary:   t.Summary,
		Metadata:  JSONField[map[string]any]{Data: t.Metadata},
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

func (m *ThreadModel) toCore() *core.Thread {
	if m == nil {
		return nil
	}
	return &core.Thread{
		ID:        m.ID,
		Title:     m.Title,
		Status:    core.ThreadStatus(m.Status),
		OwnerID:   m.OwnerID,
		Summary:   m.Summary,
		Metadata:  m.Metadata.Data,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

type ThreadMessageModel struct {
	ID               int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	ThreadID         int64                     `gorm:"column:thread_id;not null"`
	SenderID         string                    `gorm:"column:sender_id;not null"`
	Role             string                    `gorm:"column:role;not null"`
	Content          string                    `gorm:"column:content;not null"`
	ReplyToMessageID *int64                    `gorm:"column:reply_to_msg_id"`
	Metadata         JSONField[map[string]any] `gorm:"column:metadata;type:text"`
	CreatedAt        time.Time                 `gorm:"column:created_at"`
}

func (ThreadMessageModel) TableName() string { return "thread_messages" }

func (m *ThreadMessageModel) toCore() *core.ThreadMessage {
	if m == nil {
		return nil
	}
	return &core.ThreadMessage{
		ID:               m.ID,
		ThreadID:         m.ThreadID,
		SenderID:         m.SenderID,
		Role:             m.Role,
		Content:          m.Content,
		ReplyToMessageID: m.ReplyToMessageID,
		Metadata:         m.Metadata.Data,
		CreatedAt:        m.CreatedAt,
	}
}

type ThreadParticipantModel struct {
	ID       int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ThreadID int64     `gorm:"column:thread_id;not null"`
	UserID   string    `gorm:"column:user_id;not null"`
	Role     string    `gorm:"column:role;not null"`
	JoinedAt time.Time `gorm:"column:joined_at"`
}

func (ThreadParticipantModel) TableName() string { return "thread_participants" }

func (m *ThreadParticipantModel) toCore() *core.ThreadParticipant {
	if m == nil {
		return nil
	}
	return &core.ThreadParticipant{
		ID:       m.ID,
		ThreadID: m.ThreadID,
		UserID:   m.UserID,
		Role:     m.Role,
		JoinedAt: m.JoinedAt,
	}
}

// ThreadWorkItemLinkModel persists thread-workitem links.
type ThreadWorkItemLinkModel struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ThreadID     int64     `gorm:"column:thread_id;not null"`
	WorkItemID   int64     `gorm:"column:work_item_id;not null"`
	RelationType string    `gorm:"column:relation_type;not null;default:related"`
	IsPrimary    bool      `gorm:"column:is_primary;not null;default:false"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (ThreadWorkItemLinkModel) TableName() string { return "thread_work_item_links" }

func (m *ThreadWorkItemLinkModel) toCore() *core.ThreadWorkItemLink {
	if m == nil {
		return nil
	}
	return &core.ThreadWorkItemLink{
		ID:           m.ID,
		ThreadID:     m.ThreadID,
		WorkItemID:   m.WorkItemID,
		RelationType: m.RelationType,
		IsPrimary:    m.IsPrimary,
		CreatedAt:    m.CreatedAt,
	}
}

// ThreadAgentSessionModel persists thread agent sessions.
type ThreadAgentSessionModel struct {
	ID                int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	ThreadID          int64                     `gorm:"column:thread_id;not null"`
	AgentProfileID    string                    `gorm:"column:agent_profile_id;not null"`
	ACPSessionID      string                    `gorm:"column:acp_session_id;not null;default:''"`
	Status            string                    `gorm:"column:status;not null;default:joining"`
	TurnCount         int                       `gorm:"column:turn_count;not null;default:0"`
	TotalInputTokens  int64                     `gorm:"column:total_input_tokens;not null;default:0"`
	TotalOutputTokens int64                     `gorm:"column:total_output_tokens;not null;default:0"`
	ProgressSummary   string                    `gorm:"column:progress_summary;not null;default:''"`
	Metadata          JSONField[map[string]any] `gorm:"column:metadata;type:text"`
	JoinedAt          time.Time                 `gorm:"column:joined_at"`
	LastActiveAt      time.Time                 `gorm:"column:last_active_at"`
}

func (ThreadAgentSessionModel) TableName() string { return "thread_agent_sessions" }

func (m *ThreadAgentSessionModel) toCore() *core.ThreadAgentSession {
	if m == nil {
		return nil
	}
	return &core.ThreadAgentSession{
		ID:                m.ID,
		ThreadID:          m.ThreadID,
		AgentProfileID:    m.AgentProfileID,
		ACPSessionID:      m.ACPSessionID,
		Status:            core.ThreadAgentStatus(m.Status),
		TurnCount:         m.TurnCount,
		TotalInputTokens:  m.TotalInputTokens,
		TotalOutputTokens: m.TotalOutputTokens,
		ProgressSummary:   m.ProgressSummary,
		Metadata:          m.Metadata.Data,
		JoinedAt:          m.JoinedAt,
		LastActiveAt:      m.LastActiveAt,
	}
}

// FeatureManifestModel is the GORM model for feature_manifests table.
type FeatureManifestModel struct {
	ID        int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	ProjectID int64                     `gorm:"column:project_id;not null"`
	Version   int                       `gorm:"column:version;not null"`
	Summary   string                    `gorm:"column:summary;not null"`
	Metadata  JSONField[map[string]any] `gorm:"column:metadata;type:text"`
	CreatedAt time.Time                 `gorm:"column:created_at"`
	UpdatedAt time.Time                 `gorm:"column:updated_at"`
}

func (FeatureManifestModel) TableName() string { return "feature_manifests" }

func (m *FeatureManifestModel) toCore() *core.FeatureManifest {
	if m == nil {
		return nil
	}
	return &core.FeatureManifest{
		ID:        m.ID,
		ProjectID: m.ProjectID,
		Version:   m.Version,
		Summary:   m.Summary,
		Metadata:  m.Metadata.Data,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func featureManifestModelFromCore(fm *core.FeatureManifest) *FeatureManifestModel {
	if fm == nil {
		return nil
	}
	return &FeatureManifestModel{
		ID:        fm.ID,
		ProjectID: fm.ProjectID,
		Version:   fm.Version,
		Summary:   fm.Summary,
		Metadata:  JSONField[map[string]any]{Data: fm.Metadata},
		CreatedAt: fm.CreatedAt,
		UpdatedAt: fm.UpdatedAt,
	}
}

// FeatureEntryModel is the GORM model for feature_entries table.
type FeatureEntryModel struct {
	ID          int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	ManifestID  int64                     `gorm:"column:manifest_id;not null"`
	Key         string                    `gorm:"column:key;not null"`
	Description string                    `gorm:"column:description;not null"`
	Status      string                    `gorm:"column:status;not null"`
	IssueID     *int64                    `gorm:"column:issue_id"`
	StepID      *int64                    `gorm:"column:step_id"`
	Tags        JSONField[[]string]       `gorm:"column:tags;type:text"`
	Metadata    JSONField[map[string]any] `gorm:"column:metadata;type:text"`
	CreatedAt   time.Time                 `gorm:"column:created_at"`
	UpdatedAt   time.Time                 `gorm:"column:updated_at"`
}

func (FeatureEntryModel) TableName() string { return "feature_entries" }

func (m *FeatureEntryModel) toCore() *core.FeatureEntry {
	if m == nil {
		return nil
	}
	return &core.FeatureEntry{
		ID:          m.ID,
		ManifestID:  m.ManifestID,
		Key:         m.Key,
		Description: m.Description,
		Status:      core.FeatureStatus(m.Status),
		WorkItemID:  m.IssueID,
		ActionID:    m.StepID,
		Tags:        m.Tags.Data,
		Metadata:    m.Metadata.Data,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func featureEntryModelFromCore(e *core.FeatureEntry) *FeatureEntryModel {
	if e == nil {
		return nil
	}
	return &FeatureEntryModel{
		ID:          e.ID,
		ManifestID:  e.ManifestID,
		Key:         e.Key,
		Description: e.Description,
		Status:      string(e.Status),
		IssueID:     e.WorkItemID,
		StepID:      e.ActionID,
		Tags:        JSONField[[]string]{Data: e.Tags},
		Metadata:    JSONField[map[string]any]{Data: e.Metadata},
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

// ── ActionSignal ──

type ActionSignalModel struct {
	ID           int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	StepID       int64                     `gorm:"column:step_id;not null"`
	IssueID      int64                     `gorm:"column:issue_id;not null"`
	ExecID       *int64                    `gorm:"column:exec_id"`
	Type         string                    `gorm:"column:type;not null"`
	Source       string                    `gorm:"column:source;not null"`
	Summary      string                    `gorm:"column:summary;not null"`
	Content      string                    `gorm:"column:content;not null"`
	SourceStepID *int64                    `gorm:"column:source_step_id"`
	Payload      JSONField[map[string]any] `gorm:"column:payload;type:text"`
	Actor        string                    `gorm:"column:actor;not null"`
	CreatedAt    time.Time                 `gorm:"column:created_at"`
}

func (ActionSignalModel) TableName() string { return "action_signals" }

func actionSignalModelFromCore(s *core.ActionSignal) *ActionSignalModel {
	if s == nil {
		return nil
	}
	return &ActionSignalModel{
		ID:           s.ID,
		StepID:       s.ActionID,
		IssueID:      s.WorkItemID,
		ExecID:       int64PtrIfNonZero(s.RunID),
		Type:         string(s.Type),
		Source:       string(s.Source),
		Summary:      s.Summary,
		Content:      s.Content,
		SourceStepID: int64PtrIfNonZero(s.SourceActionID),
		Payload:      JSONField[map[string]any]{Data: s.Payload},
		Actor:        s.Actor,
		CreatedAt:    s.CreatedAt,
	}
}

func (m *ActionSignalModel) toCore() *core.ActionSignal {
	if m == nil {
		return nil
	}
	sig := &core.ActionSignal{
		ID:         m.ID,
		ActionID:   m.StepID,
		WorkItemID: m.IssueID,
		Type:       core.SignalType(m.Type),
		Source:     core.SignalSource(m.Source),
		Summary:    m.Summary,
		Content:    m.Content,
		Payload:    m.Payload.Data,
		Actor:      m.Actor,
		CreatedAt:  m.CreatedAt,
	}
	if m.ExecID != nil {
		sig.RunID = *m.ExecID
	}
	if m.SourceStepID != nil {
		sig.SourceActionID = *m.SourceStepID
	}
	return sig
}

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
		UpdatedAt: rb.UpdatedAt,
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
		UpdatedAt: m.UpdatedAt,
	}
}

func workItemModelFromCore(w *core.WorkItem) *WorkItemModel {
	if w == nil {
		return nil
	}
	return &WorkItemModel{
		ID:                w.ID,
		ProjectID:         w.ProjectID,
		ResourceBindingID: w.ResourceBindingID,
		Title:             w.Title,
		Body:              w.Body,
		Status:            string(w.Status),
		Priority:          string(w.Priority),
		Labels:            JSONField[[]string]{Data: w.Labels},
		Metadata:          JSONField[map[string]any]{Data: w.Metadata},
		ArchivedAt:        w.ArchivedAt,
		CreatedAt:         w.CreatedAt,
		UpdatedAt:         w.UpdatedAt,
	}
}

func (m *WorkItemModel) toCore() *core.WorkItem {
	if m == nil {
		return nil
	}
	return &core.WorkItem{
		ID:                m.ID,
		ProjectID:         m.ProjectID,
		ResourceBindingID: m.ResourceBindingID,
		Title:             m.Title,
		Body:              m.Body,
		Status:            core.WorkItemStatus(m.Status),
		Priority:          core.WorkItemPriority(m.Priority),
		Labels:            m.Labels.Data,
		Metadata:          m.Metadata.Data,
		ArchivedAt:        m.ArchivedAt,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

func actionModelFromCore(step *core.Action) *ActionModel {
	if step == nil {
		return nil
	}
	return &ActionModel{
		ID:                   step.ID,
		IssueID:              step.WorkItemID,
		Name:                 step.Name,
		Description:          step.Description,
		Type:                 string(step.Type),
		Status:               string(step.Status),
		Position:             step.Position,
		DependsOn:            JSONField[[]int64]{Data: step.DependsOn},
		Input:                step.Input,
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

func (m *ActionModel) toCore() *core.Action {
	if m == nil {
		return nil
	}
	return &core.Action{
		ID:                   m.ID,
		WorkItemID:           m.IssueID,
		Name:                 m.Name,
		Description:          m.Description,
		Type:                 core.ActionType(m.Type),
		Status:               core.ActionStatus(m.Status),
		Position:             m.Position,
		DependsOn:            m.DependsOn.Data,
		Input:                m.Input,
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

func runModelFromCore(exec *core.Run) *RunModel {
	if exec == nil {
		return nil
	}
	return &RunModel{
		ID:               exec.ID,
		StepID:           exec.ActionID,
		IssueID:          exec.WorkItemID,
		Status:           string(exec.Status),
		AgentID:          exec.AgentID,
		AgentContextID:   exec.AgentContextID,
		BriefingSnapshot: exec.BriefingSnapshot,
		ArtifactID:       exec.DeliverableID,
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

func (m *RunModel) toCore() *core.Run {
	if m == nil {
		return nil
	}
	return &core.Run{
		ID:               m.ID,
		ActionID:         m.StepID,
		WorkItemID:       m.IssueID,
		Status:           core.RunStatus(m.Status),
		AgentID:          m.AgentID,
		AgentContextID:   m.AgentContextID,
		BriefingSnapshot: m.BriefingSnapshot,
		DeliverableID:    m.ArtifactID,
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

func deliverableModelFromCore(artifact *core.Deliverable) *DeliverableModel {
	if artifact == nil {
		return nil
	}
	return &DeliverableModel{
		ID:             artifact.ID,
		ExecutionID:    artifact.RunID,
		StepID:         artifact.ActionID,
		IssueID:        artifact.WorkItemID,
		ResultMarkdown: artifact.ResultMarkdown,
		Metadata:       JSONField[map[string]any]{Data: artifact.Metadata},
		Assets:         JSONField[[]core.Asset]{Data: artifact.Assets},
		CreatedAt:      artifact.CreatedAt,
	}
}

func (m *DeliverableModel) toCore() *core.Deliverable {
	if m == nil {
		return nil
	}
	return &core.Deliverable{
		ID:             m.ID,
		RunID:          m.ExecutionID,
		ActionID:       m.StepID,
		WorkItemID:     m.IssueID,
		ResultMarkdown: m.ResultMarkdown,
		Metadata:       m.Metadata.Data,
		Assets:         m.Assets.Data,
		CreatedAt:      m.CreatedAt,
	}
}

func agentContextModelFromCore(ac *core.AgentContext) *AgentContextModel {
	if ac == nil {
		return nil
	}
	return &AgentContextModel{
		ID:               ac.ID,
		AgentID:          ac.AgentID,
		IssueID:          ac.WorkItemID,
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
		WorkItemID:       m.IssueID,
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
		IssueID:   int64PtrIfNonZero(event.WorkItemID),
		StepID:    int64PtrIfNonZero(event.ActionID),
		ExecID:    int64PtrIfNonZero(event.RunID),
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
		event.WorkItemID = *m.IssueID
	}
	if m.StepID != nil {
		event.ActionID = *m.StepID
	}
	if m.ExecID != nil {
		event.RunID = *m.ExecID
	}
	return event
}

func int64PtrIfNonZero(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

func runProbeModelFromCore(probe *core.RunProbe) *RunProbeModel {
	if probe == nil {
		return nil
	}
	return &RunProbeModel{
		ID:             probe.ID,
		ExecutionID:    probe.RunID,
		IssueID:        probe.WorkItemID,
		StepID:         probe.ActionID,
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

func (m *RunProbeModel) toCore() *core.RunProbe {
	if m == nil {
		return nil
	}
	return &core.RunProbe{
		ID:             m.ID,
		RunID:          m.ExecutionID,
		WorkItemID:     m.IssueID,
		ActionID:       m.StepID,
		AgentContextID: m.AgentContextID,
		SessionID:      m.SessionID,
		OwnerID:        m.OwnerID,
		TriggerSource:  core.RunProbeTriggerSource(m.TriggerSource),
		Question:       m.Question,
		Status:         core.RunProbeStatus(m.Status),
		Verdict:        core.RunProbeVerdict(m.Verdict),
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
		ActionsAllowed:   JSONField[[]core.AgentAction]{Data: p.ActionsAllowed},
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
		Steps:       JSONField[[]core.DAGTemplateAction]{Data: t.Actions},
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
		Actions:     m.Steps.Data,
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
		ExecutionID:      r.RunID,
		IssueID:          r.WorkItemID,
		StepID:           r.ActionID,
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
		RunID:            m.ExecutionID,
		WorkItemID:       m.IssueID,
		ActionID:         m.StepID,
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

func toolCallAuditModelFromCore(a *core.ToolCallAudit) *ToolCallAuditModel {
	if a == nil {
		return nil
	}
	return &ToolCallAuditModel{
		ID:             a.ID,
		IssueID:        a.WorkItemID,
		StepID:         a.ActionID,
		ExecutionID:    a.RunID,
		SessionID:      a.SessionID,
		ToolCallID:     a.ToolCallID,
		ToolName:       a.ToolName,
		Status:         a.Status,
		StartedAt:      a.StartedAt,
		FinishedAt:     a.FinishedAt,
		DurationMs:     a.DurationMs,
		ExitCode:       a.ExitCode,
		InputDigest:    a.InputDigest,
		OutputDigest:   a.OutputDigest,
		StdoutDigest:   a.StdoutDigest,
		StderrDigest:   a.StderrDigest,
		InputPreview:   a.InputPreview,
		OutputPreview:  a.OutputPreview,
		StdoutPreview:  a.StdoutPreview,
		StderrPreview:  a.StderrPreview,
		RedactionLevel: a.RedactionLevel,
		CreatedAt:      a.CreatedAt,
	}
}

func (m *ToolCallAuditModel) toCore() *core.ToolCallAudit {
	if m == nil {
		return nil
	}
	return &core.ToolCallAudit{
		ID:             m.ID,
		WorkItemID:     m.IssueID,
		ActionID:       m.StepID,
		RunID:          m.ExecutionID,
		SessionID:      m.SessionID,
		ToolCallID:     m.ToolCallID,
		ToolName:       m.ToolName,
		Status:         m.Status,
		StartedAt:      m.StartedAt,
		FinishedAt:     m.FinishedAt,
		DurationMs:     m.DurationMs,
		ExitCode:       m.ExitCode,
		InputDigest:    m.InputDigest,
		OutputDigest:   m.OutputDigest,
		StdoutDigest:   m.StdoutDigest,
		StderrDigest:   m.StderrDigest,
		InputPreview:   m.InputPreview,
		OutputPreview:  m.OutputPreview,
		StdoutPreview:  m.StdoutPreview,
		StderrPreview:  m.StderrPreview,
		RedactionLevel: m.RedactionLevel,
		CreatedAt:      m.CreatedAt,
	}
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// ActionResource
// ---------------------------------------------------------------------------

type ActionResourceModel struct {
	ID                int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	ActionID          int64                     `gorm:"column:action_id;not null"`
	ResourceBindingID int64                     `gorm:"column:resource_binding_id;not null"`
	Direction         string                    `gorm:"column:direction;not null"`
	Path              string                    `gorm:"column:path;not null"`
	MediaType         string                    `gorm:"column:media_type;not null"`
	Description       string                    `gorm:"column:description;not null"`
	Required          bool                      `gorm:"column:required;not null"`
	Metadata          JSONField[map[string]any] `gorm:"column:metadata;type:text"`
	CreatedAt         time.Time                 `gorm:"column:created_at"`
}

func (ActionResourceModel) TableName() string { return "action_resources" }

func actionResourceModelFromCore(ar *core.ActionResource) *ActionResourceModel {
	if ar == nil {
		return nil
	}
	return &ActionResourceModel{
		ID:                ar.ID,
		ActionID:          ar.ActionID,
		ResourceBindingID: ar.ResourceBindingID,
		Direction:         string(ar.Direction),
		Path:              ar.Path,
		MediaType:         ar.MediaType,
		Description:       ar.Description,
		Required:          ar.Required,
		Metadata:          JSONField[map[string]any]{Data: ar.Metadata},
		CreatedAt:         ar.CreatedAt,
	}
}

func (m *ActionResourceModel) toCore() *core.ActionResource {
	if m == nil {
		return nil
	}
	return &core.ActionResource{
		ID:                m.ID,
		ActionID:          m.ActionID,
		ResourceBindingID: m.ResourceBindingID,
		Direction:         core.ActionResourceDirection(m.Direction),
		Path:              m.Path,
		MediaType:         m.MediaType,
		Description:       m.Description,
		Required:          m.Required,
		Metadata:          m.Metadata.Data,
		CreatedAt:         m.CreatedAt,
	}
}
