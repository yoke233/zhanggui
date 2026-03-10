package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
	pluginfactory "github.com/yoke233/ai-workflow/internal/plugins/factory"
	"github.com/yoke233/ai-workflow/internal/teamleader"
	v2sandbox "github.com/yoke233/ai-workflow/internal/v2/sandbox"
	"github.com/yoke233/ai-workflow/internal/web"
)

var stdoutCaptureMu sync.Mutex

func TestCLI_HiCommand(t *testing.T) {
	var runErr error
	output := captureStdout(t, func() {
		runErr = runWithArgs([]string{"hi"})
	})
	if runErr != nil {
		t.Fatalf("runWithArgs(hi) error = %v", runErr)
	}

	if output != "hi\n" {
		t.Fatalf("hi command output = %q, want %q", output, "hi\n")
	}
}

func TestCaptureStdout_RestoresStdoutAfterPanic(t *testing.T) {
	originalStdout := os.Stdout

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic from captureStdout callback")
		}
		if recovered != "boom" {
			t.Fatalf("panic value = %#v, want %q", recovered, "boom")
		}
		if os.Stdout != originalStdout {
			t.Fatal("expected stdout to be restored after panic")
		}
	}()

	_ = captureStdout(t, func() {
		_, _ = os.Stdout.WriteString("partial output")
		panic("boom")
	})
}

func TestCLI_RunActionCommand(t *testing.T) {
	err := runWithArgs([]string{"Run", "action"})
	if err == nil {
		t.Fatal("expected usage error for missing Run action args")
	}
	if !strings.Contains(err.Error(), "usage: ai-flow Run action") {
		t.Fatalf("expected Run action usage error, got %v", err)
	}
}

func TestCLI_SchedulerCommand(t *testing.T) {
	err := runWithArgs([]string{"scheduler"})
	if err == nil {
		t.Fatal("expected usage error for missing scheduler subcommand")
	}
	if !strings.Contains(err.Error(), "usage: ai-flow scheduler <run|once>") {
		t.Fatalf("expected scheduler usage error, got %v", err)
	}
}

func TestCLI_ProjectScanCommand(t *testing.T) {
	err := runWithArgs([]string{"project", "scan"})
	if err == nil {
		t.Fatal("expected usage error for missing project scan root")
	}
	if !strings.Contains(err.Error(), "usage: ai-flow project scan <root>") {
		t.Fatalf("expected project scan usage error, got %v", err)
	}
}

func TestCLI_ServerCommandUsageError(t *testing.T) {
	err := runServer(context.Background(), []string{"--port"})
	if err == nil {
		t.Fatal("expected usage error for missing server port value")
	}
	if !strings.Contains(err.Error(), "usage: ai-flow server [--port <port>]") {
		t.Fatalf("expected server usage error, got %v", err)
	}
}

func TestCLI_ServerCommandRoute(t *testing.T) {
	err := runWithArgs([]string{"server", "--port"})
	if err == nil {
		t.Fatal("expected usage error for missing server port value via runWithArgs")
	}
	if !strings.Contains(err.Error(), "usage: ai-flow server [--port <port>]") {
		t.Fatalf("expected server usage error via runWithArgs, got %v", err)
	}
}

func TestResolveServerPortPriority(t *testing.T) {
	tests := []struct {
		name    string
		cliPort int
		cfgPort int
		want    int
	}{
		{
			name:    "cli port overrides config port",
			cliPort: 18080,
			cfgPort: 28080,
			want:    18080,
		},
		{
			name:    "config port used when cli port absent",
			cliPort: 0,
			cfgPort: 28080,
			want:    28080,
		},
		{
			name:    "default port used when cli and config absent",
			cliPort: 0,
			cfgPort: 0,
			want:    8080,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveServerPort(tt.cliPort, tt.cfgPort)
			if got != tt.want {
				t.Fatalf("resolveServerPort(%d, %d) = %d, want %d", tt.cliPort, tt.cfgPort, got, tt.want)
			}
		})
	}
}

