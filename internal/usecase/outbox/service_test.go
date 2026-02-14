package outbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gormsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"

	domainoutbox "zhanggui/internal/domain/outbox"
	"zhanggui/internal/infrastructure/persistence/sqlite/model"
	sqliterepo "zhanggui/internal/infrastructure/persistence/sqlite/repository"
	sqliteuow "zhanggui/internal/infrastructure/persistence/sqlite/uow"
)

type testCache struct {
	data map[string]string
}

func newTestCache() *testCache {
	return &testCache{
		data: make(map[string]string),
	}
}

func (c *testCache) Get(_ context.Context, key string) (string, bool, error) {
	v, ok := c.data[key]
	return v, ok, nil
}

func (c *testCache) Set(_ context.Context, key string, value string, _ time.Duration) error {
	c.data[key] = value
	return nil
}

func (c *testCache) Delete(_ context.Context, key string) error {
	delete(c.data, key)
	return nil
}

func setupServiceWithDB(t *testing.T) (*Service, *testCache, *gorm.DB) {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "outbox.sqlite")
	db, err := gorm.Open(gormsqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	if err := db.Exec("PRAGMA foreign_keys = ON;").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	if err := db.AutoMigrate(
		&model.Issue{},
		&model.IssueLabel{},
		&model.Event{},
		&model.OutboxKV{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	cache := newTestCache()
	repo := sqliterepo.NewOutboxRepository(db)
	uow := sqliteuow.NewUnitOfWork(db)
	return NewService(repo, uow, cache), cache, db
}

func setupService(t *testing.T) (*Service, *testCache) {
	t.Helper()
	svc, cache, _ := setupServiceWithDB(t)
	return svc, cache
}

func TestCreateIssueStoresLabelsAndCache(t *testing.T) {
	svc, cache := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "phase-1 test issue",
		Body:  "body",
		Labels: []string{
			"to:backend",
			"to:backend",
			" state:todo ",
			"",
		},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	if issueRef != "local#1" {
		t.Fatalf("CreateIssue() issueRef = %q, want local#1", issueRef)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}

	if got.Title != "phase-1 test issue" {
		t.Fatalf("issue title = %q", got.Title)
	}
	if !contains(got.Labels, "to:backend") || !contains(got.Labels, "state:todo") {
		t.Fatalf("labels = %v", got.Labels)
	}

	if cache.data[cacheIssueStatusKey(issueRef)] != "open" {
		t.Fatalf("cache issue status = %q", cache.data[cacheIssueStatusKey(issueRef)])
	}
}

func TestClaimIssueSetsAssigneeStateAndEvent(t *testing.T) {
	svc, cache := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "claim issue",
		Body:  "body",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
		Comment:  "Action: claim\nStatus: doing",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}

	if got.Assignee != "lead-backend" {
		t.Fatalf("assignee = %q", got.Assignee)
	}
	if !contains(got.Labels, "state:doing") {
		t.Fatalf("labels = %v", got.Labels)
	}
	if len(got.Events) != 1 {
		t.Fatalf("events len = %d", len(got.Events))
	}

	body := got.Events[0].Body
	if !strings.Contains(body, "IssueRef: "+issueRef) {
		t.Fatalf("claim event missing issue ref, body=%s", body)
	}
	if !strings.Contains(body, "Action: claim") {
		t.Fatalf("claim event missing action, body=%s", body)
	}
	if !strings.Contains(body, "Status: doing") {
		t.Fatalf("claim event missing status, body=%s", body)
	}
	if !strings.Contains(body, "ResultCode: none") {
		t.Fatalf("claim event missing result code, body=%s", body)
	}
	if !strings.Contains(body, "Changes:") || !strings.Contains(body, "Tests:") || !strings.Contains(body, "Next:") {
		t.Fatalf("claim event missing structured fields, body=%s", body)
	}

	if cache.data[cacheIssueStatusKey(issueRef)] != "state:doing" {
		t.Fatalf("cache status = %q", cache.data[cacheIssueStatusKey(issueRef)])
	}
	if cache.data[cacheIssueAssigneeKey(issueRef)] != "lead-backend" {
		t.Fatalf("cache assignee = %q", cache.data[cacheIssueAssigneeKey(issueRef)])
	}
}

func TestCommentIssueValidatesState(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "comment issue",
		Body:  "body",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}

	if err := svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		State:    "bad-state",
		Body:     "foo",
	}); err == nil {
		t.Fatalf("CommentIssue() expected error for invalid state")
	}

	if err := svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		State:    "review",
		Body:     "Action: update\nStatus: review",
	}); err != nil {
		t.Fatalf("CommentIssue(review) error = %v", err)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:review") {
		t.Fatalf("labels = %v", got.Labels)
	}

	lastEvent := got.Events[len(got.Events)-1].Body
	if !strings.Contains(lastEvent, "IssueRef: "+issueRef) {
		t.Fatalf("normalized event missing issue ref, body=%s", lastEvent)
	}
	if !strings.Contains(lastEvent, "Changes:") || !strings.Contains(lastEvent, "Tests:") {
		t.Fatalf("normalized event missing structured fields, body=%s", lastEvent)
	}
}

