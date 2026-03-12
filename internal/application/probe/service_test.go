package probe

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
)

type probeRuntimeStub struct {
	result *ExecutionProbeRuntimeResult
	err    error
	calls  int
	last   ExecutionProbeRuntimeRequest
}

func (s *probeRuntimeStub) ProbeExecution(_ context.Context, req ExecutionProbeRuntimeRequest) (*ExecutionProbeRuntimeResult, error) {
	s.calls++
	s.last = req
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &ExecutionProbeRuntimeResult{Reachable: true, Answered: true, ReplyText: "alive", ObservedAt: time.Now().UTC()}, nil
}

func setupProbeStore(t *testing.T) core.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "probe.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func seedRunningExecution(t *testing.T, store core.Store) (*core.Execution, *core.AgentContext) {
	t.Helper()
	ctx := context.Background()
	issue := &core.Issue{Title: "probe-issue", Status: core.IssueRunning}
	issueID, err := store.CreateIssue(ctx, issue)
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	step := &core.Step{IssueID: issueID, Name: "probe-step", Type: core.StepExec, Status: core.StepRunning}
	stepID, err := store.CreateStep(ctx, step)
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	agentCtx := &core.AgentContext{
		AgentID:   "worker",
		IssueID:   issueID,
		SessionID: "session-1",
		WorkerID:  "worker-a",
	}
	agentCtxID, err := store.CreateAgentContext(ctx, agentCtx)
	if err != nil {
		t.Fatalf("create agent context: %v", err)
	}
	agentCtx.ID = agentCtxID
	startedAt := time.Now().UTC().Add(-30 * time.Minute)
	execRec := &core.Execution{
		StepID:         stepID,
		IssueID:        issueID,
		Status:         core.ExecRunning,
		Attempt:        1,
		StartedAt:      &startedAt,
		AgentContextID: &agentCtxID,
	}
	execID, err := store.CreateExecution(ctx, execRec)
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	execRec.ID = execID
	return execRec, agentCtx
}

func TestExecutionProbeService_RequestExecutionProbeAnsweredBlocked(t *testing.T) {
	store := setupProbeStore(t)
	bus := NewMemBus()
	execRec, _ := seedRunningExecution(t, store)
	runtime := &probeRuntimeStub{
		result: &ExecutionProbeRuntimeResult{
			Reachable:  true,
			Answered:   true,
			ReplyText:  "I need authorization to continue.",
			ObservedAt: time.Now().UTC(),
		},
	}
	service := NewExecutionProbeService(ExecutionProbeServiceConfig{
		Store:          store,
		Bus:            bus,
		SessionManager: runtime,
	})

	sub := bus.Subscribe(core.SubscribeOpts{BufferSize: 10})
	defer sub.Cancel()

	probe, err := service.RequestExecutionProbe(context.Background(), execRec.ID, core.ExecutionProbeTriggerManual, "", 15*time.Second)
	if err != nil {
		t.Fatalf("RequestExecutionProbe: %v", err)
	}
	if probe.Status != core.ExecutionProbeAnswered {
		t.Fatalf("probe status = %s, want answered", probe.Status)
	}
	if probe.Verdict != core.ExecutionProbeBlocked {
		t.Fatalf("probe verdict = %s, want blocked", probe.Verdict)
	}
	if runtime.calls != 1 {
		t.Fatalf("probe runtime calls = %d, want 1", runtime.calls)
	}
	if runtime.last.OwnerID != "worker-a" {
		t.Fatalf("probe owner = %q, want worker-a", runtime.last.OwnerID)
	}

	var gotRequested, gotSent, gotAnswered bool
	timeout := time.After(200 * time.Millisecond)
	for !(gotRequested && gotSent && gotAnswered) {
		select {
		case ev := <-sub.C:
			switch ev.Type {
			case core.EventExecProbeRequested:
				gotRequested = true
			case core.EventExecProbeSent:
				gotSent = true
			case core.EventExecProbeAnswered:
				gotAnswered = true
			}
		case <-timeout:
			t.Fatalf("expected probe lifecycle events, got requested=%v sent=%v answered=%v", gotRequested, gotSent, gotAnswered)
		}
	}
}

