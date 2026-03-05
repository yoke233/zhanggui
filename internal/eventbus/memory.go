package eventbus

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/yoke233/ai-workflow/internal/core"
)

const defaultBufferSize = 64

// MemoryBus implements core.EventBus using in-process channel fan-out.
type MemoryBus struct {
	mu     sync.RWMutex
	subs   []*memSub
	closed atomic.Bool
	log    *slog.Logger
}

type memSub struct {
	name   string
	ch     chan core.Event
	types  map[core.EventType]struct{} // nil = receive all
	closed atomic.Bool
}

var _ core.EventBus = (*MemoryBus)(nil)

// New creates a new in-memory event bus.
func New() *MemoryBus {
	bus := &MemoryBus{
		log: slog.Default(),
	}

	defaultBusMu.Lock()
	if defaultBus == nil {
		defaultBus = bus
	}
	defaultBusMu.Unlock()

	return bus
}

// Publish sends an event to all matching subscribers.
// The in-memory implementation ignores ctx (never blocks on I/O).
func (b *MemoryBus) Publish(_ context.Context, evt core.Event) error {
	if b.closed.Load() {
		return nil
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subs {
		if sub.closed.Load() {
			continue
		}
		if sub.types != nil {
			if _, ok := sub.types[evt.Type]; !ok {
				continue
			}
		}
		select {
		case sub.ch <- evt:
		default:
			b.log.Warn("event dropped",
				"subscriber", sub.name,
				"type", evt.Type,
				"run_id", evt.RunID,
				"issue_id", evt.IssueID,
			)
		}
	}
	return nil
}

// Subscribe creates a new subscription with optional filtering.
func (b *MemoryBus) Subscribe(opts ...core.SubOption) (*core.Subscription, error) {
	options := core.SubOptions{
		BufferSize: defaultBufferSize,
	}
	for _, o := range opts {
		o(&options)
	}
	if options.BufferSize <= 0 {
		options.BufferSize = defaultBufferSize
	}

	var typeFilter map[core.EventType]struct{}
	if len(options.Types) > 0 {
		typeFilter = make(map[core.EventType]struct{}, len(options.Types))
		for _, t := range options.Types {
			typeFilter[t] = struct{}{}
		}
	}

	ch := make(chan core.Event, options.BufferSize)
	sub := &memSub{
		name:  options.Name,
		ch:    ch,
		types: typeFilter,
	}

	b.mu.Lock()
	b.subs = append(b.subs, sub)
	b.mu.Unlock()

	return &core.Subscription{
		C:        ch,
		CancelFn: func() { b.unsubscribe(sub) },
	}, nil
}

// unsubscribe removes a memSub and closes its channel.
func (b *MemoryBus) unsubscribe(target *memSub) {
	if target == nil || target.closed.Load() {
		return
	}
	target.closed.Store(true)

	b.mu.Lock()
	for i, sub := range b.subs {
		if sub == target {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			break
		}
	}
	b.mu.Unlock()

	close(target.ch)
}

// Close shuts down the bus and cancels all subscriptions.
func (b *MemoryBus) Close() error {
	if !b.closed.CompareAndSwap(false, true) {
		return nil
	}

	b.mu.Lock()
	subs := b.subs
	b.subs = nil
	b.mu.Unlock()

	for _, sub := range subs {
		if sub.closed.CompareAndSwap(false, true) {
			close(sub.ch)
		}
	}
	return nil
}

var (
	defaultBusMu sync.RWMutex
	defaultBus   *MemoryBus
)

// Default returns the process-wide default event bus.
func Default() *MemoryBus {
	defaultBusMu.RLock()
	defer defaultBusMu.RUnlock()
	return defaultBus
}

// SetDefault overrides the process-wide default event bus.
func SetDefault(bus *MemoryBus) {
	defaultBusMu.Lock()
	defer defaultBusMu.Unlock()
	defaultBus = bus
}
