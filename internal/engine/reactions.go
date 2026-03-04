package engine

import (
	"fmt"

	"github.com/yoke233/ai-workflow/internal/core"
)

type ReactionAction string

const (
	ReactionRetry         ReactionAction = "retry"
	ReactionEscalateHuman ReactionAction = "escalate_human"
	ReactionSkipStage     ReactionAction = "skip_stage"
	ReactionAbortRun      ReactionAction = "abort_Run"
)

type ReactionContext struct {
	Stage    core.StageConfig
	Attempt  int
	MaxRetry int
	Err      error
}

type ReactionRule struct {
	Name   string
	Match  func(ReactionContext) bool
	Action ReactionAction
}

func EvaluateReactionRules(ctx ReactionContext, rules []ReactionRule) (ReactionAction, bool) {
	for _, rule := range rules {
		match := true
		if rule.Match != nil {
			match = rule.Match(ctx)
		}
		if match {
			return rule.Action, true
		}
	}
	return "", false
}

func CompileOnFailureReactions(stage core.StageConfig) []ReactionRule {
	action := ReactionAbortRun
	switch stage.OnFailure {
	case core.OnFailureRetry:
		action = ReactionRetry
	case core.OnFailureHuman:
		action = ReactionEscalateHuman
	case core.OnFailureSkip:
		action = ReactionSkipStage
	case core.OnFailureAbort:
		action = ReactionAbortRun
	}

	ruleName := fmt.Sprintf("on_failure_%s", stage.OnFailure)
	if stage.Name != "" {
		ruleName = fmt.Sprintf("on_failure_%s_%s", stage.Name, stage.OnFailure)
	}

	return []ReactionRule{
		{
			Name:   ruleName,
			Action: action,
		},
	}
}
