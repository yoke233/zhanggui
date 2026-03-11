package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/adapters/http/server"
	"github.com/yoke233/ai-workflow/internal/adapters/sandbox"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	probeapp "github.com/yoke233/ai-workflow/internal/application/probe"
	"github.com/yoke233/ai-workflow/internal/core"
	skillset "github.com/yoke233/ai-workflow/internal/skills"
)

// Handler is the top-level HTTP handler for the workflow API.
type Handler struct {
	store               Store
	bus                 EventBus
	engine              flowapp.Runner
	lead                LeadChatService
	scheduler           flowapp.Scheduler
	registry            core.AgentRegistry
	dagGen              DAGGenerator
	probeSvc            probeapp.Service
	skillsRoot          string
	skillGitHubImporter skillset.GitHubImporter
	sandbox             sandbox.SupportInspector
	prPrompts           flowapp.PRFlowPromptsProvider
}

// NewHandler creates the workflow API handler.
func NewHandler(store Store, bus EventBus, eng flowapp.Runner, opts ...HandlerOption) *Handler {
	h := &Handler{store: store, bus: bus, engine: eng}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// HandlerOption configures the workflow API handler.
type HandlerOption func(*Handler)

// WithLeadAgent sets the lead agent for chat endpoints.
func WithLeadAgent(lead LeadChatService) HandlerOption {
	return func(h *Handler) { h.lead = lead }
}

// WithScheduler sets the flow scheduler for queued execution.
func WithScheduler(s flowapp.Scheduler) HandlerOption {
	return func(h *Handler) { h.scheduler = s }
}

// WithRegistry sets the agent registry for driver/profile management.
func WithRegistry(r core.AgentRegistry) HandlerOption {
	return func(h *Handler) { h.registry = r }
}

// WithDAGGenerator sets the DAG generator for AI-powered step decomposition.
func WithDAGGenerator(g DAGGenerator) HandlerOption {
	return func(h *Handler) { h.dagGen = g }
}

// WithExecutionProbeService sets the execution probe service for manual/admin probe APIs.
func WithExecutionProbeService(service probeapp.Service) HandlerOption {
	return func(h *Handler) { h.probeSvc = service }
}

// WithSkillsRoot sets the filesystem root directory for managing ai-workflow skills.
// This should point to the global shared skills repository, not a sandbox-local skills dir.
func WithSkillsRoot(root string) HandlerOption {
	return func(h *Handler) { h.skillsRoot = root }
}

// WithSkillGitHubImporter sets the importer used by POST /skills/import/github.
func WithSkillGitHubImporter(importer skillset.GitHubImporter) HandlerOption {
	return func(h *Handler) { h.skillGitHubImporter = importer }
}

// WithSandboxInspector sets the runtime sandbox support inspector.
func WithSandboxInspector(inspector sandbox.SupportInspector) HandlerOption {
	return func(h *Handler) { h.sandbox = inspector }
}

// WithPRFlowPromptsProvider sets a provider for built-in PR flow prompt text.
func WithPRFlowPromptsProvider(provider flowapp.PRFlowPromptsProvider) HandlerOption {
	return func(h *Handler) { h.prPrompts = provider }
}

// Register mounts all workflow routes onto the given chi router.
// Caller is responsible for mounting this under a prefix like /api.
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

	// Issues
	r.Post("/issues", h.createIssue)
	r.Get("/issues", h.listIssues)
	r.Get("/issues/{issueID}", h.getIssue)
	r.Put("/issues/{issueID}", h.updateIssue)
	r.Delete("/issues/{issueID}", h.deleteIssue)

	// Flows
	r.Post("/flows", h.createFlow)
	r.Get("/flows", h.listFlows)
	r.Get("/flows/{flowID}", h.getFlow)
	r.Post("/flows/{flowID}/bootstrap-pr", h.bootstrapPRFlow)
	r.Post("/flows/{flowID}/archive", h.archiveFlow)
	r.Post("/flows/{flowID}/unarchive", h.unarchiveFlow)
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

	// Analytics
	r.Get("/analytics/summary", h.getAnalyticsSummary)
	r.Get("/analytics/project-errors", h.getProjectErrorRanking)
	r.Get("/analytics/bottlenecks", h.getFlowBottleneckSteps)
	r.Get("/analytics/duration-stats", h.getExecutionDurationStats)
	r.Get("/analytics/error-breakdown", h.getErrorBreakdown)
	r.Get("/analytics/recent-failures", h.getRecentFailures)
	r.Get("/analytics/status-distribution", h.getFlowStatusDistribution)

	// Cron (scheduled flows)
	r.Get("/cron/flows", h.listCronFlows)
	r.Get("/flows/{flowID}/cron", h.getFlowCronStatus)
	r.Post("/flows/{flowID}/cron", h.setupFlowCron)
	r.Delete("/flows/{flowID}/cron", h.disableFlowCron)

	// WebSocket
	r.Get("/ws", h.wsEvents)

	// Agents (drivers + profiles)
	registerAgentRoutes(r, h.registry)

	// Chat (lead agent)
	registerChatRoutes(r, h.lead)

	// Admin controls
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequireScope(httpx.ScopeAdmin))
		r.Post("/executions/{execID}/probe", h.createExecutionProbe)
		r.Get("/executions/{execID}/probes", h.listExecutionProbes)
		r.Get("/executions/{execID}/probe/latest", h.getLatestExecutionProbe)
		r.Post("/admin/system-event", h.sendSystemEvent)
		registerSkillRoutes(r, h.skillsRoot, h.registry, h.skillGitHubImporter)
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
