package core

import "time"

type EventType string

const (
	EventStageStart      EventType = "stage_start"
	EventStageComplete   EventType = "stage_complete"
	EventStageFailed     EventType = "stage_failed"
	EventHumanRequired   EventType = "human_required"
	EventPipelineDone    EventType = "pipeline_done"
	EventPipelineFailed  EventType = "pipeline_failed"
	EventPipelinePaused  EventType = "pipeline_paused"
	EventPipelineResumed EventType = "pipeline_resumed"
	EventActionApplied   EventType = "action_applied"
	EventAgentOutput     EventType = "agent_output"
	EventPipelineStuck   EventType = "pipeline_stuck"

	// Secretary lifecycle events.
	EventSecretaryThinking EventType = "secretary_thinking"
	EventPlanCreated       EventType = "plan_created"
	EventPlanReviewing     EventType = "plan_reviewing"
	EventReviewAgentDone   EventType = "review_agent_done"
	EventReviewComplete    EventType = "review_complete"
	EventPlanApproved      EventType = "plan_approved"
	EventPlanWaitingHuman  EventType = "plan_waiting_human"
	EventTaskReady         EventType = "task_ready"
	EventTaskRunning       EventType = "task_running"
	EventTaskDone          EventType = "task_done"
	EventTaskFailed        EventType = "task_failed"
	EventPlanDone          EventType = "plan_done"
	EventPlanFailed        EventType = "plan_failed"
	EventPlanPartiallyDone EventType = "plan_partially_done"

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
	Type       EventType         `json:"type"`
	PipelineID string            `json:"pipeline_id"`
	ProjectID  string            `json:"project_id"`
	PlanID     string            `json:"plan_id,omitempty"`
	Stage      StageID           `json:"stage,omitempty"`
	Agent      string            `json:"agent,omitempty"`
	Data       map[string]string `json:"data,omitempty"`
	Error      string            `json:"error,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
}

func IsPlanScopedEvent(eventType EventType) bool {
	switch eventType {
	case EventSecretaryThinking,
		EventPlanCreated,
		EventPlanReviewing,
		EventReviewAgentDone,
		EventReviewComplete,
		EventPlanApproved,
		EventPlanWaitingHuman,
		EventTaskReady,
		EventTaskRunning,
		EventTaskDone,
		EventTaskFailed,
		EventPlanDone,
		EventPlanFailed,
		EventPlanPartiallyDone:
		return true
	default:
		return false
	}
}

func IsAlwaysBroadcastPlanEvent(eventType EventType) bool {
	switch eventType {
	case EventPlanCreated,
		EventPlanDone,
		EventPlanFailed,
		EventPlanPartiallyDone:
		return true
	default:
		return false
	}
}
