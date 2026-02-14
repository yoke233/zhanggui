package outbox

import (
	"errors"
	"strings"
	"testing"
)

func TestContract_CT_REF_001_GitHubIssueRef(t *testing.T) {
	ref, err := ParseGitHubIssueRef("owner/repo#123")
	if err != nil {
		t.Fatalf("ParseGitHubIssueRef() error = %v", err)
	}
	if ref.Owner != "owner" || ref.Repo != "repo" || ref.Number != 123 {
		t.Fatalf("ParseGitHubIssueRef() = %#v", ref)
	}
}

func TestContract_CT_REF_002_GitLabIssueRef(t *testing.T) {
	ref, err := ParseGitLabIssueRef("group/project#456")
	if err != nil {
		t.Fatalf("ParseGitLabIssueRef() error = %v", err)
	}
	if ref.Group != "group" || ref.Project != "project" || ref.IID != 456 {
		t.Fatalf("ParseGitLabIssueRef() = %#v", ref)
	}
}

func TestContract_CT_REF_003_LocalIssueRef(t *testing.T) {
	ref, err := ParseLocalIssueRef("local#12")
	if err != nil {
		t.Fatalf("ParseLocalIssueRef() error = %v", err)
	}
	if ref.IssueID != 12 {
		t.Fatalf("ParseLocalIssueRef() = %#v", ref)
	}
}

func TestContract_CT_REF_004_RejectPlatformInternalIDs(t *testing.T) {
	cases := []string{
		"123456789",
		"MDU6SXNzdWUxMjM0NTY=",
		"gid://gitlab/Issue/1234",
	}

	for _, input := range cases {
		err := ValidateCanonicalIssueRef(input)
		if !errors.Is(err, ErrInternalIssueIDRef) {
			t.Fatalf("ValidateCanonicalIssueRef(%q) error = %v, want ErrInternalIssueIDRef", input, err)
		}
	}
}

func TestContract_CT_RUN_001_RunIDFormat(t *testing.T) {
	run, err := ParseRunID("2026-02-14-backend-0001")
	if err != nil {
		t.Fatalf("ParseRunID() error = %v", err)
	}
	if run.Date != "2026-02-14" || run.Role != "backend" || run.Seq != 1 {
		t.Fatalf("ParseRunID() = %#v", run)
	}
}

func TestContract_CT_RUN_002_OnlyOneActiveRun(t *testing.T) {
	active := "2026-02-14-backend-0002"
	stale := "2026-02-14-backend-0001"

	if !IsStaleRun(active, stale) {
		t.Fatalf("IsStaleRun(%q, %q) should be true", active, stale)
	}
	if IsStaleRun(active, active) {
		t.Fatalf("IsStaleRun(%q, %q) should be false", active, active)
	}
}

func TestContract_CT_CLAIM_001_AssigneeIsSourceOfTruth(t *testing.T) {
	claimText := "/claim"
	_ = claimText

	if err := ValidateClaim(""); !errors.Is(err, ErrIssueNotClaimed) {
		t.Fatalf("ValidateClaim() error = %v, want ErrIssueNotClaimed", err)
	}
}

func TestContract_CT_CLAIM_002_AssigneeSetMeansClaimed(t *testing.T) {
	if err := ValidateClaim("lead-backend"); err != nil {
		t.Fatalf("ValidateClaim() error = %v", err)
	}
	if !IsClaimEffective("lead-backend") {
		t.Fatalf("IsClaimEffective() expected true")
	}
}

func TestContract_CT_WORK_001_WorkOrderRequiredFields(t *testing.T) {
	valid := WorkOrder{
		IssueRef: "local#1",
		RunID:    "2026-02-14-backend-0001",
		Role:     "backend",
		RepoDir:  "D:/project/zhanggui",
	}
	if err := ValidateWorkOrder(valid); err != nil {
		t.Fatalf("ValidateWorkOrder(valid) error = %v", err)
	}

	missingCases := []WorkOrder{
		{RunID: valid.RunID, Role: valid.Role, RepoDir: valid.RepoDir},
		{IssueRef: valid.IssueRef, Role: valid.Role, RepoDir: valid.RepoDir},
		{IssueRef: valid.IssueRef, RunID: valid.RunID, RepoDir: valid.RepoDir},
		{IssueRef: valid.IssueRef, RunID: valid.RunID, Role: valid.Role},
	}

	for _, c := range missingCases {
		err := ValidateWorkOrder(c)
		if !errors.Is(err, ErrWorkOrderMissingField) {
			t.Fatalf("ValidateWorkOrder(%#v) error = %v, want ErrWorkOrderMissingField", c, err)
		}
	}
}

