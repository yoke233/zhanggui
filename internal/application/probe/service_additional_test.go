package probe

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type probeStoreWrapper struct {
	core.Store

	getRunErr             error
	activeProbe           *core.ActionSignal
	activeProbeErr        error
	route                 *core.RunProbeRoute
	routeErr              error
	updateProbeErr        error
	listProbeSignals      []*core.ActionSignal
	listProbeSignalsErr   error
	latestProbeSignalErr  error
	listRuns              []*core.Run
	listRunsErr           error
	latestRunEventTime    *time.Time
	latestRunEventTimeErr error
}

func (w *probeStoreWrapper) GetRun(ctx context.Context, id int64) (*core.Run, error) {
	if w.getRunErr != nil {
		return nil, w.getRunErr
	}
	return w.Store.GetRun(ctx, id)
}

func (w *probeStoreWrapper) GetActiveProbeSignal(ctx context.Context, runID int64) (*core.ActionSignal, error) {
	if w.activeProbeErr != nil {
		return nil, w.activeProbeErr
	}
	if w.activeProbe != nil {
		return w.activeProbe, nil
	}
	return w.Store.GetActiveProbeSignal(ctx, runID)
}

func (w *probeStoreWrapper) GetRunProbeRoute(ctx context.Context, runID int64) (*core.RunProbeRoute, error) {
	if w.routeErr != nil {
		return nil, w.routeErr
	}
	if w.route != nil {
		return w.route, nil
	}
	return w.Store.GetRunProbeRoute(ctx, runID)
}

func (w *probeStoreWrapper) UpdateProbeSignal(ctx context.Context, sig *core.ActionSignal) error {
	if w.updateProbeErr != nil {
		return w.updateProbeErr
	}
	return w.Store.UpdateProbeSignal(ctx, sig)
}

func (w *probeStoreWrapper) ListProbeSignalsByRun(ctx context.Context, runID int64) ([]*core.ActionSignal, error) {
	if w.listProbeSignalsErr != nil {
		return nil, w.listProbeSignalsErr
	}
	if w.listProbeSignals != nil {
		return w.listProbeSignals, nil
	}
	return w.Store.ListProbeSignalsByRun(ctx, runID)
}

func (w *probeStoreWrapper) GetLatestProbeSignal(ctx context.Context, runID int64) (*core.ActionSignal, error) {
	if w.latestProbeSignalErr != nil {
		return nil, w.latestProbeSignalErr
	}
	return w.Store.GetLatestProbeSignal(ctx, runID)
}

func (w *probeStoreWrapper) ListRunsByStatus(ctx context.Context, status core.RunStatus) ([]*core.Run, error) {
	if w.listRunsErr != nil {
		return nil, w.listRunsErr
	}
	if w.listRuns != nil {
		return w.listRuns, nil
	}
	return w.Store.ListRunsByStatus(ctx, status)
}

func (w *probeStoreWrapper) GetLatestRunEventTime(ctx context.Context, runID int64, eventType core.EventType) (*time.Time, error) {
	if w.latestRunEventTimeErr != nil {
		return nil, w.latestRunEventTimeErr
	}
	if w.latestRunEventTime != nil {
		return w.latestRunEventTime, nil
	}
	return w.Store.GetLatestRunEventTime(ctx, runID, eventType)
}

