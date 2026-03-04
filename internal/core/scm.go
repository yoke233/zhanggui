package core

import "context"

// SCM defines source-control operations used by Run/task automation.
type SCM interface {
	Plugin
	CreateBranch(ctx context.Context, branch string) error
	Commit(ctx context.Context, message string) (commitHash string, err error)
	Push(ctx context.Context, remote string, branch string) error
	Merge(ctx context.Context, branch string) (mergeCommit string, err error)
	CreatePR(ctx context.Context, req PullRequest) (prURL string, err error)
	UpdatePR(ctx context.Context, req PullRequestUpdate) error
	ConvertToReady(ctx context.Context, number int) error
	MergePR(ctx context.Context, req PullRequestMerge) error
}

type PullRequest struct {
	Title     string
	Body      string
	Head      string
	Base      string
	Draft     *bool
	Reviewers []string
}

type PullRequestUpdate struct {
	Number              int
	Title               *string
	Body                *string
	Base                *string
	State               *string
	MaintainerCanModify *bool
	AddComment          string
}

type PullRequestMerge struct {
	Number             int
	CommitTitle        string
	CommitMessage      string
	Method             string
	SHA                string
	DontDefaultIfBlank bool
}
