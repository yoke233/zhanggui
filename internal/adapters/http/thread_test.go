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

func TestThreadMessageCRUD(t *testing.T) {
	_, ts := setupAPI(t)

	// Create thread.
	resp, _ := post(ts, "/threads", map[string]any{"title": "msg-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	// Create message.
	resp, err := post(ts, fmt.Sprintf("/threads/%d/messages", thread.ID), map[string]any{
		"sender_id": "user-1",
		"role":      "human",
		"content":   "hello from HTTP",
	})
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var msg core.ThreadMessage
	decodeJSON(resp, &msg)
	if msg.Content != "hello from HTTP" {
		t.Fatalf("expected content 'hello from HTTP', got %q", msg.Content)
	}
	if msg.ThreadID != thread.ID {
		t.Fatalf("expected thread_id %d, got %d", thread.ID, msg.ThreadID)
	}

	// List messages.
	resp, _ = get(ts, fmt.Sprintf("/threads/%d/messages", thread.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var msgs []core.ThreadMessage
	decodeJSON(resp, &msgs)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	// Create message on non-existent thread.
	resp, _ = post(ts, "/threads/9999/messages", map[string]any{"content": "x"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestThreadParticipantCRUD(t *testing.T) {
	_, ts := setupAPI(t)

	// Create thread.
	resp, _ := post(ts, "/threads", map[string]any{"title": "participant-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	// Add participant.
	resp, err := post(ts, fmt.Sprintf("/threads/%d/participants", thread.ID), map[string]any{
		"user_id": "user-1",
		"role":    "owner",
	})
	if err != nil {
		t.Fatalf("add participant: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var p core.ThreadParticipant
	decodeJSON(resp, &p)
	if p.UserID != "user-1" {
		t.Fatalf("expected user_id 'user-1', got %q", p.UserID)
	}

	// List participants.
	resp, _ = get(ts, fmt.Sprintf("/threads/%d/participants", thread.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var participants []core.ThreadParticipant
	decodeJSON(resp, &participants)
	if len(participants) != 1 {
		t.Fatalf("expected 1 participant, got %d", len(participants))
	}

	// Remove participant.
	req, _ := http.NewRequest(http.MethodDelete,
		ts.URL+fmt.Sprintf("/threads/%d/participants/user-1", thread.ID), nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify removed.
	resp, _ = get(ts, fmt.Sprintf("/threads/%d/participants", thread.ID))
	decodeJSON(resp, &participants)
	if len(participants) != 0 {
		t.Fatalf("expected 0 participants after remove, got %d", len(participants))
	}
}

func TestThreadWorkItemLinkCRUD(t *testing.T) {
	_, ts := setupAPI(t)

	// Create thread.
	resp, _ := post(ts, "/threads", map[string]any{"title": "link-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	// Create issue (work item).
	resp, _ = post(ts, "/issues", map[string]any{"title": "work-item-1"})
	var issue core.Issue
	decodeJSON(resp, &issue)

	// Create link.
	resp, err := post(ts, fmt.Sprintf("/threads/%d/links/work-items", thread.ID), map[string]any{
		"work_item_id":  issue.ID,
		"relation_type": "related",
		"is_primary":    true,
	})
	if err != nil {
		t.Fatalf("create link: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var link core.ThreadWorkItemLink
	decodeJSON(resp, &link)
	if link.WorkItemID != issue.ID || !link.IsPrimary {
		t.Fatalf("unexpected link: %+v", link)
	}

	// List work items by thread.
	resp, _ = get(ts, fmt.Sprintf("/threads/%d/work-items", thread.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var links []core.ThreadWorkItemLink
	decodeJSON(resp, &links)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}

	// List threads by work item.
	resp, _ = get(ts, fmt.Sprintf("/issues/%d/threads", issue.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var reverseLinks []core.ThreadWorkItemLink
	decodeJSON(resp, &reverseLinks)
	if len(reverseLinks) != 1 || reverseLinks[0].ThreadID != thread.ID {
		t.Fatalf("unexpected reverse links: %+v", reverseLinks)
	}

	// Delete link.
	req, _ := http.NewRequest(http.MethodDelete,
		ts.URL+fmt.Sprintf("/threads/%d/links/work-items/%d", thread.ID, issue.ID), nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify deleted.
	resp, _ = get(ts, fmt.Sprintf("/threads/%d/work-items", thread.ID))
	decodeJSON(resp, &links)
	if len(links) != 0 {
		t.Fatalf("expected 0 links after delete, got %d", len(links))
	}
}

func TestThreadAgentSessionCRUD(t *testing.T) {
	_, ts := setupAPI(t)

	// Create thread.
	resp, _ := post(ts, "/threads", map[string]any{"title": "agent-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	// Invite agent.
	resp, err := post(ts, fmt.Sprintf("/threads/%d/agents", thread.ID), map[string]any{
		"agent_profile_id": "worker-claude",
	})
	if err != nil {
		t.Fatalf("invite agent: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var sess core.ThreadAgentSession
	decodeJSON(resp, &sess)
	if sess.AgentProfileID != "worker-claude" || sess.Status != "active" {
		t.Fatalf("unexpected session: %+v", sess)
	}

	// List agents.
	resp, _ = get(ts, fmt.Sprintf("/threads/%d/agents", thread.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var sessions []core.ThreadAgentSession
	decodeJSON(resp, &sessions)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	// Remove agent.
	req, _ := http.NewRequest(http.MethodDelete,
		ts.URL+fmt.Sprintf("/threads/%d/agents/%d", thread.ID, sess.ID), nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify removed.
	resp, _ = get(ts, fmt.Sprintf("/threads/%d/agents", thread.ID))
	decodeJSON(resp, &sessions)
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions after remove, got %d", len(sessions))
	}
}

func TestThreadAgentSessionDeleteRejectsCrossThreadSession(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "thread-a"})
	var threadA core.Thread
	decodeJSON(resp, &threadA)

	resp, _ = post(ts, "/threads", map[string]any{"title": "thread-b"})
	var threadB core.Thread
	decodeJSON(resp, &threadB)

	resp, _ = post(ts, fmt.Sprintf("/threads/%d/agents", threadB.ID), map[string]any{
		"agent_profile_id": "worker-claude",
	})
	var sess core.ThreadAgentSession
	decodeJSON(resp, &sess)

	req, _ := http.NewRequest(http.MethodDelete,
		ts.URL+fmt.Sprintf("/threads/%d/agents/%d", threadA.ID, sess.ID), nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-thread delete, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, fmt.Sprintf("/threads/%d/agents", threadB.ID))
	var sessions []core.ThreadAgentSession
	decodeJSON(resp, &sessions)
	if len(sessions) != 1 {
		t.Fatalf("expected session to remain on original thread, got %d", len(sessions))
	}
}

func TestThreadCreateWorkItem(t *testing.T) {
	_, ts := setupAPI(t)

	// Create thread.
	resp, _ := post(ts, "/threads", map[string]any{"title": "create-wi-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	// Create work item from thread.
	resp, err := post(ts, fmt.Sprintf("/threads/%d/create-work-item", thread.ID), map[string]any{
		"title": "spawned work item",
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	// Verify link was created.
	resp, _ = get(ts, fmt.Sprintf("/threads/%d/work-items", thread.ID))
	var links []core.ThreadWorkItemLink
	decodeJSON(resp, &links)
	if len(links) != 1 {
		t.Fatalf("expected 1 auto-created link, got %d", len(links))
	}
	if !links[0].IsPrimary {
		t.Fatal("expected auto-created link to be primary")
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