func TestCloseIssueRequiresEvidence(t *testing.T) {
	svc, cache := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "close issue",
		Body:  "body",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}

	if err := svc.CloseIssue(ctx, CloseIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-integrator",
	}); err == nil {
		t.Fatalf("CloseIssue() expected error without evidence")
	}

	structured := "Role: backend\nRepo: main\nIssueRef: " + issueRef + "\nRunId: none\nSpecRef: none\nContractsRef: none\nAction: update\nStatus: review\nReadUpTo: none\nTrigger: manual:test\n\nSummary:\n- worker result\n\nChanges:\n- PR: none\n- Commit: git:abc123\n\nTests:\n- Command: go test ./...\n- Result: pass\n- Evidence: none\n\nBlockedBy:\n- none\n\nOpenQuestions:\n- none\n\nNext:\n- @integrator close issue\n"
	if err := svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		State:    "review",
		Body:     structured,
	}); err != nil {
		t.Fatalf("CommentIssue(structured) error = %v", err)
	}

	if err := svc.CloseIssue(ctx, CloseIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-integrator",
		Comment:  "Action: done\nStatus: done",
	}); err != nil {
		t.Fatalf("CloseIssue() error = %v", err)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !got.IsClosed {
		t.Fatalf("issue should be closed")
	}
	if !contains(got.Labels, "state:done") {
		t.Fatalf("labels = %v", got.Labels)
	}

	if cache.data[cacheIssueStatusKey(issueRef)] != "closed" {
		t.Fatalf("cache status = %q", cache.data[cacheIssueStatusKey(issueRef)])
	}
}

func TestIssueRefValidation(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: "12345",
		Assignee: "lead-backend",
		Actor:    "lead-backend",
	})
	if err == nil {
		t.Fatalf("ClaimIssue() expected error for invalid issue ref")
	}
}

func TestCreateTaskIssueRequiresGoalAndAcceptanceCriteria(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	_, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "[kind:task] missing sections",
		Body:   "simple body",
		Labels: []string{"kind:task"},
	})
	if !errors.Is(err, errTaskIssueBody) {
		t.Fatalf("CreateIssue() error = %v, want errTaskIssueBody", err)
	}

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "[kind:task] has sections",
		Body:   "## Goal\n- done\n\n## Acceptance Criteria\n- pass\n",
		Labels: []string{"kind:task"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() with required sections error = %v", err)
	}
	if issueRef == "" {
		t.Fatalf("CreateIssue() issueRef should not be empty")
	}
}

func TestCommentIssueRequiresClaimForWorkState(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "work state without claim",
		Body:  "body",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	err = svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		State:    "review",
		Body:     "worker finished",
	})
	if !errors.Is(err, errIssueNotClaimed) {
		t.Fatalf("CommentIssue() error = %v, want errIssueNotClaimed", err)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:blocked") {
		t.Fatalf("labels = %v", got.Labels)
	}
	if len(got.Events) == 0 {
		t.Fatalf("expected blocked event")
	}
	last := got.Events[len(got.Events)-1].Body
	if !strings.Contains(last, "Action: blocked") || !strings.Contains(last, "Status: blocked") {
		t.Fatalf("blocked event missing action/status, body=%s", last)
	}
	if !strings.Contains(last, "ResultCode: manual_intervention") {
		t.Fatalf("blocked event missing result code, body=%s", last)
	}
}

