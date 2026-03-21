package appcmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	sqlitestore "github.com/yoke233/zhanggui/internal/adapters/store/sqlite"
	"github.com/yoke233/zhanggui/internal/core"
	"github.com/yoke233/zhanggui/internal/platform/appdata"
)

// RunMCPServe starts the MCP server over stdio.
// It reads action context from environment variables and exposes tools based on action type.
func RunMCPServe(args []string) error {
	dataDir, err := appdata.ResolveDataDir()
	if err == nil {
		closeLog, logErr := InitAppLogger(dataDir, "mcp-serve")
		if logErr != nil {
			return logErr
		}
		defer closeLog()
	}

	dbPath := strings.TrimSpace(os.Getenv("AI_WORKFLOW_DB_PATH"))
	if dbPath == "" {
		return fmt.Errorf("AI_WORKFLOW_DB_PATH is required")
	}
	actionID := envInt64("AI_WORKFLOW_ACTION_ID")
	workItemID := envInt64("AI_WORKFLOW_WORK_ITEM_ID")
	actionType := strings.TrimSpace(os.Getenv("AI_WORKFLOW_ACTION_TYPE"))
	runID := envInt64("AI_WORKFLOW_RUN_ID")

	store, err := sqlitestore.New(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer store.Close()

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "ai-workflow-action",
		Version: "0.1.0",
	}, nil)

	handler := &mcpActionHandler{
		store:      store,
		actionID:   actionID,
		workItemID: workItemID,
		runID:      runID,
	}

	// action_context — available for all action types.
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "action_context",
		Description: "Get the run context: work item details, upstream action results, and your own rework history.",
	}, handler.handleActionContext)

	switch actionType {
	case "exec":
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "action_complete",
			Description: "Declare that you have completed the task. Provide a structured summary of what you did. Call this BEFORE ending your response.",
		}, handler.handleActionComplete)

		mcp.AddTool(srv, &mcp.Tool{
			Name:        "action_need_help",
			Description: "Signal that you cannot complete the task and need human assistance. Explain what you tried, what went wrong, and what kind of help you need.",
		}, handler.handleActionNeedHelp)
	case "gate":
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "gate_approve",
			Description: "Approve the gate. Call this when the review passes all criteria.",
		}, handler.handleGateApprove)

		mcp.AddTool(srv, &mcp.Tool{
			Name:        "gate_reject",
			Description: "Reject the gate. Call this when the review finds problems that must be fixed.",
		}, handler.handleGateReject)
	}

	slog.Info("mcp-serve: starting", "action_id", actionID, "work_item_id", workItemID, "action_type", actionType, "run_id", runID)
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}

func envInt64(key string) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

// mcpActionHandler implements the MCP tool handlers.
type mcpActionHandler struct {
	store      core.Store
	actionID   int64
	workItemID int64
	runID      int64
}

type actionContextInput struct{}

type actionContextOutput struct {
	Action            map[string]any   `json:"action"`
	WorkItem          map[string]any   `json:"work_item"`
	UpstreamArtifacts []map[string]any `json:"upstream_artifacts"`
	ReworkHistory     []any            `json:"rework_history"`
	Signals           []map[string]any `json:"signals,omitempty"`
}

func (h *mcpActionHandler) handleActionContext(ctx context.Context, req *mcp.CallToolRequest, _ actionContextInput) (*mcp.CallToolResult, actionContextOutput, error) {
	action, err := h.store.GetAction(ctx, h.actionID)
	if err != nil {
		return nil, actionContextOutput{}, fmt.Errorf("get action: %w", err)
	}
	workItem, err := h.store.GetWorkItem(ctx, h.workItemID)
	if err != nil {
		return nil, actionContextOutput{}, fmt.Errorf("get work item: %w", err)
	}

	// Collect upstream artifacts.
	allActions, _ := h.store.ListActionsByWorkItem(ctx, h.workItemID)
	var upstreamArtifacts []map[string]any
	for _, s := range allActions {
		if s.Position < action.Position {
			run, err := h.store.GetLatestRunWithResult(ctx, s.ID)
			if err != nil || run == nil {
				continue
			}
			upstreamArtifacts = append(upstreamArtifacts, map[string]any{
				"action_id":   s.ID,
				"action_name": s.Name,
				"summary":     run.ResultMetadata["summary"],
				"metadata":    run.ResultMetadata,
			})
		}
	}

	// Read rework history from signals (preferred) with Config fallback.
	var reworkHistory []any
	var signalTimeline []map[string]any
	if signals, sErr := h.store.ListActionSignals(ctx, h.actionID); sErr == nil && len(signals) > 0 {
		for _, sig := range signals {
			entry := map[string]any{
				"id":         sig.ID,
				"type":       string(sig.Type),
				"source":     string(sig.Source),
				"summary":    sig.Summary,
				"content":    sig.Content,
				"actor":      sig.Actor,
				"created_at": sig.CreatedAt.Format("2006-01-02T15:04:05Z"),
			}
			if sig.SourceActionID != 0 {
				entry["source_action_id"] = sig.SourceActionID
			}
			if len(sig.Payload) > 0 {
				entry["payload"] = sig.Payload
			}
			signalTimeline = append(signalTimeline, entry)

			// Build rework history from feedback signals for backward compat.
			if sig.Type == core.SignalFeedback || sig.Type == core.SignalInstruction {
				reworkHistory = append(reworkHistory, entry)
			}
		}
	}
	// Signals are the single source of truth; no Config fallback.

	out := actionContextOutput{
		Action: map[string]any{
			"id":          action.ID,
			"name":        action.Name,
			"type":        string(action.Type),
			"position":    action.Position,
			"retry_count": action.RetryCount,
			"description": action.Description,
		},
		WorkItem: map[string]any{
			"id":    workItem.ID,
			"title": workItem.Title,
			"body":  workItem.Body,
		},
		UpstreamArtifacts: upstreamArtifacts,
		ReworkHistory:     reworkHistory,
		Signals:           signalTimeline,
	}
	return nil, out, nil
}

