package flow

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// IssueScheduler manages a queue of Issues and limits concurrent execution.
// API callers submit Issues via Submit(); the scheduler runs them when capacity
// is available.
type IssueScheduler struct {
	engine *IssueEngine
	store  Store
	bus    EventPublisher

	maxConcurrent int // max issues running in parallel

	mu      sync.Mutex
	queue   []int64                      // issue IDs waiting to run
	running map[int64]context.CancelFunc // issue ID → cancel func
	closed  bool

	// notify is signalled when an issue finishes or a new issue is submitted.
	notify chan struct{}
	done   chan struct{} // closed when scheduler loop exits
}

// IssueSchedulerConfig configures the IssueScheduler.
type IssueSchedulerConfig struct {
	MaxConcurrentIssues int // default 2
	MaxConcurrentFlows  int // deprecated compatibility field
}

// NewIssueScheduler creates a multi-issue scheduler.
func NewIssueScheduler(engine *IssueEngine, store Store, bus EventPublisher, cfg IssueSchedulerConfig) *IssueScheduler {
	if cfg.MaxConcurrentIssues <= 0 && cfg.MaxConcurrentFlows > 0 {
		cfg.MaxConcurrentIssues = cfg.MaxConcurrentFlows
	}
	if cfg.MaxConcurrentIssues <= 0 {
		cfg.MaxConcurrentIssues = 2
	}
	return &IssueScheduler{
		engine:        engine,
		store:         store,
		bus:           bus,
		maxConcurrent: cfg.MaxConcurrentIssues,
		running:       make(map[int64]context.CancelFunc),
		notify:        make(chan struct{}, 1),
		done:          make(chan struct{}),
	}
}

// FlowSchedulerConfig is a compatibility wrapper for older callers.
type FlowSchedulerConfig struct {
	MaxConcurrentIssues int
	MaxConcurrentFlows  int
}

// NewFlowScheduler is an alias for backward compatibility.
func NewFlowScheduler(engine *IssueEngine, store Store, bus EventPublisher, cfg FlowSchedulerConfig) *IssueScheduler {
	return NewIssueScheduler(engine, store, bus, IssueSchedulerConfig{
		MaxConcurrentIssues: cfg.MaxConcurrentIssues,
		MaxConcurrentFlows:  cfg.MaxConcurrentFlows,
	})
}

// Start begins the scheduler loop. It blocks until ctx is cancelled.
func (s *IssueScheduler) Start(ctx context.Context) {
	defer close(s.done)

	for {
		s.dispatch(ctx)

		select {
		case <-ctx.Done():
			s.drainRunning()
			return
		case <-s.notify:
			// new submission or an issue finished — re-check
		}
	}
}

// Submit enqueues an issue for execution. The issue must be in open/accepted state.
// It transitions the issue to queued and returns immediately.
func (s *IssueScheduler) Submit(ctx context.Context, issueID int64) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("scheduler is closed")
	}
	s.mu.Unlock()

	// Atomically transition open/accepted, unarchived issues to queued.
	if err := s.store.PrepareIssueRun(ctx, issueID, core.IssueQueued); err != nil {
		return fmt.Errorf("queue issue %d: %w", issueID, err)
	}
	s.bus.Publish(ctx, core.Event{
		Type:      core.EventIssueQueued,
		IssueID:   issueID,
		Timestamp: time.Now().UTC(),
	})

	s.mu.Lock()
	s.queue = append(s.queue, issueID)
	s.mu.Unlock()

	s.signal()
	return nil
}

// Cancel cancels an issue. If queued, removes from queue. If running, cancels its context.
func (s *IssueScheduler) Cancel(ctx context.Context, issueID int64) error {
	s.mu.Lock()

	// Check if in queue — remove it.
	for i, id := range s.queue {
		if id == issueID {
			s.queue = append(s.queue[:i], s.queue[i+1:]...)
			s.mu.Unlock()
			// Update state to cancelled.
			if err := s.store.UpdateIssueStatus(ctx, issueID, core.IssueCancelled); err != nil {
				return err
			}
			s.bus.Publish(ctx, core.Event{
				Type:      core.EventIssueCancelled,
				IssueID:   issueID,
				Timestamp: time.Now().UTC(),
			})
			return nil
		}
	}

	// Check if running — cancel its context.
	cancel, ok := s.running[issueID]
	s.mu.Unlock()

	if ok {
		cancel()
		// The engine.Run goroutine will handle state transition to cancelled/failed.
		return nil
	}

	// Fallback: delegate to engine's Cancel for direct state update.
	return s.engine.Cancel(ctx, issueID)
}

// QueueLen returns the number of issues waiting to run.
func (s *IssueScheduler) QueueLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

// RunningCount returns the number of currently running issues.
func (s *IssueScheduler) RunningCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.running)
}

// Stats returns scheduler statistics.
func (s *IssueScheduler) Stats() SchedulerStats {
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
func (s *IssueScheduler) Shutdown() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	// The caller should cancel the context passed to Start().
	<-s.done
}

// dispatch starts as many queued issues as capacity allows.
func (s *IssueScheduler) dispatch(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for len(s.queue) > 0 && len(s.running) < s.maxConcurrent {
		issueID := s.queue[0]
		s.queue = s.queue[1:]

		issueCtx, cancel := context.WithCancel(ctx)
		s.running[issueID] = cancel

		go s.runIssue(issueCtx, issueID)
	}
}

// runIssue executes a single issue and cleans up when done.
func (s *IssueScheduler) runIssue(ctx context.Context, issueID int64) {
	defer func() {
		s.mu.Lock()
		delete(s.running, issueID)
		s.mu.Unlock()
		s.signal()
	}()

	err := s.engine.Run(ctx, issueID)
	if err != nil {
		// If context was cancelled, mark as cancelled (not failed).
		if ctx.Err() != nil {
			_ = s.store.UpdateIssueStatus(context.Background(), issueID, core.IssueCancelled)
			s.bus.Publish(context.Background(), core.Event{
				Type:      core.EventIssueCancelled,
				IssueID:   issueID,
				Timestamp: time.Now().UTC(),
			})
		}
		slog.Error("issue execution failed", "issue_id", issueID, "error", err)
	}
}

// signal pokes the scheduler loop to re-check capacity.
func (s *IssueScheduler) signal() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// drainRunning cancels all running issues and waits for them to finish.
func (s *IssueScheduler) drainRunning() {
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
