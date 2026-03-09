package core

type ProjectFilter struct {
	NameContains string
	Query        string // Natural language query; currently falls back to keyword matching
}

type RunFilter struct {
	Status     RunStatus
	Conclusion RunConclusion
	IssueID    string
	Limit      int
	Offset     int
}

type IssueFilter struct {
	Status    string
	SessionID string
	State     string
	ParentID  string
	Query     string // Natural language query; currently falls back to keyword matching
	Limit     int
	Offset    int
}

type IssueAttachment struct {
	ID        string `json:"id"`
	IssueID   string `json:"issue_id"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	SourceURL string `json:"source_url,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	CreatedAt string `json:"created_at"`
}

type IssueChange struct {
	ID        string `json:"id"`
	IssueID   string `json:"issue_id"`
	Field     string `json:"field"`
	OldValue  string `json:"old_value"`
	NewValue  string `json:"new_value"`
	Reason    string `json:"reason"`
	ChangedBy string `json:"changed_by"`
	CreatedAt string `json:"created_at"`
}

type HumanAction struct {
	ID        int64  `json:"id"`
	RunID     string `json:"run_id"`
	Stage     string `json:"stage"`
	Action    string `json:"action"`
	Message   string `json:"message"`
	Source    string `json:"source"`
	UserID    string `json:"user_id"`
	CreatedAt string `json:"created_at"`
}

type Store interface {
	ListProjects(filter ProjectFilter) ([]Project, error)
	GetProject(id string) (*Project, error)
	CreateProject(p *Project) error
	UpdateProject(p *Project) error
	DeleteProject(id string) error

	ListRuns(projectID string, filter RunFilter) ([]Run, error)
	GetRun(id string) (*Run, error)
	SaveRun(p *Run) error
	GetActiveRuns() ([]Run, error)
	ListRunnableRuns(limit int) ([]Run, error)
	CountInProgressRunsByProject(projectID string) (int, error)
	TryMarkRunInProgress(id string, from ...RunStatus) (bool, error)

	SaveCheckpoint(cp *Checkpoint) error
	GetCheckpoints(RunID string) ([]Checkpoint, error)
	GetLastSuccessCheckpoint(RunID string) (*Checkpoint, error)
	InvalidateCheckpointsFromStage(RunID string, stage StageID) error

	RecordAction(action HumanAction) error
	GetActions(RunID string) ([]HumanAction, error)

	CreateChatSession(s *ChatSession) error
	GetChatSession(id string) (*ChatSession, error)
	UpdateChatSession(s *ChatSession) error
	ListChatSessions(projectID string) ([]ChatSession, error)

	CreateIssue(i *Issue) error
	GetIssue(id string) (*Issue, error)
	SaveIssue(i *Issue) error
	ListIssues(projectID string, filter IssueFilter) ([]Issue, int, error)
	GetActiveIssues(projectID string) ([]Issue, error)
	GetIssueByRun(RunID string) (*Issue, error)
	GetChildIssues(parentID string) ([]Issue, error)
	SaveIssueAttachment(att *IssueAttachment) error
	GetIssueAttachments(issueID string) ([]IssueAttachment, error)
	SaveIssueChange(change *IssueChange) error
	GetIssueChanges(issueID string) ([]IssueChange, error)

	SaveReviewRecord(r *ReviewRecord) error
	GetReviewRecords(issueID string) ([]ReviewRecord, error)

	AppendChatRunEvent(event ChatRunEvent) error
	ListChatRunEvents(sessionID string) ([]ChatRunEvent, error)

	SaveRunEvent(event RunEvent) error
	ListRunEvents(runID string) ([]RunEvent, error)

	// TaskStep event sourcing.
	// SaveTaskStep persists a step and atomically updates Issue.Status if the
	// action implies a state transition. Returns the (possibly new) IssueStatus.
	SaveTaskStep(step *TaskStep) (IssueStatus, error)
	ListTaskSteps(issueID string) ([]TaskStep, error)
	RebuildIssueStatus(issueID string) (IssueStatus, error)

	// ListEvents queries the unified events table with scope/entity filtering.
	ListEvents(filter EventFilter) ([]UnifiedEvent, error)
	// SaveEvent persists an event to the unified events table.
	SaveEvent(event UnifiedEvent) error

	Close() error
}
