package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
	"github.com/yoke233/ai-workflow/internal/teamleader"
	"github.com/yoke233/ai-workflow/internal/web"
)

// --- DepScheduler adapter ---

type depSchedulerIssueAdapter struct {
	scheduler *teamleader.DepScheduler
}

func (a *depSchedulerIssueAdapter) Start(ctx context.Context) error {
	if a == nil || a.scheduler == nil {
		return errors.New("issue scheduler is not configured")
	}
	return a.scheduler.Start(ctx)
}

func (a *depSchedulerIssueAdapter) Stop(ctx context.Context) error {
	if a == nil || a.scheduler == nil {
		return nil
	}
	return a.scheduler.Stop(ctx)
}

func (a *depSchedulerIssueAdapter) RecoverExecutingIssues(ctx context.Context) error {
	if a == nil || a.scheduler == nil {
		return errors.New("issue scheduler is not configured")
	}
	return a.scheduler.RecoverExecutingIssues(ctx, "")
}

func (a *depSchedulerIssueAdapter) StartIssue(ctx context.Context, issue *core.Issue) error {
	if a == nil || a.scheduler == nil {
		return errors.New("issue scheduler is not configured")
	}
	return a.scheduler.ScheduleIssues(ctx, []*core.Issue{issue})
}

// --- TeamLeader IssueManager adapter ---

type teamLeaderIssueService interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	CreateIssues(ctx context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error)
	SubmitForReview(ctx context.Context, issueIDs []string) error
	ApplyIssueAction(ctx context.Context, issueID, action, feedback string) (*core.Issue, error)
}

type teamLeaderIssueManagerAdapter struct {
	manager teamLeaderIssueService
	store   core.Store
}

func (a *teamLeaderIssueManagerAdapter) Start(ctx context.Context) error {
	if a == nil || a.manager == nil {
		return errors.New("issue manager is not configured")
	}
	return a.manager.Start(ctx)
}

func (a *teamLeaderIssueManagerAdapter) Stop(ctx context.Context) error {
	if a == nil || a.manager == nil {
		return nil
	}
	return a.manager.Stop(ctx)
}

func (a *teamLeaderIssueManagerAdapter) CreateIssues(ctx context.Context, input web.IssueCreateInput) ([]core.Issue, error) {
	if a == nil || a.manager == nil {
		return nil, errors.New("issue manager is not configured")
	}

	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		return nil, errors.New("project id is required")
	}

	failPolicy := input.FailPolicy
	if failPolicy == "" {
		failPolicy = core.FailBlock
	}

	created, err := a.manager.CreateIssues(ctx, teamleader.CreateIssuesInput{
		ProjectID: projectID,
		SessionID: strings.TrimSpace(input.SessionID),
		Issues: []teamleader.CreateIssueSpec{
			{
				Title:      resolveIssueTitle(input),
				Body:       buildIssueBody(input),
				Template:   "standard",
				AutoMerge:  input.AutoMerge,
				Labels:     resolveIssueLabels(input),
				FailPolicy: failPolicy,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	out := make([]core.Issue, 0, len(created))
	for i := range created {
		if created[i] == nil {
			continue
		}
		out = append(out, *created[i])
	}
	return out, nil
}

func (a *teamLeaderIssueManagerAdapter) SubmitForReview(ctx context.Context, issueID string, _ web.IssueReviewInput) (*core.Issue, error) {
	if a == nil || a.manager == nil {
		return nil, errors.New("issue manager is not configured")
	}
	id := strings.TrimSpace(issueID)
	if id == "" {
		return nil, errors.New("issue id is required")
	}
	if err := a.manager.SubmitForReview(ctx, []string{id}); err != nil {
		return nil, err
	}
	if a.store == nil {
		return &core.Issue{ID: id}, nil
	}
	return a.store.GetIssue(id)
}

func (a *teamLeaderIssueManagerAdapter) ApplyIssueAction(ctx context.Context, issueID string, action web.IssueAction) (*core.Issue, error) {
	if a == nil || a.manager == nil {
		return nil, errors.New("issue manager is not configured")
	}
	feedback := ""
	if action.Feedback != nil {
		feedback = strings.TrimSpace(action.Feedback.Detail)
	}
	return a.manager.ApplyIssueAction(ctx, issueID, action.Action, feedback)
}

// --- Issue helper functions ---

func resolveIssueTitle(input web.IssueCreateInput) string {
	if trimmed := strings.TrimSpace(input.Name); trimmed != "" {
		return trimmed
	}
	if len(input.SourceFiles) == 1 {
		return fmt.Sprintf("Plan from %s", strings.TrimSpace(input.SourceFiles[0]))
	}
	if len(input.SourceFiles) > 1 {
		return fmt.Sprintf("Plan from %d files", len(input.SourceFiles))
	}
	return "Plan from chat session"
}

func resolveIssueLabels(input web.IssueCreateInput) []string {
	labels := []string{"plan"}
	if len(input.SourceFiles) > 0 {
		labels = append(labels, "from-files")
	}
	return labels
}

func buildIssueBody(input web.IssueCreateInput) string {
	parts := make([]string, 0, 3)

	conversation := strings.TrimSpace(input.Request.Conversation)
	if conversation != "" {
		parts = append(parts, "## Conversation\n\n"+conversation)
	}

	if len(input.SourceFiles) > 0 {
		var b strings.Builder
		b.WriteString("## Source Files\n\n")
		for _, file := range input.SourceFiles {
			path := strings.TrimSpace(file)
			if path == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(path)
			b.WriteString("\n")
		}
		for _, file := range input.SourceFiles {
			path := strings.TrimSpace(file)
			if path == "" {
				continue
			}
			content, ok := input.FileContents[path]
			if !ok {
				continue
			}
			b.WriteString("\n### ")
			b.WriteString(path)
			b.WriteString("\n\n```text\n")
			b.WriteString(strings.TrimSpace(content))
			b.WriteString("\n```\n")
		}
		parts = append(parts, strings.TrimSpace(b.String()))
	}

	if len(parts) == 0 {
		return "Auto-created issue from chat session."
	}
	return strings.Join(parts, "\n\n")
}

// --- ACP handler factory adapter ---

// acpHandlerFactoryAdapter bridges engine.ACPHandlerFactory to teamleader.ACPHandler.
type acpHandlerFactoryAdapter struct{}

func (f *acpHandlerFactoryAdapter) NewHandler(cwd string, publisher engine.ACPEventPublisher) acpproto.Client {
	return teamleader.NewACPHandler(cwd, "", publisher)
}

func (f *acpHandlerFactoryAdapter) SetPermissionPolicy(handler acpproto.Client, policy []acpclient.PermissionRule) {
	if h, ok := handler.(*teamleader.ACPHandler); ok {
		h.SetPermissionPolicy(policy)
	}
}
