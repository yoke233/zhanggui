package sqlite

import (
	"context"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestThreadCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	thread := &core.Thread{
		Title:    "design discussion",
		OwnerID:  "user-1",
		Metadata: map[string]any{"topic": "architecture"},
	}
	id, err := s.CreateThread(ctx, thread)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id <= 0 {
		t.Fatal("expected positive id")
	}
	if thread.Status != core.ThreadActive {
		t.Fatalf("expected active status, got %s", thread.Status)
	}

	got, err := s.GetThread(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "design discussion" || got.Status != core.ThreadActive {
		t.Fatalf("unexpected thread: %+v", got)
	}
	if got.OwnerID != "user-1" {
		t.Fatalf("owner_id not preserved: %s", got.OwnerID)
	}
	if got.Metadata["topic"] != "architecture" {
		t.Fatalf("metadata not preserved: %v", got.Metadata)
	}

	// Update
	got.Title = "updated title"
	got.Summary = "summary of discussion"
	got.Status = core.ThreadClosed
	if err := s.UpdateThread(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = s.GetThread(ctx, id)
	if got.Title != "updated title" || got.Status != core.ThreadClosed {
		t.Fatalf("update not applied: %+v", got)
	}
	if got.Summary != "summary of discussion" {
		t.Fatalf("summary not updated: %s", got.Summary)
	}

	// List
	threads, err := s.ListThreads(ctx, core.ThreadFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}

	// List with status filter
	active := core.ThreadActive
	threads, err = s.ListThreads(ctx, core.ThreadFilter{Status: &active, Limit: 10})
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(threads) != 0 {
		t.Fatalf("expected 0 active threads, got %d", len(threads))
	}

	// Delete
	if err := s.DeleteThread(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = s.GetThread(ctx, id)
	if err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestThreadNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetThread(context.Background(), 9999)
	if err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestThreadDeleteNotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.DeleteThread(context.Background(), 9999); err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestThreadUpdateNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.UpdateThread(context.Background(), &core.Thread{ID: 9999, Title: "x"})
	if err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestThreadTitleRequired(t *testing.T) {
	s := newTestStore(t)
	_, err := s.CreateThread(context.Background(), &core.Thread{Title: "  "})
	if err == nil {
		t.Fatal("expected error for blank title")
	}
}
