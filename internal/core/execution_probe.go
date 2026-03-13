package core

import "time"

// RunProbeTriggerSource identifies how a probe was initiated.
type RunProbeTriggerSource string

const (
	RunProbeTriggerWatchdog RunProbeTriggerSource = "watchdog"
	RunProbeTriggerManual   RunProbeTriggerSource = "manual"
)

// RunProbeStatus tracks the runtime lifecycle of a probe.
type RunProbeStatus string

const (
	RunProbePending     RunProbeStatus = "pending"
	RunProbeSent        RunProbeStatus = "sent"
	RunProbeAnswered    RunProbeStatus = "answered"
	RunProbeTimeout     RunProbeStatus = "timeout"
	RunProbeUnreachable RunProbeStatus = "unreachable"
	RunProbeFailed      RunProbeStatus = "failed"
)

// RunProbeVerdict summarizes what the probe implies about a running run.
type RunProbeVerdict string

const (
	RunProbeAlive   RunProbeVerdict = "alive"
	RunProbeBlocked RunProbeVerdict = "blocked"
	RunProbeHung    RunProbeVerdict = "hung"
	RunProbeDead    RunProbeVerdict = "dead"
	RunProbeUnknown RunProbeVerdict = "unknown"
)

// RunProbe stores a single side-channel diagnostic interaction for a running run.
type RunProbe struct {
	ID             int64                 `json:"id"`
	RunID          int64                 `json:"run_id"`
	WorkItemID     int64                 `json:"work_item_id"`
	ActionID       int64                 `json:"action_id"`
	AgentContextID *int64                `json:"agent_context_id,omitempty"`
	SessionID      string                `json:"-"`
	OwnerID        string                `json:"owner_id,omitempty"`
	TriggerSource  RunProbeTriggerSource `json:"trigger_source"`
	Question       string                `json:"question"`
	Status         RunProbeStatus        `json:"status"`
	Verdict        RunProbeVerdict       `json:"verdict"`
	ReplyText      string                `json:"reply_text,omitempty"`
	Error          string                `json:"error,omitempty"`
	SentAt         *time.Time            `json:"sent_at,omitempty"`
	AnsweredAt     *time.Time            `json:"answered_at,omitempty"`
	CreatedAt      time.Time             `json:"created_at"`
}

// RunProbeRoute resolves the internal runtime route for probing a run.
type RunProbeRoute struct {
	RunID           int64
	WorkItemID      int64
	ActionID        int64
	AgentContextID  *int64
	SessionID       string
	OwnerID         string
	OwnerLastSeenAt *time.Time
}