func TestRunProbeService_ListAndGetLatestRunProbe(t *testing.T) {
	store := setupProbeStore(t)
	runRec, _ := seedRunningRun(t, store)
	runtime := &probeRuntimeStub{
		result: &RunProbeRuntimeResult{
			Reachable:  true,
			Answered:   true,
			ReplyText:  "still alive",
			ObservedAt: time.Now().UTC(),
		},
	}
	service := NewRunProbeService(RunProbeServiceConfig{
		Store:          store,
		SessionManager: runtime,
	})

	probe, err := service.RequestRunProbe(context.Background(), runRec.ID, core.RunProbeTriggerManual, "   ", 5*time.Second)
	if err != nil {
		t.Fatalf("RequestRunProbe() error = %v", err)
	}
	if runtime.last.Question != defaultRunProbeQuestion {
		t.Fatalf("runtime question = %q, want default question", runtime.last.Question)
	}

	probes, err := service.ListRunProbes(context.Background(), runRec.ID)
	if err != nil {
		t.Fatalf("ListRunProbes() error = %v", err)
	}
	if len(probes) != 1 {
		t.Fatalf("ListRunProbes() len = %d, want 1 updated probe signal", len(probes))
	}

	latest, err := service.GetLatestRunProbe(context.Background(), runRec.ID)
	if err != nil {
		t.Fatalf("GetLatestRunProbe() error = %v", err)
	}
	if latest.ID != probe.ID || latest.Status != core.RunProbeAnswered || latest.Verdict != core.RunProbeAlive {
		t.Fatalf("latest probe = %#v", latest)
	}
}

func TestRunProbeService_RequestRunProbeRejectsNonRunningRun(t *testing.T) {
	store := setupProbeStore(t)
	runRec, _ := seedRunningRun(t, store)
	runRec.Status = core.RunSucceeded
	if err := store.UpdateRun(context.Background(), runRec); err != nil {
		t.Fatalf("UpdateRun() error = %v", err)
	}

	service := NewRunProbeService(RunProbeServiceConfig{
		Store:          store,
		SessionManager: &probeRuntimeStub{},
	})

	_, err := service.RequestRunProbe(context.Background(), runRec.ID, core.RunProbeTriggerManual, "status?", 0)
	if !errors.Is(err, ErrRunNotRunning) {
		t.Fatalf("RequestRunProbe() error = %v, want ErrRunNotRunning", err)
	}
}

func TestRunProbeService_RequestRunProbeRuntimeErrorPersistsFailure(t *testing.T) {
	store := setupProbeStore(t)
	runRec, _ := seedRunningRun(t, store)
	service := NewRunProbeService(RunProbeServiceConfig{
		Store:          store,
		SessionManager: &probeRuntimeStub{err: errors.New("runtime unavailable")},
	})

	probe, err := service.RequestRunProbe(context.Background(), runRec.ID, core.RunProbeTriggerManual, "status?", time.Second)
	if err != nil {
		t.Fatalf("RequestRunProbe() error = %v", err)
	}
	if probe.Status != core.RunProbeFailed || probe.Verdict != core.RunProbeUnknown {
		t.Fatalf("probe = %#v, want failed/unknown", probe)
	}
	if probe.Error != "runtime unavailable" {
		t.Fatalf("probe.Error = %q, want runtime unavailable", probe.Error)
	}

	latest, err := service.GetLatestRunProbe(context.Background(), runRec.ID)
	if err != nil {
		t.Fatalf("GetLatestRunProbe() error = %v", err)
	}
	if latest.Status != core.RunProbeFailed || latest.Error != "runtime unavailable" {
		t.Fatalf("latest probe = %#v", latest)
	}
}

func TestRunProbeService_RequestRunProbeStoreErrors(t *testing.T) {
	store := setupProbeStore(t)
	runRec, agentCtx := seedRunningRun(t, store)

	t.Run("active probe lookup error", func(t *testing.T) {
		service := NewRunProbeService(RunProbeServiceConfig{
			Store: &probeStoreWrapper{
				Store:          store,
				activeProbeErr: errors.New("db down"),
			},
			SessionManager: &probeRuntimeStub{},
		})
		_, err := service.RequestRunProbe(context.Background(), runRec.ID, core.RunProbeTriggerManual, "status?", 0)
		if err == nil || err.Error() != "db down" {
			t.Fatalf("RequestRunProbe() error = %v, want db down", err)
		}
	})

	t.Run("route lookup error", func(t *testing.T) {
		service := NewRunProbeService(RunProbeServiceConfig{
			Store: &probeStoreWrapper{
				Store:    store,
				routeErr: errors.New("route missing"),
			},
			SessionManager: &probeRuntimeStub{},
		})
		_, err := service.RequestRunProbe(context.Background(), runRec.ID, core.RunProbeTriggerManual, "status?", 0)
		if err == nil || err.Error() != "route missing" {
			t.Fatalf("RequestRunProbe() error = %v, want route missing", err)
		}
	})

	t.Run("update signal error", func(t *testing.T) {
		service := NewRunProbeService(RunProbeServiceConfig{
			Store: &probeStoreWrapper{
				Store:          store,
				route:          &core.RunProbeRoute{RunID: runRec.ID, WorkItemID: runRec.WorkItemID, ActionID: runRec.ActionID, AgentContextID: &agentCtx.ID, SessionID: agentCtx.SessionID, OwnerID: agentCtx.WorkerID},
				updateProbeErr: errors.New("update failed"),
			},
			SessionManager: &probeRuntimeStub{},
		})
		_, err := service.RequestRunProbe(context.Background(), runRec.ID, core.RunProbeTriggerManual, "status?", 0)
		if err == nil || err.Error() != "update failed" {
			t.Fatalf("RequestRunProbe() error = %v, want update failed", err)
		}
	})
}

