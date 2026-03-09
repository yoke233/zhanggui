package teamleader

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// ChildCompletionHandler listens for EventIssueDone / EventIssueFailed and
// checks whether all siblings of the completed child are finished.  When every
// child is done the parent issue is closed automatically.
type ChildCompletionHandler struct {
	store  core.Store
	bus    core.EventBus
	pub    eventPublisher
	log    *slog.Logger
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewChildCompletionHandler creates a handler that tracks child issue completion.
func NewChildCompletionHandler(store core.Store, bus core.EventBus) *ChildCompletionHandler {
	return &ChildCompletionHandler{
		store: store,
		bus:   bus,
		pub:   bus,
		log:   slog.Default(),
	}
}

// Start subscribes to the event bus and processes completion events in a goroutine.
func (h *ChildCompletionHandler) Start(ctx context.Context) error {
	sub, err := h.bus.Subscribe(
		core.WithName("child-completion"),
		core.WithTypes(core.EventIssueDone, core.EventIssueFailed),
	)
	if err != nil {
		return fmt.Errorf("child-completion subscribe: %w", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	h.cancel = cancel
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		defer sub.Unsubscribe()
		for {
			select {
			case <-runCtx.Done():
				return
			case evt, ok := <-sub.C:
				if !ok {
					return
				}
				h.OnEvent(runCtx, evt)
			}
		}
	}()
	return nil
}

// Stop cancels the subscription goroutine and waits for it to exit.
func (h *ChildCompletionHandler) Stop(_ context.Context) error {
	if h.cancel != nil {
		h.cancel()
	}
	h.wg.Wait()
	return nil
}

// OnEvent handles a single event.  Reacts to EventIssueDone and EventIssueFailed.
func (h *ChildCompletionHandler) OnEvent(ctx context.Context, evt core.Event) {
	if evt.Type != core.EventIssueDone && evt.Type != core.EventIssueFailed {
		return
	}
	issueID := strings.TrimSpace(evt.IssueID)
	if issueID == "" {
		return
	}

	child, err := h.store.GetIssue(issueID)
	if err != nil || child == nil {
		return
	}
	if child.ParentID == "" {
		return // not a child issue
	}

	parent, err := h.store.GetIssue(child.ParentID)
	if err != nil || parent == nil {
		h.log.Warn("child_completion: parent not found", "parent_id", child.ParentID, "error", err)
		return
	}
	if parent.Status != core.IssueStatusDecomposed {
		return // parent not in decomposed state
	}

	siblings, err := h.store.GetChildIssues(parent.ID)
	if err != nil {
		h.log.Error("child_completion: get children failed", "parent_id", parent.ID, "error", err)
		return
	}

	var allDone, anyFailed bool
	allDone = true
	for _, s := range siblings {
		switch s.Status {
		case core.IssueStatusDone:
			// ok
		case core.IssueStatusFailed:
			anyFailed = true
		default:
			allDone = false
		}
	}

	if !allDone {
		return
	}

	if anyFailed {
		h.resolveParentWithFailures(parent)
	} else {
		h.resolveParentSuccess(parent)
	}
}

func (h *ChildCompletionHandler) resolveParentSuccess(parent *core.Issue) {
	now := time.Now()
	if err := transitionIssueStatus(parent, core.IssueStatusDone); err != nil {
		h.log.Error("child_completion: invalid parent transition to done", "parent_id", parent.ID, "error", err)
		return
	}
	parent.State = core.IssueStateClosed
	parent.ClosedAt = &now
	if err := h.store.SaveIssue(parent); err != nil {
		h.log.Error("child_completion: save parent done", "parent_id", parent.ID, "error", err)
		return
	}
	h.recordTaskStep(parent, core.StepCompleted, "system", "all children done")
	h.pub.Publish(context.Background(), core.Event{
		Type:      core.EventIssueDone,
		IssueID:   parent.ID,
		ProjectID: parent.ProjectID,
		Timestamp: now,
	})
	h.log.Info("child_completion: parent done", "parent_id", parent.ID)
}

func (h *ChildCompletionHandler) resolveParentWithFailures(parent *core.Issue) {
	switch parent.FailPolicy {
	case core.FailSkip:
		// Treat as success if non-failed children are all done.
		h.resolveParentSuccess(parent)
	case core.FailHuman:
		h.pub.Publish(context.Background(), core.Event{
			Type:      core.EventIssueFailed,
			IssueID:   parent.ID,
			ProjectID: parent.ProjectID,
			Error:     "child issues failed, human review required",
			Timestamp: time.Now(),
		})
	default: // FailBlock
		if err := transitionIssueStatus(parent, core.IssueStatusFailed); err != nil {
			h.log.Error("child_completion: invalid parent transition to failed", "parent_id", parent.ID, "error", err)
			return
		}
		if err := h.store.SaveIssue(parent); err != nil {
			h.log.Error("child_completion: save parent failed", "parent_id", parent.ID, "error", err)
			return
		}
		h.recordTaskStep(parent, core.StepFailed, "system", "child failed (block policy)")
		h.pub.Publish(context.Background(), core.Event{
			Type:      core.EventIssueFailed,
			IssueID:   parent.ID,
			ProjectID: parent.ProjectID,
			Error:     "one or more child issues failed",
			Timestamp: time.Now(),
		})
		h.log.Info("child_completion: parent failed", "parent_id", parent.ID)
	}
}

func (h *ChildCompletionHandler) recordTaskStep(issue *core.Issue, action core.TaskStepAction, agentID, note string) {
	if h == nil || h.store == nil || issue == nil || strings.TrimSpace(issue.ID) == "" {
		return
	}
	if _, err := h.store.SaveTaskStep(&core.TaskStep{
		ID:        core.NewTaskStepID(),
		IssueID:   strings.TrimSpace(issue.ID),
		RunID:     strings.TrimSpace(issue.RunID),
		Action:    action,
		AgentID:   strings.TrimSpace(agentID),
		Note:      strings.TrimSpace(note),
		CreatedAt: time.Now(),
	}); err != nil {
		h.log.Warn("failed to save task step", "error", err, "issue", issue.ID, "action", action)
	}
}
