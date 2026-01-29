package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestCapsValidate(t *testing.T) {
	if err := (Caps{}).Validate(); err == nil {
		t.Fatalf("expected error")
	}
	if err := (Caps{GlobalMax: 1, PerRole: map[string]int{"": 1}}).Validate(); err == nil {
		t.Fatalf("expected error")
	}
	if err := (Caps{GlobalMax: 1, PerRole: map[string]int{"writer": 0}}).Validate(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLimiter_TryAcquire_RespectsCaps(t *testing.T) {
	l, err := NewLimiter(Caps{
		GlobalMax: 2,
		PerRole:   map[string]int{"writer": 1},
	})
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}

	a, ok, err := l.TryAcquire(Key{Role: "writer"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if !ok {
		t.Fatalf("expected acquire ok")
	}

	_, ok, err = l.TryAcquire(Key{Role: "writer"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if ok {
		t.Fatalf("expected acquire blocked by per-role cap")
	}

	if err := a.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	_, ok, err = l.TryAcquire(Key{Role: "writer"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if !ok {
		t.Fatalf("expected acquire ok after release")
	}
}

func TestLimiter_TryAcquire_PerTeamCap(t *testing.T) {
	l, err := NewLimiter(Caps{
		GlobalMax: 10,
		PerTeam:   map[string]int{"team_a": 1},
	})
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}
	a, ok, err := l.TryAcquire(Key{TeamID: "team_a"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok")
	}
	_, ok, err = l.TryAcquire(Key{TeamID: "team_a"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if ok {
		t.Fatalf("expected blocked by per-team cap")
	}
	_ = a.Release()
}

func TestLimiter_GlobalCap_BlockingAcquire(t *testing.T) {
	l, err := NewLimiter(Caps{GlobalMax: 3})
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	acquired := make(chan struct{}, 10)
	release := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lease, err := l.Acquire(ctx, Key{TeamID: "team_a"})
			if err != nil {
				return
			}
			acquired <- struct{}{}
			<-release
			_ = lease.Release()
		}()
	}

	// 允许最多 3 个并行持有 lease。
	for i := 0; i < 3; i++ {
		select {
		case <-acquired:
		case <-ctx.Done():
			t.Fatalf("timeout waiting for initial acquisitions: %v", ctx.Err())
		}
	}

	s := l.Snapshot()
	if s.InUseGlobal != 3 {
		t.Fatalf("expected InUseGlobal=3, got %d", s.InUseGlobal)
	}

	// 如果超过 3，会在 acquired channel 里出现额外事件（无需等待/超时）。
	select {
	case <-acquired:
		t.Fatalf("expected no extra acquisitions beyond cap")
	default:
	}

	close(release)
	wg.Wait()
}

func TestLimiter_Acquire_ContextCancel(t *testing.T) {
	l, err := NewLimiter(Caps{GlobalMax: 1})
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}
	hold, ok, err := l.TryAcquire(Key{TeamID: "team_a"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := l.Acquire(ctx, Key{TeamID: "team_a"})
		done <- err
	}()
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	_ = hold.Release()
}

func TestLimiter_UpdateCaps_UnblocksAcquire(t *testing.T) {
	l, err := NewLimiter(Caps{GlobalMax: 1})
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}
	hold, ok, err := l.TryAcquire(Key{TeamID: "team_a"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	acquired := make(chan struct{}, 1)
	go func() {
		lease, err := l.Acquire(ctx, Key{TeamID: "team_a"})
		if err != nil {
			return
		}
		_ = lease.Release()
		acquired <- struct{}{}
	}()

	// 提高上限后，等待中的 Acquire 应该能被唤醒并成功拿到 lease。
	if err := l.UpdateCaps(Caps{GlobalMax: 2}); err != nil {
		t.Fatalf("UpdateCaps: %v", err)
	}

	select {
	case <-acquired:
	case <-ctx.Done():
		t.Fatalf("timeout waiting for acquire to unblock: %v", ctx.Err())
	}

	_ = hold.Release()
}

func TestLimiter_UpdateCaps_DecreaseAndIncrease(t *testing.T) {
	l, err := NewLimiter(Caps{GlobalMax: 2, PerRole: map[string]int{"writer": 2}})
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}

	a1, ok, err := l.TryAcquire(Key{Role: "writer"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if !ok {
		t.Fatalf("expected acquire ok")
	}
	a2, ok, err := l.TryAcquire(Key{Role: "writer"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if !ok {
		t.Fatalf("expected acquire ok")
	}

	// 降低全局并行上限：已有 lease 不回收，但新增应被阻塞。
	if err := l.UpdateCaps(Caps{GlobalMax: 1, PerRole: map[string]int{"writer": 1}}); err != nil {
		t.Fatalf("UpdateCaps: %v", err)
	}
	_, ok, err = l.TryAcquire(Key{Role: "writer"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if ok {
		t.Fatalf("expected acquire blocked after decreasing caps")
	}

	if err := a1.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	_, ok, err = l.TryAcquire(Key{Role: "writer"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if ok {
		t.Fatalf("expected still blocked (inUse=1, globalMax=1)")
	}

	if err := a2.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	_, ok, err = l.TryAcquire(Key{Role: "writer"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if !ok {
		t.Fatalf("expected acquire ok after releasing to <= cap")
	}

	// 再提高上限，应能并行更多。
	if err := l.UpdateCaps(Caps{GlobalMax: 3, PerRole: map[string]int{"writer": 3}}); err != nil {
		t.Fatalf("UpdateCaps: %v", err)
	}
	_, ok, err = l.TryAcquire(Key{Role: "writer"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if !ok {
		t.Fatalf("expected acquire ok after increasing caps")
	}
}

func TestLease_DoubleRelease(t *testing.T) {
	l, err := NewLimiter(Caps{GlobalMax: 1})
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}
	lease, ok, err := l.TryAcquire(Key{TeamID: "team_a"})
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok")
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := lease.Release(); err != ErrLeaseReleased {
		t.Fatalf("expected ErrLeaseReleased, got %v", err)
	}
}
