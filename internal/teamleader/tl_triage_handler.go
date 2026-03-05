package teamleader

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

const defaultMergeConflictMaxRetries = 3

// TLTriageHandler handles merge conflict events and decides retry vs fail.
type TLTriageHandler struct {
	store      core.Store
	bus        core.EventBus
	maxRetries int
	log        *slog.Logger
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewTLTriageHandler(store core.Store, bus core.EventBus, maxRetries int) *TLTriageHandler {
	if maxRetries <= 0 {
		maxRetries = defaultMergeConflictMaxRetries
	}
	return &TLTriageHandler{
		store:      store,
		bus:        bus,
		maxRetries: maxRetries,
		log:        slog.Default(),
	}
}

// Start subscribes to the event bus and processes triage events in a goroutine.
func (h *TLTriageHandler) Start(ctx context.Context) error {
	sub, err := h.bus.Subscribe(
		core.WithName("tl-triage"),
		core.WithTypes(core.EventIssueMergeConflict),
	)
	if err != nil {
		return fmt.Errorf("tl-triage subscribe: %w", err)
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
func (h *TLTriageHandler) Stop(_ context.Context) error {
	if h.cancel != nil {
		h.cancel()
	}
	h.wg.Wait()
	return nil
}

func (h *TLTriageHandler) OnEvent(_ context.Context, evt core.Event) {
	if h == nil || h.store == nil {
		return
	}
	if evt.Type != core.EventIssueMergeConflict {
		return
	}
	issueID := strings.TrimSpace(evt.IssueID)
	if issueID == "" {
		return
	}

	issue, err := h.store.GetIssue(issueID)
	if err != nil || issue == nil {
		h.log.Warn("tl_triage: issue not found", "issue_id", issueID, "error", err)
		return
	}
	if issue.Status != core.IssueStatusMerging {
		return
	}

	nextRetries := issue.MergeRetries + 1
	oldRunID := strings.TrimSpace(issue.RunID)
	if nextRetries >= h.maxRetries {
		issue.MergeRetries = nextRetries
		if err := transitionIssueStatus(issue, core.IssueStatusFailed); err != nil {
			h.log.Error("tl_triage: invalid transition to failed", "issue_id", issue.ID, "error", err)
			return
		}
		if err := h.store.SaveIssue(issue); err != nil {
			h.log.Error("tl_triage: save failed issue", "issue_id", issue.ID, "error", err)
			return
		}
		h.publish(core.Event{
			Type:      core.EventIssueFailed,
			RunID:     oldRunID,
			ProjectID: issue.ProjectID,
			IssueID:   issue.ID,
			Data: map[string]string{
				"action":        "retry_exhausted",
				"merge_retries": strconv.Itoa(issue.MergeRetries),
			},
			Error:     "merge conflict retries exhausted",
			Timestamp: time.Now(),
		})
		return
	}

	issue.MergeRetries = nextRetries
	if err := transitionIssueStatus(issue, core.IssueStatusQueued); err != nil {
		h.log.Error("tl_triage: invalid transition to queued", "issue_id", issue.ID, "error", err)
		return
	}
	issue.RunID = ""
	if err := h.store.SaveIssue(issue); err != nil {
		h.log.Error("tl_triage: save retry issue", "issue_id", issue.ID, "error", err)
		return
	}
	h.publish(core.Event{
		Type:      core.EventIssueMergeRetry,
		RunID:     oldRunID,
		ProjectID: issue.ProjectID,
		IssueID:   issue.ID,
		Data: map[string]string{
			"action":        "retry_requeue",
			"merge_retries": strconv.Itoa(issue.MergeRetries),
		},
		Timestamp: time.Now(),
	})
}

func (h *TLTriageHandler) publish(evt core.Event) {
	if h == nil || h.bus == nil {
		return
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}
	h.bus.Publish(context.Background(), evt)
}