func TestResolveServerFrontendFS_UsesOverrideDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("override-index"), 0o644); err != nil {
		t.Fatalf("write override index: %v", err)
	}
	t.Setenv(frontendDirEnvVar, dir)

	frontendFS, err := resolveServerFrontendFS()
	if err != nil {
		t.Fatalf("resolveServerFrontendFS() error = %v, want nil", err)
	}
	if frontendFS == nil {
		t.Fatal("resolveServerFrontendFS() = nil, want non-nil frontend fs")
	}

	indexContent := mustReadFrontendIndex(t, frontendFS)
	if !strings.Contains(indexContent, "override-index") {
		t.Fatalf("override frontend index mismatch, got %q", indexContent)
	}
}

func TestResolveServerFrontendFS_FallsBackToRepoDist(t *testing.T) {
	if _, err := os.Stat(defaultFrontendDir); err == nil {
		t.Skipf("default frontend dir %q exists on host; skip repo fallback assertion", defaultFrontendDir)
	}

	repoRoot := t.TempDir()
	distDir := filepath.Join(repoRoot, "web", "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("mkdir dist dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("repo-index"), 0o644); err != nil {
		t.Fatalf("write repo dist index: %v", err)
	}
	t.Chdir(repoRoot)
	t.Setenv(frontendDirEnvVar, "")

	frontendFS, err := resolveServerFrontendFS()
	if err != nil {
		t.Fatalf("resolveServerFrontendFS() error = %v, want nil", err)
	}
	if frontendFS == nil {
		t.Fatal("resolveServerFrontendFS() = nil, want repo dist frontend fs")
	}

	indexContent := mustReadFrontendIndex(t, frontendFS)
	if !strings.Contains(indexContent, "repo-index") {
		t.Fatalf("repo dist frontend index mismatch, got %q", indexContent)
	}
}

func TestResolveServerFrontendFS_ReturnsNilWhenNoFrontendFound(t *testing.T) {
	if _, err := os.Stat(defaultFrontendDir); err == nil {
		t.Skipf("default frontend dir %q exists on host; skip nil assertion", defaultFrontendDir)
	}

	repoRoot := t.TempDir()
	t.Chdir(repoRoot)
	t.Setenv(frontendDirEnvVar, "")

	frontendFS, err := resolveServerFrontendFS()
	if err != nil {
		t.Fatalf("resolveServerFrontendFS() error = %v, want nil", err)
	}
	if frontendFS != nil {
		t.Fatal("resolveServerFrontendFS() returned frontend fs, want nil when no frontend dir exists")
	}
}

func TestResolveServerFrontendFS_OverrideMissingReturnsError(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "missing-dist")
	t.Setenv(frontendDirEnvVar, missingDir)

	frontendFS, err := resolveServerFrontendFS()
	if err == nil {
		t.Fatal("resolveServerFrontendFS() error = nil, want not-exist error for missing override dir")
	}
	if frontendFS != nil {
		t.Fatal("resolveServerFrontendFS() frontend fs should be nil when override dir is missing")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing override error to wrap os.ErrNotExist, got %v", err)
	}
}

