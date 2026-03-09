package storesqlite

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

var _ core.Memory = (*SQLiteMemory)(nil)

// SQLiteMemory implements layered prompt memory on top of SQLiteStore.
type SQLiteMemory struct {
	store *SQLiteStore
}

const (
	maxHotTaskSteps = 20
	maxHotRunEvents = 5
	maxHotReviews   = 5
)

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

	issue, err := m.getIssue(issueID)
	if err != nil || issue == nil {
		return "", err
	}

	var sections []string
	runID = strings.TrimSpace(runID)

	steps, err := m.store.ListTaskSteps(issue.ID)
	if err != nil {
		return "", err
	}
	if lines := summarizeTaskSteps(steps, runID); len(lines) > 0 {
		sections = append(sections, "## 最近事件\n"+strings.Join(lines, "\n"))
	}

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

	reviews, err := m.store.GetReviewRecords(issue.ID)
	if err != nil {
		return "", err
	}
	if lines := summarizeReviewRecords(reviews); len(lines) > 0 {
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
		if errors.Is(err, errIssueNotFound) || strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}
	return issue, nil
}

func summarizeTaskSteps(steps []core.TaskStep, runID string) []string {
	if len(steps) == 0 {
		return nil
	}

	filtered := make([]core.TaskStep, 0, len(steps))
	for _, step := range steps {
		if runID != "" && strings.TrimSpace(step.RunID) != runID {
			continue
		}
		filtered = append(filtered, step)
	}
	if len(filtered) == 0 {
		return nil
	}

	start := max(0, len(filtered)-maxHotTaskSteps)
	lines := make([]string, 0, len(filtered[start:]))
	for _, step := range filtered[start:] {
		line := fmt.Sprintf("- %s %s", step.CreatedAt.UTC().Format(timeOnlyLayout), step.Action)
		if note := truncateRunes(strings.TrimSpace(step.Note), 120); note != "" {
			line += ": " + note
		}
		lines = append(lines, line)
	}
	return lines
}

func summarizeRunEvents(events []core.RunEvent) []string {
	if len(events) == 0 {
		return nil
	}

	filtered := make([]core.RunEvent, 0, len(events))
	for _, event := range events {
		if isRelevantRunEvent(event) {
			filtered = append(filtered, event)
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	start := max(0, len(filtered)-maxHotRunEvents)
	lines := make([]string, 0, len(filtered[start:]))
	for _, event := range filtered[start:] {
		line := fmt.Sprintf("- %s", event.EventType)
		if stage := strings.TrimSpace(event.Stage); stage != "" {
			line += fmt.Sprintf(" [%s]", stage)
		}
		if event.Error != "" {
			line += ": " + truncateRunes(strings.TrimSpace(event.Error), 200)
		}
		lines = append(lines, line)
	}
	return lines
}

func summarizeReviewRecords(reviews []core.ReviewRecord) []string {
	if len(reviews) == 0 {
		return nil
	}

	start := max(0, len(reviews)-maxHotReviews)
	lines := make([]string, 0, len(reviews[start:]))
	for _, review := range reviews[start:] {
		line := fmt.Sprintf("- 第%d轮 %s: %s", review.Round, review.Reviewer, review.Verdict)
		if summary := truncateRunes(strings.TrimSpace(review.Summary), 200); summary != "" {
			line += " - " + summary
		}
		lines = append(lines, line)
	}
	return lines
}

func isRelevantRunEvent(event core.RunEvent) bool {
	switch strings.TrimSpace(event.EventType) {
	case string(core.EventStageStart),
		string(core.EventStageComplete),
		string(core.EventStageFailed),
		string(core.EventHumanRequired),
		string(core.EventRunStarted),
		string(core.EventRunUpdate),
		string(core.EventRunCompleted),
		string(core.EventRunDone),
		string(core.EventRunFailed),
		string(core.EventIssueMergeConflict),
		string(core.EventMergeFailed):
		return true
	default:
		return false
	}
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
