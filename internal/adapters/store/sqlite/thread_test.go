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

func TestThreadRejectsInvalidStatus(t *testing.T) {
	s := newTestStore(t)

	_, err := s.CreateThread(context.Background(), &core.Thread{
		Title:  "invalid-thread",
		Status: core.ThreadStatus("broken"),
	})
	if err == nil {
		t.Fatal("expected error for invalid thread status")
	}
}

func TestCreateThreadWithParticipants(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	thread := &core.Thread{
		Title:   "tx-thread",
		Status:  core.ThreadActive,
		OwnerID: "owner-1",
	}
	participants := []*core.ThreadMember{
		{Kind: "human", UserID: "owner-1", Role: "owner"},
		{Kind: "human", UserID: "member-1", Role: "member"},
		{Kind: "human", UserID: "member-1", Role: "member"},
	}

	if err := s.CreateThreadWithParticipants(ctx, thread, participants); err != nil {
		t.Fatalf("create thread with participants: %v", err)
	}
	if thread.ID <= 0 {
		t.Fatal("expected persisted thread id")
	}

	listed, err := s.ListThreadMembers(ctx, thread.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 unique members, got %d", len(listed))
	}
}

func TestThreadMessageCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create thread first.
	thread := &core.Thread{Title: "msg-test"}
	threadID, err := s.CreateThread(ctx, thread)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	// Create message.
	msg := &core.ThreadMessage{
		ThreadID: threadID,
		SenderID: "user-1",
		Role:     "human",
		Content:  "hello world",
		Metadata: map[string]any{"source": "test"},
	}
	msgID, err := s.CreateThreadMessage(ctx, msg)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	if msgID <= 0 {
		t.Fatal("expected positive message id")
	}

	// Create second message.
	msg2 := &core.ThreadMessage{
		ThreadID: threadID,
		SenderID: "agent-1",
		Role:     "agent",
		Content:  "hi there",
	}
	if _, err := s.CreateThreadMessage(ctx, msg2); err != nil {
		t.Fatalf("create message 2: %v", err)
	}

	// List messages.
	msgs, err := s.ListThreadMessages(ctx, threadID, 10, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "hello world" {
		t.Fatalf("expected first message content 'hello world', got %q", msgs[0].Content)
	}
	if msgs[1].Role != "agent" {
		t.Fatalf("expected second message role 'agent', got %q", msgs[1].Role)
	}
}

func TestThreadWorkItemLinkCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create thread and issue (work item).
	thread := &core.Thread{Title: "link-test"}
	threadID, err := s.CreateThread(ctx, thread)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	issue := &core.WorkItem{Title: "work-item-1", Status: core.WorkItemOpen}
	issueID, err := s.CreateWorkItem(ctx, issue)
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	// Create link.
	link := &core.ThreadWorkItemLink{
		ThreadID:     threadID,
		WorkItemID:   issueID,
		RelationType: "related",
		IsPrimary:    true,
	}
	linkID, err := s.CreateThreadWorkItemLink(ctx, link)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}
	if linkID <= 0 {
		t.Fatal("expected positive link id")
	}

	// List by thread.
	links, err := s.ListWorkItemsByThread(ctx, threadID)
	if err != nil {
		t.Fatalf("list by thread: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].WorkItemID != issueID || links[0].IsPrimary != true {
		t.Fatalf("unexpected link: %+v", links[0])
	}

	// List by work item.
	links2, err := s.ListThreadsByWorkItem(ctx, issueID)
	if err != nil {
		t.Fatalf("list by work item: %v", err)
	}
	if len(links2) != 1 || links2[0].ThreadID != threadID {
		t.Fatalf("unexpected reverse link: %+v", links2)
	}

	// Duplicate link should fail (UNIQUE constraint).
	dup := &core.ThreadWorkItemLink{
		ThreadID:     threadID,
		WorkItemID:   issueID,
		RelationType: "drives",
	}
	if _, err := s.CreateThreadWorkItemLink(ctx, dup); err == nil {
		t.Fatal("expected error for duplicate link")
	}

	// Delete specific link.
	if err := s.DeleteThreadWorkItemLink(ctx, threadID, issueID); err != nil {
		t.Fatalf("delete link: %v", err)
	}
	links, _ = s.ListWorkItemsByThread(ctx, threadID)
	if len(links) != 0 {
		t.Fatalf("expected 0 links after delete, got %d", len(links))
	}
}

