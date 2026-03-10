package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
	pluginfactory "github.com/yoke233/ai-workflow/internal/plugins/factory"
	"github.com/yoke233/ai-workflow/internal/teamleader"
	"github.com/yoke233/ai-workflow/internal/web"
)

const defaultServerPort = 8080

const (
	defaultFrontendDir = "/opt/ai-workflow/web/dist"
	repoFrontendDir    = "web/dist"
	frontendDirEnvVar  = "AI_WORKFLOW_FRONTEND_DIR"
)

type apiServer interface {
	Start() error
	Shutdown(ctx context.Context) error
}

type serverScheduler interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type serverIssueManager interface {
	web.IssueManager
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

var (
	newAPIServer = func(cfg web.Config) apiServer {
		return web.NewServer(cfg)
	}
	newServerScheduler = func(exec *engine.Executor, store core.Store) (serverScheduler, error) {
		return buildScheduler(exec, store)
	}
	newServerIssueManager = func(
		exec *engine.Executor,
		bootstrapSet *pluginfactory.BootstrapSet,
		bus core.EventBus,
		watchdogCfg config.WatchdogConfig,
		teamLeaderCfg config.TeamLeaderConfig,
		roleBinds config.RoleBindings,
	) (serverIssueManager, error) {
		if exec == nil {
			return nil, errors.New("executor is required for issue manager")
		}
		if bootstrapSet == nil {
			return nil, errors.New("bootstrap set is required for issue manager")
		}
		reviewPanel, err := teamleader.NewDefaultReviewOrchestratorFromBindings(
			bootstrapSet.Store,
			teamleader.ReviewRoleBindingInput{
				Reviewers:  cloneStringMap(roleBinds.ReviewOrchestrator.Reviewers),
				Aggregator: roleBinds.ReviewOrchestrator.Aggregator,
			},
			bootstrapSet.RoleResolver,
		)
		if err != nil {
			return nil, fmt.Errorf("build review orchestrator from role bindings: %w", err)
		}
		if teamLeaderCfg.ReviewOrchestrator.MaxRounds > 0 {
			reviewPanel.MaxRounds = teamLeaderCfg.ReviewOrchestrator.MaxRounds
		}
		runTaskRun := func(ctx context.Context, RunID string) error {
			ok, err := bootstrapSet.Store.TryMarkRunInProgress(RunID, core.StatusQueued)
			if err != nil {
				return err
			}
			if !ok {
				// Run is already claimed by another scheduler loop.
				return nil
			}
			return exec.RunScheduled(ctx, RunID)
		}
		depScheduler := teamleader.NewDepScheduler(
			bootstrapSet.Store,
			bus,
			runTaskRun,
			bootstrapSet.Tracker,
			teamLeaderCfg.DAGScheduler.MaxConcurrentTasks,
		)
		depScheduler.SetWatchdogConfig(watchdogCfg)
		depScheduler.SetStageRoles(roleBinds.Run.StageRoles)

		demandReviewer := reviewPanel.DemandReviewer()
		if demandReviewer == nil {
			return nil, errors.New("gate chain requires a demand reviewer")
		}
		gateChain := &teamleader.GateChain{
			Store: bootstrapSet.Store,
			Runners: map[core.GateType]core.GateRunner{
				core.GateTypeAuto:        &teamleader.AutoGateRunner{Reviewer: demandReviewer},
				core.GateTypeOwnerReview: &teamleader.OwnerReviewRunner{},
				core.GateTypePeerReview:  &teamleader.PeerReviewRunner{},
				core.GateTypeVote:        &teamleader.VoteGateRunner{},
			},
		}

		opts := make([]teamleader.ManagerOption, 0, 3)
		opts = append(opts, teamleader.WithEventPublisher(bus))
		opts = append(opts, teamleader.WithGateChain(gateChain))
		if bootstrapSet.ReviewGate != nil {
			opts = append(opts, teamleader.WithReviewGate(bootstrapSet.ReviewGate))
		}
		manager, err := teamleader.NewManager(
			bootstrapSet.Store,
			nil,
			reviewPanel,
			&depSchedulerIssueAdapter{scheduler: depScheduler},
			opts...,
		)
		if err != nil {
			return nil, err
		}
		return &teamLeaderIssueManagerAdapter{
			manager: manager,
			store:   bootstrapSet.Store,
		}, nil
	}
)

func cmdServer(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runServer(ctx, args)
}

func runServer(ctx context.Context, args []string) error {
	port, err := parseServerPort(args)
	if err != nil {
		return err
	}

	exec, bootstrapSet, bus, err := bootstrapWithEventBus()
	if err != nil {
		return err
	}
	store := bootstrapSet.Store
	defer store.Close()
	defer bus.Close()

	launch, err := prepareServerLaunch(port)
	if err != nil {
		return err
	}

	runtime, err := setupServerRuntime(ctx, exec, bootstrapSet, bus, launch.cfg, launch.listenAddr, launch.adminToken)
	if err != nil {
		return err
	}
	runtimeActive := true
	defer func() {
		if runtimeActive {
			_ = runtime.stop(nil)
		}
	}()

	// --- V2 Engine Bootstrap ---
	v2MCPEnv := teamleader.MCPEnvConfig{
		DBPath:     expandStorePath(launch.cfg.Store.Path),
		ServerAddr: "http://" + launch.listenAddr,
		AuthToken:  launch.adminToken,
	}
	_, _, runtimeManager, v2Cleanup, v2RouteRegistrar := bootstrapV2(expandStorePath(launch.cfg.Store.Path), bootstrapSet.RoleResolver, launch.cfg, v2MCPEnv, launch.githubTokens)
	if v2Cleanup != nil {
		defer v2Cleanup()
	}

	restartCh := make(chan struct{}, 1)
	restartFunc := func() {
		select {
		case restartCh <- struct{}{}:
		default:
		}
	}

	apiSrv := newAPIServer(web.Config{
		Addr:                 launch.listenAddr,
		Auth:                 launch.tokenRegistry,
		WebhookSecret:        launch.cfg.GitHub.WebhookSecret,
		A2AEnabled:           launch.cfg.A2A.Enabled,
		A2AVersion:           launch.cfg.A2A.Version,
		Frontend:             launch.frontendFS,
		Store:                store,
		ContextStore:         runtime.contextStore,
		A2ABridge:            runtime.a2aBridge,
		IssueManager:         runtime.issueManager,
		DecomposePlanner:     runtime.decomposePlanner,
		ProposalIssueCreator: runtime.proposalIssueCreator,
		ChatAssistant:        runtime.chatAssistant,
		EventPublisher:       bus,
		RunExec:              exec,
		StageSessionMgr:      exec,
		RunstageRoles:        launch.cfg.RoleBinds.Run.StageRoles,
		IssueParserRoleID:    strings.TrimSpace(launch.cfg.RoleBinds.PlanParser.Role),
		SCM:                  bootstrapSet.SCM,
		Hub:                  runtime.hub,
		RestartFunc:          restartFunc,
		MCPServerOpts: web.MCPServerOptions{
			DBPath:     expandStorePath(launch.cfg.Store.Path),
			ServerAddr: "http://" + launch.listenAddr,
			ConfigDir:  launch.configDir,
		},
		MCPDeps:          buildMCPDeps(runtime.issueManager, exec, store),
		RoleResolver:     bootstrapSet.RoleResolver,
		V2RouteRegistrar: v2RouteRegistrar,
		RuntimeConfig:    runtimeManager,
	})

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- apiSrv.Start()
	}()

	fmt.Printf("Server started on %s (ws: /api/v1/ws). Press Ctrl+C to stop.\n", launch.listenAddr)

	select {
	case serverErr := <-serverErrCh:
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		runtimeActive = false
		return errors.Join(serverErr, runtime.stop(stopCtx))
	case <-restartCh:
		fmt.Println("Restart signal received, shutting down for restart...")
	case <-ctx.Done():
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var shutdownErr error
	if err := apiSrv.Shutdown(stopCtx); err != nil {
		shutdownErr = err
	}
	runtimeActive = false
	if err := runtime.stop(stopCtx); err != nil && shutdownErr == nil {
		shutdownErr = err
	}

	select {
	case serverErr := <-serverErrCh:
		if serverErr != nil && shutdownErr == nil {
			shutdownErr = serverErr
		}
	case <-stopCtx.Done():
	}

	return shutdownErr
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func resolveTeamLeaderRoleID(roleBindings config.RoleBindings) string {
	return strings.TrimSpace(roleBindings.TeamLeader.Role)
}

func parseServerPort(args []string) (int, error) {
	port := 0
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--port":
			i++
			if i >= len(args) {
				return 0, fmt.Errorf("usage: ai-flow server [--port <port>]")
			}
			parsed, err := parsePortValue(args[i])
			if err != nil {
				return 0, err
			}
			port = parsed
		case strings.HasPrefix(arg, "--port="):
			raw := strings.TrimSpace(strings.TrimPrefix(arg, "--port="))
			parsed, err := parsePortValue(raw)
			if err != nil {
				return 0, err
			}
			port = parsed
		default:
			return 0, fmt.Errorf("usage: ai-flow server [--port <port>]")
		}
	}
	return port, nil
}

