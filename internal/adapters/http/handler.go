package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/adapters/http/server"
	"github.com/yoke233/ai-workflow/internal/adapters/llmconfig"
	"github.com/yoke233/ai-workflow/internal/adapters/sandbox"
	issueapp "github.com/yoke233/ai-workflow/internal/application/flow"
	inspectionapp "github.com/yoke233/ai-workflow/internal/application/inspection"
	probeapp "github.com/yoke233/ai-workflow/internal/application/probe"
	"github.com/yoke233/ai-workflow/internal/core"
	skillset "github.com/yoke233/ai-workflow/internal/skills"
)

// Handler is the top-level HTTP handler for the workflow API.
type Handler struct {
	store               Store
	bus                 EventBus
	engine              issueapp.Runner
	lead                LeadChatService
	scheduler           issueapp.Scheduler
	registry            core.AgentRegistry
	dagGen              DAGGenerator
	probeSvc            probeapp.Service
	skillsRoot          string
	skillGitHubImporter skillset.GitHubImporter
	sandbox             sandbox.ControlService
	llmConfig           llmconfig.ControlService
	prPrompts           issueapp.PRFlowPromptsProvider
	gitPAT              string
	textCompleter       TextCompleter
	threadPool          ThreadAgentRuntime
	inspectionEngine    *inspectionapp.Engine
	dataDir             string
}

