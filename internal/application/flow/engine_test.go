package flow

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
)

func setup(t *testing.T) (core.Store, core.EventBus) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, NewMemBus()
}

// TestLinearFlow: A → B → C, all succeed (sequential by Position).
func TestLinearFlow(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var callOrder []string
	var counter int32
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		atomic.AddInt32(&counter, 1)
		callOrder = append(callOrder, step.Name)
		exec.Output = map[string]any{"ok": true}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	// Create issue + steps.
	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "linear", Status: core.IssueOpen})

	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "A", Type: core.StepExec, Status: core.StepPending, Position: 0})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "B", Type: core.StepExec, Status: core.StepPending, Position: 1})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "C", Type: core.StepExec, Status: core.StepPending, Position: 2})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	if counter != 3 {
		t.Fatalf("expected 3 executions, got %d", counter)
	}
	if callOrder[0] != "A" || callOrder[1] != "B" || callOrder[2] != "C" {
		t.Fatalf("unexpected order: %v", callOrder)
	}

	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}
}

// TestParallelFanOut: steps at same Position run concurrently.
func TestParallelFanOut(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var counter int32
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		atomic.AddInt32(&counter, 1)
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(4))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "fanout", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "A", Type: core.StepExec, Status: core.StepPending, Position: 0})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "B", Type: core.StepExec, Status: core.StepPending, Position: 1})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "C", Type: core.StepExec, Status: core.StepPending, Position: 1})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}
	if counter != 3 {
		t.Fatalf("expected 3 executions, got %d", counter)
	}
}

// TestStepFailure: A fails, issue fails.
func TestStepFailure(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		return fmt.Errorf("boom")
	}

	eng := New(store, bus, executor)

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "fail", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "A", Type: core.StepExec, Status: core.StepPending, Position: 0})

	err := eng.Run(ctx, issueID)
	if err == nil {
		t.Fatal("expected error")
	}

	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueFailed {
		t.Fatalf("expected failed, got %s", issue.Status)
	}
}

// TestRetry: step fails once, retries, succeeds.
func TestRetry(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var attempts int32
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			return fmt.Errorf("transient")
		}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "retry", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "A", Type: core.StepExec, Status: core.StepPending, Position: 0, MaxRetries: 1})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

// TestCancelIssue: cancel a running issue.
func TestCancelIssue(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	eng := New(store, bus, nil)

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "cancel-test", Status: core.IssueOpen})
	_ = store.UpdateIssueStatus(ctx, issueID, core.IssueRunning) // simulate running

	if err := eng.Cancel(ctx, issueID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueCancelled {
		t.Fatalf("expected cancelled, got %s", issue.Status)
	}
}

// TestRetryPersistence: verify retry_count is persisted, preventing infinite retries.
func TestRetryPersistence(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		return fmt.Errorf("always fail")
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "retry-persist", Status: core.IssueOpen})
	sID, _ := store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "A", Type: core.StepExec, Status: core.StepPending, Position: 0, MaxRetries: 2})

	// Should fail after 3 attempts (1 original + 2 retries).
	err := eng.Run(ctx, issueID)
	if err == nil {
		t.Fatal("expected error")
	}

	// Verify retry_count was persisted.
	step, _ := store.GetStep(ctx, sID)
	if step.RetryCount != 2 {
		t.Fatalf("expected retry_count=2, got %d", step.RetryCount)
	}
}

// TestGateAutoPass: exec → gate(pass) → issue done.
func TestGateAutoPass(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		if step.Type == core.StepGate {
			_, err := store.CreateArtifact(ctx, &core.Artifact{
				ExecutionID:    exec.ID,
				StepID:         step.ID,
				IssueID:        step.IssueID,
				ResultMarkdown: "LGTM, all tests pass.",
				Metadata:       map[string]any{"verdict": "pass"},
			})
			return err
		}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "gate-pass", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "impl", Type: core.StepExec, Status: core.StepPending, Position: 0})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "review", Type: core.StepGate, Status: core.StepPending, Position: 1})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}
	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}
}

