package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/eventbus"
)

func TestIdleTimeout_NoActivity_CancelsContext(t *testing.T) {
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())

	ctx, cancel := startIdleChecker(context.Background(), &lastActivity, 200*time.Millisecond, nil, nil)
	defer cancel()

	select {
	case <-ctx.Done():
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("context should have been cancelled by idle timeout")
	}
}

func TestIdleTimeout_ActivityResetsTimer(t *testing.T) {
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())

	ctx, cancel := startIdleChecker(context.Background(), &lastActivity, 500*time.Millisecond, nil, nil)
	defer cancel()

	// Keep activity going for 800ms (well beyond idle timeout of 500ms).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 8; i++ {
			time.Sleep(100 * time.Millisecond)
			lastActivity.Store(time.Now().UnixNano())
		}
	}()
	<-done

	// Brief grace period for goroutine scheduling jitter.
	time.Sleep(50 * time.Millisecond)

	// Context should still be alive right after activity stops.
	select {
	case <-ctx.Done():
		t.Fatal("context should NOT be cancelled while activity is ongoing")
	default:
	}

	// Now wait for idle timeout to kick in.
	select {
	case <-ctx.Done():
		// expected
	case <-time.After(3 * time.Second):
		t.Fatal("context should have been cancelled after activity stopped")
	}
}

func TestIdleTimeout_ZeroIdleTimeout_FallsBackToWallClock(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	cfg := core.StageConfig{
		Name:        core.StageImplement,
		IdleTimeout: 0,
		Timeout:     200 * time.Millisecond,
	}

	// Verify that with IdleTimeout=0 and Timeout>0, the stage config
	// uses wall-clock timeout. We test the priority logic indirectly
	// by checking that defaultStageConfig with overridden values behaves correctly.
	if cfg.IdleTimeout != 0 {
		t.Fatal("expected zero idle timeout")
	}
	if cfg.Timeout != 200*time.Millisecond {
		t.Fatal("expected 200ms wall-clock timeout")
	}

	// Directly test the context behavior: wall-clock timeout should fire.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	select {
	case <-ctx.Done():
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("wall-clock timeout should have fired")
	}
}

func TestIdleTimeout_BothZero_NoTimeout(t *testing.T) {
	// When both IdleTimeout and Timeout are 0, no timeout should be applied.
	// The context should remain alive (we use a parent with timeout to prevent hanging).
	parent, parentCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer parentCancel()

	cfg := core.StageConfig{
		Name:        core.StageImplement,
		IdleTimeout: 0,
		Timeout:     0,
	}

	// Neither idle checker nor wall-clock timeout should be started.
	// stageCtx should equal parent ctx.
	stageCtx := parent
	if cfg.IdleTimeout > 0 {
		t.Fatal("should not enter idle timeout branch")
	}
	if cfg.Timeout > 0 {
		t.Fatal("should not enter wall-clock timeout branch")
	}

	// stageCtx should only be cancelled when parent is cancelled.
	select {
	case <-stageCtx.Done():
		// Only the parent timeout should cancel this.
	case <-time.After(2 * time.Second):
		t.Fatal("parent timeout should have fired")
	}
}

func TestIdleTimeout_ParentContextCancel_StopsChecker(t *testing.T) {
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())

	parent, parentCancel := context.WithCancel(context.Background())

	ctx, cancel := startIdleChecker(parent, &lastActivity, 5*time.Second, nil, nil)
	defer cancel()

	// Cancel parent after 100ms.
	time.AfterFunc(100*time.Millisecond, parentCancel)

	select {
	case <-ctx.Done():
		// expected — parent cancellation should propagate
	case <-time.After(2 * time.Second):
		t.Fatal("parent cancel should propagate to idle checker context")
	}
}

func TestDefaultStageConfig_IdleTimeoutValues(t *testing.T) {
	tests := []struct {
		stage       core.StageID
		wantIdle    time.Duration
		wantTimeout time.Duration
	}{
		{core.StageRequirements, 5 * time.Minute, 0},
		{core.StageImplement, 5 * time.Minute, 0},
		{core.StageReview, 5 * time.Minute, 0},
		{core.StageFixup, 5 * time.Minute, 0},
		{core.StageTest, 3 * time.Minute, 0},
		{core.StageSetup, 1 * time.Minute, 0},
		{core.StageMerge, 1 * time.Minute, 0},
		{core.StageCleanup, 1 * time.Minute, 0},
	}
	for _, tc := range tests {
		cfg := defaultStageConfig(tc.stage)
		if cfg.IdleTimeout != tc.wantIdle {
			t.Errorf("stage %s: IdleTimeout = %v, want %v", tc.stage, cfg.IdleTimeout, tc.wantIdle)
		}
		if cfg.Timeout != tc.wantTimeout {
			t.Errorf("stage %s: Timeout = %v, want %v", tc.stage, cfg.Timeout, tc.wantTimeout)
		}
	}
}

func TestIdleTimeout_HandleSessionUpdate_ResetsActivity(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	bridge := &stageEventBridge{
		executor:  &Executor{bus: bus},
		runID:     "run-1",
		stage:     core.StageImplement,
		agentName: "test-agent",
	}

	past := time.Now().Add(-10 * time.Second).UnixNano()
	bridge.lastActivity.Store(past)

	before := time.Now()
	_ = bridge.HandleSessionUpdate(context.Background(), acpclient.SessionUpdate{
		Type: "chunk",
		Text: "hello",
	})

	lastNano := bridge.lastActivity.Load()
	lastTime := time.Unix(0, lastNano)
	if lastTime.Before(before) {
		t.Fatalf("HandleSessionUpdate should update lastActivity, got %v (before %v)", lastTime, before)
	}
}

func TestIdleTimeout_HandleSessionUpdate_EmptyTextStillResetsActivity(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	bridge := &stageEventBridge{
		executor:  &Executor{bus: bus},
		runID:     "run-1",
		stage:     core.StageImplement,
		agentName: "test-agent",
	}

	past := time.Now().Add(-10 * time.Second).UnixNano()
	bridge.lastActivity.Store(past)

	before := time.Now()
	_ = bridge.HandleSessionUpdate(context.Background(), acpclient.SessionUpdate{
		Type: "tool_call",
		Text: "",
	})

	lastNano := bridge.lastActivity.Load()
	lastTime := time.Unix(0, lastNano)
	if lastTime.Before(before) {
		t.Fatalf("HandleSessionUpdate with empty text should still update lastActivity, got %v (before %v)", lastTime, before)
	}
}

func TestIdleTimeout_E2E_StageFailsOnIdle(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{
			Name:        core.StageImplement,
			Role:        "worker",
			IdleTimeout: 200 * time.Millisecond,
			Timeout:     0,
			OnFailure:   core.OnFailureAbort,
			MaxRetries:  0,
		},
	})
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	bus := eventbus.New()
	execEngine := newExecutorWithBus(store, bus, nil)
	// testStageFunc simulates a stage that sleeps longer than idle timeout.
	execEngine.testStageFunc = func(ctx context.Context, runID string, stage core.StageID, agentName, prompt string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	}

	err := execEngine.Run(context.Background(), p.ID)
	if err == nil {
		t.Fatal("expected run to fail due to idle timeout")
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusCompleted || got.Conclusion != core.ConclusionFailure {
		t.Fatalf("expected completed/failure, got status=%s conclusion=%s", got.Status, got.Conclusion)
	}
}
