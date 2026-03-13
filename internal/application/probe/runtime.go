package probe

import (
	"context"
	"time"
)

// Runtime sends a diagnostic probe to a running run through the active runtime.
type Runtime interface {
	ProbeRun(ctx context.Context, req RunProbeRuntimeRequest) (*RunProbeRuntimeResult, error)
}

// RunProbeRuntimeRequest contains the routing data needed to send a probe.
type RunProbeRuntimeRequest struct {
	RunID        int64
	InvocationID string
	SessionID    string
	OwnerID      string
	Question     string
	Timeout      time.Duration
}

// RunProbeRuntimeResult is the low-level runtime response for a probe request.
type RunProbeRuntimeResult struct {
	Reachable  bool
	Answered   bool
	ReplyText  string
	Error      string
	ObservedAt time.Time
}
