package threadtaskapp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/yoke233/ai-workflow/internal/application/planning"
	"github.com/yoke233/ai-workflow/internal/core"
)

// MaterializerStore is the persistence contract needed by DefaultMaterializer.
type MaterializerStore interface {
	GetThread(ctx context.Context, id int64) (*core.Thread, error)
	CreateWorkItem(ctx context.Context, w *core.WorkItem) (int64, error)
	CreateThreadWorkItemLink(ctx context.Context, link *core.ThreadWorkItemLink) (int64, error)
	CreateAction(ctx context.Context, a *core.Action) (int64, error)
	UpdateActionDependsOn(ctx context.Context, id int64, dependsOn []int64) error
}

// DefaultMaterializer creates a WorkItem and links it to the thread.
type DefaultMaterializer struct {
	store  MaterializerStore
	dagGen ActionDAGGenerator // optional
}

// MaterializerOption configures the DefaultMaterializer.
type MaterializerOption func(*DefaultMaterializer)

// WithDAGGenerator injects an LLM-based DAG generator into the materializer.
func WithDAGGenerator(g ActionDAGGenerator) MaterializerOption {
	return func(m *DefaultMaterializer) {
		m.dagGen = g
	}
}

// NewDefaultMaterializer returns a materializer backed by the given store.
func NewDefaultMaterializer(store MaterializerStore, opts ...MaterializerOption) *DefaultMaterializer {
	m := &DefaultMaterializer{store: store}
	for _, opt := range opts {
		opt(m)
	}
	return m
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

	result := &MaterializeResult{
		WorkItemID: workItemID,
		LinkID:     linkID,
	}

	// Materialize Actions via LLM DAG generation if dagGen and file contents are available
	if m.dagGen != nil && len(input.FileContents) > 0 {
		actionIDs, err := m.generateAndMaterializeActions(ctx, workItemID, input.FileContents)
		if err != nil {
			slog.Warn("materialize actions from file contents failed, falling back to work item only",
				"work_item_id", workItemID, "error", err)
		} else {
			result.ActionIDs = actionIDs
		}
	}

	return result, nil
}

// generateAndMaterializeActions calls the LLM to generate a DAG from file contents
// and materializes the resulting actions into the store.
func (m *DefaultMaterializer) generateAndMaterializeActions(ctx context.Context, workItemID int64, fileContents map[string]string) ([]int64, error) {
	dag, err := m.dagGen.Generate(ctx, planning.GenerateInput{Files: fileContents})
	if err != nil {
		return nil, fmt.Errorf("generate dag: %w", err)
	}

	actions, err := planning.MaterializeDAG(ctx, m.store, workItemID, dag)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, len(actions))
	for i, a := range actions {
		ids[i] = a.ID
	}
	return ids, nil
}
