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

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
	"github.com/yoke233/ai-workflow/internal/eventbus"
	gitops "github.com/yoke233/ai-workflow/internal/git"
	ghwebhook "github.com/yoke233/ai-workflow/internal/github"
	pluginfactory "github.com/yoke233/ai-workflow/internal/plugins/factory"
	"github.com/yoke233/ai-workflow/internal/teamleader"
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

type serverIssueManager interface {
	web.IssueManager
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type depSchedulerIssueAdapter struct {
	scheduler *teamleader.DepScheduler
}

func (a *depSchedulerIssueAdapter) Start(ctx context.Context) error {
	if a == nil || a.scheduler == nil {
		return errors.New("issue scheduler is not configured")
	}
	return a.scheduler.Start(ctx)
}

func (a *depSchedulerIssueAdapter) Stop(ctx context.Context) error {
	if a == nil || a.scheduler == nil {
		return nil
	}
	return a.scheduler.Stop(ctx)
}

func (a *depSchedulerIssueAdapter) RecoverExecutingIssues(ctx context.Context) error {
	if a == nil || a.scheduler == nil {
		return errors.New("issue scheduler is not configured")
	}
	return a.scheduler.RecoverExecutingIssues(ctx, "")
}

func (a *depSchedulerIssueAdapter) StartIssue(ctx context.Context, issue *core.Issue) error {
	if a == nil || a.scheduler == nil {
		return errors.New("issue scheduler is not configured")
	}
	return a.scheduler.ScheduleIssues(ctx, []*core.Issue{issue})
}

type teamLeaderIssueManagerAdapter struct {
	manager teamLeaderIssueService
	store   core.Store
}

type teamLeaderIssueService interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	CreateIssues(ctx context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error)
	SubmitForReview(ctx context.Context, issueIDs []string) error
	ApplyIssueAction(ctx context.Context, issueID, action, feedback string) (*core.Issue, error)
}

func (a *teamLeaderIssueManagerAdapter) Start(ctx context.Context) error {
	if a == nil || a.manager == nil {
		return errors.New("issue manager is not configured")
	}
	return a.manager.Start(ctx)
}

func (a *teamLeaderIssueManagerAdapter) Stop(ctx context.Context) error {
	if a == nil || a.manager == nil {
		return nil
	}
	return a.manager.Stop(ctx)
}

