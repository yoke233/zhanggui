package engine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/eventbus"
	runtimeprocess "github.com/yoke233/ai-workflow/internal/plugins/runtime-process"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
	workspaceworktree "github.com/yoke233/ai-workflow/internal/plugins/workspace-worktree"
)

type fakeAgent struct {
	name     string
	buildFn  func(core.ExecOpts) ([]string, error)
	parserFn func(io.Reader) core.StreamParser

	mu      sync.Mutex
	options []core.ExecOpts
}

func (a *fakeAgent) Name() string { return a.name }
func (a *fakeAgent) Init(context.Context) error {
	return nil
}
func (a *fakeAgent) Close() error { return nil }
func (a *fakeAgent) BuildCommand(opts core.ExecOpts) ([]string, error) {
	a.mu.Lock()
	a.options = append(a.options, opts)
	a.mu.Unlock()
	if a.buildFn != nil {
		return a.buildFn(opts)
	}
	return []string{"noop"}, nil
}
func (a *fakeAgent) NewStreamParser(r io.Reader) core.StreamParser {
	if a.parserFn != nil {
		return a.parserFn(r)
	}
	return &eofParser{}
}

func (a *fakeAgent) lastPrompt() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.options) == 0 {
		return ""
	}
	return a.options[len(a.options)-1].Prompt
}

type fakeRuntime struct {
	waitResults []error
	calls       int
	workDirs    []string
}

func (r *fakeRuntime) Name() string { return "fake-runtime" }
func (r *fakeRuntime) Init(context.Context) error {
	return nil
}
func (r *fakeRuntime) Close() error { return nil }
func (r *fakeRuntime) Kill(string) error {
	return nil
}
func (r *fakeRuntime) Create(_ context.Context, opts core.RuntimeOpts) (*core.Session, error) {
	r.calls++
	r.workDirs = append(r.workDirs, opts.WorkDir)
	idx := r.calls - 1
	waitErr := error(nil)
	if idx < len(r.waitResults) {
		waitErr = r.waitResults[idx]
	}
	return &core.Session{
		ID:     "s",
		Stdin:  nopWriteCloser{},
		Stdout: strings.NewReader(""),
		Stderr: strings.NewReader(""),
		Wait: func() error {
			return waitErr
		},
	}, nil
}

type fakeWorkspace struct {
	setupErr   error
	cleanupErr error

	setupResult  core.WorkspaceSetupResult
	setupCalls   int
	cleanupCalls int
	setupReqs    []core.WorkspaceSetupRequest
	cleanupReqs  []core.WorkspaceCleanupRequest
}

func (w *fakeWorkspace) Name() string { return "fake-workspace" }
func (w *fakeWorkspace) Init(context.Context) error {
	return nil
}
func (w *fakeWorkspace) Close() error { return nil }
func (w *fakeWorkspace) Setup(_ context.Context, req core.WorkspaceSetupRequest) (core.WorkspaceSetupResult, error) {
	w.setupCalls++
	w.setupReqs = append(w.setupReqs, req)
	if w.setupErr != nil {
		return core.WorkspaceSetupResult{}, w.setupErr
	}
	return w.setupResult, nil
}
func (w *fakeWorkspace) Cleanup(_ context.Context, req core.WorkspaceCleanupRequest) error {
	w.cleanupCalls++
	w.cleanupReqs = append(w.cleanupReqs, req)
	return w.cleanupErr
}

type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }

type eofParser struct{}

func (*eofParser) Next() (*core.StreamEvent, error) { return nil, io.EOF }

type failOnceParser struct {
	err  error
	done bool
}

func (p *failOnceParser) Next() (*core.StreamEvent, error) {
	if p.done {
		return nil, io.EOF
	}
	p.done = true
	return nil, p.err
}

