package outbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"zhanggui/internal/errs"
)

const (
	pipelineDefaultCodingRole     = "backend"
	pipelineDefaultMaxReviewRound = 3
	pipelineDefaultMaxTestRound   = 3
	pipelineSource                = "codex-pipeline"
	pipelineLeadActor             = "lead-pipeline"
	pipelineReviewActor           = "codex-reviewer"
	pipelineQAActor               = "codex-qa"
)

type RunCodexPipelineInput struct {
	IssueRef       string
	ProjectDir     string
	PromptFile     string
	CodingRole     string
	MaxReviewRound int
	MaxTestRound   int
}

type RunCodexPipelineResult struct {
	IssueRef       string
	Rounds         int
	ReadyToMerge   bool
	LastResult     string
	LastResultCode string
}

func (s *Service) RunCodexPipeline(ctx context.Context, in RunCodexPipelineInput) (RunCodexPipelineResult, error) {
	if ctx == nil {
		return RunCodexPipelineResult{}, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return RunCodexPipelineResult{}, errs.Wrap(err, "check context")
	}
	if s.repo == nil {
		return RunCodexPipelineResult{}, errors.New("outbox repository is required")
	}
	if s.uow == nil {
		return RunCodexPipelineResult{}, errors.New("outbox unit of work is required")
	}
	if s.codexRunner == nil {
		return RunCodexPipelineResult{}, errors.New("codex runner is required")
	}

	normalized, err := normalizeRunCodexPipelineInput(in)
	if err != nil {
		return RunCodexPipelineResult{}, err
	}
	if _, err := s.GetIssue(ctx, normalized.IssueRef); err != nil {
		return RunCodexPipelineResult{}, err
	}

	out := RunCodexPipelineResult{
		IssueRef:       normalized.IssueRef,
		LastResult:     "pipeline started",
		LastResultCode: "none",
	}
	pipelineExecutionID := newPipelineExecutionID()

	reviewFailures := 0
	testFailures := 0
	codingSeq := 0
	reviewSeq := 0
	testSeq := 0

	for {
		out.Rounds++

		codingSeq++
		codingRunID := pipelineRunID(pipelineExecutionID, CodexRunCoding, codingSeq)
		codingOut, runErr := s.codexRunner.Run(ctx, CodexRunInput{
			Mode:       CodexRunCoding,
			Role:       normalized.CodingRole,
			ProjectDir: normalized.ProjectDir,
			PromptFile: normalized.PromptFile,
			IssueRef:   normalized.IssueRef,
			RunID:      codingRunID,
		})
		if runErr != nil {
			return s.markPipelineManualIntervention(ctx, out, normalized.IssueRef, "coding step failed: "+runErr.Error())
		}
		if !isCodexRunPass(codingOut) {
			reason := firstNonEmpty(strings.TrimSpace(codingOut.Summary), "coding step did not pass")
			code := strings.TrimSpace(codingOut.ResultCode)
			if code != "" && code != "none" {
				reason = reason + " (" + code + ")"
			}
			return s.markPipelineManualIntervention(ctx, out, normalized.IssueRef, reason)
		}

		reviewSeq++
		reviewRunID := pipelineRunID(pipelineExecutionID, CodexRunReview, reviewSeq)
		reviewOut, runErr := s.codexRunner.Run(ctx, CodexRunInput{
			Mode:       CodexRunReview,
			Role:       "reviewer",
			ProjectDir: normalized.ProjectDir,
			PromptFile: normalized.PromptFile,
			IssueRef:   normalized.IssueRef,
			RunID:      reviewRunID,
		})
		if runErr != nil {
			return s.markPipelineManualIntervention(ctx, out, normalized.IssueRef, "review step failed: "+runErr.Error())
		}

		if isCodexRunPass(reviewOut) {
			reviewSummary := firstNonEmpty(strings.TrimSpace(reviewOut.Summary), "review approved")
			if err := s.ingestPipelineQualityEvent(ctx, normalized.IssueRef, qualityCategoryReview, qualityResultApproved, pipelineReviewActor, reviewSummary, optionalCodexEvidence(reviewOut.Evidence), pipelineExecutionID, reviewRunID); err != nil {
				return RunCodexPipelineResult{}, err
			}
			if err := s.AddIssueLabels(ctx, AddIssueLabelsInput{
				IssueRef: normalized.IssueRef,
				Actor:    pipelineLeadActor,
				Labels:   []string{"review:approved"},
			}); err != nil {
				return RunCodexPipelineResult{}, err
			}
		} else {
			reviewFailures++
			reviewSummary := firstNonEmpty(strings.TrimSpace(reviewOut.Summary), "review changes requested")
			reviewCode := firstNonEmpty(strings.TrimSpace(reviewOut.ResultCode), "review_changes_requested")

			if err := s.ingestPipelineQualityEvent(ctx, normalized.IssueRef, qualityCategoryReview, qualityResultChangesRequested, pipelineReviewActor, reviewSummary, requiredCodexEvidence(reviewOut.Evidence, CodexRunReview, reviewRunID), pipelineExecutionID, reviewRunID); err != nil {
				return RunCodexPipelineResult{}, err
			}

			if reviewFailures > normalized.MaxReviewRound {
				return s.markPipelineManualIntervention(ctx, out, normalized.IssueRef, fmt.Sprintf("review exceeded max rounds (%d)", normalized.MaxReviewRound))
			}

			out.LastResult = reviewSummary
			out.LastResultCode = reviewCode
			continue
		}

		testSeq++
		testRunID := pipelineRunID(pipelineExecutionID, CodexRunTest, testSeq)
		testOut, runErr := s.codexRunner.Run(ctx, CodexRunInput{
			Mode:       CodexRunTest,
			Role:       "qa",
			ProjectDir: normalized.ProjectDir,
			PromptFile: normalized.PromptFile,
			IssueRef:   normalized.IssueRef,
			RunID:      testRunID,
		})
		if runErr != nil {
			return s.markPipelineManualIntervention(ctx, out, normalized.IssueRef, "test step failed: "+runErr.Error())
		}

		if isCodexRunPass(testOut) {
			testSummary := firstNonEmpty(strings.TrimSpace(testOut.Summary), "ci checks passed")
			if err := s.ingestPipelineQualityEvent(ctx, normalized.IssueRef, qualityCategoryCI, qualityResultPass, pipelineQAActor, testSummary, optionalCodexEvidence(testOut.Evidence), pipelineExecutionID, testRunID); err != nil {
				return RunCodexPipelineResult{}, err
			}
			if err := s.AddIssueLabels(ctx, AddIssueLabelsInput{
				IssueRef: normalized.IssueRef,
				Actor:    pipelineLeadActor,
				Labels:   []string{"qa:pass"},
			}); err != nil {
				return RunCodexPipelineResult{}, err
			}

			ready, reason, err := s.CanMergeIssue(ctx, normalized.IssueRef)
			if err != nil {
				return RunCodexPipelineResult{}, err
			}
			if !ready {
				return s.markPipelineManualIntervention(ctx, out, normalized.IssueRef, firstNonEmpty(reason, "merge gate not ready"))
			}

			out.ReadyToMerge = true
			out.LastResult = "ready_to_merge"
			out.LastResultCode = "none"
			return out, nil
		}

		testFailures++
		testSummary := firstNonEmpty(strings.TrimSpace(testOut.Summary), "ci checks failed")
		testCode := firstNonEmpty(strings.TrimSpace(testOut.ResultCode), "ci_failed")

		if err := s.ingestPipelineQualityEvent(ctx, normalized.IssueRef, qualityCategoryCI, qualityResultFail, pipelineQAActor, testSummary, requiredCodexEvidence(testOut.Evidence, CodexRunTest, testRunID), pipelineExecutionID, testRunID); err != nil {
			return RunCodexPipelineResult{}, err
		}

		if testFailures > normalized.MaxTestRound {
			return s.markPipelineManualIntervention(ctx, out, normalized.IssueRef, fmt.Sprintf("test exceeded max rounds (%d)", normalized.MaxTestRound))
		}

		out.LastResult = testSummary
		out.LastResultCode = testCode
	}
}

