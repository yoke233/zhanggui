package web

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/mcpserver"
	"github.com/yoke233/ai-workflow/internal/teamleader"
	webassets "github.com/yoke233/ai-workflow/web"
)

// IssueCreateRequest defines request context for issue generation.
type IssueCreateRequest struct {
	Conversation string
	ProjectName  string
	RepoPath     string
	Role         string
	WorkDir      string
}

// IssueCreateInput defines issue generation input.
type IssueCreateInput struct {
	ProjectID    string
	SessionID    string
	Name         string
	FailPolicy   core.FailurePolicy
	AutoMerge    *bool
	Request      IssueCreateRequest
	SourceFiles  []string
	FileContents map[string]string
}

// IssueReviewInput defines issue review context input.
type IssueReviewInput struct {
	Conversation   string
	ProjectContext string
	FileContents   map[string]string
}

// IssueFeedback carries human feedback for issue action.
type IssueFeedback struct {
	Category          string
	Detail            string
	ExpectedDirection string
}

// IssueAction defines review decision action for issues.
type IssueAction struct {
	Action   string
	Feedback *IssueFeedback
}

// IssueManager defines issue orchestration APIs required by issue handlers.
type IssueManager interface {
	CreateIssues(ctx context.Context, input IssueCreateInput) ([]core.Issue, error)
	SubmitForReview(ctx context.Context, issueID string, input IssueReviewInput) (*core.Issue, error)
	ApplyIssueAction(ctx context.Context, issueID string, action IssueAction) (*core.Issue, error)
}

type DecomposePlanner interface {
	Plan(ctx context.Context, projectID, prompt string) (*core.DecomposeProposal, error)
}

type ProposalIssueCreator interface {
	CreateIssues(ctx context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error)
	ConfirmCreatedIssues(ctx context.Context, issueIDs []string, feedback string) ([]*core.Issue, error)
}

// RunExecutor defines Run human-action entrypoints used by web handlers.
type RunExecutor interface {
	ApplyAction(ctx context.Context, action core.RunAction) error
}

// StageSessionManager provides stage-level ACP session lifecycle operations.
type StageSessionManager interface {
	GetStageSessionStatus(runID string, stage core.StageID) core.StageSessionStatus
	WakeStageSession(ctx context.Context, runID string, stage core.StageID) (string, error)
	PromptStageSession(ctx context.Context, runID string, stage core.StageID, message string) error
}

// A2ABridge defines A2A task bridge methods.
type A2ABridge interface {
	SendMessage(ctx context.Context, input teamleader.A2ASendMessageInput) (*teamleader.A2ATaskSnapshot, error)
	GetTask(ctx context.Context, input teamleader.A2AGetTaskInput) (*teamleader.A2ATaskSnapshot, error)
	CancelTask(ctx context.Context, input teamleader.A2ACancelTaskInput) (*teamleader.A2ATaskSnapshot, error)
	ListTasks(ctx context.Context, input teamleader.A2AListTasksInput) (*teamleader.A2ATaskList, error)
}

// WebhookDeliveryReplayer replays failed webhook deliveries by delivery id.
type WebhookDeliveryReplayer interface {
	ReplayByDeliveryID(ctx context.Context, deliveryID string) (bool, error)
}

// Config controls web server behavior.
type Config struct {
	Addr                   string
	Token                  string         // legacy single token; upgraded to Auth with wildcard scope
	Auth                   *TokenRegistry // scope-based token auth for all endpoints
	WebhookSecret          string
	AllowedOrigins         []string
	A2AEnabled             bool
	A2AVersion             string
	A2ABridge              A2ABridge
	Frontend               fs.FS
	Store                  core.Store
	ContextStore           core.ContextStore
	IssueManager           IssueManager
	DecomposePlanner       DecomposePlanner
	ProposalIssueCreator   ProposalIssueCreator
	ChatAssistant          ChatAssistant
	EventPublisher         chatEventPublisher
	RunExec                RunExecutor
	StageSessionMgr        StageSessionManager
	RunstageRoles          map[string]string
	IssueParserRoleID      string
	WebhookReplayer        WebhookDeliveryReplayer
	SCM                    core.SCM
	Hub                    *Hub
	ProjectRepoProvisioner ProjectRepoProvisioner
	ProjectReposRoot       string
	Logger                 *log.Logger
	RestartFunc            func() // triggers graceful server restart; nil = not supported
	MCPServerOpts          MCPServerOptions
	MCPDeps                MCPDeps
	RoleResolver           *acpclient.RoleResolver
	V2RouteRegistrar       func(chi.Router) // optional: registers v2 API routes under /api/v2
}

// MCPDeps carries business-layer dependencies for MCP write tools.
type MCPDeps struct {
	IssueManager mcpserver.IssueManager
	RunExecutor  mcpserver.RunExecutor
}

// MCPServerOptions carries configuration for the embedded MCP server.
type MCPServerOptions struct {
	DevMode    bool
	SourceRoot string
	ServerAddr string
	ConfigDir  string
	DBPath     string
}

// Server wraps the HTTP server and router for API serving.
type Server struct {
	cfg        Config
	router     chi.Router
	httpServer *http.Server
	logger     *log.Logger
}