func newTestStore(t *testing.T) *storesqlite.SQLiteStore {
	t.Helper()
	s, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newExecutor(store core.Store, agents map[string]core.AgentPlugin, runtime core.RuntimePlugin) *Executor {
	return newExecutorWithBus(store, eventbus.New(), agents, runtime)
}

func newExecutorWithBus(store core.Store, bus *eventbus.Bus, agents map[string]core.AgentPlugin, runtime core.RuntimePlugin) *Executor {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	execEngine := NewExecutor(store, bus, agents, runtime, logger)
	if defaultAgent := defaultTestAgentName(agents); defaultAgent != "" {
		execEngine.SetRoleResolver(acpclient.NewRoleResolver(
			[]acpclient.AgentProfile{
				{
					ID: defaultAgent,
					CapabilitiesMax: acpclient.ClientCapabilities{
						FSRead:   true,
						FSWrite:  true,
						Terminal: true,
					},
				},
			},
			[]acpclient.RoleProfile{
				{
					ID:      "worker",
					AgentID: defaultAgent,
					Capabilities: acpclient.ClientCapabilities{
						FSRead:   true,
						FSWrite:  true,
						Terminal: true,
					},
				},
				{
					ID:      "reviewer",
					AgentID: defaultAgent,
					Capabilities: acpclient.ClientCapabilities{
						FSRead:   true,
						FSWrite:  true,
						Terminal: true,
					},
				},
			},
		))
	}
	return execEngine
}

func defaultTestAgentName(agents map[string]core.AgentPlugin) string {
	if plugin, ok := agents["codex"]; ok && plugin != nil {
		return "codex"
	}
	if plugin, ok := agents["claude"]; ok && plugin != nil {
		return "claude"
	}
	for rawName, plugin := range agents {
		name := strings.TrimSpace(rawName)
		if name == "" || plugin == nil {
			continue
		}
		return name
	}
	return ""
}

func setupProjectAndRun(t *testing.T, store core.Store, repoPath string, stages []core.StageConfig) *core.Run {
	t.Helper()

	normalizedStages := make([]core.StageConfig, len(stages))
	copy(normalizedStages, stages)
	for i := range normalizedStages {
		if strings.TrimSpace(normalizedStages[i].Role) != "" {
			continue
		}
		if !stageRequiresRole(normalizedStages[i].Name) {
			continue
		}
		normalizedStages[i].Role = defaultTestRoleForStage(normalizedStages[i].Name)
	}

	project := &core.Project{
		ID:       "proj-1",
		Name:     "proj",
		RepoPath: repoPath,
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	p := &core.Run{
		ID:              "20260228-pipe",
		ProjectID:       project.ID,
		Name:            "pipe",
		Description:     "需求A",
		Template:        "quick",
		Status:          core.StatusQueued,
		Stages:          normalizedStages,
		Artifacts:       map[string]string{},
		Config:          map[string]any{},
		MaxTotalRetries: 20,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func defaultTestRoleForStage(stage core.StageID) string {
	switch stage {
	case core.StageReview:
		return "reviewer"
	default:
		return "worker"
	}
}

func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@example.com"},
		{"git", "-C", dir, "config", "user.name", "test-user"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	}
	for _, cmd := range cmds {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("cmd %v failed: %s (%v)", cmd, string(out), err)
		}
	}
	return dir
}

func TestExecutor_Run_WorktreeMergeCleanupAndWorkDir(t *testing.T) {
	repo := setupGitRepo(t)
	store := newTestStore(t)
	defer store.Close()

	agent := &fakeAgent{
		name: "codex",
		buildFn: func(core.ExecOpts) ([]string, error) {
			return []string{"git", "commit", "--allow-empty", "-m", "feat-from-agent"}, nil
		},
		parserFn: func(io.Reader) core.StreamParser { return &eofParser{} },
	}

	p := setupProjectAndRun(t, store, repo, []core.StageConfig{
		{Name: core.StageSetup, OnFailure: core.OnFailureAbort},
		{Name: core.StageImplement, Agent: "codex", PromptTemplate: "implement", OnFailure: core.OnFailureAbort},
		{Name: core.StageMerge, OnFailure: core.OnFailureAbort},
		{Name: core.StageCleanup, OnFailure: core.OnFailureAbort},
	})

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtimeprocess.New())
	execEngine.SetWorkspace(workspaceworktree.New())
	if err := execEngine.Run(context.Background(), p.ID); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.BranchName == "" || got.WorktreePath == "" {
		t.Fatalf("worktree_setup must persist branch/worktree, got branch=%q worktree=%q", got.BranchName, got.WorktreePath)
	}
	if _, err := os.Stat(got.WorktreePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup stage must remove worktree path, stat err=%v", err)
	}

	logOut, err := exec.Command("git", "-C", repo, "log", "--oneline", "-n", "20").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logOut), "feat-from-agent") {
		t.Fatalf("merge stage did not bring feature commit into base branch: %s", string(logOut))
	}
}

