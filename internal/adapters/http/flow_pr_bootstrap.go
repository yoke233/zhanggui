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

	workspaceclone "github.com/yoke233/zhanggui/internal/adapters/workspace/clone"
	issueapp "github.com/yoke233/zhanggui/internal/application/flow"
	"github.com/yoke233/zhanggui/internal/core"
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
	WorkItemID   int64 `json:"work_item_id"`
	ImplementID  int64 `json:"implement_action_id"`
	CommitPushID int64 `json:"commit_push_action_id"`
	OpenPRID     int64 `json:"open_pr_action_id"`
	GateID       int64 `json:"gate_action_id"`
}

var (
	errBootstrapPRWorkItemMissingProject = errors.New("work item must belong to a project")
	errBootstrapPRWorkItemMissingSpace   = errors.New("project does not have an enabled supported SCM git space")
	errBootstrapPRWorkItemAmbiguousSpace = errors.New("work item must select a resource space when multiple SCM git spaces are enabled")
	errBootstrapPRWorkItemHasActions     = errors.New("work item already has actions")
)

// bootstrapPRWorkItem creates a standard PR automation pipeline for a work item:
// implement(exec) → commit_push(exec,builtin) → open_pr(exec,builtin) → review_merge_gate(gate).
//
// Requirements:
// - Work item must belong to a project
// - Project must have an enabled supported SCM git resource space (GitHub / Codeup)
func (h *Handler) bootstrapPRWorkItem(w http.ResponseWriter, r *http.Request) {
	workItemID, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work item ID", "BAD_ID")
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

	resp, err := h.bootstrapPRWorkItemActions(r.Context(), workItemID, req)
	if err != nil {
		switch {
		case errors.Is(err, errBootstrapPRWorkItemMissingProject), errors.Is(err, errBootstrapPRWorkItemMissingSpace):
			writeError(w, http.StatusBadRequest, err.Error(), "MISSING_SCM_BINDING")
		case errors.Is(err, errBootstrapPRWorkItemAmbiguousSpace):
			writeError(w, http.StatusConflict, err.Error(), "AMBIGUOUS_SCM_BINDING")
		case errors.Is(err, errBootstrapPRWorkItemHasActions):
			writeError(w, http.StatusConflict, err.Error(), "WORK_ITEM_HAS_ACTIONS")
		default:
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		}
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) bootstrapPRWorkItemActions(ctx context.Context, workItemID int64, req bootstrapPRWorkItemRequest) (bootstrapPRWorkItemResponse, error) {
	workItem, err := h.store.GetWorkItem(ctx, workItemID)
	if err != nil {
		return bootstrapPRWorkItemResponse{}, err
	}
	if workItem.ProjectID == nil {
		return bootstrapPRWorkItemResponse{}, errBootstrapPRWorkItemMissingProject
	}

	spaces, err := h.store.ListResourceSpaces(ctx, *workItem.ProjectID)
	if err != nil {
		return bootstrapPRWorkItemResponse{}, err
	}
	spaces, err = filterSpacesForWorkItem(workItem, spaces)
	if err != nil {
		return bootstrapPRWorkItemResponse{}, err
	}
	bindingInfo, ok := resolveEnabledSCMRepoFromSpaces(ctx, spaces)
	if !ok {
		return bootstrapPRWorkItemResponse{}, errBootstrapPRWorkItemMissingSpace
	}

	actions, err := h.store.ListActionsByWorkItem(ctx, workItemID)
	if err != nil {
		return bootstrapPRWorkItemResponse{}, err
	}
	if len(actions) > 0 {
		return bootstrapPRWorkItemResponse{}, errBootstrapPRWorkItemHasActions
	}

	baseBranch := bindingInfo.DefaultBranch
	if req.BaseBranch != nil && strings.TrimSpace(*req.BaseBranch) != "" {
		baseBranch = strings.TrimSpace(*req.BaseBranch)
	}

	title := fmt.Sprintf("ai-flow: work item %d", workItemID)
	body := fmt.Sprintf("Automated change request for %s/%s.", bindingInfo.RemoteOwner, bindingInfo.RemoteRepo)
	if req.Title != nil && strings.TrimSpace(*req.Title) != "" {
		title = strings.TrimSpace(*req.Title)
	}
	if req.Body != nil && strings.TrimSpace(*req.Body) != "" {
		body = strings.TrimSpace(*req.Body)
	}

	providerPrompts := h.currentPRWorkItemPrompts().Provider(bindingInfo.Provider)
	implementObjective := providerPrompts.ImplementObjective
	gateObjective := providerPrompts.GateObjective
	commitMessage := defaultPRCommitMessage(workItemID)

	implement := &core.Action{
		WorkItemID: workItemID,
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
	createdActionIDs := make([]int64, 0, 4)
	rollbackCreatedActions := func(cause error) error {
		for i := len(createdActionIDs) - 1; i >= 0; i-- {
			if delErr := h.store.DeleteAction(ctx, createdActionIDs[i]); delErr != nil && !errors.Is(delErr, core.ErrNotFound) {
				return fmt.Errorf("%w; rollback delete action %d: %v", cause, createdActionIDs[i], delErr)
			}
		}
		return cause
	}

	implementID, err := h.store.CreateAction(ctx, implement)
	if err != nil {
		return bootstrapPRWorkItemResponse{}, err
	}
	createdActionIDs = append(createdActionIDs, implementID)

	commitPush := &core.Action{
		WorkItemID: workItemID,
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
		return bootstrapPRWorkItemResponse{}, rollbackCreatedActions(err)
	}
	createdActionIDs = append(createdActionIDs, commitPushID)

	openPR := &core.Action{
		WorkItemID: workItemID,
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
		return bootstrapPRWorkItemResponse{}, rollbackCreatedActions(err)
	}
	createdActionIDs = append(createdActionIDs, openPRID)

	gate := &core.Action{
		WorkItemID: workItemID,
		Name:       "review_merge_gate",
		Type:       core.ActionGate,
		Status:     core.ActionPending,
		Position:   3,
		AgentRole:  "gate",
		Timeout:    10 * time.Minute,
		MaxRetries: 0,
		RequiredCapabilities: []string{
			"review",
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
		return bootstrapPRWorkItemResponse{}, rollbackCreatedActions(err)
	}

	return bootstrapPRWorkItemResponse{
		WorkItemID:   workItemID,
		ImplementID:  implementID,
		CommitPushID: commitPushID,
		OpenPRID:     openPRID,
		GateID:       gateID,
	}, nil
}

func (h *Handler) currentPRWorkItemPrompts() issueapp.PRFlowPrompts {
	if h != nil && h.prPrompts != nil {
		return issueapp.MergePRFlowPrompts(h.prPrompts())
	}
	return issueapp.DefaultPRFlowPrompts()
}

func (h *Handler) currentPRFlowPrompts() issueapp.PRFlowPrompts {
	return h.currentPRWorkItemPrompts()
}

func defaultPRCommitMessage(workItemID int64) string {
	return fmt.Sprintf("chore(pr-work-item): apply work item %d updates", workItemID)
}

func resolveEnabledSCMRepoFromSpaces(ctx context.Context, spaces []*core.ResourceSpace) (scmBindingInfo, bool) {
	candidates := make([]scmBindingInfo, 0, len(spaces))
	for _, space := range spaces {
		if !spaceSCMFlowEnabled(space) {
			continue
		}
		repoPath := resolveGitSpaceWorkDir(space)
		if repoPath == "" {
			continue
		}
		defaultBranch := spaceDefaultBranch(space)
		originURL, err := gitOriginURL(ctx, repoPath)
		if err != nil {
			continue
		}
		remote, err := workspaceclone.ParseRemoteURL(originURL)
		if err != nil {
			continue
		}
		provider := spaceProvider(space, remote.Host)
		if provider == "" {
			continue
		}
		candidates = append(candidates, scmBindingInfo{
			Provider:      provider,
			RepoPath:      repoPath,
			DefaultBranch: defaultBranch,
			MergeMethod:   spaceMergeMethod(space),
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

func filterSpacesForWorkItem(workItem *core.WorkItem, spaces []*core.ResourceSpace) ([]*core.ResourceSpace, error) {
	if workItem == nil {
		return nil, errBootstrapPRWorkItemMissingProject
	}
	if workItem.ResourceSpaceID != nil {
		for _, space := range spaces {
			if space != nil && space.ID == *workItem.ResourceSpaceID {
				return []*core.ResourceSpace{space}, nil
			}
		}
		return nil, errBootstrapPRWorkItemMissingSpace
	}

	enabledGitSpaces := 0
	for _, space := range spaces {
		if spaceSCMFlowEnabled(space) {
			enabledGitSpaces++
		}
	}
	if enabledGitSpaces > 1 {
		return nil, errBootstrapPRWorkItemAmbiguousSpace
	}
	return spaces, nil
}

func spaceProvider(space *core.ResourceSpace, host string) string {
	if space != nil && space.Config != nil {
		if v, ok := space.Config["provider"].(string); ok && strings.TrimSpace(v) != "" {
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

func spaceSCMFlowEnabled(space *core.ResourceSpace) bool {
	if space == nil || strings.TrimSpace(strings.ToLower(space.Kind)) != core.ResourceKindGit || space.Config == nil {
		return false
	}
	return bindingConfigBool(space.Config["enable_scm_flow"])
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

func spaceDefaultBranch(space *core.ResourceSpace) string {
	if space != nil && space.Config != nil {
		for _, key := range []string{"base_branch", "default_branch"} {
			if v, ok := space.Config[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return "main"
}

func spaceMergeMethod(space *core.ResourceSpace) string {
	if space != nil && space.Config != nil {
		if v, ok := space.Config["merge_method"].(string); ok && strings.TrimSpace(v) != "" {
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
