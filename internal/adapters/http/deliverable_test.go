package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/yoke233/zhanggui/internal/core"
)

func TestWorkItemDeliverableAdoptionEndpoints(t *testing.T) {
	h, ts := setupAPI(t)
	ctx := context.Background()

	workItemID, err := h.store.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "deliverable-adoption",
		Status:   core.WorkItemOpen,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	threadID, err := h.store.CreateThread(ctx, &core.Thread{
		Title:  "artifact-thread",
		Status: core.ThreadActive,
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	deliverableID, err := h.store.CreateDeliverable(ctx, &core.Deliverable{
		ThreadID:     &threadID,
		Kind:         core.DeliverableDocument,
		Title:        "Login Flow Design",
		Summary:      "thread result",
		ProducerType: core.DeliverableProducerThread,
		ProducerID:   threadID,
		Status:       core.DeliverableFinal,
	})
	if err != nil {
		t.Fatalf("create deliverable: %v", err)
	}
	if _, err := h.store.CreateThreadWorkItemLink(ctx, &core.ThreadWorkItemLink{
		ThreadID:     threadID,
		WorkItemID:   workItemID,
		RelationType: "drives",
		IsPrimary:    true,
	}); err != nil {
		t.Fatalf("create thread-work item link: %v", err)
	}
	gateActionID, err := h.store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "gate review",
		Type:       core.ActionGate,
		Status:     core.ActionReady,
		Position:   1,
	})
	if err != nil {
		t.Fatalf("create gate action: %v", err)
	}

	resp, err := post(ts, fmt.Sprintf("/work-items/%d/final-deliverable", workItemID), map[string]any{
		"deliverable_id": deliverableID,
	})
	if err != nil {
		t.Fatalf("post adoption: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /work-items/{id}/final-deliverable status = %d", resp.StatusCode)
	}
	var updated core.WorkItem
	if err := decodeJSON(resp, &updated); err != nil {
		t.Fatalf("decode adoption response: %v", err)
	}
	if updated.FinalDeliverableID == nil || *updated.FinalDeliverableID != deliverableID {
		t.Fatalf("FinalDeliverableID = %v, want %d", updated.FinalDeliverableID, deliverableID)
	}
	if updated.Status != core.WorkItemCompleted {
		t.Fatalf("Status = %q, want %q", updated.Status, core.WorkItemCompleted)
	}

	gateAction, err := h.store.GetAction(ctx, gateActionID)
	if err != nil {
		t.Fatalf("get gate action: %v", err)
	}
	if gateAction.Status != core.ActionCancelled {
		t.Fatalf("gate action status = %q, want %q", gateAction.Status, core.ActionCancelled)
	}

	resp, err = get(ts, "/work-items/pending?profile_id=human")
	if err != nil {
		t.Fatalf("get pending work items: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /work-items/pending status = %d", resp.StatusCode)
	}
	var pending []pendingWorkItemItem
	if err := decodeJSON(resp, &pending); err != nil {
		t.Fatalf("decode pending work items: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending items = %+v, want empty", pending)
	}

	resp, err = get(ts, fmt.Sprintf("/work-items/%d/deliverables", workItemID))
	if err != nil {
		t.Fatalf("get work item deliverables: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /work-items/{id}/deliverables status = %d", resp.StatusCode)
	}
	var items []core.Deliverable
	if err := decodeJSON(resp, &items); err != nil {
		t.Fatalf("decode work item deliverables: %v", err)
	}
	if len(items) != 1 || items[0].ID != deliverableID {
		t.Fatalf("work item deliverables = %+v", items)
	}
}

func TestWorkItemDeliverableAdoptionRejectsInvalidDeliverables(t *testing.T) {
	h, ts := setupAPI(t)
	ctx := context.Background()

	workItemID, err := h.store.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "deliverable-invalid-adoption",
		Status:   core.WorkItemPendingReview,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	threadID, err := h.store.CreateThread(ctx, &core.Thread{
		Title:  "unlinked-thread",
		Status: core.ThreadActive,
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	deliverableID, err := h.store.CreateDeliverable(ctx, &core.Deliverable{
		ThreadID:     &threadID,
		Kind:         core.DeliverableDocument,
		Title:        "Unlinked thread result",
		Summary:      "cannot adopt",
		ProducerType: core.DeliverableProducerThread,
		ProducerID:   threadID,
		Status:       core.DeliverableFinal,
	})
	if err != nil {
		t.Fatalf("create deliverable: %v", err)
	}

	resp, err := post(ts, fmt.Sprintf("/work-items/%d/final-deliverable", workItemID), map[string]any{
		"deliverable_id": deliverableID,
	})
	if err != nil {
		t.Fatalf("post adoption: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("POST /work-items/{id}/final-deliverable status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestThreadDeliverablesEndpoint(t *testing.T) {
	_, ts := setupAPI(t)

	resp, err := post(ts, "/threads", map[string]any{"title": "thread-deliverables"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode thread: %v", err)
	}

	resp, err = post(ts, fmt.Sprintf("/threads/%d/messages", thread.ID), map[string]any{
		"sender_id": "user-1",
		"role":      "human",
		"content":   "# Login Flow Design\n\nThread design ready.",
		"metadata": map[string]any{
			core.ResultMetaArtifactNamespace: "gstack",
			core.ResultMetaArtifactType:      "design_doc",
			core.ResultMetaArtifactFormat:    "markdown",
			core.ResultMetaArtifactRelPath:   ".ai-workflow/artifacts/gstack/office-hours/2026-03-31-login-flow.md",
			core.ResultMetaArtifactTitle:     "Login Flow Design",
			core.ResultMetaSummary:           "thread-level design note",
		},
	})
	if err != nil {
		t.Fatalf("create thread message: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /threads/{id}/messages status = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	resp, err = get(ts, fmt.Sprintf("/threads/%d/deliverables", thread.ID))
	if err != nil {
		t.Fatalf("get thread deliverables: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /threads/{id}/deliverables status = %d", resp.StatusCode)
	}
	var items []core.Deliverable
	if err := decodeJSON(resp, &items); err != nil {
		t.Fatalf("decode thread deliverables: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("thread deliverables len = %d, want 1", len(items))
	}
	if items[0].ProducerType != core.DeliverableProducerThread {
		t.Fatalf("ProducerType = %q, want %q", items[0].ProducerType, core.DeliverableProducerThread)
	}
	if items[0].Title != "Login Flow Design" {
		t.Fatalf("Title = %q, want Login Flow Design", items[0].Title)
	}
}