func TestExecutor_Run_WorktreeStagesUseWorkspacePlugin(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	repoPath := t.TempDir()
	workspace := &fakeWorkspace{
		setupResult: core.WorkspaceSetupResult{
			BranchName:   "ai-flow/20260228-pipe",
			WorktreePath: filepath.Join(repoPath, ".worktrees", "20260228-pipe"),
			BaseBranch:   "main",
		},
	}

	p := setupProjectAndRun(t, store, repoPath, []core.StageConfig{
		{Name: core.StageSetup, OnFailure: core.OnFailureAbort},
		{Name: core.StageCleanup, OnFailure: core.OnFailureAbort},
	})

	execEngine := newExecutor(store, map[string]core.AgentPlugin{}, &fakeRuntime{})
	execEngine.SetWorkspace(workspace)
	if err := execEngine.Run(context.Background(), p.ID); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if workspace.setupCalls != 1 {
		t.Fatalf("expected setup called once, got %d", workspace.setupCalls)
	}
	if workspace.cleanupCalls != 1 {
		t.Fatalf("expected cleanup called once, got %d", workspace.cleanupCalls)
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.BranchName != workspace.setupResult.BranchName {
		t.Fatalf("Run branch mismatch, got=%q want=%q", got.BranchName, workspace.setupResult.BranchName)
	}
	if got.WorktreePath != workspace.setupResult.WorktreePath {
		t.Fatalf("Run worktree mismatch, got=%q want=%q", got.WorktreePath, workspace.setupResult.WorktreePath)
	}
	if baseBranch, _ := got.Config["base_branch"].(string); baseBranch != workspace.setupResult.BaseBranch {
		t.Fatalf("Run base branch mismatch, got=%q want=%q", baseBranch, workspace.setupResult.BaseBranch)
	}
}

func TestExecutor_Run_OnFailureRetryAndMaxRetries(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{
		errors.New("boom-1"),
		errors.New("boom-2"),
		nil,
	}}
	agent := &fakeAgent{name: "codex"}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{
			Name:         core.StageImplement,
			Agent:        "codex",
			OnFailure:    core.OnFailureRetry,
			MaxRetries:   2,
			Timeout:      0,
			RequireHuman: false,
		},
	})
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	if err := execEngine.Run(context.Background(), p.ID); err != nil {
		t.Fatalf("expected retry to eventually succeed, got err: %v", err)
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.Conclusion != core.ConclusionSuccess {
		t.Fatalf("expected success conclusion, got %s", got.Conclusion)
	}
	if runtime.calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", runtime.calls)
	}
}

func TestExecutor_Run_OnFailureSkip(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{
		errors.New("first-stage-fail"),
		nil,
	}}
	agent := &fakeAgent{name: "codex"}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{Name: core.StageImplement, Agent: "codex", OnFailure: core.OnFailureSkip, MaxRetries: 0},
		{Name: core.StageFixup, Agent: "codex", OnFailure: core.OnFailureAbort, MaxRetries: 0},
	})
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	if err := execEngine.Run(context.Background(), p.ID); err != nil {
		t.Fatalf("skip should continue to next stage, got err: %v", err)
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.Conclusion != core.ConclusionSuccess {
		t.Fatalf("expected success conclusion, got %s", got.Conclusion)
	}
	if runtime.calls != 2 {
		t.Fatalf("expected two stage executions, got %d", runtime.calls)
	}
}

