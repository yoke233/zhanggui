package github

import (
	"context"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestStatusSyncer_StageStart_UpdatesStatusLabelByStageID(t *testing.T) {
	client := &fakeRunIssueSyncClient{}
	syncer := NewRunStatusSyncer(client)

	err := syncer.SyncRunEvent(context.Background(), core.Event{
		Type:  core.EventStageStart,
		Stage: core.StageImplement,
		Data: map[string]string{
			"issue_number": "42",
		},
	})
	if err != nil {
		t.Fatalf("SyncRunEvent() error = %v", err)
	}

	if len(client.updatedLabels) != 1 {
		t.Fatalf("expected one status label update, got %d", len(client.updatedLabels))
	}
	got := client.updatedLabels[0]
	if got.issueNumber != 42 {
		t.Fatalf("expected issue #42, got #%d", got.issueNumber)
	}
	if len(got.labels) != 1 || got.labels[0] != "status: run_active:implement" {
		t.Fatalf("expected stage status label by stage id, got %#v", got.labels)
	}
}

func TestStatusSyncer_HumanRequired_PostsActionComment(t *testing.T) {
	client := &fakeRunIssueSyncClient{}
	syncer := NewRunStatusSyncer(client)

	err := syncer.SyncRunEvent(context.Background(), core.Event{
		Type:  core.EventHumanRequired,
		Stage: core.StageReview,
		Data: map[string]string{
			"issue_number": "88",
		},
	})
	if err != nil {
		t.Fatalf("SyncRunEvent() error = %v", err)
	}

	if len(client.comments) != 1 {
		t.Fatalf("expected one comment, got %d", len(client.comments))
	}
	comment := client.comments[0]
	if comment.issueNumber != 88 {
		t.Fatalf("expected issue #88, got #%d", comment.issueNumber)
	}
	if !strings.Contains(comment.body, "/approve") || !strings.Contains(comment.body, "review") {
		t.Fatalf("unexpected human-required comment: %q", comment.body)
	}
}

func TestStatusSyncer_Done_ReplacesRunActiveWithDone(t *testing.T) {
	client := &fakeRunIssueSyncClient{}
	syncer := NewRunStatusSyncer(client)

	if err := syncer.SyncRunEvent(context.Background(), core.Event{
		Type:  core.EventStageStart,
		Stage: core.StageImplement,
		Data: map[string]string{
			"issue_number": "66",
		},
	}); err != nil {
		t.Fatalf("stage start sync error = %v", err)
	}
	if err := syncer.SyncRunEvent(context.Background(), core.Event{
		Type: core.EventRunDone,
		Data: map[string]string{
			"issue_number": "66",
		},
	}); err != nil {
		t.Fatalf("Run done sync error = %v", err)
	}

	if len(client.updatedLabels) != 2 {
		t.Fatalf("expected two status updates, got %d", len(client.updatedLabels))
	}
	latest := client.updatedLabels[1]
	if len(latest.labels) != 1 || latest.labels[0] != "status: run_done" {
		t.Fatalf("expected done status label, got %#v", latest.labels)
	}
}

func TestStatusSyncer_NoIssueNumber_SkipSilently(t *testing.T) {
	client := &fakeRunIssueSyncClient{}
	syncer := NewRunStatusSyncer(client)

	err := syncer.SyncRunEvent(context.Background(), core.Event{
		Type:  core.EventStageStart,
		Stage: core.StageImplement,
		Data:  map[string]string{},
	})
	if err != nil {
		t.Fatalf("SyncRunEvent() error = %v", err)
	}

	if len(client.updatedLabels) != 0 {
		t.Fatalf("expected no status update, got %d", len(client.updatedLabels))
	}
	if len(client.comments) != 0 {
		t.Fatalf("expected no comments, got %d", len(client.comments))
	}
}

type fakeRunIssueSyncClient struct {
	updatedLabels []issueLabelUpdate
	comments      []issueComment
}

type issueLabelUpdate struct {
	issueNumber int
	labels      []string
}

type issueComment struct {
	issueNumber int
	body        string
}

func (f *fakeRunIssueSyncClient) UpdateIssueLabels(_ context.Context, issueNumber int, labels []string) error {
	f.updatedLabels = append(f.updatedLabels, issueLabelUpdate{
		issueNumber: issueNumber,
		labels:      append([]string(nil), labels...),
	})
	return nil
}

func (f *fakeRunIssueSyncClient) AddIssueComment(_ context.Context, issueNumber int, body string) error {
	f.comments = append(f.comments, issueComment{
		issueNumber: issueNumber,
		body:        body,
	})
	return nil
}
