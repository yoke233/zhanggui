package execution

import (
	"context"
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
