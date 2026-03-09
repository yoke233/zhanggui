package core

import (
	"context"
	"fmt"
	"time"
)

type GateType string

const (
	GateTypeAuto        GateType = "auto"
	GateTypeOwnerReview GateType = "owner_review"
	GateTypePeerReview  GateType = "peer_review"
	GateTypeVote        GateType = "vote"
)

type GateFallback string

const (
	GateFallbackEscalate  GateFallback = "escalate"
	GateFallbackForcePass GateFallback = "force_pass"
	GateFallbackAbort     GateFallback = "abort"
)

type GateStatus string

const (
	GateStatusPending GateStatus = "pending"
	GateStatusPassed  GateStatus = "passed"
	GateStatusFailed  GateStatus = "failed"
	GateStatusSkipped GateStatus = "skipped"
)

// Gate defines a single checkpoint in the review pipeline.
type Gate struct {
	Name        string       `json:"name"`
	Type        GateType     `json:"type"`
	Rules       string       `json:"rules"`
	MaxAttempts int          `json:"max_attempts,omitempty"`
	Fallback    GateFallback `json:"fallback,omitempty"`
}

// GateCheck records one attempt at passing a gate.
type GateCheck struct {
	ID         string     `json:"id"`
	IssueID    string     `json:"issue_id"`
	GateName   string     `json:"gate_name"`
	GateType   GateType   `json:"gate_type"`
	Attempt    int        `json:"attempt"`
	Status     GateStatus `json:"status"`
	Reason     string     `json:"reason"`
	DecisionID string     `json:"decision_id,omitempty"`
	CheckedBy  string     `json:"checked_by"`
	CreatedAt  time.Time  `json:"created_at"`
}

func NewGateCheckID() string {
	return fmt.Sprintf("gc-%s-%s", time.Now().Format("20060102-150405"), randomHex(4))
}

// GateRunner evaluates a single gate for an issue.
type GateRunner interface {
	Check(ctx context.Context, issue *Issue, gate Gate, attempt int) (*GateCheck, error)
}

// Validate checks Gate fields.
func (g Gate) Validate() error {
	if g.Name == "" {
		return fmt.Errorf("gate name is required")
	}
	switch g.Type {
	case GateTypeAuto, GateTypeOwnerReview, GateTypePeerReview, GateTypeVote:
	default:
		return fmt.Errorf("invalid gate type %q", g.Type)
	}
	if g.MaxAttempts < 0 {
		return fmt.Errorf("max_attempts must be >= 0")
	}
	if g.Fallback != "" {
		switch g.Fallback {
		case GateFallbackEscalate, GateFallbackForcePass, GateFallbackAbort:
		default:
			return fmt.Errorf("invalid gate fallback %q", g.Fallback)
		}
	}
	return nil
}

// ValidateGates checks a gate chain for validity.
func ValidateGates(gates []Gate) error {
	names := make(map[string]struct{}, len(gates))
	for i, g := range gates {
		if err := g.Validate(); err != nil {
			return fmt.Errorf("gate[%d]: %w", i, err)
		}
		if _, exists := names[g.Name]; exists {
			return fmt.Errorf("gate[%d]: duplicate gate name %q", i, g.Name)
		}
		names[g.Name] = struct{}{}
	}
	return nil
}
