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
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
	"github.com/yoke233/ai-workflow/internal/eventbus"
	pluginfactory "github.com/yoke233/ai-workflow/internal/plugins/factory"
	"github.com/yoke233/ai-workflow/internal/secretary"
	"github.com/yoke233/ai-workflow/internal/web"
)

var recoveryOnce sync.Once

const defaultServerPort = 8080

type apiServer interface {
	Start() error
	Shutdown(ctx context.Context) error
}

type serverScheduler interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type serverPlanManager interface {
	web.PlanManager
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
	newServerPlanManager = func(
		exec *engine.Executor,
		bootstrapSet *pluginfactory.BootstrapSet,
		bus *eventbus.Bus,
		secretaryCfg config.SecretaryConfig,
		roleBinds config.RoleBindings,
	) (serverPlanManager, error) {
		if exec == nil {
			return nil, errors.New("executor is required for plan manager")
		}
		if bootstrapSet == nil {
			return nil, errors.New("bootstrap set is required for plan manager")
		}
		agentPlugin, err := selectSecretaryAgentPlugin(bootstrapSet.Agents)
		if err != nil {
			return nil, err
		}
		agent, err := secretary.NewAgent(agentPlugin, bootstrapSet.Runtime)
		if err != nil {
			return nil, err
		}

		reviewPanel, err := secretary.NewDefaultReviewOrchestratorFromBindings(
			bootstrapSet.Store,
			secretary.ReviewRoleBindingInput{
				Reviewers:  cloneStringMap(roleBinds.ReviewOrchestrator.Reviewers),
				Aggregator: roleBinds.ReviewOrchestrator.Aggregator,
			},
			bootstrapSet.RoleResolver,
		)
		if err != nil {
			return nil, fmt.Errorf("build review orchestrator from role bindings: %w", err)
		}
		if secretaryCfg.ReviewOrchestrator.MaxRounds > 0 {
			reviewPanel.MaxRounds = secretaryCfg.ReviewOrchestrator.MaxRounds
		}
		runTaskPipeline := func(ctx context.Context, pipelineID string) error {
			ok, err := bootstrapSet.Store.TryMarkPipelineRunning(pipelineID, core.StatusCreated)
			if err != nil {
				return err
			}
			if !ok {
				// Pipeline is already claimed by another scheduler loop.
				return nil
			}
			return exec.RunScheduled(ctx, pipelineID)
		}
		depScheduler := secretary.NewDepScheduler(
			bootstrapSet.Store,
			bus,
			runTaskPipeline,
			bootstrapSet.Tracker,
			secretaryCfg.DAGScheduler.MaxConcurrentTasks,
		)

		opts := make([]secretary.ManagerOption, 0, 1)
		if bootstrapSet.ReviewGate != nil {
			opts = append(opts, secretary.WithReviewGate(bootstrapSet.ReviewGate))
		}
		return secretary.NewManager(bootstrapSet.Store, agent, reviewPanel, depScheduler, opts...)
	}
)

func bootstrap() (*engine.Executor, core.Store, error) {
	exec, bootstrapSet, _, err := bootstrapWithEventBus()
	if err != nil {
		return nil, nil, err
	}
	return exec, bootstrapSet.Store, nil
}

