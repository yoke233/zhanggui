package probe

import (
	"context"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Service is the minimal application contract required by transport adapters.
type Service interface {
	ListRunProbes(ctx context.Context, runID int64) ([]*core.RunProbe, error)
	GetLatestRunProbe(ctx context.Context, runID int64) (*core.RunProbe, error)
	RequestRunProbe(ctx context.Context, runID int64, source core.RunProbeTriggerSource, question string, timeout time.Duration) (*core.RunProbe, error)
}