// NewHandler creates the workflow API handler.
func NewHandler(store Store, bus EventBus, eng issueapp.Runner, opts ...HandlerOption) *Handler {
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

// WithScheduler sets the issue scheduler for queued execution.
func WithScheduler(s issueapp.Scheduler) HandlerOption {
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

// WithRunProbeService sets the execution probe service for manual/admin probe APIs.
func WithRunProbeService(service probeapp.Service) HandlerOption {
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
	return func(h *Handler) { h.sandbox = sandbox.NewReadOnlyControlService(inspector) }
}

// WithSandboxController sets the runtime sandbox support + update controller.
func WithSandboxController(controller sandbox.ControlService) HandlerOption {
	return func(h *Handler) { h.sandbox = controller }
}

// WithLLMConfigController sets the runtime LLM config controller.
func WithLLMConfigController(controller llmconfig.ControlService) HandlerOption {
	return func(h *Handler) { h.llmConfig = controller }
}

// WithPRFlowPromptsProvider sets a provider for built-in PR issue prompt text.
func WithPRFlowPromptsProvider(provider issueapp.PRFlowPromptsProvider) HandlerOption {
	return func(h *Handler) { h.prPrompts = provider }
}

// WithGitPAT sets the PAT used for pushing git tags to remote.
func WithGitPAT(pat string) HandlerOption {
	return func(h *Handler) { h.gitPAT = pat }
}

// WithTextCompleter sets the LLM text completer for title generation, etc.
func WithTextCompleter(tc TextCompleter) HandlerOption {
	return func(h *Handler) { h.textCompleter = tc }
}

// WithThreadAgentRuntime sets the thread agent runtime for real ACP sessions.
func WithThreadAgentRuntime(pool ThreadAgentRuntime) HandlerOption {
	return func(h *Handler) { h.threadPool = pool }
}

// WithInspectionEngine sets the inspection engine for self-evolving inspections.
func WithInspectionEngine(engine *inspectionapp.Engine) HandlerOption {
	return func(h *Handler) { h.inspectionEngine = engine }
}

// WithDataDir sets the data directory for file storage (uploads, etc.).
func WithDataDir(dir string) HandlerOption {
	return func(h *Handler) { h.dataDir = dir }
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

	// Resource Bindings (unified: workspace sources + I/O storage)
	r.Post("/projects/{projectID}/resources", h.createResourceBinding)
	r.Get("/projects/{projectID}/resources", h.listResourceBindings)
	r.Get("/resources/{resourceID}", h.getResourceBinding)
	r.Put("/resources/{resourceID}", h.updateResourceBinding)
	r.Delete("/resources/{resourceID}", h.deleteResourceBinding)

	// Action Resources (per-action input/output resource declarations)
	r.Post("/actions/{actionID}/resources", h.createActionResource)
	r.Get("/actions/{actionID}/resources", h.listActionResources)
	r.Delete("/action-resources/{resourceID}", h.deleteActionResource)

	// Work Item public routes.
	h.registerWorkItemRoutes(r, "/work-items")

	// Issue Attachments
	r.Get("/attachments/{attachmentID}", h.getWorkItemAttachment)
	r.Get("/attachments/{attachmentID}/download", h.downloadWorkItemAttachment)
	r.Delete("/attachments/{attachmentID}", h.deleteWorkItemAttachment)
	r.Get("/steps/{stepID}", h.getAction)
	r.Put("/steps/{stepID}", h.updateAction)
	r.Delete("/steps/{stepID}", h.deleteAction)

	// DAG Templates
	r.Post("/templates", h.createDAGTemplate)
	r.Get("/templates", h.listDAGTemplates)
	r.Get("/templates/{templateID}", h.getDAGTemplate)
	r.Put("/templates/{templateID}", h.updateDAGTemplate)
	r.Delete("/templates/{templateID}", h.deleteDAGTemplate)
	r.Post("/templates/{templateID}/create-issue", h.createWorkItemFromTemplate)

	// Step signals (human intervention)
	r.Post("/steps/{stepID}/decision", h.actionDecision)
	r.Post("/steps/{stepID}/unblock", h.actionUnblock)
	r.Get("/steps/{stepID}/signals", h.listActionSignals)
	r.Get("/pending-decisions", h.listPendingDecisions)

	// Executions
	r.Get("/steps/{stepID}/executions", h.listRuns)
	r.Get("/executions/{execID}", h.getRun)

	// Artifacts
	r.Get("/artifacts/{artifactID}", h.getDeliverable)
	r.Get("/steps/{stepID}/artifact", h.getLatestDeliverable)
	r.Get("/executions/{execID}/artifacts", h.listDeliverablesByRun)

	// Events
	r.Get("/events", h.listEvents)

	// Analytics
	r.Get("/analytics/summary", h.getAnalyticsSummary)
	r.Get("/analytics/project-errors", h.getProjectErrorRanking)
	r.Get("/analytics/bottlenecks", h.getWorkItemBottleneckActions)
	r.Get("/analytics/duration-stats", h.getRunDurationStats)
	r.Get("/analytics/error-breakdown", h.getErrorBreakdown)
	r.Get("/analytics/recent-failures", h.getRecentFailures)
	r.Get("/analytics/status-distribution", h.getWorkItemStatusDistribution)

	// Usage analytics
	r.Get("/analytics/usage", h.getUsageSummary)
	r.Get("/analytics/usage/by-project", h.getUsageByProject)
	r.Get("/analytics/usage/by-agent", h.getUsageByAgent)
	r.Get("/analytics/usage/by-profile", h.getUsageByProfile)
	r.Get("/executions/{execID}/usage", h.getUsageByRun)

	// Cron (scheduled issues)
	r.Get("/cron/issues", h.listCronWorkItems)

	// Git tags (version tagging & CI/CD trigger)
	h.registerGitTagRoutes(r)

	// Utility endpoints
	r.Post("/utils/detect-git", h.detectGitInfo)

	// WebSocket
	r.Get("/ws", h.wsEvents)

	// Agents (drivers + profiles)
	registerAgentRoutes(r, h.registry)

	// Feature Manifest (per-project feature checklist)
	r.Get("/projects/{projectID}/manifest", h.getManifest)
	r.Get("/projects/{projectID}/manifest/entries", h.listManifestEntries)
	r.Post("/projects/{projectID}/manifest/entries", h.createManifestEntry)
	r.Get("/projects/{projectID}/manifest/summary", h.getManifestSummary)
	r.Get("/projects/{projectID}/manifest/snapshot", h.getManifestSnapshot)
	r.Get("/manifest/entries/{entryID}", h.getManifestEntry)
	r.Put("/manifest/entries/{entryID}", h.updateManifestEntry)
	r.Patch("/manifest/entries/{entryID}/status", h.updateManifestEntryStatus)

	// Threads (multi-participant discussion)
	registerThreadRoutes(r, h)

	// Notifications
	registerNotificationRoutes(r, h)

	// Chat (lead agent)
	registerChatRoutes(r, h)

	// Inspections (self-evolving inspection system)
	r.Get("/inspections", h.listInspections)
	r.Get("/inspections/{inspectionID}", h.getInspection)
	r.Post("/inspections/trigger", h.triggerInspection)
	r.Get("/inspections/{inspectionID}/findings", h.listInspectionFindings)
	r.Get("/inspections/{inspectionID}/insights", h.listInspectionInsights)

	// Admin controls
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequireScope(httpx.ScopeAdmin))
		r.Get("/admin/system/llm-config", h.getLLMConfig)
		r.Put("/admin/system/sandbox-support", h.updateSandboxSupport)
		r.Put("/admin/system/llm-config", h.updateLLMConfig)
		r.Get("/executions/{execID}/tool-calls", h.listToolCallAuditsByRun)
		r.Get("/tool-calls/{auditID}", h.getToolCallAudit)
		r.Get("/executions/{execID}/audit-timeline", h.getExecutionAuditTimeline)
		r.Post("/executions/{execID}/probe", h.createRunProbe)
		r.Get("/executions/{execID}/probes", h.listExecutionProbes)
		r.Get("/executions/{execID}/probe/latest", h.getLatestRunProbe)
		r.Post("/admin/system-event", h.sendSystemEvent)
		r.Delete("/manifest/entries/{entryID}", h.deleteManifestEntry)
		registerSkillRoutes(r, h.skillsRoot, h.registry, h.skillGitHubImporter)
	})
}

