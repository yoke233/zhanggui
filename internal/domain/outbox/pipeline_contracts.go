package outbox

import (
	"errors"
	"strings"
)

type PipelineRequest struct {
	IssueRef       string
	ProjectDir     string
	PromptFile     string
	CodingRole     string
	MaxReviewRound int
	MaxTestRound   int
}

func ValidatePipelineRequest(in PipelineRequest) error {
	if strings.TrimSpace(in.IssueRef) == "" {
		return errors.New("issue_ref is required")
	}
	if strings.TrimSpace(in.ProjectDir) == "" {
		return errors.New("project_dir is required")
	}
	if strings.TrimSpace(in.PromptFile) == "" {
		return errors.New("prompt_file is required")
	}
	return nil
}

func NormalizePipelineRequest(in PipelineRequest) (PipelineRequest, error) {
	if err := ValidatePipelineRequest(in); err != nil {
		return PipelineRequest{}, err
	}

	out := in
	if strings.TrimSpace(out.CodingRole) == "" {
		out.CodingRole = "backend"
	}
	if out.MaxReviewRound <= 0 {
		out.MaxReviewRound = 3
	}
	if out.MaxTestRound <= 0 {
		out.MaxTestRound = 3
	}
	return out, nil
}