func TestCommentIssueBlockedByNeedsHuman(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "needs human issue",
		Body:   "body",
		Labels: []string{"needs-human"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}

	err = svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		State:    "review",
		Body:     "try progress",
	})
	if !errors.Is(err, errNeedsHuman) {
		t.Fatalf("CommentIssue() error = %v, want errNeedsHuman", err)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:blocked") {
		t.Fatalf("labels = %v", got.Labels)
	}
	if len(got.Events) == 0 || !strings.Contains(got.Events[len(got.Events)-1].Body, "needs-human") {
		t.Fatalf("last event should mention needs-human, events=%v", got.Events)
	}

	last := got.Events[len(got.Events)-1].Body
	if !strings.Contains(last, "Action: blocked") || !strings.Contains(last, "Status: blocked") {
		t.Fatalf("blocked event missing action/status, body=%s", last)
	}
	if !strings.Contains(last, "ResultCode: manual_intervention") {
		t.Fatalf("blocked event missing result code, body=%s", last)
	}
}

func TestCommentIssueBlockedByUnresolvedDependsOn(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	depRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "dependency",
		Body:  "body",
	})
	if err != nil {
		t.Fatalf("CreateIssue(dependency) error = %v", err)
	}

	mainRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "main issue",
		Body:  "## Dependencies\n- DependsOn:\n  - " + depRef + "\n- BlockedBy:\n  - none\n",
	})
	if err != nil {
		t.Fatalf("CreateIssue(main) error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: mainRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
	}); err != nil {
		t.Fatalf("ClaimIssue(main) error = %v", err)
	}

	err = svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: mainRef,
		Actor:    "lead-backend",
		State:    "review",
		Body:     "try progress",
	})
	if !errors.Is(err, errDependsUnresolved) {
		t.Fatalf("CommentIssue(main) error = %v, want errDependsUnresolved", err)
	}

	mainIssue, err := svc.GetIssue(ctx, mainRef)
	if err != nil {
		t.Fatalf("GetIssue(main) error = %v", err)
	}
	if !contains(mainIssue.Labels, "state:blocked") {
		t.Fatalf("labels = %v", mainIssue.Labels)
	}
	if len(mainIssue.Events) == 0 || !strings.Contains(mainIssue.Events[len(mainIssue.Events)-1].Body, depRef) {
		t.Fatalf("blocked event should contain dependency ref, events=%v", mainIssue.Events)
	}

	last := mainIssue.Events[len(mainIssue.Events)-1].Body
	if !strings.Contains(last, "Action: blocked") || !strings.Contains(last, "Status: blocked") {
		t.Fatalf("blocked event missing action/status, body=%s", last)
	}
	if !strings.Contains(last, "ResultCode: dep_unresolved") {
		t.Fatalf("blocked event missing result code, body=%s", last)
	}
}

func TestCloseIssueAppendsDoneEventWithoutComment(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "close without comment",
		Body:  "body",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}

	structuredEvidence := "Role: backend\nRepo: main\nIssueRef: " + issueRef + "\nRunId: none\nSpecRef: none\nContractsRef: none\nAction: update\nStatus: review\nReadUpTo: none\nTrigger: manual:test\n\nSummary:\n- worker result\n\nChanges:\n- PR: none\n- Commit: git:abc123\n\nTests:\n- Command: go test ./...\n- Result: pass\n- Evidence: none\n\nBlockedBy:\n- none\n\nOpenQuestions:\n- none\n\nNext:\n- @integrator close issue\n"
	if err := svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		State:    "review",
		Body:     structuredEvidence,
	}); err != nil {
		t.Fatalf("CommentIssue(structured) error = %v", err)
	}

	if err := svc.CloseIssue(ctx, CloseIssueInput{
		IssueRef: issueRef,
	}); err != nil {
		t.Fatalf("CloseIssue() error = %v", err)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !got.IsClosed {
		t.Fatalf("issue should be closed")
	}
	if len(got.Events) == 0 {
		t.Fatalf("expected events")
	}

	last := got.Events[len(got.Events)-1]
	if last.Actor != "lead-backend" {
		t.Fatalf("done event actor = %q, want lead-backend", last.Actor)
	}
	if !strings.Contains(last.Body, "Action: done") || !strings.Contains(last.Body, "Status: done") {
		t.Fatalf("done event missing action/status, body=%s", last.Body)
	}
	if !strings.Contains(last.Body, "ResultCode: none") {
		t.Fatalf("done event missing result code, body=%s", last.Body)
	}
}