func normalizeRunCodexPipelineInput(in RunCodexPipelineInput) (RunCodexPipelineInput, error) {
	out := in
	out.IssueRef = strings.TrimSpace(out.IssueRef)
	out.ProjectDir = strings.TrimSpace(out.ProjectDir)
	out.PromptFile = strings.TrimSpace(out.PromptFile)
	out.CodingRole = strings.TrimSpace(out.CodingRole)

	if out.IssueRef == "" {
		return RunCodexPipelineInput{}, errors.New("issue ref is required")
	}
	if out.ProjectDir == "" {
		return RunCodexPipelineInput{}, errors.New("project dir is required")
	}
	if out.PromptFile == "" {
		return RunCodexPipelineInput{}, errors.New("prompt file is required")
	}
	if out.CodingRole == "" {
		out.CodingRole = pipelineDefaultCodingRole
	}
	if out.MaxReviewRound <= 0 {
		out.MaxReviewRound = pipelineDefaultMaxReviewRound
	}
	if out.MaxTestRound <= 0 {
		out.MaxTestRound = pipelineDefaultMaxTestRound
	}
	return out, nil
}

func (s *Service) ingestPipelineQualityEvent(ctx context.Context, issueRef string, category string, resultValue string, actor string, summary string, evidence []string, executionID string, runID string) error {
	externalEventID := fmt.Sprintf("pipeline:%s:%s:%s", executionID, category, runID)
	_, err := s.IngestQualityEvent(ctx, IngestQualityEventInput{
		IssueRef:         issueRef,
		Source:           pipelineSource,
		ExternalEventID:  externalEventID,
		Category:         category,
		Result:           resultValue,
		Actor:            actor,
		Summary:          summary,
		Evidence:         evidence,
		ProvidedEventKey: fmt.Sprintf("pipeline:%s:%s:%s:%s", issueRef, executionID, category, runID),
	})
	return err
}

