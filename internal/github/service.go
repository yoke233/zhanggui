package github

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ghapi "github.com/google/go-github/v68/github"
)

const statusLabelPrefix = "status:"

// GitHubService provides reusable Issue/PR operations for business plugins.
type GitHubService struct {
	client *ghapi.Client
	owner  string
	repo   string
}

type CreateIssueInput struct {
	Title  string
	Body   string
	Labels []string
}

type CreatePRInput struct {
	Title               string
	Body                string
	Head                string
	Base                string
	Draft               bool
	MaintainerCanModify *bool
}

type UpdatePRInput struct {
	Title               *string
	Body                *string
	Base                *string
	State               *string
	MaintainerCanModify *bool
}

type MergePRInput struct {
	CommitTitle        string
	CommitMessage      string
	MergeMethod        string
	SHA                string
	DontDefaultIfBlank bool
}

func NewGitHubService(client *Client, owner string, repo string) (*GitHubService, error) {
	if client == nil || client.Client() == nil {
		return nil, errors.New("github service init: github client is required")
	}

	trimmedOwner := strings.TrimSpace(owner)
	trimmedRepo := strings.TrimSpace(repo)
	if trimmedOwner == "" || trimmedRepo == "" {
		return nil, errors.New("github service init: owner and repo are required")
	}

	return &GitHubService{
		client: client.Client(),
		owner:  trimmedOwner,
		repo:   trimmedRepo,
	}, nil
}

func (s *GitHubService) CreateIssue(ctx context.Context, input CreateIssueInput) (*ghapi.Issue, error) {
	if err := s.ensureReady("create issue"); err != nil {
		return nil, err
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, s.inputError("create issue", "title is required")
	}

	req := &ghapi.IssueRequest{
		Title: &title,
	}
	if body := strings.TrimSpace(input.Body); body != "" {
		req.Body = &body
	}
	if labels := normalizeLabels(input.Labels); len(labels) > 0 {
		req.Labels = &labels
	}

	issue, _, err := s.client.Issues.Create(ctx, s.owner, s.repo, req)
	if err != nil {
		return nil, s.wrapError("create issue", err)
	}
	return issue, nil
}

func (s *GitHubService) UpdateIssueLabels(ctx context.Context, issueNumber int, labels []string) error {
	if err := s.ensureReady("update issue labels"); err != nil {
		return err
	}

	currentLabels, _, err := s.client.Issues.ListLabelsByIssue(ctx, s.owner, s.repo, issueNumber, nil)
	if err != nil {
		return s.wrapError("list issue labels", err)
	}

	for _, label := range currentLabels {
		name := strings.TrimSpace(label.GetName())
		if !isStatusLabel(name) {
			continue
		}
		if _, removeErr := s.client.Issues.RemoveLabelForIssue(ctx, s.owner, s.repo, issueNumber, name); removeErr != nil {
			return s.wrapError("remove issue status label", removeErr)
		}
	}

	added := normalizeLabels(labels)
	if len(added) == 0 {
		return nil
	}

	if _, _, err := s.client.Issues.AddLabelsToIssue(ctx, s.owner, s.repo, issueNumber, added); err != nil {
		return s.wrapError("add issue labels", err)
	}
	return nil
}

func (s *GitHubService) AddIssueComment(ctx context.Context, issueNumber int, body string) (*ghapi.IssueComment, error) {
	if err := s.ensureReady("add issue comment"); err != nil {
		return nil, err
	}

	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return nil, s.inputError("add issue comment", "body is required")
	}

	req := &ghapi.IssueComment{Body: &trimmed}
	comment, _, err := s.client.Issues.CreateComment(ctx, s.owner, s.repo, issueNumber, req)
	if err != nil {
		return nil, s.wrapError("add issue comment", err)
	}
	return comment, nil
}

