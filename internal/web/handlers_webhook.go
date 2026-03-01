package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/user/ai-workflow/internal/core"
	"github.com/user/ai-workflow/internal/eventbus"
	ghwebhook "github.com/user/ai-workflow/internal/github"
	"github.com/user/ai-workflow/internal/observability"
)

type webhookHandlers struct {
	store      core.Store
	secret     string
	dispatcher *ghwebhook.WebhookDispatcher
}

type githubWebhookEnvelope struct {
	Action     string `json:"action"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

func registerWebhookRoutes(r chi.Router, store core.Store, secret string) WebhookDeliveryReplayer {
	var publisher interface{ Publish(evt core.Event) }
	if bus := eventbus.Default(); bus != nil {
		publisher = bus
	}

	h := &webhookHandlers{
		store:  store,
		secret: strings.TrimSpace(secret),
		dispatcher: ghwebhook.NewWebhookDispatcher(ghwebhook.WebhookDispatcherOptions{
			Publisher: publisher,
			DLQStore:  ghwebhook.DefaultDLQStore(),
		}),
	}
	r.Post("/webhook", h.handleWebhook)
	return h.dispatcher
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
	case "issues", "issue_comment":
		return true
	default:
		return false
	}
}
