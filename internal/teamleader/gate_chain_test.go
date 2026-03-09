package teamleader

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

// --- mock GateStore ---

type mockGateStore struct {
	mu         sync.Mutex
	gateChecks []core.GateCheck
	taskSteps  []core.TaskStep
}

func (s *mockGateStore) SaveGateCheck(gc *core.GateCheck) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gateChecks = append(s.gateChecks, *gc)
	return nil
}

func (s *mockGateStore) GetGateChecks(issueID string) ([]core.GateCheck, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []core.GateCheck
	for _, gc := range s.gateChecks {
		if gc.IssueID == issueID {
			out = append(out, gc)
		}
	}
	return out, nil
}

func (s *mockGateStore) GetLatestGateCheck(issueID, gateName string) (*core.GateCheck, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest *core.GateCheck
	for i := range s.gateChecks {
		gc := &s.gateChecks[i]
		if gc.IssueID == issueID && gc.GateName == gateName {
			latest = gc
		}
	}
	return latest, nil
}

func (s *mockGateStore) SaveTaskStep(step *core.TaskStep) (core.IssueStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskSteps = append(s.taskSteps, *step)
	return "", nil
}

// --- mock GateRunner ---

type mockGateRunner struct {
	checks []*core.GateCheck // one per attempt, cycled if fewer than attempts
	err    error
	calls  int
}

func (r *mockGateRunner) Check(_ context.Context, _ *core.Issue, _ core.Gate, _ int) (*core.GateCheck, error) {
	if r.err != nil {
		return nil, r.err
	}
	idx := r.calls
	if idx >= len(r.checks) {
		idx = len(r.checks) - 1
	}
	r.calls++
	return r.checks[idx], nil
}

func newPassCheck(issueID, gateName string) *core.GateCheck {
	return &core.GateCheck{
		ID:        core.NewGateCheckID(),
		IssueID:   issueID,
		GateName:  gateName,
		GateType:  core.GateTypeAuto,
		Attempt:   1,
		Status:    core.GateStatusPassed,
		Reason:    "all good",
		CheckedBy: "auto",
	}
}

func newFailCheck(issueID, gateName string) *core.GateCheck {
	return &core.GateCheck{
		ID:        core.NewGateCheckID(),
		IssueID:   issueID,
		GateName:  gateName,
		GateType:  core.GateTypeAuto,
		Attempt:   1,
		Status:    core.GateStatusFailed,
		Reason:    "lint errors",
		CheckedBy: "auto",
	}
}

func newPendingCheck(issueID, gateName string) *core.GateCheck {
	return &core.GateCheck{
		ID:        core.NewGateCheckID(),
		IssueID:   issueID,
		GateName:  gateName,
		GateType:  core.GateTypeOwnerReview,
		Attempt:   1,
		Status:    core.GateStatusPending,
		Reason:    "awaiting owner review",
		CheckedBy: "human",
	}
}

