package storesqlite

import (
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestChatSessionCRUD(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-chat", Name: "chat", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	session := &core.ChatSession{
		ID:             "chat-20260301-aaaabbbb",
		ProjectID:      project.ID,
		AgentSessionID: "claude-session-initial",
		Messages: []core.ChatMessage{
			{Role: "user", Content: "需要新增 OAuth 登录", Time: now},
		},
	}
	if err := s.CreateChatSession(session); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetChatSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectID != project.ID {
		t.Fatalf("expected project_id=%s, got %s", project.ID, got.ProjectID)
	}
	if got.AgentSessionID != "claude-session-initial" {
		t.Fatalf("expected agent_session_id persisted, got %q", got.AgentSessionID)
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" {
		t.Fatalf("unexpected chat messages: %#v", got.Messages)
	}

	list, err := s.ListChatSessions(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != session.ID {
		t.Fatalf("unexpected list result: %#v", list)
	}

	session.Messages = append(session.Messages, core.ChatMessage{
		Role:    "assistant",
		Content: "我先拆分任务",
		Time:    now.Add(time.Minute),
	})
	session.AgentSessionID = "claude-session-updated"
	if err := s.UpdateChatSession(session); err != nil {
		t.Fatal(err)
	}

	updated, err := s.GetChatSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Messages) != 2 || updated.Messages[1].Role != "assistant" {
		t.Fatalf("unexpected updated messages: %#v", updated.Messages)
	}
	if updated.AgentSessionID != "claude-session-updated" {
		t.Fatalf("expected updated agent_session_id, got %q", updated.AgentSessionID)
	}

	if err := s.DeleteChatSession(session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetChatSession(session.ID); err == nil {
		t.Fatalf("expected deleted chat session %s to be not found", session.ID)
	}
}

func TestChatRunEventCRUD(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-chat-event", Name: "chat-event", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	session := &core.ChatSession{
		ID:        "chat-20260303-run-events",
		ProjectID: project.ID,
		Messages: []core.ChatMessage{
			{Role: "user", Content: "run event test", Time: time.Now().UTC().Truncate(time.Second)},
		},
	}
	if err := s.CreateChatSession(session); err != nil {
		t.Fatal(err)
	}

	eventTime := time.Now().UTC().Truncate(time.Second)
	if err := s.AppendChatRunEvent(core.ChatRunEvent{
		SessionID:  session.ID,
		ProjectID:  project.ID,
		EventType:  "chat_run_update",
		UpdateType: "tool_call",
		Payload: map[string]any{
			"session_id": session.ID,
			"acp": map[string]any{
				"sessionUpdate": "tool_call",
				"title":         "Terminal",
				"status":        "pending",
			},
		},
		CreatedAt: eventTime,
	}); err != nil {
		t.Fatalf("append chat run event: %v", err)
	}

	events, err := s.ListChatRunEvents(session.ID)
	if err != nil {
		t.Fatalf("list chat run events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 run event, got %d", len(events))
	}
	if events[0].SessionID != session.ID {
		t.Fatalf("event session_id mismatch: got=%q want=%q", events[0].SessionID, session.ID)
	}
	if events[0].ProjectID != project.ID {
		t.Fatalf("event project_id mismatch: got=%q want=%q", events[0].ProjectID, project.ID)
	}
	if events[0].EventType != "chat_run_update" || events[0].UpdateType != "tool_call" {
		t.Fatalf("unexpected event type payload: %#v", events[0])
	}
	if events[0].CreatedAt.IsZero() {
		t.Fatalf("expected created_at to be persisted")
	}
	if events[0].Payload == nil {
		t.Fatalf("expected non-nil payload")
	}
	if gotSessionID, ok := events[0].Payload["session_id"].(string); !ok || gotSessionID != session.ID {
		t.Fatalf("payload session_id mismatch: %#v", events[0].Payload)
	}
}