func bootstrapWithEventBus() (*engine.Executor, *pluginfactory.BootstrapSet, *eventbus.Bus, error) {
	cfg, err := loadBootstrapConfig()
	if err != nil {
		return nil, nil, nil, err
	}

	bootstrapSet, err := pluginfactory.BuildFromConfig(*cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	if bootstrapSet.Workspace == nil {
		return nil, nil, nil, errors.New("workspace plugin is not configured in bootstrap set")
	}

	bus := eventbus.New()
	logger := slog.Default()
	exec := engine.NewExecutor(bootstrapSet.Store, bus, bootstrapSet.Agents, bootstrapSet.Runtime, logger)
	exec.SetRoleResolver(bootstrapSet.RoleResolver)
	exec.SetWorkspace(bootstrapSet.Workspace)
	exec.SetPipelineStageRoles(cfg.RoleBinds.Pipeline.StageRoles)

	recoveryOnce.Do(func() {
		go func() {
			if recErr := exec.RecoverActivePipelines(context.Background()); recErr != nil {
				logger.Error("recovery failed", "error", recErr)
			}
		}()
	})

	return exec, bootstrapSet, bus, nil
}

func loadBootstrapConfig() (*config.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dataDir := filepath.Join(home, ".ai-workflow")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(dataDir, "config.yaml")
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		return config.LoadGlobal(cfgPath)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}

	cfg := config.Defaults()
	if err := config.ApplyEnvOverrides(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func cmdProjectAdd(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: ai-flow project add <id> <repo-path>")
	}
	_, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	p := &core.Project{ID: args[0], Name: args[0], RepoPath: args[1]}
	return store.CreateProject(p)
}

func cmdProjectList() error {
	_, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	projects, err := store.ListProjects(core.ProjectFilter{})
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tPATH")
	for _, p := range projects {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", p.ID, p.Name, p.RepoPath)
	}
	return w.Flush()
}

func cmdPipelineCreate(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: ai-flow pipeline create <project-id> <name> <description> [template]")
	}

	exec, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	template := "standard"
	if len(args) > 3 {
		template = args[3]
	}

	p, err := exec.CreatePipeline(args[0], args[1], args[2], template)
	if err != nil {
		return err
	}
	fmt.Printf("Pipeline created: %s (template: %s, stages: %d)\n", p.ID, p.Template, len(p.Stages))
	return nil
}

func cmdPipelineStart(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai-flow pipeline start <pipeline-id>")
	}

	exec, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	scheduler, err := buildScheduler(exec, store)
	if err != nil {
		return err
	}
	if err := scheduler.Enqueue(args[0]); err != nil {
		return err
	}
	fmt.Printf("Pipeline enqueued: %s\n", args[0])
	return nil
}

func cmdPipelineStatus(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai-flow pipeline status <pipeline-id>")
	}

	_, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	p, err := store.GetPipeline(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Pipeline: %s\n", p.ID)
	fmt.Printf("Status:   %s\n", p.Status)
	fmt.Printf("Stage:    %s\n", p.CurrentStage)
	fmt.Printf("Template: %s\n", p.Template)
	return nil
}

func cmdProjectScan(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai-flow project scan <root>")
	}
	root := args[0]

	_, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	repos, err := scanGitRepos(root)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		fmt.Printf("No git repositories found under %s\n", root)
		return nil
	}

	existingProjects, err := store.ListProjects(core.ProjectFilter{})
	if err != nil {
		return err
	}
	existingRepo := map[string]struct{}{}
	usedIDs := map[string]struct{}{}
	for _, p := range existingProjects {
		existingRepo[filepath.Clean(p.RepoPath)] = struct{}{}
		usedIDs[p.ID] = struct{}{}
	}

	added := 0
	skipped := 0
	for _, repoPath := range repos {
		cleanPath := filepath.Clean(repoPath)
		if _, ok := existingRepo[cleanPath]; ok {
			skipped++
			continue
		}

		id := uniqueProjectID(filepath.Base(cleanPath), usedIDs)
		project := &core.Project{
			ID:       id,
			Name:     filepath.Base(cleanPath),
			RepoPath: cleanPath,
		}
		if err := store.CreateProject(project); err != nil {
			return err
		}
		existingRepo[cleanPath] = struct{}{}
		usedIDs[id] = struct{}{}
		added++
	}

	fmt.Printf("Scan complete: discovered=%d added=%d skipped=%d\n", len(repos), added, skipped)
	return nil
}

func scanGitRepos(root string) ([]string, error) {
	var repos []string
	seen := map[string]struct{}{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		switch d.Name() {
		case ".worktrees":
			return filepath.SkipDir
		case ".git":
			repo := filepath.Dir(path)
			clean := filepath.Clean(repo)
			if _, ok := seen[clean]; !ok {
				seen[clean] = struct{}{}
				repos = append(repos, clean)
			}
			return filepath.SkipDir
		default:
			return nil
		}
	})
	return repos, err
}

