package pmconsole

import (
	"context"
	"strings"
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

func TestFilterIssuesAllDoesNotFilterByRole(t *testing.T) {
	items := []outbox.IssueListItem{
		{IssueRef: "local#1", UpdatedAt: "2026-02-14T11:00:00Z", Labels: []string{"to:backend", "state:todo"}},
		{IssueRef: "local#2", UpdatedAt: "2026-02-14T10:00:00Z", Labels: []string{"to:frontend", "state:todo"}},
		{IssueRef: "local#3", UpdatedAt: "2026-02-14T09:00:00Z", Labels: []string{"state:review"}},
	}

	filtered := filterIssues(items, "all", "", "state:todo")
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if filtered[0].IssueRef != "local#1" || filtered[1].IssueRef != "local#2" {
		t.Fatalf("filtered refs = %q, %q", filtered[0].IssueRef, filtered[1].IssueRef)
	}

	withAssignee := filterIssues([]outbox.IssueListItem{
		{IssueRef: "local#1", UpdatedAt: "2026-02-14T11:00:00Z", Assignee: "lead-backend"},
		{IssueRef: "local#2", UpdatedAt: "2026-02-14T10:00:00Z", Assignee: "lead-frontend"},
	}, "all", "lead-backend", "")
	if len(withAssignee) != 1 || withAssignee[0].IssueRef != "local#1" {
		t.Fatalf("assignee filter mismatch: %+v", withAssignee)
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

func TestResolveRouteAssigneePreferred(t *testing.T) {
	model := &pmModel{
		enabledRoleSet: map[string]struct{}{
			"backend":    {},
			"frontend":   {},
			"reviewer":   {},
			"integrator": {},
		},
	}

	route := model.resolveRoute("local#1", "lead-backend", []string{"to:frontend", "state:review"})
	if route.Source != "assignee" || route.Role != "backend" || route.Assignee != "lead-backend" {
		t.Fatalf("route = %+v", route)
	}
}

func TestResolveRouteToLabelAmbiguous(t *testing.T) {
	model := &pmModel{
		enabledRoleSet: map[string]struct{}{
			"backend":  {},
			"frontend": {},
		},
	}

	route := model.resolveRoute("local#2", "", []string{"to:backend", "to:frontend", "state:todo"})
	if route.Source != "ambiguous" {
		t.Fatalf("route.Source = %q, want ambiguous", route.Source)
	}
	if route.Err == nil || !strings.Contains(route.Err.Error(), "multiple to:* labels") {
		t.Fatalf("route.Err = %v, want multiple to labels error", route.Err)
	}
}

func TestResolveRouteStateReviewToReviewer(t *testing.T) {
	model := &pmModel{
		enabledRoleSet: map[string]struct{}{
			"reviewer": {},
		},
	}

	route := model.resolveRoute("local#3", "", []string{"state:review"})
	if route.Role != "reviewer" || route.Assignee != "lead-reviewer" || route.Source != "state-review" {
		t.Fatalf("route = %+v", route)
	}
}

func TestCheckActionAllowedNeedsHumanBlocksAutoAdvance(t *testing.T) {
	model := &pmModel{
		enabledRoleSet: map[string]struct{}{
			"backend": {},
		},
	}
	route := model.resolveRoute("local#4", "lead-backend", []string{"to:backend", "needs-human", "state:doing"})
	detail := outbox.IssueDetail{
		IssueRef: "local#4",
		Assignee: "lead-backend",
		Labels:   []string{"to:backend", "needs-human", "state:doing"},
	}

	if err := model.checkActionAllowed("spawn", detail, route); err == nil {
		t.Fatalf("checkActionAllowed(spawn) expected error")
	}
	if err := model.checkActionAllowed("reply", detail, route); err == nil {
		t.Fatalf("checkActionAllowed(reply) expected error")
	}
	if err := model.checkActionAllowed("close", detail, route); err == nil {
		t.Fatalf("checkActionAllowed(close) expected error")
	}
	if err := model.checkActionAllowed("unclaim", detail, route); err != nil {
		t.Fatalf("checkActionAllowed(unclaim) error = %v, want nil", err)
	}
}

func TestIssueDetailLoadedIgnoresStaleSelection(t *testing.T) {
	model := &pmModel{
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

	updated, ok := nextModel.(*pmModel)
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
	model := &pmModel{
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

	updated, ok := nextModel.(*pmModel)
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

func TestViewDetailIncludesQualitySection(t *testing.T) {
	model := &pmModel{
		ctx:       context.Background(),
		hasDetail: true,
		detail:    outbox.IssueDetail{IssueRef: "local#10", Assignee: "lead-backend", Labels: []string{"to:backend", "state:doing"}},
		qualityEvents: []outbox.QualityEventItem{
			{Category: "review", Result: "approved", Actor: "alice", IngestedAt: "2026-02-16T10:00:00Z"},
			{Category: "ci", Result: "pass", Actor: "quality-bot", IngestedAt: "2026-02-16T09:00:00Z"},
			{Category: "review", Result: "changes_requested", Actor: "bob", IngestedAt: "2026-02-16T08:00:00Z"},
			{Category: "ci", Result: "fail", Actor: "quality-bot", IngestedAt: "2026-02-16T07:00:00Z"},
		},
	}

	view := model.View()
	if !strings.Contains(view, "Quality:") {
		t.Fatalf("view missing Quality section: %s", view)
	}
	if !strings.Contains(view, "- review/approved actor=alice time=2026-02-16T10:00:00Z") {
		t.Fatalf("view missing newest quality event: %s", view)
	}
	if !strings.Contains(view, "- review/changes_requested actor=bob time=2026-02-16T08:00:00Z") {
		t.Fatalf("view missing third quality event: %s", view)
	}
	if strings.Contains(view, "- ci/fail actor=quality-bot time=2026-02-16T07:00:00Z") {
		t.Fatalf("view should only render latest 3 quality events: %s", view)
	}
}

func TestIssueDetailLoadedQualityUnavailableDoesNotBreakDetail(t *testing.T) {
	model := &pmModel{
		ctx: context.Background(),
		issues: []outbox.IssueListItem{
			{IssueRef: "local#20"},
		},
		selectedIndex: 0,
	}

	nextModel, _ := model.Update(issueDetailLoadedMsg{
		issueRef:           "local#20",
		hasDetail:          true,
		detail:             outbox.IssueDetail{IssueRef: "local#20", Assignee: "lead-backend", Labels: []string{"to:backend", "state:doing"}},
		qualityUnavailable: true,
		activeRunID:        "2026-02-16-backend-0009",
		activeRunFound:     true,
	})

	updated, ok := nextModel.(*pmModel)
	if !ok {
		t.Fatalf("type assertion failed: %T", nextModel)
	}
	if !updated.hasDetail {
		t.Fatalf("detail should still be available when quality query fails")
	}
	if !updated.qualityUnavailable {
		t.Fatalf("qualityUnavailable = false, want true")
	}

	view := updated.View()
	if !strings.Contains(view, "Quality: unavailable") {
		t.Fatalf("view should show quality unavailable state: %s", view)
	}
}
