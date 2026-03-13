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

	sqlitestore "github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/appdata"
)

// RunMCPServe starts the MCP server over stdio.
// It reads step context from environment variables and exposes tools based on step type.
func RunMCPServe(args []string) error {
	dataDir, err := appdata.ResolveDataDir()
	if err == nil {
		closeLog, logErr := initAppLogger(dataDir, "mcp-serve")
		if logErr != nil {
			return logErr
		}
		defer closeLog()
	}

	dbPath := strings.TrimSpace(os.Getenv("AI_WORKFLOW_DB_PATH"))
	if dbPath == "" {
		return fmt.Errorf("AI_WORKFLOW_DB_PATH is required")
	}
	stepID := envInt64("AI_WORKFLOW_STEP_ID")
	issueID := envInt64("AI_WORKFLOW_ISSUE_ID")
	stepType := strings.TrimSpace(os.Getenv("AI_WORKFLOW_STEP_TYPE"))
	execID := envInt64("AI_WORKFLOW_EXEC_ID")

	store, err := sqlitestore.New(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer store.Close()

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "ai-workflow-step",
		Version: "0.1.0",
	}, nil)

	handler := &mcpStepHandler{
		store:   store,
		stepID:  stepID,
		issueID: issueID,
		execID:  execID,
	}

	// step_context — available for all step types.
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "step_context",
		Description: "Get the execution context: issue details, upstream step results, and your own rework history.",
	}, handler.handleStepContext)

	switch stepType {
	case "exec":
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "step_complete",
			Description: "Declare that you have completed the task. Provide a structured summary of what you did. Call this BEFORE ending your response.",
		}, handler.handleStepComplete)

		mcp.AddTool(srv, &mcp.Tool{
			Name:        "step_need_help",
			Description: "Signal that you cannot complete the task and need human assistance. Explain what you tried, what went wrong, and what kind of help you need.",
		}, handler.handleStepNeedHelp)
	case "gate":
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "gate_approve",
			Description: "Approve the gate. Call this when the review passes all criteria.",
		}, handler.handleGateApprove)

		mcp.AddTool(srv, &mcp.Tool{
			Name:        "gate_reject",
			Description: "Reject the gate. Call this when the review finds issues that must be fixed.",
		}, handler.handleGateReject)
	}

	slog.Info("mcp-serve: starting", "step_id", stepID, "issue_id", issueID, "step_type", stepType, "exec_id", execID)
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

// mcpStepHandler implements the MCP tool handlers.
type mcpStepHandler struct {
	store   core.Store
	stepID  int64
	issueID int64
	execID  int64
}

type stepContextInput struct{}

type stepContextOutput struct {
	Step              map[string]any   `json:"step"`
	Issue             map[string]any   `json:"issue"`
	UpstreamArtifacts []map[string]any `json:"upstream_artifacts"`
	ReworkHistory     []any            `json:"rework_history"`
	Signals           []map[string]any `json:"signals,omitempty"`
}