func TestRunProbeService_ApplyRuntimeResultBranches(t *testing.T) {
	service := NewRunProbeService(RunProbeServiceConfig{})

	t.Run("unreachable", func(t *testing.T) {
		probe := &core.RunProbe{}
		service.applyRuntimeResult(probe, &RunProbeRuntimeResult{Reachable: false})
		if probe.Status != core.RunProbeUnreachable || probe.Verdict != core.RunProbeUnknown {
			t.Fatalf("probe = %#v", probe)
		}
	})

	t.Run("answered infers blocked", func(t *testing.T) {
		probe := &core.RunProbe{}
		service.applyRuntimeResult(probe, &RunProbeRuntimeResult{
			Reachable: true,
			Answered:  true,
			ReplyText: "waiting for approval from user",
		})
		if probe.Status != core.RunProbeAnswered || probe.Verdict != core.RunProbeBlocked || probe.AnsweredAt == nil {
			t.Fatalf("probe = %#v", probe)
		}
	})

	t.Run("timeout becomes hung", func(t *testing.T) {
		probe := &core.RunProbe{}
		service.applyRuntimeResult(probe, &RunProbeRuntimeResult{
			Reachable: true,
			Error:     "Probe timeout exceeded",
		})
		if probe.Status != core.RunProbeTimeout || probe.Verdict != core.RunProbeHung {
			t.Fatalf("probe = %#v", probe)
		}
	})

	t.Run("other failures stay unknown", func(t *testing.T) {
		probe := &core.RunProbe{}
		service.applyRuntimeResult(probe, &RunProbeRuntimeResult{
			Reachable: true,
			Error:     "transport error",
		})
		if probe.Status != core.RunProbeFailed || probe.Verdict != core.RunProbeUnknown {
			t.Fatalf("probe = %#v", probe)
		}
	})
}

func TestRunProbeService_PublishTerminalProbeEvent(t *testing.T) {
	bus := NewMemBus()
	sub := bus.Subscribe(core.SubscribeOpts{BufferSize: 10})
	defer sub.Cancel()

	service := NewRunProbeService(RunProbeServiceConfig{Bus: bus})
	service.publishTerminalProbeEvent(context.Background(), &core.RunProbe{Status: core.RunProbeAnswered})
	service.publishTerminalProbeEvent(context.Background(), &core.RunProbe{Status: core.RunProbeTimeout})
	service.publishTerminalProbeEvent(context.Background(), &core.RunProbe{Status: core.RunProbeUnreachable})
	service.publishTerminalProbeEvent(context.Background(), &core.RunProbe{Status: core.RunProbeFailed})

	var got []core.EventType
	timeout := time.After(200 * time.Millisecond)
	for len(got) < 3 {
		select {
		case ev := <-sub.C:
			got = append(got, ev.Type)
		case <-timeout:
			t.Fatalf("published terminal events = %#v", got)
		}
	}
	if got[0] != core.EventRunProbeAnswered || got[1] != core.EventRunProbeTimeout || got[2] != core.EventRunProbeUnreachable {
		t.Fatalf("published terminal events = %#v", got)
	}
}

