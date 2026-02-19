package outbox

import "testing"

func TestValidatePipelineRequest_RequireFields(t *testing.T) {
	req := PipelineRequest{}
	if err := ValidatePipelineRequest(req); err == nil {
		t.Fatalf("expected error for empty request")
	}
}

func TestValidatePipelineRequest_DefaultsAndBounds(t *testing.T) {
	req := PipelineRequest{
		IssueRef:       "local#1",
		ProjectDir:     "D:/project/zhanggui",
		PromptFile:     "mailbox/issue.md",
		CodingRole:     "backend",
		MaxReviewRound: 0,
		MaxTestRound:   0,
	}

	normalized, err := NormalizePipelineRequest(req)
	if err != nil {
		t.Fatalf("NormalizePipelineRequest() error = %v", err)
	}
	if normalized.MaxReviewRound != 3 || normalized.MaxTestRound != 3 {
		t.Fatalf("unexpected defaults: %#v", normalized)
	}
}