func TestRunServer_PortPriority(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		envPort   string
		wantAddr  string
		unsetPort bool
	}{
		{
			name:     "cli port overrides config env port",
			args:     []string{"--port", "18080"},
			envPort:  "28080",
			wantAddr: "127.0.0.1:18080",
		},
		{
			name:     "config env port used when cli absent",
			args:     nil,
			envPort:  "28080",
			wantAddr: "127.0.0.1:28080",
		},
		{
			name:      "default port used when cli and config env absent",
			args:      nil,
			wantAddr:  "127.0.0.1:8080",
			unsetPort: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempHome := t.TempDir()
			t.Setenv("HOME", tempHome)
			t.Setenv("USERPROFILE", tempHome)
			t.Chdir(tempHome)
			if tt.unsetPort {
				prev, existed := os.LookupEnv("AI_WORKFLOW_SERVER_PORT")
				if err := os.Unsetenv("AI_WORKFLOW_SERVER_PORT"); err != nil {
					t.Fatalf("unset AI_WORKFLOW_SERVER_PORT: %v", err)
				}
				t.Cleanup(func() {
					if existed {
						_ = os.Setenv("AI_WORKFLOW_SERVER_PORT", prev)
						return
					}
					_ = os.Unsetenv("AI_WORKFLOW_SERVER_PORT")
				})
			} else {
				t.Setenv("AI_WORKFLOW_SERVER_PORT", tt.envPort)
			}

			origSchedulerFactory := newServerScheduler
			origServerFactory := newAPIServer
			origIssueManagerFactory := newServerIssueManager
			t.Cleanup(func() {
				newServerScheduler = origSchedulerFactory
				newAPIServer = origServerFactory
				newServerIssueManager = origIssueManagerFactory
			})

			startErr := errors.New("server start failed")
			fakeScheduler := &testScheduler{}
			fakeIssueManager := &testServerIssueManager{}
			capturedAddr := ""

			newServerScheduler = func(_ *engine.Executor, _ core.Store) (serverScheduler, error) {
				return fakeScheduler, nil
			}
			newServerIssueManager = func(_ *engine.Executor, _ *pluginfactory.BootstrapSet, _ core.EventBus, _ config.WatchdogConfig, _ config.TeamLeaderConfig, _ config.RoleBindings) (serverIssueManager, error) {
				return fakeIssueManager, nil
			}
			newAPIServer = func(cfg web.Config) apiServer {
				capturedAddr = cfg.Addr
				return &testAPIServer{startErr: startErr}
			}

			err := runServer(context.Background(), tt.args)
			if !errors.Is(err, startErr) {
				t.Fatalf("expected server start error, got %v", err)
			}
			if capturedAddr != tt.wantAddr {
				t.Fatalf("expected listen addr %s, got %s", tt.wantAddr, capturedAddr)
			}
			if !fakeScheduler.stopCalled {
				t.Fatal("expected scheduler stop to be called on startup failure")
			}
			if !fakeIssueManager.stopCalled {
				t.Fatal("expected issue manager stop to be called on startup failure")
			}
		})
	}
}

func TestRunServer_StartFailureJoinsSchedulerStopError(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	t.Chdir(tempHome)

	origSchedulerFactory := newServerScheduler
	origServerFactory := newAPIServer
	origIssueManagerFactory := newServerIssueManager
	t.Cleanup(func() {
		newServerScheduler = origSchedulerFactory
		newAPIServer = origServerFactory
		newServerIssueManager = origIssueManagerFactory
	})

	startErr := errors.New("server start failed")
	stopErr := errors.New("scheduler stop failed")
	fakeScheduler := &testScheduler{stopErr: stopErr}
	fakeIssueManager := &testServerIssueManager{}

	newServerScheduler = func(_ *engine.Executor, _ core.Store) (serverScheduler, error) {
		return fakeScheduler, nil
	}
	newServerIssueManager = func(_ *engine.Executor, _ *pluginfactory.BootstrapSet, _ core.EventBus, _ config.WatchdogConfig, _ config.TeamLeaderConfig, _ config.RoleBindings) (serverIssueManager, error) {
		return fakeIssueManager, nil
	}
	newAPIServer = func(_ web.Config) apiServer {
		return &testAPIServer{startErr: startErr}
	}

	err := runServer(context.Background(), nil)
	if err == nil {
		t.Fatal("expected joined error when server start and scheduler stop both fail")
	}
	if !errors.Is(err, startErr) {
		t.Fatalf("expected server start error to be included, got %v", err)
	}
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected scheduler stop error to be included, got %v", err)
	}
	if !fakeScheduler.stopCalled {
		t.Fatal("expected scheduler stop to be called on server start failure")
	}
	if !fakeIssueManager.stopCalled {
		t.Fatal("expected issue manager stop to be called on server start failure")
	}
}