func (s *Service) markPipelineManualIntervention(ctx context.Context, out RunCodexPipelineResult, issueRef string, reason string) (RunCodexPipelineResult, error) {
	if err := s.AddIssueLabels(ctx, AddIssueLabelsInput{
		IssueRef: issueRef,
		Actor:    pipelineLeadActor,
		Labels:   []string{"needs-human", "state:blocked"},
	}); err != nil {
		return RunCodexPipelineResult{}, err
	}

	out.ReadyToMerge = false
	out.LastResult = firstNonEmpty(strings.TrimSpace(reason), "manual intervention required")
	out.LastResultCode = "manual_intervention"
	return out, nil
}

func pipelineRunID(executionID string, mode CodexRunMode, seq int) string {
	normalizedMode := strings.TrimSpace(string(mode))
	if normalizedMode == "" {
		normalizedMode = "unknown"
	}
	normalizedExecution := strings.TrimSpace(executionID)
	if normalizedExecution == "" {
		normalizedExecution = "run-unknown"
	}
	return fmt.Sprintf("pipeline-%s-%s-%03d", normalizedExecution, normalizedMode, seq)
}

func newPipelineExecutionID() string {
	return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
}

func isCodexRunPass(out CodexRunOutput) bool {
	return strings.EqualFold(strings.TrimSpace(out.Status), "pass")
}

func optionalCodexEvidence(value string) []string {
	item := strings.TrimSpace(value)
	if item == "" {
		return nil
	}
	return []string{item}
}

func requiredCodexEvidence(value string, mode CodexRunMode, runID string) []string {
	item := strings.TrimSpace(value)
	if item != "" {
		return []string{item}
	}
	return []string{fmt.Sprintf("codex://%s/%s", strings.TrimSpace(string(mode)), strings.TrimSpace(runID))}
}
