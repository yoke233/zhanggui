package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/v2/core"
	"github.com/yoke233/ai-workflow/internal/v2/engine"
	v2sandbox "github.com/yoke233/ai-workflow/internal/v2/sandbox"
	"github.com/yoke233/ai-workflow/internal/web"
)

// Handler is the top-level HTTP handler for the v2 API.
type Handler struct {
	store      core.Store
	bus        core.EventBus
	engine     *engine.FlowEngine
	lead       *engine.LeadAgent
	scheduler  *engine.FlowScheduler
	registry   core.AgentRegistry
	dagGen     *engine.DAGGenerator
	skillsRoot string
	sandbox    v2sandbox.SupportInspector
}

// NewHandler creates the v2 API handler.
func NewHandler(store core.Store, bus core.EventBus, eng *engine.FlowEngine, opts ...HandlerOption) *Handler {
	h := &Handler{store: store, bus: bus, engine: eng}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// HandlerOption configures the v2 API Handler.
type HandlerOption func(*Handler)

// WithLeadAgent sets the lead agent for chat endpoints.
func WithLeadAgent(lead *engine.LeadAgent) HandlerOption {
	return func(h *Handler) { h.lead = lead }
}

// WithScheduler sets the flow scheduler for queued execution.
func WithScheduler(s *engine.FlowScheduler) HandlerOption {
	return func(h *Handler) { h.scheduler = s }
}

// WithRegistry sets the agent registry for driver/profile management.
func WithRegistry(r core.AgentRegistry) HandlerOption {
	return func(h *Handler) { h.registry = r }
}

// WithDAGGenerator sets the DAG generator for AI-powered step decomposition.
func WithDAGGenerator(g *engine.DAGGenerator) HandlerOption {
	return func(h *Handler) { h.dagGen = g }
}

// WithSkillsRoot sets the filesystem root directory for managing ai-workflow skills.
// This should point to the global shared skills repository, not a sandbox-local skills dir.
func WithSkillsRoot(root string) HandlerOption {
	return func(h *Handler) { h.skillsRoot = root }
}

// WithSandboxInspector sets the runtime sandbox support inspector.
func WithSandboxInspector(inspector v2sandbox.SupportInspector) HandlerOption {
	return func(h *Handler) { h.sandbox = inspector }
}

// Register mounts all v2 routes onto the given chi router.
// Caller is responsible for mounting this under a prefix like /api/v2.
func (h *Handler) Register(r chi.Router) {
	// Scheduler stats
	r.Get("/stats", h.getStats)
	r.Get("/scheduler/stats", h.getSchedulerStats)
	r.Get("/system/sandbox-support", h.getSandboxSupport)

	// Projects
	r.Post("/projects", h.createProject)
	r.Get("/projects", h.listProjects)
	r.Get("/projects/{projectID}", h.getProject)
	r.Put("/projects/{projectID}", h.updateProject)
	r.Delete("/projects/{projectID}", h.deleteProject)

	// Resource Bindings
	r.Post("/projects/{projectID}/resources", h.createResourceBinding)
	r.Get("/projects/{projectID}/resources", h.listResourceBindings)
	r.Get("/resources/{resourceID}", h.getResourceBinding)
	r.Delete("/resources/{resourceID}", h.deleteResourceBinding)

	// Flows
	r.Post("/flows", h.createFlow)
	r.Get("/flows", h.listFlows)
	r.Get("/flows/{flowID}", h.getFlow)
	r.Post("/flows/{flowID}/bootstrap-pr", h.bootstrapPRFlow)
	r.Post("/flows/{flowID}/run", h.runFlow)
	r.Post("/flows/{flowID}/cancel", h.cancelFlow)

	// Steps
	r.Post("/flows/{flowID}/steps", h.createStep)
	r.Get("/flows/{flowID}/steps", h.listSteps)
	r.Get("/steps/{stepID}", h.getStep)
	r.Put("/steps/{stepID}", h.updateStep)
	r.Delete("/steps/{stepID}", h.deleteStep)

	// DAG generation (AI-powered)
	r.Post("/flows/{flowID}/generate-steps", h.generateSteps)

	// Executions
	r.Get("/steps/{stepID}/executions", h.listExecutions)
	r.Get("/executions/{execID}", h.getExecution)

	// Artifacts
	r.Get("/artifacts/{artifactID}", h.getArtifact)
	r.Get("/steps/{stepID}/artifact", h.getLatestArtifact)
	r.Get("/executions/{execID}/artifacts", h.listArtifactsByExec)

	// Briefings
	r.Get("/briefings/{briefingID}", h.getBriefing)
	r.Get("/steps/{stepID}/briefing", h.getBriefingByStep)

	// Events
	r.Get("/events", h.listEvents)
	r.Get("/flows/{flowID}/events", h.listFlowEvents)

	// WebSocket
	r.Get("/ws", h.wsEvents)

	// Agents (drivers + profiles)
	registerAgentRoutes(r, h.registry)

	// Chat (lead agent)
	registerChatRoutes(r, h.lead)

	// Admin controls
	r.Group(func(r chi.Router) {
		r.Use(web.RequireScope(web.ScopeAdmin))
		r.Post("/admin/system-event", h.sendSystemEvent)
		registerSkillRoutes(r, h.skillsRoot)
	})
}

// urlParamInt64 extracts an int64 from a chi URL path parameter.
func urlParamInt64(r *http.Request, name string) (int64, bool) {
	s := chi.URLParam(r, name)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// queryInt parses an optional int query parameter with a default value.
func queryInt(r *http.Request, name string, defaultVal int) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}

// queryInt64 parses an optional int64 query parameter. Returns false if absent or invalid.
func queryInt64(r *http.Request, name string) (int64, bool) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