func TestThreadWorkItemLinkPrimarySwitchDemotesPrevious(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	threadID, _ := s.CreateThread(ctx, &core.Thread{Title: "primary-switch"})
	issueID1, _ := s.CreateWorkItem(ctx, &core.WorkItem{Title: "wi-1", Status: core.WorkItemOpen})
	issueID2, _ := s.CreateWorkItem(ctx, &core.WorkItem{Title: "wi-2", Status: core.WorkItemOpen})

	if _, err := s.CreateThreadWorkItemLink(ctx, &core.ThreadWorkItemLink{
		ThreadID: threadID, WorkItemID: issueID1, RelationType: "related", IsPrimary: true,
	}); err != nil {
		t.Fatalf("create first primary: %v", err)
	}
	if _, err := s.CreateThreadWorkItemLink(ctx, &core.ThreadWorkItemLink{
		ThreadID: threadID, WorkItemID: issueID2, RelationType: "drives", IsPrimary: true,
	}); err != nil {
		t.Fatalf("create second primary: %v", err)
	}

	links, err := s.ListWorkItemsByThread(ctx, threadID)
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if links[0].IsPrimary {
		t.Fatal("expected first link to be demoted from primary")
	}
	if !links[1].IsPrimary {
		t.Fatal("expected second link to be primary")
	}
}

func TestThreadWorkItemLinkDuplicatePrimaryRollsBackDemotion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	threadID, _ := s.CreateThread(ctx, &core.Thread{Title: "primary-rollback"})
	issueID, _ := s.CreateWorkItem(ctx, &core.WorkItem{Title: "wi-1", Status: core.WorkItemOpen})

	if _, err := s.CreateThreadWorkItemLink(ctx, &core.ThreadWorkItemLink{
		ThreadID: threadID, WorkItemID: issueID, RelationType: "related", IsPrimary: true,
	}); err != nil {
		t.Fatalf("create primary: %v", err)
	}

	if _, err := s.CreateThreadWorkItemLink(ctx, &core.ThreadWorkItemLink{
		ThreadID: threadID, WorkItemID: issueID, RelationType: "related", IsPrimary: true,
	}); err == nil {
		t.Fatal("expected duplicate link creation to fail")
	}

	links, err := s.ListWorkItemsByThread(ctx, threadID)
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected original link to remain, got %d links", len(links))
	}
	if !links[0].IsPrimary {
		t.Fatal("expected original primary flag to remain after rollback")
	}
}

func TestThreadWorkItemLinkCleanup(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Setup: thread + 2 issues + links.
	thread := &core.Thread{Title: "cleanup-test"}
	threadID, _ := s.CreateThread(ctx, thread)

	issue1 := &core.WorkItem{Title: "wi-1", Status: core.WorkItemOpen}
	issueID1, _ := s.CreateWorkItem(ctx, issue1)
	issue2 := &core.WorkItem{Title: "wi-2", Status: core.WorkItemOpen}
	issueID2, _ := s.CreateWorkItem(ctx, issue2)

	s.CreateThreadWorkItemLink(ctx, &core.ThreadWorkItemLink{ThreadID: threadID, WorkItemID: issueID1, RelationType: "related"})
	s.CreateThreadWorkItemLink(ctx, &core.ThreadWorkItemLink{ThreadID: threadID, WorkItemID: issueID2, RelationType: "drives"})

	// Cleanup by thread: deletes all links for that thread.
	if err := s.DeleteThreadWorkItemLinksByThread(ctx, threadID); err != nil {
		t.Fatalf("cleanup by thread: %v", err)
	}
	links, _ := s.ListWorkItemsByThread(ctx, threadID)
	if len(links) != 0 {
		t.Fatalf("expected 0 links after thread cleanup, got %d", len(links))
	}

	// Re-create links for work-item cleanup test.
	s.CreateThreadWorkItemLink(ctx, &core.ThreadWorkItemLink{ThreadID: threadID, WorkItemID: issueID1, RelationType: "related"})

	// Cleanup by work item.
	if err := s.DeleteThreadWorkItemLinksByWorkItem(ctx, issueID1); err != nil {
		t.Fatalf("cleanup by work item: %v", err)
	}
	links2, _ := s.ListThreadsByWorkItem(ctx, issueID1)
	if len(links2) != 0 {
		t.Fatalf("expected 0 links after work item cleanup, got %d", len(links2))
	}
}

func TestThreadMemberAgentCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create thread.
	thread := &core.Thread{Title: "agent-test"}
	threadID, err := s.CreateThread(ctx, thread)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	// Create agent member.
	member := &core.ThreadMember{
		ThreadID:       threadID,
		Kind:           "agent",
		UserID:         "worker-claude",
		AgentProfileID: "worker-claude",
		Role:           "agent",
		Status:         core.ThreadAgentActive,
		AgentData:      map[string]any{"acp_session_id": "acp-sess-001"},
	}
	memberID, err := s.AddThreadMember(ctx, member)
	if err != nil {
		t.Fatalf("add agent member: %v", err)
	}
	if memberID <= 0 {
		t.Fatal("expected positive member id")
	}

	// Get member.
	got, err := s.GetThreadMember(ctx, memberID)
	if err != nil {
		t.Fatalf("get member: %v", err)
	}
	if got.AgentProfileID != "worker-claude" || got.Status != core.ThreadAgentActive {
		t.Fatalf("unexpected member: %+v", got)
	}

	// List members (filter for agents).
	members, err := s.ListThreadMembers(ctx, threadID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	agentCount := 0
	for _, m := range members {
		if m.Kind == "agent" {
			agentCount++
		}
	}
	if agentCount != 1 {
		t.Fatalf("expected 1 agent member, got %d", agentCount)
	}

	// Update status.
	got.Status = core.ThreadAgentLeft
	if err := s.UpdateThreadMember(ctx, got); err != nil {
		t.Fatalf("update member: %v", err)
	}
	got2, _ := s.GetThreadMember(ctx, memberID)
	if got2.Status != core.ThreadAgentLeft {
		t.Fatalf("expected status 'left', got %q", got2.Status)
	}

	// Delete member.
	if err := s.RemoveThreadMember(ctx, memberID); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	_, err = s.GetThreadMember(ctx, memberID)
	if err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestThreadMemberAgentDataRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	thread := &core.Thread{Title: "agent-data-test"}
	threadID, err := s.CreateThread(ctx, thread)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	member := &core.ThreadMember{
		ThreadID:       threadID,
		Kind:           "agent",
		UserID:         "claude-worker",
		AgentProfileID: "claude-worker",
		Role:           "agent",
		Status:         core.ThreadAgentActive,
		AgentData: map[string]any{
			"acp_session_id": "acp-123",
			"model":          "claude-4",
		},
	}
	memberID, err := s.AddThreadMember(ctx, member)
	if err != nil {
		t.Fatalf("add member: %v", err)
	}

	got, err := s.GetThreadMember(ctx, memberID)
	if err != nil {
		t.Fatalf("get member: %v", err)
	}

	// Update with runtime fields in AgentData.
	got.Status = core.ThreadAgentPaused
	got.AgentData = map[string]any{
		"acp_session_id":      "acp-123",
		"turn_count":          float64(10),
		"total_input_tokens":  float64(25000),
		"total_output_tokens": float64(8000),
		"progress_summary":    "Completed feature X with tests.",
		"model":               "claude-4",
	}
	if err := s.UpdateThreadMember(ctx, got); err != nil {
		t.Fatalf("update member: %v", err)
	}

	updated, err := s.GetThreadMember(ctx, memberID)
	if err != nil {
		t.Fatalf("get updated member: %v", err)
	}
	if updated.Status != core.ThreadAgentPaused {
		t.Fatalf("expected status %q, got %q", core.ThreadAgentPaused, updated.Status)
	}
	if updated.AgentData == nil || updated.AgentData["model"] != "claude-4" {
		t.Fatalf("unexpected agent_data: %v", updated.AgentData)
	}
}

func TestThreadMemberCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create thread.
	thread := &core.Thread{Title: "member-test"}
	threadID, err := s.CreateThread(ctx, thread)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	// Add human member.
	p := &core.ThreadMember{
		ThreadID: threadID,
		Kind:     "human",
		UserID:   "user-1",
		Role:     "owner",
	}
	pID, err := s.AddThreadMember(ctx, p)
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
	if pID <= 0 {
		t.Fatal("expected positive member id")
	}

	// Add second member (agent).
	p2 := &core.ThreadMember{
		ThreadID:       threadID,
		Kind:           "agent",
		UserID:         "agent-1",
		AgentProfileID: "agent-1",
		Role:           "agent",
	}
	if _, err := s.AddThreadMember(ctx, p2); err != nil {
		t.Fatalf("add member 2: %v", err)
	}

	// List members.
	members, err := s.ListThreadMembers(ctx, threadID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	// Remove member by user.
	if err := s.RemoveThreadMemberByUser(ctx, threadID, "user-1"); err != nil {
		t.Fatalf("remove member: %v", err)
	}

	members, _ = s.ListThreadMembers(ctx, threadID)
	if len(members) != 1 {
		t.Fatalf("expected 1 member after remove, got %d", len(members))
	}

	// Remove non-existent.
	if err := s.RemoveThreadMemberByUser(ctx, threadID, "nobody"); err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