// TestGateAutoReject: exec → gate(reject) → exec retries → gate(pass) → issue done.
func TestGateAutoReject(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var gateCount int32
	var execCount int32
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		if step.Type == core.StepGate {
			n := atomic.AddInt32(&gateCount, 1)
			verdict := "reject"
			if n > 1 {
				verdict = "pass"
			}
			_, err := store.CreateArtifact(ctx, &core.Artifact{
				ExecutionID:    exec.ID,
				StepID:         step.ID,
				IssueID:        step.IssueID,
				ResultMarkdown: "Review result",
				Metadata:       map[string]any{"verdict": verdict, "reason": "needs improvement"},
			})
			return err
		}
		atomic.AddInt32(&execCount, 1)
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "gate-reject", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "impl", Type: core.StepExec, Status: core.StepPending, Position: 0, MaxRetries: 1})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "review", Type: core.StepGate, Status: core.StepPending, Position: 1})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}
	if gateCount != 2 {
		t.Fatalf("expected 2 gate evaluations, got %d", gateCount)
	}
	if execCount != 2 {
		t.Fatalf("expected 2 exec runs, got %d", execCount)
	}
}

// TestStepTimeout: step times out on first attempt, retries, succeeds.
func TestStepTimeout(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var attempts int32
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			select {
			case <-time.After(500 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "timeout", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{
		IssueID:    issueID,
		Name:       "slow",
		Type:       core.StepExec,
		Status:     core.StepPending,
		Position:   0,
		Timeout:    50 * time.Millisecond,
		MaxRetries: 1,
	})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

// TestErrorKindPermanent: permanent error skips retry despite MaxRetries > 0.
func TestErrorKindPermanent(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		exec.ErrorKind = core.ErrKindPermanent
		return fmt.Errorf("fatal: invalid configuration")
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "permanent", Status: core.IssueOpen})
	sID, _ := store.CreateStep(ctx, &core.Step{
		IssueID:    issueID,
		Name:       "A",
		Type:       core.StepExec,
		Status:     core.StepPending,
		Position:   0,
		MaxRetries: 5,
	})

	err := eng.Run(ctx, issueID)
	if err == nil {
		t.Fatal("expected error")
	}

	step, _ := store.GetStep(ctx, sID)
	if step.RetryCount != 0 {
		t.Fatalf("expected retry_count=0 (permanent skips retry), got %d", step.RetryCount)
	}
	if step.Status != core.StepFailed {
		t.Fatalf("expected failed, got %s", step.Status)
	}
}

// TestProfileRegistry: resolve by role + capabilities.
func TestProfileRegistry(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "claude-worker", Role: core.RoleWorker, Capabilities: []string{"backend", "frontend"}},
		{ID: "claude-gate", Role: core.RoleGate, Capabilities: []string{"review"}},
		{ID: "codex-worker", Role: core.RoleWorker, Capabilities: []string{"backend", "qa"}},
	}
	reg := NewProfileRegistry(profiles)
	ctx := context.Background()

	// Match role + capability.
	id, err := reg.Resolve(ctx, &core.Step{AgentRole: "worker", RequiredCapabilities: []string{"qa"}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if id != "codex-worker" {
		t.Fatalf("expected codex-worker, got %s", id)
	}

	// Match role only (no capability filter).
	id, err = reg.Resolve(ctx, &core.Step{AgentRole: "gate"})
	if err != nil {
		t.Fatalf("resolve gate: %v", err)
	}
	if id != "claude-gate" {
		t.Fatalf("expected claude-gate, got %s", id)
	}

	// No role filter — first match.
	id, err = reg.Resolve(ctx, &core.Step{})
	if err != nil {
		t.Fatalf("resolve any: %v", err)
	}
	if id != "claude-worker" {
		t.Fatalf("expected claude-worker, got %s", id)
	}

	// No match.
	_, err = reg.Resolve(ctx, &core.Step{AgentRole: "worker", RequiredCapabilities: []string{"k8s"}})
	if err != core.ErrNoMatchingAgent {
		t.Fatalf("expected ErrNoMatchingAgent, got %v", err)
	}
}

// TestBriefingBuilder: assembles briefing from upstream artifacts.
func TestBriefingBuilder(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	// Create an issue with A → B (by Position).
	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "briefing-test", Status: core.IssueOpen})
	aID, _ := store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "A", Type: core.StepExec, Status: core.StepDone, Position: 0})
	bID, _ := store.CreateStep(ctx, &core.Step{
		IssueID:            issueID,
		Name:               "B",
		Type:               core.StepExec,
		Status:             core.StepPending,
		Position:           1,
		AcceptanceCriteria: []string{"must pass lint", "must have tests"},
		Config:             map[string]any{"objective": "Implement login endpoint"},
	})

	// A has an artifact.
	eID, _ := store.CreateExecution(ctx, &core.Execution{StepID: aID, IssueID: issueID, Status: core.ExecSucceeded, Attempt: 1})
	store.CreateArtifact(ctx, &core.Artifact{
		ExecutionID:    eID,
		StepID:         aID,
		IssueID:        issueID,
		ResultMarkdown: "## Design\nAPI design for login.",
	})

	builder := NewBriefingBuilder(store)
	stepB, _ := store.GetStep(ctx, bID)
	briefing, err := builder.Build(ctx, stepB)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if briefing.Objective != "Implement login endpoint" {
		t.Fatalf("expected objective from config, got %q", briefing.Objective)
	}
	if len(briefing.Constraints) != 2 {
		t.Fatalf("expected 2 constraints, got %d", len(briefing.Constraints))
	}
	if len(briefing.ContextRefs) != 1 {
		t.Fatalf("expected 1 context ref, got %d", len(briefing.ContextRefs))
	}
	if briefing.ContextRefs[0].Type != core.CtxUpstreamArtifact {
		t.Fatalf("expected upstream_artifact ref, got %s", briefing.ContextRefs[0].Type)
	}
	if briefing.ContextRefs[0].Inline != "## Design\nAPI design for login." {
		t.Fatalf("expected inline content, got %q", briefing.ContextRefs[0].Inline)
	}

	_ = bus // satisfy usage
}

