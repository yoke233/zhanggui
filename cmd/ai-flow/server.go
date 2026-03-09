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
	ghwebhook "github.com/yoke233/ai-workflow/internal/github"
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

		opts := make([]teamleader.ManagerOption, 0, 2)
		opts = append(opts, teamleader.WithEventPublisher(bus))
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

	cfg, err := loadBootstrapConfig()
	if err != nil {
		return err
	}

	configDir, _ := resolveDataDir()

	secrets, err := config.LoadSecrets(secretsFilePath(configDir))
	if err != nil {
		return fmt.Errorf("load secrets: %w", err)
	}
	tokenRegistry := web.NewTokenRegistry(secrets.Tokens)

	port = resolveServerPort(port, cfg.Server.Port)
	listenAddr := buildServerAddress(cfg.Server.Host, port)
	frontendFS, err := resolveServerFrontendFS()
	if err != nil {
		return err
	}

	scheduler, err := newServerScheduler(exec, store)
	if err != nil {
		return err
	}
	if err := scheduler.Start(ctx); err != nil {
		return err
	}

	issueManager, err := newServerIssueManager(exec, bootstrapSet, bus, cfg.Scheduler.Watchdog, cfg.TeamLeader, cfg.RoleBinds)
	if err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		stopErr := scheduler.Stop(stopCtx)
		return errors.Join(err, stopErr)
	}
	if issueManager == nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		stopErr := scheduler.Stop(stopCtx)
		return errors.Join(errors.New("issue manager is not configured"), stopErr)
	}
	if err := issueManager.Start(ctx); err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		managerStopErr := issueManager.Stop(stopCtx)
		stopErr := scheduler.Stop(stopCtx)
		return errors.Join(err, managerStopErr, stopErr)
	}

	var a2aBridge web.A2ABridge
	if adapter, ok := issueManager.(*teamLeaderIssueManagerAdapter); ok {
		bridge, bridgeErr := teamleader.NewA2ABridge(store, adapter.manager)
		if bridgeErr != nil {
			slog.Warn("A2ABridge creation failed, A2A endpoint will be unavailable", "error", bridgeErr)
		} else {
			a2aBridge = bridge
			slog.Info("A2ABridge created successfully")
		}
	} else {
		slog.Warn("issueManager is not teamLeaderIssueManagerAdapter, A2A bridge unavailable", "type", fmt.Sprintf("%T", issueManager))
	}

	hub := web.NewHub()
	if bootstrapSet.RoleResolver == nil {
		return errors.New("chat assistant requires role resolver")
	}
	var runEventRecorder teamleader.ChatRunEventRecorder
	if recorder, ok := store.(teamleader.ChatRunEventRecorder); ok {
		runEventRecorder = recorder
	}
	chatAssistant := web.NewACPChatAssistantWithDeps(web.ACPChatAssistantDeps{
		DefaultRoleID:    resolveTeamLeaderRoleID(cfg.RoleBinds),
		RoleResolver:     bootstrapSet.RoleResolver,
		EventPublisher:   bus,
		RunEventRecorder: runEventRecorder,
		MCPEnv: teamleader.MCPEnvConfig{
			DBPath:     expandStorePath(cfg.Store.Path),
			ServerAddr: "http://" + listenAddr,
		},
	})
	var decomposePlanner *teamleader.DecomposePlanner
	if chatAssistant != nil {
		decomposePlanner = teamleader.NewDecomposePlanner(func(ctx context.Context, projectID, systemPrompt, userMessage string) (string, error) {
			var workDir string
			if trimmedProjectID := strings.TrimSpace(projectID); trimmedProjectID != "" {
				project, err := store.GetProject(trimmedProjectID)
				if err == nil && project != nil {
					workDir = strings.TrimSpace(project.RepoPath)
				}
			}
			resp, err := chatAssistant.Reply(ctx, web.ChatAssistantRequest{
				Message:   systemPrompt + "\n\nUser request:\n" + userMessage,
				ProjectID: strings.TrimSpace(projectID),
				WorkDir:   workDir,
			})
			if err != nil {
				return "", err
			}
			return resp.Reply, nil
		})
	}
	var proposalIssueCreator web.ProposalIssueCreator

	var merger teamleader.PRMerger
	if cfg.GitHub.Enabled {
		merger = ghwebhook.NewPRLifecycle(store, bootstrapSet.SCM)
	}
	autoMerger := teamleader.NewAutoMergeHandler(store, bus, merger)
	tlTriageHandler := teamleader.NewTLTriageHandler(store, bus, 3)

	// Decompose handler: listens for EventIssueDecomposing and creates child issues.
	decomposeHandler := teamleader.NewDecomposeHandler(store, bus, func(ctx context.Context, parent *core.Issue) ([]teamleader.DecomposeSpec, error) {
		return nil, fmt.Errorf("decomposer agent not configured for issue %s", parent.ID)
	})
	if adapter, ok := issueManager.(*teamLeaderIssueManagerAdapter); ok {
		decomposeHandler.SetReviewSubmitter(adapter.manager)
		proposalIssueCreator = adapter.manager
	}

	childCompletionHandler := teamleader.NewChildCompletionHandler(store, bus)

	wsBroadcaster := newWSBroadcaster(hub, bus)
	if err := wsBroadcaster.Start(ctx); err != nil {
		return fmt.Errorf("start ws broadcaster: %w", err)
	}

	eventPersister := newEventPersister(store, bus)
	if err := eventPersister.Start(ctx); err != nil {
		return fmt.Errorf("start event persister: %w", err)
	}

	if err := autoMerger.Start(ctx); err != nil {
		return fmt.Errorf("start auto merger: %w", err)
	}
	if err := tlTriageHandler.Start(ctx); err != nil {
		return fmt.Errorf("start tl triage handler: %w", err)
	}
	if err := decomposeHandler.Start(ctx); err != nil {
		return fmt.Errorf("start decompose handler: %w", err)
	}
	if err := childCompletionHandler.Start(ctx); err != nil {
		return fmt.Errorf("start child completion handler: %w", err)
	}

	restartCh := make(chan struct{}, 1)
	restartFunc := func() {
		select {
		case restartCh <- struct{}{}:
		default:
		}
	}

	apiSrv := newAPIServer(web.Config{
		Addr:                 listenAddr,
		Auth:                 tokenRegistry,
		WebhookSecret:        cfg.GitHub.WebhookSecret,
		A2AEnabled:           cfg.A2A.Enabled,
		A2AVersion:           cfg.A2A.Version,
		Frontend:             frontendFS,
		Store:                store,
		A2ABridge:            a2aBridge,
		IssueManager:         issueManager,
		DecomposePlanner:     decomposePlanner,
		ProposalIssueCreator: proposalIssueCreator,
		ChatAssistant:        chatAssistant,
		EventPublisher:       bus,
		RunExec:              exec,
		StageSessionMgr:      exec,
		RunstageRoles:        cfg.RoleBinds.Run.StageRoles,
		IssueParserRoleID:    strings.TrimSpace(cfg.RoleBinds.PlanParser.Role),
		SCM:                  bootstrapSet.SCM,
		Hub:                  hub,
		RestartFunc:          restartFunc,
		MCPServerOpts: web.MCPServerOptions{
			DBPath:     expandStorePath(cfg.Store.Path),
			ServerAddr: "http://" + listenAddr,
			ConfigDir:  configDir,
		},
		MCPDeps:      buildMCPDeps(issueManager, exec, store),
		RoleResolver: bootstrapSet.RoleResolver,
	})

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- apiSrv.Start()
	}()

	fmt.Printf("Server started on %s (ws: /api/v1/ws). Press Ctrl+C to stop.\n", listenAddr)

	select {
	case serverErr := <-serverErrCh:
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		managerStopErr := issueManager.Stop(stopCtx)
		stopErr := scheduler.Stop(stopCtx)
		stopHandlers(stopCtx, childCompletionHandler, decomposeHandler, tlTriageHandler, autoMerger, eventPersister, wsBroadcaster)
		return errors.Join(serverErr, managerStopErr, stopErr)
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
	if err := issueManager.Stop(stopCtx); err != nil && shutdownErr == nil {
		shutdownErr = err
	}
	if err := scheduler.Stop(stopCtx); err != nil && shutdownErr == nil {
		shutdownErr = err
	}
	stopHandlers(stopCtx, childCompletionHandler, decomposeHandler, tlTriageHandler, autoMerger, eventPersister, wsBroadcaster)

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
