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

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
	ghwebhook "github.com/yoke233/ai-workflow/internal/github"
	contextsqlite "github.com/yoke233/ai-workflow/internal/plugins/context-sqlite"
	pluginfactory "github.com/yoke233/ai-workflow/internal/plugins/factory"
	"github.com/yoke233/ai-workflow/internal/teamleader"
	v2api "github.com/yoke233/ai-workflow/internal/v2/api"
	v2core "github.com/yoke233/ai-workflow/internal/v2/core"
	v2engine "github.com/yoke233/ai-workflow/internal/v2/engine"
	v2llm "github.com/yoke233/ai-workflow/internal/v2/llm"
	v2sqlite "github.com/yoke233/ai-workflow/internal/v2/store/sqlite"
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
	if tokenRegistry.IsEmpty() {
		return fmt.Errorf("auth is required: no tokens configured in %s", secretsFilePath(configDir))
	}
	adminToken := strings.TrimSpace(secrets.AdminToken())

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
			AuthToken:  adminToken,
		},
	})

	// Ensure ContextStore is available for MCP context_* tools.
	// Prefer configured plugin; otherwise default to local context-sqlite store.
	contextStore := bootstrapSet.ContextStore
	if contextStore == nil {
		if s, err := contextsqlite.New(".ai-workflow/context.db"); err == nil {
			contextStore = s
		}
	}
	if contextStore != nil {
		defer contextStore.Close()
	}
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

	// --- V2 Engine Bootstrap ---
	v2MCPEnv := teamleader.MCPEnvConfig{
		DBPath:     expandStorePath(cfg.Store.Path),
		ServerAddr: "http://" + listenAddr,
		AuthToken:  adminToken,
	}
	_, _, v2Cleanup, v2RouteRegistrar := bootstrapV2(expandStorePath(cfg.Store.Path), bootstrapSet.RoleResolver, cfg, v2MCPEnv)
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
		Addr:                 listenAddr,
		Auth:                 tokenRegistry,
		WebhookSecret:        cfg.GitHub.WebhookSecret,
		A2AEnabled:           cfg.A2A.Enabled,
		A2AVersion:           cfg.A2A.Version,
		Frontend:             frontendFS,
		Store:                store,
		ContextStore:         contextStore,
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
		MCPDeps:          buildMCPDeps(issueManager, exec, store),
		RoleResolver:     bootstrapSet.RoleResolver,
		V2RouteRegistrar: v2RouteRegistrar,
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

// seedV2Registry seeds agent drivers and profiles into the SQLite store from TOML config.
// Uses upsert so TOML always acts as the source of truth for configured agents,
// while runtime additions via API are also persisted.
func seedV2Registry(ctx context.Context, store *v2sqlite.Store, cfg *config.Config, v1Resolver *acpclient.RoleResolver) {
	if cfg == nil {
		return
	}

	var drivers []*v2core.AgentDriver
	var profiles []*v2core.AgentProfile

	// Primary: use v2-native agent config if present.
	if len(cfg.V2.Agents.Drivers) > 0 {
		reg := v2engine.NewConfigRegistryFromConfig(cfg.V2.Agents)
		drivers, _ = reg.ListDrivers(ctx)
		profiles, _ = reg.ListProfiles(ctx)
	} else if len(cfg.EffectiveAgentProfiles()) > 0 {
		// Fallback: derive from v1 agents.profiles + roles config.
		for _, ap := range cfg.EffectiveAgentProfiles() {
			drivers = append(drivers, &v2core.AgentDriver{
				ID:            ap.Name,
				LaunchCommand: ap.LaunchCommand,
				LaunchArgs:    ap.LaunchArgs,
				Env:           ap.Env,
				CapabilitiesMax: v2core.DriverCapabilities{
					FSRead:   ap.CapabilitiesMax.FSRead,
					FSWrite:  ap.CapabilitiesMax.FSWrite,
					Terminal: ap.CapabilitiesMax.Terminal,
				},
			})
		}
		for _, rc := range cfg.Roles {
			var actions []v2core.Action
			if rc.Capabilities.FSRead {
				actions = append(actions, v2core.ActionReadContext, v2core.ActionSearchFiles)
			}
			if rc.Capabilities.FSWrite {
				actions = append(actions, v2core.ActionFSWrite)
			}
			if rc.Capabilities.Terminal {
				actions = append(actions, v2core.ActionTerminal)
			}
			role := inferV2Role(rc.Name)
			profiles = append(profiles, &v2core.AgentProfile{
				ID:             rc.Name,
				Name:           rc.Name,
				DriverID:       rc.Agent,
				Role:           role,
				ActionsAllowed: actions,
				PromptTemplate: rc.PromptTemplate,
				Session: v2core.ProfileSession{
					Reuse:    rc.Session.Reuse,
					MaxTurns: rc.Session.MaxTurns,
				},
				MCP: v2core.ProfileMCP{
					Enabled: rc.MCP.Enabled,
					Tools:   rc.MCP.Tools,
				},
			})
		}
	}

	if len(drivers) == 0 {
		slog.Warn("v2 registry: no agent config to seed")
		return
	}

	// Upsert drivers first (profiles reference them).
	for _, d := range drivers {
		if err := store.UpsertDriver(ctx, d); err != nil {
			slog.Warn("v2 registry: seed driver failed", "id", d.ID, "error", err)
		}
	}
	for _, p := range profiles {
		if err := store.UpsertProfile(ctx, p); err != nil {
			slog.Warn("v2 registry: seed profile failed", "id", p.ID, "error", err)
		}
	}
	slog.Info("v2 registry: seeded from config", "drivers", len(drivers), "profiles", len(profiles))
}

