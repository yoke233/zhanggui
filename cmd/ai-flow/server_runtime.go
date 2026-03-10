package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/appdata"
	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
	ghwebhook "github.com/yoke233/ai-workflow/internal/github"
	contextsqlite "github.com/yoke233/ai-workflow/internal/plugins/context-sqlite"
	pluginfactory "github.com/yoke233/ai-workflow/internal/plugins/factory"
	"github.com/yoke233/ai-workflow/internal/teamleader"
	"github.com/yoke233/ai-workflow/internal/web"
)

type serverLaunchInputs struct {
	cfg           *config.Config
	configDir     string
	tokenRegistry *web.TokenRegistry
	adminToken    string
	githubTokens  v2GitHubTokens
	listenAddr    string
	frontendFS    fs.FS
}

func prepareServerLaunch(cliPort int) (*serverLaunchInputs, error) {
	cfg, err := loadBootstrapConfig()
	if err != nil {
		return nil, err
	}

	configDir, _ := appdata.ResolveDataDir()
	secrets, err := config.LoadSecrets(secretsFilePath(configDir))
	if err != nil {
		return nil, fmt.Errorf("load secrets: %w", err)
	}
	tokenRegistry := web.NewTokenRegistry(secrets.Tokens)
	if tokenRegistry.IsEmpty() {
		return nil, fmt.Errorf("auth is required: no tokens configured in %s", secretsFilePath(configDir))
	}

	listenAddr := buildServerAddress(cfg.Server.Host, resolveServerPort(cliPort, cfg.Server.Port))
	frontendFS, err := resolveServerFrontendFS()
	if err != nil {
		return nil, err
	}

	return &serverLaunchInputs{
		cfg:           cfg,
		configDir:     configDir,
		tokenRegistry: tokenRegistry,
		adminToken:    strings.TrimSpace(secrets.AdminToken()),
		githubTokens: v2GitHubTokens{
			CommitPAT: strings.TrimSpace(secrets.CommitPAT),
			MergePAT:  strings.TrimSpace(secrets.MergePAT),
		},
		listenAddr: listenAddr,
		frontendFS: frontendFS,
	}, nil
}

type serverRuntime struct {
	scheduler              serverScheduler
	issueManager           serverIssueManager
	a2aBridge              web.A2ABridge
	hub                    *web.Hub
	chatAssistant          web.ChatAssistant
	contextStore           core.ContextStore
	decomposePlanner       *teamleader.DecomposePlanner
	proposalIssueCreator   web.ProposalIssueCreator
	autoMerger             *teamleader.AutoMergeHandler
	tlTriageHandler        *teamleader.TLTriageHandler
	decomposeHandler       *teamleader.DecomposeHandler
	childCompletionHandler *teamleader.ChildCompletionHandler
	wsBroadcaster          *wsBroadcaster
	eventPersister         *eventPersister
}

