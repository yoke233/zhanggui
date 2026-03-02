package storesqlite

import (
	"testing"
	"time"

	"github.com/user/ai-workflow/internal/core"
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
		ID:        "chat-20260301-aaaabbbb",
		ProjectID: project.ID,
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

func TestTaskPlanTaskItemAndReviewRecordCRUD(t *testing.T) {
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

	plan := &core.TaskPlan{
		ID:          "plan-20260301-a3f1b2c0",
		ProjectID:   project.ID,
		SessionID:   session.ID,
		Name:        "add-oauth-login",
		Status:      core.PlanDraft,
		WaitReason:  core.WaitNone,
		FailPolicy:  core.FailBlock,
		ReviewRound: 1,
	}
	if err := s.CreateTaskPlan(plan); err != nil {
		t.Fatal(err)
	}

	gotPlan, err := s.GetTaskPlan(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotPlan.Status != core.PlanDraft {
		t.Fatalf("expected draft status, got %s", gotPlan.Status)
	}
	if len(gotPlan.Tasks) != 0 {
		t.Fatalf("expected empty tasks on new plan, got %d", len(gotPlan.Tasks))
	}

	pipeline := &core.Pipeline{
		ID:        "20260301-123456abcdef",
		ProjectID: project.ID,
		Name:      "task-runner",
		Template:  "standard",
		Status:    core.StatusCreated,
		Stages:    []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
	}
	if err := s.SavePipeline(pipeline); err != nil {
		t.Fatal(err)
	}

	item := &core.TaskItem{
		ID:          "task-a3f1b2c0-1",
		PlanID:      plan.ID,
		Title:       "后端 OAuth 接口",
		Description: "实现 OAuth 登录接口并添加测试",
		Labels:      []string{"backend", "auth"},
		DependsOn:   []string{},
		Template:    "standard",
		Status:      core.ItemPending,
	}
	if err := s.CreateTaskItem(item); err != nil {
		t.Fatal(err)
	}

	createdItem, err := s.GetTaskItem(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if createdItem.Status != core.ItemPending {
		t.Fatalf("expected pending item, got %s", createdItem.Status)
	}

	item.Status = core.ItemRunning
	item.PipelineID = pipeline.ID
	item.ExternalID = "ISSUE-101"
	if err := s.SaveTaskItem(item); err != nil {
		t.Fatal(err)
	}

	byPipeline, err := s.GetTaskItemByPipeline(pipeline.ID)
	if err != nil {
		t.Fatal(err)
	}
	if byPipeline.ID != item.ID {
		t.Fatalf("expected task %s by pipeline, got %s", item.ID, byPipeline.ID)
	}

	items, err := s.GetTaskItemsByPlan(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("unexpected items by plan: %#v", items)
	}

	plan.Status = core.PlanExecuting
	plan.WaitReason = core.WaitNone
	if err := s.SaveTaskPlan(plan); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListTaskPlans(project.ID, core.TaskPlanFilter{
		Status: string(core.PlanExecuting),
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != plan.ID {
		t.Fatalf("unexpected task plan list: %#v", list)
	}

	active, err := s.GetActiveTaskPlans()
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != plan.ID {
		t.Fatalf("unexpected active task plans: %#v", active)
	}

	score := 88
	record := &core.ReviewRecord{
		PlanID:   plan.ID,
		Round:    1,
		Reviewer: "completeness",
		Verdict:  "issues_found",
		Issues: []core.ReviewIssue{
			{
				Severity:    "warning",
				TaskID:      item.ID,
				Description: "任务粒度略大",
				Suggestion:  "拆分为接口实现和回归测试两个任务",
			},
		},
		Fixes: []core.ProposedFix{
			{
				TaskID:      item.ID,
				Description: "补充一个独立测试任务",
				Suggestion:  "新增 task-a3f1b2c0-2",
			},
		},
		Score: &score,
	}
	if err := s.SaveReviewRecord(record); err != nil {
		t.Fatal(err)
	}

	records, err := s.GetReviewRecords(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 review record, got %d", len(records))
	}
	if records[0].Reviewer != "completeness" || records[0].Score == nil || *records[0].Score != 88 {
		t.Fatalf("unexpected review record: %#v", records[0])
	}
	if len(records[0].Issues) != 1 || len(records[0].Fixes) != 1 {
		t.Fatalf("unexpected review payload: issues=%d fixes=%d", len(records[0].Issues), len(records[0].Fixes))
	}
}