func (s *GitHubService) CreatePR(ctx context.Context, input CreatePRInput) (*ghapi.PullRequest, error) {
	if err := s.ensureReady("create pr"); err != nil {
		return nil, err
	}

	title := strings.TrimSpace(input.Title)
	head := strings.TrimSpace(input.Head)
	base := strings.TrimSpace(input.Base)
	if title == "" || head == "" || base == "" {
		return nil, s.inputError("create pr", "title/head/base are required")
	}

	req := &ghapi.NewPullRequest{
		Title: &title,
		Head:  &head,
		Base:  &base,
		Draft: &input.Draft,
	}
	if body := strings.TrimSpace(input.Body); body != "" {
		req.Body = &body
	}
	if input.MaintainerCanModify != nil {
		req.MaintainerCanModify = input.MaintainerCanModify
	}

	pr, _, err := s.client.PullRequests.Create(ctx, s.owner, s.repo, req)
	if err != nil {
		return nil, s.wrapError("create pr", err)
	}
	return pr, nil
}

func (s *GitHubService) UpdatePR(ctx context.Context, number int, input UpdatePRInput) (*ghapi.PullRequest, error) {
	if err := s.ensureReady("update pr"); err != nil {
		return nil, err
	}

	update := &ghapi.PullRequest{}
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		update.Title = &title
	}
	if input.Body != nil {
		body := strings.TrimSpace(*input.Body)
		update.Body = &body
	}
	if input.State != nil {
		state := strings.TrimSpace(*input.State)
		update.State = &state
	}
	if input.Base != nil {
		ref := strings.TrimSpace(*input.Base)
		update.Base = &ghapi.PullRequestBranch{Ref: &ref}
	}
	if input.MaintainerCanModify != nil {
		update.MaintainerCanModify = input.MaintainerCanModify
	}

	pr, _, err := s.client.PullRequests.Edit(ctx, s.owner, s.repo, number, update)
	if err != nil {
		return nil, s.wrapError("update pr", err)
	}
	return pr, nil
}

func (s *GitHubService) MergePR(ctx context.Context, number int, input MergePRInput) (*ghapi.PullRequestMergeResult, error) {
	if err := s.ensureReady("merge pr"); err != nil {
		return nil, err
	}

	trimmedMessage := strings.TrimSpace(input.CommitMessage)
	options := &ghapi.PullRequestOptions{
		CommitTitle:        strings.TrimSpace(input.CommitTitle),
		SHA:                strings.TrimSpace(input.SHA),
		MergeMethod:        strings.TrimSpace(input.MergeMethod),
		DontDefaultIfBlank: input.DontDefaultIfBlank,
	}
	if options.CommitTitle == "" && options.SHA == "" && options.MergeMethod == "" && !options.DontDefaultIfBlank {
		options = nil
	}

	result, _, err := s.client.PullRequests.Merge(ctx, s.owner, s.repo, number, trimmedMessage, options)
	if err != nil {
		return nil, s.wrapError("merge pr", err)
	}
	return result, nil
}

func (s *GitHubService) ClosePR(ctx context.Context, number int) (*ghapi.PullRequest, error) {
	closed := "closed"
	return s.UpdatePR(ctx, number, UpdatePRInput{State: &closed})
}

func (s *GitHubService) ensureReady(operation string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("github service %s: client is not initialized", operation)
	}
	if strings.TrimSpace(s.owner) == "" || strings.TrimSpace(s.repo) == "" {
		return fmt.Errorf("github service %s: owner/repo are required", operation)
	}
	return nil
}

func (s *GitHubService) wrapError(operation string, err error) error {
	return fmt.Errorf("github service %s (%s/%s): %w", operation, s.owner, s.repo, err)
}

func (s *GitHubService) inputError(operation string, message string) error {
	return fmt.Errorf("github service %s (%s/%s): %s", operation, s.owner, s.repo, message)
}

func isStatusLabel(label string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(label)), statusLabelPrefix)
}

func normalizeLabels(labels []string) []string {
	if len(labels) == 0 {
		return nil
	}

	out := make([]string, 0, len(labels))
	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		trimmed := strings.TrimSpace(label)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
