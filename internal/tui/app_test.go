package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yoke233/ai-workflow/internal/core"
)

type noopExecutor struct{}

func (noopExecutor) CreateRun(projectID, name, description, template string) (*core.Run, error) {
	return &core.Run{}, nil
}

func (noopExecutor) Run(ctx context.Context, RunID string) error {
	return nil
}

func (noopExecutor) ApplyAction(ctx context.Context, action core.RunAction) error {
	return nil
}

type noopStore struct{}

func (noopStore) ListProjects(filter core.ProjectFilter) ([]core.Project, error) {
	return nil, nil
}

func (noopStore) GetProject(id string) (*core.Project, error) {
	return nil, nil
}

func (noopStore) CreateProject(p *core.Project) error {
	return nil
}

func (noopStore) UpdateProject(p *core.Project) error {
	return nil
}

func (noopStore) DeleteProject(id string) error {
	return nil
}

func (noopStore) ListRuns(projectID string, filter core.RunFilter) ([]core.Run, error) {
	return nil, nil
}

func (noopStore) GetRun(id string) (*core.Run, error) {
	return nil, nil
}

func (noopStore) SaveRun(p *core.Run) error {
	return nil
}

func (noopStore) GetActiveRuns() ([]core.Run, error) {
	return nil, nil
}

func (noopStore) ListRunnableRuns(limit int) ([]core.Run, error) {
	return nil, nil
}

func (noopStore) CountRunningRunsByProject(projectID string) (int, error) {
	return 0, nil
}

func (noopStore) TryMarkRunRunning(id string, from ...core.RunStatus) (bool, error) {
	return false, nil
}

func (noopStore) SaveCheckpoint(cp *core.Checkpoint) error {
	return nil
}

func (noopStore) GetCheckpoints(RunID string) ([]core.Checkpoint, error) {
	return nil, nil
}

func (noopStore) GetLastSuccessCheckpoint(RunID string) (*core.Checkpoint, error) {
	return nil, nil
}

func (noopStore) InvalidateCheckpointsFromStage(RunID string, stage core.StageID) error {
	return nil
}

func (noopStore) AppendLog(entry core.LogEntry) error {
	return nil
}

func (noopStore) GetLogs(RunID string, stage string, limit int, offset int) ([]core.LogEntry, int, error) {
	return nil, 0, nil
}

func (noopStore) RecordAction(action core.HumanAction) error {
	return nil
}

func (noopStore) GetActions(RunID string) ([]core.HumanAction, error) {
	return nil, nil
}

func (noopStore) CreateChatSession(s *core.ChatSession) error {
	return nil
}

func (noopStore) GetChatSession(id string) (*core.ChatSession, error) {
	return nil, nil
}

func (noopStore) UpdateChatSession(s *core.ChatSession) error {
	return nil
}

func (noopStore) ListChatSessions(projectID string) ([]core.ChatSession, error) {
	return nil, nil
}

func (noopStore) CreateIssue(i *core.Issue) error {
	return nil
}

func (noopStore) GetIssue(id string) (*core.Issue, error) {
	return nil, nil
}

func (noopStore) SaveIssue(i *core.Issue) error {
	return nil
}

func (noopStore) ListIssues(projectID string, filter core.IssueFilter) ([]core.Issue, int, error) {
	return nil, 0, nil
}

func (noopStore) GetActiveIssues(projectID string) ([]core.Issue, error) {
	return nil, nil
}

func (noopStore) GetIssueByRun(RunID string) (*core.Issue, error) {
	return nil, nil
}

func (noopStore) SaveIssueAttachment(issueID, path, content string) error {
	return nil
}

func (noopStore) GetIssueAttachments(issueID string) ([]core.IssueAttachment, error) {
	return nil, nil
}

func (noopStore) SaveIssueChange(change *core.IssueChange) error {
	return nil
}

func (noopStore) GetIssueChanges(issueID string) ([]core.IssueChange, error) {
	return nil, nil
}

func (noopStore) SaveReviewRecord(r *core.ReviewRecord) error {
	return nil
}

func (noopStore) GetReviewRecords(issueID string) ([]core.ReviewRecord, error) {
	return nil, nil
}

func (noopStore) Close() error {
	return nil
}

type createSpyStore struct {
	noopStore
	created []core.Project
}

func (s *createSpyStore) CreateProject(p *core.Project) error {
	s.created = append(s.created, *p)
	return nil
}

type createFailStore struct {
	noopStore
	err error
}

func (s *createFailStore) CreateProject(p *core.Project) error {
	return s.err
}