func parsePortValue(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	port, err := strconv.Atoi(trimmed)
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("invalid --port value: %s", raw)
	}
	return port, nil
}

func resolveServerPort(cliPort int, cfgPort int) int {
	if cliPort > 0 {
		return cliPort
	}
	if cfgPort > 0 && cfgPort <= 65535 {
		return cfgPort
	}
	return defaultServerPort
}

func resolveServerFrontendFS() (fs.FS, error) {
	rawDir, hasOverride := os.LookupEnv(frontendDirEnvVar)
	frontendDir := strings.TrimSpace(rawDir)
	if hasOverride && frontendDir != "" {
		frontendFS, found, err := resolveFrontendDirFS(frontendDir)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("resolve frontend dir %q: %w", frontendDir, os.ErrNotExist)
		}
		return frontendFS, nil
	}

	candidates := []string{
		defaultFrontendDir,
		repoFrontendDir,
	}
	for _, candidate := range candidates {
		frontendFS, found, err := resolveFrontendDirFS(candidate)
		if err != nil {
			return nil, err
		}
		if found {
			return frontendFS, nil
		}
	}

	return nil, nil
}

func resolveFrontendDirFS(frontendDir string) (fs.FS, bool, error) {
	info, err := os.Stat(frontendDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("resolve frontend dir %q: %w", frontendDir, err)
	}
	if !info.IsDir() {
		return nil, false, fmt.Errorf("resolve frontend dir %q: not a directory", frontendDir)
	}
	return os.DirFS(frontendDir), true, nil
}