func TestExecutor_Run_OnFailureHuman(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{errors.New("need-human")}}
	agent := &fakeAgent{name: "codex"}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{Name: core.StageImplement, Agent: "codex", OnFailure: core.OnFailureHuman, MaxRetries: 0},
	})
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	if err := execEngine.Run(context.Background(), p.ID); err != nil {
		t.Fatalf("human gate should pause Run, got err: %v", err)
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusActionRequired {
		t.Fatalf("expected waiting_review, got %s", got.Status)
	}
}

func TestExecutor_Run_OnFailureAbort(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{errors.New("fatal")}}
	agent := &fakeAgent{name: "codex"}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{Name: core.StageImplement, Agent: "codex", OnFailure: core.OnFailureAbort, MaxRetries: 0},
	})
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	if err := execEngine.Run(context.Background(), p.ID); err == nil {
		t.Fatal("abort should return error")
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.Conclusion != core.ConclusionFailure {
		t.Fatalf("expected failure conclusion, got %s", got.Conclusion)
	}
}

func TestExecutor_Run_ParserErrorShouldFailStage(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{nil}}
	agent := &fakeAgent{
		name: "codex",
		parserFn: func(io.Reader) core.StreamParser {
			return &failOnceParser{err: errors.New("bad-stream")}
		},
	}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{Name: core.StageImplement, Agent: "codex", OnFailure: core.OnFailureAbort, MaxRetries: 0},
	})
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	err := execEngine.Run(context.Background(), p.ID)
	if err == nil {
		t.Fatal("expected parser error to fail stage")
	}
	if !strings.Contains(err.Error(), "bad-stream") {
		t.Fatalf("expected parser error in run error, got: %v", err)
	}
}

func TestExecutor_Run_AgentPromptFromTemplate(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{nil}}
	agent := &fakeAgent{name: "codex"}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{
			Name:           core.StageImplement,
			Agent:          "codex",
			PromptTemplate: "implement",
			OnFailure:      core.OnFailureAbort,
			MaxRetries:     0,
		},
	})
	p.WorktreePath = workDir
	p.Description = "这里是需求文本XYZ"
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	if err := execEngine.Run(context.Background(), p.ID); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	prompt := agent.lastPrompt()
	if !strings.Contains(prompt, "这里是需求文本XYZ") {
		t.Fatalf("prompt should contain requirements from Run description, got: %s", prompt)
	}
	if !strings.Contains(prompt, "请根据以下需求实现代码") {
		t.Fatalf("prompt should come from implement template, got: %s", prompt)
	}
	for _, required := range []string{
		".ai-workflow/issue_plan.md",
		".ai-workflow/progress.md",
		".ai-workflow/findings.md",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt should include execution documentation instruction %q, got: %s", required, prompt)
		}
	}
	if len(runtime.workDirs) == 0 || runtime.workDirs[0] != workDir {
		t.Fatalf("runtime should execute in worktree dir %q, got %v", workDir, runtime.workDirs)
	}
}

