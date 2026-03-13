package agentruntime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestACPSessionPoolAcquireCoalescesConcurrentCreates(t *testing.T) {
	t.Parallel()

	var createCalls atomic.Int32
	ready := make(chan struct{})
	release := make(chan struct{})
	sess := &pooledACPSession{}
	ac := &core.AgentContext{ID: 42}

	pool := &ACPSessionPool{
		sessions: make(map[acpSessionKey]*pooledACPSession),
		inflight: make(map[acpSessionKey]*acpSessionFlight),
		createSessionFn: func(context.Context, acpSessionKey, acpSessionAcquireInput) (*pooledACPSession, *core.AgentContext, error) {
			createCalls.Add(1)
			close(ready)
			<-release
			return sess, ac, nil
		},
	}

	input := acpSessionAcquireInput{
		Profile: &core.AgentProfile{ID: "worker"},
		Driver:  &core.AgentDriver{ID: "codex"},
		IssueID: 101,
	}

	var wg sync.WaitGroup
	type acquireResult struct {
		sess *pooledACPSession
		ac   *core.AgentContext
		err  error
	}
	results := make(chan acquireResult, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gotSess, gotAC, err := pool.Acquire(context.Background(), input)
			results <- acquireResult{sess: gotSess, ac: gotAC, err: err}
		}()
	}

	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session creation to start")
	}
	close(release)
	wg.Wait()
	close(results)

	if createCalls.Load() != 1 {
		t.Fatalf("createSessionFn call count = %d, want 1", createCalls.Load())
	}

	for result := range results {
		if result.err != nil {
			t.Fatalf("Acquire() error = %v", result.err)
		}
		if result.sess != sess {
			t.Fatalf("Acquire() session = %p, want %p", result.sess, sess)
		}
		if result.ac != nil && result.ac != ac {
			t.Fatalf("Acquire() agent context = %p, want %p or nil", result.ac, ac)
		}
	}
}