func TestRunServer_IssueManagerReceivesReviewRoleBindings(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	t.Chdir(tempHome)

	origSchedulerFactory := newServerScheduler
	origServerFactory := newAPIServer
	origIssueManagerFactory := newServerIssueManager
	t.Cleanup(func() {
		newServerScheduler = origSchedulerFactory
		newAPIServer = origServerFactory
		newServerIssueManager = origIssueManagerFactory
	})

	startErr := errors.New("server start failed")
	fakeScheduler := &testScheduler{}
	fakeIssueManager := &testServerIssueManager{}
	var capturedRoleBinds config.RoleBindings

	newServerScheduler = func(_ *engine.Executor, _ core.Store) (serverScheduler, error) {
		return fakeScheduler, nil
	}
	newServerIssueManager = func(_ *engine.Executor, _ *pluginfactory.BootstrapSet, _ core.EventBus, _ config.WatchdogConfig, _ config.TeamLeaderConfig, roleBinds config.RoleBindings) (serverIssueManager, error) {
		capturedRoleBinds = roleBinds
		return fakeIssueManager, nil
	}
	newAPIServer = func(_ web.Config) apiServer {
		return &testAPIServer{startErr: startErr}
	}

	err := runServer(context.Background(), nil)
	if !errors.Is(err, startErr) {
		t.Fatalf("expected server start error, got %v", err)
	}
	if got := capturedRoleBinds.ReviewOrchestrator.Aggregator; got != "aggregator" {
		t.Fatalf("captured review_orchestrator aggregator binding = %q, want %q", got, "aggregator")
	}
	if got := capturedRoleBinds.ReviewOrchestrator.Reviewers["completeness"]; got != "reviewer" {
		t.Fatalf("captured completeness reviewer binding = %q, want %q", got, "reviewer")
	}
	if got := capturedRoleBinds.ReviewOrchestrator.Reviewers["dependency"]; got != "reviewer" {
		t.Fatalf("captured dependency reviewer binding = %q, want %q", got, "reviewer")
	}
	if got := capturedRoleBinds.ReviewOrchestrator.Reviewers["feasibility"]; got != "reviewer" {
		t.Fatalf("captured feasibility reviewer binding = %q, want %q", got, "reviewer")
	}
}

func TestRunServer_IssueManagerReceivesWatchdogConfig(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	t.Chdir(tempHome)

	origSchedulerFactory := newServerScheduler
	origServerFactory := newAPIServer
	origIssueManagerFactory := newServerIssueManager
	t.Cleanup(func() {
		newServerScheduler = origSchedulerFactory
		newAPIServer = origServerFactory
		newServerIssueManager = origIssueManagerFactory
	})

	startErr := errors.New("server start failed")
	fakeScheduler := &testScheduler{}
	fakeIssueManager := &testServerIssueManager{}
	var capturedWatchdog config.WatchdogConfig

	newServerScheduler = func(_ *engine.Executor, _ core.Store) (serverScheduler, error) {
		return fakeScheduler, nil
	}
	newServerIssueManager = func(_ *engine.Executor, _ *pluginfactory.BootstrapSet, _ core.EventBus, watchdogCfg config.WatchdogConfig, _ config.TeamLeaderConfig, _ config.RoleBindings) (serverIssueManager, error) {
		capturedWatchdog = watchdogCfg
		return fakeIssueManager, nil
	}
	newAPIServer = func(_ web.Config) apiServer {
		return &testAPIServer{startErr: startErr}
	}

	err := runServer(context.Background(), nil)
	if !errors.Is(err, startErr) {
		t.Fatalf("expected server start error, got %v", err)
	}
	if !capturedWatchdog.Enabled {
		t.Fatal("expected watchdog config to be passed into issue manager factory")
	}
	if got := capturedWatchdog.Interval.Duration; got != 5*time.Minute {
		t.Fatalf("captured watchdog interval = %s, want %s", got, 5*time.Minute)
	}
	if got := capturedWatchdog.StuckRunTTL.Duration; got != 30*time.Minute {
		t.Fatalf("captured watchdog stuck_run_ttl = %s, want %s", got, 30*time.Minute)
	}
	if got := capturedWatchdog.StuckMergeTTL.Duration; got != 15*time.Minute {
		t.Fatalf("captured watchdog stuck_merge_ttl = %s, want %s", got, 15*time.Minute)
	}
	if got := capturedWatchdog.QueueStaleTTL.Duration; got != 60*time.Minute {
		t.Fatalf("captured watchdog queue_stale_ttl = %s, want %s", got, 60*time.Minute)
	}
}

func TestCLI_ServerCommandStartsAndHealth(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	t.Chdir(tempHome)

	port := reserveFreePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runServer(ctx, []string{"--port", strconv.Itoa(port)})
	}()

	healthURL := "http://127.0.0.1:" + strconv.Itoa(port) + "/health"
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			t.Fatalf("server exited before health check: %v", err)
		default:
		}

		resp, err := client.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				cancel()
				select {
				case stopErr := <-errCh:
					if stopErr != nil {
						t.Fatalf("server shutdown failed: %v", stopErr)
					}
				case <-time.After(8 * time.Second):
					t.Fatal("server did not shut down after context cancel")
				}
				return
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server failed to start: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("server start timeout and shutdown timeout")
	}
	t.Fatalf("health check did not return 200 within timeout")
}