func TestExecuteStageByRole(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{nil}}
	agent := &fakeAgent{name: "codex"}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{
			Name:           core.StageImplement,
			Role:           "worker",
			PromptTemplate: "implement",
			OnFailure:      core.OnFailureAbort,
			MaxRetries:     0,
		},
	})
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	resolver := acpclient.NewRoleResolver(
		[]acpclient.AgentProfile{
			{
				ID: "codex",
				CapabilitiesMax: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
		[]acpclient.RoleProfile{
			{
				ID:      "worker",
				AgentID: "codex",
				Capabilities: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
	)

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	execEngine.SetRoleResolver(resolver)
	if err := execEngine.Run(context.Background(), p.ID); err != nil {
		t.Fatalf("run by role failed: %v", err)
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.Conclusion != core.ConclusionSuccess {
		t.Fatalf("expected success conclusion, got %s", got.Conclusion)
	}
	if runtime.calls != 1 {
		t.Fatalf("expected one stage execution, got %d", runtime.calls)
	}

	checkpoints, err := store.GetCheckpoints(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoints) == 0 {
		t.Fatal("expected checkpoints to be persisted")
	}
	last := checkpoints[len(checkpoints)-1]
	if last.AgentUsed != "codex" {
		t.Fatalf("expected checkpoint agent_used codex, got %q", last.AgentUsed)
	}
}

func TestExecuteStageByRole_MissingRoleFails(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{nil}}
	agent := &fakeAgent{name: "codex"}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{
			Name:       core.StageImplement,
			Role:       "missing-role",
			OnFailure:  core.OnFailureAbort,
			MaxRetries: 0,
		},
	})
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	resolver := acpclient.NewRoleResolver(
		[]acpclient.AgentProfile{
			{
				ID: "codex",
				CapabilitiesMax: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
		[]acpclient.RoleProfile{
			{
				ID:      "worker",
				AgentID: "codex",
				Capabilities: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
	)

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	execEngine.SetRoleResolver(resolver)

	err := execEngine.Run(context.Background(), p.ID)
	if err == nil {
		t.Fatal("expected missing role to fail Run run")
	}
	if !strings.Contains(err.Error(), "role not found") {
		t.Fatalf("expected role resolution failure, got %v", err)
	}
	if runtime.calls != 0 {
		t.Fatalf("expected runtime not to start session on missing role, got calls=%d", runtime.calls)
	}
}

func TestExecuteStageByRole_EmptyRoleFails(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{nil}}
	agent := &fakeAgent{name: "codex"}

	project := &core.Project{
		ID:       "proj-empty-role",
		Name:     "proj-empty-role",
		RepoPath: workDir,
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	p := &core.Run{
		ID:           "pipe-empty-role",
		ProjectID:    project.ID,
		Name:         "pipe-empty-role",
		Template:     "quick",
		Status:       core.StatusQueued,
		CurrentStage: core.StageImplement,
		Stages: []core.StageConfig{
			{
				Name:       core.StageImplement,
				Agent:      "codex",
				OnFailure:  core.OnFailureAbort,
				MaxRetries: 0,
			},
		},
		Artifacts:       map[string]string{},
		Config:          map[string]any{},
		WorktreePath:    workDir,
		MaxTotalRetries: 5,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	resolver := acpclient.NewRoleResolver(
		[]acpclient.AgentProfile{
			{
				ID: "codex",
				CapabilitiesMax: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
		[]acpclient.RoleProfile{
			{
				ID:      "worker",
				AgentID: "codex",
				Capabilities: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
	)

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	execEngine.SetRoleResolver(resolver)

	err := execEngine.Run(context.Background(), p.ID)
	if err == nil {
		t.Fatal("expected empty stage role to fail Run run")
	}
	if !strings.Contains(err.Error(), "stage role is required") {
		t.Fatalf("expected missing stage role failure, got %v", err)
	}
	if runtime.calls != 0 {
		t.Fatalf("expected runtime not to start session on empty role, got calls=%d", runtime.calls)
	}
}

func TestExecuteStageByRole_EmptyRoleDoesNotFallbackToStageAgent(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{nil}}
	agent := &fakeAgent{name: "codex"}

	project := &core.Project{
		ID:       "proj-no-fallback",
		Name:     "proj-no-fallback",
		RepoPath: workDir,
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	p := &core.Run{
		ID:           "pipe-no-fallback",
		ProjectID:    project.ID,
		Name:         "pipe-no-fallback",
		Template:     "quick",
		Status:       core.StatusQueued,
		CurrentStage: core.StageImplement,
		Stages: []core.StageConfig{
			{
				Name:       core.StageImplement,
				Agent:      "legacy-agent",
				OnFailure:  core.OnFailureAbort,
				MaxRetries: 0,
			},
		},
		Artifacts:       map[string]string{},
		Config:          map[string]any{},
		WorktreePath:    workDir,
		MaxTotalRetries: 5,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	resolver := acpclient.NewRoleResolver(
		[]acpclient.AgentProfile{
			{
				ID: "codex",
				CapabilitiesMax: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
		[]acpclient.RoleProfile{
			{
				ID:      "worker",
				AgentID: "codex",
				Capabilities: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
	)

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	execEngine.SetRoleResolver(resolver)

	err := execEngine.Run(context.Background(), p.ID)
	if err == nil {
		t.Fatal("expected empty stage role to fail Run run")
	}

	checkpoints, cpErr := store.GetCheckpoints(p.ID)
	if cpErr != nil {
		t.Fatalf("get checkpoints: %v", cpErr)
	}
	if len(checkpoints) == 0 {
		t.Fatal("expected checkpoint to be persisted on failed stage")
	}
	for _, cp := range checkpoints {
		if cp.AgentUsed == "legacy-agent" {
			t.Fatalf("stage.agent fallback should be removed, got checkpoint agent_used=%q", cp.AgentUsed)
		}
	}
}

func TestExecuteStageByRole_MissingResolverFails(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{nil}}
	agent := &fakeAgent{name: "codex"}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{
			Name:       core.StageImplement,
			Role:       "worker",
			OnFailure:  core.OnFailureAbort,
			MaxRetries: 0,
		},
	})
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	execEngine.SetRoleResolver(nil)

	err := execEngine.Run(context.Background(), p.ID)
	if err == nil {
		t.Fatal("expected missing role resolver to fail Run run")
	}
	if !strings.Contains(err.Error(), "role resolver is not configured") {
		t.Fatalf("expected missing resolver failure, got %v", err)
	}
	if runtime.calls != 0 {
		t.Fatalf("expected runtime not to start session without resolver, got calls=%d", runtime.calls)
	}
}

// --- ACP Session Pool Tests ---

func TestACPPool_PutGetCleanup(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	e := newExecutor(store, map[string]core.AgentPlugin{"codex": &fakeAgent{name: "codex"}}, &fakeRuntime{})

	entry := &acpSessionEntry{sessionID: "sid-1"}
	e.acpPoolPut("run-1", core.StageImplement, entry)

	// Get returns the stored entry.
	got := e.acpPoolGet("run-1", core.StageImplement)
	if got != entry {
		t.Fatalf("acpPoolGet returned %v, want %v", got, entry)
	}

	// Get with wrong stage returns nil.
	if e.acpPoolGet("run-1", core.StageReview) != nil {
		t.Fatal("expected nil for different stage")
	}

	// Get with wrong run returns nil.
	if e.acpPoolGet("run-2", core.StageImplement) != nil {
		t.Fatal("expected nil for different run")
	}

	// Cleanup removes entries for the run.
	e.acpPoolCleanup("run-1")
	if e.acpPoolGet("run-1", core.StageImplement) != nil {
		t.Fatal("expected nil after cleanup")
	}
}

func TestACPPool_CleanupDoesNotAffectOtherRuns(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	e := newExecutor(store, map[string]core.AgentPlugin{"codex": &fakeAgent{name: "codex"}}, &fakeRuntime{})

	entry1 := &acpSessionEntry{sessionID: "sid-1"}
	entry2 := &acpSessionEntry{sessionID: "sid-2"}
	e.acpPoolPut("run-1", core.StageImplement, entry1)
	e.acpPoolPut("run-2", core.StageImplement, entry2)

	e.acpPoolCleanup("run-1")

	if e.acpPoolGet("run-2", core.StageImplement) != entry2 {
		t.Fatal("cleanup of run-1 should not affect run-2")
	}
}

func TestDefaultStageConfig_FixupReusesImplementSession(t *testing.T) {
	cfg := defaultStageConfig(core.StageFixup)
	if cfg.ReuseSessionFrom != core.StageImplement {
		t.Fatalf("fixup ReuseSessionFrom = %q, want %q", cfg.ReuseSessionFrom, core.StageImplement)
	}
}

func TestDefaultStageConfig_ImplementNoReuse(t *testing.T) {
	cfg := defaultStageConfig(core.StageImplement)
	if cfg.ReuseSessionFrom != "" {
		t.Fatalf("implement ReuseSessionFrom = %q, want empty", cfg.ReuseSessionFrom)
	}
}

func TestDefaultStageConfig_ReviewNoReuse(t *testing.T) {
	cfg := defaultStageConfig(core.StageReview)
	if cfg.ReuseSessionFrom != "" {
		t.Fatalf("review ReuseSessionFrom = %q, want empty", cfg.ReuseSessionFrom)
	}
}
