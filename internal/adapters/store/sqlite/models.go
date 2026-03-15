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
	IssueID   *int64                    `gorm:"column:issue_id"`
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
	Input            JSONField[map[string]any] `gorm:"column:input;type:text"`
	Output           JSONField[map[string]any] `gorm:"column:output;type:text"`
	ErrorMessage     string                    `gorm:"column:error_message"`
	ErrorKind        string                    `gorm:"column:error_kind"`
	Attempt          int                       `gorm:"column:attempt"`
	StartedAt        *time.Time                `gorm:"column:started_at"`
	FinishedAt       *time.Time                `gorm:"column:finished_at"`
	CreatedAt        time.Time                 `gorm:"column:created_at"`
	ResultMarkdown   string                    `gorm:"column:result_markdown"`
	ResultMetadata   JSONField[map[string]any] `gorm:"column:result_metadata;type:text"`
	ResultAssets     JSONField[[]core.Asset]   `gorm:"column:result_assets;type:text"`
}

func (RunModel) TableName() string { return "executions" }

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
	Category  string                    `gorm:"column:category;not null;default:domain"`
	IssueID   *int64                    `gorm:"column:issue_id"`
	StepID    *int64                    `gorm:"column:step_id"`
	ExecID    *int64                    `gorm:"column:exec_id"`
	Data      JSONField[map[string]any] `gorm:"column:data;type:text"`
	Timestamp time.Time                 `gorm:"column:timestamp"`
}

func (EventModel) TableName() string { return "event_log" }

