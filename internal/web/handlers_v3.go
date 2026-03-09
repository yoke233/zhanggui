package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
)

// issueListResponse is the paginated list response for issues.
type issueListResponse struct {
	Items  []core.Issue `json:"items"`
	Total  int          `json:"total"`
	Offset int          `json:"offset"`
}

func normalizeIssuesForAPI(items []core.Issue) []core.Issue {
	if len(items) == 0 {
		return []core.Issue{}
	}
	out := make([]core.Issue, len(items))
	for i := range items {
		normalized := normalizeIssueForAPI(&items[i])
		if normalized == nil {
			out[i] = core.Issue{}
			continue
		}
		out[i] = *normalized
	}
	return out
}

func normalizeIssueForAPI(issue *core.Issue) *core.Issue {
	if issue == nil {
		return nil
	}
	clone := *issue
	clone.Labels = normalizeStringSlice(issue.Labels)
	clone.Attachments = normalizeStringSlice(issue.Attachments)
	clone.DependsOn = normalizeStringSlice(issue.DependsOn)
	clone.Blocks = normalizeStringSlice(issue.Blocks)
	return &clone
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

// registerV1Routes registers REST API routes (called inside /api/v1 group).
func registerV1Routes(
	r chi.Router,
	store core.Store,
	issueManager IssueManager,
	decomposePlanner DecomposePlanner,
	proposalIssueCreator ProposalIssueCreator,
	issueParserRoleID string,
	executor RunExecutor,
	stageSessionMgr StageSessionManager,
	stageRoleBindings map[string]string,
	hub *Hub,
	provisioner ProjectRepoProvisioner,
	chatAssistant ChatAssistant,
	eventPublisher chatEventPublisher,
	webhookReplayer WebhookDeliveryReplayer,
	restartFunc func(),
	roleResolver *acpclient.RoleResolver,
) {
	r.Get("/stats", handleStats)
	r.Get("/ws", hub.HandleWS)

	if roleResolver != nil {
		agentH := &agentHandlers{resolver: roleResolver}
		r.With(RequireScope(ScopeChatRead)).Get("/agents", agentH.list)
	}

	registerProjectRoutes(r, store, hub, provisioner)
	registerRepoRoutes(r, store)
	registerChatRoutes(r, store, chatAssistant, eventPublisher)
	registerDecomposeRoutes(r, store, decomposePlanner, proposalIssueCreator)
	r.Group(func(r chi.Router) {
		r.Use(RequireScope(ScopeAdmin))
		registerAdminOpsRoutes(r, store, webhookReplayer, restartFunc, hub)
	})

	issueHandlers := &v2IssueHandlers{store: store}
	r.With(RequireScope(ScopeIssuesRead)).Get("/issues", issueHandlers.listIssues)
	r.With(RequireScope(ScopeIssuesRead)).Get("/issues/{id}", issueHandlers.getIssue)
	r.With(RequireScope(ScopeIssuesRead)).Get("/projects/{projectId}/issues/{issueId}/timeline", issueHandlers.listIssueTimeline)

	r.Get("/workflow-profiles", handleListWorkflowProfiles)
	r.Get("/workflow-profiles/{type}", handleGetWorkflowProfile)

	runHandlers := &v2RunHandlers{store: store}
	r.With(RequireScope(ScopeRunsRead)).Get("/runs", runHandlers.listRuns)
	r.With(RequireScope(ScopeRunsRead)).Get("/runs/{id}", runHandlers.getRun)
	r.With(RequireScope(ScopeRunsRead)).Get("/runs/{id}/events", runHandlers.listRunEvents)
	r.With(RequireScope(ScopeRunsRead)).Get("/runs/{id}/stage-summary", runHandlers.runStageSummary)
	r.With(RequireScope(ScopeRunsRead)).Get("/runs/{id}/checkpoints", runHandlers.listCheckpoints)

	if stageSessionMgr != nil {
		ssHandlers := &stageSessionHandlers{mgr: stageSessionMgr}
		r.With(RequireScope(ScopeRunsRead)).Get("/runs/{id}/stages/{stage}/session", ssHandlers.getStatus)
		r.With(RequireScope(ScopeRunsWrite)).Post("/runs/{id}/stages/{stage}/session/wake", ssHandlers.wake)
		r.With(RequireScope(ScopeRunsWrite)).Post("/runs/{id}/stages/{stage}/session/prompt", ssHandlers.prompt)
	}

	gateH := &gateHandlers{store: store}
	if resolver, ok := issueManager.(interface {
		ResolveGate(ctx context.Context, issueID, gateName, action, reason string) (*core.Issue, error)
	}); ok {
		gateH.resolver = resolver
	}
	r.With(RequireScope(ScopeIssuesRead)).Get("/issues/{id}/gates", gateH.listGates)
	r.With(RequireScope(ScopeIssuesWrite)).Post("/issues/{id}/gates/{gateName}/resolve", gateH.resolveGate)

	decH := &decisionHandlers{store: store}
	r.With(RequireScope(ScopeIssuesRead)).Get("/issues/{id}/decisions", decH.listIssueDecisions)
	r.With(RequireScope(ScopeIssuesRead)).Get("/decisions/{id}", decH.getDecision)
}

// stageSessionHandlers handles stage-level ACP session operations.
type stageSessionHandlers struct {
	mgr StageSessionManager
}

func (h *stageSessionHandlers) getStatus(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(chi.URLParam(r, "id"))
	stage := core.StageID(strings.TrimSpace(chi.URLParam(r, "stage")))
	if runID == "" || stage == "" {
		writeAPIError(w, http.StatusBadRequest, "run_id and stage are required", "PARAMS_REQUIRED")
		return
	}
	status := h.mgr.GetStageSessionStatus(runID, stage)
	writeJSON(w, http.StatusOK, status)
}

func (h *stageSessionHandlers) wake(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(chi.URLParam(r, "id"))
	stage := core.StageID(strings.TrimSpace(chi.URLParam(r, "stage")))
	if runID == "" || stage == "" {
		writeAPIError(w, http.StatusBadRequest, "run_id and stage are required", "PARAMS_REQUIRED")
		return
	}
	sessionID, err := h.mgr.WakeStageSession(r.Context(), runID, stage)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error(), "WAKE_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"session_id": sessionID})
}

func (h *stageSessionHandlers) prompt(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(chi.URLParam(r, "id"))
	stage := core.StageID(strings.TrimSpace(chi.URLParam(r, "stage")))
	if runID == "" || stage == "" {
		writeAPIError(w, http.StatusBadRequest, "run_id and stage are required", "PARAMS_REQUIRED")
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid request body", "INVALID_BODY")
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		writeAPIError(w, http.StatusBadRequest, "message is required", "MESSAGE_REQUIRED")
		return
	}
	if err := h.mgr.PromptStageSession(r.Context(), runID, stage, body.Message); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error(), "PROMPT_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
