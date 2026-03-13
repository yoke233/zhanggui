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
	result *RunProbeRuntimeResult
	err    error
	calls  int
	last   RunProbeRuntimeRequest
}

func (s *probeRuntimeStub) ProbeRun(_ context.Context, req RunProbeRuntimeRequest) (*RunProbeRuntimeResult, error) {
	s.calls++
	s.last = req
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &RunProbeRuntimeResult{Reachable: true, Answered: true, ReplyText: "alive", ObservedAt: time.Now().UTC()}, nil
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

func seedRunningRun(t *testing.T, store core.Store) (*core.Run, *core.AgentContext) {
	t.Helper()
	ctx := context.Background()
	workItem := &core.WorkItem{Title: "probe-workitem", Status: core.WorkItemRunning}
	workItemID, err := store.CreateWorkItem(ctx, workItem)
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	action := &core.Action{WorkItemID: workItemID, Name: "probe-action", Type: core.ActionExec, Status: core.ActionRunning}
	actionID, err := store.CreateAction(ctx, action)
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	agentCtx := &core.AgentContext{
		AgentID:    "worker",
		WorkItemID: workItemID,
		SessionID:  "session-1",
		WorkerID:   "worker-a",
	}
	agentCtxID, err := store.CreateAgentContext(ctx, agentCtx)
	if err != nil {
		t.Fatalf("create agent context: %v", err)
	}
	agentCtx.ID = agentCtxID
	startedAt := time.Now().UTC().Add(-30 * time.Minute)
	runRec := &core.Run{
		ActionID:       actionID,
		WorkItemID:     workItemID,
		Status:         core.RunRunning,
		Attempt:        1,
		StartedAt:      &startedAt,
		AgentContextID: &agentCtxID,
	}
	runID, err := store.CreateRun(ctx, runRec)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	runRec.ID = runID
	return runRec, agentCtx
}

func TestRunProbeService_RequestRunProbeAnsweredBlocked(t *testing.T) {
	store := setupProbeStore(t)
	bus := NewMemBus()
	runRec, _ := seedRunningRun(t, store)
	runtime := &probeRuntimeStub{
		result: &RunProbeRuntimeResult{
			Reachable:  true,
			Answered:   true,
			ReplyText:  "I need authorization to continue.",
			ObservedAt: time.Now().UTC(),
		},
	}
	service := NewRunProbeService(RunProbeServiceConfig{
		Store:          store,
		Bus:            bus,
		SessionManager: runtime,
	})

	sub := bus.Subscribe(core.SubscribeOpts{BufferSize: 10})
	defer sub.Cancel()

	probe, err := service.RequestRunProbe(context.Background(), runRec.ID, core.RunProbeTriggerManual, "", 15*time.Second)
	if err != nil {
		t.Fatalf("RequestRunProbe: %v", err)
	}
	if probe.Status != core.RunProbeAnswered {
		t.Fatalf("probe status = %s, want answered", probe.Status)
	}
	if probe.Verdict != core.RunProbeBlocked {
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
			case core.EventRunProbeRequested:
				gotRequested = true
			case core.EventRunProbeSent:
				gotSent = true
			case core.EventRunProbeAnswered:
				gotAnswered = true
			}
		case <-timeout:
			t.Fatalf("expected probe lifecycle events, got requested=%v sent=%v answered=%v", gotRequested, gotSent, gotAnswered)
		}
	}
}

func TestRunProbeService_RejectsConcurrentActiveProbe(t *testing.T) {
	store := setupProbeStore(t)
	runRec, agentCtx := seedRunningRun(t, store)
	now := time.Now().UTC()
	if _, err := store.CreateRunProbe(context.Background(), &core.RunProbe{
		RunID:          runRec.ID,
		WorkItemID:     runRec.WorkItemID,
		ActionID:       runRec.ActionID,
		AgentContextID: &agentCtx.ID,
		SessionID:      agentCtx.SessionID,
		OwnerID:        agentCtx.WorkerID,
		TriggerSource:  core.RunProbeTriggerManual,
		Question:       "probe",
		Status:         core.RunProbeSent,
		Verdict:        core.RunProbeUnknown,
		SentAt:         &now,
	}); err != nil {
		t.Fatalf("CreateRunProbe: %v", err)
	}

	service := NewRunProbeService(RunProbeServiceConfig{
		Store:          store,
		SessionManager: &probeRuntimeStub{},
	})
	_, err := service.RequestRunProbe(context.Background(), runRec.ID, core.RunProbeTriggerManual, "probe", 0)
	if !errors.Is(err, ErrRunProbeConflict) {
		t.Fatalf("expected ErrRunProbeConflict, got %v", err)
	}
}

func TestRunProbeWatchdog_TriggersOnlyWhenIdle(t *testing.T) {
	ctx := context.Background()
	store := setupProbeStore(t)
	runRec, _ := seedRunningRun(t, store)
	oldActivity := time.Now().UTC().Add(-20 * time.Minute)
	if _, err := store.CreateEvent(ctx, &core.Event{
		Type:       core.EventRunAgentOutput,
		WorkItemID: runRec.WorkItemID,
		ActionID:   runRec.ActionID,
		RunID:      runRec.ID,
		Data:       map[string]any{"type": "agent_message", "content": "still running"},
		Timestamp:  oldActivity,
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	runtime := &probeRuntimeStub{
		result: &RunProbeRuntimeResult{
			Reachable:  true,
			Answered:   true,
			ReplyText:  "alive",
			ObservedAt: time.Now().UTC(),
		},
	}
	service := NewRunProbeService(RunProbeServiceConfig{
		Store:          store,
		SessionManager: runtime,
	})
	watchdog := NewRunProbeWatchdog(store, service, RunProbeWatchdogConfig{
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
	runRec2, _ := seedRunningRun(t, store2)
	recentActivity := time.Now().UTC().Add(-30 * time.Second)
	if _, err := store2.CreateEvent(ctx, &core.Event{
		Type:       core.EventRunAgentOutput,
		WorkItemID: runRec2.WorkItemID,
		ActionID:   runRec2.ActionID,
		RunID:      runRec2.ID,
		Data:       map[string]any{"type": "agent_message", "content": "fresh output"},
		Timestamp:  recentActivity,
	}); err != nil {
		t.Fatalf("CreateEvent recent: %v", err)
	}

	runtime2 := &probeRuntimeStub{}
	service2 := NewRunProbeService(RunProbeServiceConfig{
		Store:          store2,
		SessionManager: runtime2,
	})
	watchdog2 := NewRunProbeWatchdog(store2, service2, RunProbeWatchdogConfig{
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
