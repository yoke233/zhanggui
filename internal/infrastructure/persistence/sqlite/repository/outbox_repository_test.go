package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	gormsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"zhanggui/internal/infrastructure/persistence/sqlite/model"
	"zhanggui/internal/ports"
)

func setupOutboxRepository(t *testing.T) *OutboxRepository {
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
	if err := db.AutoMigrate(&model.Issue{}, &model.IssueLabel{}, &model.Event{}, &model.OutboxKV{}, &model.QualityEvent{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return NewOutboxRepository(db)
}

func TestListIssuesWithIncludeLabels(t *testing.T) {
	repo := setupOutboxRepository(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	issue1, err := repo.CreateIssue(ctx, ports.OutboxIssue{
		Title:     "i1",
		Body:      "body",
		CreatedAt: now,
		UpdatedAt: now,
	}, []string{"to:backend", "state:todo"})
	if err != nil {
		t.Fatalf("create issue1: %v", err)
	}
	if _, err := repo.CreateIssue(ctx, ports.OutboxIssue{
		Title:     "i2",
		Body:      "body",
		CreatedAt: now,
		UpdatedAt: now,
	}, []string{"to:backend"}); err != nil {
		t.Fatalf("create issue2: %v", err)
	}
	if _, err := repo.CreateIssue(ctx, ports.OutboxIssue{
		Title:     "i3",
		Body:      "body",
		CreatedAt: now,
		UpdatedAt: now,
	}, []string{"to:qa", "state:todo"}); err != nil {
		t.Fatalf("create issue3: %v", err)
	}

	items, err := repo.ListIssues(ctx, ports.OutboxIssueFilter{
		IncludeLabels: []string{"to:backend", "state:todo"},
	})
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListIssues() len = %d", len(items))
	}
	if items[0].IssueID != issue1.IssueID {
		t.Fatalf("ListIssues() issue_id = %d", items[0].IssueID)
	}
}

func TestListEventsAfter(t *testing.T) {
	repo := setupOutboxRepository(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	issue, err := repo.CreateIssue(ctx, ports.OutboxIssue{
		Title:     "events",
		Body:      "body",
		CreatedAt: now,
		UpdatedAt: now,
	}, []string{"to:backend"})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := repo.AppendEvent(ctx, ports.OutboxEventCreate{
			IssueID:   issue.IssueID,
			Actor:     "lead-backend",
			Body:      "event",
			CreatedAt: now,
		}); err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}

	events, err := repo.ListEventsAfter(ctx, 1, 10)
	if err != nil {
		t.Fatalf("ListEventsAfter() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("ListEventsAfter() len = %d", len(events))
	}
	if events[0].EventID != 2 || events[1].EventID != 3 {
		t.Fatalf("ListEventsAfter() event ids = %d,%d", events[0].EventID, events[1].EventID)
	}
}

func TestCreateQualityEventDeduplicatesByIdempotencyKey(t *testing.T) {
	repo := setupOutboxRepository(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	issue, err := repo.CreateIssue(ctx, ports.OutboxIssue{
		Title:     "quality",
		Body:      "body",
		CreatedAt: now,
		UpdatedAt: now,
	}, []string{"to:backend", "state:review"})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	input := ports.OutboxQualityEventCreate{
		IssueID:         issue.IssueID,
		IdempotencyKey:  "qevt:local#1:review:1",
		Source:          "manual",
		ExternalEventID: "review#1",
		Category:        "review",
		Result:          "approved",
		Actor:           "quality-bot",
		Summary:         "review approved",
		EvidenceJSON:    `["https://example.com/review/1"]`,
		PayloadJSON:     `{"kind":"review","result":"approved"}`,
		IngestedAt:      now,
	}

	inserted, err := repo.CreateQualityEvent(ctx, input)
	if err != nil {
		t.Fatalf("CreateQualityEvent(first) error = %v", err)
	}
	if !inserted {
		t.Fatalf("CreateQualityEvent(first) inserted = false, want true")
	}

	inserted, err = repo.CreateQualityEvent(ctx, input)
	if err != nil {
		t.Fatalf("CreateQualityEvent(duplicate) error = %v", err)
	}
	if inserted {
		t.Fatalf("CreateQualityEvent(duplicate) inserted = true, want false")
	}

	events, err := repo.ListQualityEvents(ctx, issue.IssueID, 20)
	if err != nil {
		t.Fatalf("ListQualityEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ListQualityEvents() len = %d, want 1", len(events))
	}
	if events[0].IdempotencyKey != input.IdempotencyKey {
		t.Fatalf("ListQualityEvents() key = %q, want %q", events[0].IdempotencyKey, input.IdempotencyKey)
	}
}
