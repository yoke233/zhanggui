package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	workspaceclone "github.com/yoke233/ai-workflow/internal/adapters/workspace/clone"
	issueapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
)

type bootstrapPRWorkItemRequest struct {
	BaseBranch *string `json:"base_branch,omitempty"`
	Title      *string `json:"title,omitempty"`
	Body       *string `json:"body,omitempty"`
}

type scmBindingInfo struct {
	Provider      string
	RepoPath      string
	DefaultBranch string
	MergeMethod   string
	RemoteHost    string
	RemoteOwner   string
	RemoteRepo    string
}

type bootstrapPRWorkItemResponse struct {
	IssueID      int64 `json:"issue_id"`
	ImplementID  int64 `json:"implement_step_id"`
	CommitPushID int64 `json:"commit_push_step_id"`
	OpenPRID     int64 `json:"open_pr_step_id"`
	GateID       int64 `json:"gate_step_id"`
}

var (
	errBootstrapPRIssueMissingProject   = errors.New("issue must belong to a project")
	errBootstrapPRIssueMissingBinding   = errors.New("project does not have an enabled supported SCM git binding")
	errBootstrapPRIssueAmbiguousBinding = errors.New("issue must select a resource binding when multiple SCM git bindings are enabled")
	errBootstrapPRIssueHasSteps         = errors.New("issue already has steps")
)