func TestSplitArgsQuoted(t *testing.T) {
	args, err := splitArgs(`Run create demo auth "实现 登录 与 注册" quick`)
	if err != nil {
		t.Fatalf("split args failed: %v", err)
	}

	want := []string{"Run", "create", "demo", "auth", "实现 登录 与 注册", "quick"}
	if len(args) != len(want) {
		t.Fatalf("unexpected args length: got=%d want=%d (%v)", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("arg[%d] mismatch: got=%q want=%q", i, args[i], want[i])
		}
	}
}

func TestSplitArgsUnclosedQuote(t *testing.T) {
	_, err := splitArgs(`Run create demo auth "bad`)
	if err == nil {
		t.Fatal("expected unclosed quote error, got nil")
	}
}

func TestRunCommandHelp(t *testing.T) {
	out, err := runCommand(context.Background(), noopStore{}, noopExecutor{}, "help")
	if err != nil {
		t.Fatalf("help command failed: %v", err)
	}
	if !strings.Contains(out, "/Run start <Run-id>") {
		t.Fatalf("help output missing Run start command: %s", out)
	}
}

func TestResolveChatInputSingleProject(t *testing.T) {
	msg, proj, err := resolveChatInput("请整理需求", []core.Project{
		{ID: "demo", RepoPath: "D:/repo/demo"},
	}, "D:/repo/any")
	if err != nil {
		t.Fatalf("resolve chat input failed: %v", err)
	}
	if msg != "请整理需求" {
		t.Fatalf("unexpected message: %s", msg)
	}
	if proj.ID != "demo" {
		t.Fatalf("unexpected project: %s", proj.ID)
	}
}

func TestResolveChatInputMultipleProjectsNeedPrefix(t *testing.T) {
	_, _, err := resolveChatInput("请整理需求", []core.Project{
		{ID: "a", RepoPath: "D:/repo/a"},
		{ID: "b", RepoPath: "D:/repo/b"},
	}, "D:/repo/unknown")
	if err == nil {
		t.Fatal("expected error when multiple projects and no @prefix")
	}
}

func TestResolveChatInputWithPrefix(t *testing.T) {
	msg, proj, err := resolveChatInput("@b 请整理需求", []core.Project{
		{ID: "a", RepoPath: "D:/repo/a"},
		{ID: "b", RepoPath: "D:/repo/b"},
	}, "D:/repo/a")
	if err != nil {
		t.Fatalf("resolve prefixed chat input failed: %v", err)
	}
	if msg != "请整理需求" {
		t.Fatalf("unexpected message: %s", msg)
	}
	if proj.ID != "b" {
		t.Fatalf("unexpected project: %s", proj.ID)
	}
}

func TestResolveChatInputAutoInferByDir(t *testing.T) {
	msg, proj, err := resolveChatInput("讨论需求", []core.Project{
		{ID: "a", RepoPath: "D:/repo/a"},
		{ID: "b", RepoPath: "D:/repo/b"},
	}, "D:/repo/b/service/api")
	if err != nil {
		t.Fatalf("resolve auto infer failed: %v", err)
	}
	if msg != "讨论需求" {
		t.Fatalf("unexpected message: %s", msg)
	}
	if proj.ID != "b" {
		t.Fatalf("expected inferred project b, got %s", proj.ID)
	}
}

func TestResolveChatInputWithSelectionPrefersSelectedProject(t *testing.T) {
	msg, proj, autoMatched, err := resolveChatInputWithSelection("讨论需求", []core.Project{
		{ID: "a", RepoPath: "D:/repo/a"},
		{ID: "b", RepoPath: "D:/repo/b"},
	}, "D:/repo/a/service", "b")
	if err != nil {
		t.Fatalf("resolve with selected project failed: %v", err)
	}
	if msg != "讨论需求" {
		t.Fatalf("unexpected message: %s", msg)
	}
	if proj.ID != "b" {
		t.Fatalf("expected selected project b, got %s", proj.ID)
	}
	if !autoMatched {
		t.Fatal("expected autoMatched=true")
	}
}

func TestResolveChatInputUnknownPrefixFallbackToDir(t *testing.T) {
	msg, proj, err := resolveChatInput("@demo 讨论需求", []core.Project{
		{ID: "a", RepoPath: "D:/repo/a"},
		{ID: "b", RepoPath: "D:/repo/b"},
	}, "D:/repo/a")
	if err != nil {
		t.Fatalf("resolve fallback failed: %v", err)
	}
	if msg != "讨论需求" {
		t.Fatalf("unexpected message: %s", msg)
	}
	if proj.ID != "a" {
		t.Fatalf("expected inferred project a, got %s", proj.ID)
	}
}