func TestIssueAndReviewRecordCRUD(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-plan", Name: "plan", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	session := &core.ChatSession{
		ID:        "chat-20260301-ccccdddd",
		ProjectID: project.ID,
		Messages: []core.ChatMessage{
			{Role: "user", Content: "拆成多个任务", Time: time.Now().UTC().Truncate(time.Second)},
		},
	}
	if err := s.CreateChatSession(session); err != nil {
		t.Fatal(err)
	}

	Run := &core.Run{
		ID:        "20260301-123456abcdef",
		ProjectID: project.ID,
		Name:      "task-runner",
		Template:  "standard",
		Status:    core.StatusQueued,
		Stages:    []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
	}
	if err := s.SaveRun(Run); err != nil {
		t.Fatal(err)
	}

	issue := &core.Issue{
		ID:         "issue-20260301-a3f1b2c0",
		ProjectID:  project.ID,
		SessionID:  session.ID,
		Title:      "后端 OAuth 接口",
		Body:       "实现 OAuth 登录接口并添加测试",
		Labels:     []string{"backend", "auth"},
		DependsOn:  []string{},
		Template:   "standard",
		State:      core.IssueStateOpen,
		Status:     core.IssueStatusDraft,
		FailPolicy: core.FailBlock,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatal(err)
	}

	createdIssue, err := s.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if createdIssue.Status != core.IssueStatusDraft {
		t.Fatalf("expected draft issue status, got %s", createdIssue.Status)
	}

	issue.Status = core.IssueStatusExecuting
	issue.RunID = Run.ID
	issue.ExternalID = "ISSUE-101"
	if err := s.SaveIssue(issue); err != nil {
		t.Fatal(err)
	}

	byRun, err := s.GetIssueByRun(Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if byRun == nil || byRun.ID != issue.ID {
		t.Fatalf("expected issue %s by Run, got %#v", issue.ID, byRun)
	}

	list, total, err := s.ListIssues(project.ID, core.IssueFilter{
		Status: string(core.IssueStatusExecuting),
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 || list[0].ID != issue.ID {
		t.Fatalf("unexpected issue list: total=%d issues=%#v", total, list)
	}

	active, err := s.GetActiveIssues(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != issue.ID {
		t.Fatalf("unexpected active issues: %#v", active)
	}

	if err := s.SaveIssueAttachment(issue.ID, "docs/oauth.md", "oauth design"); err != nil {
		t.Fatal(err)
	}
	attachments, err := s.GetIssueAttachments(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 1 || attachments[0].Path != "docs/oauth.md" {
		t.Fatalf("unexpected attachments: %#v", attachments)
	}

	if err := s.SaveIssueChange(&core.IssueChange{
		IssueID:   issue.ID,
		Field:     "status",
		OldValue:  "draft",
		NewValue:  "executing",
		Reason:    "Run started",
		ChangedBy: "scheduler",
	}); err != nil {
		t.Fatal(err)
	}
	changes, err := s.GetIssueChanges(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Field != "status" {
		t.Fatalf("unexpected changes: %#v", changes)
	}

	score := 88
	record := &core.ReviewRecord{
		IssueID:   issue.ID,
		Round:     1,
		Reviewer:  "completeness",
		Verdict:   "issues_found",
		Summary:   "需要补充测试覆盖",
		RawOutput: "发现问题:\n- 缺少边界条件测试\n建议:\n- 增加失败路径与回滚验证",
		Issues: []core.ReviewIssue{
			{
				Severity:    "warning",
				IssueID:     issue.ID,
				Description: "任务粒度略大",
				Suggestion:  "拆分为接口实现和回归测试两个任务",
			},
		},
		Fixes: []core.ProposedFix{
			{
				IssueID:     issue.ID,
				Description: "补充一个独立测试任务",
				Suggestion:  "新增 issue-20260301-a3f1b2c1",
			},
		},
		Score: &score,
	}
	if err := s.SaveReviewRecord(record); err != nil {
		t.Fatal(err)
	}

	records, err := s.GetReviewRecords(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 review record, got %d", len(records))
	}
	if records[0].Reviewer != "completeness" || records[0].Score == nil || *records[0].Score != 88 {
		t.Fatalf("unexpected review record: %#v", records[0])
	}
	if records[0].Summary != "需要补充测试覆盖" {
		t.Fatalf("expected review summary persisted, got %q", records[0].Summary)
	}
	if records[0].RawOutput == "" {
		t.Fatalf("expected review raw_output persisted")
	}
	if len(records[0].Issues) != 1 || len(records[0].Fixes) != 1 {
		t.Fatalf("unexpected review payload: issues=%d fixes=%d", len(records[0].Issues), len(records[0].Fixes))
	}
}
