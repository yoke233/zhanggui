package agentruntime

import (
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestCheckTokenBudget_NoBudget(t *testing.T) {
	pool := &ACPSessionPool{}
	sess := &pooledACPSession{inputTokens: 99999, outputTokens: 99999}
	profile := &core.AgentProfile{Session: core.ProfileSession{MaxContextTokens: 0}}

	if pool.CheckTokenBudget(sess, profile) != TokenBudgetOK {
		t.Error("expected OK when no budget configured")
	}
}

func TestCheckTokenBudget_UnderThreshold(t *testing.T) {
	pool := &ACPSessionPool{}
	sess := &pooledACPSession{inputTokens: 1000, outputTokens: 500}
	profile := &core.AgentProfile{Session: core.ProfileSession{MaxContextTokens: 10000}}

	if pool.CheckTokenBudget(sess, profile) != TokenBudgetOK {
		t.Error("expected OK when well under budget")
	}
}

func TestCheckTokenBudget_Warning(t *testing.T) {
	pool := &ACPSessionPool{}
	// 8500 total out of 10000 = 85%, above default 80% warn ratio
	sess := &pooledACPSession{inputTokens: 5000, outputTokens: 3500}
	profile := &core.AgentProfile{Session: core.ProfileSession{MaxContextTokens: 10000}}

	if pool.CheckTokenBudget(sess, profile) != TokenBudgetWarning {
		t.Error("expected Warning when above 80% threshold")
	}
}

func TestCheckTokenBudget_CustomWarnRatio(t *testing.T) {
	pool := &ACPSessionPool{}
	sess := &pooledACPSession{inputTokens: 5000, outputTokens: 3500}
	profile := &core.AgentProfile{Session: core.ProfileSession{
		MaxContextTokens: 10000,
		ContextWarnRatio: 0.9, // 90% threshold
	}}

	// 8500/10000 = 85% — under 90% threshold
	if pool.CheckTokenBudget(sess, profile) != TokenBudgetOK {
		t.Error("expected OK with custom 90% threshold")
	}
}

func TestCheckTokenBudget_Exceeded(t *testing.T) {
	pool := &ACPSessionPool{}
	sess := &pooledACPSession{inputTokens: 6000, outputTokens: 5000}
	profile := &core.AgentProfile{Session: core.ProfileSession{MaxContextTokens: 10000}}

	if pool.CheckTokenBudget(sess, profile) != TokenBudgetExceeded {
		t.Error("expected Exceeded when at or over limit")
	}
}

func TestCheckTokenBudget_ExactlyAtLimit(t *testing.T) {
	pool := &ACPSessionPool{}
	sess := &pooledACPSession{inputTokens: 5000, outputTokens: 5000}
	profile := &core.AgentProfile{Session: core.ProfileSession{MaxContextTokens: 10000}}

	if pool.CheckTokenBudget(sess, profile) != TokenBudgetExceeded {
		t.Error("expected Exceeded when exactly at limit")
	}
}

func TestNoteTokens(t *testing.T) {
	pool := &ACPSessionPool{}
	sess := &pooledACPSession{}

	pool.NoteTokens(sess, 100, 50)
	pool.NoteTokens(sess, 200, 80)

	input, output := pool.SessionTokenUsage(sess)
	if input != 300 {
		t.Errorf("expected 300 input tokens, got %d", input)
	}
	if output != 130 {
		t.Errorf("expected 130 output tokens, got %d", output)
	}
}

func TestCheckTokenBudget_NilInputs(t *testing.T) {
	pool := &ACPSessionPool{}

	if pool.CheckTokenBudget(nil, &core.AgentProfile{}) != TokenBudgetOK {
		t.Error("expected OK for nil session")
	}
	if pool.CheckTokenBudget(&pooledACPSession{}, nil) != TokenBudgetOK {
		t.Error("expected OK for nil profile")
	}
}
