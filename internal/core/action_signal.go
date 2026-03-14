package core

import (
	"context"
	"time"
)

// SignalType identifies what the signal declares.
type SignalType string

const (
	// Exec action signals -- agent declares outcome.
	SignalComplete SignalType = "complete"  // agent finished the task
	SignalNeedHelp SignalType = "need_help" // agent needs human/lead assistance
	SignalBlocked  SignalType = "blocked"   // agent blocked by external dependency
	SignalProgress SignalType = "progress"  // agent reporting intermediate progress (non-terminal)

	// Gate action signals -- reviewer declares verdict.
	SignalApprove SignalType = "approve" // gate passes
	SignalReject  SignalType = "reject"  // gate rejects, triggers rework

	// Human / system signals.
	SignalUnblock  SignalType = "unblock"  // human resolves a blocked action
	SignalOverride SignalType = "override" // human force-overrides the result

	// Interaction record signals -- unified action interaction history.
	SignalFeedback    SignalType = "feedback"    // gate/review feedback for rework
	SignalContext     SignalType = "context"     // system context (merge conflict, error info)
	SignalInstruction SignalType = "instruction" // human instruction for agent followup

	// Probe signals -- diagnostic side-channel for running runs.
	SignalProbeRequest  SignalType = "probe_request"  // outbound probe question
	SignalProbeResponse SignalType = "probe_response" // inbound probe answer / terminal state
)

// IsTerminal returns true for signal types that represent a final decision
// (only one terminal signal per run is accepted).
func (t SignalType) IsTerminal() bool {
	switch t {
	case SignalComplete, SignalNeedHelp, SignalBlocked, SignalApprove, SignalReject:
		return true
	}
	return false
}

// SignalSource distinguishes who emitted the signal.
type SignalSource string

const (
	SignalSourceAgent  SignalSource = "agent"
	SignalSourceHuman  SignalSource = "human"
	SignalSourceSystem SignalSource = "system"
)

