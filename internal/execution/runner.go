package execution

import (
	"context"
	"fmt"
	"sync"

	"github.com/yoke233/zhanggui/internal/scheduler"
)

func RunMPUs(ctx context.Context, limiter *scheduler.Limiter, mpus []MPU, fn func(ctx context.Context, m MPU) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if limiter == nil {
		return fmt.Errorf("limiter missing")
	}
	if fn == nil {
		return fmt.Errorf("fn missing")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error

	setErr := func(err error) {
		if err == nil {
			return
		}
		once.Do(func() {
			firstErr = err
			cancel()
		})
	}

	for _, m := range mpus {
		m := m
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := runCtx.Err(); err != nil {
				setErr(err)
				return
			}

			lease, err := limiter.Acquire(runCtx, scheduler.Key{TeamID: m.TeamID, Role: m.Role})
			if err != nil {
				setErr(err)
				return
			}
			defer func() {
				if err := lease.Release(); err != nil {
					setErr(err)
				}
			}()

			if err := runCtx.Err(); err != nil {
				setErr(err)
				return
			}

			if err := fn(runCtx, m); err != nil {
				setErr(err)
				return
			}
		}()
	}

	wg.Wait()
	return firstErr
}