// TestCollectorWiring: collector extracts metadata into artifact after success.
func TestCollectorWiring(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	// Collector that extracts a "summary" field.
	collector := CollectorFunc(func(_ context.Context, stepType core.StepType, markdown string) (map[string]any, error) {
		return map[string]any{"summary": "extracted from: " + stepType}, nil
	})

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		// Simulate agent creating an artifact.
		_, err := store.CreateArtifact(ctx, &core.Artifact{
			ExecutionID:    exec.ID,
			StepID:         step.ID,
			IssueID:        step.IssueID,
			ResultMarkdown: "## Implementation\nDid the thing.",
		})
		return err
	}

	eng := New(store, bus, executor, WithConcurrency(1), WithCollector(collector))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "collector-test", Status: core.IssueOpen})
	sID, _ := store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "A", Type: core.StepExec, Status: core.StepPending, Position: 0})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	art, err := store.GetLatestArtifactByStep(ctx, sID)
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	if art.Metadata["summary"] != "extracted from: exec" {
		t.Fatalf("expected extracted metadata, got %v", art.Metadata)
	}
}

// TestResolverIntegration: engine uses resolver to set agent_id on execution.
func TestResolverIntegration(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	profiles := []*core.AgentProfile{
		{ID: "my-worker", Role: core.RoleWorker, Capabilities: []string{"go"}},
	}

	var capturedAgentID string
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		capturedAgentID = exec.AgentID
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1), WithResolver(NewProfileRegistry(profiles)))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "resolver-test", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{
		IssueID:              issueID,
		Name:                 "build",
		Type:                 core.StepExec,
		Status:               core.StepPending,
		Position:             0,
		AgentRole:            "worker",
		RequiredCapabilities: []string{"go"},
	})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}
	if capturedAgentID != "my-worker" {
		t.Fatalf("expected agent_id=my-worker, got %q", capturedAgentID)
	}
}

// TestEventBus: subscribe and receive events.
func TestEventBus(t *testing.T) {
	bus := NewMemBus()
	ctx := context.Background()

	sub := bus.Subscribe(core.SubscribeOpts{
		Types:      []core.EventType{core.EventIssueStarted},
		BufferSize: 8,
	})
	defer sub.Cancel()

	bus.Publish(ctx, core.Event{Type: core.EventIssueStarted, IssueID: 1})
	bus.Publish(ctx, core.Event{Type: core.EventStepReady, IssueID: 1})   // should be filtered out
	bus.Publish(ctx, core.Event{Type: core.EventIssueStarted, IssueID: 2}) // should be received

	ev := <-sub.C
	if ev.IssueID != 1 {
		t.Fatalf("expected issue 1, got %d", ev.IssueID)
	}
	ev = <-sub.C
	if ev.IssueID != 2 {
		t.Fatalf("expected issue 2, got %d", ev.IssueID)
	}
}

