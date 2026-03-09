package storesqlite

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

var _ core.Memory = (*SQLiteMemory)(nil)

// SQLiteMemory implements layered prompt memory on top of SQLiteStore.
type SQLiteMemory struct {
	store *SQLiteStore
}

// NewSQLiteMemory creates a memory adapter backed by SQLiteStore.
func NewSQLiteMemory(store *SQLiteStore) *SQLiteMemory {
	return &SQLiteMemory{store: store}
}

func (m *SQLiteMemory) RecallCold(issueID string) (string, error) {
	issue, err := m.getIssue(issueID)
	if err != nil || issue == nil {
		return "", err
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("标题: %s", issue.Title))
	if body := truncateRunes(strings.TrimSpace(issue.Body), 500); body != "" {
		parts = append(parts, fmt.Sprintf("描述: %s", body))
	}

	return "## 任务背景\n" + strings.Join(parts, "\n"), nil
}

func (m *SQLiteMemory) RecallWarm(issueID string) (string, error) {
	issue, err := m.getIssue(issueID)
	if err != nil || issue == nil {
		return "", err
	}

	parentID := strings.TrimSpace(issue.ParentID)
	if parentID == "" {
		return "", nil
	}

	parent, err := m.getIssue(parentID)
	if err != nil || parent == nil {
		return "", err
	}

	var sections []string
	parentLines := []string{fmt.Sprintf("标题: %s", parent.Title)}
	if body := truncateRunes(strings.TrimSpace(parent.Body), 300); body != "" {
		parentLines = append(parentLines, fmt.Sprintf("描述: %s", body))
	}
	sections = append(sections, "## 父任务\n"+strings.Join(parentLines, "\n"))

	siblings, err := m.store.GetChildIssues(parentID)
	if err != nil {
		return "", err
	}

	lines := make([]string, 0, len(siblings))
	for _, sibling := range siblings {
		if sibling.ID == issue.ID {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s [状态: %s]", sibling.Title, sibling.Status))
		if len(lines) >= 10 {
			break
		}
	}
	if len(lines) > 0 {
		sections = append(sections, "## 兄弟任务\n"+strings.Join(lines, "\n"))
	}

	return strings.Join(sections, "\n\n"), nil
}

func (m *SQLiteMemory) RecallHot(issueID string, runID string) (string, error) {
	if m == nil || m.store == nil {
		return "", nil
	}

	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return "", nil
	}

	var sections []string

	steps, err := m.store.ListTaskSteps(issueID)
	if err != nil {
		return "", err
	}
	if len(steps) > 0 {
		start := max(0, len(steps)-20)
		lines := make([]string, 0, len(steps[start:]))
		for _, step := range steps[start:] {
			line := fmt.Sprintf("- %s %s", step.CreatedAt.UTC().Format(timeOnlyLayout), step.Action)
			if note := truncateRunes(strings.TrimSpace(step.Note), 120); note != "" {
				line += ": " + note
			}
			lines = append(lines, line)
		}
		sections = append(sections, "## 最近事件\n"+strings.Join(lines, "\n"))
	}

	runID = strings.TrimSpace(runID)
	if runID != "" {
		events, err := m.store.ListRunEvents(runID)
		if err != nil {
			return "", err
		}
		lines := summarizeRunEvents(events)
		if len(lines) > 0 {
			sections = append(sections, "## 最近执行\n"+strings.Join(lines, "\n"))
		}
	}

	reviews, err := m.store.GetReviewRecords(issueID)
	if err != nil {
		return "", err
	}
	if len(reviews) > 0 {
		lines := make([]string, 0, len(reviews))
		for _, review := range reviews {
			line := fmt.Sprintf("- 第%d轮 %s: %s", review.Round, review.Reviewer, review.Verdict)
			if summary := truncateRunes(strings.TrimSpace(review.Summary), 200); summary != "" {
				line += " - " + summary
			}
			lines = append(lines, line)
		}
		sections = append(sections, "## 审查记录\n"+strings.Join(lines, "\n"))
	}

	if len(sections) == 0 {
		return "", nil
	}
	return strings.Join(sections, "\n\n"), nil
}

const timeOnlyLayout = "15:04:05"

func (m *SQLiteMemory) getIssue(issueID string) (*core.Issue, error) {
	if m == nil || m.store == nil {
		return nil, nil
	}

	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return nil, nil
	}

	issue, err := m.store.GetIssue(issueID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}
	return issue, nil
}

func summarizeRunEvents(events []core.RunEvent) []string {
	if len(events) == 0 {
		return nil
	}

	filtered := make([]core.RunEvent, 0, len(events))
	for _, event := range events {
		if isRelevantRunEvent(event.EventType) {
			filtered = append(filtered, event)
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	start := max(0, len(filtered)-5)
	lines := make([]string, 0, len(filtered[start:]))
	for _, event := range filtered[start:] {
		line := fmt.Sprintf("- %s", event.EventType)
		if stage := strings.TrimSpace(event.Stage); stage != "" {
			line += fmt.Sprintf(" [%s]", stage)
		}
		if payload := stringifyRunEventData(event.Data); payload != "" {
			line += ": " + truncateRunes(payload, 200)
		} else if event.Error != "" {
			line += ": " + truncateRunes(strings.TrimSpace(event.Error), 200)
		}
		lines = append(lines, line)
	}
	return lines
}

func isRelevantRunEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case string(core.EventAgentOutput),
		string(core.EventRunStarted),
		string(core.EventRunUpdate),
		string(core.EventRunCompleted),
		string(core.EventRunFailed),
		string(core.EventStageFailed):
		return true
	default:
		return false
	}
}

func stringifyRunEventData(data map[string]string) string {
	if len(data) == 0 {
		return ""
	}

	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(data[key])
		if value == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
	}
	return strings.Join(parts, ", ")
}

func truncateRunes(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