func uniqueProjectID(base string, used map[string]struct{}) string {
	clean := strings.ToLower(strings.TrimSpace(base))
	clean = strings.ReplaceAll(clean, " ", "-")
	if clean == "" {
		clean = "project"
	}
	if _, exists := used[clean]; !exists {
		return clean
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", clean, i)
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
}

func cmdPipelineList(args []string) error {
	_, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PROJECT\tPIPELINE\tSTATUS\tSTAGE\tQUEUED")

	if len(args) >= 1 && strings.TrimSpace(args[0]) != "" {
		pipelines, err := store.ListPipelines(args[0], core.PipelineFilter{Limit: 200})
		if err != nil {
			return err
		}
		for _, p := range pipelines {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				p.ProjectID, p.ID, p.Status, p.CurrentStage, formatTime(p.QueuedAt))
		}
		return w.Flush()
	}

	projects, err := store.ListProjects(core.ProjectFilter{})
	if err != nil {
		return err
	}
	for _, project := range projects {
		pipelines, err := store.ListPipelines(project.ID, core.PipelineFilter{Limit: 200})
		if err != nil {
			return err
		}
		for _, p := range pipelines {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				p.ProjectID, p.ID, p.Status, p.CurrentStage, formatTime(p.QueuedAt))
		}
	}
	return w.Flush()
}

func cmdPipelineAction(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: ai-flow pipeline action <pipeline-id> <approve|reject|modify|skip|rerun|change_role|abort|pause|resume> [--stage <stage>] [--role <role>] [--message <text>]")
	}

	actionType, err := parseActionType(args[1])
	if err != nil {
		return err
	}

	action := core.PipelineAction{
		PipelineID: args[0],
		Type:       actionType,
	}

	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--stage":
			i++
			if i >= len(args) {
				return fmt.Errorf("--stage requires a value")
			}
			action.Stage = core.StageID(args[i])
		case "--role":
			i++
			if i >= len(args) {
				return fmt.Errorf("--role requires a value")
			}
			action.Role = strings.TrimSpace(args[i])
		case "--message":
			i++
			if i >= len(args) {
				return fmt.Errorf("--message requires a value")
			}
			action.Message = strings.Join(args[i:], " ")
			i = len(args)
		default:
			// Backward-compatible positional tail as message.
			action.Message = strings.Join(args[i:], " ")
			i = len(args)
		}
	}

	exec, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	if err := exec.ApplyAction(context.Background(), action); err != nil {
		return err
	}
	fmt.Printf("Action applied: pipeline=%s action=%s\n", action.PipelineID, action.Type)
	return nil
}