// ---------------------------------------------------------------------------
// Composite Step Tests
// ---------------------------------------------------------------------------

// TestCompositeSimple: A(exec) → B(composite[B1,B2]) → C(exec), all succeed.
func TestCompositeSimple(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var callOrder []string
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		callOrder = append(callOrder, step.Name)
		return nil
	}

	// Expander returns two child steps: B1 and B2.
	expander := ExpanderFunc(func(_ context.Context, step *core.Step) ([]*core.Step, error) {
		b1 := &core.Step{Name: "B1", Type: core.StepExec}
		b2 := &core.Step{Name: "B2", Type: core.StepExec}
		return []*core.Step{b1, b2}, nil
	})

	eng := New(store, bus, executor, WithConcurrency(1), WithExpander(expander))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "composite-simple", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "A", Type: core.StepExec, Status: core.StepPending, Position: 0})
	bID, _ := store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "B", Type: core.StepComposite, Status: core.StepPending, Position: 1})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "C", Type: core.StepExec, Status: core.StepPending, Position: 2})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	// A runs first, then B expands (B1, B2 run in child issue), then C.
	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}

	// Verify A ran, B1/B2 ran inside child issue, then C ran.
	if len(callOrder) != 4 {
		t.Fatalf("expected 4 executor calls (A, B1, B2, C), got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != "A" {
		t.Fatalf("expected A first, got %s", callOrder[0])
	}
	if callOrder[3] != "C" {
		t.Fatalf("expected C last, got %s", callOrder[3])
	}

	// B should have child_issue_id in Config.
	stepB, _ := store.GetStep(ctx, bID)
	childID := childIssueID(stepB)
	if childID == nil {
		t.Fatal("expected B to have child_issue_id in Config")
	}
	if stepB.Status != core.StepDone {
		t.Fatalf("expected B done, got %s", stepB.Status)
	}

	// Child issue should also be done.
	childIssue, _ := store.GetIssue(ctx, *childID)
	if childIssue.Status != core.IssueDone {
		t.Fatalf("expected child issue done, got %s", childIssue.Status)
	}
}

// TestCompositeChainedChildren: composite with sequential children B1 → B2.
func TestCompositeChainedChildren(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var callOrder []string
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		callOrder = append(callOrder, step.Name)
		return nil
	}

	expander := ExpanderFunc(func(_ context.Context, step *core.Step) ([]*core.Step, error) {
		return []*core.Step{
			{Name: "B1", Type: core.StepExec},
			{Name: "B2", Type: core.StepExec},
		}, nil
	})

	eng := New(store, bus, executor, WithConcurrency(1), WithExpander(expander))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "composite-chain", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "B", Type: core.StepComposite, Status: core.StepPending, Position: 0})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}
	if len(callOrder) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(callOrder), callOrder)
	}
}

// TestCompositeSubFlowFail: composite child fails → composite fails → parent issue fails.
func TestCompositeSubFlowFail(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		if step.Name == "child-bad" {
			return fmt.Errorf("child failure")
		}
		return nil
	}

	expander := ExpanderFunc(func(_ context.Context, step *core.Step) ([]*core.Step, error) {
		return []*core.Step{
			{Name: "child-bad", Type: core.StepExec},
		}, nil
	})

	eng := New(store, bus, executor, WithConcurrency(1), WithExpander(expander))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "composite-fail", Status: core.IssueOpen})
	compID, _ := store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "comp", Type: core.StepComposite, Status: core.StepPending, Position: 0})

	err := eng.Run(ctx, issueID)
	if err == nil {
		t.Fatal("expected error from child issue failure")
	}

	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueFailed {
		t.Fatalf("expected failed, got %s", issue.Status)
	}

	compStep, _ := store.GetStep(ctx, compID)
	if compStep.Status != core.StepFailed {
		t.Fatalf("expected composite step failed, got %s", compStep.Status)
	}
}

