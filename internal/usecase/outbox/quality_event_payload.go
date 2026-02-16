package outbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type qualityPayloadFields struct {
	Category        string
	Result          string
	ExternalEventID string
	Actor           string
	Summary         string
	Evidence        []string
}

func inferQualityFieldsFromPayload(source string, payload string) (qualityPayloadFields, error) {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return qualityPayloadFields{}, errors.New("payload is required for inference")
	}

	var root map[string]any
	if err := json.Unmarshal([]byte(trimmed), &root); err != nil {
		return qualityPayloadFields{}, fmt.Errorf("parse payload json: %w", err)
	}

	normalizedSource := strings.ToLower(strings.TrimSpace(source))
	switch normalizedSource {
	case "github":
		return inferGitHubQualityFields(root)
	case "gitlab":
		return inferGitLabQualityFields(root)
	default:
		return inferGenericQualityFields(root)
	}
}

func inferGitHubQualityFields(root map[string]any) (qualityPayloadFields, error) {
	if review := mapField(root, "review"); review != nil {
		state := strings.ToLower(strings.TrimSpace(stringField(review, "state")))
		result := ""
		switch state {
		case "approved":
			result = qualityResultApproved
		case "changes_requested":
			result = qualityResultChangesRequested
		default:
			return qualityPayloadFields{}, fmt.Errorf("unsupported github review.state %q", state)
		}

		reviewID := firstNonEmpty(
			stringField(review, "id"),
			stringField(root, "delivery"),
		)
		if strings.TrimSpace(reviewID) == "" {
			reviewID = "unknown"
		}
		summary := firstNonEmpty(stringField(review, "body"), "github review "+state)
		actor := firstNonEmpty(
			stringField(mapField(review, "user"), "login"),
			stringField(mapField(root, "sender"), "login"),
			"github-bot",
		)
		evidence := normalizeEvidenceLinks([]string{
			stringField(review, "html_url"),
			stringField(mapField(root, "pull_request"), "html_url"),
		})

		return qualityPayloadFields{
			Category:        qualityCategoryReview,
			Result:          result,
			ExternalEventID: "github:review:" + strings.TrimSpace(reviewID),
			Actor:           actor,
			Summary:         summary,
			Evidence:        evidence,
		}, nil
	}

	if checkRun := mapField(root, "check_run"); checkRun != nil {
		result, err := mapGitHubCheckConclusionToResult(firstNonEmpty(
			stringField(checkRun, "conclusion"),
			stringField(checkRun, "status"),
		))
		if err != nil {
			return qualityPayloadFields{}, err
		}

		checkID := firstNonEmpty(stringField(checkRun, "id"), "unknown")
		summary := firstNonEmpty(
			stringField(checkRun, "name"),
			"github check_run "+firstNonEmpty(stringField(checkRun, "conclusion"), stringField(checkRun, "status"), "unknown"),
		)
		actor := firstNonEmpty(
			stringField(mapField(mapField(checkRun, "app"), "owner"), "login"),
			stringField(mapField(root, "sender"), "login"),
			"github-bot",
		)
		evidence := normalizeEvidenceLinks([]string{
			stringField(checkRun, "html_url"),
			stringField(checkRun, "details_url"),
		})

		return qualityPayloadFields{
			Category:        qualityCategoryCI,
			Result:          result,
			ExternalEventID: "github:check_run:" + strings.TrimSpace(checkID),
			Actor:           actor,
			Summary:         summary,
			Evidence:        evidence,
		}, nil
	}

	if checkSuite := mapField(root, "check_suite"); checkSuite != nil {
		result, err := mapGitHubCheckConclusionToResult(firstNonEmpty(
			stringField(checkSuite, "conclusion"),
			stringField(checkSuite, "status"),
		))
		if err != nil {
			return qualityPayloadFields{}, err
		}
		suiteID := firstNonEmpty(stringField(checkSuite, "id"), "unknown")
		summary := firstNonEmpty(
			stringField(checkSuite, "head_branch"),
			"github check_suite "+firstNonEmpty(stringField(checkSuite, "conclusion"), stringField(checkSuite, "status"), "unknown"),
		)
		actor := firstNonEmpty(stringField(mapField(root, "sender"), "login"), "github-bot")
		evidence := normalizeEvidenceLinks([]string{
			stringField(checkSuite, "url"),
		})

		return qualityPayloadFields{
			Category:        qualityCategoryCI,
			Result:          result,
			ExternalEventID: "github:check_suite:" + strings.TrimSpace(suiteID),
			Actor:           actor,
			Summary:         summary,
			Evidence:        evidence,
		}, nil
	}

	return qualityPayloadFields{}, errors.New("unsupported github payload kind")
}