type actionCompleteInput struct {
	Summary      string   `json:"summary" jsonschema:"One-sentence summary of what was accomplished"`
	FilesChanged []string `json:"files_changed,omitempty" jsonschema:"File paths that were created or modified"`
	TestsPassed  *bool    `json:"tests_passed,omitempty" jsonschema:"Whether you ran tests and they passed"`
	Details      string   `json:"details,omitempty" jsonschema:"Additional details about the changes if needed"`
}

type signalResult struct {
	Status string `json:"status"`
	Type   string `json:"type,omitempty"`
}

func (h *mcpActionHandler) handleActionComplete(ctx context.Context, req *mcp.CallToolRequest, input actionCompleteInput) (*mcp.CallToolResult, signalResult, error) {
	if ok, existing := h.checkIdempotent(ctx); !ok {
		return nil, signalResult{Status: "already_decided", Type: string(existing)}, nil
	}
	payload := map[string]any{"summary": input.Summary}
	if len(input.FilesChanged) > 0 {
		payload["files_changed"] = input.FilesChanged
	}
	if input.TestsPassed != nil {
		payload["tests_passed"] = *input.TestsPassed
	}
	if input.Details != "" {
		payload["details"] = input.Details
	}
	h.createSignal(ctx, core.SignalComplete, payload)
	return nil, signalResult{Status: "accepted"}, nil
}

type actionNeedHelpInput struct {
	Reason    string `json:"reason" jsonschema:"Why you cannot proceed. Be specific about what is blocking you"`
	Attempted string `json:"attempted,omitempty" jsonschema:"What you already tried before giving up"`
	HelpType  string `json:"help_type,omitempty" jsonschema:"What kind of help is needed: access or clarification or decision or manual_action or other"`
}

func (h *mcpActionHandler) handleActionNeedHelp(ctx context.Context, req *mcp.CallToolRequest, input actionNeedHelpInput) (*mcp.CallToolResult, signalResult, error) {
	if ok, existing := h.checkIdempotent(ctx); !ok {
		return nil, signalResult{Status: "already_decided", Type: string(existing)}, nil
	}
	payload := map[string]any{"reason": input.Reason}
	if input.Attempted != "" {
		payload["attempted"] = input.Attempted
	}
	if input.HelpType != "" {
		payload["help_type"] = input.HelpType
	}
	h.createSignal(ctx, core.SignalNeedHelp, payload)
	return nil, signalResult{Status: "accepted"}, nil
}

type gateApproveInput struct {
	Reason string `json:"reason" jsonschema:"Why the gate passes. Be specific about what was verified"`
}

func (h *mcpActionHandler) handleGateApprove(ctx context.Context, req *mcp.CallToolRequest, input gateApproveInput) (*mcp.CallToolResult, signalResult, error) {
	if ok, existing := h.checkIdempotent(ctx); !ok {
		return nil, signalResult{Status: "already_decided", Type: string(existing)}, nil
	}
	h.createSignal(ctx, core.SignalApprove, map[string]any{"reason": input.Reason})
	return nil, signalResult{Status: "accepted"}, nil
}

type gateRejectInput struct {
	Reason        string  `json:"reason" jsonschema:"What needs to be fixed. Be specific and actionable"`
	RejectTargets []int64 `json:"reject_targets,omitempty" jsonschema:"Action IDs to reset for rework. Omit to reset immediate predecessors"`
}

func (h *mcpActionHandler) handleGateReject(ctx context.Context, req *mcp.CallToolRequest, input gateRejectInput) (*mcp.CallToolResult, signalResult, error) {
	if ok, existing := h.checkIdempotent(ctx); !ok {
		return nil, signalResult{Status: "already_decided", Type: string(existing)}, nil
	}
	payload := map[string]any{"reason": input.Reason}
	if len(input.RejectTargets) > 0 {
		targets := make([]any, len(input.RejectTargets))
		for i, t := range input.RejectTargets {
			targets[i] = t
		}
		payload["reject_targets"] = targets
	}
	h.createSignal(ctx, core.SignalReject, payload)
	return nil, signalResult{Status: "accepted"}, nil
}

// checkIdempotent returns (true, "") if no terminal signal exists yet for this run,
// or (false, existingType) if one does.
func (h *mcpActionHandler) checkIdempotent(ctx context.Context) (bool, core.SignalType) {
	signals, err := h.store.ListActionSignals(ctx, h.actionID)
	if err != nil {
		return true, "" // error → allow (best-effort)
	}
	for _, sig := range signals {
		if sig.RunID == h.runID && sig.Type.IsTerminal() {
			return false, sig.Type
		}
	}
	return true, ""
}

func (h *mcpActionHandler) createSignal(ctx context.Context, sigType core.SignalType, payload map[string]any) {
	sig := &core.ActionSignal{
		ActionID:   h.actionID,
		WorkItemID: h.workItemID,
		RunID:      h.runID,
		Type:       sigType,
		Source:     core.SignalSourceAgent,
		Payload:    payload,
		Actor:      "agent",
		CreatedAt:  time.Now().UTC(),
	}
	if _, err := h.store.CreateActionSignal(ctx, sig); err != nil {
		slog.Error("mcp-serve: failed to create signal", "type", sigType, "error", err)
	}
}