func TestEnsureProjectForWorkDirCreatesDefaultProject(t *testing.T) {
	store := &createSpyStore{}
	proj, created, err := ensureProjectForWorkDir(store, []core.Project{
		{ID: "demo", RepoPath: "D:/repo/demo"},
	}, "D:/project/ai-workflow")
	if err != nil {
		t.Fatalf("ensure project failed: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}
	if proj.ID != "ai-workflow" {
		t.Fatalf("expected id ai-workflow, got %s", proj.ID)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected one create call, got %d", len(store.created))
	}
}

func TestEnsureProjectForWorkDirCreateWithSuffixWhenIDExists(t *testing.T) {
	store := &createSpyStore{}
	proj, created, err := ensureProjectForWorkDir(store, []core.Project{
		{ID: "ai-workflow", RepoPath: "D:/other/path"},
	}, "D:/project/ai-workflow")
	if err != nil {
		t.Fatalf("ensure project failed: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}
	if proj.ID != "ai-workflow-2" {
		t.Fatalf("expected id ai-workflow-2, got %s", proj.ID)
	}
}

func TestCanAttemptAutoCreateProject(t *testing.T) {
	if !canAttemptAutoCreateProject("讨论需求") {
		t.Fatal("expected plain message to allow auto-create")
	}
	if canAttemptAutoCreateProject("@demo") {
		t.Fatal("expected malformed @prefix to block auto-create")
	}
	if !canAttemptAutoCreateProject("@demo 讨论需求") {
		t.Fatal("expected valid @prefix to allow auto-create")
	}
}

func TestTUI_SlashClearAlias(t *testing.T) {
	m := NewModel(noopExecutor{}, noopStore{}, nil, nil)
	m.history = []string{"[10:00:00] old"}
	m.input = "/clear"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	after := updated.(Model)
	if len(after.history) != 0 {
		t.Fatalf("expected history cleared, got: %v", after.history)
	}
	if after.running {
		t.Fatal("expected running=false after /clear")
	}
}

func TestTUI_AutoCreateProjectErrorSurfaced(t *testing.T) {
	store := &createFailStore{err: errors.New("db down")}
	m := NewModel(noopExecutor{}, store, nil, nil)
	m.workDir = "D:/repo/new-proj"
	m.input = "讨论需求"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	after := updated.(Model)
	historyText := strings.Join(after.history, "\n")
	if !strings.Contains(historyText, "自动创建项目失败") {
		t.Fatalf("expected auto-create failure in history, got: %s", historyText)
	}
	if !strings.Contains(historyText, "db down") {
		t.Fatalf("expected underlying create error in history, got: %s", historyText)
	}
}

type actionSpyExecutor struct {
	noopExecutor
	lastAction core.RunAction
}

func (s *actionSpyExecutor) ApplyAction(ctx context.Context, action core.RunAction) error {
	s.lastAction = action
	return nil
}

func TestTUI_ProjectSwitchChangesRunContext(t *testing.T) {
	m := NewModel(noopExecutor{}, noopStore{}, nil, nil)
	m.projects = []core.Project{
		{ID: "a", RepoPath: "D:/repo/a"},
		{ID: "b", RepoPath: "D:/repo/b"},
	}
	m.Runs = []core.Run{
		{ID: "pipe-a", ProjectID: "a", Name: "A", Status: core.StatusCreated},
		{ID: "pipe-b", ProjectID: "b", Name: "B", Status: core.StatusCreated},
	}
	m.syncProjectSelection()

	viewA := m.View()
	if !strings.Contains(viewA, "pipe-a") {
		t.Fatalf("expected project a Run visible, got: %s", viewA)
	}
	if strings.Contains(viewA, "pipe-b") {
		t.Fatalf("expected project b Run hidden before switch, got: %s", viewA)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	after := updated.(Model).View()
	if !strings.Contains(after, "pipe-b") {
		t.Fatalf("expected project b Run visible after switch, got: %s", after)
	}
}

func TestTUI_ActionApproveCommand(t *testing.T) {
	spy := &actionSpyExecutor{}
	out, err := runCommand(context.Background(), noopStore{}, spy, "Run action p-1 approve --message 已通过")
	if err != nil {
		t.Fatalf("action command failed: %v", err)
	}
	if !strings.Contains(out, "Action applied") {
		t.Fatalf("expected action output, got: %s", out)
	}
	if spy.lastAction.RunID != "p-1" || spy.lastAction.Type != core.ActionApprove {
		t.Fatalf("unexpected action parsed: %+v", spy.lastAction)
	}
}