func TestGateChain_AllPass(t *testing.T) {
	t.Parallel()

	store := &mockGateStore{}
	chain := &GateChain{
		Store: store,
		Runners: map[core.GateType]core.GateRunner{
			core.GateTypeAuto: &mockGateRunner{
				checks: []*core.GateCheck{
					newPassCheck("issue-1", "lint"),
					newPassCheck("issue-1", "tests"),
				},
			},
		},
	}

	issue := &core.Issue{ID: "issue-1", Title: "test", Template: "standard"}
	gates := []core.Gate{
		{Name: "lint", Type: core.GateTypeAuto, MaxAttempts: 1},
		{Name: "tests", Type: core.GateTypeAuto, MaxAttempts: 1},
	}

	result, err := chain.Run(context.Background(), issue, gates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AllPassed {
		t.Error("expected AllPassed=true")
	}
	if result.PendingGate != "" {
		t.Errorf("expected no PendingGate, got %q", result.PendingGate)
	}
	if result.FailedCheck != nil {
		t.Errorf("expected no FailedCheck, got %+v", result.FailedCheck)
	}
	if result.ForcePassed {
		t.Error("expected ForcePassed=false")
	}

	// Verify gate checks and task steps were recorded.
	if len(store.gateChecks) != 2 {
		t.Errorf("expected 2 gate checks saved, got %d", len(store.gateChecks))
	}
	if len(store.taskSteps) < 2 {
		t.Errorf("expected at least 2 task steps, got %d", len(store.taskSteps))
	}
}

func TestGateChain_PendingOwner(t *testing.T) {
	t.Parallel()

	store := &mockGateStore{}
	chain := &GateChain{
		Store: store,
		Runners: map[core.GateType]core.GateRunner{
			core.GateTypeAuto: &mockGateRunner{
				checks: []*core.GateCheck{newPassCheck("issue-2", "lint")},
			},
			core.GateTypeOwnerReview: &mockGateRunner{
				checks: []*core.GateCheck{newPendingCheck("issue-2", "owner")},
			},
		},
	}

	issue := &core.Issue{ID: "issue-2", Title: "test", Template: "standard"}
	gates := []core.Gate{
		{Name: "lint", Type: core.GateTypeAuto, MaxAttempts: 1},
		{Name: "owner", Type: core.GateTypeOwnerReview},
	}

	result, err := chain.Run(context.Background(), issue, gates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AllPassed {
		t.Error("expected AllPassed=false when pending")
	}
	if result.PendingGate != "owner" {
		t.Errorf("expected PendingGate='owner', got %q", result.PendingGate)
	}
	if result.FailedCheck != nil {
		t.Errorf("expected no FailedCheck, got %+v", result.FailedCheck)
	}
}

func TestGateChain_FailEscalate(t *testing.T) {
	t.Parallel()

	store := &mockGateStore{}
	chain := &GateChain{
		Store: store,
		Runners: map[core.GateType]core.GateRunner{
			core.GateTypeAuto: &mockGateRunner{
				checks: []*core.GateCheck{newFailCheck("issue-3", "lint")},
			},
		},
	}

	issue := &core.Issue{ID: "issue-3", Title: "test", Template: "standard"}
	gates := []core.Gate{
		{Name: "lint", Type: core.GateTypeAuto, MaxAttempts: 1, Fallback: core.GateFallbackEscalate},
	}

	result, err := chain.Run(context.Background(), issue, gates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AllPassed {
		t.Error("expected AllPassed=false on escalate")
	}
	if result.FailedCheck == nil {
		t.Fatal("expected FailedCheck to be set")
	}
	if result.FailedCheck.Status != core.GateStatusFailed {
		t.Errorf("expected failed status, got %q", result.FailedCheck.Status)
	}
	if result.ForcePassed {
		t.Error("expected ForcePassed=false on escalate")
	}
}

func TestGateChain_FailForcePass(t *testing.T) {
	t.Parallel()

	store := &mockGateStore{}
	chain := &GateChain{
		Store: store,
		Runners: map[core.GateType]core.GateRunner{
			core.GateTypeAuto: &mockGateRunner{
				checks: []*core.GateCheck{newFailCheck("issue-4", "lint")},
			},
		},
	}

	issue := &core.Issue{ID: "issue-4", Title: "test", Template: "standard"}
	gates := []core.Gate{
		{Name: "lint", Type: core.GateTypeAuto, MaxAttempts: 1, Fallback: core.GateFallbackForcePass},
	}

	result, err := chain.Run(context.Background(), issue, gates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AllPassed {
		t.Error("expected AllPassed=true on force_pass")
	}
	if !result.ForcePassed {
		t.Error("expected ForcePassed=true")
	}
	if result.FailedCheck != nil {
		t.Errorf("expected no FailedCheck on force_pass, got %+v", result.FailedCheck)
	}
}

func TestGateChain_EmptyGates(t *testing.T) {
	t.Parallel()

	store := &mockGateStore{}
	chain := &GateChain{
		Store:   store,
		Runners: map[core.GateType]core.GateRunner{},
	}

	issue := &core.Issue{ID: "issue-5", Title: "test", Template: "standard"}
	result, err := chain.Run(context.Background(), issue, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AllPassed {
		t.Error("expected AllPassed=true for empty gates")
	}
}

func TestGateChain_RunnerError(t *testing.T) {
	t.Parallel()

	store := &mockGateStore{}
	chain := &GateChain{
		Store: store,
		Runners: map[core.GateType]core.GateRunner{
			core.GateTypeAuto: &mockGateRunner{
				err: errors.New("runner crashed"),
			},
		},
	}

	issue := &core.Issue{ID: "issue-6", Title: "test", Template: "standard"}
	gates := []core.Gate{
		{Name: "lint", Type: core.GateTypeAuto, MaxAttempts: 1},
	}

	_, err := chain.Run(context.Background(), issue, gates)
	if err == nil {
		t.Fatal("expected error from runner")
	}
}

func TestGateChain_FailAbort(t *testing.T) {
	t.Parallel()

	store := &mockGateStore{}
	chain := &GateChain{
		Store: store,
		Runners: map[core.GateType]core.GateRunner{
			core.GateTypeAuto: &mockGateRunner{
				checks: []*core.GateCheck{newFailCheck("issue-7", "lint")},
			},
		},
	}

	issue := &core.Issue{ID: "issue-7", Title: "test", Template: "standard"}
	gates := []core.Gate{
		{Name: "lint", Type: core.GateTypeAuto, MaxAttempts: 1, Fallback: core.GateFallbackAbort},
	}

	result, err := chain.Run(context.Background(), issue, gates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AllPassed {
		t.Error("expected AllPassed=false on abort")
	}
	if result.FailedCheck == nil {
		t.Fatal("expected FailedCheck to be set on abort")
	}
	if result.ForcePassed {
		t.Error("expected ForcePassed=false on abort")
	}
}

func TestGateChain_NoRunnerFails(t *testing.T) {
	t.Parallel()

	store := &mockGateStore{}
	chain := &GateChain{
		Store:   store,
		Runners: map[core.GateType]core.GateRunner{}, // no runners registered
	}

	issue := &core.Issue{ID: "issue-8", Title: "test", Template: "standard"}
	gates := []core.Gate{
		{Name: "peer_review", Type: core.GateTypePeerReview},
	}

	_, err := chain.Run(context.Background(), issue, gates)
	if err == nil {
		t.Fatal("expected error when gate runner is missing")
	}
}

func TestGateChain_MaxAttemptsZeroFallsBackImmediately(t *testing.T) {
	t.Parallel()

	store := &mockGateStore{}
	chain := &GateChain{
		Store: store,
		Runners: map[core.GateType]core.GateRunner{
			core.GateTypeAuto: &mockGateRunner{
				checks: []*core.GateCheck{newFailCheck("issue-9", "lint")},
			},
		},
	}

	issue := &core.Issue{ID: "issue-9", Title: "test", Template: "standard"}
	gates := []core.Gate{
		{Name: "lint", Type: core.GateTypeAuto, MaxAttempts: 0, Fallback: core.GateFallbackAbort},
	}

	result, err := chain.Run(context.Background(), issue, gates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FailedCheck == nil {
		t.Fatal("expected failed check when max attempts defaults to one")
	}
	if len(store.gateChecks) != 1 {
		t.Fatalf("expected exactly one gate check, got %d", len(store.gateChecks))
	}
}