func TestTeamLeaderIssueManagerAdapterCreateIssuesDelegatesToManager(t *testing.T) {
	fakeManager := &fakeTeamLeaderIssueService{
		createIssuesFn: func(_ context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error) {
			if input.ProjectID != "proj-1" {
				t.Fatalf("create issues project id = %q, want %q", input.ProjectID, "proj-1")
			}
			if input.SessionID != "chat-1" {
				t.Fatalf("create issues session id = %q, want %q", input.SessionID, "chat-1")
			}
			if len(input.Issues) != 1 {
				t.Fatalf("create issues size = %d, want 1", len(input.Issues))
			}
			spec := input.Issues[0]
			if spec.Title != "my plan" {
				t.Fatalf("issue title = %q, want %q", spec.Title, "my plan")
			}
			if spec.Template != "standard" {
				t.Fatalf("issue template = %q, want %q", spec.Template, "standard")
			}
			if spec.FailPolicy != core.FailSkip {
				t.Fatalf("issue fail policy = %q, want %q", spec.FailPolicy, core.FailSkip)
			}
			if len(spec.Labels) != 2 || spec.Labels[0] != "plan" || spec.Labels[1] != "from-files" {
				t.Fatalf("issue labels = %#v, want [plan from-files]", spec.Labels)
			}
			if !strings.Contains(spec.Body, "## Conversation") || !strings.Contains(spec.Body, "## Source Files") {
				t.Fatalf("issue body should include conversation and source files sections, got %q", spec.Body)
			}
			if !strings.Contains(spec.Body, "docs/spec/demo.md") || !strings.Contains(spec.Body, "hello spec") {
				t.Fatalf("issue body should include file path and file content, got %q", spec.Body)
			}
			return []*core.Issue{
				{
					ID:        "issue-1",
					ProjectID: input.ProjectID,
					SessionID: input.SessionID,
					Title:     spec.Title,
					Template:  spec.Template,
					Status:    core.IssueStatusDraft,
					State:     core.IssueStateOpen,
				},
			}, nil
		},
	}

	adapter := &teamLeaderIssueManagerAdapter{manager: fakeManager}
	issues, err := adapter.CreateIssues(context.Background(), web.IssueCreateInput{
		ProjectID:  "proj-1",
		SessionID:  "chat-1",
		Name:       "my plan",
		FailPolicy: core.FailSkip,
		Request: web.IssueCreateRequest{
			Conversation: "请生成计划",
		},
		SourceFiles: []string{"docs/spec/demo.md"},
		FileContents: map[string]string{
			"docs/spec/demo.md": "hello spec",
		},
	})
	if err != nil {
		t.Fatalf("CreateIssues() error = %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("CreateIssues() returned %d issues, want 1", len(issues))
	}
	if issues[0].ID != "issue-1" {
		t.Fatalf("created issue id = %q, want %q", issues[0].ID, "issue-1")
	}
}

func TestTeamLeaderIssueManagerAdapterCreateIssuesDefaultFailPolicy(t *testing.T) {
	fakeManager := &fakeTeamLeaderIssueService{
		createIssuesFn: func(_ context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error) {
			if len(input.Issues) != 1 {
				t.Fatalf("create issues size = %d, want 1", len(input.Issues))
			}
			spec := input.Issues[0]
			if spec.FailPolicy != core.FailBlock {
				t.Fatalf("default fail policy = %q, want %q", spec.FailPolicy, core.FailBlock)
			}
			if spec.Title != "Plan from chat session" {
				t.Fatalf("default issue title = %q, want %q", spec.Title, "Plan from chat session")
			}
			return []*core.Issue{{ID: "issue-default"}}, nil
		},
	}

	adapter := &teamLeaderIssueManagerAdapter{manager: fakeManager}
	issues, err := adapter.CreateIssues(context.Background(), web.IssueCreateInput{
		ProjectID: "proj-2",
		SessionID: "chat-2",
		Request: web.IssueCreateRequest{
			Conversation: "hello",
		},
	})
	if err != nil {
		t.Fatalf("CreateIssues() error = %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "issue-default" {
		t.Fatalf("CreateIssues() returned %#v, want issue-default", issues)
	}
}

func TestResolveTeamLeaderRoleIDReturnsBindingRole(t *testing.T) {
	roleBindings := config.RoleBindings{
		TeamLeader: config.SingleRoleBinding{Role: " team-leader "},
	}

	if got := resolveTeamLeaderRoleID(roleBindings); got != "team-leader" {
		t.Fatalf("resolveTeamLeaderRoleID() = %q, want %q", got, "team-leader")
	}
}

func TestResolveTeamLeaderRoleIDReturnsEmptyWhenUnset(t *testing.T) {
	if got := resolveTeamLeaderRoleID(config.RoleBindings{}); got != "" {
		t.Fatalf("resolveTeamLeaderRoleID() = %q, want empty", got)
	}
}

func TestBuildV2SandboxDisabledReturnsNoop(t *testing.T) {
	t.Setenv("AI_WORKFLOW_CODEX_REQUIRE_AUTH", "true")

	got := buildV2Sandbox(&config.Config{
		V2: config.V2Config{
			Sandbox: config.V2SandboxConfig{Enabled: false},
		},
	}, t.TempDir())

	if _, ok := got.(v2sandbox.NoopSandbox); !ok {
		t.Fatalf("buildV2Sandbox() = %T, want NoopSandbox when config disabled", got)
	}
}

func TestBuildV2SandboxEnabledReturnsHomeDirSandbox(t *testing.T) {
	t.Setenv("AI_WORKFLOW_CODEX_REQUIRE_AUTH", "true")
	dataDir := t.TempDir()

	got := buildV2Sandbox(&config.Config{
		V2: config.V2Config{
			Sandbox: config.V2SandboxConfig{Enabled: true},
		},
	}, dataDir)

	sb, ok := got.(v2sandbox.HomeDirSandbox)
	if !ok {
		t.Fatalf("buildV2Sandbox() = %T, want HomeDirSandbox when config enabled", got)
	}
	if sb.DataDir != dataDir {
		t.Fatalf("HomeDirSandbox.DataDir = %q, want %q", sb.DataDir, dataDir)
	}
	if !sb.RequireCodexAuth {
		t.Fatal("HomeDirSandbox.RequireCodexAuth = false, want true from env")
	}
}

func TestBuildV2SandboxLiteBoxProviderReturnsLiteBoxSandbox(t *testing.T) {
	t.Setenv("AI_WORKFLOW_CODEX_REQUIRE_AUTH", "true")
	dataDir := t.TempDir()

	got := buildV2Sandbox(&config.Config{
		V2: config.V2Config{
			Sandbox: config.V2SandboxConfig{
				Enabled:  true,
				Provider: "litebox",
				LiteBox: config.V2LiteBoxConfig{
					BridgeCommand: "litebox-acp",
					RunnerPath:    "D:\\litebox\\runner.exe",
					RunnerArgs:    []string{"--rootfs", "D:\\rootfs"},
				},
			},
		},
	}, dataDir)

	sb, ok := got.(v2sandbox.LiteBoxSandbox)
	if !ok {
		t.Fatalf("buildV2Sandbox() = %T, want LiteBoxSandbox when provider=litebox", got)
	}
	if sb.BridgeCommand != "litebox-acp" {
		t.Fatalf("LiteBoxSandbox.BridgeCommand = %q, want %q", sb.BridgeCommand, "litebox-acp")
	}
	if sb.RunnerPath != "D:\\litebox\\runner.exe" {
		t.Fatalf("LiteBoxSandbox.RunnerPath = %q, want runner path", sb.RunnerPath)
	}
	base, ok := sb.Base.(v2sandbox.HomeDirSandbox)
	if !ok {
		t.Fatalf("LiteBoxSandbox.Base = %T, want HomeDirSandbox", sb.Base)
	}
	if base.DataDir != dataDir {
		t.Fatalf("HomeDirSandbox.DataDir = %q, want %q", base.DataDir, dataDir)
	}
	if !base.RequireCodexAuth {
		t.Fatal("HomeDirSandbox.RequireCodexAuth = false, want true from env")
	}
}

func reserveFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for free port: %v", err)
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener addr type: %T", ln.Addr())
	}
	return addr.Port
}

