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
	"github.com/user/ai-workflow/internal/core"
	"github.com/user/ai-workflow/internal/secretary"
	webassets "github.com/user/ai-workflow/web"
)

// PlanManager defines the task-plan orchestration APIs required by plan handlers.
type PlanManager interface {
	CreateDraft(ctx context.Context, input secretary.CreateDraftInput) (*core.TaskPlan, error)
	SubmitReview(ctx context.Context, planID string, input secretary.ReviewInput) (*core.TaskPlan, error)
	ApplyPlanAction(ctx context.Context, planID string, action secretary.PlanAction) (*core.TaskPlan, error)
}

// PipelineExecutor defines pipeline human-action entrypoints used by web handlers.
type PipelineExecutor interface {
	ApplyAction(ctx context.Context, action core.PipelineAction) error
}

// WebhookDeliveryReplayer replays failed webhook deliveries by delivery id.
type WebhookDeliveryReplayer interface {
	ReplayByDeliveryID(ctx context.Context, deliveryID string) (bool, error)
}

// Config controls web server behavior.
type Config struct {
	Addr                   string
	AuthEnabled            bool
	BearerToken            string
	WebhookSecret          string
	AllowedOrigins         []string
	Frontend               fs.FS
	Store                  core.Store
	PlanManager            PlanManager
	ChatAssistant          ChatAssistant
	PipelineExec           PipelineExecutor
	PipelineStageRoles     map[string]string
	WebhookReplayer        WebhookDeliveryReplayer
	Hub                    *Hub
	ProjectRepoProvisioner ProjectRepoProvisioner
	ProjectReposRoot       string
	Logger                 *log.Logger
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
	r.Get("/api/v1/health", handleHealth)
	webhookReplayer := registerWebhookRoutes(r, cfg.Store, cfg.PipelineExec, strings.TrimSpace(cfg.WebhookSecret), cfg.PipelineStageRoles)
	if cfg.WebhookReplayer != nil {
		webhookReplayer = cfg.WebhookReplayer
	}
	r.Route("/api/v1", func(r chi.Router) {
		if cfg.AuthEnabled {
			r.Use(BearerAuthMiddleware(cfg.BearerToken))
		}
		r.Get("/stats", handleStats)
		registerProjectRoutes(r, cfg.Store, hub, projectRepoProvisioner)
		registerPipelineRoutes(r, cfg.Store, cfg.PipelineExec, cfg.PipelineStageRoles)
		registerChatRoutes(r, cfg.Store, cfg.ChatAssistant)
		registerPlanRoutes(r, cfg.Store, cfg.PlanManager)
		registerTaskRoutes(r, cfg.Store)
		registerAdminOpsRoutes(r, cfg.Store, cfg.BearerToken, webhookReplayer)
		r.Get("/ws", hub.HandleWS)
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
