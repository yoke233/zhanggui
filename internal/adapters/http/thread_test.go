package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
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

	resp, err = get(ts, fmt.Sprintf("/threads/%d/participants", thread.ID))
	if err != nil {
		t.Fatalf("get participants: %v", err)
	}
	var participants []core.ThreadMember
	if err := decodeJSON(resp, &participants); err != nil {
		t.Fatalf("decode participants: %v", err)
	}
	if len(participants) != 1 || participants[0].UserID != "user-1" || participants[0].Role != "owner" {
		t.Fatalf("unexpected owner participants: %+v", participants)
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

func TestThreadListRejectsInvalidStatusFilter(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := get(ts, "/threads?status=broken")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestThreadUpdateRejectsInvalidStatus(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "state-check"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	resp, _ = put(ts, fmt.Sprintf("/threads/%d", thread.ID), map[string]any{
		"status": "broken",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestThreadUpdateRejectsInvalidStatusTransition(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "archived-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	resp, _ = put(ts, fmt.Sprintf("/threads/%d", thread.ID), map[string]any{"status": "archived"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 archiving thread, got %d", resp.StatusCode)
	}

	resp, _ = put(ts, fmt.Sprintf("/threads/%d", thread.ID), map[string]any{"status": "active"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for archived -> active, got %d", resp.StatusCode)
	}
}

func TestThreadDeleteCleansUpRuntime(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{}
	h.threadPool = threadPool

	resp, _ := post(ts, "/threads", map[string]any{"title": "cleanup-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+fmt.Sprintf("/threads/%d", thread.ID), nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if len(threadPool.cleanupCalls) != 1 || threadPool.cleanupCalls[0] != thread.ID {
		t.Fatalf("unexpected cleanup calls: %+v", threadPool.cleanupCalls)
	}
}

func TestThreadDeleteStopsWhenRuntimeCleanupFails(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{cleanupErr: fmt.Errorf("cleanup failed")}
	h.threadPool = threadPool

	resp, _ := post(ts, "/threads", map[string]any{"title": "cleanup-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+fmt.Sprintf("/threads/%d", thread.ID), nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, fmt.Sprintf("/threads/%d", thread.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected thread to remain after cleanup failure, got %d", resp.StatusCode)
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

func TestThreadMessageReplyTo(t *testing.T) {
	h, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "reply-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	root := &core.ThreadMessage{
		ThreadID: thread.ID,
		SenderID: "user-1",
		Role:     "human",
		Content:  "root",
	}
	rootID, err := h.store.CreateThreadMessage(context.Background(), root)
	if err != nil {
		t.Fatalf("seed root message: %v", err)
	}
	root.ID = rootID

	resp, err = post(ts, fmt.Sprintf("/threads/%d/messages", thread.ID), map[string]any{
		"sender_id":       "user-2",
		"content":         "reply",
		"reply_to_msg_id": root.ID,
	})
	if err != nil {
		t.Fatalf("create reply: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var reply core.ThreadMessage
	if err := decodeJSON(resp, &reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if reply.ReplyToMessageID == nil || *reply.ReplyToMessageID != root.ID {
		t.Fatalf("unexpected reply_to_msg_id: %+v", reply.ReplyToMessageID)
	}
}

func TestThreadMessageReplyToRejectsCrossThread(t *testing.T) {
	h, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "thread-a"})
	var threadA core.Thread
	decodeJSON(resp, &threadA)
	resp, _ = post(ts, "/threads", map[string]any{"title": "thread-b"})
	var threadB core.Thread
	decodeJSON(resp, &threadB)

	foreign := &core.ThreadMessage{
		ThreadID: threadB.ID,
		SenderID: "user-1",
		Role:     "human",
		Content:  "foreign",
	}
	foreignID, err := h.store.CreateThreadMessage(context.Background(), foreign)
	if err != nil {
		t.Fatalf("seed foreign message: %v", err)
	}

	resp, err = post(ts, fmt.Sprintf("/threads/%d/messages", threadA.ID), map[string]any{
		"sender_id":       "user-2",
		"content":         "reply",
		"reply_to_msg_id": foreignID,
	})
	if err != nil {
		t.Fatalf("create reply: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
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
	var p core.ThreadMember
	decodeJSON(resp, &p)
	if p.UserID != "user-1" {
		t.Fatalf("expected user_id 'user-1', got %q", p.UserID)
	}

	// List participants.
	resp, _ = get(ts, fmt.Sprintf("/threads/%d/participants", thread.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var participants []core.ThreadMember
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
	resp, _ = post(ts, "/work-items", map[string]any{"title": "work-item-1"})
	var issue core.WorkItem
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
	resp, _ = get(ts, fmt.Sprintf("/work-items/%d/threads", issue.ID))
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

func TestThreadWorkItemReverseLookupAlias(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "link-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	resp, _ = post(ts, "/work-items", map[string]any{"title": "work-item-1"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)

	resp, _ = post(ts, fmt.Sprintf("/threads/%d/links/work-items", thread.ID), map[string]any{
		"work_item_id": issue.ID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating link, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, fmt.Sprintf("/work-items/%d/threads", issue.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /work-items/{id}/threads, got %d", resp.StatusCode)
	}
	var reverseLinks []core.ThreadWorkItemLink
	decodeJSON(resp, &reverseLinks)
	if len(reverseLinks) != 1 || reverseLinks[0].ThreadID != thread.ID {
		t.Fatalf("unexpected reverse links from alias route: %+v", reverseLinks)
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
	var sess core.ThreadMember
	decodeJSON(resp, &sess)
	if sess.AgentProfileID != "worker-claude" || sess.Status != "active" {
		t.Fatalf("unexpected session: %+v", sess)
	}

	resp, _ = get(ts, fmt.Sprintf("/threads/%d/participants", thread.ID))
	var participants []core.ThreadMember
	decodeJSON(resp, &participants)
	if len(participants) != 1 || participants[0].UserID != "worker-claude" || participants[0].Role != "agent" {
		t.Fatalf("unexpected agent participants: %+v", participants)
	}

	// List agents.
	resp, _ = get(ts, fmt.Sprintf("/threads/%d/agents", thread.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var sessions []core.ThreadMember
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

	resp, _ = get(ts, fmt.Sprintf("/threads/%d/participants", thread.ID))
	decodeJSON(resp, &participants)
	if len(participants) != 1 {
		t.Fatalf("expected agent participant snapshot to remain, got %d", len(participants))
	}
}

func TestThreadParticipantRemoveRejectsActiveAgentSession(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "agent-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	resp, _ = post(ts, fmt.Sprintf("/threads/%d/agents", thread.ID), map[string]any{
		"agent_profile_id": "worker-claude",
	})
	var sess core.ThreadMember
	decodeJSON(resp, &sess)

	req, _ := http.NewRequest(http.MethodDelete,
		ts.URL+fmt.Sprintf("/threads/%d/participants/worker-claude", thread.ID), nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
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
	var sess core.ThreadMember
	decodeJSON(resp, &sess)

	req, _ := http.NewRequest(http.MethodDelete,
		ts.URL+fmt.Sprintf("/threads/%d/agents/%d", threadA.ID, sess.ID), nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-thread delete, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, fmt.Sprintf("/threads/%d/agents", threadB.ID))
	var sessions []core.ThreadMember
	decodeJSON(resp, &sessions)
	if len(sessions) != 1 {
		t.Fatalf("expected session to remain on original thread, got %d", len(sessions))
	}
}

func TestThreadCreateWorkItem(t *testing.T) {
	_, ts := setupAPI(t)

	// Create thread.
	resp, _ := post(ts, "/threads", map[string]any{
		"title":   "create-wi-thread",
		"summary": "summary should be ignored on create",
	})
	var thread core.Thread
	decodeJSON(resp, &thread)

	resp, _ = put(ts, fmt.Sprintf("/threads/%d", thread.ID), map[string]any{
		"summary": "Ship the thread summary as the default work item body.",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 updating summary, got %d", resp.StatusCode)
	}

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
	var issue core.WorkItem
	if err := decodeJSON(resp, &issue); err != nil {
		t.Fatalf("decode issue: %v", err)
	}
	if issue.Body != "Ship the thread summary as the default work item body." {
		t.Fatalf("expected summary-backed body, got %q", issue.Body)
	}
	if issue.Metadata["source_thread_id"] != float64(thread.ID) {
		t.Fatalf("expected source_thread_id=%d, got %#v", thread.ID, issue.Metadata["source_thread_id"])
	}
	if issue.Metadata["source_type"] != "thread_summary" {
		t.Fatalf("expected source_type=thread_summary, got %#v", issue.Metadata["source_type"])
	}
	if issue.Metadata["body_from_summary"] != true {
		t.Fatalf("expected body_from_summary=true, got %#v", issue.Metadata["body_from_summary"])
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

func TestThreadCreateWorkItemRequiresSummaryWhenBodyMissing(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "create-wi-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	resp, err := post(ts, fmt.Sprintf("/threads/%d/create-work-item", thread.ID), map[string]any{
		"title": "spawned work item",
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var apiErr map[string]any
	if err := decodeJSON(resp, &apiErr); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if apiErr["error"] != "please generate or fill in summary first" {
		t.Fatalf("unexpected error message: %#v", apiErr["error"])
	}
	if apiErr["code"] != "MISSING_THREAD_SUMMARY" {
		t.Fatalf("unexpected error code: %#v", apiErr["code"])
	}
}

func TestThreadCreateWorkItemWithExplicitBodyMarksManualSource(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "create-wi-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	resp, err := post(ts, fmt.Sprintf("/threads/%d/create-work-item", thread.ID), map[string]any{
		"title": "spawned work item",
		"body":  "Custom body that does not come from summary.",
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var issue core.WorkItem
	if err := decodeJSON(resp, &issue); err != nil {
		t.Fatalf("decode issue: %v", err)
	}
	if issue.Body != "Custom body that does not come from summary." {
		t.Fatalf("unexpected body: %q", issue.Body)
	}
	if issue.Metadata["source_thread_id"] != float64(thread.ID) {
		t.Fatalf("expected source_thread_id=%d, got %#v", thread.ID, issue.Metadata["source_thread_id"])
	}
	if issue.Metadata["source_type"] != "thread_manual" {
		t.Fatalf("expected source_type=thread_manual, got %#v", issue.Metadata["source_type"])
	}
	if issue.Metadata["body_from_summary"] != false {
		t.Fatalf("expected body_from_summary=false, got %#v", issue.Metadata["body_from_summary"])
	}
}

func TestThreadAndWorkItemRoutesIndependent(t *testing.T) {
	_, ts := setupAPI(t)

	// /work-items should be accessible alongside /threads.
	resp, err := get(ts, "/work-items")
	if err != nil {
		t.Fatalf("get work-items: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/work-items expected 200, got %d", resp.StatusCode)
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

func TestThreadMessageHTTPBroadcastsToWebSocketSubscribers(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "http-broadcast-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)
	if err := conn.WriteJSON(map[string]any{
		"type": "subscribe_thread",
		"data": map[string]any{"thread_id": thread.ID},
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var subAck map[string]any
	if err := conn.ReadJSON(&subAck); err != nil {
		t.Fatalf("read subscribe ack: %v", err)
	}

	resp, err = post(ts, fmt.Sprintf("/threads/%d/messages", thread.ID), map[string]any{
		"sender_id": "user-1",
		"content":   "hello via http",
	})
	if err != nil {
		t.Fatalf("http create message: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ev core.Event
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if ev.Type != core.EventThreadMessage {
		t.Fatalf("event type = %q, want %q", ev.Type, core.EventThreadMessage)
	}
	if ev.Data["message"] != "hello via http" {
		t.Fatalf("unexpected event payload: %+v", ev.Data)
	}
}
