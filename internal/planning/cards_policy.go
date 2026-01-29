package planning

import (
	"fmt"

	"github.com/yoke233/zhanggui/internal/policyexpr"
)

type CardsPolicy struct {
	RequiredWhen []string `yaml:"required_when,omitempty" json:"required_when,omitempty"`
	OptionalWhen []string `yaml:"optional_when,omitempty" json:"optional_when,omitempty"`
}

type CardsDecisionInput struct {
	ParallelTeams         int
	MustAnswerCount       int
	HasTradeoffs          bool
	NeedsComparisonMatrix bool
	ConflictDetected      bool
	EvidenceRequired      bool
}

type CardsDecision struct {
	Required        bool
	RequiredReasons []string
	OptionalReasons []string
}

func DecideCards(policy CardsPolicy, in CardsDecisionInput) (CardsDecision, error) {
	vars := map[string]any{
		"parallel_teams":          in.ParallelTeams,
		"must_answer_count":       in.MustAnswerCount,
		"has_tradeoffs":           in.HasTradeoffs,
		"needs_comparison_matrix": in.NeedsComparisonMatrix,
		"conflict_detected":       in.ConflictDetected,
		"evidence_required":       in.EvidenceRequired,
	}

	var requiredReasons []string
	for _, rule := range policy.RequiredWhen {
		expr, err := policyexpr.ParseBoolExpr(rule)
		if err != nil {
			return CardsDecision{}, fmt.Errorf("cards_policy.required_when 规则非法: %w (rule=%q)", err, rule)
		}
		ok, err := expr.Eval(vars)
		if err != nil {
			return CardsDecision{}, fmt.Errorf("cards_policy.required_when 规则执行失败: %w (rule=%q)", err, rule)
		}
		if ok {
			requiredReasons = append(requiredReasons, rule)
		}
	}

	var optionalReasons []string
	for _, rule := range policy.OptionalWhen {
		expr, err := policyexpr.ParseBoolExpr(rule)
		if err != nil {
			return CardsDecision{}, fmt.Errorf("cards_policy.optional_when 规则非法: %w (rule=%q)", err, rule)
		}
		ok, err := expr.Eval(vars)
		if err != nil {
			return CardsDecision{}, fmt.Errorf("cards_policy.optional_when 规则执行失败: %w (rule=%q)", err, rule)
		}
		if ok {
			optionalReasons = append(optionalReasons, rule)
		}
	}

	return CardsDecision{
		Required:        len(requiredReasons) > 0,
		RequiredReasons: requiredReasons,
		OptionalReasons: optionalReasons,
	}, nil
}