func TestExecutionProbeService_RejectsConcurrentActiveProbe(t *testing.T) {
	store := setupProbeStore(t)
	execRec, agentCtx := seedRunningExecution(t, store)
	now := time.Now().UTC()
	if _, err := store.CreateExecutionProbe(context.Background(), &core.ExecutionProbe{
		ExecutionID:    execRec.ID,
		IssueID:        execRec.IssueID,
		StepID:         execRec.StepID,
		AgentContextID: &agentCtx.ID,
		SessionID:      agentCtx.SessionID,
		OwnerID:        agentCtx.WorkerID,
		TriggerSource:  core.ExecutionProbeTriggerManual,
		Question:       "probe",
		Status:         core.ExecutionProbeSent,
		Verdict:        core.ExecutionProbeUnknown,
		SentAt:         &now,
	}); err != nil {
		t.Fatalf("CreateExecutionProbe: %v", err)
	}

	service := NewExecutionProbeService(ExecutionProbeServiceConfig{
		Store:          store,
		SessionManager: &probeRuntimeStub{},
	})
	_, err := service.RequestExecutionProbe(context.Background(), execRec.ID, core.ExecutionProbeTriggerManual, "probe", 0)
	if !errors.Is(err, ErrExecutionProbeConflict) {
		t.Fatalf("expected ErrExecutionProbeConflict, got %v", err)
	}
}

func TestExecutionProbeWatchdog_TriggersOnlyWhenIdle(t *testing.T) {
	ctx := context.Background()
	store := setupProbeStore(t)
	execRec, _ := seedRunningExecution(t, store)
	oldActivity := time.Now().UTC().Add(-20 * time.Minute)
	if _, err := store.CreateEvent(ctx, &core.Event{
		Type:      core.EventExecAgentOutput,
		IssueID:   execRec.IssueID,
		StepID:    execRec.StepID,
		ExecID:    execRec.ID,
		Data:      map[string]any{"type": "agent_message", "content": "still running"},
		Timestamp: oldActivity,
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	runtime := &probeRuntimeStub{
		result: &ExecutionProbeRuntimeResult{
			Reachable:  true,
			Answered:   true,
			ReplyText:  "alive",
			ObservedAt: time.Now().UTC(),
		},
	}
	service := NewExecutionProbeService(ExecutionProbeServiceConfig{
		Store:          store,
		SessionManager: runtime,
	})
	watchdog := NewExecutionProbeWatchdog(store, service, ExecutionProbeWatchdogConfig{
		Enabled:      true,
		ProbeAfter:   10 * time.Minute,
		IdleAfter:    5 * time.Minute,
		ProbeTimeout: 15 * time.Second,
		MaxAttempts:  1,
	})

	watchdog.runOnce(ctx)
	if runtime.calls != 1 {
		t.Fatalf("expected 1 probe request, got %d", runtime.calls)
	}

	store2 := setupProbeStore(t)
	execRec2, _ := seedRunningExecution(t, store2)
	recentActivity := time.Now().UTC().Add(-30 * time.Second)
	if _, err := store2.CreateEvent(ctx, &core.Event{
		Type:      core.EventExecAgentOutput,
		IssueID:   execRec2.IssueID,
		StepID:    execRec2.StepID,
		ExecID:    execRec2.ID,
		Data:      map[string]any{"type": "agent_message", "content": "fresh output"},
		Timestamp: recentActivity,
	}); err != nil {
		t.Fatalf("CreateEvent recent: %v", err)
	}

	runtime2 := &probeRuntimeStub{}
	service2 := NewExecutionProbeService(ExecutionProbeServiceConfig{
		Store:          store2,
		SessionManager: runtime2,
	})
	watchdog2 := NewExecutionProbeWatchdog(store2, service2, ExecutionProbeWatchdogConfig{
		Enabled:      true,
		ProbeAfter:   10 * time.Minute,
		IdleAfter:    5 * time.Minute,
		ProbeTimeout: 15 * time.Second,
		MaxAttempts:  1,
	})
	watchdog2.runOnce(ctx)
	if runtime2.calls != 0 {
		t.Fatalf("expected no probe for recent activity, got %d", runtime2.calls)
	}
}