func (a *teamLeaderIssueManagerAdapter) CreateIssues(ctx context.Context, input web.IssueCreateInput) ([]core.Issue, error) {
	if a == nil || a.manager == nil {
		return nil, errors.New("issue manager is not configured")
	}

	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		return nil, errors.New("project id is required")
	}

	failPolicy := input.FailPolicy
	if failPolicy == "" {
		failPolicy = core.FailBlock
	}

	created, err := a.manager.CreateIssues(ctx, teamleader.CreateIssuesInput{
		ProjectID: projectID,
		SessionID: strings.TrimSpace(input.SessionID),
		Issues: []teamleader.CreateIssueSpec{
			{
				Title:      resolveIssueTitle(input),
				Body:       buildIssueBody(input),
				Template:   "standard",
				AutoMerge:  input.AutoMerge,
				Labels:     resolveIssueLabels(input),
				FailPolicy: failPolicy,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	out := make([]core.Issue, 0, len(created))
	for i := range created {
		if created[i] == nil {
			continue
		}
		out = append(out, *created[i])
	}
	return out, nil
}

func (a *teamLeaderIssueManagerAdapter) SubmitForReview(ctx context.Context, issueID string, _ web.IssueReviewInput) (*core.Issue, error) {
	if a == nil || a.manager == nil {
		return nil, errors.New("issue manager is not configured")
	}
	id := strings.TrimSpace(issueID)
	if id == "" {
		return nil, errors.New("issue id is required")
	}
	if err := a.manager.SubmitForReview(ctx, []string{id}); err != nil {
		return nil, err
	}
	if a.store == nil {
		return &core.Issue{ID: id}, nil
	}
	return a.store.GetIssue(id)
}

func (a *teamLeaderIssueManagerAdapter) ApplyIssueAction(ctx context.Context, issueID string, action web.IssueAction) (*core.Issue, error) {
	if a == nil || a.manager == nil {
		return nil, errors.New("issue manager is not configured")
	}
	feedback := ""
	if action.Feedback != nil {
		feedback = strings.TrimSpace(action.Feedback.Detail)
	}
	return a.manager.ApplyIssueAction(ctx, issueID, action.Action, feedback)
}

func resolveIssueTitle(input web.IssueCreateInput) string {
	if trimmed := strings.TrimSpace(input.Name); trimmed != "" {
		return trimmed
	}
	if len(input.SourceFiles) == 1 {
		return fmt.Sprintf("Plan from %s", strings.TrimSpace(input.SourceFiles[0]))
	}
	if len(input.SourceFiles) > 1 {
		return fmt.Sprintf("Plan from %d files", len(input.SourceFiles))
	}
	return "Plan from chat session"
}

func resolveIssueLabels(input web.IssueCreateInput) []string {
	labels := []string{"plan"}
	if len(input.SourceFiles) > 0 {
		labels = append(labels, "from-files")
	}
	return labels
}

func buildIssueBody(input web.IssueCreateInput) string {
	parts := make([]string, 0, 3)

	conversation := strings.TrimSpace(input.Request.Conversation)
	if conversation != "" {
		parts = append(parts, "## Conversation\n\n"+conversation)
	}

	if len(input.SourceFiles) > 0 {
		var b strings.Builder
		b.WriteString("## Source Files\n\n")
		for _, file := range input.SourceFiles {
			path := strings.TrimSpace(file)
			if path == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(path)
			b.WriteString("\n")
		}
		for _, file := range input.SourceFiles {
			path := strings.TrimSpace(file)
			if path == "" {
				continue
			}
			content, ok := input.FileContents[path]
			if !ok {
				continue
			}
			b.WriteString("\n### ")
			b.WriteString(path)
			b.WriteString("\n\n```text\n")
			b.WriteString(strings.TrimSpace(content))
			b.WriteString("\n```\n")
		}
		parts = append(parts, strings.TrimSpace(b.String()))
	}

	if len(parts) == 0 {
		return "Auto-created issue from chat session."
	}
	return strings.Join(parts, "\n\n")
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
		bus *eventbus.Bus,
		teamLeaderCfg config.TeamLeaderConfig,
		roleBinds config.RoleBindings,
	) (serverIssueManager, error) {
		if exec == nil {
			return nil, errors.New("executor is required for issue manager")
		}
		if bootstrapSet == nil {
			return nil, errors.New("bootstrap set is required for issue manager")
		}
		agentPlugin, err := selectTeamLeaderAgentPlugin(bootstrapSet.Agents)
		if err != nil {
			return nil, err
		}
		agent, err := teamleader.NewAgent(agentPlugin, bootstrapSet.Runtime)
		if err != nil {
			return nil, err
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
		depScheduler.SetStageRoles(roleBinds.Run.StageRoles)

		opts := make([]teamleader.ManagerOption, 0, 1)
		if bootstrapSet.ReviewGate != nil {
			opts = append(opts, teamleader.WithReviewGate(bootstrapSet.ReviewGate))
		}
		manager, err := teamleader.NewManager(
			bootstrapSet.Store,
			agent,
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
	exec.SetRunstageRoles(cfg.RoleBinds.Run.StageRoles)
	exec.SetACPHandlerFactory(&acpHandlerFactoryAdapter{})

	recoveryOnce.Do(func() {
		go func() {
			if recErr := exec.RecoverActiveRuns(context.Background()); recErr != nil {
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
	if err := store.CreateProject(p); err != nil {
		return err
	}
	if p.DefaultBranch == "" {
		p.DefaultBranch = gitops.DetectDefaultBranch(p.RepoPath)
		if p.DefaultBranch != "" {
			_ = store.UpdateProject(p)
		}
	}
	return nil
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

func cmdRunCreate(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: ai-flow Run create <project-id> <name> <description> [template]")
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

	p, err := exec.CreateRun(args[0], args[1], args[2], template)
	if err != nil {
		return err
	}
	fmt.Printf("Run created: %s (template: %s, stages: %d)\n", p.ID, p.Template, len(p.Stages))
	return nil
}

func cmdRunstart(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai-flow Run start <Run-id>")
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
	fmt.Printf("Run enqueued: %s\n", args[0])
	return nil
}

func cmdRunStatus(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai-flow Run status <Run-id>")
	}

	_, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	p, err := store.GetRun(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Run: %s\n", p.ID)
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

func cmdRunList(args []string) error {
	_, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PROJECT\tRun\tSTATUS\tSTAGE\tQUEUED")

	if len(args) >= 1 && strings.TrimSpace(args[0]) != "" {
		Runs, err := store.ListRuns(args[0], core.RunFilter{Limit: 200})
		if err != nil {
			return err
		}
		for _, p := range Runs {
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
		Runs, err := store.ListRuns(project.ID, core.RunFilter{Limit: 200})
		if err != nil {
			return err
		}
		for _, p := range Runs {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				p.ProjectID, p.ID, p.Status, p.CurrentStage, formatTime(p.QueuedAt))
		}
	}
	return w.Flush()
}

func cmdRunAction(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: ai-flow Run action <Run-id> <approve|reject|modify|skip|rerun|change_role|abort|pause|resume> [--stage <stage>] [--role <role>] [--message <text>]")
	}

	actionType, err := parseActionType(args[1])
	if err != nil {
		return err
	}

	action := core.RunAction{
		RunID: args[0],
		Type:  actionType,
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
	fmt.Printf("Action applied: Run=%s action=%s\n", action.RunID, action.Type)
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

	issueManager, err := newServerIssueManager(exec, bootstrapSet, bus, cfg.TeamLeader, cfg.RoleBinds)
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
	})
	var merger teamleader.PRMerger
	if cfg.GitHub.Enabled {
		merger = ghwebhook.NewPRLifecycle(store, bootstrapSet.SCM)
	}
	autoMerger := teamleader.NewAutoMergeHandler(store, bus, merger)

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
				if evt.RunID != "" && !isTransientChunkEvent(evt) {
					if err := store.SaveRunEvent(core.RunEvent{
						RunID:     evt.RunID,
						ProjectID: evt.ProjectID,
						IssueID:   evt.IssueID,
						EventType: string(evt.Type),
						Stage:     string(evt.Stage),
						Agent:     evt.Agent,
						Data:      evt.Data,
						Error:     evt.Error,
						CreatedAt: evt.Timestamp,
					}); err != nil {
						slog.Warn("failed to persist run event", "run_id", evt.RunID, "type", evt.Type, "error", err)
					}
				}
				autoMerger.OnEvent(ctx, evt)
			}
		}
	}()

	apiServer := newAPIServer(web.Config{
		Addr:              listenAddr,
		AuthEnabled:       cfg.Server.AuthEnabled,
		BearerToken:       cfg.Server.AuthToken,
		WebhookSecret:     cfg.GitHub.WebhookSecret,
		A2AEnabled:        cfg.A2A.Enabled,
		A2AToken:          cfg.A2A.Token,
		A2AVersion:        cfg.A2A.Version,
		Store:             store,
		A2ABridge:         a2aBridge,
		IssueManager:      issueManager,
		ChatAssistant:     chatAssistant,
		EventPublisher:    bus,
		RunExec:           exec,
		RunstageRoles:     cfg.RoleBinds.Run.StageRoles,
		IssueParserRoleID: strings.TrimSpace(cfg.RoleBinds.PlanParser.Role),
		SCM:               bootstrapSet.SCM,
		Hub:               hub,
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
		managerStopErr := issueManager.Stop(stopCtx)
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
	if err := issueManager.Stop(stopCtx); err != nil && shutdownErr == nil {
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

func selectTeamLeaderAgentPlugin(agents map[string]core.AgentPlugin) (core.AgentPlugin, error) {
	if len(agents) == 0 {
		return nil, errors.New("no agent plugins configured for TeamLeader manager")
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
		return nil, errors.New("no non-nil agent plugins configured for TeamLeader manager")
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
		cfg.Scheduler.MaxProjectRuns,
	), nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}

// acpHandlerFactoryAdapter bridges engine.ACPHandlerFactory to teamleader.ACPHandler.
type acpHandlerFactoryAdapter struct{}

func (f *acpHandlerFactoryAdapter) NewHandler(cwd string, publisher engine.ACPEventPublisher) acpproto.Client {
	return teamleader.NewACPHandler(cwd, "", publisher)
}

func (f *acpHandlerFactoryAdapter) SetPermissionPolicy(handler acpproto.Client, policy []acpclient.PermissionRule) {
	if h, ok := handler.(*teamleader.ACPHandler); ok {
		h.SetPermissionPolicy(policy)
	}
}

// isTransientChunkEvent returns true for high-frequency streaming chunk events
// that should NOT be persisted to run_events (they are broadcast via WS only).
func isTransientChunkEvent(evt core.Event) bool {
	if evt.Type != core.EventAgentOutput {
		return false
	}
	switch evt.Data["type"] {
	case "agent_message_chunk", "agent_thought_chunk", "user_message_chunk",
		"available_commands_update", "current_mode_update",
		"config_option_update", "session_info_update":
		return true
	}
	return false
}
