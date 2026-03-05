package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
	"github.com/yoke233/ai-workflow/internal/eventbus"
	ghwebhook "github.com/yoke233/ai-workflow/internal/github"
	"github.com/yoke233/ai-workflow/internal/observability"
)

// webhookSCM abstracts SCM operations needed by webhook-triggered PR lifecycle.
type webhookSCM interface {
	CreatePR(ctx context.Context, req core.PullRequest) (prURL string, err error)
	ConvertToReady(ctx context.Context, number int) error
	MergePR(ctx context.Context, req core.PullRequestMerge) error
}

type webhookHandlers struct {
	store       core.Store
	secret      string
	executor    RunExecutor
	stageRoles  map[core.StageID]string
	dispatcher  *ghwebhook.WebhookDispatcher
	prLifecycle *ghwebhook.PRLifecycle
}

type githubWebhookEnvelope struct {
	Action     string `json:"action"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Issue struct {
		Number int `json:"number"`
		Title  string
		Body   string
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	} `json:"issue"`
	Comment struct {
		Body              string `json:"body"`
		AuthorAssociation string `json:"author_association"`
	} `json:"comment"`
	PullRequest struct {
		Number int  `json:"number"`
		Merged bool `json:"merged"`
	} `json:"pull_request"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

func registerWebhookRoutes(r chi.Router, store core.Store, executor RunExecutor, secret string, stageRoleBindings map[string]string, scm webhookSCM) WebhookDeliveryReplayer {
	var publisher interface {
		Publish(ctx context.Context, evt core.Event) error
	}
	if bus := eventbus.Default(); bus != nil {
		publisher = bus
	}

	h := &webhookHandlers{
		store:       store,
		secret:      strings.TrimSpace(secret),
		executor:    executor,
		stageRoles:  normalizeStageRoleBindings(stageRoleBindings),
		prLifecycle: ghwebhook.NewPRLifecycle(store, scm),
	}
	h.dispatcher = ghwebhook.NewWebhookDispatcher(ghwebhook.WebhookDispatcherOptions{
		Handler: ghwebhook.WebhookDispatchHandlerFunc(func(ctx context.Context, req ghwebhook.WebhookDispatchRequest) error {
			return h.handleDispatchedWebhook(ctx, req)
		}),
		Publisher: publisher,
		DLQStore:  ghwebhook.DefaultDLQStore(),
	})
	r.Post("/webhook", h.handleWebhook)
	return h.dispatcher
}

func (h *webhookHandlers) handleDispatchedWebhook(ctx context.Context, req ghwebhook.WebhookDispatchRequest) error {
	var envelope githubWebhookEnvelope
	if err := json.Unmarshal(req.Payload, &envelope); err != nil {
		return err
	}

	switch strings.TrimSpace(req.EventType) {
	case "issues":
		return h.handleIssuesEvent(ctx, req, envelope)
	case "issue_comment":
		return h.handleIssueCommentEvent(ctx, req, envelope)
	case "pull_request":
		return h.handlePullRequestEvent(ctx, req, envelope)
	default:
		return nil
	}
}

func (h *webhookHandlers) handleIssuesEvent(
	ctx context.Context,
	req ghwebhook.WebhookDispatchRequest,
	envelope githubWebhookEnvelope,
) error {
	if strings.TrimSpace(envelope.Action) != "opened" {
		return nil
	}
	if h.store == nil {
		return errors.New("store is required")
	}

	trigger := ghwebhook.NewRunTrigger(h.store, h.createRunForTrigger)
	_, err := trigger.TriggerFromIssue(ctx, ghwebhook.IssueTriggerInput{
		ProjectID:            req.ProjectID,
		IssueNumber:          envelope.Issue.Number,
		IssueTitle:           strings.TrimSpace(envelope.Issue.Title),
		IssueBody:            strings.TrimSpace(envelope.Issue.Body),
		Labels:               extractIssueLabels(envelope),
		LabelTemplateMapping: nil,
		TraceID:              req.TraceID,
	})
	return err
}

func (h *webhookHandlers) handleIssueCommentEvent(
	ctx context.Context,
	req ghwebhook.WebhookDispatchRequest,
	envelope githubWebhookEnvelope,
) error {
	if strings.TrimSpace(envelope.Action) != "created" {
		return nil
	}

	command, ok, err := ghwebhook.ParseSlashCommand(envelope.Comment.Body)
	if err != nil || !ok {
		return err
	}

	allowed := ghwebhook.IsSlashCommandAllowed(
		envelope.Sender.Login,
		envelope.Comment.AuthorAssociation,
		command.Type,
		ghwebhook.SlashACLConfig{},
	)
	if !allowed {
		return nil
	}

	switch command.Type {
	case ghwebhook.SlashCommandRun:
		trigger := ghwebhook.NewRunTrigger(h.store, h.createRunForTrigger)
		_, err := trigger.TriggerFromCommand(ctx, ghwebhook.CommandTriggerInput{
			ProjectID:       req.ProjectID,
			IssueNumber:     envelope.Issue.Number,
			Message:         strings.TrimSpace(envelope.Comment.Body),
			Template:        strings.TrimSpace(command.Template),
			DefaultTemplate: "standard",
			Labels:          extractIssueLabels(envelope),
			TraceID:         req.TraceID,
		})
		return err
	case ghwebhook.SlashCommandApprove:
		return h.applySlashRunAction(ctx, req.ProjectID, envelope.Issue.Number, core.RunAction{
			Type: core.ActionApprove,
		})
	case ghwebhook.SlashCommandReject:
		return h.applySlashRunAction(ctx, req.ProjectID, envelope.Issue.Number, core.RunAction{
			Type:    core.ActionReject,
			Stage:   command.Stage,
			Message: command.Reason,
		})
	case ghwebhook.SlashCommandAbort:
		return h.applySlashRunAction(ctx, req.ProjectID, envelope.Issue.Number, core.RunAction{
			Type:    core.ActionAbort,
			Message: command.Reason,
		})
	case ghwebhook.SlashCommandStatus:
		return nil
	default:
		return nil
	}
}

func (h *webhookHandlers) handlePullRequestEvent(
	ctx context.Context,
	req ghwebhook.WebhookDispatchRequest,
	envelope githubWebhookEnvelope,
) error {
	if strings.TrimSpace(envelope.Action) != "closed" {
		return nil
	}
	if h.prLifecycle == nil {
		return nil
	}
	return h.prLifecycle.OnPullRequestClosed(ctx, req.ProjectID, envelope.PullRequest.Number, envelope.PullRequest.Merged)
}

func (h *webhookHandlers) applySlashRunAction(
	ctx context.Context,
	projectID string,
	issueNumber int,
	action core.RunAction,
) error {
	if h.executor == nil || h.store == nil {
		return nil
	}

	Run, err := engine.FindRunByIssueNumber(h.store, projectID, issueNumber)
	if err != nil {
		return err
	}
	if Run == nil {
		return nil
	}

	action.RunID = Run.ID
	if action.Stage == "" {
		action.Stage = Run.CurrentStage
	}
	return h.executor.ApplyAction(ctx, action)
}

func (h *webhookHandlers) createRunForTrigger(
	projectID,
	name,
	description,
	template string,
) (*core.Run, error) {
	if h == nil || h.store == nil {
		return nil, errors.New("store is required")
	}
	stages, err := buildRunstages(template, h.stageRoles)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	Run := &core.Run{
		ID:              engine.NewRunID(),
		ProjectID:       strings.TrimSpace(projectID),
		Name:            strings.TrimSpace(name),
		Description:     strings.TrimSpace(description),
		Template:        strings.TrimSpace(template),
		Status:          core.StatusQueued,
		Stages:          stages,
		Artifacts:       map[string]string{},
		Config:          map[string]any{},
		MaxTotalRetries: 5,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := h.store.SaveRun(Run); err != nil {
		return nil, err
	}
	return Run, nil
}

func (h *webhookHandlers) handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid request body", "INVALID_BODY")
		return
	}

	signature := strings.TrimSpace(r.Header.Get("X-Hub-Signature-256"))
	if !verifyWebhookSignature(payload, signature, h.secret) {
		writeAPIError(w, http.StatusUnauthorized, "invalid webhook signature", "INVALID_WEBHOOK_SIGNATURE")
		return
	}

	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	var envelope githubWebhookEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid webhook payload", "INVALID_WEBHOOK_PAYLOAD")
		return
	}

	owner := strings.TrimSpace(envelope.Repository.Owner.Login)
	repo := strings.TrimSpace(envelope.Repository.Name)
	if owner == "" || repo == "" {
		writeAPIError(w, http.StatusBadRequest, "repository owner/repo is required", "REPOSITORY_REQUIRED")
		return
	}

	project, err := findProjectByOwnerRepo(h.store, owner, repo)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to route webhook project", "PROJECT_ROUTING_FAILED")
		return
	}
	if project == nil {
		writeAPIError(w, http.StatusNotFound, "project for repository not found", "PROJECT_NOT_FOUND")
		return
	}

	eventType := strings.TrimSpace(r.Header.Get("X-GitHub-Event"))
	if !isSupportedWebhookEvent(eventType) {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":     "ignored",
			"project_id": project.ID,
		})
		return
	}

	if h.dispatcher != nil {
		deliveryID := strings.TrimSpace(r.Header.Get("X-GitHub-Delivery"))
		traceID := observability.TraceIDFromWebhook(strings.TrimSpace(r.Header.Get("X-Trace-ID")), deliveryID)
		dispatchCtx := observability.WithTraceID(r.Context(), traceID)
		_ = extractIssueLabels(envelope)

		result, err := h.dispatcher.Dispatch(dispatchCtx, ghwebhook.WebhookDispatchRequest{
			ProjectID:  project.ID,
			EventType:  eventType,
			Action:     strings.TrimSpace(envelope.Action),
			DeliveryID: deliveryID,
			TraceID:    traceID,
			Payload:    payload,
			ReceivedAt: time.Now(),
		})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "failed to dispatch webhook", "WEBHOOK_DISPATCH_FAILED")
			return
		}

		status := "accepted"
		if result.Duplicate {
			status = "deduplicated"
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":     status,
			"project_id": project.ID,
			"event":      eventType,
			"action":     envelope.Action,
		})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":     "accepted",
		"project_id": project.ID,
		"event":      eventType,
		"action":     envelope.Action,
	})
}

