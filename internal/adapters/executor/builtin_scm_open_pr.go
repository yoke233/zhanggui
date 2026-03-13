package executor

import (
	"context"
	"fmt"
	"strings"
	"time"

	scmadapter "github.com/yoke233/ai-workflow/internal/adapters/scm"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
)

// runBuiltinSCMOpenPR creates or finds an open change request using the registered SCM providers.
// It is provider-agnostic (GitHub PR today; GitLab MR later).
func runBuiltinSCMOpenPR(ctx context.Context, store core.Store, bus core.EventBus, tokens flowapp.SCMTokens, step *core.Step, execRec *core.Execution) error {
	if store == nil {
		return fmt.Errorf("builtin scm_open_pr: store is nil")
	}
	ws := flowapp.WorkspaceFromContext(ctx)
	if ws == nil || strings.TrimSpace(ws.Path) == "" {
		return fmt.Errorf("builtin scm_open_pr: workspace is required")
	}

	baseBranch := "main"
	if ws.Metadata != nil {
		if v, ok := ws.Metadata["default_branch"].(string); ok && strings.TrimSpace(v) != "" {
			baseBranch = strings.TrimSpace(v)
		}
	}
	if step.Config != nil {
		if v, ok := step.Config["base"].(string); ok && strings.TrimSpace(v) != "" {
			baseBranch = strings.TrimSpace(v)
		}
	}

	headBranch := ""
	if ws.Metadata != nil {
		if v, ok := ws.Metadata["branch"].(string); ok && strings.TrimSpace(v) != "" {
			headBranch = strings.TrimSpace(v)
		}
	}
	if headBranch == "" {
		headBranch = "HEAD"
	}

	title := fmt.Sprintf("ai-flow: issue %d", step.IssueID)
	body := "Automated change request created by ai-workflow."
	if step.Config != nil {
		if v, ok := step.Config["title"].(string); ok && strings.TrimSpace(v) != "" {
			title = strings.TrimSpace(v)
		}
		if v, ok := step.Config["body"].(string); ok && strings.TrimSpace(v) != "" {
			body = strings.TrimSpace(v)
		}
	}

	originURL, err := gitOutput(ctx, ws.Path, nil, "remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("builtin scm_open_pr: resolve origin url: %w", err)
	}
	originURL = strings.TrimSpace(originURL)

	token := tokens.EffectivePAT()
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("builtin scm_open_pr: missing SCM PAT")
	}

	providers := scmadapter.NewChangeRequestProviders(token)
	provider, repo, ok, err := scmadapter.DetectChangeRequestProvider(ctx, originURL, providers)
	if err != nil {
		return err
	}
	if !ok || provider == nil {
		return fmt.Errorf("builtin scm_open_pr: unsupported origin url: %s", originURL)
	}

	var fallbackProvider flowapp.ChangeRequestProvider

	extra := map[string]any{}
	if ws.Metadata != nil {
		for _, key := range []string{
			"organization_id",
			"repository_id",
			"project_id",
			"source_project_id",
			"target_project_id",
			"reviewer_user_ids",
			"trigger_ai_review_run",
			"work_item_ids",
		} {
			if value, exists := ws.Metadata[key]; exists {
				extra[key] = value
			}
		}
	}
	if step.Config != nil {
		for _, key := range []string{
			"organization_id",
			"repository_id",
			"project_id",
			"source_project_id",
			"target_project_id",
			"reviewer_user_ids",
			"trigger_ai_review_run",
			"work_item_ids",
		} {
			if value, exists := step.Config[key]; exists {
				extra[key] = value
			}
		}
	}

	cr, created, err := provider.EnsureOpen(ctx, repo, flowapp.EnsureOpenInput{
		Head:  headBranch,
		Base:  baseBranch,
		Title: title,
		Body:  body,
		Extra: extra,
	})
	if err != nil {
		// Auto-fallback: if commit PAT is invalid/insufficient, retry with merge PAT.
		if fallbackProvider != nil && isAuthError(err) {
			cr2, created2, err2 := fallbackProvider.EnsureOpen(ctx, repo, flowapp.EnsureOpenInput{
				Head:  headBranch,
				Base:  baseBranch,
				Title: title,
				Body:  body,
				Extra: extra,
			})
			if err2 == nil {
				cr, created, err = cr2, created2, nil
			} else {
				return err2
			}
		} else {
			return err
		}
	}

	md := strings.Join([]string{
		"## Change Request",
		"",
		fmt.Sprintf("- provider: %s", repo.Kind),
		fmt.Sprintf("- action: %s", map[bool]string{true: "created", false: "found"}[created]),
		fmt.Sprintf("- number: %d", cr.Number),
		fmt.Sprintf("- url: %s", cr.URL),
		fmt.Sprintf("- base: %s", baseBranch),
		fmt.Sprintf("- head: %s", headBranch),
		fmt.Sprintf("- head_sha: %s", cr.HeadSHA),
		fmt.Sprintf("- time_utc: %s", time.Now().UTC().Format(time.RFC3339)),
	}, "\n")

	return storeBuiltinArtifact(ctx, store, bus, step, execRec, md, map[string]any{
		"provider":    repo.Kind,
		"pr_number":   cr.Number,
		"pr_url":      cr.URL,
		"base_branch": baseBranch,
		"head_branch": headBranch,
		"head_sha":    cr.HeadSHA,
	})
}
