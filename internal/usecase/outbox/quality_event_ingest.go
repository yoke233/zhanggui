package outbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"zhanggui/internal/errs"
	"zhanggui/internal/ports"
)

const (
	qualityCategoryReview = "review"
	qualityCategoryCI     = "ci"

	qualityResultApproved         = "approved"
	qualityResultChangesRequested = "changes_requested"
	qualityResultPass             = "pass"
	qualityResultFail             = "fail"
)

func (s *Service) IngestQualityEvent(ctx context.Context, input IngestQualityEventInput) (IngestQualityEventResult, error) {
	if ctx == nil {
		return IngestQualityEventResult{}, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return IngestQualityEventResult{}, errs.Wrap(err, "check context")
	}
	if s.repo == nil {
		return IngestQualityEventResult{}, errors.New("outbox repository is required")
	}
	if s.uow == nil {
		return IngestQualityEventResult{}, errors.New("outbox unit of work is required")
	}

	issueRef := strings.TrimSpace(input.IssueRef)
	if issueRef == "" {
		return IngestQualityEventResult{}, errors.New("issue ref is required")
	}
	issueID, err := parseIssueRef(issueRef)
	if err != nil {
		return IngestQualityEventResult{}, err
	}

	source := firstNonEmpty(strings.ToLower(strings.TrimSpace(input.Source)), "manual")
	input = mergeInputWithInferredPayloadFields(input, source)

	category, err := normalizeQualityCategory(input.Category)
	if err != nil {
		return IngestQualityEventResult{}, err
	}
	resultValue, err := normalizeQualityResult(category, input.Result)
	if err != nil {
		return IngestQualityEventResult{}, err
	}

	externalEventID := firstNonEmpty(strings.TrimSpace(input.ExternalEventID), "none")
	actor := firstNonEmpty(strings.TrimSpace(input.Actor), "quality-bot")
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		summary = defaultQualitySummary(category, resultValue)
	}
	evidence := normalizeEvidenceLinks(input.Evidence)
	if isQualityFailure(category, resultValue) && len(evidence) == 0 {
		return IngestQualityEventResult{}, errors.New("quality failure requires evidence")
	}

	payloadJSON := strings.TrimSpace(input.Payload)
	if payloadJSON == "" {
		payloadJSON, err = marshalDefaultQualityPayload(issueRef, source, externalEventID, category, resultValue, actor, summary, evidence)
		if err != nil {
			return IngestQualityEventResult{}, err
		}
	}

	idempotencyKey := strings.TrimSpace(input.ProvidedEventKey)
	if idempotencyKey == "" {
		idempotencyKey = deriveQualityEventKey(issueRef, source, externalEventID, category, resultValue, actor, summary, evidence, payloadJSON)
	}

	ingestedAt := nowUTCString()
	evidenceJSON := "[]"
	if len(evidence) > 0 {
		rawEvidence, marshalErr := json.Marshal(evidence)
		if marshalErr != nil {
			return IngestQualityEventResult{}, marshalErr
		}
		evidenceJSON = string(rawEvidence)
	}

	writeback, err := resolveQualityWriteback(category, resultValue, summary)
	if err != nil {
		return IngestQualityEventResult{}, err
	}

	out := IngestQualityEventResult{
		IssueRef:         issueRef,
		IdempotencyKey:   idempotencyKey,
		NormalizedKind:   category,
		NormalizedResult: resultValue,
		Marker:           writeback.Marker,
	}

	if err := s.uow.WithTx(ctx, func(txCtx context.Context) error {
		issue, getErr := s.repo.GetIssue(txCtx, issueID)
		if getErr != nil {
			if errors.Is(getErr, ports.ErrIssueNotFound) {
				return fmt.Errorf("issue %s not found", issueRef)
			}
			return getErr
		}
		if issue.IsClosed {
			return fmt.Errorf("issue %s is closed", issueRef)
		}

		labels, labelsErr := s.repo.ListIssueLabels(txCtx, issueID)
		if labelsErr != nil {
			return labelsErr
		}

		inserted, createErr := s.repo.CreateQualityEvent(txCtx, ports.OutboxQualityEventCreate{
			IssueID:         issueID,
			IdempotencyKey:  idempotencyKey,
			Source:          source,
			ExternalEventID: externalEventID,
			Category:        category,
			Result:          resultValue,
			Actor:           actor,
			Summary:         summary,
			EvidenceJSON:    evidenceJSON,
			PayloadJSON:     payloadJSON,
			IngestedAt:      ingestedAt,
		})
		if createErr != nil {
			return createErr
		}
		if !inserted {
			out.Duplicate = true
			out.CommentWritten = false
			out.RoutedRole = "none"
			return nil
		}

		routedRole := "integrator"
		if writeback.IsFailure {
			routedRole = nextRoleForReviewChanges(labels)
		}
		out.RoutedRole = routedRole

		changes := WorkResultChanges{
			PR:     "none",
			Commit: "none",
		}
		tests := WorkResultTests{
			Command:  writeback.TestsCommand,
			Result:   writeback.TestsResult,
			Evidence: firstNonEmpty(strings.Join(evidence, ", "), "none"),
		}
		commentBody := buildStructuredComment(StructuredCommentInput{
			Role:       actor,
			IssueRef:   issueRef,
			RunID:      "none",
			Action:     writeback.Action,
			Status:     writeback.Status,
			ResultCode: writeback.ResultCode,
			ReadUpTo:   "none",
			Trigger:    "quality:" + category + ":" + idempotencyKey,
			Summary:    writeback.Marker + "; " + writeback.Summary,
			Changes:    changes,
			Tests:      tests,
			BlockedBy:  writeback.BlockedBy,
			Next:       writeback.NextPrefix + routedRole + writeback.NextSuffix,
		})
		if err := appendEventTx(txCtx, s.repo, issueID, actor, commentBody, ingestedAt); err != nil {
			return err
		}
		if err := s.repo.UpdateIssueUpdatedAt(txCtx, issueID, ingestedAt); err != nil {
			return err
		}

		out.CommentWritten = true
		return nil
	}); err != nil {
		return IngestQualityEventResult{}, err
	}

	return out, nil
}

