package storesqlite

import (
	"reflect"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestProjectCRUD(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	p := &core.Project{ID: "test-1", Name: "Test", RepoPath: "/tmp/test"}
	if err := s.CreateProject(p); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetProject("test-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Test" {
		t.Errorf("expected Test, got %s", got.Name)
	}

	got.Name = "Updated"
	if err := s.UpdateProject(got); err != nil {
		t.Fatal(err)
	}

	got2, _ := s.GetProject("test-1")
	if got2.Name != "Updated" {
		t.Errorf("expected Updated, got %s", got2.Name)
	}

	if err := s.DeleteProject("test-1"); err != nil {
		t.Fatal(err)
	}
	_, err = s.GetProject("test-1")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestRunsaveAndGet(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_ = s.CreateProject(&core.Project{ID: "proj-1", Name: "P", RepoPath: "/tmp/p"})

	pipe := &core.Run{
		ID:        "20260228-aabbccddeeff",
		ProjectID: "proj-1",
		Name:      "test-pipe",
		Template:  "standard",
		Status:    core.StatusQueued,
		IssueID:   "issue-20260302-aabbccdd",
		Stages:    []core.StageConfig{{Name: core.StageImplement, Agent: "claude"}},
		Artifacts: map[string]string{},

		MaxTotalRetries: 5,
	}
	if err := s.SaveRun(pipe); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetRun("20260228-aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	if got.Template != "standard" {
		t.Errorf("expected standard, got %s", got.Template)
	}
	if got.IssueID != pipe.IssueID {
		t.Fatalf("Run issue_id mismatch: got=%q want=%q", got.IssueID, pipe.IssueID)
	}
}

func TestIssueRoundTrip_PersistsStructuredFields(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-issue-rt", Name: "issue-rt", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	session := &core.ChatSession{
		ID:        "chat-issue-rt",
		ProjectID: project.ID,
		Messages:  []core.ChatMessage{{Role: "user", Content: "拆 issue", Time: time.Now().UTC().Truncate(time.Second)}},
	}
	if err := s.CreateChatSession(session); err != nil {
		t.Fatal(err)
	}
	Run := &core.Run{
		ID:        "pipe-issue-rt",
		ProjectID: project.ID,
		Name:      "Run-issue-rt",
		Template:  "standard",
		Status:    core.StatusQueued,
		Stages:    []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		Artifacts: map[string]string{},
	}
	if err := s.SaveRun(Run); err != nil {
		t.Fatal(err)
	}

	issue := &core.Issue{
		ID:          "issue-20260302-11223344",
		ProjectID:   project.ID,
		SessionID:   session.ID,
		Title:       "OAuth 登录",
		Body:        "实现 OAuth 登录接口并补齐回归测试",
		Labels:      []string{"backend", "auth"},
		MilestoneID: "ms-auth",
		Attachments: []string{"docs/auth-spec.md"},
		DependsOn:   []string{"issue-20260302-deadbeef"},
		Blocks:      []string{"issue-20260302-feedface"},
		Priority:    3,
		Template:    "standard",
		State:       core.IssueStateOpen,
		Status:      core.IssueStatusDraft,
		RunID:       Run.ID,
		Version:     1,
		ExternalID:  "ISSUE-101",
		FailPolicy:  core.FailBlock,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("create issue: %v", err)
	}

	got, err := s.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if got.Title != issue.Title || got.Status != issue.Status || got.State != issue.State {
		t.Fatalf("issue core fields mismatch: got=%#v want=%#v", got, issue)
	}
	if !reflect.DeepEqual(got.Labels, issue.Labels) ||
		!reflect.DeepEqual(got.Attachments, issue.Attachments) ||
		!reflect.DeepEqual(got.DependsOn, issue.DependsOn) ||
		!reflect.DeepEqual(got.Blocks, issue.Blocks) {
		t.Fatalf("issue structured fields mismatch: got=%#v want=%#v", got, issue)
	}

	issue.Status = core.IssueStatusExecuting
	issue.Version = 2
	issue.Labels = append(issue.Labels, "critical")
	if err := s.SaveIssue(issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	got2, err := s.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("get issue after save: %v", err)
	}
	if got2.Status != core.IssueStatusExecuting || got2.Version != 2 {
		t.Fatalf("issue update not persisted: got=%#v", got2)
	}
	if !reflect.DeepEqual(got2.Labels, issue.Labels) {
		t.Fatalf("issue labels mismatch after save: got=%#v want=%#v", got2.Labels, issue.Labels)
	}

	byRun, err := s.GetIssueByRun(Run.ID)
	if err != nil {
		t.Fatalf("get issue by Run: %v", err)
	}
	if byRun == nil || byRun.ID != issue.ID {
		t.Fatalf("expected issue %q by Run, got %#v", issue.ID, byRun)
	}
}

func TestRunRoundTrip_PersistsIssueID(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-Run-issue", Name: "pipe", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	p := &core.Run{
		ID:        "pipe-issue-1",
		ProjectID: project.ID,
		Name:      "Run-with-issue",
		Template:  "standard",
		Status:    core.StatusQueued,
		IssueID:   "issue-55667788-1",
		Stages:    []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		Artifacts: map[string]string{},
	}
	if err := s.SaveRun(p); err != nil {
		t.Fatalf("save Run: %v", err)
	}

	got, err := s.GetRun(p.ID)
	if err != nil {
		t.Fatalf("get Run: %v", err)
	}
	if got.IssueID != p.IssueID {
		t.Fatalf("Run issue_id mismatch: got=%q want=%q", got.IssueID, p.IssueID)
	}
}

func TestIssueListAndActiveIssues(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-issue-list", Name: "issue-list", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	issues := []*core.Issue{
		{
			ID:        "issue-20260302-list-a",
			ProjectID: project.ID,
			Title:     "A",
			Template:  "standard",
			State:     core.IssueStateOpen,
			Status:    core.IssueStatusExecuting,
		},
		{
			ID:        "issue-20260302-list-b",
			ProjectID: project.ID,
			Title:     "B",
			Template:  "standard",
			State:     core.IssueStateOpen,
			Status:    core.IssueStatusReviewing,
		},
		{
			ID:        "issue-20260302-list-c",
			ProjectID: project.ID,
			Title:     "C",
			Template:  "standard",
			State:     core.IssueStateClosed,
			Status:    core.IssueStatusDone,
		},
	}
	for _, issue := range issues {
		if err := s.CreateIssue(issue); err != nil {
			t.Fatalf("create issue %s: %v", issue.ID, err)
		}
	}

	filtered, total, err := s.ListIssues(project.ID, core.IssueFilter{
		Status: string(core.IssueStatusExecuting),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("list issues by status: %v", err)
	}
	if total != 1 || len(filtered) != 1 || filtered[0].ID != "issue-20260302-list-a" {
		t.Fatalf("unexpected status list result: total=%d issues=%#v", total, filtered)
	}

	paged, totalPaged, err := s.ListIssues(project.ID, core.IssueFilter{
		Limit:  1,
		Offset: 1,
	})
	if err != nil {
		t.Fatalf("list issues paged: %v", err)
	}
	if totalPaged != 3 || len(paged) != 1 {
		t.Fatalf("unexpected paged result: total=%d issues=%#v", totalPaged, paged)
	}

	active, err := s.GetActiveIssues(project.ID)
	if err != nil {
		t.Fatalf("get active issues: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active issues, got %d (%#v)", len(active), active)
	}
}

func TestIssueAttachmentAndChangeCRUD(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-issue-art", Name: "issue-art", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	issue := &core.Issue{
		ID:        "issue-20260302-artchg",
		ProjectID: project.ID,
		Title:     "附件与变更",
		Template:  "standard",
		Status:    core.IssueStatusDraft,
		State:     core.IssueStateOpen,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("create issue: %v", err)
	}

	if err := s.SaveIssueAttachment(issue.ID, "docs/spec.md", "spec v1"); err != nil {
		t.Fatalf("save attachment #1: %v", err)
	}
	if err := s.SaveIssueAttachment(issue.ID, "docs/test-plan.md", "test plan"); err != nil {
		t.Fatalf("save attachment #2: %v", err)
	}
	attachments, err := s.GetIssueAttachments(issue.ID)
	if err != nil {
		t.Fatalf("get attachments: %v", err)
	}
	if len(attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(attachments))
	}
	if attachments[0].Path != "docs/spec.md" || attachments[1].Path != "docs/test-plan.md" {
		t.Fatalf("attachments order/content mismatch: %#v", attachments)
	}

	change := &core.IssueChange{
		IssueID:   issue.ID,
		Field:     "status",
		OldValue:  "draft",
		NewValue:  "queued",
		Reason:    "review passed",
		ChangedBy: "scheduler",
	}
	if err := s.SaveIssueChange(change); err != nil {
		t.Fatalf("save issue change: %v", err)
	}
	changes, err := s.GetIssueChanges(issue.ID)
	if err != nil {
		t.Fatalf("get issue changes: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Field != "status" || changes[0].NewValue != "queued" {
		t.Fatalf("unexpected issue change: %#v", changes[0])
	}
}