func buildServerAddress(host string, port int) string {
	trimmedHost := strings.TrimSpace(host)
	if trimmedHost == "" {
		return fmt.Sprintf(":%d", port)
	}
	return net.JoinHostPort(trimmedHost, strconv.Itoa(port))
}

func buildMCPDeps(issueManager serverIssueManager, exec *engine.Executor, store core.Store) web.MCPDeps {
	var deps web.MCPDeps
	if adapter, ok := issueManager.(*teamLeaderIssueManagerAdapter); ok {
		deps.IssueManager = &mcpIssueManagerAdapter{manager: adapter.manager, store: store}
	}
	if exec != nil {
		deps.RunExecutor = exec
	}
	return deps
}

// secretsFilePath returns the path to secrets.toml within the given data directory.
// Falls back to secrets.yaml if the .toml file does not exist (migration support).
func secretsFilePath(dataDir string) string {
	tomlPath := filepath.Join(dataDir, "secrets.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		return tomlPath
	}
	yamlPath := filepath.Join(dataDir, "secrets.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath
	}
	return tomlPath // default to .toml for new installations
}

func buildScheduler(exec *engine.Executor, store core.Store) (*engine.Scheduler, error) {
	cfg, err := loadBootstrapConfig()
	if err != nil {
		return nil, err
	}
	return engine.NewScheduler(
		store,
		exec,
		slog.Default(),
		cfg.Scheduler.MaxGlobalAgents,
		cfg.Scheduler.MaxProjectRuns,
	), nil
}
