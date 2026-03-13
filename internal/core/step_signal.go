package core

import (
	"context"
	"time"
)

// SignalType identifies what the signal declares.
type SignalType string

const (
	// Exec step signals — agent declares outcome.
	SignalComplete SignalType = "complete"  // agent finished the task
	SignalNeedHelp SignalType = "need_help" // agent needs human/lead assistance
	SignalBlocked  SignalType = "blocked"   // agent blocked by external dependency
	SignalProgress SignalType = "progress"  // agent reporting intermediate progress (non-terminal)

	// Gate step signals — reviewer declares verdict.
	SignalApprove SignalType = "approve" // gate passes
	SignalReject  SignalType = "reject"  // gate rejects, triggers rework

	// Human / system signals.
	SignalUnblock  SignalType = "unblock"  // human resolves a blocked step
	SignalOverride SignalType = "override" // human force-overrides the result

	// Interaction record signals — unified step interaction history.
	SignalFeedback    SignalType = "feedback"    // gate/review feedback for rework
	SignalContext     SignalType = "context"     // system context (merge conflict, error info)
	SignalInstruction SignalType = "instruction" // human instruction for agent followup
)

// IsTerminal returns true for signal types that represent a final decision
// (only one terminal signal per execution is accepted).
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

// StepSignal captures an explicit declaration from an agent or human
// about a step's outcome or status change.
type StepSignal struct {
	ID           int64          `json:"id"`
	StepID       int64          `json:"step_id"`
	IssueID      int64          `json:"issue_id"`
	ExecID       int64          `json:"exec_id,omitempty"`
	Type         SignalType     `json:"type"`
	Source       SignalSource   `json:"source"`
	Summary      string         `json:"summary,omitempty"`
	Content      string         `json:"content,omitempty"`
	SourceStepID int64          `json:"source_step_id,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
	Actor        string         `json:"actor,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// StepSignalStore persists StepSignal records.
type StepSignalStore interface {
	CreateStepSignal(ctx context.Context, s *StepSignal) (int64, error)
	GetLatestStepSignal(ctx context.Context, stepID int64, types ...SignalType) (*StepSignal, error)
	ListStepSignals(ctx context.Context, stepID int64) ([]*StepSignal, error)
	ListStepSignalsByType(ctx context.Context, stepID int64, types ...SignalType) ([]*StepSignal, error)
	CountStepSignals(ctx context.Context, stepID int64, types ...SignalType) (int, error)
	ListPendingHumanSteps(ctx context.Context, issueID int64) ([]*Step, error)
	ListAllPendingHumanSteps(ctx context.Context) ([]*Step, error)
}