type AgentProfileModel struct {
	ID               string                        `gorm:"column:id;primaryKey"`
	Name             string                        `gorm:"column:name;not null"`
	DriverConfig     JSONField[core.DriverConfig]  `gorm:"column:driver_config;type:text"`
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

// ── Thread ──

type ThreadModel struct {
	ID        int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	Title     string                    `gorm:"column:title;not null"`
	Status    string                    `gorm:"column:status;not null"`
	OwnerID   string                    `gorm:"column:owner_id;not null"`
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

// ThreadMemberModel persists unified thread members (human + agent).
type ThreadMemberModel struct {
	ID             int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	ThreadID       int64                     `gorm:"column:thread_id;not null"`
	Kind           string                    `gorm:"column:kind;not null"`
	UserID         string                    `gorm:"column:user_id;not null;default:''"`
	AgentProfileID string                    `gorm:"column:agent_profile_id;not null;default:''"`
	Role           string                    `gorm:"column:role;not null;default:'member'"`
	Status         string                    `gorm:"column:status;not null;default:''"`
	AgentData      JSONField[map[string]any] `gorm:"column:agent_data;type:text"`
	JoinedAt       time.Time                 `gorm:"column:joined_at"`
	LastActiveAt   time.Time                 `gorm:"column:last_active_at"`
}

func (ThreadMemberModel) TableName() string { return "thread_members" }

func (m *ThreadMemberModel) toCore() *core.ThreadMember {
	if m == nil {
		return nil
	}
	return &core.ThreadMember{
		ID:             m.ID,
		ThreadID:       m.ThreadID,
		Kind:           m.Kind,
		UserID:         m.UserID,
		AgentProfileID: m.AgentProfileID,
		Role:           m.Role,
		Status:         core.ThreadAgentStatus(m.Status),
		AgentData:      m.AgentData.Data,
		JoinedAt:       m.JoinedAt,
		LastActiveAt:   m.LastActiveAt,
	}
}

func threadMemberModelFromCore(m *core.ThreadMember) *ThreadMemberModel {
	if m == nil {
		return nil
	}
	return &ThreadMemberModel{
		ID:             m.ID,
		ThreadID:       m.ThreadID,
		Kind:           m.Kind,
		UserID:         m.UserID,
		AgentProfileID: m.AgentProfileID,
		Role:           m.Role,
		Status:         string(m.Status),
		AgentData:      JSONField[map[string]any]{Data: m.AgentData},
		JoinedAt:       m.JoinedAt,
		LastActiveAt:   m.LastActiveAt,
	}
}

// ThreadWorkItemLinkModel persists thread-workitem links.
type ThreadWorkItemLinkModel struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ThreadID     int64     `gorm:"column:thread_id;not null;uniqueIndex:idx_thread_work_item_links_unique"`
	WorkItemID   int64     `gorm:"column:work_item_id;not null;uniqueIndex:idx_thread_work_item_links_unique"`
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

type ThreadContextRefModel struct {
	ID        int64      `gorm:"column:id;primaryKey;autoIncrement"`
	ThreadID  int64      `gorm:"column:thread_id;not null;uniqueIndex:idx_thread_context_refs_thread_project"`
	ProjectID int64      `gorm:"column:project_id;not null;uniqueIndex:idx_thread_context_refs_thread_project"`
	Access    string     `gorm:"column:access;not null;default:read"`
	Note      string     `gorm:"column:note;not null;default:''"`
	GrantedBy string     `gorm:"column:granted_by;not null;default:''"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	ExpiresAt *time.Time `gorm:"column:expires_at"`
}

func (ThreadContextRefModel) TableName() string { return "thread_context_refs" }

type ThreadAttachmentModel struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ThreadID    int64     `gorm:"column:thread_id;not null;index"`
	MessageID   *int64    `gorm:"column:message_id"`
	FileName    string    `gorm:"column:file_name;not null"`
	FilePath    string    `gorm:"column:file_path;not null"`
	FileSize    int64     `gorm:"column:file_size;not null;default:0"`
	ContentType string    `gorm:"column:content_type;not null;default:''"`
	IsDirectory bool      `gorm:"column:is_directory;not null;default:false"`
	UploadedBy  string    `gorm:"column:uploaded_by;not null;default:''"`
	Note        string    `gorm:"column:note;not null;default:''"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (ThreadAttachmentModel) TableName() string { return "thread_attachments" }

func (m *ThreadAttachmentModel) toCore() *core.ThreadAttachment {
	if m == nil {
		return nil
	}
	return &core.ThreadAttachment{
		ID:          m.ID,
		ThreadID:    m.ThreadID,
		MessageID:   m.MessageID,
		FileName:    m.FileName,
		FilePath:    m.FilePath,
		FileSize:    m.FileSize,
		ContentType: m.ContentType,
		IsDirectory: m.IsDirectory,
		UploadedBy:  m.UploadedBy,
		Note:        m.Note,
		CreatedAt:   m.CreatedAt,
	}
}

func threadAttachmentModelFromCore(a *core.ThreadAttachment) *ThreadAttachmentModel {
	if a == nil {
		return nil
	}
	return &ThreadAttachmentModel{
		ID:          a.ID,
		ThreadID:    a.ThreadID,
		MessageID:   a.MessageID,
		FileName:    a.FileName,
		FilePath:    a.FilePath,
		FileSize:    a.FileSize,
		ContentType: a.ContentType,
		IsDirectory: a.IsDirectory,
		UploadedBy:  a.UploadedBy,
		Note:        a.Note,
		CreatedAt:   a.CreatedAt,
	}
}

func (m *ThreadContextRefModel) toCore() *core.ThreadContextRef {
	if m == nil {
		return nil
	}
	return &core.ThreadContextRef{
		ID:        m.ID,
		ThreadID:  m.ThreadID,
		ProjectID: m.ProjectID,
		Access:    core.ContextAccess(m.Access),
		Note:      m.Note,
		GrantedBy: m.GrantedBy,
		CreatedAt: m.CreatedAt,
		ExpiresAt: m.ExpiresAt,
	}
}

func threadContextRefModelFromCore(ref *core.ThreadContextRef) *ThreadContextRefModel {
	if ref == nil {
		return nil
	}
	return &ThreadContextRefModel{
		ID:        ref.ID,
		ThreadID:  ref.ThreadID,
		ProjectID: ref.ProjectID,
		Access:    string(ref.Access),
		Note:      ref.Note,
		GrantedBy: ref.GrantedBy,
		CreatedAt: ref.CreatedAt,
		ExpiresAt: ref.ExpiresAt,
	}
}

// ThreadTaskGroupModel is the GORM model for thread_task_groups table.
type ThreadTaskGroupModel struct {
	ID                     int64      `gorm:"column:id;primaryKey;autoIncrement"`
	ThreadID               int64      `gorm:"column:thread_id;not null;index:idx_thread_task_groups_thread"`
	Status                 string     `gorm:"column:status;not null;default:'pending'"`
	SourceMessageID        *int64     `gorm:"column:source_message_id"`
	StatusMessageID        *int64     `gorm:"column:status_message_id"`
	NotifyOnComplete       bool       `gorm:"column:notify_on_complete;not null;default:true"`
	MaterializeToWorkItem  bool       `gorm:"column:materialize_to_workitem;not null;default:false"`
	MaterializedWorkItemID *int64     `gorm:"column:materialized_work_item_id"`
	CreatedAt              time.Time  `gorm:"column:created_at"`
	CompletedAt            *time.Time `gorm:"column:completed_at"`
}

func (ThreadTaskGroupModel) TableName() string { return "thread_task_groups" }

func threadTaskGroupModelFromCore(g *core.ThreadTaskGroup) *ThreadTaskGroupModel {
	if g == nil {
		return nil
	}
	return &ThreadTaskGroupModel{
		ID:                     g.ID,
		ThreadID:               g.ThreadID,
		Status:                 string(g.Status),
		SourceMessageID:        g.SourceMessageID,
		StatusMessageID:        g.StatusMessageID,
		NotifyOnComplete:       g.NotifyOnComplete,
		MaterializeToWorkItem:  g.MaterializeToWorkItem,
		MaterializedWorkItemID: g.MaterializedWorkItemID,
		CreatedAt:              g.CreatedAt,
		CompletedAt:            g.CompletedAt,
	}
}

func (m *ThreadTaskGroupModel) toCore() *core.ThreadTaskGroup {
	if m == nil {
		return nil
	}
	return &core.ThreadTaskGroup{
		ID:                     m.ID,
		ThreadID:               m.ThreadID,
		Status:                 core.TaskGroupStatus(m.Status),
		SourceMessageID:        m.SourceMessageID,
		StatusMessageID:        m.StatusMessageID,
		NotifyOnComplete:       m.NotifyOnComplete,
		MaterializeToWorkItem:  m.MaterializeToWorkItem,
		MaterializedWorkItemID: m.MaterializedWorkItemID,
		CreatedAt:              m.CreatedAt,
		CompletedAt:            m.CompletedAt,
	}
}

// ThreadTaskModel is the GORM model for thread_tasks table.
type ThreadTaskModel struct {
	ID              int64              `gorm:"column:id;primaryKey;autoIncrement"`
	GroupID         int64              `gorm:"column:group_id;not null;index:idx_thread_tasks_group"`
	ThreadID        int64              `gorm:"column:thread_id;not null;index:idx_thread_tasks_thread"`
	Assignee        string             `gorm:"column:assignee;not null"`
	Type            string             `gorm:"column:type;not null;default:'work'"`
	Instruction     string             `gorm:"column:instruction;not null"`
	DependsOnJSON   JSONField[[]int64] `gorm:"column:depends_on_json;type:text"`
	Status          string             `gorm:"column:status;not null;default:'pending'"`
	OutputFilePath  string             `gorm:"column:output_file_path;not null;default:''"`
	OutputMessageID *int64             `gorm:"column:output_message_id"`
	ReviewFeedback  string             `gorm:"column:review_feedback;not null;default:''"`
	MaxRetries      int                `gorm:"column:max_retries;not null;default:0"`
	RetryCount      int                `gorm:"column:retry_count;not null;default:0"`
	CreatedAt       time.Time          `gorm:"column:created_at"`
	CompletedAt     *time.Time         `gorm:"column:completed_at"`
}

func (ThreadTaskModel) TableName() string { return "thread_tasks" }

func threadTaskModelFromCore(t *core.ThreadTask) *ThreadTaskModel {
	if t == nil {
		return nil
	}
	deps := t.DependsOn
	if deps == nil {
		deps = []int64{}
	}
	return &ThreadTaskModel{
		ID:              t.ID,
		GroupID:         t.GroupID,
		ThreadID:        t.ThreadID,
		Assignee:        t.Assignee,
		Type:            string(t.Type),
		Instruction:     t.Instruction,
		DependsOnJSON:   JSONField[[]int64]{Data: deps},
		Status:          string(t.Status),
		OutputFilePath:  t.OutputFilePath,
		OutputMessageID: t.OutputMessageID,
		ReviewFeedback:  t.ReviewFeedback,
		MaxRetries:      t.MaxRetries,
		RetryCount:      t.RetryCount,
		CreatedAt:       t.CreatedAt,
		CompletedAt:     t.CompletedAt,
	}
}

func (m *ThreadTaskModel) toCore() *core.ThreadTask {
	if m == nil {
		return nil
	}
	deps := m.DependsOnJSON.Data
	if deps == nil {
		deps = []int64{}
	}
	return &core.ThreadTask{
		ID:              m.ID,
		GroupID:         m.GroupID,
		ThreadID:        m.ThreadID,
		Assignee:        m.Assignee,
		Type:            core.TaskType(m.Type),
		Instruction:     m.Instruction,
		DependsOn:       deps,
		Status:          core.ThreadTaskStatus(m.Status),
		OutputFilePath:  m.OutputFilePath,
		OutputMessageID: m.OutputMessageID,
		ReviewFeedback:  m.ReviewFeedback,
		MaxRetries:      m.MaxRetries,
		RetryCount:      m.RetryCount,
		CreatedAt:       m.CreatedAt,
		CompletedAt:     m.CompletedAt,
	}
}

// FeatureEntryModel is the GORM model for feature_entries table.
type FeatureEntryModel struct {
	ID          int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	ProjectID   int64                     `gorm:"column:project_id;not null;uniqueIndex:idx_feature_entries_project_key"`
	Key         string                    `gorm:"column:key;not null;uniqueIndex:idx_feature_entries_project_key"`
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
		ProjectID:   m.ProjectID,
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
		ProjectID:   e.ProjectID,
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

// ── ActivityJournal ──

type JournalModel struct {
	ID             int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	WorkItemID     *int64                    `gorm:"column:work_item_id"`
	ActionID       *int64                    `gorm:"column:action_id"`
	RunID          *int64                    `gorm:"column:run_id"`
	Kind           string                    `gorm:"column:kind;not null"`
	Source         string                    `gorm:"column:source;not null;default:system"`
	Summary        string                    `gorm:"column:summary;not null;default:''"`
	Payload        JSONField[map[string]any] `gorm:"column:payload;type:text"`
	Ref            *string                   `gorm:"column:ref"`
	Actor          string                    `gorm:"column:actor;not null;default:''"`
	SourceActionID *int64                    `gorm:"column:source_action_id"`
	CreatedAt      time.Time                 `gorm:"column:created_at"`
}

func (JournalModel) TableName() string { return "activity_journal" }

func journalModelFromCore(e *core.JournalEntry) *JournalModel {
	if e == nil {
		return nil
	}
	m := &JournalModel{
		ID:             e.ID,
		WorkItemID:     int64PtrIfNonZero(e.WorkItemID),
		ActionID:       int64PtrIfNonZero(e.ActionID),
		RunID:          int64PtrIfNonZero(e.RunID),
		Kind:           string(e.Kind),
		Source:         string(e.Source),
		Summary:        e.Summary,
		Payload:        JSONField[map[string]any]{Data: e.Payload},
		Actor:          e.Actor,
		SourceActionID: int64PtrIfNonZero(e.SourceActionID),
		CreatedAt:      e.CreatedAt,
	}
	if e.Ref != "" {
		m.Ref = &e.Ref
	}
	return m
}

func (m *JournalModel) toCore() *core.JournalEntry {
	if m == nil {
		return nil
	}
	e := &core.JournalEntry{
		ID:        m.ID,
		Kind:      core.JournalKind(m.Kind),
		Source:    core.JournalSource(m.Source),
		Summary:   m.Summary,
		Payload:   m.Payload.Data,
		Actor:     m.Actor,
		CreatedAt: m.CreatedAt,
	}
	if m.WorkItemID != nil {
		e.WorkItemID = *m.WorkItemID
	}
	if m.ActionID != nil {
		e.ActionID = *m.ActionID
	}
	if m.RunID != nil {
		e.RunID = *m.RunID
	}
	if m.Ref != nil {
		e.Ref = *m.Ref
	}
	if m.SourceActionID != nil {
		e.SourceActionID = *m.SourceActionID
	}
	return e
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
		IssueID:   rb.IssueID,
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
		IssueID:   m.IssueID,
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
		DependsOn:         JSONField[[]int64]{Data: w.DependsOn},
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
		DependsOn:         m.DependsOn.Data,
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
		Input:            JSONField[map[string]any]{Data: exec.Input},
		Output:           JSONField[map[string]any]{Data: exec.Output},
		ErrorMessage:     exec.ErrorMessage,
		ErrorKind:        string(exec.ErrorKind),
		Attempt:          exec.Attempt,
		StartedAt:        exec.StartedAt,
		FinishedAt:       exec.FinishedAt,
		CreatedAt:        exec.CreatedAt,
		ResultMarkdown:   exec.ResultMarkdown,
		ResultMetadata:   JSONField[map[string]any]{Data: exec.ResultMetadata},
		ResultAssets:     JSONField[[]core.Asset]{Data: exec.ResultAssets},
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
		Input:            m.Input.Data,
		Output:           m.Output.Data,
		ErrorMessage:     m.ErrorMessage,
		ErrorKind:        core.ErrorKind(m.ErrorKind),
		Attempt:          m.Attempt,
		StartedAt:        m.StartedAt,
		FinishedAt:       m.FinishedAt,
		CreatedAt:        m.CreatedAt,
		ResultMarkdown:   m.ResultMarkdown,
		ResultMetadata:   m.ResultMetadata.Data,
		ResultAssets:     m.ResultAssets.Data,
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
	category := event.Category
	if category == "" {
		category = core.EventCategoryDomain
	}
	return &EventModel{
		ID:        event.ID,
		Type:      string(event.Type),
		Category:  category,
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
		Category:  m.Category,
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

func agentProfileModelFromCore(p *core.AgentProfile) *AgentProfileModel {
	if p == nil {
		return nil
	}
	return &AgentProfileModel{
		ID:               p.ID,
		Name:             p.Name,
		DriverConfig:     JSONField[core.DriverConfig]{Data: p.Driver},
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
		Driver:         m.DriverConfig.Data,
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
