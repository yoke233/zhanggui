package outbox

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	runIDPattern       = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-([a-z][a-z0-9-]*)-(\d{4})$`)
	graphQLNodeIDRegex = regexp.MustCompile(`^[A-Za-z0-9+/]{16,}={0,2}$`)
)

type GitHubIssueRef struct {
	Owner  string
	Repo   string
	Number uint64
}

type GitLabIssueRef struct {
	Group   string
	Project string
	IID     uint64
}

type LocalIssueRef struct {
	IssueID uint64
}

type RunID struct {
	Date string
	Role string
	Seq  int
}

type WorkOrder struct {
	IssueRef string
	RunID    string
	Role     string
	RepoDir  string
}

type WorkChanges struct {
	PR     string
	Commit string
}

type WorkTests struct {
	Command  string
	Result   string
	Evidence string
}

type WorkResult struct {
	IssueRef   string
	RunID      string
	ResultCode string
	Changes    *WorkChanges
	Tests      *WorkTests
}

var allowedResultCodeSet = map[string]struct{}{
	"dep_unresolved":           {},
	"test_failed":              {},
	"ci_failed":                {},
	"review_changes_requested": {},
	"env_unavailable":          {},
	"permission_denied":        {},
	"output_unparseable":       {},
	"stale_run":                {},
	"manual_intervention":      {},
}

func ParseGitHubIssueRef(issueRef string) (GitHubIssueRef, error) {
	left, number, err := parseRemoteIssueRef(issueRef)
	if err != nil {
		return GitHubIssueRef{}, err
	}

	parts := strings.Split(left, "/")
	if len(parts) != 2 {
		return GitHubIssueRef{}, fmt.Errorf("%w: %q", ErrInvalidIssueRef, issueRef)
	}

	return GitHubIssueRef{
		Owner:  parts[0],
		Repo:   parts[1],
		Number: number,
	}, nil
}

func ParseGitLabIssueRef(issueRef string) (GitLabIssueRef, error) {
	left, iid, err := parseRemoteIssueRef(issueRef)
	if err != nil {
		return GitLabIssueRef{}, err
	}

	parts := strings.Split(left, "/")
	if len(parts) != 2 {
		return GitLabIssueRef{}, fmt.Errorf("%w: %q", ErrInvalidIssueRef, issueRef)
	}

	return GitLabIssueRef{
		Group:   parts[0],
		Project: parts[1],
		IID:     iid,
	}, nil
}

func ParseLocalIssueRef(issueRef string) (LocalIssueRef, error) {
	issueID, err := ParseIssueRef(issueRef)
	if err != nil {
		return LocalIssueRef{}, err
	}
	return LocalIssueRef{IssueID: issueID}, nil
}

func ValidateCanonicalIssueRef(issueRef string) error {
	trimmed := strings.TrimSpace(issueRef)
	if trimmed == "" {
		return ErrIssueRefRequired
	}
	if isInternalIssueID(trimmed) {
		return fmt.Errorf("%w: %q", ErrInternalIssueIDRef, issueRef)
	}

	if _, err := ParseLocalIssueRef(trimmed); err == nil {
		return nil
	}
	if _, err := ParseGitHubIssueRef(trimmed); err == nil {
		return nil
	}
	if _, err := ParseGitLabIssueRef(trimmed); err == nil {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrInvalidIssueRef, issueRef)
}

func ParseRunID(runID string) (RunID, error) {
	trimmed := strings.TrimSpace(runID)
	if trimmed == "" {
		return RunID{}, ErrRunIDRequired
	}

	match := runIDPattern.FindStringSubmatch(trimmed)
	if len(match) != 4 {
		return RunID{}, fmt.Errorf("%w: %q", ErrInvalidRunID, runID)
	}

	dateText := match[1]
	if _, err := time.Parse("2006-01-02", dateText); err != nil {
		return RunID{}, fmt.Errorf("%w: %q", ErrInvalidRunID, runID)
	}

	seq, err := strconv.Atoi(match[3])
	if err != nil || seq <= 0 {
		return RunID{}, fmt.Errorf("%w: %q", ErrInvalidRunID, runID)
	}

	return RunID{
		Date: dateText,
		Role: match[2],
		Seq:  seq,
	}, nil
}

func IsStaleRun(activeRunID string, incomingRunID string) bool {
	active := strings.TrimSpace(activeRunID)
	incoming := strings.TrimSpace(incomingRunID)
	if active == "" || incoming == "" {
		return false
	}
	return active != incoming
}

func IsClaimEffective(assignee string) bool {
	return strings.TrimSpace(assignee) != ""
}

func ValidateClaim(assignee string) error {
	if !IsClaimEffective(assignee) {
		return ErrIssueNotClaimed
	}
	return nil
}

func ValidateWorkOrder(order WorkOrder) error {
	if strings.TrimSpace(order.IssueRef) == "" {
		return fmt.Errorf("%w: issue_ref", ErrWorkOrderMissingField)
	}
	if strings.TrimSpace(order.RunID) == "" {
		return fmt.Errorf("%w: run_id", ErrWorkOrderMissingField)
	}
	if strings.TrimSpace(order.Role) == "" {
		return fmt.Errorf("%w: role", ErrWorkOrderMissingField)
	}
	if strings.TrimSpace(order.RepoDir) == "" {
		return fmt.Errorf("%w: repo_dir", ErrWorkOrderMissingField)
	}

	if err := ValidateCanonicalIssueRef(order.IssueRef); err != nil {
		return err
	}
	if _, err := ParseRunID(order.RunID); err != nil {
		return err
	}
	return nil
}

func ValidateWorkResultEcho(order WorkOrder, result WorkResult) error {
	if strings.TrimSpace(result.IssueRef) == "" {
		return fmt.Errorf("%w: issue_ref is required", ErrWorkResultInvalid)
	}
	if strings.TrimSpace(result.RunID) == "" {
		return fmt.Errorf("%w: run_id is required", ErrWorkResultInvalid)
	}
	if result.IssueRef != order.IssueRef {
		return fmt.Errorf("%w: issue_ref mismatch", ErrWorkResultInvalid)
	}
	if result.RunID != order.RunID {
		return fmt.Errorf("%w: run_id mismatch", ErrWorkResultInvalid)
	}
	return nil
}

func ValidateWorkResultEvidence(result WorkResult) error {
	if result.Changes == nil {
		return fmt.Errorf("%w: changes is required", ErrWorkResultInvalid)
	}
	if IsNoneLike(result.Changes.PR) && IsNoneLike(result.Changes.Commit) {
		return fmt.Errorf("%w: changes require pr or commit", ErrWorkResultInvalid)
	}
	if result.Tests == nil {
		return fmt.Errorf("%w: tests is required", ErrWorkResultInvalid)
	}
	if strings.TrimSpace(result.Tests.Result) == "" {
		return fmt.Errorf("%w: tests.result is required", ErrWorkResultInvalid)
	}
	return nil
}

func ValidateResultCode(code string) error {
	normalized := strings.TrimSpace(code)
	if _, ok := allowedResultCodeSet[normalized]; ok {
		return nil
	}
	return fmt.Errorf("%w: %q (allowed: %s)", ErrInvalidResultCode, code, strings.Join(AllowedResultCodes(), ", "))
}

func AllowedResultCodes() []string {
	out := make([]string, 0, len(allowedResultCodeSet))
	for code := range allowedResultCodeSet {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

func parseRemoteIssueRef(issueRef string) (string, uint64, error) {
	trimmed := strings.TrimSpace(issueRef)
	if trimmed == "" {
		return "", 0, ErrIssueRefRequired
	}
	if isInternalIssueID(trimmed) {
		return "", 0, fmt.Errorf("%w: %q", ErrInternalIssueIDRef, issueRef)
	}

	parts := strings.Split(trimmed, "#")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("%w: %q", ErrInvalidIssueRef, issueRef)
	}

	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])
	if left == "" || right == "" {
		return "", 0, fmt.Errorf("%w: %q", ErrInvalidIssueRef, issueRef)
	}

	pathParts := strings.Split(left, "/")
	if len(pathParts) != 2 || pathParts[0] == "" || pathParts[1] == "" {
		return "", 0, fmt.Errorf("%w: %q", ErrInvalidIssueRef, issueRef)
	}

	number, err := strconv.ParseUint(right, 10, 64)
	if err != nil || number == 0 {
		return "", 0, fmt.Errorf("%w: %q", ErrInvalidIssueRef, issueRef)
	}
	return left, number, nil
}

func isInternalIssueID(issueRef string) bool {
	trimmed := strings.TrimSpace(issueRef)
	if trimmed == "" {
		return false
	}

	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "gid://gitlab/") {
		return true
	}

	if strings.Contains(trimmed, "#") || strings.Contains(trimmed, "/") {
		return false
	}

	if _, err := strconv.ParseUint(trimmed, 10, 64); err == nil {
		return true
	}

	return graphQLNodeIDRegex.MatchString(trimmed)
}
