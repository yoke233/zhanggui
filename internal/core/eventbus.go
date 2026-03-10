package core

import "context"

// EventBus is the core event publish-subscribe abstraction.
// The in-memory implementation uses channel fan-out; future implementations
// may use NATS or other message brokers.
type EventBus interface {
	// Publish sends an event to all matching subscribers.
	Publish(ctx context.Context, evt Event) error

	// Subscribe creates a subscription and returns it.
	// Use SubOption to control filtering, buffer size, and name.
	Subscribe(opts ...SubOption) (*Subscription, error)

	// Close shuts down the bus; all subscriptions are automatically cancelled.
	Close() error
}

// Subscription represents an active event subscription.
type Subscription struct {
	C        <-chan Event // consumer reads events from this channel
	CancelFn func()       // called by Unsubscribe to release resources
}

// Unsubscribe cancels the subscription and releases resources.
func (s *Subscription) Unsubscribe() {
	if s != nil && s.CancelFn != nil {
		s.CancelFn()
	}
}

// SubOption configures subscription behavior.
type SubOption func(*SubOptions)

// SubOptions holds resolved subscription configuration.
type SubOptions struct {
	Name       string      // subscriber name (for logging/metrics)
	Types      []EventType // filter: only receive these event types (empty = all)
	BufferSize int         // channel buffer size (default 64)
}

// WithName sets the subscriber name for logging and metrics.
func WithName(name string) SubOption {
	return func(o *SubOptions) { o.Name = name }
}

// WithTypes filters events so only the specified types are delivered.
func WithTypes(types ...EventType) SubOption {
	return func(o *SubOptions) { o.Types = types }
}

// WithBufferSize sets the channel buffer size for this subscription.
func WithBufferSize(n int) SubOption {
	return func(o *SubOptions) { o.BufferSize = n }
}