// NewServer builds a chi-based API server with middleware and routes.
func NewServer(cfg Config) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = log.New(os.Stdout, "[web] ", log.LstdFlags)
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.Auth == nil && strings.TrimSpace(cfg.Token) != "" {
		cfg.Auth = NewTokenRegistry(map[string]config.TokenEntry{
			"legacy": {
				Token:  strings.TrimSpace(cfg.Token),
				Scopes: []string{ScopeAll},
			},
		})
	}
	hub := cfg.Hub
	if hub == nil {
		hub = NewHub()
	}
	projectRepoProvisioner := cfg.ProjectRepoProvisioner
	if projectRepoProvisioner == nil {
		projectRepoProvisioner = NewProjectRepoProvisioner(cfg.ProjectReposRoot)
	}
	frontendFS := cfg.Frontend
	if frontendFS == nil {
		embeddedFS, err := webassets.DistFS()
		if err != nil {
			logger.Printf("frontend assets unavailable: %v", err)
		} else {
			frontendFS = embeddedFS
		}
	}

	r := chi.NewRouter()
	r.Use(RecoveryMiddleware(logger))
	r.Use(LoggingMiddleware(logger))
	r.Use(CORSMiddleware(cfg.AllowedOrigins))

	r.Get("/health", handleHealth)

	// Public routes (no auth)
	if cfg.A2AEnabled {
		r.Get("/.well-known/agent-card.json", handleA2AAgentCard(cfg))
	}

	// V2 API routes (Flow/Step/Execution model)
	if cfg.V2RouteRegistrar != nil {
		r.Route("/api/v2", func(r chi.Router) {
			if cfg.Auth != nil && !cfg.Auth.IsEmpty() {
				r.Use(TokenAuthMiddleware(cfg.Auth))
			}
			cfg.V2RouteRegistrar(r)
		})
	}

	// All API routes under /api/v1 with unified auth
	r.Route("/api/v1", func(r chi.Router) {
		// Health — public (no auth)
		r.Get("/health", handleHealth)

		// Webhook — uses its own HMAC auth, no token auth
		webhookReplayer := registerWebhookRoutes(r, cfg.Store, cfg.RunExec, strings.TrimSpace(cfg.WebhookSecret), cfg.RunstageRoles, cfg.SCM)
		if cfg.WebhookReplayer != nil {
			webhookReplayer = cfg.WebhookReplayer
		}

		// Authenticated routes
		r.Group(func(r chi.Router) {
			if cfg.Auth != nil && !cfg.Auth.IsEmpty() {
				r.Use(TokenAuthMiddleware(cfg.Auth))
			}

			// A2A endpoint
			registerA2ARoutes(r, cfg)

			// MCP endpoint
			registerMCPRoutes(r, cfg)

			// REST API
			issueManager := cfg.IssueManager
			issueParserRoleID := strings.TrimSpace(cfg.IssueParserRoleID)
			registerV1Routes(r, cfg.Store, issueManager, cfg.DecomposePlanner, cfg.ProposalIssueCreator, issueParserRoleID, cfg.RunExec, cfg.StageSessionMgr, cfg.RunstageRoles,
				hub, projectRepoProvisioner, cfg.ChatAssistant, cfg.EventPublisher, webhookReplayer, cfg.RestartFunc, cfg.RoleResolver)
		})
	})
	if frontendFS != nil {
		spa := newSPAFallbackHandler(frontendFS)
		r.NotFound(spa.ServeHTTP)
	}

	httpSrv := &http.Server{
		Addr:    cfg.Addr,
		Handler: r,
	}

	return &Server{
		cfg:        cfg,
		router:     r,
		httpServer: httpSrv,
		logger:     logger,
	}
}

// Handler returns the configured router for embedding/tests.
func (s *Server) Handler() http.Handler {
	return s.router
}

// Start runs the HTTP server and blocks until shutdown or error.
func (s *Server) Start() error {
	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

type spaFallbackHandler struct {
	files fs.FS
}

func newSPAFallbackHandler(frontendFS fs.FS) spaFallbackHandler {
	return spaFallbackHandler{
		files: frontendFS,
	}
}

func (h spaFallbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}

	cleanPath := cleanRequestPath(r.URL.Path)
	if isAPIRoute(cleanPath) {
		http.NotFound(w, r)
		return
	}

	staticPath := requestPathToStaticFile(cleanPath)
	if staticPath != "" && fileExists(h.files, staticPath) {
		http.ServeFileFS(w, r, h.files, staticPath)
		return
	}
	if shouldReturnNotFoundForMissingStatic(cleanPath, staticPath) {
		http.NotFound(w, r)
		return
	}

	http.ServeFileFS(w, r, h.files, "index.html")
}

func isAPIRoute(requestPath string) bool {
	const apiPrefix = "/api"
	if len(requestPath) < len(apiPrefix) {
		return false
	}
	if !strings.EqualFold(requestPath[:len(apiPrefix)], apiPrefix) {
		return false
	}
	return len(requestPath) == len(apiPrefix) || requestPath[len(apiPrefix)] == '/'
}

func shouldReturnNotFoundForMissingStatic(cleanPath string, staticPath string) bool {
	if staticPath == "" {
		return false
	}
	if cleanPath == "/assets" || strings.HasPrefix(cleanPath, "/assets/") {
		return true
	}
	return isStaticAssetExtension(path.Ext(staticPath))
}

func isStaticAssetExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".avif", ".css", ".eot", ".gif", ".html", ".ico", ".jpeg", ".jpg", ".js", ".json", ".map", ".mjs", ".otf", ".png", ".svg", ".ttf", ".txt", ".wasm", ".webmanifest", ".webp", ".woff", ".woff2", ".xml":
		return true
	default:
		return false
	}
}

func requestPathToStaticFile(cleanPath string) string {
	return strings.TrimPrefix(cleanPath, "/")
}

func cleanRequestPath(requestPath string) string {
	return path.Clean("/" + requestPath)
}

func fileExists(frontendFS fs.FS, name string) bool {
	f, err := frontendFS.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return false
	}
	return !info.IsDir()
}
