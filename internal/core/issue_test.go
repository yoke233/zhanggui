package core

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestNewIssueID(t *testing.T) {
	id := NewIssueID()
	pat := regexp.MustCompile(`^issue-\d{8}-[0-9a-f]{8}$`)
	if !pat.MatchString(id) {
		t.Fatalf("invalid issue id: %s", id)
	}
}

func TestIssueValidate_TitleRequired(t *testing.T) {
	issue := Issue{
		Title:    "   ",
		Template: "default",
	}

	err := issue.Validate()
	if err == nil {
		t.Fatal("expected validation error for empty title")
	}
	if !strings.Contains(err.Error(), "title") {
		t.Fatalf("expected title validation error, got: %v", err)
	}
}

func TestIssueValidate_TemplateValidation(t *testing.T) {
	cases := []struct {
		name     string
		template string
		wantErr  bool
	}{
		{name: "empty", template: "", wantErr: true},
		{name: "whitespace", template: "   ", wantErr: true},
		{name: "contains space", template: "foo bar", wantErr: true},
		{name: "valid", template: "default_v1", wantErr: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			issue := Issue{
				Title:    "test issue",
				Template: tc.template,
			}

			err := issue.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected template validation error for template %q", tc.template)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected validation success for template %q, got: %v", tc.template, err)
			}
		})
	}
}

func TestIssueStatusValidate_Merging(t *testing.T) {
	if err := IssueStatusMerging.Validate(); err != nil {
		t.Fatalf("IssueStatusMerging should be valid: %v", err)
	}
}

func TestIssueValidate_AllowsMergingStatus(t *testing.T) {
	issue := Issue{
		Title:    "merge branch",
		Template: "standard",
		Status:   IssueStatusMerging,
	}
	if err := issue.Validate(); err != nil {
		t.Fatalf("Issue.Validate() with merging status should pass: %v", err)
	}
}

func TestIssueValidate_ChildrenMode(t *testing.T) {
	cases := []struct {
		name         string
		childrenMode ChildrenMode
		wantErr      bool
	}{
		{name: "empty", childrenMode: "", wantErr: false},
		{name: "parallel", childrenMode: ChildrenModeParallel, wantErr: false},
		{name: "sequential", childrenMode: ChildrenModeSequential, wantErr: false},
		{name: "invalid", childrenMode: ChildrenMode("serial"), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issue := Issue{
				Title:        "children mode validate",
				Template:     "standard",
				ChildrenMode: tc.childrenMode,
			}
			err := issue.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for children mode %q", tc.childrenMode)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for children mode %q: %v", tc.childrenMode, err)
			}
		})
	}
}

func TestIssueJSON_MergeRetriesRoundTrip(t *testing.T) {
	issue := Issue{
		ID:                 "issue-20260305-a1b2c3d4",
		Title:              "merge conflict retry",
		Template:           "standard",
		MergeRetries:       2,
		TriageInstructions: "check git conflict markers before retry",
	}

	raw, err := json.Marshal(issue)
	if err != nil {
		t.Fatalf("marshal issue: %v", err)
	}
	if !strings.Contains(string(raw), `"merge_retries":2`) {
		t.Fatalf("expected merge_retries in JSON, got %s", string(raw))
	}
	if !strings.Contains(string(raw), `"triage_instructions":"check git conflict markers before retry"`) {
		t.Fatalf("expected triage_instructions in JSON, got %s", string(raw))
	}

	var decoded Issue
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal issue: %v", err)
	}
	if decoded.MergeRetries != 2 {
		t.Fatalf("decoded MergeRetries=%d, want 2", decoded.MergeRetries)
	}
	if decoded.TriageInstructions != "check git conflict markers before retry" {
		t.Fatalf("decoded TriageInstructions=%q, want %q", decoded.TriageInstructions, "check git conflict markers before retry")
	}
}

func TestValidateIssueTransition_AllowsExpectedPaths(t *testing.T) {
	cases := []struct {
		name string
		from IssueStatus
		to   IssueStatus
	}{
		{
			name: "review approve to queued",
			from: IssueStatusReviewing,
			to:   IssueStatusQueued,
		},
		{
			name: "execution to merging",
			from: IssueStatusExecuting,
			to:   IssueStatusMerging,
		},
		{
			name: "merge conflict retry requeue",
			from: IssueStatusMerging,
			to:   IssueStatusQueued,
		},
		{
			name: "decomposed parent to done",
			from: IssueStatusDecomposed,
			to:   IssueStatusDone,
		},
		{
			name: "idempotent",
			from: IssueStatusQueued,
			to:   IssueStatusQueued,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateIssueTransition(tc.from, tc.to); err != nil {
				t.Fatalf("ValidateIssueTransition(%q -> %q) returned error: %v", tc.from, tc.to, err)
			}
		})
	}
}

func TestValidateIssueTransition_RejectsInvalidPaths(t *testing.T) {
	cases := []struct {
		name string
		from IssueStatus
		to   IssueStatus
	}{
		{
			name: "draft cannot jump to done",
			from: IssueStatusDraft,
			to:   IssueStatusDone,
		},
		{
			name: "terminal done cannot go back to queued",
			from: IssueStatusDone,
			to:   IssueStatusQueued,
		},
		{
			name: "abandoned terminal cannot resume",
			from: IssueStatusAbandoned,
			to:   IssueStatusQueued,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateIssueTransition(tc.from, tc.to); err == nil {
				t.Fatalf("ValidateIssueTransition(%q -> %q) expected error, got nil", tc.from, tc.to)
			}
		})
	}
}
