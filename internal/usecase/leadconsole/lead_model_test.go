package leadconsole

import (
	"context"
	"testing"

	"zhanggui/internal/usecase/outbox"
)

func TestRoleMatchesIssue(t *testing.T) {
	testCases := []struct {
		name     string
		role     string
		assignee string
		issue    outbox.IssueListItem
		want     bool
	}{
		{
			name:     "assignee match",
			role:     "backend",
			assignee: "lead-backend",
			issue: outbox.IssueListItem{
				Assignee: "lead-backend",
				Labels:   []string{"to:other"},
			},
			want: true,
		},
		{
			name:     "backend label match",
			role:     "backend",
			assignee: "lead-backend",
			issue: outbox.IssueListItem{
				Assignee: "someone",
				Labels:   []string{"to:backend"},
			},
			want: true,
		},
		{
			name:     "reviewer state review match",
			role:     "reviewer",
			assignee: "lead-reviewer",
			issue: outbox.IssueListItem{
				Assignee: "someone",
				Labels:   []string{"state:review"},
			},
			want: true,
		},
		{
			name:     "no match",
			role:     "backend",
			assignee: "lead-backend",
			issue: outbox.IssueListItem{
				Assignee: "other",
				Labels:   []string{"to:frontend", "state:todo"},
			},
			want: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := roleMatchesIssue(testCase.role, testCase.assignee, testCase.issue)
			if got != testCase.want {
				t.Fatalf("roleMatchesIssue() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestFilterIssuesSortAndState(t *testing.T) {
	items := []outbox.IssueListItem{
		{IssueRef: "local#2", UpdatedAt: "2026-02-14T10:00:00Z", Labels: []string{"to:reviewer", "state:blocked"}},
		{IssueRef: "local#1", UpdatedAt: "2026-02-14T11:00:00Z", Labels: []string{"state:review"}},
		{IssueRef: "local#3", UpdatedAt: "2026-02-14T09:00:00Z", Labels: []string{"to:backend", "state:review"}},
	}

	filtered := filterIssues(items, "reviewer", "lead-reviewer", "state:review")
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if filtered[0].IssueRef != "local#1" {
		t.Fatalf("filtered[0].IssueRef = %q, want local#1", filtered[0].IssueRef)
	}
	if filtered[1].IssueRef != "local#3" {
		t.Fatalf("filtered[1].IssueRef = %q, want local#3", filtered[1].IssueRef)
	}

	all := filterIssues(items, "reviewer", "lead-reviewer", "")
	if len(all) != 3 {
		t.Fatalf("len(all) = %d, want 3", len(all))
	}
	if all[0].IssueRef != "local#1" || all[1].IssueRef != "local#2" || all[2].IssueRef != "local#3" {
		t.Fatalf("sorted refs = %q, %q, %q", all[0].IssueRef, all[1].IssueRef, all[2].IssueRef)
	}
}

func TestNormalizeStateFilter(t *testing.T) {
	testCases := []struct {
		input string
		want  string
	}{
		{input: "", want: ""},
		{input: "review", want: "state:review"},
		{input: "state:done", want: "state:done"},
		{input: "blocked", want: "state:blocked"},
	}

	for _, testCase := range testCases {
		got := normalizeStateFilter(testCase.input)
		if got != testCase.want {
			t.Fatalf("normalizeStateFilter(%q) = %q, want %q", testCase.input, got, testCase.want)
		}
	}
}

func TestTrimStatePrefix(t *testing.T) {
	if got := trimStatePrefix("state:review"); got != "review" {
		t.Fatalf("trimStatePrefix(state:review) = %q, want review", got)
	}
	if got := trimStatePrefix("doing"); got != "doing" {
		t.Fatalf("trimStatePrefix(doing) = %q, want doing", got)
	}
}

func TestShouldBlockAutoActionNeedsHuman(t *testing.T) {
	model := &leadModel{
		ctx:       context.Background(),
		hasDetail: true,
		detail: outbox.IssueDetail{
			Labels: []string{"to:backend", "state:blocked", "needs-human"},
		},
	}

	if !model.shouldBlockAutoAction("spawn") {
		t.Fatalf("shouldBlockAutoAction(spawn) = false, want true")
	}
	if !model.shouldBlockAutoAction("close") {
		t.Fatalf("shouldBlockAutoAction(close) = false, want true")
	}
	if model.shouldBlockAutoAction("claim") {
		t.Fatalf("shouldBlockAutoAction(claim) = true, want false")
	}
}

func TestIssueDetailLoadedIgnoresStaleSelection(t *testing.T) {
	model := &leadModel{
		ctx: context.Background(),
		issues: []outbox.IssueListItem{
			{IssueRef: "local#1"},
			{IssueRef: "local#2"},
		},
		selectedIndex: 1,
	}

	nextModel, _ := model.Update(issueDetailLoadedMsg{
		issueRef:       "local#1",
		hasDetail:      true,
		detail:         outbox.IssueDetail{IssueRef: "local#1", Labels: []string{"state:blocked", "needs-human"}},
		activeRunID:    "2026-02-15-backend-0001",
		activeRunFound: true,
	})

	updated, ok := nextModel.(*leadModel)
	if !ok {
		t.Fatalf("type assertion failed: %T", nextModel)
	}
	if updated.hasDetail {
		t.Fatalf("stale detail should be ignored")
	}
	if updated.activeRunFound {
		t.Fatalf("stale active run should be ignored")
	}
}

func TestIssueDetailLoadedAppliesCurrentSelection(t *testing.T) {
	model := &leadModel{
		ctx: context.Background(),
		issues: []outbox.IssueListItem{
			{IssueRef: "local#1"},
			{IssueRef: "local#2"},
		},
		selectedIndex: 1,
	}

	nextModel, _ := model.Update(issueDetailLoadedMsg{
		issueRef:       "local#2",
		hasDetail:      true,
		detail:         outbox.IssueDetail{IssueRef: "local#2", Labels: []string{"state:doing"}},
		activeRunID:    "2026-02-15-backend-0002",
		activeRunFound: true,
	})

	updated, ok := nextModel.(*leadModel)
	if !ok {
		t.Fatalf("type assertion failed: %T", nextModel)
	}
	if !updated.hasDetail {
		t.Fatalf("current detail should be applied")
	}
	if updated.detail.IssueRef != "local#2" {
		t.Fatalf("detail issue_ref = %q, want local#2", updated.detail.IssueRef)
	}
	if !updated.activeRunFound || updated.activeRunID != "2026-02-15-backend-0002" {
		t.Fatalf("active run = (%v,%q), want (true,2026-02-15-backend-0002)", updated.activeRunFound, updated.activeRunID)
	}
}
