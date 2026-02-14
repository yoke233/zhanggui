package outbox

import (
	"errors"
	"testing"
)

func TestParseIssueRef(t *testing.T) {
	id, err := ParseIssueRef("local#12")
	if err != nil {
		t.Fatalf("ParseIssueRef() error = %v", err)
	}
	if id != 12 {
		t.Fatalf("ParseIssueRef() id = %d", id)
	}

	_, err = ParseIssueRef("owner/repo#1")
	if !errors.Is(err, ErrUnsupportedIssueRef) {
		t.Fatalf("ParseIssueRef() error = %v, want ErrUnsupportedIssueRef", err)
	}
}

func TestNormalizeStateLabel(t *testing.T) {
	got, err := NormalizeStateLabel("review")
	if err != nil {
		t.Fatalf("NormalizeStateLabel() error = %v", err)
	}
	if got != "state:review" {
		t.Fatalf("NormalizeStateLabel() = %q", got)
	}

	_, err = NormalizeStateLabel("foo")
	if !errors.Is(err, ErrInvalidStateLabel) {
		t.Fatalf("NormalizeStateLabel() error = %v, want ErrInvalidStateLabel", err)
	}
}

func TestParseDependsOnRefs(t *testing.T) {
	body := "## Dependencies\n- DependsOn:\n  - local#1, local#2\n- BlockedBy:\n  - none\n"
	deps := ParseDependsOnRefs(body)
	if len(deps) != 2 || deps[0] != "local#1" || deps[1] != "local#2" {
		t.Fatalf("ParseDependsOnRefs() = %#v", deps)
	}
}

func TestHasCloseEvidenceFromBody(t *testing.T) {
	okBody := "Changes:\n- PR: none\n- Commit: git:abc\n\nTests:\n- Command: go test ./...\n- Result: pass\n- Evidence: none\n"
	if !HasCloseEvidenceFromBody(okBody) {
		t.Fatalf("HasCloseEvidenceFromBody() expected true")
	}

	badBody := "Changes:\n- PR: none\n- Commit: none\n\nTests:\n- Result: pass\n"
	if HasCloseEvidenceFromBody(badBody) {
		t.Fatalf("HasCloseEvidenceFromBody() expected false")
	}
}

func TestEvaluateWorkPreconditions(t *testing.T) {
	_, err := EvaluateWorkPreconditions(WorkPreconditions{
		TargetState: "state:review",
		Assignee:    "",
	})
	if !errors.Is(err, ErrIssueNotClaimed) {
		t.Fatalf("EvaluateWorkPreconditions() error = %v", err)
	}

	blockedBy, err := EvaluateWorkPreconditions(WorkPreconditions{
		TargetState:            "state:review",
		Assignee:               "lead-backend",
		UnresolvedDependencies: []string{"local#2"},
	})
	if !errors.Is(err, ErrDependsUnresolved) {
		t.Fatalf("EvaluateWorkPreconditions() error = %v", err)
	}
	if len(blockedBy) != 1 || blockedBy[0] != "local#2" {
		t.Fatalf("blockedBy = %#v", blockedBy)
	}
}