func setupServerRuntime(
	ctx context.Context,
	exec *engine.Executor,
	bootstrapSet *pluginfactory.BootstrapSet,
	bus core.EventBus,
	cfg *config.Config,
	listenAddr string,
	adminToken string,
) (*serverRuntime, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	store := bootstrapSet.Store

	rt := &serverRuntime{}

	scheduler, err := newServerScheduler(exec, store)
	if err != nil {
		return nil, err
	}
	if err := scheduler.Start(ctx); err != nil {
		return nil, err
	}
	rt.scheduler = scheduler

	issueManager, err := newServerIssueManager(exec, bootstrapSet, bus, cfg.Scheduler.Watchdog, cfg.TeamLeader, cfg.RoleBinds)
	if err != nil {
		rt.stop(context.Background())
		return nil, err
	}
	if issueManager == nil {
		rt.stop(context.Background())
		return nil, errors.New("issue manager is not configured")
	}
	if err := issueManager.Start(ctx); err != nil {
		rt.issueManager = issueManager
		rt.stop(context.Background())
		return nil, err
	}
	rt.issueManager = issueManager

	if adapter, ok := issueManager.(*teamLeaderIssueManagerAdapter); ok {
		bridge, bridgeErr := teamleader.NewA2ABridge(store, adapter.manager)
		if bridgeErr != nil {
			slog.Warn("A2ABridge creation failed, A2A endpoint will be unavailable", "error", bridgeErr)
		} else {
			rt.a2aBridge = bridge
			slog.Info("A2ABridge created successfully")
		}
	} else {
		slog.Warn("issueManager is not teamLeaderIssueManagerAdapter, A2A bridge unavailable", "type", fmt.Sprintf("%T", issueManager))
	}

	rt.hub = web.NewHub()
	if bootstrapSet.RoleResolver == nil {
		rt.stop(context.Background())
		return nil, errors.New("chat assistant requires role resolver")
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
	rt.chatAssistant = chatAssistant

	rt.contextStore = bootstrapSet.ContextStore
	if rt.contextStore == nil {
		if dataDir, err := appdata.ResolveDataDir(); err == nil {
			if s, err := contextsqlite.New(filepath.Join(dataDir, "context.db")); err == nil {
				rt.contextStore = s
			}
		}
	}

	if chatAssistant != nil {
		rt.decomposePlanner = teamleader.NewDecomposePlanner(func(ctx context.Context, projectID, systemPrompt, userMessage string) (string, error) {
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

	var merger teamleader.PRMerger
	if cfg.GitHub.Enabled {
		merger = ghwebhook.NewPRLifecycle(store, bootstrapSet.SCM)
	}
	rt.autoMerger = teamleader.NewAutoMergeHandler(store, bus, merger)
	rt.tlTriageHandler = teamleader.NewTLTriageHandler(store, bus, 3)
	rt.decomposeHandler = teamleader.NewDecomposeHandler(store, bus, func(ctx context.Context, parent *core.Issue) ([]teamleader.DecomposeSpec, error) {
		return nil, fmt.Errorf("decomposer agent not configured for issue %s", parent.ID)
	})
	if adapter, ok := issueManager.(*teamLeaderIssueManagerAdapter); ok {
		rt.decomposeHandler.SetReviewSubmitter(adapter.manager)
		rt.proposalIssueCreator = adapter.manager
	}
	rt.childCompletionHandler = teamleader.NewChildCompletionHandler(store, bus)
	rt.wsBroadcaster = newWSBroadcaster(rt.hub, bus)
	rt.eventPersister = newEventPersister(store, bus)

	if err := rt.wsBroadcaster.Start(ctx); err != nil {
		rt.stop(context.Background())
		return nil, fmt.Errorf("start ws broadcaster: %w", err)
	}
	if err := rt.eventPersister.Start(ctx); err != nil {
		rt.stop(context.Background())
		return nil, fmt.Errorf("start event persister: %w", err)
	}
	if err := rt.autoMerger.Start(ctx); err != nil {
		rt.stop(context.Background())
		return nil, fmt.Errorf("start auto merger: %w", err)
	}
	if err := rt.tlTriageHandler.Start(ctx); err != nil {
		rt.stop(context.Background())
		return nil, fmt.Errorf("start tl triage handler: %w", err)
	}
	if err := rt.decomposeHandler.Start(ctx); err != nil {
		rt.stop(context.Background())
		return nil, fmt.Errorf("start decompose handler: %w", err)
	}
	if err := rt.childCompletionHandler.Start(ctx); err != nil {
		rt.stop(context.Background())
		return nil, fmt.Errorf("start child completion handler: %w", err)
	}

	return rt, nil
}

func (rt *serverRuntime) stop(ctx context.Context) error {
	if rt == nil {
		return nil
	}

	var errs []error
	stopCtx := ctx
	var cancel context.CancelFunc
	if stopCtx == nil {
		stopCtx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
	}
	if rt.contextStore != nil {
		errs = append(errs, rt.contextStore.Close())
	}
	if rt.issueManager != nil {
		errs = append(errs, rt.issueManager.Stop(stopCtx))
	}
	if rt.scheduler != nil {
		errs = append(errs, rt.scheduler.Stop(stopCtx))
	}
	stopHandlers(stopCtx, rt.childCompletionHandler, rt.decomposeHandler, rt.tlTriageHandler, rt.autoMerger, rt.eventPersister, rt.wsBroadcaster)
	return errors.Join(errs...)
}
