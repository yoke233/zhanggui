package core

import "time"

type EventType string

const (
	EventStageStart        EventType = "stage_start"
	EventStageComplete     EventType = "stage_complete"
	EventStageFailed       EventType = "stage_failed"
	EventHumanRequired     EventType = "human_required"
	EventRunDone           EventType = "run_done"
	EventRunWaitingReview  EventType = "run_waiting_review"
	EventRunResumed        EventType = "run_resumed"
	EventActionApplied     EventType = "action_applied"
	EventAgentOutput       EventType = "agent_output"
	EventRunStuck          EventType = "run_stuck"
	EventRunwaiting_review EventType = EventRunWaitingReview // Deprecated: keep for current call sites.
	EventRunstuck          EventType = EventRunStuck         // Deprecated: keep for current call sites.

	// Team Leader and run lifecycle events.
	EventTeamLeaderThinking     EventType = "team_leader_thinking"
	EventTeamLeaderFilesChanged EventType = "team_leader_files_changed"
	EventRunStarted             EventType = "run_started"
	EventRunUpdate              EventType = "run_update"
	EventRunCompleted           EventType = "run_completed"
	EventRunFailed              EventType = "run_failed"
	EventRunCancelled           EventType = "run_cancelled"
	EventIssueCreated           EventType = "issue_created"
	EventIssueReviewing         EventType = "issue_reviewing"
	EventReviewDone             EventType = "review_done"
	EventIssueApproved          EventType = "issue_approved"
	EventIssueQueued            EventType = "issue_queued"
	EventIssueReady             EventType = "issue_ready"
	EventIssueExecuting         EventType = "issue_executing"
	EventIssueDone              EventType = "issue_done"
	EventIssueFailed            EventType = "issue_failed"
	EventIssueDependencyChanged EventType = "issue_dependency_changed"

	// GitHub integration lifecycle events.
	EventGitHubWebhookReceived            EventType = "github_webhook_received"
	EventGitHubIssueOpened                EventType = "github_issue_opened"
	EventGitHubIssueCommentCreated        EventType = "github_issue_comment_created"
	EventGitHubPullRequestReviewSubmitted EventType = "github_pull_request_review_submitted"
	EventGitHubPullRequestClosed          EventType = "github_pull_request_closed"
	EventGitHubReconnected                EventType = "github_reconnected"
	EventAdminOperation                   EventType = "admin_operation"
)

type Event struct {
	Type      EventType         `json:"type"`
	RunID     string            `json:"run_id"`
	ProjectID string            `json:"project_id"`
	IssueID   string            `json:"issue_id,omitempty"`
	Stage     StageID           `json:"stage,omitempty"`
	Agent     string            `json:"agent,omitempty"`
	Data      map[string]string `json:"data,omitempty"`
	Error     string            `json:"error,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

func IsIssueScopedEvent(eventType EventType) bool {
	switch eventType {
	case EventTeamLeaderThinking,
		EventIssueCreated,
		EventIssueReviewing,
		EventReviewDone,
		EventIssueApproved,
		EventIssueQueued,
		EventIssueReady,
		EventIssueExecuting,
		EventIssueDone,
		EventIssueFailed,
		EventIssueDependencyChanged:
		return true
	default:
		return false
	}
}

func IsAlwaysBroadcastIssueEvent(eventType EventType) bool {
	switch eventType {
	case EventIssueCreated,
		EventIssueDone,
		EventIssueFailed:
		return true
	default:
		return false
	}
}
