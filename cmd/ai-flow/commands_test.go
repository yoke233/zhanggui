package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
	"github.com/yoke233/ai-workflow/internal/eventbus"
	pluginfactory "github.com/yoke233/ai-workflow/internal/plugins/factory"
	"github.com/yoke233/ai-workflow/internal/teamleader"
	"github.com/yoke233/ai-workflow/internal/web"
)

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
			newServerIssueManager = func(_ *engine.Executor, _ *pluginfactory.BootstrapSet, _ *eventbus.Bus, _ config.TeamLeaderConfig, _ config.RoleBindings) (serverIssueManager, error) {
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
	newServerIssueManager = func(_ *engine.Executor, _ *pluginfactory.BootstrapSet, _ *eventbus.Bus, _ config.TeamLeaderConfig, _ config.RoleBindings) (serverIssueManager, error) {
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
	newServerIssueManager = func(_ *engine.Executor, _ *pluginfactory.BootstrapSet, _ *eventbus.Bus, _ config.TeamLeaderConfig, roleBinds config.RoleBindings) (serverIssueManager, error) {
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

func TestCLI_ServerCommandStartsAndHealth(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

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