func TestContract_CT_WORK_002_WorkResultEcho(t *testing.T) {
	order := WorkOrder{
		IssueRef: "local#1",
		RunID:    "2026-02-14-backend-0001",
		Role:     "backend",
		RepoDir:  "D:/project/zhanggui",
	}
	result := WorkResult{
		IssueRef: order.IssueRef,
		RunID:    order.RunID,
	}
	if err := ValidateWorkResultEcho(order, result); err != nil {
		t.Fatalf("ValidateWorkResultEcho(valid) error = %v", err)
	}

	issueMismatch := WorkResult{
		IssueRef: "local#2",
		RunID:    order.RunID,
	}
	if err := ValidateWorkResultEcho(order, issueMismatch); !errors.Is(err, ErrWorkResultInvalid) {
		t.Fatalf("ValidateWorkResultEcho(issue mismatch) error = %v, want ErrWorkResultInvalid", err)
	}

	runMismatch := WorkResult{
		IssueRef: order.IssueRef,
		RunID:    "2026-02-14-backend-0002",
	}
	if err := ValidateWorkResultEcho(order, runMismatch); !errors.Is(err, ErrWorkResultInvalid) {
		t.Fatalf("ValidateWorkResultEcho(run mismatch) error = %v, want ErrWorkResultInvalid", err)
	}
}

func TestContract_CT_WORK_003_ChangesAndTestsMinimum(t *testing.T) {
	valid := WorkResult{
		IssueRef: "local#1",
		RunID:    "2026-02-14-backend-0001",
		Changes: &WorkChanges{
			PR:     "none",
			Commit: "git:abc123",
		},
		Tests: &WorkTests{
			Command: "go test ./...",
			Result:  "n/a",
		},
	}
	if err := ValidateWorkResultEvidence(valid); err != nil {
		t.Fatalf("ValidateWorkResultEvidence(valid) error = %v", err)
	}

	missingChanges := WorkResult{
		IssueRef: "local#1",
		RunID:    "2026-02-14-backend-0001",
		Tests: &WorkTests{
			Result: "pass",
		},
	}
	if err := ValidateWorkResultEvidence(missingChanges); !errors.Is(err, ErrWorkResultInvalid) {
		t.Fatalf("ValidateWorkResultEvidence(missing changes) error = %v, want ErrWorkResultInvalid", err)
	}

	noneChanges := WorkResult{
		IssueRef: "local#1",
		RunID:    "2026-02-14-backend-0001",
		Changes: &WorkChanges{
			PR:     "none",
			Commit: "none",
		},
		Tests: &WorkTests{
			Result: "pass",
		},
	}
	if err := ValidateWorkResultEvidence(noneChanges); !errors.Is(err, ErrWorkResultInvalid) {
		t.Fatalf("ValidateWorkResultEvidence(none changes) error = %v, want ErrWorkResultInvalid", err)
	}

	missingTests := WorkResult{
		IssueRef: "local#1",
		RunID:    "2026-02-14-backend-0001",
		Changes: &WorkChanges{
			Commit: "git:abc123",
		},
	}
	if err := ValidateWorkResultEvidence(missingTests); !errors.Is(err, ErrWorkResultInvalid) {
		t.Fatalf("ValidateWorkResultEvidence(missing tests) error = %v, want ErrWorkResultInvalid", err)
	}

	missingResult := WorkResult{
		IssueRef: "local#1",
		RunID:    "2026-02-14-backend-0001",
		Changes: &WorkChanges{
			Commit: "git:abc123",
		},
		Tests: &WorkTests{
			Command: "go test ./...",
			Result:  "",
		},
	}
	if err := ValidateWorkResultEvidence(missingResult); !errors.Is(err, ErrWorkResultInvalid) {
		t.Fatalf("ValidateWorkResultEvidence(missing tests.result) error = %v, want ErrWorkResultInvalid", err)
	}
}

func TestContract_CT_CODE_001_ResultCodeInEnum(t *testing.T) {
	cases := []string{"dep_unresolved", "test_failed", "stale_run"}
	for _, code := range cases {
		if err := ValidateResultCode(code); err != nil {
			t.Fatalf("ValidateResultCode(%q) error = %v", code, err)
		}
	}
}

func TestContract_CT_CODE_002_ResultCodeOutOfEnum(t *testing.T) {
	err := ValidateResultCode("unknown_code")
	if !errors.Is(err, ErrInvalidResultCode) {
		t.Fatalf("ValidateResultCode() error = %v, want ErrInvalidResultCode", err)
	}
	if !strings.Contains(err.Error(), "allowed:") {
		t.Fatalf("ValidateResultCode() error = %v, expected allowed list", err)
	}
}