func inferGitLabQualityFields(root map[string]any) (qualityPayloadFields, error) {
	objectKind := strings.ToLower(strings.TrimSpace(stringField(root, "object_kind")))
	switch objectKind {
	case "pipeline":
		attrs := mapField(root, "object_attributes")
		if attrs == nil {
			return qualityPayloadFields{}, errors.New("gitlab pipeline payload missing object_attributes")
		}
		status := strings.ToLower(strings.TrimSpace(stringField(attrs, "status")))
		result := ""
		switch status {
		case "success":
			result = qualityResultPass
		case "failed", "canceled", "cancelled", "manual", "skipped":
			result = qualityResultFail
		default:
			return qualityPayloadFields{}, fmt.Errorf("unsupported gitlab pipeline status %q", status)
		}

		pipelineID := firstNonEmpty(stringField(attrs, "id"), "unknown")
		actor := firstNonEmpty(
			stringField(mapField(root, "user"), "username"),
			"gitlab-bot",
		)
		summary := firstNonEmpty(
			"gitlab pipeline "+status,
			stringField(attrs, "ref"),
		)
		evidence := normalizeEvidenceLinks([]string{
			stringField(attrs, "url"),
			stringField(attrs, "web_url"),
		})

		return qualityPayloadFields{
			Category:        qualityCategoryCI,
			Result:          result,
			ExternalEventID: "gitlab:pipeline:" + strings.TrimSpace(pipelineID),
			Actor:           actor,
			Summary:         summary,
			Evidence:        evidence,
		}, nil
	default:
		return qualityPayloadFields{}, fmt.Errorf("unsupported gitlab object_kind %q", objectKind)
	}
}

func inferGenericQualityFields(root map[string]any) (qualityPayloadFields, error) {
	category := strings.ToLower(strings.TrimSpace(stringField(root, "category")))
	resultValue := strings.ToLower(strings.TrimSpace(stringField(root, "result")))
	if category == "" || resultValue == "" {
		return qualityPayloadFields{}, errors.New("payload missing category/result for generic inference")
	}

	fields := qualityPayloadFields{
		Category:        category,
		Result:          resultValue,
		ExternalEventID: firstNonEmpty(stringField(root, "external_event_id"), "none"),
		Actor:           firstNonEmpty(stringField(root, "actor"), "quality-bot"),
		Summary:         strings.TrimSpace(stringField(root, "summary")),
		Evidence:        normalizeEvidenceLinks(stringSliceField(root, "evidence")),
	}
	if strings.TrimSpace(fields.Summary) == "" {
		fields.Summary = defaultQualitySummary(category, resultValue)
	}
	return fields, nil
}

func mapGitHubCheckConclusionToResult(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "success", "completed_success":
		return qualityResultPass, nil
	case "failure", "timed_out", "cancelled", "canceled", "action_required", "startup_failure", "stale", "neutral", "skipped", "completed":
		return qualityResultFail, nil
	default:
		if normalized == "" {
			return "", errors.New("github check payload missing conclusion/status")
		}
		return "", fmt.Errorf("unsupported github check conclusion/status %q", normalized)
	}
}

func mapField(root map[string]any, key string) map[string]any {
	if root == nil {
		return nil
	}
	raw, ok := root[key]
	if !ok || raw == nil {
		return nil
	}
	out, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return out
}

func stringField(root map[string]any, key string) string {
	if root == nil {
		return ""
	}
	raw, ok := root[key]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case uint64:
		return fmt.Sprintf("%d", v)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", raw))
	}
}

func stringSliceField(root map[string]any, key string) []string {
	if root == nil {
		return nil
	}
	raw, ok := root[key]
	if !ok || raw == nil {
		return nil
	}
	array, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(array))
	for _, item := range array {
		switch value := item.(type) {
		case string:
			out = append(out, strings.TrimSpace(value))
		default:
			out = append(out, strings.TrimSpace(fmt.Sprintf("%v", value)))
		}
	}
	return normalizeEvidenceLinks(out)
}
