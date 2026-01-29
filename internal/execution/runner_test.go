package execution

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/zhanggui/internal/scheduler"
)

func TestRunMPUs_RespectsGlobalCap(t *testing.T) {
	lim, err := scheduler.NewLimiter(scheduler.Caps{GlobalMax: 1})
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}

	var inFlight int32
	var maxSeen int32

	mpus := []MPU{{TeamID: "team_a", Role: "writer"}, {TeamID: "team_a", Role: "writer"}, {TeamID: "team_a", Role: "writer"}}
	err = RunMPUs(context.Background(), lim, mpus, func(ctx context.Context, m MPU) error {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxSeen)
			if n <= old || atomic.CompareAndSwapInt32(&maxSeen, old, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	})
	if err != nil {
		t.Fatalf("RunMPUs: %v", err)
	}
	if maxSeen != 1 {
		t.Fatalf("expected maxSeen=1, got %d", maxSeen)
	}
}

func TestRunMPUs_RespectsRoleCap(t *testing.T) {
	lim, err := scheduler.NewLimiter(scheduler.Caps{GlobalMax: 3, PerRole: map[string]int{"writer": 1}})
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}

	var inFlight int32
	var maxSeen int32

	mpus := []MPU{{Role: "writer"}, {Role: "writer"}, {Role: "writer"}}
	err = RunMPUs(context.Background(), lim, mpus, func(ctx context.Context, m MPU) error {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxSeen)
			if n <= old || atomic.CompareAndSwapInt32(&maxSeen, old, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	})
	if err != nil {
		t.Fatalf("RunMPUs: %v", err)
	}
	if maxSeen != 1 {
		t.Fatalf("expected maxSeen=1, got %d", maxSeen)
	}
}

func TestRunMPUs_RespectsTeamCap(t *testing.T) {
	lim, err := scheduler.NewLimiter(scheduler.Caps{GlobalMax: 3, PerTeam: map[string]int{"team_a": 1}})
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}

	var inFlight int32
	var maxSeen int32

	mpus := []MPU{{TeamID: "team_a"}, {TeamID: "team_a"}, {TeamID: "team_a"}}
	err = RunMPUs(context.Background(), lim, mpus, func(ctx context.Context, m MPU) error {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxSeen)
			if n <= old || atomic.CompareAndSwapInt32(&maxSeen, old, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	})
	if err != nil {
		t.Fatalf("RunMPUs: %v", err)
	}
	if maxSeen != 1 {
		t.Fatalf("expected maxSeen=1, got %d", maxSeen)
	}
}

func TestRunMPUs_CancelsOnFirstError(t *testing.T) {
	lim, err := scheduler.NewLimiter(scheduler.Caps{GlobalMax: 1})
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}

	mpus := []MPU{{TeamID: "team_a"}, {TeamID: "team_a"}, {TeamID: "team_a"}}
	var calls int32
	gotErr := RunMPUs(context.Background(), lim, mpus, func(ctx context.Context, m MPU) error {
		if atomic.AddInt32(&calls, 1) == 1 {
			return fmt.Errorf("boom")
		}
		t.Fatalf("expected later MPUs to be canceled before fn runs")
		return nil
	})
	if gotErr == nil || gotErr.Error() != "boom" {
		t.Fatalf("expected error boom, got %v", gotErr)
	}
	if calls != 1 {
		t.Fatalf("expected calls=1, got %d", calls)
	}
}