// TestCompositeRetry: composite child issue fails once, composite retries with fresh child issue, succeeds.
func TestCompositeRetry(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var expandCount int32
	var execCount int32

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		n := atomic.AddInt32(&execCount, 1)
		// First child execution fails, second succeeds.
		if n == 1 {
			return fmt.Errorf("transient child failure")
		}
		return nil
	}

	expander := ExpanderFunc(func(_ context.Context, step *core.Step) ([]*core.Step, error) {
		atomic.AddInt32(&expandCount, 1)
		return []*core.Step{
			{Name: "child", Type: core.StepExec},
		}, nil
	})

	eng := New(store, bus, executor, WithConcurrency(1), WithExpander(expander))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "composite-retry", Status: core.IssueOpen})
	compID, _ := store.CreateStep(ctx, &core.Step{
		IssueID:    issueID,
		Name:       "comp",
		Type:       core.StepComposite,
		Status:     core.StepPending,
		Position:   0,
		MaxRetries: 1,
	})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}

	compStep, _ := store.GetStep(ctx, compID)
	if compStep.Status != core.StepDone {
		t.Fatalf("expected composite done, got %s", compStep.Status)
	}
	if compStep.RetryCount != 1 {
		t.Fatalf("expected retry_count=1, got %d", compStep.RetryCount)
	}

	// Expander should have been called twice (original + retry).
	if expandCount != 2 {
		t.Fatalf("expected 2 expansions, got %d", expandCount)
	}
}

// ---------------------------------------------------------------------------
// Issue Integration Tests — cross-cutting scenarios
// ---------------------------------------------------------------------------

// TestIssueE2E_ResolverBriefingCollector: full pipeline with all 3 injectable interfaces.
func TestIssueE2E_ResolverBriefingCollector(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	profiles := []*core.AgentProfile{
		{ID: "designer", Role: core.RoleWorker, Capabilities: []string{"design"}},
		{ID: "coder", Role: core.RoleWorker, Capabilities: []string{"go"}},
	}

	collector := CollectorFunc(func(_ context.Context, stepType core.StepType, md string) (map[string]any, error) {
		return map[string]any{"collected": true, "type": string(stepType)}, nil
	})

	var capturedBriefing string
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		if step.Name == "implement" {
			capturedBriefing = exec.BriefingSnapshot
		}
		// Every step produces an artifact.
		_, err := store.CreateArtifact(ctx, &core.Artifact{
			ExecutionID:    exec.ID,
			StepID:         step.ID,
			IssueID:        step.IssueID,
			ResultMarkdown: fmt.Sprintf("## %s output\nDone.", step.Name),
		})
		return err
	}

	eng := New(store, bus, executor,
		WithConcurrency(1),
		WithResolver(NewProfileRegistry(profiles)),
		WithBriefingBuilder(NewBriefingBuilder(store)),
		WithCollector(collector),
	)

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "e2e-pipeline", Status: core.IssueOpen})
	designID, _ := store.CreateStep(ctx, &core.Step{
		IssueID:              issueID,
		Name:                 "design",
		Type:                 core.StepExec,
		Status:               core.StepPending,
		Position:             0,
		AgentRole:            "worker",
		RequiredCapabilities: []string{"design"},
	})
	implID, _ := store.CreateStep(ctx, &core.Step{
		IssueID:              issueID,
		Name:                 "implement",
		Type:                 core.StepExec,
		Status:               core.StepPending,
		Position:             1,
		AgentRole:            "worker",
		RequiredCapabilities: []string{"go"},
		Config:               map[string]any{"objective": "Build login API"},
	})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}

	// Verify briefing was assembled with upstream artifact content.
	if !strings.Contains(capturedBriefing, "Build login API") {
		t.Fatalf("expected briefing snapshot to contain objective, got %q", capturedBriefing)
	}
	if !strings.Contains(capturedBriefing, "design output") {
		t.Fatalf("expected briefing snapshot to contain upstream artifact content, got %q", capturedBriefing)
	}

	// Verify collector extracted metadata into both artifacts.
	designArt, _ := store.GetLatestArtifactByStep(ctx, designID)
	if designArt.Metadata["collected"] != true {
		t.Fatalf("design artifact metadata not collected: %v", designArt.Metadata)
	}

	implArt, _ := store.GetLatestArtifactByStep(ctx, implID)
	if implArt.Metadata["collected"] != true {
		t.Fatalf("implement artifact metadata not collected: %v", implArt.Metadata)
	}
}

