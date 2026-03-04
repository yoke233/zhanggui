package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestReactions_FirstMatchWins(t *testing.T) {
	rules := []ReactionRule{
		{
			Name: "first",
			Match: func(ReactionContext) bool {
				return true
			},
			Action: ReactionRetry,
		},
		{
			Name: "second",
			Match: func(ReactionContext) bool {
				return true
			},
			Action: ReactionAbortRun,
		},
	}

	action, matched := EvaluateReactionRules(ReactionContext{}, rules)
	if !matched {
		t.Fatal("expected first rule to match")
	}
	if action != ReactionRetry {
		t.Fatalf("expected first-match action retry, got %s", action)
	}
}

func TestReactions_CompileOnFailureSugar(t *testing.T) {
	cases := []struct {
		name     string
		failure  core.OnFailure
		expected ReactionAction
	}{
		{name: "retry", failure: core.OnFailureRetry, expected: ReactionRetry},
		{name: "human", failure: core.OnFailureHuman, expected: ReactionEscalateHuman},
		{name: "skip", failure: core.OnFailureSkip, expected: ReactionSkipStage},
		{name: "abort", failure: core.OnFailureAbort, expected: ReactionAbortRun},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stage := core.StageConfig{Name: core.StageImplement, OnFailure: tc.failure}
			rules := CompileOnFailureReactions(stage)
			if len(rules) == 0 {
				t.Fatal("expected compiled reaction rules")
			}

			action, matched := EvaluateReactionRules(ReactionContext{Stage: stage}, rules)
			if !matched {
				t.Fatal("expected compiled reaction to match")
			}
			if action != tc.expected {
				t.Fatalf("expected action %s, got %s", tc.expected, action)
			}
		})
	}
}

func TestReactions_RetryConsumesGlobalBudget(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{
		errors.New("boom-1"),
		nil,
	}}
	agent := &fakeAgent{name: "codex"}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{
			Name:       core.StageImplement,
			Agent:      "codex",
			OnFailure:  core.OnFailureRetry,
			MaxRetries: 3,
			Timeout:    0,
		},
	})
	p.WorktreePath = workDir
	p.MaxTotalRetries = 1
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	err := execEngine.Run(context.Background(), p.ID)
	if err == nil {
		t.Fatal("expected retry budget exhaustion error")
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusCompleted {
		t.Fatalf("expected completed status after retry budget exhaustion, got %s", got.Status)
	}
	if got.Conclusion != core.ConclusionFailure {
		t.Fatalf("expected failure conclusion after retry budget exhaustion, got %s", got.Conclusion)
	}
	if runtime.calls != 1 {
		t.Fatalf("expected only one attempt due global budget, got calls=%d", runtime.calls)
	}
}