func captureStdout(t *testing.T, fn func()) (output string) {
	t.Helper()

	stdoutCaptureMu.Lock()
	defer stdoutCaptureMu.Unlock()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}

	outputCh := make(chan string, 1)
	go func() {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, reader)
		outputCh <- buffer.String()
	}()

	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
		_ = writer.Close()
		output = <-outputCh
		_ = reader.Close()
	}()

	fn()
	return output
}

func mustReadFrontendIndex(t *testing.T, frontendFS fs.FS) string {
	t.Helper()
	content, err := fs.ReadFile(frontendFS, "index.html")
	if err != nil {
		t.Fatalf("read frontend index.html from fs: %v", err)
	}
	return string(content)
}

type testAPIServer struct {
	startErr    error
	shutdownErr error
}

func (s *testAPIServer) Start() error {
	return s.startErr
}

func (s *testAPIServer) Shutdown(_ context.Context) error {
	return s.shutdownErr
}

type testScheduler struct {
	startErr    error
	stopErr     error
	startCalled bool
	stopCalled  bool
}

func (s *testScheduler) Start(_ context.Context) error {
	s.startCalled = true
	return s.startErr
}

func (s *testScheduler) Stop(_ context.Context) error {
	s.stopCalled = true
	return s.stopErr
}

type testServerIssueManager struct {
	startErr    error
	stopErr     error
	startCalled bool
	stopCalled  bool
}

