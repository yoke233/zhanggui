package web

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/teamleader"
)

// registerA2ARoutes mounts A2A JSON-RPC endpoint.
// Called inside /api/v1 group which already has TokenAuthMiddleware.
func registerA2ARoutes(r chi.Router, cfg Config) {
	if !cfg.A2AEnabled {
		r.Handle("/a2a", http.HandlerFunc(handleA2ADisabled))
		return
	}
	r.With(RequireScope(ScopeA2A)).Post("/a2a", handleA2AJSONRPC(cfg))
}

func handleA2ADisabled(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func handleA2AAgentCard(cfg Config) http.HandlerFunc {
	version := strings.TrimSpace(cfg.A2AVersion)
	if version == "" {
		version = "0.3"
	}

	return func(w http.ResponseWriter, r *http.Request) {
		card := &a2a.AgentCard{
			Name:               "ai-workflow",
			Description:        "ai-workflow a2a endpoint",
			URL:                requestAbsoluteURL(r, "/api/v1/a2a"),
			PreferredTransport: a2a.TransportProtocolJSONRPC,
			ProtocolVersion:    version,
			Capabilities:       a2a.AgentCapabilities{Streaming: true},
			DefaultInputModes:  []string{"text/plain"},
			DefaultOutputModes: []string{"text/plain"},
			Skills:             a2aSkillsForRequest(r, cfg),
			Version:            "0.1.0",
		}
		writeJSON(w, http.StatusOK, card)
	}
}

// a2aSkillsForRequest returns role-appropriate skills for the agent card.
// If an authenticated identity is present (via query token), skills are filtered by role operations.
func a2aSkillsForRequest(r *http.Request, cfg Config) []a2a.AgentSkill {
	identity, hasIdentity := resolveOptionalA2AIdentity(r, cfg)

	allSkills := []a2a.AgentSkill{
		{ID: "send", Name: "Send Message", Description: "Send a task message to the workflow agent"},
		{ID: "get", Name: "Get Task", Description: "Query task status and artifacts"},
		{ID: "cancel", Name: "Cancel Task", Description: "Cancel a running task"},
		{ID: "list", Name: "List Tasks", Description: "List tasks with filtering and pagination"},
	}

	if !hasIdentity {
		return allSkills
	}

	var filtered []a2a.AgentSkill
	for _, skill := range allSkills {
		if identity.CanA2AOperation(skill.ID) {
			filtered = append(filtered, skill)
		}
	}
	return filtered
}

// resolveOptionalA2AIdentity tries to extract identity from context or resolve via token registry.
func resolveOptionalA2AIdentity(r *http.Request, cfg Config) (A2AIdentity, bool) {
	if id, ok := A2AIdentityFromContext(r.Context()); ok {
		return id, true
	}
	if cfg.Auth == nil || cfg.Auth.IsEmpty() {
		return A2AIdentity{}, false
	}
	token := extractRequestToken(r)
	if token == "" {
		return A2AIdentity{}, false
	}
	info, ok := cfg.Auth.Lookup(token)
	if !ok {
		return A2AIdentity{}, false
	}
	return A2AIdentity{
		Submitter: info.Submitter,
		Role:      info.Role,
		Projects:  info.Projects,
	}, true
}

func handleA2AJSONRPC(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := decodeA2ARPCRequest(r)
		if err != nil {
			writeA2ARPCError(w, nil, a2aRPCInvalidRequest, "invalid request")
			return
		}
		if strings.TrimSpace(req.JSONRPC) != a2aJSONRPCVersion {
			writeA2ARPCError(w, req.ID, a2aRPCInvalidRequest, "invalid request")
			return
		}

		method := strings.TrimSpace(req.Method)
		if method == "" {
			writeA2ARPCError(w, req.ID, a2aRPCInvalidRequest, "invalid request")
			return
		}

		if !a2aCheckOperationAllowed(r, method) {
			writeA2ARPCError(w, req.ID, a2aRPCMethodNotFound, "method not found")
			return
		}

		switch method {
		case a2aMethodMessageSend:
			handleA2AMessageSend(w, r, cfg, req)
		case a2aMethodTasksGet:
			handleA2ATasksGet(w, r, cfg, req)
		case a2aMethodTasksCancel:
			handleA2ATasksCancel(w, r, cfg, req)
		case a2aMethodMessageStream:
			handleA2AMessageStream(w, r, cfg, req)
		case a2aMethodTasksList:
			handleA2ATasksList(w, r, cfg, req)
		default:
			writeA2ARPCError(w, req.ID, a2aRPCMethodNotFound, "method not found")
		}
	}
}

func handleA2AMessageSend(w http.ResponseWriter, r *http.Request, cfg Config, req a2aRPCRequest) {
	if cfg.A2ABridge == nil {
		writeA2ARPCError(w, req.ID, a2aRPCInternalError, "internal error")
		return
	}

	params, err := decodeA2AMessageSendParams(req.Params)
	if err != nil {
		writeA2ARPCError(w, req.ID, a2aRPCInvalidParams, "invalid params")
		return
	}

	projectID := a2aProjectID(params.Metadata)
	if !a2aCheckProjectAccess(r, projectID) {
		writeA2ARPCError(w, req.ID, a2aRPCProjectScopeCode, "project scope mismatch")
		return
	}

	snapshot, err := cfg.A2ABridge.SendMessage(r.Context(), teamleader.A2ASendMessageInput{
		ProjectID:    projectID,
		SessionID:    strings.TrimSpace(params.Message.ContextID),
		TaskID:       strings.TrimSpace(string(params.Message.TaskID)),
		Conversation: a2aMessageText(params.Message),
	})
	if err != nil {
		slog.Error("A2A SendMessage failed", "error", err, "project_id", a2aProjectID(params.Metadata))
		code, message := mapA2ABridgeError(err)
		writeA2ARPCError(w, req.ID, code, message)
		return
	}
	writeA2ARPCResult(w, req.ID, a2aTaskFromSnapshot(snapshot))
}

