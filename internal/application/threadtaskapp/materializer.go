package threadtaskapp

import (
	"context"
	"fmt"

	"github.com/yoke233/ai-workflow/internal/core"
)

// MaterializerStore is the persistence contract needed by DefaultMaterializer.
type MaterializerStore interface {
	GetThread(ctx context.Context, id int64) (*core.Thread, error)
	CreateWorkItem(ctx context.Context, w *core.WorkItem) (int64, error)
	CreateThreadWorkItemLink(ctx context.Context, link *core.ThreadWorkItemLink) (int64, error)
}

// DefaultMaterializer creates a WorkItem and links it to the thread.
type DefaultMaterializer struct {
	store MaterializerStore
}

// NewDefaultMaterializer returns a materializer backed by the given store.
func NewDefaultMaterializer(store MaterializerStore) *DefaultMaterializer {
	return &DefaultMaterializer{store: store}
}

func (m *DefaultMaterializer) MaterializeWorkItem(ctx context.Context, input MaterializeInput) (*MaterializeResult, error) {
	thread, err := m.store.GetThread(ctx, input.ThreadID)
	if err != nil {
		return nil, fmt.Errorf("get thread %d: %w", input.ThreadID, err)
	}

	var projectID *int64
	if pid, ok := core.ReadThreadFocusProjectID(thread); ok {
		projectID = &pid
	}

	workItem := &core.WorkItem{
		ProjectID: projectID,
		Title:     input.Title,
		Body:      input.Body,
		Status:    core.WorkItemOpen,
		Priority:  core.PriorityMedium,
		Metadata: map[string]any{
			"source_thread_id":     input.ThreadID,
			"source_task_group_id": input.GroupID,
			"source_type":          "task_group_materialize",
		},
	}

	workItemID, err := m.store.CreateWorkItem(ctx, workItem)
	if err != nil {
		return nil, fmt.Errorf("create work item: %w", err)
	}

	link := &core.ThreadWorkItemLink{
		ThreadID:     input.ThreadID,
		WorkItemID:   workItemID,
		RelationType: "drives",
		IsPrimary:    true,
	}
	linkID, err := m.store.CreateThreadWorkItemLink(ctx, link)
	if err != nil {
		return nil, fmt.Errorf("create thread-workitem link: %w", err)
	}

	return &MaterializeResult{
		WorkItemID: workItemID,
		LinkID:     linkID,
	}, nil
}
