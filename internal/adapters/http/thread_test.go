package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestThreadCRUD(t *testing.T) {
	_, ts := setupAPI(t)

	// Create thread
	resp, err := post(ts, "/threads", map[string]any{
		"title":    "design discussion",
		"owner_id": "user-1",
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if thread.Title != "design discussion" {
		t.Fatalf("expected title 'design discussion', got %q", thread.Title)
	}
	if thread.Status != core.ThreadActive {
		t.Fatalf("expected active status, got %s", thread.Status)
	}
	if thread.OwnerID != "user-1" {
		t.Fatalf("expected owner_id 'user-1', got %q", thread.OwnerID)
	}

	// Get thread
	resp, err = get(ts, fmt.Sprintf("/threads/%d", thread.ID))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var got core.Thread
	if err := decodeJSON(resp, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != thread.ID {
		t.Fatalf("expected id %d, got %d", thread.ID, got.ID)
	}

	// List threads
	resp, err = get(ts, "/threads")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var threads []core.Thread
	if err := decodeJSON(resp, &threads); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}

	// Update thread
	resp, err = put(ts, fmt.Sprintf("/threads/%d", thread.ID), map[string]any{
		"title":   "updated title",
		"summary": "key decisions made",
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var updated core.Thread
	if err := decodeJSON(resp, &updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.Title != "updated title" {
		t.Fatalf("expected 'updated title', got %q", updated.Title)
	}
	if updated.Summary != "key decisions made" {
		t.Fatalf("expected summary, got %q", updated.Summary)
	}

	// Delete thread
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+fmt.Sprintf("/threads/%d", thread.ID), nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Get after delete -> 404
	resp, _ = get(ts, fmt.Sprintf("/threads/%d", thread.ID))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
}

func TestThreadCreateMissingTitle(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": ""})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestThreadGetNotFound(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := get(ts, "/threads/9999")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestThreadAndIssueRoutesIndependent(t *testing.T) {
	_, ts := setupAPI(t)

	// /issues should still be accessible alongside /threads
	resp, err := get(ts, "/issues")
	if err != nil {
		t.Fatalf("get issues: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/issues expected 200, got %d", resp.StatusCode)
	}

	// /threads should also be accessible
	resp, err = get(ts, "/threads")
	if err != nil {
		t.Fatalf("get threads: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/threads expected 200, got %d", resp.StatusCode)
	}
}
