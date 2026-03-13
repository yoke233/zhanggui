package flow

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// manifestCheckEnabled returns true if the gate action has manifest_check: true.
func manifestCheckEnabled(action *core.Action) bool {
	if action.Config == nil {
		return false
	}
	v, ok := action.Config["manifest_check"].(bool)
	return ok && v
}

// checkManifestEntries evaluates the feature manifest for the gate action's work item/project.
// Returns (passed, reason, error).
func (e *WorkItemEngine) checkManifestEntries(ctx context.Context, action *core.Action) (bool, string, error) {
	workItem, err := e.store.GetWorkItem(ctx, action.WorkItemID)
	if err != nil || workItem == nil || workItem.ProjectID == nil {
		return true, "", nil // no project → skip check
	}

	manifest, err := e.store.GetFeatureManifestByProject(ctx, *workItem.ProjectID)
	if err != nil {
		return true, "", nil // no manifest → skip check
	}

	// Determine which entries to check.
	filter := core.FeatureEntryFilter{ManifestID: manifest.ID, Limit: 500}

	// If manifest_issue_id is configured, check only entries linked to that work item.
	if workItemID, ok := action.Config["manifest_issue_id"].(float64); ok {
		id := int64(workItemID)
		filter.WorkItemID = &id
	}
	// If manifest_required_tags is configured, filter entries by tags.
	if rawTags, ok := action.Config["manifest_required_tags"].([]any); ok {
		for _, t := range rawTags {
			if tag, ok := t.(string); ok {
				filter.Tags = append(filter.Tags, tag)
			}
		}
	}

	entries, err := e.store.ListFeatureEntries(ctx, filter)
	if err != nil {
		return true, "", err
	}
	if len(entries) == 0 {
		return true, "", nil
	}

	// Count by status.
	failCount := 0
	pendingCount := 0
	passCount := 0
	for _, entry := range entries {
		switch entry.Status {
		case core.FeatureFail:
			failCount++
		case core.FeaturePending:
			pendingCount++
		case core.FeaturePass:
			passCount++
		}
	}

	maxFail := 0
	if v, ok := action.Config["manifest_max_fail"].(float64); ok {
		maxFail = int(v)
	}
	maxPending := len(entries) // default: allow all pending
	if v, ok := action.Config["manifest_max_pending"].(float64); ok {
		maxPending = int(v)
	}

	// Publish gate-checked event.
	e.bus.Publish(ctx, core.Event{
		Type:       core.EventManifestGateChecked,
		WorkItemID: action.WorkItemID,
		ActionID:   action.ID,
		Timestamp:  time.Now().UTC(),
		Data: map[string]any{
			"passed":        failCount <= maxFail && pendingCount <= maxPending,
			"total":         len(entries),
			"pass_count":    passCount,
			"fail_count":    failCount,
			"pending_count": pendingCount,
		},
	})

	if failCount > maxFail {
		return false, fmt.Sprintf("feature manifest: %d entries failed (max allowed: %d)", failCount, maxFail), nil
	}
	if pendingCount > maxPending {
		return false, fmt.Sprintf("feature manifest: %d entries still pending (max allowed: %d)", pendingCount, maxPending), nil
	}
	return true, "", nil
}