func handleA2ATasksGet(w http.ResponseWriter, r *http.Request, cfg Config, req a2aRPCRequest) {
	if cfg.A2ABridge == nil {
		writeA2ARPCError(w, req.ID, a2aRPCInternalError, "internal error")
		return
	}

	params, err := decodeA2ATaskQueryParams(req.Params)
	if err != nil {
		writeA2ARPCError(w, req.ID, a2aRPCInvalidParams, "invalid params")
		return
	}

	projectID := a2aProjectID(params.Metadata)
	if !a2aCheckProjectAccess(r, projectID) {
		writeA2ARPCError(w, req.ID, a2aRPCProjectScopeCode, "project scope mismatch")
		return
	}

	snapshot, err := cfg.A2ABridge.GetTask(r.Context(), teamleader.A2AGetTaskInput{
		ProjectID: projectID,
		TaskID:    strings.TrimSpace(string(params.ID)),
	})
	if err != nil {
		code, message := mapA2ABridgeError(err)
		writeA2ARPCError(w, req.ID, code, message)
		return
	}
	writeA2ARPCResult(w, req.ID, a2aTaskFromSnapshot(snapshot))
}

func handleA2ATasksCancel(w http.ResponseWriter, r *http.Request, cfg Config, req a2aRPCRequest) {
	if cfg.A2ABridge == nil {
		writeA2ARPCError(w, req.ID, a2aRPCInternalError, "internal error")
		return
	}

	params, err := decodeA2ATaskIDParams(req.Params)
	if err != nil {
		writeA2ARPCError(w, req.ID, a2aRPCInvalidParams, "invalid params")
		return
	}

	projectID := a2aProjectID(params.Metadata)
	if !a2aCheckProjectAccess(r, projectID) {
		writeA2ARPCError(w, req.ID, a2aRPCProjectScopeCode, "project scope mismatch")
		return
	}

	snapshot, err := cfg.A2ABridge.CancelTask(r.Context(), teamleader.A2ACancelTaskInput{
		ProjectID: projectID,
		TaskID:    strings.TrimSpace(string(params.ID)),
	})
	if err != nil {
		code, message := mapA2ABridgeError(err)
		writeA2ARPCError(w, req.ID, code, message)
		return
	}
	writeA2ARPCResult(w, req.ID, a2aTaskFromSnapshot(snapshot))
}

func handleA2ATasksList(w http.ResponseWriter, r *http.Request, cfg Config, req a2aRPCRequest) {
	if cfg.A2ABridge == nil {
		writeA2ARPCError(w, req.ID, a2aRPCInternalError, "internal error")
		return
	}

	params, err := decodeA2AListTasksParams(req.Params)
	if err != nil {
		writeA2ARPCError(w, req.ID, a2aRPCInvalidParams, "invalid params")
		return
	}

	list, err := cfg.A2ABridge.ListTasks(r.Context(), teamleader.A2AListTasksInput{
		SessionID: strings.TrimSpace(params.ContextID),
		State:     teamleader.A2ATaskState(params.Status),
		PageSize:  params.PageSize,
		PageToken: strings.TrimSpace(params.PageToken),
	})
	if err != nil {
		code, message := mapA2ABridgeError(err)
		writeA2ARPCError(w, req.ID, code, message)
		return
	}
	writeA2ARPCResult(w, req.ID, a2aListTasksResponse(list))
}

// a2aMethodToOperation maps JSON-RPC method to the operation name used in role policies.
func a2aMethodToOperation(method string) string {
	switch method {
	case a2aMethodMessageSend:
		return "send"
	case a2aMethodMessageStream:
		return "send"
	case a2aMethodTasksGet:
		return "get"
	case a2aMethodTasksCancel:
		return "cancel"
	case a2aMethodTasksList:
		return "list"
	default:
		return ""
	}
}

// a2aCheckOperationAllowed returns true if the request identity allows the given method.
// If no identity is in context (legacy mode), all operations are allowed.
func a2aCheckOperationAllowed(r *http.Request, method string) bool {
	identity, ok := A2AIdentityFromContext(r.Context())
	if !ok {
		return true
	}
	op := a2aMethodToOperation(method)
	if op == "" {
		return true // unknown methods fall through to "method not found"
	}
	return identity.CanA2AOperation(op)
}

// a2aCheckProjectAccess returns true if the request identity has access to the given project.
// If no identity is in context (legacy mode), all projects are accessible.
func a2aCheckProjectAccess(r *http.Request, projectID string) bool {
	identity, ok := A2AIdentityFromContext(r.Context())
	if !ok {
		return true
	}
	return identity.HasProjectAccess(projectID)
}

func requestAbsoluteURL(r *http.Request, path string) string {
	if r == nil {
		return path
	}

	host := strings.TrimSpace(firstForwardedValue(r.Header.Get("X-Forwarded-Host")))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		return path
	}

	scheme := strings.ToLower(strings.TrimSpace(firstForwardedValue(r.Header.Get("X-Forwarded-Proto"))))
	if scheme != "http" && scheme != "https" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	return scheme + "://" + host + path
}

func firstForwardedValue(raw string) string {
	for _, part := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