func extractIssueLabels(envelope githubWebhookEnvelope) []string {
	if len(envelope.Issue.Labels) == 0 {
		return nil
	}
	labels := make([]string, 0, len(envelope.Issue.Labels))
	for _, label := range envelope.Issue.Labels {
		name := strings.TrimSpace(label.Name)
		if name == "" {
			continue
		}
		labels = append(labels, name)
	}
	return labels
}

func verifyWebhookSignature(payload []byte, signatureHeader, secret string) bool {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return false
	}

	const prefix = "sha256="
	signatureHeader = strings.TrimSpace(signatureHeader)
	if len(signatureHeader) <= len(prefix) || !strings.EqualFold(signatureHeader[:len(prefix)], prefix) {
		return false
	}

	signatureHex := signatureHeader[len(prefix):]
	givenSignature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	expectedSignature := mac.Sum(nil)

	return hmac.Equal(givenSignature, expectedSignature)
}

func findProjectByOwnerRepo(store core.Store, owner, repo string) (*core.Project, error) {
	projects, err := store.ListProjects(core.ProjectFilter{})
	if err != nil {
		return nil, err
	}

	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	for _, project := range projects {
		if strings.EqualFold(strings.TrimSpace(project.GitHubOwner), owner) &&
			strings.EqualFold(strings.TrimSpace(project.GitHubRepo), repo) {
			p := project
			return &p, nil
		}
	}
	return nil, nil
}

func isSupportedWebhookEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "issues", "issue_comment", "pull_request":
		return true
	default:
		return false
	}
}