func TestStructuredCommentRejectsInvalidResultCode(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "invalid result code",
		Body:  "body",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}

	invalid := "Role: backend\nRepo: main\nIssueRef: " + issueRef + "\nRunId: none\nSpecRef: none\nContractsRef: none\nAction: update\nStatus: review\nResultCode: unknown_code\nReadUpTo: none\nTrigger: manual:test\n\nSummary:\n- worker result\n\nChanges:\n- PR: none\n- Commit: git:abc123\n\nTests:\n- Command: go test ./...\n- Result: pass\n- Evidence: none\n\nBlockedBy:\n- none\n\nOpenQuestions:\n- none\n\nNext:\n- @integrator close issue\n"
	err = svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		State:    "review",
		Body:     invalid,
	})
	if err == nil {
		t.Fatalf("CommentIssue() expected error")
	}
	if !errors.Is(err, domainoutbox.ErrInvalidResultCode) {
		t.Fatalf("CommentIssue() error = %v, want ErrInvalidResultCode", err)
	}
}

func TestMissingStateLabelDoesNotBlockProgress(t *testing.T) {
	svc, _, db := setupServiceWithDB(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "missing state label flow",
		Body:  "body",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}

	issueID, err := parseIssueRef(issueRef)
	if err != nil {
		t.Fatalf("parseIssueRef() error = %v", err)
	}
	if err := db.Where("issue_id = ? AND label LIKE ?", issueID, "state:%").Delete(&model.IssueLabel{}).Error; err != nil {
		t.Fatalf("delete state labels error = %v", err)
	}

	if err := svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		State:    "review",
		Body:     "worker update",
	}); err != nil {
		t.Fatalf("CommentIssue() error = %v", err)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:review") {
		t.Fatalf("labels = %v", got.Labels)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func writeTestWorkflow(t *testing.T) string {
	t.Helper()

	content := `
version = 1

[outbox]
backend = "sqlite"
path = "state/outbox.sqlite"

[roles]
enabled = ["backend"]

[repos]
main = "."

[role_repo]
backend = "main"

[groups.backend]
role = "backend"
max_concurrent = 4
listen_labels = ["to:backend"]

[executors.backend]
program = "go"
args = ["test", "./..."]
timeout_seconds = 30
`
	path := filepath.Join(t.TempDir(), "workflow.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow file: %v", err)
	}
	return path
}

func TestLeadSyncOnceSkipsUnclaimedIssue(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "unclaimed issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:todo"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	result, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: workflowPath,
		EventBatch:   100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if result.Processed != 0 {
		t.Fatalf("Processed = %d, want 0", result.Processed)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if len(got.Events) != 0 {
		t.Fatalf("events len = %d, want 0", len(got.Events))
	}
}

func TestLeadSyncOnceSkipsReviewIssue(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "review issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:todo"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}
	if err := svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		State:    "review",
		Body:     "worker finished",
	}); err != nil {
		t.Fatalf("CommentIssue() error = %v", err)
	}

	before, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue(before) error = %v", err)
	}

	result, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:           "backend",
		Assignee:       "lead-backend",
		WorkflowFile:   workflowPath,
		ExecutablePath: "definitely-not-existing-executable",
		EventBatch:     100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if result.Processed != 0 {
		t.Fatalf("Processed = %d, want 0", result.Processed)
	}
	if result.Spawned != 0 {
		t.Fatalf("Spawned = %d, want 0", result.Spawned)
	}

	after, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue(after) error = %v", err)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("events len = %d, want %d", len(after.Events), len(before.Events))
	}
}

func TestLeadSyncOnceWorkerFailureWritesBlocked(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "claimed issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:todo"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}

	result, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:           "backend",
		Assignee:       "lead-backend",
		WorkflowFile:   workflowPath,
		ExecutablePath: "definitely-not-existing-executable",
		EventBatch:     100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if result.Blocked != 1 {
		t.Fatalf("Blocked = %d, want 1", result.Blocked)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:blocked") {
		t.Fatalf("labels = %v", got.Labels)
	}
	if len(got.Events) == 0 {
		t.Fatalf("events should not be empty")
	}
	last := got.Events[len(got.Events)-1].Body
	if !strings.Contains(last, "worker execution failed") {
		t.Fatalf("last event body = %s", last)
	}
}