func (m *testServerIssueManager) Start(_ context.Context) error {
	m.startCalled = true
	return m.startErr
}

func (m *testServerIssueManager) Stop(_ context.Context) error {
	m.stopCalled = true
	return m.stopErr
}

func (m *testServerIssueManager) CreateIssues(_ context.Context, _ web.IssueCreateInput) ([]core.Issue, error) {
	return []core.Issue{}, nil
}

func (m *testServerIssueManager) SubmitForReview(_ context.Context, issueID string, _ web.IssueReviewInput) (*core.Issue, error) {
	return &core.Issue{ID: issueID}, nil
}

func (m *testServerIssueManager) ApplyIssueAction(_ context.Context, issueID string, _ web.IssueAction) (*core.Issue, error) {
	return &core.Issue{ID: issueID}, nil
}

type fakeTeamLeaderIssueService struct {
	startErr          error
	stopErr           error
	createIssuesFn    func(ctx context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error)
	confirmIssuesFn   func(ctx context.Context, issueIDs []string, feedback string) ([]*core.Issue, error)
	submitForReviewFn func(ctx context.Context, issueIDs []string) error
	applyActionFn     func(ctx context.Context, issueID, action, feedback string) (*core.Issue, error)
}

func (s *fakeTeamLeaderIssueService) Start(_ context.Context) error {
	return s.startErr
}

func (s *fakeTeamLeaderIssueService) Stop(_ context.Context) error {
	return s.stopErr
}

func (s *fakeTeamLeaderIssueService) CreateIssues(ctx context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error) {
	if s.createIssuesFn == nil {
		return nil, errors.New("unexpected CreateIssues call")
	}
	return s.createIssuesFn(ctx, input)
}

func (s *fakeTeamLeaderIssueService) ConfirmCreatedIssues(ctx context.Context, issueIDs []string, feedback string) ([]*core.Issue, error) {
	if s.confirmIssuesFn == nil {
		out := make([]*core.Issue, 0, len(issueIDs))
		for _, issueID := range issueIDs {
			out = append(out, &core.Issue{ID: issueID})
		}
		return out, nil
	}
	return s.confirmIssuesFn(ctx, issueIDs, feedback)
}

func (s *fakeTeamLeaderIssueService) SubmitForReview(ctx context.Context, issueIDs []string) error {
	if s.submitForReviewFn == nil {
		return nil
	}
	return s.submitForReviewFn(ctx, issueIDs)
}

func (s *fakeTeamLeaderIssueService) ApplyIssueAction(ctx context.Context, issueID, action, feedback string) (*core.Issue, error) {
	if s.applyActionFn == nil {
		return &core.Issue{ID: issueID}, nil
	}
	return s.applyActionFn(ctx, issueID, action, feedback)
}