func mergeInputWithInferredPayloadFields(input IngestQualityEventInput, source string) IngestQualityEventInput {
	if strings.TrimSpace(input.Payload) == "" {
		return input
	}

	needInference := strings.TrimSpace(input.Category) == "" ||
		strings.TrimSpace(input.Result) == "" ||
		strings.TrimSpace(input.ExternalEventID) == "" ||
		strings.TrimSpace(input.Actor) == "" ||
		strings.TrimSpace(input.Summary) == "" ||
		len(normalizeEvidenceLinks(input.Evidence)) == 0
	if !needInference {
		return input
	}

	fields, err := inferQualityFieldsFromPayload(source, input.Payload)
	if err != nil {
		return input
	}

	if strings.TrimSpace(input.Category) == "" {
		input.Category = fields.Category
	}
	if strings.TrimSpace(input.Result) == "" {
		input.Result = fields.Result
	}
	if strings.TrimSpace(input.ExternalEventID) == "" {
		input.ExternalEventID = fields.ExternalEventID
	}
	if strings.TrimSpace(input.Actor) == "" {
		input.Actor = fields.Actor
	}
	if strings.TrimSpace(input.Summary) == "" {
		input.Summary = fields.Summary
	}
	if len(normalizeEvidenceLinks(input.Evidence)) == 0 {
		input.Evidence = fields.Evidence
	}
	return input
}

func (s *Service) ListQualityEvents(ctx context.Context, issueRef string, limit int) ([]QualityEventItem, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(err, "check context")
	}
	if s.repo == nil {
		return nil, errors.New("outbox repository is required")
	}

	issueID, err := parseIssueRef(issueRef)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.repo.ListQualityEvents(ctx, issueID, limit)
	if err != nil {
		return nil, err
	}

	items := make([]QualityEventItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, QualityEventItem{
			QualityEventID:  row.QualityEventID,
			IdempotencyKey:  row.IdempotencyKey,
			Source:          row.Source,
			ExternalEventID: row.ExternalEventID,
			Category:        row.Category,
			Result:          row.Result,
			Actor:           row.Actor,
			Summary:         row.Summary,
			Evidence:        parseEvidenceJSON(row.EvidenceJSON),
			PayloadJSON:     row.PayloadJSON,
			IngestedAt:      row.IngestedAt,
		})
	}
	return items, nil
}

type qualityWriteback struct {
	Marker       string
	Summary      string
	Action       string
	Status       string
	ResultCode   string
	BlockedBy    []string
	TestsCommand string
	TestsResult  string
	NextPrefix   string
	NextSuffix   string
	IsFailure    bool
}

