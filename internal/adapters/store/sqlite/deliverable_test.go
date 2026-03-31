package sqlite

import (
	"context"
	"testing"

	"github.com/yoke233/zhanggui/internal/core"
)

func TestDeliverableStoreCreateAndListByWorkItem(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	workItemID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		Title:  "deliverable-work-item",
		Status: core.WorkItemOpen,
	})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}

	runID, err := store.CreateRun(ctx, &core.Run{
		ActionID:   1,
		WorkItemID: workItemID,
		Status:     core.RunSucceeded,
		Attempt:    1,
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	id, err := store.CreateDeliverable(ctx, &core.Deliverable{
		WorkItemID:   &workItemID,
		Kind:         core.DeliverablePullRequest,
		Title:        "Open PR",
		Summary:      "PR ready for review",
		Payload:      map[string]any{"url": "https://example.test/pr/1"},
		ProducerType: core.DeliverableProducerRun,
		ProducerID:   runID,
		Status:       core.DeliverableFinal,
	})
	if err != nil || id == 0 {
		t.Fatalf("CreateDeliverable() id=%d err=%v", id, err)
	}

	got, err := store.GetDeliverable(ctx, id)
	if err != nil {
		t.Fatalf("GetDeliverable() error = %v", err)
	}
	if got.Kind != core.DeliverablePullRequest || got.ProducerID != runID {
		t.Fatalf("GetDeliverable() = %+v", got)
	}

	items, err := store.ListDeliverablesByWorkItem(ctx, workItemID)
	if err != nil {
		t.Fatalf("ListDeliverablesByWorkItem() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != id {
		t.Fatalf("ListDeliverablesByWorkItem() = %+v", items)
	}

	byProducer, err := store.ListDeliverablesByProducer(ctx, core.DeliverableProducerRun, runID)
	if err != nil {
		t.Fatalf("ListDeliverablesByProducer() error = %v", err)
	}
	if len(byProducer) != 1 || byProducer[0].ID != id {
		t.Fatalf("ListDeliverablesByProducer() = %+v", byProducer)
	}
}

func TestDeliverableStoreListByThread(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	threadID, err := store.CreateThread(ctx, &core.Thread{
		Title:   "design sync",
		OwnerID: "ceo",
		Status:  core.ThreadActive,
	})
	if err != nil {
		t.Fatalf("CreateThread() error = %v", err)
	}

	id, err := store.CreateDeliverable(ctx, &core.Deliverable{
		ThreadID:     &threadID,
		Kind:         core.DeliverableMeetingSummary,
		Title:        "Sync summary",
		Summary:      "Captured decisions",
		ProducerType: core.DeliverableProducerThread,
		ProducerID:   threadID,
		Status:       core.DeliverableFinal,
	})
	if err != nil {
		t.Fatalf("CreateDeliverable(thread) error = %v", err)
	}

	items, err := store.ListDeliverablesByThread(ctx, threadID)
	if err != nil {
		t.Fatalf("ListDeliverablesByThread() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != id {
		t.Fatalf("ListDeliverablesByThread() = %+v", items)
	}
}