func (h *Handler) registerWorkItemRoutes(r chi.Router, basePath string) {
	r.Post(basePath, h.createWorkItem)
	r.Get(basePath, h.listWorkItems)
	r.Get(basePath+"/{issueID}", h.getWorkItem)
	r.Put(basePath+"/{issueID}", h.updateWorkItem)
	r.Delete(basePath+"/{issueID}", h.deleteWorkItem)
	r.Post(basePath+"/{issueID}/bootstrap-pr", h.bootstrapPRWorkItem)
	r.Post(basePath+"/{issueID}/archive", h.archiveWorkItem)
	r.Post(basePath+"/{issueID}/unarchive", h.unarchiveWorkItem)
	r.Post(basePath+"/{issueID}/run", h.runWorkItem)
	r.Post(basePath+"/{issueID}/cancel", h.cancelWorkItem)
	r.Post(basePath+"/{issueID}/attachments", h.uploadWorkItemAttachment)
	r.Get(basePath+"/{issueID}/attachments", h.listWorkItemAttachments)
	r.Post(basePath+"/{issueID}/steps", h.createAction)
	r.Get(basePath+"/{issueID}/steps", h.listActions)
	r.Post(basePath+"/{issueID}/generate-steps", h.generateActions)
	r.Post(basePath+"/generate-title", h.generateTitle)
	r.Post(basePath+"/{issueID}/save-as-template", h.saveWorkItemAsTemplate)
	r.Get(basePath+"/{issueID}/events", h.listWorkItemEvents)
	r.Get(basePath+"/{issueID}/cron", h.getWorkItemCronStatus)
	r.Post(basePath+"/{issueID}/cron", h.setupWorkItemCron)
	r.Delete(basePath+"/{issueID}/cron", h.disableWorkItemCron)
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