// TestIssueE2E_GateRejectRetryWithCollector: full gate reject → retry → pass cycle.
func TestIssueE2E_GateRejectRetryWithCollector(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var gateCount int32
	var implCount int32
	var deployCount int32

	collector := CollectorFunc(func(_ context.Context, stepType core.StepType, md string) (map[string]any, error) {
		return map[string]any{"step_type": string(stepType)}, nil
	})

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		if step.Type == core.StepGate {
			n := atomic.AddInt32(&gateCount, 1)
			verdict := "reject"
			if n > 1 {
				verdict = "pass"
			}
			_, err := store.CreateArtifact(ctx, &core.Artifact{
				ExecutionID:    exec.ID,
				StepID:         step.ID,
				IssueID:        step.IssueID,
				ResultMarkdown: "Review feedback",
				Metadata:       map[string]any{"verdict": verdict, "reason": "iteration " + fmt.Sprint(n)},
			})
			return err
		}
		if step.Name == "impl" {
			atomic.AddInt32(&implCount, 1)
		} else if step.Name == "deploy" {
			atomic.AddInt32(&deployCount, 1)
		}
		_, err := store.CreateArtifact(ctx, &core.Artifact{
			ExecutionID:    exec.ID,
			StepID:         step.ID,
			IssueID:        step.IssueID,
			ResultMarkdown: fmt.Sprintf("## %s output", step.Name),
		})
		return err
	}

	eng := New(store, bus, executor, WithConcurrency(1), WithCollector(collector))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "e2e-gate-retry", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{
		IssueID:    issueID,
		Name:       "impl",
		Type:       core.StepExec,
		Status:     core.StepPending,
		Position:   0,
		MaxRetries: 1,
	})
	store.CreateStep(ctx, &core.Step{
		IssueID:  issueID,
		Name:     "review",
		Type:     core.StepGate,
		Status:   core.StepPending,
		Position: 1,
	})
	deployID, _ := store.CreateStep(ctx, &core.Step{
		IssueID:  issueID,
		Name:     "deploy",
		Type:     core.StepExec,
		Status:   core.StepPending,
		Position: 2,
	})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}

	if implCount != 2 {
		t.Fatalf("expected 2 impl runs, got %d", implCount)
	}
	if gateCount != 2 {
		t.Fatalf("expected 2 gate evaluations, got %d", gateCount)
	}
	if deployCount != 1 {
		t.Fatalf("expected 1 deploy run, got %d", deployCount)
	}

	deployStep, _ := store.GetStep(ctx, deployID)
	if deployStep.Status != core.StepDone {
		t.Fatalf("expected deploy done, got %s", deployStep.Status)
	}

	deployArt, _ := store.GetLatestArtifactByStep(ctx, deployID)
	if deployArt.Metadata["step_type"] != "exec" {
		t.Fatalf("deploy artifact missing collected metadata: %v", deployArt.Metadata)
	}
}

// TestIssueE2E_CompositeWithGate: composite containing a gate inside its child issue.
func TestIssueE2E_CompositeWithGate(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var callOrder []string
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		callOrder = append(callOrder, step.Name)
		if step.Type == core.StepGate {
			_, err := store.CreateArtifact(ctx, &core.Artifact{
				ExecutionID:    exec.ID,
				StepID:         step.ID,
				IssueID:        step.IssueID,
				ResultMarkdown: "Gate pass",
				Metadata:       map[string]any{"verdict": "pass"},
			})
			return err
		}
		return nil
	}

	expander := ExpanderFunc(func(_ context.Context, step *core.Step) ([]*core.Step, error) {
		return []*core.Step{
			{Name: "B1", Type: core.StepExec},
			{Name: "B2", Type: core.StepGate},
		}, nil
	})

	eng := New(store, bus, executor, WithConcurrency(1), WithExpander(expander))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "e2e-composite-gate", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "A", Type: core.StepExec, Status: core.StepPending, Position: 0})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "B", Type: core.StepComposite, Status: core.StepPending, Position: 1})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "C", Type: core.StepExec, Status: core.StepPending, Position: 2})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}

	if len(callOrder) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != "A" || callOrder[3] != "C" {
		t.Fatalf("expected A..C ordering, got %v", callOrder)
	}
}