func TestRunProbeWatchdog_StartAndShouldProbeRunBranches(t *testing.T) {
	NewRunProbeWatchdog(nil, nil, RunProbeWatchdogConfig{}).Start(context.Background())

	store := setupProbeStore(t)
	runRec, _ := seedRunningRun(t, store)
	oldActivity := time.Now().UTC().Add(-30 * time.Minute)
	if _, err := store.CreateEvent(context.Background(), &core.Event{
		Type:       core.EventRunAgentOutput,
		WorkItemID: runRec.WorkItemID,
		ActionID:   runRec.ActionID,
		RunID:      runRec.ID,
		Timestamp:  oldActivity,
	}); err != nil {
		t.Fatalf("CreateEvent() error = %v", err)
	}

	runtime := &probeRuntimeStub{}
	service := NewRunProbeService(RunProbeServiceConfig{
		Store:          store,
		SessionManager: runtime,
	})
	watchdog := NewRunProbeWatchdog(store, service, RunProbeWatchdogConfig{
		Enabled:      true,
		Interval:     10 * time.Millisecond,
		ProbeAfter:   time.Minute,
		IdleAfter:    time.Minute,
		ProbeTimeout: time.Second,
		MaxAttempts:  1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		watchdog.Start(ctx)
		close(done)
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for runtime.calls == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchdog Start() did not stop after cancel")
	}
	if runtime.calls == 0 {
		t.Fatal("watchdog Start() should trigger at least one probe")
	}

	baseNow := time.Now().UTC()
	baseRun := &core.Run{ID: 99, CreatedAt: baseNow.Add(-30 * time.Minute)}

	cases := []struct {
		name string
		cfg  RunProbeWatchdogConfig
		wrap *probeStoreWrapper
		want bool
	}{
		{
			name: "probe after not reached",
			cfg:  RunProbeWatchdogConfig{ProbeAfter: time.Hour},
			wrap: &probeStoreWrapper{Store: store},
			want: false,
		},
		{
			name: "active probe exists",
			cfg:  RunProbeWatchdogConfig{},
			wrap: &probeStoreWrapper{Store: store, activeProbe: &core.ActionSignal{ID: 1}},
			want: false,
		},
		{
			name: "active probe lookup error",
			cfg:  RunProbeWatchdogConfig{},
			wrap: &probeStoreWrapper{Store: store, activeProbeErr: errors.New("lookup failed")},
			want: false,
		},
		{
			name: "list probes error",
			cfg:  RunProbeWatchdogConfig{},
			wrap: &probeStoreWrapper{Store: store, listProbeSignalsErr: errors.New("list failed")},
			want: false,
		},
		{
			name: "max attempts reached",
			cfg:  RunProbeWatchdogConfig{MaxAttempts: 1},
			wrap: &probeStoreWrapper{Store: store, listProbeSignals: []*core.ActionSignal{{ID: 1}}},
			want: false,
		},
		{
			name: "latest activity lookup error",
			cfg:  RunProbeWatchdogConfig{},
			wrap: &probeStoreWrapper{Store: store, latestRunEventTimeErr: errors.New("event failed")},
			want: false,
		},
		{
			name: "idle after not reached",
			cfg:  RunProbeWatchdogConfig{IdleAfter: time.Hour},
			wrap: &probeStoreWrapper{Store: store},
			want: false,
		},
		{
			name: "eligible",
			cfg:  RunProbeWatchdogConfig{ProbeAfter: time.Minute, IdleAfter: time.Minute, MaxAttempts: 10},
			wrap: &probeStoreWrapper{Store: store},
			want: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			w := NewRunProbeWatchdog(tt.wrap, service, tt.cfg)
			if got := w.shouldProbeRun(context.Background(), baseNow, baseRun); got != tt.want {
				t.Fatalf("shouldProbeRun() = %v, want %v", got, tt.want)
			}
		})
	}
}
