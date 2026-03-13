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
}
