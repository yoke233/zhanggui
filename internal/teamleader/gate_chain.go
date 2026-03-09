package teamleader

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// GateStore is the persistence interface for gate checks.
type GateStore interface {
	SaveGateCheck(gc *core.GateCheck) error
	GetGateChecks(issueID string) ([]core.GateCheck, error)
	GetLatestGateCheck(issueID, gateName string) (*core.GateCheck, error)
	SaveTaskStep(step *core.TaskStep) (core.IssueStatus, error)
}

type gateDecisionStore interface {
	SaveDecision(d *core.Decision) error
}

// GateChain executes a sequence of gates for an issue.
type GateChain struct {
	Store   GateStore
	Runners map[core.GateType]core.GateRunner
}

// GateChainResult holds the outcome of running a gate chain.
type GateChainResult struct {
	AllPassed   bool
	PendingGate string
	FailedCheck *core.GateCheck
	ForcePassed bool
}

// Run executes gates sequentially. For each gate it invokes the matching
// runner and retries up to MaxAttempts on failure. When all attempts are
// exhausted, the gate's Fallback strategy decides the outcome.
func (c *GateChain) Run(ctx context.Context, issue *core.Issue, gates []core.Gate) (*GateChainResult, error) {
	if issue == nil {
		return nil, fmt.Errorf("issue is required")
	}
	if c == nil || c.Store == nil {
		return nil, fmt.Errorf("gate chain store is required")
	}
	if err := core.ValidateGates(gates); err != nil {
		return nil, fmt.Errorf("validate gates: %w", err)
	}
	if len(gates) == 0 {
		return &GateChainResult{AllPassed: true}, nil
	}

	for _, gate := range gates {
		runner, ok := c.Runners[gate.Type]
		if !ok {
			return nil, fmt.Errorf("gate %q: no runner registered for type %q", gate.Name, gate.Type)
		}

		maxAttempts := gate.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = 1
		}
		for attempt := 1; ; attempt++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			check, err := runner.Check(ctx, issue, gate, attempt)
			if err != nil {
				return nil, fmt.Errorf("gate %q attempt %d: %w", gate.Name, attempt, err)
			}

			if err := c.saveDecision(issue, gate, check); err != nil {
				return nil, err
			}
			if err := c.Store.SaveGateCheck(check); err != nil {
				return nil, fmt.Errorf("save gate check %q attempt %d: %w", gate.Name, attempt, err)
			}

			switch check.Status {
			case core.GateStatusPassed:
				if err := c.recordStep(issue.ID, gate.Name, core.StepGatePassed, check.Reason, check.ID); err != nil {
					return nil, err
				}
				goto nextGate

			case core.GateStatusPending:
				if err := c.recordStep(issue.ID, gate.Name, core.StepGateCheck, "awaiting resolution", check.ID); err != nil {
					return nil, err
				}
				return &GateChainResult{PendingGate: gate.Name}, nil

			case core.GateStatusFailed:
				if err := c.recordStep(issue.ID, gate.Name, core.StepGateCheck, fmt.Sprintf("attempt %d failed: %s", attempt, check.Reason), check.ID); err != nil {
					return nil, err
				}
				if attempt >= maxAttempts {
					return c.applyFallback(issue.ID, gate, check)
				}

			default:
				return nil, fmt.Errorf("gate %q: unexpected status %q", gate.Name, check.Status)
			}
		}
	nextGate:
	}

	return &GateChainResult{AllPassed: true}, nil
}

func (c *GateChain) applyFallback(issueID string, gate core.Gate, check *core.GateCheck) (*GateChainResult, error) {
	fallback := gate.Fallback
	if fallback == "" {
		fallback = core.GateFallbackEscalate
	}

	switch fallback {
	case core.GateFallbackForcePass:
		if err := c.recordStep(issueID, gate.Name, core.StepGatePassed, "force_pass after max attempts", check.ID); err != nil {
			return nil, err
		}
		return &GateChainResult{AllPassed: true, ForcePassed: true}, nil

	case core.GateFallbackAbort:
		if err := c.recordStep(issueID, gate.Name, core.StepGateFailed, "aborted after max attempts", check.ID); err != nil {
			return nil, err
		}
		return &GateChainResult{FailedCheck: check}, nil

	case core.GateFallbackEscalate:
		if err := c.recordStep(issueID, gate.Name, core.StepGateFailed, "escalated after max attempts", check.ID); err != nil {
			return nil, err
		}
		return &GateChainResult{FailedCheck: check}, nil

	default:
		if err := c.recordStep(issueID, gate.Name, core.StepGateFailed, "unknown fallback: "+string(fallback), check.ID); err != nil {
			return nil, err
		}
		return &GateChainResult{FailedCheck: check}, nil
	}
}

func (c *GateChain) recordStep(issueID, gateName string, action core.TaskStepAction, note, refID string) error {
	step := &core.TaskStep{
		ID:        core.NewTaskStepID(),
		IssueID:   issueID,
		Action:    action,
		Note:      fmt.Sprintf("[gate:%s] %s", gateName, note),
		RefID:     refID,
		RefType:   "gate_check",
		CreatedAt: time.Now(),
	}
	if _, err := c.Store.SaveTaskStep(step); err != nil {
		return fmt.Errorf("save gate task step %q: %w", gateName, err)
	}
	return nil
}

func (c *GateChain) saveDecision(issue *core.Issue, gate core.Gate, check *core.GateCheck) error {
	saver, ok := c.Store.(gateDecisionStore)
	if !ok || issue == nil || check == nil {
		return nil
	}
	decision, err := buildGateDecision(issue, gate, check)
	if err != nil {
		return err
	}
	if err := saver.SaveDecision(decision); err != nil {
		return fmt.Errorf("save gate decision %q attempt %d: %w", gate.Name, check.Attempt, err)
	}
	check.DecisionID = decision.ID
	return nil
}

func buildGateDecision(issue *core.Issue, gate core.Gate, check *core.GateCheck) (*core.Decision, error) {
	outputData, err := json.Marshal(map[string]any{
		"gate_name": gate.Name,
		"gate_type": gate.Type,
		"attempt":   check.Attempt,
		"status":    check.Status,
		"reason":    check.Reason,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal gate decision output: %w", err)
	}
	action := "pending"
	switch check.Status {
	case core.GateStatusPassed:
		action = "pass"
	case core.GateStatusFailed:
		action = "fail"
	}
	prompt := gate.Rules
	return &core.Decision{
		ID:            core.NewDecisionID(),
		IssueID:       issue.ID,
		RunID:         issue.RunID,
		AgentID:       check.CheckedBy,
		Type:          core.DecisionTypeGateCheck,
		PromptHash:    core.PromptHash(prompt),
		PromptPreview: core.TruncateString(prompt, 240),
		Template:      issue.Template,
		Action:        action,
		Reasoning:     check.Reason,
		OutputData:    string(outputData),
		CreatedAt:     check.CreatedAt,
	}, nil
}
