package agentruntime

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	membus "github.com/yoke233/ai-workflow/internal/adapters/events/memory"
	sqlitestore "github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
)

type recordingEventHandler struct {
	calls  int32
	update acpclient.SessionUpdate
}

func (h *recordingEventHandler) HandleSessionUpdate(_ context.Context, update acpclient.SessionUpdate) error {
	atomic.AddInt32(&h.calls, 1)
	h.update = update
	return nil
}

func newRuntimeTestStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "runtime-test.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestCloneEnv(t *testing.T) {
	clonedNil := cloneEnv(nil)
	if clonedNil == nil || len(clonedNil) != 0 {
		t.Fatalf("cloneEnv(nil) = %#v, want empty map", clonedNil)
	}

	original := map[string]string{"A": "1"}
	cloned := cloneEnv(original)
	cloned["A"] = "2"
	if original["A"] != "1" {
		t.Fatalf("expected original map to stay unchanged, got %q", original["A"])
	}
}

func TestSwitchingEventHandler(t *testing.T) {
	switcher := &switchingEventHandler{}
	if err := switcher.HandleSessionUpdate(context.Background(), acpclient.SessionUpdate{Type: "noop"}); err != nil {
		t.Fatalf("HandleSessionUpdate(nil) error = %v", err)
	}

	recorder := &recordingEventHandler{}
	switcher.Set(recorder)
	update := acpclient.SessionUpdate{Type: "agent_message", Text: "hello"}
	if err := switcher.HandleSessionUpdate(context.Background(), update); err != nil {
		t.Fatalf("HandleSessionUpdate(set) error = %v", err)
	}
	if recorder.calls != 1 || recorder.update.Text != "hello" {
		t.Fatalf("unexpected forwarded update: calls=%d update=%+v", recorder.calls, recorder.update)
	}
}

func TestACPSessionPoolAcquireReuseAndEviction(t *testing.T) {
	t.Run("reuses existing session", func(t *testing.T) {
		existing := &pooledACPSession{lastUsed: time.Now().UTC(), turns: 1}
		pool := &ACPSessionPool{
			sessions: map[acpSessionKey]*pooledACPSession{
				{workItemID: 7, agentID: "worker"}: existing,
			},
			inflight: make(map[acpSessionKey]*acpSessionFlight),
		}
		var createCalls int
		pool.createSessionFn = func(context.Context, acpSessionKey, acpSessionAcquireInput) (*pooledACPSession, *core.AgentContext, error) {
			createCalls++
			return &pooledACPSession{}, nil, nil
		}

		got, _, err := pool.Acquire(context.Background(), acpSessionAcquireInput{
			Profile:    &core.AgentProfile{ID: "worker"},
			WorkItemID: 7,
			IdleTTL:    time.Hour,
			MaxTurns:   10,
		})
		if err != nil {
			t.Fatalf("Acquire() error = %v", err)
		}
		if got != existing {
			t.Fatalf("Acquire() session = %p, want existing %p", got, existing)
		}
		if createCalls != 0 {
			t.Fatalf("createSessionFn called %d times, want 0", createCalls)
		}
	})

	t.Run("evicts idle session and recreates", func(t *testing.T) {
		existing := &pooledACPSession{
			lastUsed: time.Now().UTC().Add(-2 * time.Hour),
			client:   &acpclient.Client{},
		}
		recreated := &pooledACPSession{}
		pool := &ACPSessionPool{
			sessions: map[acpSessionKey]*pooledACPSession{
				{workItemID: 8, agentID: "worker"}: existing,
			},
			inflight: make(map[acpSessionKey]*acpSessionFlight),
			createSessionFn: func(context.Context, acpSessionKey, acpSessionAcquireInput) (*pooledACPSession, *core.AgentContext, error) {
				return recreated, nil, nil
			},
		}

		got, _, err := pool.Acquire(context.Background(), acpSessionAcquireInput{
			Profile:    &core.AgentProfile{ID: "worker"},
			WorkItemID: 8,
			IdleTTL:    time.Minute,
		})
		if err != nil {
			t.Fatalf("Acquire() error = %v", err)
		}
		if got != recreated {
			t.Fatalf("Acquire() session = %p, want recreated %p", got, recreated)
		}
	})

	t.Run("evicts max-turn session and recreates", func(t *testing.T) {
		existing := &pooledACPSession{
			lastUsed: time.Now().UTC(),
			turns:    5,
			client:   &acpclient.Client{},
		}
		recreated := &pooledACPSession{}
		pool := &ACPSessionPool{
			sessions: map[acpSessionKey]*pooledACPSession{
				{workItemID: 9, agentID: "worker"}: existing,
			},
			inflight: make(map[acpSessionKey]*acpSessionFlight),
			createSessionFn: func(context.Context, acpSessionKey, acpSessionAcquireInput) (*pooledACPSession, *core.AgentContext, error) {
				return recreated, nil, nil
			},
		}

		got, _, err := pool.Acquire(context.Background(), acpSessionAcquireInput{
			Profile:    &core.AgentProfile{ID: "worker"},
			WorkItemID: 9,
			MaxTurns:   5,
		})
		if err != nil {
			t.Fatalf("Acquire() error = %v", err)
		}
		if got != recreated {
			t.Fatalf("Acquire() session = %p, want recreated %p", got, recreated)
		}
	})
}