// TestIssueE2E_FanOutMerge: steps at different Positions execute sequentially.
func TestIssueE2E_FanOutMerge(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var counter int32
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		atomic.AddInt32(&counter, 1)
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(4))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "e2e-fan-merge", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "A", Type: core.StepExec, Status: core.StepPending, Position: 0})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "B", Type: core.StepExec, Status: core.StepPending, Position: 1})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "C", Type: core.StepExec, Status: core.StepPending, Position: 1})
	dID, _ := store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "D", Type: core.StepExec, Status: core.StepPending, Position: 2})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}
	if counter != 4 {
		t.Fatalf("expected 4 executions, got %d", counter)
	}

	stepD, _ := store.GetStep(ctx, dID)
	if stepD.Status != core.StepDone {
		t.Fatalf("expected D done, got %s", stepD.Status)
	}
}

// TestIssueE2E_TimeoutRetryGatePass: slow step times out → retries → gate passes → done.
func TestIssueE2E_TimeoutRetryGatePass(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var implAttempts int32
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		if step.Name == "impl" {
			n := atomic.AddInt32(&implAttempts, 1)
			if n == 1 {
				select {
				case <-time.After(500 * time.Millisecond):
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		}
		if step.Type == core.StepGate {
			_, err := store.CreateArtifact(ctx, &core.Artifact{
				ExecutionID:    exec.ID,
				StepID:         step.ID,
				IssueID:        step.IssueID,
				ResultMarkdown: "Approved",
				Metadata:       map[string]any{"verdict": "pass"},
			})
			return err
		}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "e2e-timeout-gate", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{
		IssueID:    issueID,
		Name:       "impl",
		Type:       core.StepExec,
		Status:     core.StepPending,
		Position:   0,
		Timeout:    50 * time.Millisecond,
		MaxRetries: 1,
	})
	store.CreateStep(ctx, &core.Step{
		IssueID:  issueID,
		Name:     "review",
		Type:     core.StepGate,
		Status:   core.StepPending,
		Position: 1,
	})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}
	if implAttempts != 2 {
		t.Fatalf("expected 2 impl attempts, got %d", implAttempts)
	}
}

// TestIssueE2E_PermanentErrorStopsIssue: step hits permanent error.
func TestIssueE2E_PermanentErrorStopsIssue(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		if step.Name == "B" {
			exec.ErrorKind = core.ErrKindPermanent
			return fmt.Errorf("bad config")
		}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(4))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "e2e-permanent", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "A", Type: core.StepExec, Status: core.StepPending, Position: 0})
	bID, _ := store.CreateStep(ctx, &core.Step{
		IssueID:    issueID,
		Name:       "B",
		Type:       core.StepExec,
		Status:     core.StepPending,
		Position:   1,
		MaxRetries: 3,
	})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "C", Type: core.StepExec, Status: core.StepPending, Position: 1})

	err := eng.Run(ctx, issueID)
	if err == nil {
		t.Fatal("expected error")
	}

	stepB, _ := store.GetStep(ctx, bID)
	if stepB.RetryCount != 0 {
		t.Fatalf("permanent error should skip retry, got retry_count=%d", stepB.RetryCount)
	}
	if stepB.Status != core.StepFailed {
		t.Fatalf("expected B failed, got %s", stepB.Status)
	}
}

