package cmd

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"zhanggui/internal/bootstrap"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
	"zhanggui/internal/usecase/outbox"
)

var outboxQualityWebhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Start GitHub/GitLab webhook receiver for quality events",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		addr, _ := cmd.Flags().GetString("addr")
		githubSecret, _ := cmd.Flags().GetString("github-secret")
		gitlabToken, _ := cmd.Flags().GetString("gitlab-token")

		addr = strings.TrimSpace(addr)
		if addr == "" {
			addr = ":8088"
		}

		server := &http.Server{
			Addr: addr,
			Handler: newOutboxQualityWebhookHandler(svc, qualityWebhookAuthConfig{
				GitHubSecret: githubSecret,
				GitLabToken:  gitlabToken,
			}),
		}

		logging.Info(
			ctx,
			"quality webhook server started",
			slog.String("addr", addr),
		)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.Error(ctx, "quality webhook server failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "serve quality webhook")
		}
		return nil
	}),
}

type qualityWebhookAuthConfig struct {
	GitHubSecret string
	GitLabToken  string
}

type qualityWebhookIngestService interface {
	IngestQualityEvent(context.Context, outbox.IngestQualityEventInput) (outbox.IngestQualityEventResult, error)
}

type qualityWebhookHTTPHandler struct {
	svc  qualityWebhookIngestService
	auth qualityWebhookAuthConfig
}

type qualityWebhookResponse struct {
	IssueRef   string `json:"issue_ref"`
	Duplicate  bool   `json:"duplicate"`
	Marker     string `json:"marker"`
	RoutedRole string `json:"routed_role"`
}

type qualityWebhookErrorResponse struct {
	Error string `json:"error"`
}

func init() {
	outboxQualityCmd.AddCommand(outboxQualityWebhookCmd)

	outboxQualityWebhookCmd.Flags().String("addr", ":8088", "Webhook listen address")
	outboxQualityWebhookCmd.Flags().String("github-secret", "", "GitHub webhook secret for X-Hub-Signature-256 (empty to skip)")
	outboxQualityWebhookCmd.Flags().String("gitlab-token", "", "GitLab webhook token for X-Gitlab-Token (empty to skip)")
}

func newOutboxQualityWebhookHandler(svc qualityWebhookIngestService, auth qualityWebhookAuthConfig) http.Handler {
	h := &qualityWebhookHTTPHandler{
		svc:  svc,
		auth: auth,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/webhooks/github", h.handleGitHub)
	mux.HandleFunc("/webhooks/gitlab", h.handleGitLab)
	return mux
}

func (h *qualityWebhookHTTPHandler) handleGitHub(w http.ResponseWriter, r *http.Request) {
	h.handleIngest(
		w,
		r,
		"github",
		strings.TrimSpace(r.Header.Get("X-GitHub-Delivery")),
		func(payload []byte) error {
			return validateGitHubSignature(h.auth.GitHubSecret, r.Header.Get("X-Hub-Signature-256"), payload)
		},
	)
}

func (h *qualityWebhookHTTPHandler) handleGitLab(w http.ResponseWriter, r *http.Request) {
	h.handleIngest(
		w,
		r,
		"gitlab",
		strings.TrimSpace(r.Header.Get("X-Gitlab-Event-UUID")),
		func(_ []byte) error {
			return validateGitLabToken(h.auth.GitLabToken, r.Header.Get("X-Gitlab-Token"))
		},
	)
}

func (h *qualityWebhookHTTPHandler) handleIngest(
	w http.ResponseWriter,
	r *http.Request,
	source string,
	externalEventID string,
	validateAuth func(payload []byte) error,
) {
	if r.Method != http.MethodPost {
		writeWebhookError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.svc == nil {
		writeWebhookError(w, http.StatusInternalServerError, "quality service is not configured")
		return
	}

	issueRef := strings.TrimSpace(r.URL.Query().Get("issue_ref"))
	if issueRef == "" {
		writeWebhookError(w, http.StatusBadRequest, "issue_ref is required")
		return
	}

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		writeWebhookError(w, http.StatusBadRequest, "failed to read payload")
		return
	}

	if err := validateAuth(payload); err != nil {
		writeWebhookError(w, http.StatusUnauthorized, err.Error())
		return
	}

	out, err := h.svc.IngestQualityEvent(r.Context(), outbox.IngestQualityEventInput{
		IssueRef:        issueRef,
		Source:          source,
		ExternalEventID: externalEventID,
		Payload:         string(payload),
	})
	if err != nil {
		writeWebhookError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeWebhookJSON(w, http.StatusOK, qualityWebhookResponse{
		IssueRef:   out.IssueRef,
		Duplicate:  out.Duplicate,
		Marker:     out.Marker,
		RoutedRole: out.RoutedRole,
	})
}

func validateGitHubSignature(secret string, signatureHeader string, payload []byte) error {
	normalizedSecret := strings.TrimSpace(secret)
	if normalizedSecret == "" {
		return nil
	}

	signature := strings.TrimSpace(signatureHeader)
	if signature == "" {
		return errors.New("missing X-Hub-Signature-256")
	}

	const prefix = "sha256="
	if len(signature) <= len(prefix) || !strings.EqualFold(signature[:len(prefix)], prefix) {
		return errors.New("invalid X-Hub-Signature-256 format")
	}

	decoded, err := hex.DecodeString(strings.TrimSpace(signature[len(prefix):]))
	if err != nil {
		return errors.New("invalid X-Hub-Signature-256 digest")
	}

	mac := hmac.New(sha256.New, []byte(normalizedSecret))
	if _, err := mac.Write(payload); err != nil {
		return errs.Wrap(err, "compute github webhook signature")
	}

	if !hmac.Equal(decoded, mac.Sum(nil)) {
		return errors.New("invalid X-Hub-Signature-256")
	}
	return nil
}

func validateGitLabToken(secret string, tokenHeader string) error {
	normalizedSecret := strings.TrimSpace(secret)
	if normalizedSecret == "" {
		return nil
	}

	token := strings.TrimSpace(tokenHeader)
	if token == "" {
		return errors.New("missing X-Gitlab-Token")
	}
	if !hmac.Equal([]byte(token), []byte(normalizedSecret)) {
		return errors.New("invalid X-Gitlab-Token")
	}
	return nil
}

func writeWebhookError(w http.ResponseWriter, status int, message string) {
	writeWebhookJSON(w, status, qualityWebhookErrorResponse{Error: message})
}

func writeWebhookJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
