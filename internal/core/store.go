package core

type ProjectFilter struct {
	NameContains string
}

type RunFilter struct {
	Status RunStatus
	Limit  int
	Offset int
}

type IssueFilter struct {
	Status    string
	SessionID string
	State     string
	Limit     int
	Offset    int
}

type IssueAttachment struct {
	ID        string `json:"id"`
	IssueID   string `json:"issue_id"`
	Path      string `json:"path"`
	Content   string `json:"content"`
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
	SaveIssueAttachment(issueID, path, content string) error
	GetIssueAttachments(issueID string) ([]IssueAttachment, error)
	SaveIssueChange(change *IssueChange) error
	GetIssueChanges(issueID string) ([]IssueChange, error)

	SaveReviewRecord(r *ReviewRecord) error
	GetReviewRecords(issueID string) ([]ReviewRecord, error)

	AppendChatRunEvent(event ChatRunEvent) error
	ListChatRunEvents(sessionID string) ([]ChatRunEvent, error)

	SaveRunEvent(event RunEvent) error
	ListRunEvents(runID string) ([]RunEvent, error)

	Close() error
}