// bootstrapPRWorkItem creates a standard PR automation pipeline for an issue:
// implement(exec) → commit_push(exec,builtin) → open_pr(exec,builtin) → review_merge_gate(gate).
//
// Requirements:
// - Issue must belong to a project
// - Project must have an enabled supported SCM git resource binding (GitHub / Codeup)
func (h *Handler) bootstrapPRWorkItem(w http.ResponseWriter, r *http.Request) {
	issueID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	var req bootstrapPRWorkItemRequest
	var err error
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != context.Canceled {
		// Allow empty body.
		if strings.TrimSpace(err.Error()) != "EOF" {
			writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
			return
		}
	}

	resp, err := h.bootstrapPRWorkItemForIssue(r.Context(), issueID, req)
	if err != nil {
		switch {
		case errors.Is(err, errBootstrapPRIssueMissingProject), errors.Is(err, errBootstrapPRIssueMissingBinding):
			writeError(w, http.StatusBadRequest, err.Error(), "MISSING_SCM_BINDING")
		case errors.Is(err, errBootstrapPRIssueAmbiguousBinding):
			writeError(w, http.StatusConflict, err.Error(), "AMBIGUOUS_SCM_BINDING")
		case errors.Is(err, errBootstrapPRIssueHasSteps):
			writeError(w, http.StatusConflict, err.Error(), "ISSUE_HAS_STEPS")
		default:
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		}
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) bootstrapPRWorkItemForIssue(ctx context.Context, issueID int64, req bootstrapPRWorkItemRequest) (bootstrapPRWorkItemResponse, error) {
	issue, err := h.store.GetWorkItem(ctx, issueID)
	if err != nil {
		return bootstrapPRWorkItemResponse{}, err
	}
	if issue.ProjectID == nil {
		return bootstrapPRWorkItemResponse{}, errBootstrapPRIssueMissingProject
	}

	bindings, err := h.store.ListResourceBindings(ctx, *issue.ProjectID)
	if err != nil {
		return bootstrapPRWorkItemResponse{}, err
	}
	bindings, err = bindingsForIssue(issue, bindings)
	if err != nil {
		return bootstrapPRWorkItemResponse{}, err
	}
	bindingInfo, ok := resolveEnabledSCMRepoFromBindings(ctx, bindings)
	if !ok {
		return bootstrapPRWorkItemResponse{}, errBootstrapPRIssueMissingBinding
	}

	steps, err := h.store.ListActionsByWorkItem(ctx, issueID)
	if err != nil {
		return bootstrapPRWorkItemResponse{}, err
	}
	if len(steps) > 0 {
		return bootstrapPRWorkItemResponse{}, errBootstrapPRIssueHasSteps
	}

	baseBranch := bindingInfo.DefaultBranch
	if req.BaseBranch != nil && strings.TrimSpace(*req.BaseBranch) != "" {
		baseBranch = strings.TrimSpace(*req.BaseBranch)
	}

	title := fmt.Sprintf("ai-flow: issue %d", issueID)
	body := fmt.Sprintf("Automated change request for %s/%s.", bindingInfo.RemoteOwner, bindingInfo.RemoteRepo)
	if req.Title != nil && strings.TrimSpace(*req.Title) != "" {
		title = strings.TrimSpace(*req.Title)
	}
	if req.Body != nil && strings.TrimSpace(*req.Body) != "" {
		body = strings.TrimSpace(*req.Body)
	}

	providerPrompts := h.currentPRIssuePrompts().Provider(bindingInfo.Provider)
	implementObjective := providerPrompts.ImplementObjective
	gateObjective := providerPrompts.GateObjective
	commitMessage := defaultPRCommitMessage(issueID)

	implement := &core.Action{
		WorkItemID:    issueID,
		Name:       "implement",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   0,
		AgentRole:  "worker",
		Timeout:    15 * time.Minute,
		MaxRetries: 3,
		Config: map[string]any{
			"objective": implementObjective,
		},
	}
	createdStepIDs := make([]int64, 0, 4)
	rollbackCreatedSteps := func(cause error) error {
		for i := len(createdStepIDs) - 1; i >= 0; i-- {
			if delErr := h.store.DeleteAction(ctx, createdStepIDs[i]); delErr != nil && !errors.Is(delErr, core.ErrNotFound) {
				return fmt.Errorf("%w; rollback delete step %d: %v", cause, createdStepIDs[i], delErr)
			}
		}
		return cause
	}

	implementID, err := h.store.CreateAction(ctx, implement)
	if err != nil {
		return bootstrapPRWorkItemResponse{}, err
	}
	createdStepIDs = append(createdStepIDs, implementID)

	commitPush := &core.Action{
		WorkItemID:    issueID,
		Name:       "commit_push",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   1,
		AgentRole:  "worker",
		Timeout:    5 * time.Minute,
		MaxRetries: 0,
		Config: map[string]any{
			"builtin":        "git_commit_push",
			"commit_message": commitMessage,
		},
	}
	commitPushID, err := h.store.CreateAction(ctx, commitPush)
	if err != nil {
		return bootstrapPRWorkItemResponse{}, rollbackCreatedSteps(err)
	}
	createdStepIDs = append(createdStepIDs, commitPushID)

	openPR := &core.Action{
		WorkItemID:    issueID,
		Name:       "open_pr",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   2,
		AgentRole:  "worker",
		Timeout:    5 * time.Minute,
		MaxRetries: 0,
		Config: map[string]any{
			"builtin": "scm_open_pr",
			"base":    baseBranch,
			"title":   title,
			"body":    body,
		},
	}
	openPRID, err := h.store.CreateAction(ctx, openPR)
	if err != nil {
		return bootstrapPRWorkItemResponse{}, rollbackCreatedSteps(err)
	}
	createdStepIDs = append(createdStepIDs, openPRID)

	gate := &core.Action{
		WorkItemID:    issueID,
		Name:       "review_merge_gate",
		Type:       core.ActionGate,
		Status:     core.ActionPending,
		Position:   3,
		AgentRole:  "gate",
		Timeout:    10 * time.Minute,
		MaxRetries: 0,
		RequiredCapabilities: []string{
			"prreview",
		},
		Config: map[string]any{
			"merge_on_pass":          true,
			"merge_method":           bindingInfo.MergeMethod,
			"reset_upstream_closure": true,
			"max_rework_rounds":      float64(3),
			"objective":              gateObjective,
		},
	}
	gateID, err := h.store.CreateAction(ctx, gate)
	if err != nil {
		return bootstrapPRWorkItemResponse{}, rollbackCreatedSteps(err)
	}

	return bootstrapPRWorkItemResponse{
		IssueID:      issueID,
		ImplementID:  implementID,
		CommitPushID: commitPushID,
		OpenPRID:     openPRID,
		GateID:       gateID,
	}, nil
}

func (h *Handler) currentPRIssuePrompts() issueapp.PRFlowPrompts {
	if h != nil && h.prPrompts != nil {
		return issueapp.MergePRFlowPrompts(h.prPrompts())
	}
	return issueapp.DefaultPRFlowPrompts()
}

func (h *Handler) currentPRFlowPrompts() issueapp.PRFlowPrompts {
	return h.currentPRIssuePrompts()
}

func defaultPRCommitMessage(issueID int64) string {
	return fmt.Sprintf("chore(pr-issue): apply issue %d updates", issueID)
}

func resolveEnabledSCMRepoFromBindings(ctx context.Context, bindings []*core.ResourceBinding) (scmBindingInfo, bool) {
	candidates := make([]scmBindingInfo, 0, len(bindings))
	for _, b := range bindings {
		if !bindingSCMFlowEnabled(b) {
			continue
		}
		repoPath := strings.TrimSpace(b.URI)
		if repoPath == "" {
			continue
		}
		defaultBranch := bindingDefaultBranch(b)
		originURL, err := gitOriginURL(ctx, repoPath)
		if err != nil {
			continue
		}
		remote, err := workspaceclone.ParseRemoteURL(originURL)
		if err != nil {
			continue
		}
		provider := bindingProvider(b, remote.Host)
		if provider == "" {
			continue
		}
		candidates = append(candidates, scmBindingInfo{
			Provider:      provider,
			RepoPath:      repoPath,
			DefaultBranch: defaultBranch,
			MergeMethod:   bindingMergeMethod(b),
			RemoteHost:    strings.TrimSpace(remote.Host),
			RemoteOwner:   strings.TrimSpace(remote.Owner),
			RemoteRepo:    strings.TrimSpace(remote.Repo),
		})
	}
	if len(candidates) != 1 {
		return scmBindingInfo{}, false
	}
	return candidates[0], true
}

func bindingsForIssue(issue *core.WorkItem, bindings []*core.ResourceBinding) ([]*core.ResourceBinding, error) {
	if issue == nil {
		return nil, errBootstrapPRIssueMissingProject
	}
	if issue.ResourceBindingID != nil {
		for _, binding := range bindings {
			if binding != nil && binding.ID == *issue.ResourceBindingID {
				return []*core.ResourceBinding{binding}, nil
			}
		}
		return nil, errBootstrapPRIssueMissingBinding
	}

	enabledGitBindings := 0
	for _, binding := range bindings {
		if bindingSCMFlowEnabled(binding) {
			enabledGitBindings++
		}
	}
	if enabledGitBindings > 1 {
		return nil, errBootstrapPRIssueAmbiguousBinding
	}
	return bindings, nil
}

func bindingProvider(b *core.ResourceBinding, host string) string {
	if b != nil && b.Config != nil {
		if v, ok := b.Config["provider"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.ToLower(strings.TrimSpace(v))
		}
	}
	host = strings.ToLower(strings.TrimSpace(host))
	switch {
	case host == "github.com":
		return "github"
	case strings.Contains(host, "rdc.aliyuncs.com"), strings.Contains(host, "codeup.aliyun.com"):
		return "codeup"
	default:
		return ""
	}
}

func bindingSCMFlowEnabled(b *core.ResourceBinding) bool {
	if b == nil || strings.TrimSpace(strings.ToLower(b.Kind)) != "git" || b.Config == nil {
		return false
	}
	return bindingConfigBool(b.Config["enable_scm_flow"])
}

func bindingConfigBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func bindingDefaultBranch(b *core.ResourceBinding) string {
	if b != nil && b.Config != nil {
		for _, key := range []string{"base_branch", "default_branch"} {
			if v, ok := b.Config[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return "main"
}

func bindingMergeMethod(b *core.ResourceBinding) string {
	if b != nil && b.Config != nil {
		if v, ok := b.Config["merge_method"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return "squash"
}

func gitOriginURL(ctx context.Context, repoPath string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "remote", "get-url", "origin")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git origin url: %s", strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}