func (h *mcpStepHandler) handleStepContext(ctx context.Context, req *mcp.CallToolRequest, _ stepContextInput) (*mcp.CallToolResult, stepContextOutput, error) {
	step, err := h.store.GetAction(ctx, h.stepID)
	if err != nil {
		return nil, stepContextOutput{}, fmt.Errorf("get action: %w", err)
	}
	issue, err := h.store.GetWorkItem(ctx, h.issueID)
	if err != nil {
		return nil, stepContextOutput{}, fmt.Errorf("get work item: %w", err)
	}

	// Collect upstream artifacts.
	allSteps, _ := h.store.ListActionsByWorkItem(ctx, h.issueID)
	var upstreamArtifacts []map[string]any
	for _, s := range allSteps {
		if s.Position < step.Position {
			art, err := h.store.GetLatestDeliverableByAction(ctx, s.ID)
			if err != nil || art == nil {
				continue
			}
			upstreamArtifacts = append(upstreamArtifacts, map[string]any{
				"step_id":   s.ID,
				"step_name": s.Name,
				"summary":   art.Metadata["summary"],
				"metadata":  art.Metadata,
			})
		}
	}

	// Read rework history from signals (preferred) with Config fallback.
	var reworkHistory []any
	var signalTimeline []map[string]any
	if signals, sErr := h.store.ListActionSignals(ctx, h.stepID); sErr == nil && len(signals) > 0 {
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
				entry["source_step_id"] = sig.SourceActionID
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

	out := stepContextOutput{
		Step: map[string]any{
			"id":          step.ID,
			"name":        step.Name,
			"type":        string(step.Type),
			"position":    step.Position,
			"retry_count": step.RetryCount,
			"description": step.Description,
		},
		Issue: map[string]any{
			"id":    issue.ID,
			"title": issue.Title,
			"body":  issue.Body,
		},
		UpstreamArtifacts: upstreamArtifacts,
		ReworkHistory:     reworkHistory,
		Signals:           signalTimeline,
	}
	return nil, out, nil
}

type stepCompleteInput struct {
	Summary      string   `json:"summary" jsonschema:"One-sentence summary of what was accomplished"`
	FilesChanged []string `json:"files_changed,omitempty" jsonschema:"File paths that were created or modified"`
	TestsPassed  *bool    `json:"tests_passed,omitempty" jsonschema:"Whether you ran tests and they passed"`
	Details      string   `json:"details,omitempty" jsonschema:"Additional details about the changes if needed"`
}

type signalResult struct {
	Status string `json:"status"`
	Type   string `json:"type,omitempty"`
}

func (h *mcpStepHandler) handleStepComplete(ctx context.Context, req *mcp.CallToolRequest, input stepCompleteInput) (*mcp.CallToolResult, signalResult, error) {
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

type stepNeedHelpInput struct {
	Reason    string `json:"reason" jsonschema:"Why you cannot proceed. Be specific about what is blocking you"`
	Attempted string `json:"attempted,omitempty" jsonschema:"What you already tried before giving up"`
	HelpType  string `json:"help_type,omitempty" jsonschema:"What kind of help is needed: access or clarification or decision or manual_action or other"`
}

func (h *mcpStepHandler) handleStepNeedHelp(ctx context.Context, req *mcp.CallToolRequest, input stepNeedHelpInput) (*mcp.CallToolResult, signalResult, error) {
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

func (h *mcpStepHandler) handleGateApprove(ctx context.Context, req *mcp.CallToolRequest, input gateApproveInput) (*mcp.CallToolResult, signalResult, error) {
	if ok, existing := h.checkIdempotent(ctx); !ok {
		return nil, signalResult{Status: "already_decided", Type: string(existing)}, nil
	}
	h.createSignal(ctx, core.SignalApprove, map[string]any{"reason": input.Reason})
	return nil, signalResult{Status: "accepted"}, nil
}

type gateRejectInput struct {
	Reason        string  `json:"reason" jsonschema:"What needs to be fixed. Be specific and actionable"`
	RejectTargets []int64 `json:"reject_targets,omitempty" jsonschema:"Step IDs to reset for rework. Omit to reset immediate predecessors"`
}

func (h *mcpStepHandler) handleGateReject(ctx context.Context, req *mcp.CallToolRequest, input gateRejectInput) (*mcp.CallToolResult, signalResult, error) {
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

// checkIdempotent returns (true, "") if no terminal signal exists yet for this exec,
// or (false, existingType) if one does.
func (h *mcpStepHandler) checkIdempotent(ctx context.Context) (bool, core.SignalType) {
	signals, err := h.store.ListActionSignals(ctx, h.stepID)
	if err != nil {
		return true, "" // error → allow (best-effort)
	}
	for _, sig := range signals {
		if sig.RunID == h.execID && sig.Type.IsTerminal() {
			return false, sig.Type
		}
	}
	return true, ""
}

func (h *mcpStepHandler) createSignal(ctx context.Context, sigType core.SignalType, payload map[string]any) {
	sig := &core.ActionSignal{
		ActionID:   h.stepID,
		WorkItemID: h.issueID,
		RunID:      h.execID,
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