func resolveQualityWriteback(category string, resultValue string, summary string) (qualityWriteback, error) {
	switch category {
	case qualityCategoryReview:
		switch resultValue {
		case qualityResultApproved:
			return qualityWriteback{
				Marker:       "review:approved",
				Summary:      summary,
				Action:       "update",
				Status:       "review",
				ResultCode:   "none",
				BlockedBy:    []string{"none"},
				TestsCommand: "review verdict",
				TestsResult:  "pass",
				NextPrefix:   "@",
				NextSuffix:   " review quality gate signals",
			}, nil
		case qualityResultChangesRequested:
			return qualityWriteback{
				Marker:       "review:changes_requested",
				Summary:      summary,
				Action:       "blocked",
				Status:       "blocked",
				ResultCode:   "review_changes_requested",
				BlockedBy:    []string{"review-changes-requested"},
				TestsCommand: "review verdict",
				TestsResult:  "fail",
				NextPrefix:   "@",
				NextSuffix:   " address quality failure and rerun",
				IsFailure:    true,
			}, nil
		}
	case qualityCategoryCI:
		switch resultValue {
		case qualityResultPass:
			return qualityWriteback{
				Marker:       "qa:pass",
				Summary:      summary,
				Action:       "update",
				Status:       "review",
				ResultCode:   "none",
				BlockedBy:    []string{"none"},
				TestsCommand: "ci checks",
				TestsResult:  "pass",
				NextPrefix:   "@",
				NextSuffix:   " review quality gate signals",
			}, nil
		case qualityResultFail:
			return qualityWriteback{
				Marker:       "qa:fail",
				Summary:      summary,
				Action:       "blocked",
				Status:       "blocked",
				ResultCode:   "ci_failed",
				BlockedBy:    []string{"ci-failed"},
				TestsCommand: "ci checks",
				TestsResult:  "fail",
				NextPrefix:   "@",
				NextSuffix:   " address quality failure and rerun",
				IsFailure:    true,
			}, nil
		}
	}
	return qualityWriteback{}, errors.New("unsupported quality event mapping")
}

func normalizeQualityCategory(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case qualityCategoryReview, qualityCategoryCI:
		return normalized, nil
	case "":
		return "", errors.New("quality event category is required")
	default:
		return "", fmt.Errorf("unsupported quality category %q", value)
	}
}

func normalizeQualityResult(category string, value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch category {
	case qualityCategoryReview:
		if normalized == qualityResultApproved || normalized == qualityResultChangesRequested {
			return normalized, nil
		}
	case qualityCategoryCI:
		if normalized == qualityResultPass || normalized == qualityResultFail {
			return normalized, nil
		}
	}
	if normalized == "" {
		return "", errors.New("quality event result is required")
	}
	return "", fmt.Errorf("unsupported quality result %q for category %s", value, category)
}

func normalizeEvidenceLinks(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func isQualityFailure(category string, resultValue string) bool {
	return (category == qualityCategoryReview && resultValue == qualityResultChangesRequested) ||
		(category == qualityCategoryCI && resultValue == qualityResultFail)
}

func defaultQualitySummary(category string, resultValue string) string {
	switch category {
	case qualityCategoryReview:
		if resultValue == qualityResultApproved {
			return "review approved"
		}
		return "review changes requested"
	case qualityCategoryCI:
		if resultValue == qualityResultPass {
			return "ci checks passed"
		}
		return "ci checks failed"
	default:
		return "quality event ingested"
	}
}

func deriveQualityEventKey(issueRef string, source string, externalEventID string, category string, resultValue string, actor string, summary string, evidence []string, payloadJSON string) string {
	parts := []string{
		strings.TrimSpace(issueRef),
		strings.TrimSpace(source),
		strings.TrimSpace(externalEventID),
		strings.TrimSpace(category),
		strings.TrimSpace(resultValue),
		strings.TrimSpace(actor),
		strings.TrimSpace(summary),
		strings.Join(evidence, ","),
		strings.TrimSpace(payloadJSON),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "qevt:" + hex.EncodeToString(sum[:])
}

func marshalDefaultQualityPayload(issueRef string, source string, externalEventID string, category string, resultValue string, actor string, summary string, evidence []string) (string, error) {
	payload := map[string]any{
		"issue_ref":         issueRef,
		"source":            source,
		"external_event_id": externalEventID,
		"category":          category,
		"result":            resultValue,
		"actor":             actor,
		"summary":           summary,
		"evidence":          evidence,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func parseEvidenceJSON(raw string) []string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	var items []string
	if err := json.Unmarshal([]byte(value), &items); err != nil {
		return nil
	}
	return normalizeEvidenceLinks(items)
}
