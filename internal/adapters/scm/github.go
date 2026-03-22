package scm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	ghapi "github.com/google/go-github/v68/github"
	workspaceclone "github.com/yoke233/zhanggui/internal/adapters/workspace/clone"
	flowapp "github.com/yoke233/zhanggui/internal/application/flow"
	"golang.org/x/oauth2"
)

type GitHubProvider struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

func NewGitHubProvider(token string) *GitHubProvider {
	return &GitHubProvider{token: strings.TrimSpace(token)}
}

func (p *GitHubProvider) Kind() string { return "github" }

func (p *GitHubProvider) Detect(_ context.Context, originURL string) (flowapp.ChangeRequestRepo, bool, error) {
	origin := strings.TrimSpace(originURL)
	if origin == "" {
		return flowapp.ChangeRequestRepo{}, false, nil
	}
	remote, err := workspaceclone.ParseRemoteURL(origin)
	if err != nil {
		return flowapp.ChangeRequestRepo{}, false, nil
	}
	if strings.ToLower(strings.TrimSpace(remote.Host)) != "github.com" {
		return flowapp.ChangeRequestRepo{}, false, nil
	}
	return flowapp.ChangeRequestRepo{
		Kind:      p.Kind(),
		Host:      strings.ToLower(strings.TrimSpace(remote.Host)),
		Namespace: strings.TrimSpace(remote.Owner),
		Name:      strings.TrimSpace(remote.Repo),
	}, true, nil
}

func (p *GitHubProvider) EnsureOpen(ctx context.Context, repo flowapp.ChangeRequestRepo, input flowapp.EnsureOpenInput) (flowapp.ChangeRequest, bool, error) {
	if strings.TrimSpace(p.token) == "" {
		return flowapp.ChangeRequest{}, false, errors.New("github provider token is required")
	}
	client := ghapi.NewClient(oauth2.NewClient(context.Background(),
		oauth2.StaticTokenSource(&oauth2.Token{AccessToken: p.token}),
	))
	owner := strings.TrimSpace(repo.Namespace)
	name := strings.TrimSpace(repo.Name)
	if owner == "" || name == "" {
		return flowapp.ChangeRequest{}, false, errors.New("github repo is missing namespace/name")
	}

	title := strings.TrimSpace(input.Title)
	body := strings.TrimSpace(input.Body)
	head := strings.TrimSpace(input.Head)
	base := strings.TrimSpace(input.Base)
	if head == "" || base == "" {
		return flowapp.ChangeRequest{}, false, errors.New("github ensure open requires head/base")
	}

	req := &ghapi.NewPullRequest{
		Title: &title,
		Head:  &head,
		Base:  &base,
		Body:  &body,
	}
	if input.Draft {
		req.Draft = ghapi.Ptr(true)
	}

	pr, resp, err := client.PullRequests.Create(ctx, owner, name, req)
	if err == nil {
		return toChangeRequest(pr), true, nil
	}

	if resp != nil && resp.StatusCode == http.StatusUnprocessableEntity {
		headQ := owner + ":" + head
		list, _, listErr := client.PullRequests.List(ctx, owner, name, &ghapi.PullRequestListOptions{
			State: "open",
			Head:  headQ,
			Base:  base,
		})
		if listErr == nil && len(list) > 0 {
			return toChangeRequest(list[0]), false, nil
		}
	}

	return flowapp.ChangeRequest{}, false, fmt.Errorf("github create PR failed: %w", err)
}

func (p *GitHubProvider) Merge(ctx context.Context, repo flowapp.ChangeRequestRepo, number int, input flowapp.MergeInput) error {
	if strings.TrimSpace(p.token) == "" {
		return errors.New("github provider token is required")
	}
	if number <= 0 {
		return errors.New("github merge requires a positive PR number")
	}
	client, err := p.client()
	if err != nil {
		return err
	}
	owner := strings.TrimSpace(repo.Namespace)
	name := strings.TrimSpace(repo.Name)
	if owner == "" || name == "" {
		return errors.New("github repo is missing namespace/name")
	}

	msg := strings.TrimSpace(input.CommitMessage)
	opts := &ghapi.PullRequestOptions{
		MergeMethod: strings.TrimSpace(input.Method),
		SHA:         strings.TrimSpace(input.SHA),
	}
	title := strings.TrimSpace(input.CommitTitle)
	if title == "" {
		title = fmt.Sprintf("merge: #%d", number)
	}

	res, resp, err := client.PullRequests.Merge(ctx, owner, name, number, title+"\n\n"+msg, opts)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusMethodNotAllowed {
			pr, _, getErr := client.PullRequests.Get(ctx, owner, name, number)
			if getErr == nil && pr.GetMerged() {
				return nil
			}
			mergeErr := &flowapp.MergeError{
				Provider: "github",
				Repo:     repo,
				Number:   number,
				Message:  err.Error(),
			}
			if pr != nil {
				mergeErr.URL = strings.TrimSpace(pr.GetHTMLURL())
				mergeErr.MergeableState = strings.TrimSpace(pr.GetMergeableState())
				mergeErr.AlreadyMerged = pr.GetMerged()
			}
			return mergeErr
		}
		return fmt.Errorf("github merge PR #%d failed: %w", number, err)
	}
	if res != nil && !res.GetMerged() {
		return &flowapp.MergeError{
			Provider: "github",
			Repo:     repo,
			Number:   number,
			Message:  strings.TrimSpace(res.GetMessage()),
		}
	}
	return nil
}

func (p *GitHubProvider) GetState(ctx context.Context, repo flowapp.ChangeRequestRepo, number int) (string, error) {
	if strings.TrimSpace(p.token) == "" {
		return "", errors.New("github provider token is required")
	}
	if number <= 0 {
		return "", errors.New("github get state requires a positive PR number")
	}
	client, err := p.client()
	if err != nil {
		return "", err
	}
	owner := strings.TrimSpace(repo.Namespace)
	name := strings.TrimSpace(repo.Name)
	pr, _, err := client.PullRequests.Get(ctx, owner, name, number)
	if err != nil {
		return "", fmt.Errorf("github get PR #%d failed: %w", number, err)
	}
	if pr.GetMerged() {
		return "merged", nil
	}
	state := strings.ToLower(strings.TrimSpace(pr.GetState()))
	if state == "" {
		return "open", nil
	}
	return state, nil
}

func (p *GitHubProvider) client() (*ghapi.Client, error) {
	httpClient := p.httpClient
	if httpClient == nil {
		httpClient = oauth2.NewClient(context.Background(),
			oauth2.StaticTokenSource(&oauth2.Token{AccessToken: p.token}),
		)
	}
	client := ghapi.NewClient(httpClient)
	if strings.TrimSpace(p.baseURL) == "" {
		return client, nil
	}
	baseURL, err := url.Parse(strings.TrimSpace(p.baseURL))
	if err != nil {
		return nil, fmt.Errorf("invalid github base url: %w", err)
	}
	client.BaseURL = baseURL
	if !strings.HasSuffix(client.BaseURL.Path, "/") {
		client.BaseURL.Path += "/"
	}
	if client.UploadURL == nil {
		client.UploadURL = baseURL
	}
	return client, nil
}

func toChangeRequest(pr *ghapi.PullRequest) flowapp.ChangeRequest {
	cr := flowapp.ChangeRequest{}
	if pr == nil {
		return cr
	}
	cr.Number = pr.GetNumber()
	cr.URL = strings.TrimSpace(pr.GetHTMLURL())
	if pr.Head != nil {
		cr.HeadSHA = strings.TrimSpace(pr.Head.GetSHA())
	}
	return cr
}