// inferV2Role maps v1 role names to v2 AgentRole.
func inferV2Role(name string) v2core.AgentRole {
	switch name {
	case "team_leader":
		return v2core.RoleLead
	case "reviewer", "aggregator":
		return v2core.RoleGate
	case "worker", "plan_parser":
		return v2core.RoleWorker
	default:
		return v2core.RoleWorker
	}
}

// bootstrapV2 creates the v2 store, event bus, engine, event persister, and API handler.
// Returns the v2 store (for lifecycle), the agent registry, a cleanup func, and a route registrar for mounting.
func bootstrapV2(v1StorePath string, roleResolver *acpclient.RoleResolver, bootstrapCfg *config.Config, mcpEnv teamleader.MCPEnvConfig) (*v2sqlite.Store, v2core.AgentRegistry, func(), func(chi.Router)) {
	v2DBPath := strings.TrimSuffix(v1StorePath, filepath.Ext(v1StorePath)) + "_v2.db"
	v2Store, err := v2sqlite.New(v2DBPath)
	if err != nil {
		slog.Error("v2 bootstrap: failed to open store", "path", v2DBPath, "error", err)
		return nil, nil, nil, nil
	}

	v2Bus := v2engine.NewMemBus()

	// Event persister: subscribe to bus → write to store.
	persister := v2engine.NewEventPersister(v2Store, v2Bus)
	if err := persister.Start(context.Background()); err != nil {
		slog.Error("v2 bootstrap: failed to start event persister", "error", err)
		v2Store.Close()
		return nil, nil, nil, nil
	}

	// Seed agent drivers/profiles from TOML config into SQLite (upsert).
	seedV2Registry(context.Background(), v2Store, bootstrapCfg, roleResolver)

	// The store itself implements AgentRegistry (SQLite-backed).
	var registry v2core.AgentRegistry = v2Store

	// Step executor: ACP agent process spawning.
	executor := v2engine.NewACPStepExecutor(v2engine.ACPExecutorConfig{
		Registry: registry,
		Store:    v2Store,
		Bus:      v2Bus,
		MCPEnv:   mcpEnv,
	})

	wsProvider := v2engine.NewCompositeProvider()

	// Optional: shared LLM client for collector + DAG generator.
	var llmClient *v2llm.Client
	var engOpts []v2engine.Option
	engOpts = append(engOpts, v2engine.WithWorkspaceProvider(wsProvider))
	if bootstrapCfg != nil {
		openaiCfg := bootstrapCfg.V2.Collector.OpenAI
		if strings.TrimSpace(openaiCfg.APIKey) != "" && strings.TrimSpace(openaiCfg.Model) != "" {
			c, err := v2llm.New(v2llm.Config{
				BaseURL:    openaiCfg.BaseURL,
				APIKey:     openaiCfg.APIKey,
				Model:      openaiCfg.Model,
				MaxRetries: bootstrapCfg.V2.Collector.MaxRetries,
			})
			if err != nil {
				slog.Warn("v2 bootstrap: LLM client disabled (invalid openai config)", "error", err)
			} else {
				llmClient = c
				engOpts = append(engOpts, v2engine.WithCollector(v2engine.NewLLMCollector(llmClient.Complete)))
				slog.Info("v2 bootstrap: LLM client enabled (collector + DAG generator)")
			}
		}
	}

	eng := v2engine.New(v2Store, v2Bus, executor, engOpts...)

	// Flow scheduler: queue + concurrency control.
	scheduler := v2engine.NewFlowScheduler(eng, v2Store, v2Bus, v2engine.FlowSchedulerConfig{
		MaxConcurrentFlows: 2,
	})
	schedCtx, schedCancel := context.WithCancel(context.Background())
	go scheduler.Start(schedCtx)

	// Recover interrupted flows from previous process.
	if n, err := v2engine.RecoverInterruptedFlows(context.Background(), v2Store, scheduler); err != nil {
		slog.Warn("v2 bootstrap: flow recovery error", "error", err)
	} else if n > 0 {
		slog.Info("v2 bootstrap: recovered interrupted flows", "count", n)
	}

	// Lead agent: direct chat entry point.
	leadAgent := v2engine.NewLeadAgent(v2engine.LeadAgentConfig{
		Registry: registry,
		Bus:      v2Bus,
	})

	// DAG generator: AI-powered step decomposition.
	var dagGen *v2engine.DAGGenerator
	if llmClient != nil {
		dagGen = v2engine.NewDAGGenerator(llmClient, registry)
	}

	handler := v2api.NewHandler(v2Store, v2Bus, eng,
		v2api.WithLeadAgent(leadAgent),
		v2api.WithScheduler(scheduler),
		v2api.WithRegistry(registry),
		v2api.WithDAGGenerator(dagGen),
	)
	registrar := func(r chi.Router) {
		handler.Register(r)
	}

	cleanup := func() {
		if leadAgent != nil {
			leadAgent.Shutdown()
		}
		schedCancel()
		scheduler.Shutdown()
		persister.Stop()
		v2Store.Close()
	}

	slog.Info("v2 engine bootstrapped", "db", v2DBPath)
	return v2Store, registry, cleanup, registrar
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
