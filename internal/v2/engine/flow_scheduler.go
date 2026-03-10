package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

// FlowScheduler manages a queue of Flows and limits concurrent execution.
// API callers submit Flows via Submit(); the scheduler runs them when capacity
// is available.
type FlowScheduler struct {
	engine *FlowEngine
	store  core.Store
	bus    core.EventBus

	maxConcurrent int // max flows running in parallel

	mu      sync.Mutex
	queue   []int64                      // flow IDs waiting to run
	running map[int64]context.CancelFunc // flow ID → cancel func
	closed  bool

	// notify is signalled when a flow finishes or a new flow is submitted.
	notify chan struct{}
	done   chan struct{} // closed when scheduler loop exits
}

// FlowSchedulerConfig configures the FlowScheduler.
type FlowSchedulerConfig struct {
	MaxConcurrentFlows int // default 2
}

// NewFlowScheduler creates a multi-flow scheduler.
func NewFlowScheduler(engine *FlowEngine, store core.Store, bus core.EventBus, cfg FlowSchedulerConfig) *FlowScheduler {
	if cfg.MaxConcurrentFlows <= 0 {
		cfg.MaxConcurrentFlows = 2
	}
	return &FlowScheduler{
		engine:        engine,
		store:         store,
		bus:           bus,
		maxConcurrent: cfg.MaxConcurrentFlows,
		running:       make(map[int64]context.CancelFunc),
		notify:        make(chan struct{}, 1),
		done:          make(chan struct{}),
	}
}

// Start begins the scheduler loop. It blocks until ctx is cancelled.
func (s *FlowScheduler) Start(ctx context.Context) {
	defer close(s.done)

	for {
		s.dispatch(ctx)

		select {
		case <-ctx.Done():
			s.drainRunning()
			return
		case <-s.notify:
			// new submission or a flow finished — re-check
		}
	}
}

// Submit enqueues a flow for execution. The flow must be in pending state.
// It transitions the flow to queued and returns immediately.
func (s *FlowScheduler) Submit(ctx context.Context, flowID int64) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("scheduler is closed")
	}
	s.mu.Unlock()

	// Validate flow state.
	flow, err := s.store.GetFlow(ctx, flowID)
	if err != nil {
		return err
	}
	if flow.Status != core.FlowPending {
		return fmt.Errorf("flow %d is %s, expected pending", flowID, flow.Status)
	}

	// Transition to queued.
	if err := s.store.UpdateFlowStatus(ctx, flowID, core.FlowQueued); err != nil {
		return fmt.Errorf("queue flow %d: %w", flowID, err)
	}
	s.bus.Publish(ctx, core.Event{
		Type:      core.EventFlowQueued,
		FlowID:    flowID,
		Timestamp: time.Now().UTC(),
	})

	s.mu.Lock()
	s.queue = append(s.queue, flowID)
	s.mu.Unlock()

	s.signal()
	return nil
}

// Cancel cancels a flow. If queued, removes from queue. If running, cancels its context.
func (s *FlowScheduler) Cancel(ctx context.Context, flowID int64) error {
	s.mu.Lock()

	// Check if in queue — remove it.
	for i, id := range s.queue {
		if id == flowID {
			s.queue = append(s.queue[:i], s.queue[i+1:]...)
			s.mu.Unlock()
			// Update state to cancelled.
			if err := s.store.UpdateFlowStatus(ctx, flowID, core.FlowCancelled); err != nil {
				return err
			}
			s.bus.Publish(ctx, core.Event{
				Type:      core.EventFlowCancelled,
				FlowID:    flowID,
				Timestamp: time.Now().UTC(),
			})
			return nil
		}
	}

	// Check if running — cancel its context.
	cancel, ok := s.running[flowID]
	s.mu.Unlock()

	if ok {
		cancel()
		// The engine.Run goroutine will handle state transition to cancelled/failed.
		return nil
	}

	// Fallback: delegate to engine's Cancel for direct state update.
	return s.engine.Cancel(ctx, flowID)
}

// QueueLen returns the number of flows waiting to run.
func (s *FlowScheduler) QueueLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

// RunningCount returns the number of currently running flows.
func (s *FlowScheduler) RunningCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.running)
}

// Stats returns scheduler statistics.
func (s *FlowScheduler) Stats() SchedulerStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	runningIDs := make([]int64, 0, len(s.running))
	for id := range s.running {
		runningIDs = append(runningIDs, id)
	}
	queuedIDs := make([]int64, len(s.queue))
	copy(queuedIDs, s.queue)

	return SchedulerStats{
		MaxConcurrent: s.maxConcurrent,
		RunningCount:  len(s.running),
		QueuedCount:   len(s.queue),
		RunningIDs:    runningIDs,
		QueuedIDs:     queuedIDs,
	}
}

// SchedulerStats holds runtime stats for the scheduler.
type SchedulerStats struct {
	MaxConcurrent int     `json:"max_concurrent"`
	RunningCount  int     `json:"running_count"`
	QueuedCount   int     `json:"queued_count"`
	RunningIDs    []int64 `json:"running_ids"`
	QueuedIDs     []int64 `json:"queued_ids"`
}

// Shutdown gracefully stops the scheduler and waits for it to finish.
func (s *FlowScheduler) Shutdown() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	// The caller should cancel the context passed to Start().
	<-s.done
}

// dispatch starts as many queued flows as capacity allows.
func (s *FlowScheduler) dispatch(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for len(s.queue) > 0 && len(s.running) < s.maxConcurrent {
		flowID := s.queue[0]
		s.queue = s.queue[1:]

		flowCtx, cancel := context.WithCancel(ctx)
		s.running[flowID] = cancel

		go s.runFlow(flowCtx, flowID)
	}
}

// runFlow executes a single flow and cleans up when done.
func (s *FlowScheduler) runFlow(ctx context.Context, flowID int64) {
	defer func() {
		s.mu.Lock()
		delete(s.running, flowID)
		s.mu.Unlock()
		s.signal()
	}()

	err := s.engine.Run(ctx, flowID)
	if err != nil {
		// If context was cancelled, mark as cancelled (not failed).
		if ctx.Err() != nil {
			_ = s.store.UpdateFlowStatus(context.Background(), flowID, core.FlowCancelled)
			s.bus.Publish(context.Background(), core.Event{
				Type:      core.EventFlowCancelled,
				FlowID:    flowID,
				Timestamp: time.Now().UTC(),
			})
		}
		slog.Error("flow execution failed", "flow_id", flowID, "error", err)
	}
}

// signal pokes the scheduler loop to re-check capacity.
func (s *FlowScheduler) signal() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// drainRunning cancels all running flows and waits for them to finish.
func (s *FlowScheduler) drainRunning() {
	s.mu.Lock()
	for _, cancel := range s.running {
		cancel()
	}
	s.mu.Unlock()

	// Wait for all goroutines to exit.
	for {
		s.mu.Lock()
		n := len(s.running)
		s.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
