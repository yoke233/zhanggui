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

	"github.com/user/ai-workflow/internal/config"
	"github.com/user/ai-workflow/internal/core"
	"github.com/user/ai-workflow/internal/engine"
	"github.com/user/ai-workflow/internal/eventbus"
	pluginfactory "github.com/user/ai-workflow/internal/plugins/factory"
	"github.com/user/ai-workflow/internal/secretary"
	"github.com/user/ai-workflow/internal/web"
)

func TestCLI_PipelineActionCommand(t *testing.T) {
	err := runWithArgs([]string{"pipeline", "action"})
	if err == nil {
		t.Fatal("expected usage error for missing pipeline action args")
	}
	if !strings.Contains(err.Error(), "usage: ai-flow pipeline action") {
		t.Fatalf("expected pipeline action usage error, got %v", err)
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
			origPlanManagerFactory := newServerPlanManager
			t.Cleanup(func() {
				newServerScheduler = origSchedulerFactory
				newAPIServer = origServerFactory
				newServerPlanManager = origPlanManagerFactory
			})

			startErr := errors.New("server start failed")
			fakeScheduler := &testScheduler{}
			fakePlanManager := &testServerPlanManager{}
			capturedAddr := ""

			newServerScheduler = func(_ *engine.Executor, _ core.Store) (serverScheduler, error) {
				return fakeScheduler, nil
			}
			newServerPlanManager = func(_ *engine.Executor, _ *pluginfactory.BootstrapSet, _ *eventbus.Bus, _ config.SecretaryConfig, _ config.RoleBindings) (serverPlanManager, error) {
				return fakePlanManager, nil
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
			if !fakePlanManager.stopCalled {
				t.Fatal("expected plan manager stop to be called on startup failure")
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
	origPlanManagerFactory := newServerPlanManager
	t.Cleanup(func() {
		newServerScheduler = origSchedulerFactory
		newAPIServer = origServerFactory
		newServerPlanManager = origPlanManagerFactory
	})

	startErr := errors.New("server start failed")
	stopErr := errors.New("scheduler stop failed")
	fakeScheduler := &testScheduler{stopErr: stopErr}
	fakePlanManager := &testServerPlanManager{}

	newServerScheduler = func(_ *engine.Executor, _ core.Store) (serverScheduler, error) {
		return fakeScheduler, nil
	}
	newServerPlanManager = func(_ *engine.Executor, _ *pluginfactory.BootstrapSet, _ *eventbus.Bus, _ config.SecretaryConfig, _ config.RoleBindings) (serverPlanManager, error) {
		return fakePlanManager, nil
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
	if !fakePlanManager.stopCalled {
		t.Fatal("expected plan manager stop to be called on server start failure")
	}
}

func TestRunServer_PlanManagerReceivesReviewRoleBindings(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

	origSchedulerFactory := newServerScheduler
	origServerFactory := newAPIServer
	origPlanManagerFactory := newServerPlanManager
	t.Cleanup(func() {
		newServerScheduler = origSchedulerFactory
		newAPIServer = origServerFactory
		newServerPlanManager = origPlanManagerFactory
	})

	startErr := errors.New("server start failed")
	fakeScheduler := &testScheduler{}
	fakePlanManager := &testServerPlanManager{}
	var capturedRoleBinds config.RoleBindings

	newServerScheduler = func(_ *engine.Executor, _ core.Store) (serverScheduler, error) {
		return fakeScheduler, nil
	}
	newServerPlanManager = func(_ *engine.Executor, _ *pluginfactory.BootstrapSet, _ *eventbus.Bus, _ config.SecretaryConfig, roleBinds config.RoleBindings) (serverPlanManager, error) {
		capturedRoleBinds = roleBinds
		return fakePlanManager, nil
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

func TestBootstrapWithEventBus_ContainsSpecPlugin(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

	_, bootstrapSet, bus, err := bootstrapWithEventBus()
	if err != nil {
		t.Fatalf("bootstrapWithEventBus() error = %v", err)
	}
	defer bootstrapSet.Store.Close()
	defer bus.Close()

	if bootstrapSet.Spec == nil {
		t.Fatal("expected bootstrap set to include spec plugin")
	}
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

type testServerPlanManager struct {
	startErr    error
	stopErr     error
	startCalled bool
	stopCalled  bool
}

func (m *testServerPlanManager) Start(_ context.Context) error {
	m.startCalled = true
	return m.startErr
}

func (m *testServerPlanManager) Stop(_ context.Context) error {
	m.stopCalled = true
	return m.stopErr
}

func (m *testServerPlanManager) CreateDraft(_ context.Context, _ secretary.CreateDraftInput) (*core.TaskPlan, error) {
	return nil, nil
}

func (m *testServerPlanManager) SubmitReview(_ context.Context, _ string, _ secretary.ReviewInput) (*core.TaskPlan, error) {
	return nil, nil
}

func (m *testServerPlanManager) ApplyPlanAction(_ context.Context, _ string, _ secretary.PlanAction) (*core.TaskPlan, error) {
	return nil, nil
}
