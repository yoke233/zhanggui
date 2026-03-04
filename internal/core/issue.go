package core

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// IssueState represents tracker-facing open/closed state in V2.
type IssueState string

const (
	IssueStateOpen   IssueState = "open"
	IssueStateClosed IssueState = "closed"
)

// IssueStatus represents orchestration progress for one issue.
type IssueStatus string

const (
	IssueStatusDraft      IssueStatus = "draft"
	IssueStatusReviewing  IssueStatus = "reviewing"
	IssueStatusQueued     IssueStatus = "queued"
	IssueStatusReady      IssueStatus = "ready"
	IssueStatusExecuting  IssueStatus = "executing"
	IssueStatusDone       IssueStatus = "done"
	IssueStatusFailed     IssueStatus = "failed"
	IssueStatusSuperseded IssueStatus = "superseded"
	IssueStatusAbandoned  IssueStatus = "abandoned"
)

var validIssueStates = map[IssueState]struct{}{
	IssueStateOpen:   {},
	IssueStateClosed: {},
}

var validIssueStatuses = map[IssueStatus]struct{}{
	IssueStatusDraft:      {},
	IssueStatusReviewing:  {},
	IssueStatusQueued:     {},
	IssueStatusReady:      {},
	IssueStatusExecuting:  {},
	IssueStatusDone:       {},
	IssueStatusFailed:     {},
	IssueStatusSuperseded: {},
	IssueStatusAbandoned:  {},
}

type FailurePolicy string

const (
	FailBlock FailurePolicy = "block"
	FailSkip  FailurePolicy = "skip"
	FailHuman FailurePolicy = "human"
)

// Issue is the V2 requirement unit and single tracker-facing aggregate.
//
// NOTE: DependsOn/Blocks/RunID are retained as cutover fields during
// transition away from task-plan runtime semantics.
type Issue struct {
	ID           string        `json:"id"`
	ProjectID    string        `json:"project_id"`
	SessionID    string        `json:"session_id"`
	Title        string        `json:"title"`
	Body         string        `json:"body"`
	Labels       []string      `json:"labels"`
	MilestoneID  string        `json:"milestone_id"`
	Attachments  []string      `json:"attachments"`
	DependsOn    []string      `json:"depends_on"`
	Blocks       []string      `json:"blocks"`
	Priority     int           `json:"priority"`
	Template     string        `json:"template"`
	AutoMerge    bool          `json:"auto_merge"`
	State        IssueState    `json:"state"`
	Status       IssueStatus   `json:"status"`
	RunID        string        `json:"run_id"`
	Version      int           `json:"version"`
	SupersededBy string        `json:"superseded_by"`
	ExternalID   string        `json:"external_id"`
	FailPolicy   FailurePolicy `json:"fail_policy"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	ClosedAt     *time.Time    `json:"closed_at,omitempty"`
}

// Validate checks whether the issue state is supported.
func (s IssueState) Validate() error {
	if _, ok := validIssueStates[s]; !ok {
		return fmt.Errorf("invalid issue state %q", s)
	}
	return nil
}

// Validate checks whether the issue status is supported.
func (s IssueStatus) Validate() error {
	if _, ok := validIssueStatuses[s]; !ok {
		return fmt.Errorf("invalid issue status %q", s)
	}
	return nil
}

// NewIssueID generates an ID in format: issue-YYYYMMDD-xxxxxxxx.
func NewIssueID() string {
	return fmt.Sprintf("issue-%s-%s", time.Now().Format("20060102"), randomHex(4))
}

// Validate checks required Issue fields at the domain-model layer.
func (i Issue) Validate() error {
	if strings.TrimSpace(i.Title) == "" {
		return errors.New("issue title is required")
	}
	if strings.TrimSpace(i.Template) == "" {
		return errors.New("issue template is required")
	}
	if strings.ContainsAny(i.Template, " \t\r\n") {
		return errors.New("issue template must not contain spaces")
	}
	if i.State != "" {
		if err := i.State.Validate(); err != nil {
			return err
		}
	}
	if i.Status != "" {
		if err := i.Status.Validate(); err != nil {
			return err
		}
	}
	return nil
}
