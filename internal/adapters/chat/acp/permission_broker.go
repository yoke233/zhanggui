package acp

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
)

// permissionDecision is the result of a user's permission choice.
type permissionDecision struct {
	OptionID string
	Cancel   bool
}

type pendingPermission struct {
	ch chan permissionDecision
}

// permissionBroker mediates between the ACP permission request (blocking) and
// the frontend WebSocket response (async).  Submit blocks until the frontend
// responds or a timeout fires.
type permissionBroker struct {
	mu      sync.Mutex
	pending map[string]*pendingPermission
	seq     atomic.Int64
}

func newPermissionBroker() *permissionBroker {
	return &permissionBroker{
		pending: make(map[string]*pendingPermission),
	}
}

// NextID returns a monotonically increasing permission request ID.
func (b *permissionBroker) NextID() string {
	return fmt.Sprintf("perm-%d", b.seq.Add(1))
}

// Submit registers a pending permission request, blocks until resolved via
// Resolve or until the timeout/context expires. On timeout, the request is
// rejected by default.
func (b *permissionBroker) Submit(ctx context.Context, id string, _ acpproto.RequestPermissionRequest, timeout time.Duration) (acpproto.RequestPermissionResponse, error) {
	ch := make(chan permissionDecision, 1)
	b.mu.Lock()
	b.pending[id] = &pendingPermission{ch: ch}
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case d := <-ch:
		if d.Cancel {
			return cancelledResponse(), nil
		}
		return selectedResponse(d.OptionID), nil
	case <-timer.C:
		return cancelledResponse(), nil
	case <-ctx.Done():
		return cancelledResponse(), ctx.Err()
	}
}

// Resolve unblocks a pending permission request with the user's decision.
// Returns false if the request ID is not found (already timed out or resolved).
func (b *permissionBroker) Resolve(id string, optionID string, cancel bool) bool {
	b.mu.Lock()
	p, ok := b.pending[id]
	b.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case p.ch <- permissionDecision{OptionID: optionID, Cancel: cancel}:
		return true
	default:
		return false
	}
}

func cancelledResponse() acpproto.RequestPermissionResponse {
	return acpproto.RequestPermissionResponse{
		Outcome: acpproto.RequestPermissionOutcome{
			Cancelled: &acpproto.RequestPermissionOutcomeCancelled{Outcome: "cancelled"},
		},
	}
}

func selectedResponse(optionID string) acpproto.RequestPermissionResponse {
	return acpproto.RequestPermissionResponse{
		Outcome: acpproto.RequestPermissionOutcome{
			Selected: &acpproto.RequestPermissionOutcomeSelected{
				Outcome:  "selected",
				OptionId: acpproto.PermissionOptionId(optionID),
			},
		},
	}
}
