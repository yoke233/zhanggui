package github

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type prLifecycleSCM interface {
	CreatePR(ctx context.Context, req core.PullRequest) (prURL string, err error)
	ConvertToReady(ctx context.Context, number int) error
	MergePR(ctx context.Context, req core.PullRequestMerge) error
}

type PRLifecycle struct {
	store core.Store
	scm   prLifecycleSCM
	now   func() time.Time
}

func NewPRLifecycle(store core.Store, scm prLifecycleSCM) *PRLifecycle {
	return &PRLifecycle{
		store: store,
		scm:   scm,
		now:   time.Now,
	}
}

func (l *PRLifecycle) OnImplementComplete(ctx context.Context, RunID string) (string, error) {
	if l == nil || l.store == nil {
		return "", errors.New("pr lifecycle store is required")
	}
	if l.scm == nil {
		return "", errors.New("pr lifecycle scm is required")
	}

	Run, err := l.store.GetRun(strings.TrimSpace(RunID))
	if err != nil {
		return "", err
	}

	if existing := prNumberFromRun(Run); existing > 0 {
		if Run.Config == nil {
			Run.Config = map[string]any{}
		}
		if url, _ := Run.Config["pr_url"].(string); strings.TrimSpace(url) != "" {
			return strings.TrimSpace(url), nil
		}
	}

	base := "main"
	if Run.Config != nil {
		if v, _ := Run.Config["base_branch"].(string); strings.TrimSpace(v) != "" {
			base = strings.TrimSpace(v)
		}
	}
	head := strings.TrimSpace(Run.BranchName)
	if head == "" {
		head = "ai-flow/" + Run.ID
	}

	body := Run.Description
	if issueNumber := issueNumberFromRun(Run); issueNumber > 0 {
		body = fmt.Sprintf("%s\n\nCloses #%d", body, issueNumber)
	}

	draft := true
	prURL, err := l.scm.CreatePR(ctx, core.PullRequest{
		Title: Run.Name,
		Body:  body,
		Head:  head,
		Base:  base,
		Draft: &draft,
	})
	if err != nil {
		return "", err
	}

	if Run.Config == nil {
		Run.Config = map[string]any{}
	}
	Run.Config["pr_url"] = strings.TrimSpace(prURL)
	if prNumber := parsePRNumber(prURL); prNumber > 0 {
		Run.Config["pr_number"] = prNumber
	}
	Run.UpdatedAt = l.now()
	if err := l.store.SaveRun(Run); err != nil {
		return "", err
	}
	return strings.TrimSpace(prURL), nil
}

func (l *PRLifecycle) OnMergeApproved(ctx context.Context, RunID string) error {
	if l == nil || l.store == nil {
		return errors.New("pr lifecycle store is required")
	}
	if l.scm == nil {
		return errors.New("pr lifecycle scm is required")
	}

	Run, err := l.store.GetRun(strings.TrimSpace(RunID))
	if err != nil {
		return err
	}
	prNumber := prNumberFromRun(Run)
	if prNumber <= 0 {
		return errors.New("pr number is required")
	}

	if err := l.scm.ConvertToReady(ctx, prNumber); err != nil {
		return err
	}
	return l.scm.MergePR(ctx, core.PullRequestMerge{
		Number:      prNumber,
		CommitTitle: fmt.Sprintf("merge Run %s", Run.ID),
	})
}

func (l *PRLifecycle) OnPullRequestClosed(
	ctx context.Context,
	projectID string,
	prNumber int,
	merged bool,
) error {
	if l == nil || l.store == nil || strings.TrimSpace(projectID) == "" || prNumber <= 0 {
		return nil
	}

	Run, err := findRunByPRNumber(l.store, projectID, prNumber)
	if err != nil {
		return err
	}
	if Run == nil {
		return nil
	}

	if merged {
		Run.Status = core.StatusCompleted
		Run.Conclusion = core.ConclusionSuccess
		Run.ErrorMessage = ""
	} else {
		Run.Status = core.StatusCompleted
		Run.Conclusion = core.ConclusionFailure
		Run.ErrorMessage = "pull request closed without merge"
	}
	Run.FinishedAt = l.now()
	Run.UpdatedAt = l.now()
	return l.store.SaveRun(Run)
}

func findRunByPRNumber(store core.Store, projectID string, prNumber int) (*core.Run, error) {
	if store == nil || strings.TrimSpace(projectID) == "" || prNumber <= 0 {
		return nil, nil
	}

	Runs, err := store.ListRuns(projectID, core.RunFilter{Limit: 500})
	if err != nil {
		return nil, err
	}
	for i := range Runs {
		Run, err := store.GetRun(Runs[i].ID)
		if err != nil {
			return nil, err
		}
		if prNumberFromRun(Run) == prNumber {
			return Run, nil
		}
	}
	return nil, nil
}

func issueNumberFromRun(p *core.Run) int {
	if p == nil || p.Config == nil {
		return 0
	}
	for _, key := range []string{"issue_number", "github_issue_number"} {
		if n := parsePRNumberValue(p.Config[key]); n > 0 {
			return n
		}
	}
	return 0
}

func prNumberFromRun(p *core.Run) int {
	if p == nil {
		return 0
	}
	if p.Config != nil {
		for _, key := range []string{"pr_number", "github_pr_number"} {
			if prNumber := parsePRNumberValue(p.Config[key]); prNumber > 0 {
				return prNumber
			}
		}
	}
	if p.Artifacts != nil {
		for _, key := range []string{"pr_number", "github_pr_number"} {
			if prNumber := parsePRNumberValue(p.Artifacts[key]); prNumber > 0 {
				return prNumber
			}
		}
	}
	return 0
}

func parsePRNumberValue(raw any) int {
	switch value := raw.(type) {
	case int:
		if value > 0 {
			return value
		}
	case int32:
		if value > 0 {
			return int(value)
		}
	case int64:
		if value > 0 {
			return int(value)
		}
	case float64:
		if value > 0 {
			return int(value)
		}
	case string:
		if number := parsePRNumber(strings.TrimSpace(value)); number > 0 {
			return number
		}
	}
	return 0
}

func parsePRNumber(prURL string) int {
	trimmed := strings.TrimSpace(prURL)
	if trimmed == "" {
		return 0
	}

	parts := strings.Split(trimmed, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil && n > 0 {
			return n
		}
	}
	return 0
}