func TestACPSessionPoolLifecycleAndStats(t *testing.T) {
	store := newRuntimeTestStore(t)
	bus := membus.NewBus()
	pool := NewACPSessionPool(store, bus)
	defer pool.Close()

	ctx := context.Background()
	ac := &core.AgentContext{
		AgentID:    "worker",
		WorkItemID: 11,
		TurnCount:  1,
	}
	id, err := store.CreateAgentContext(ctx, ac)
	if err != nil {
		t.Fatalf("CreateAgentContext() error = %v", err)
	}
	ac.ID = id

	sess := &pooledACPSession{}
	pool.NoteTurn(ctx, ac, sess)
	pool.NoteTokens(sess, 10, 20)
	pool.NoteTokens(sess, 5, 7)

	if lastUsed, turns, input, output := sess.statsSnapshot(); lastUsed.IsZero() || turns != 1 || input != 15 || output != 27 {
		t.Fatalf("statsSnapshot() = (%v,%d,%d,%d)", lastUsed, turns, input, output)
	}
	if input, output := pool.SessionTokenUsage(sess); input != 15 || output != 27 {
		t.Fatalf("SessionTokenUsage() = (%d,%d), want (15,27)", input, output)
	}

	stored, err := store.FindAgentContext(ctx, "worker", 11)
	if err != nil {
		t.Fatalf("FindAgentContext() error = %v", err)
	}
	if stored.TurnCount != 2 {
		t.Fatalf("TurnCount = %d, want 2", stored.TurnCount)
	}

	keyA := acpSessionKey{workItemID: 11, agentID: "worker"}
	keyB := acpSessionKey{workItemID: 22, agentID: "worker"}
	pool.mu.Lock()
	pool.sessions[keyA] = sess
	pool.sessions[keyB] = &pooledACPSession{}
	pool.mu.Unlock()

	bus.Publish(ctx, core.Event{Type: core.EventWorkItemCompleted, WorkItemID: 11})

	deadline := time.Now().Add(time.Second)
	for {
		pool.mu.Lock()
		_, existsA := pool.sessions[keyA]
		_, existsB := pool.sessions[keyB]
		pool.mu.Unlock()
		if !existsA {
			if !existsB {
				t.Fatal("expected unrelated work item session to remain")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for lifecycle cleanup")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestACPSessionPoolHelpers(t *testing.T) {
	if _, _, err := (*ACPSessionPool)(nil).Acquire(context.Background(), acpSessionAcquireInput{}); err == nil {
		t.Fatal("expected nil pool acquire to fail")
	}

	pool := &ACPSessionPool{}
	if _, _, err := pool.Acquire(context.Background(), acpSessionAcquireInput{}); err == nil {
		t.Fatal("expected missing profile to fail")
	}

	if _, err := pool.findAgentContext(context.Background(), "worker", 1); err != core.ErrNotFound {
		t.Fatalf("findAgentContext(nil store) = %v, want ErrNotFound", err)
	}

	pool.CleanupWorkItem(1)
	pool.Close()

	if input, output := pool.SessionTokenUsage(nil); input != 0 || output != 0 {
		t.Fatalf("SessionTokenUsage(nil) = (%d,%d), want (0,0)", input, output)
	}

	if lastUsed, turns, input, output := (*pooledACPSession)(nil).statsSnapshot(); !lastUsed.IsZero() || turns != 0 || input != 0 || output != 0 {
		t.Fatalf("statsSnapshot(nil) = (%v,%d,%d,%d)", lastUsed, turns, input, output)
	}
}