// ActionSignal captures an explicit declaration from an agent or human
// about an action's outcome or status change.
type ActionSignal struct {
	ID             int64          `json:"id"`
	ActionID       int64          `json:"action_id"`
	WorkItemID     int64          `json:"work_item_id"`
	RunID          int64          `json:"run_id,omitempty"`
	Type           SignalType     `json:"type"`
	Source         SignalSource   `json:"source"`
	Summary        string         `json:"summary,omitempty"`
	Content        string         `json:"content,omitempty"`
	SourceActionID int64          `json:"source_action_id,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
	Actor          string         `json:"actor,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// ActionSignalStore persists ActionSignal records.
type ActionSignalStore interface {
	CreateActionSignal(ctx context.Context, s *ActionSignal) (int64, error)
	GetLatestActionSignal(ctx context.Context, actionID int64, types ...SignalType) (*ActionSignal, error)
	ListActionSignals(ctx context.Context, actionID int64) ([]*ActionSignal, error)
	ListActionSignalsByType(ctx context.Context, actionID int64, types ...SignalType) ([]*ActionSignal, error)
	CountActionSignals(ctx context.Context, actionID int64, types ...SignalType) (int, error)
	ListPendingHumanActions(ctx context.Context, workItemID int64) ([]*Action, error)
	ListAllPendingHumanActions(ctx context.Context) ([]*Action, error)

	// Probe signal queries (merged from RunProbeStore).
	ListProbeSignalsByRun(ctx context.Context, runID int64) ([]*ActionSignal, error)
	GetLatestProbeSignal(ctx context.Context, runID int64) (*ActionSignal, error)
	GetActiveProbeSignal(ctx context.Context, runID int64) (*ActionSignal, error)
	UpdateProbeSignal(ctx context.Context, sig *ActionSignal) error
	GetRunProbeRoute(ctx context.Context, runID int64) (*RunProbeRoute, error)
}

// ── Probe ↔ Signal conversion helpers ──

// newProbeSignalPayload builds the shared payload map from a RunProbe.
func newProbeSignalPayload(probe *RunProbe) map[string]any {
	payload := map[string]any{
		"trigger_source": string(probe.TriggerSource),
		"question":       probe.Question,
		"status":         string(probe.Status),
		"verdict":        string(probe.Verdict),
	}
	if probe.AgentContextID != nil {
		payload["agent_context_id"] = *probe.AgentContextID
	}
	if probe.SessionID != "" {
		payload["session_id"] = probe.SessionID
	}
	if probe.OwnerID != "" {
		payload["owner_id"] = probe.OwnerID
	}
	if probe.Error != "" {
		payload["error"] = probe.Error
	}
	if probe.SentAt != nil {
		payload["sent_at"] = probe.SentAt.Format(time.RFC3339Nano)
	}
	return payload
}

// NewProbeRequestSignal converts a RunProbe into an ActionSignal with type=probe_request.
func NewProbeRequestSignal(probe *RunProbe) *ActionSignal {
	payload := newProbeSignalPayload(probe)
	return &ActionSignal{
		ActionID:   probe.ActionID,
		WorkItemID: probe.WorkItemID,
		RunID:      probe.RunID,
		Type:       SignalProbeRequest,
		Source:     SignalSourceSystem,
		Summary:    probe.Question,
		Content:    probe.Question,
		Payload:    payload,
	}
}

// NewProbeResponseSignal converts a RunProbe into an ActionSignal with type=probe_response.
func NewProbeResponseSignal(probe *RunProbe) *ActionSignal {
	payload := newProbeSignalPayload(probe)
	if probe.ReplyText != "" {
		payload["reply_text"] = probe.ReplyText
	}
	if probe.AnsweredAt != nil {
		payload["answered_at"] = probe.AnsweredAt.Format(time.RFC3339Nano)
	}
	return &ActionSignal{
		ActionID:   probe.ActionID,
		WorkItemID: probe.WorkItemID,
		RunID:      probe.RunID,
		Type:       SignalProbeResponse,
		Source:     SignalSourceSystem,
		Summary:    string(probe.Verdict),
		Content:    probe.ReplyText,
		Payload:    payload,
	}
}

// ProbeFromSignal converts an ActionSignal (probe_request or probe_response) back to a RunProbe.
func ProbeFromSignal(sig *ActionSignal) *RunProbe {
	if sig == nil {
		return nil
	}
	p := &RunProbe{
		ID:         sig.ID,
		RunID:      sig.RunID,
		WorkItemID: sig.WorkItemID,
		ActionID:   sig.ActionID,
		CreatedAt:  sig.CreatedAt,
	}
	if v, ok := sig.Payload["trigger_source"].(string); ok {
		p.TriggerSource = RunProbeTriggerSource(v)
	}
	if v, ok := sig.Payload["question"].(string); ok {
		p.Question = v
	}
	if v, ok := sig.Payload["status"].(string); ok {
		p.Status = RunProbeStatus(v)
	}
	if v, ok := sig.Payload["verdict"].(string); ok {
		p.Verdict = RunProbeVerdict(v)
	}
	if v, ok := sig.Payload["session_id"].(string); ok {
		p.SessionID = v
	}
	if v, ok := sig.Payload["owner_id"].(string); ok {
		p.OwnerID = v
	}
	if v, ok := sig.Payload["reply_text"].(string); ok {
		p.ReplyText = v
	}
	if v, ok := sig.Payload["error"].(string); ok {
		p.Error = v
	}
	// agent_context_id can be float64 (from JSON) or int64.
	if v, ok := sig.Payload["agent_context_id"].(float64); ok {
		id := int64(v)
		p.AgentContextID = &id
	}
	if v, ok := sig.Payload["sent_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			p.SentAt = &t
		}
	}
	if v, ok := sig.Payload["answered_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			p.AnsweredAt = &t
		}
	}
	return p
}
