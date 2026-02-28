package engine

import (
	"fmt"

	"github.com/user/ai-workflow/internal/core"
)

type ReactionAction string

const (
	ReactionRetry         ReactionAction = "retry"
	ReactionEscalateHuman ReactionAction = "escalate_human"
	ReactionSkipStage     ReactionAction = "skip_stage"
	ReactionAbortPipeline ReactionAction = "abort_pipeline"
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
	action := ReactionAbortPipeline
	switch stage.OnFailure {
	case core.OnFailureRetry:
		action = ReactionRetry
	case core.OnFailureHuman:
		action = ReactionEscalateHuman
	case core.OnFailureSkip:
		action = ReactionSkipStage
	case core.OnFailureAbort:
		action = ReactionAbortPipeline
	}

	return []ReactionRule{
		{
			Name:   fmt.Sprintf("on_failure_%s", stage.OnFailure),
			Action: action,
		},
	}
}