func parseActionType(raw string) (core.HumanActionType, error) {
	switch core.HumanActionType(strings.ToLower(strings.TrimSpace(raw))) {
	case core.ActionApprove,
		core.ActionReject,
		core.ActionModify,
		core.ActionSkip,
		core.ActionRerun,
		core.ActionChangeRole,
		core.ActionAbort,
		core.ActionPause,
		core.ActionResume:
		return core.HumanActionType(strings.ToLower(strings.TrimSpace(raw))), nil
	default:
		return "", fmt.Errorf("unknown action type: %s", raw)
	}
}

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

	port = resolveServerPort(port, cfg.Server.Port)
	listenAddr := buildServerAddress(cfg.Server.Host, port)

	scheduler, err := newServerScheduler(exec, store)
	if err != nil {
		return err
	}
	if err := scheduler.Start(ctx); err != nil {
		return err
	}

	planManager, err := newServerPlanManager(exec, bootstrapSet, bus, cfg.Secretary, cfg.RoleBinds)
	if err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		stopErr := scheduler.Stop(stopCtx)
		return errors.Join(err, stopErr)
	}
	if planManager == nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		stopErr := scheduler.Stop(stopCtx)
		return errors.Join(errors.New("plan manager is not configured"), stopErr)
	}
	if err := planManager.Start(ctx); err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		managerStopErr := planManager.Stop(stopCtx)
		stopErr := scheduler.Stop(stopCtx)
		return errors.Join(err, managerStopErr, stopErr)
	}

	hub := web.NewHub()
	if bootstrapSet.RoleResolver == nil {
		return errors.New("chat assistant requires role resolver")
	}
	var runEventRecorder secretary.ChatRunEventRecorder
	if recorder, ok := store.(secretary.ChatRunEventRecorder); ok {
		runEventRecorder = recorder
	}
	chatAssistant := web.NewACPChatAssistantWithDeps(web.ACPChatAssistantDeps{
		DefaultRoleID:    strings.TrimSpace(cfg.RoleBinds.Secretary.Role),
		RoleResolver:     bootstrapSet.RoleResolver,
		EventPublisher:   bus,
		RunEventRecorder: runEventRecorder,
	})
	sub := bus.Subscribe()
	bridgeDone := make(chan struct{})
	go func() {
		defer close(bridgeDone)
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-sub:
				if !ok {
					return
				}
				hub.BroadcastCoreEvent(evt)
			}
		}
	}()

	apiServer := newAPIServer(web.Config{
		Addr:               listenAddr,
		AuthEnabled:        cfg.Server.AuthEnabled,
		BearerToken:        cfg.Server.AuthToken,
		A2AEnabled:         cfg.A2A.Enabled,
		A2AToken:           cfg.A2A.Token,
		A2AVersion:         cfg.A2A.Version,
		WebhookSecret:      cfg.GitHub.WebhookSecret,
		Store:              store,
		PlanManager:        planManager,
		ChatAssistant:      chatAssistant,
		EventPublisher:     bus,
		PipelineExec:       exec,
		PipelineStageRoles: cfg.RoleBinds.Pipeline.StageRoles,
		PlanParserRoleID:   strings.TrimSpace(cfg.RoleBinds.PlanParser.Role),
		Hub:                hub,
	})

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- apiServer.Start()
	}()

	fmt.Printf("Server started on %s (ws: /api/v1/ws). Press Ctrl+C to stop.\n", listenAddr)

	select {
	case serverErr := <-serverErrCh:
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		managerStopErr := planManager.Stop(stopCtx)
		stopErr := scheduler.Stop(stopCtx)
		bus.Unsubscribe(sub)
		<-bridgeDone
		return errors.Join(serverErr, managerStopErr, stopErr)
	case <-ctx.Done():
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var shutdownErr error
	if err := apiServer.Shutdown(stopCtx); err != nil {
		shutdownErr = err
	}
	if err := planManager.Stop(stopCtx); err != nil && shutdownErr == nil {
		shutdownErr = err
	}
	if err := scheduler.Stop(stopCtx); err != nil && shutdownErr == nil {
		shutdownErr = err
	}
	bus.Unsubscribe(sub)
	<-bridgeDone

	select {
	case serverErr := <-serverErrCh:
		if serverErr != nil && shutdownErr == nil {
			shutdownErr = serverErr
		}
	case <-stopCtx.Done():
	}

	return shutdownErr
}

func selectSecretaryAgentPlugin(agents map[string]core.AgentPlugin) (core.AgentPlugin, error) {
	if len(agents) == 0 {
		return nil, errors.New("no agent plugins configured for secretary manager")
	}
	if agent, ok := agents["codex"]; ok && agent != nil {
		return agent, nil
	}
	if agent, ok := agents["claude"]; ok && agent != nil {
		return agent, nil
	}

	names := make([]string, 0, len(agents))
	for name, plugin := range agents {
		if plugin == nil {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, errors.New("no non-nil agent plugins configured for secretary manager")
	}
	sort.Strings(names)
	return agents[names[0]], nil
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

func buildServerAddress(host string, port int) string {
	trimmedHost := strings.TrimSpace(host)
	if trimmedHost == "" {
		return fmt.Sprintf(":%d", port)
	}
	return net.JoinHostPort(trimmedHost, strconv.Itoa(port))
}

func cmdSchedulerRun() error {
	exec, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	scheduler, err := buildScheduler(exec, store)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := scheduler.Start(ctx); err != nil {
		return err
	}
	fmt.Println("Scheduler started. Press Ctrl+C to stop.")
	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return scheduler.Stop(stopCtx)
}

func cmdSchedulerOnce() error {
	exec, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	scheduler, err := buildScheduler(exec, store)
	if err != nil {
		return err
	}
	if err := scheduler.RunOnce(context.Background()); err != nil {
		return err
	}
	fmt.Println("Scheduler run-once completed.")
	return nil
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
		cfg.Scheduler.MaxProjectPipelines,
	), nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}