// TestIssueE2E_FullOrchestration: all features in a single issue.
// Issue: design(exec) → impl(composite[code,test]) → review(gate,reject→pass) → deploy(exec)
func TestIssueE2E_FullOrchestration(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	profiles := []*core.AgentProfile{
		{ID: "architect", Role: core.RoleWorker, Capabilities: []string{"design"}},
		{ID: "coder", Role: core.RoleWorker, Capabilities: []string{"go"}},
		{ID: "reviewer", Role: core.RoleGate, Capabilities: []string{"review"}},
		{ID: "deployer", Role: core.RoleWorker, Capabilities: []string{"deploy"}},
	}

	collector := CollectorFunc(func(_ context.Context, stepType core.StepType, md string) (map[string]any, error) {
		return map[string]any{"collected": true}, nil
	})

	var gateCount int32
	var designCount int32
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		switch step.Name {
		case "design":
			atomic.AddInt32(&designCount, 1)
			_, err := store.CreateArtifact(ctx, &core.Artifact{
				ExecutionID: exec.ID, StepID: step.ID, IssueID: step.IssueID,
				ResultMarkdown: "## Architecture\nLogin API with JWT.",
			})
			return err
		case "code", "test":
			_, err := store.CreateArtifact(ctx, &core.Artifact{
				ExecutionID: exec.ID, StepID: step.ID, IssueID: step.IssueID,
				ResultMarkdown: fmt.Sprintf("## %s\nDone.", step.Name),
			})
			return err
		case "review":
			n := atomic.AddInt32(&gateCount, 1)
			verdict := "reject"
			if n > 1 {
				verdict = "pass"
			}
			_, err := store.CreateArtifact(ctx, &core.Artifact{
				ExecutionID: exec.ID, StepID: step.ID, IssueID: step.IssueID,
				ResultMarkdown: "Review feedback",
				Metadata:       map[string]any{"verdict": verdict, "reason": "round " + fmt.Sprint(n)},
			})
			return err
		case "deploy":
			_, err := store.CreateArtifact(ctx, &core.Artifact{
				ExecutionID: exec.ID, StepID: step.ID, IssueID: step.IssueID,
				ResultMarkdown: "## Deploy\nDeployed to staging.",
			})
			return err
		}
		return nil
	}

	expander := ExpanderFunc(func(_ context.Context, step *core.Step) ([]*core.Step, error) {
		return []*core.Step{
			{Name: "code", Type: core.StepExec, AgentRole: "worker", RequiredCapabilities: []string{"go"}},
			{Name: "test", Type: core.StepExec, AgentRole: "worker", RequiredCapabilities: []string{"go"}},
		}, nil
	})

	eng := New(store, bus, executor,
		WithConcurrency(2),
		WithResolver(NewProfileRegistry(profiles)),
		WithBriefingBuilder(NewBriefingBuilder(store)),
		WithCollector(collector),
		WithExpander(expander),
	)

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "e2e-full", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{
		IssueID:              issueID,
		Name:                 "design",
		Type:                 core.StepExec,
		Status:               core.StepPending,
		Position:             0,
		AgentRole:            "worker",
		RequiredCapabilities: []string{"design"},
	})
	implID, _ := store.CreateStep(ctx, &core.Step{
		IssueID:    issueID,
		Name:       "impl",
		Type:       core.StepComposite,
		Status:     core.StepPending,
		Position:   1,
		MaxRetries: 1,
	})
	store.CreateStep(ctx, &core.Step{
		IssueID:              issueID,
		Name:                 "review",
		Type:                 core.StepGate,
		Status:               core.StepPending,
		Position:             2,
		AgentRole:            "gate",
		RequiredCapabilities: []string{"review"},
	})
	deployID, _ := store.CreateStep(ctx, &core.Step{
		IssueID:              issueID,
		Name:                 "deploy",
		Type:                 core.StepExec,
		Status:               core.StepPending,
		Position:             3,
		AgentRole:            "worker",
		RequiredCapabilities: []string{"deploy"},
	})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Verify issue completed.
	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}

	// Verify all steps done.
	for _, id := range []int64{implID, deployID} {
		s, _ := store.GetStep(ctx, id)
		if s.Status != core.StepDone {
			t.Fatalf("step %s (id=%d) expected done, got %s", s.Name, id, s.Status)
		}
	}

	// Gate rejected once then passed.
	if gateCount != 2 {
		t.Fatalf("expected 2 gate evaluations, got %d", gateCount)
	}

	// Composite should have been expanded (impl has child_issue_id).
	implStep, _ := store.GetStep(ctx, implID)
	if childIssueID(implStep) == nil {
		t.Fatal("expected impl to have child_issue_id")
	}

	// Collector should have enriched deploy artifact.
	deployArt, _ := store.GetLatestArtifactByStep(ctx, deployID)
	if deployArt == nil {
		t.Fatal("expected deploy artifact")
	}
	if deployArt.Metadata["collected"] != true {
		t.Fatalf("expected collected metadata on deploy, got %v", deployArt.Metadata)
	}

	// Design should have run only once.
	if designCount != 1 {
		t.Fatalf("expected 1 design run, got %d", designCount)
	}
}
