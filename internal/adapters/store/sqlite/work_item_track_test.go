package sqlite

import (
	"context"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestWorkItemTrackCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	primaryThreadID := int64(12)
	workItemID := int64(34)
	track := &core.WorkItemTrack{
		Title:                    "stabilize thread incubation",
		Objective:                "land Phase 1",
		Status:                   core.WorkItemTrackPlanning,
		PrimaryThreadID:          &primaryThreadID,
		WorkItemID:               &workItemID,
		PlannerStatus:            "running",
		ReviewerStatus:           "idle",
		AwaitingUserConfirmation: false,
		LatestSummary:            "initial summary",
		PlannerOutput:            map[string]any{"plan": "A"},
		ReviewOutput:             map[string]any{"result": "pending"},
		Metadata:                 map[string]any{"source": "test"},
		CreatedBy:                "user-1",
	}

	id, err := s.CreateWorkItemTrack(ctx, track)
	if err != nil {
		t.Fatalf("create work item track: %v", err)
	}
	if id <= 0 {
		t.Fatal("expected positive track id")
	}

	got, err := s.GetWorkItemTrack(ctx, id)
	if err != nil {
		t.Fatalf("get work item track: %v", err)
	}
	if got.Title != track.Title || got.Status != core.WorkItemTrackPlanning {
		t.Fatalf("unexpected work item track: %+v", got)
	}
	if got.PlannerOutput["plan"] != "A" || got.Metadata["source"] != "test" {
		t.Fatalf("expected json fields to round-trip, got %+v", got)
	}

	got.Title = "updated track"
	got.Status = core.WorkItemTrackReviewing
	got.ReviewerStatus = "running"
	got.AwaitingUserConfirmation = true
	got.LatestSummary = "updated summary"
	got.ReviewOutput = map[string]any{"result": "approved"}
	if err := s.UpdateWorkItemTrack(ctx, got); err != nil {
		t.Fatalf("update work item track: %v", err)
	}

	updated, err := s.GetWorkItemTrack(ctx, id)
	if err != nil {
		t.Fatalf("get updated work item track: %v", err)
	}
	if updated.Title != "updated track" || updated.Status != core.WorkItemTrackReviewing {
		t.Fatalf("unexpected updated work item track: %+v", updated)
	}
	if updated.ReviewOutput["result"] != "approved" || !updated.AwaitingUserConfirmation {
		t.Fatalf("expected updated outputs to persist, got %+v", updated)
	}

	if err := s.UpdateWorkItemTrackStatus(ctx, id, core.WorkItemTrackPaused); err != nil {
		t.Fatalf("update work item track status: %v", err)
	}
	paused, err := s.GetWorkItemTrack(ctx, id)
	if err != nil {
		t.Fatalf("get paused work item track: %v", err)
	}
	if paused.Status != core.WorkItemTrackPaused {
		t.Fatalf("expected paused status, got %q", paused.Status)
	}
}

func TestWorkItemTrackListFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	threadID1 := int64(101)
	threadID2 := int64(202)
	workItemID := int64(303)

	track1 := &core.WorkItemTrack{Title: "track-1", Status: core.WorkItemTrackPlanning, PrimaryThreadID: &threadID1}
	track2 := &core.WorkItemTrack{Title: "track-2", Status: core.WorkItemTrackReviewing, PrimaryThreadID: &threadID2, WorkItemID: &workItemID}
	if _, err := s.CreateWorkItemTrack(ctx, track1); err != nil {
		t.Fatalf("create track1: %v", err)
	}
	if _, err := s.CreateWorkItemTrack(ctx, track2); err != nil {
		t.Fatalf("create track2: %v", err)
	}

	status := core.WorkItemTrackReviewing
	filtered, err := s.ListWorkItemTracks(ctx, core.WorkItemTrackFilter{Status: &status, Limit: 10})
	if err != nil {
		t.Fatalf("list by status: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Title != "track-2" {
		t.Fatalf("unexpected status filtered tracks: %+v", filtered)
	}

	filtered, err = s.ListWorkItemTracks(ctx, core.WorkItemTrackFilter{PrimaryThreadID: &threadID1, Limit: 10})
	if err != nil {
		t.Fatalf("list by primary thread: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Title != "track-1" {
		t.Fatalf("unexpected primary thread filtered tracks: %+v", filtered)
	}

	filtered, err = s.ListWorkItemTracksByWorkItem(ctx, workItemID)
	if err != nil {
		t.Fatalf("list by work item: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Title != "track-2" {
		t.Fatalf("unexpected work item filtered tracks: %+v", filtered)
	}
}

func TestWorkItemTrackThreadAssociation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	threadA, _ := s.CreateThread(ctx, &core.Thread{Title: "thread-a"})
	threadB, _ := s.CreateThread(ctx, &core.Thread{Title: "thread-b"})

	track := &core.WorkItemTrack{Title: "track-association", Status: core.WorkItemTrackDraft}
	trackID, err := s.CreateWorkItemTrack(ctx, track)
	if err != nil {
		t.Fatalf("create track: %v", err)
	}

	if _, err := s.AttachThreadToWorkItemTrack(ctx, &core.WorkItemTrackThread{
		TrackID:      trackID,
		ThreadID:     threadA,
		RelationType: core.WorkItemTrackThreadPrimary,
	}); err != nil {
		t.Fatalf("attach primary thread: %v", err)
	}
	if _, err := s.AttachThreadToWorkItemTrack(ctx, &core.WorkItemTrackThread{
		TrackID:      trackID,
		ThreadID:     threadB,
		RelationType: core.WorkItemTrackThreadSource,
	}); err != nil {
		t.Fatalf("attach source thread: %v", err)
	}

	links, err := s.ListWorkItemTrackThreads(ctx, trackID)
	if err != nil {
		t.Fatalf("list track threads: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 track threads, got %d", len(links))
	}
	if links[0].RelationType != core.WorkItemTrackThreadPrimary || links[1].RelationType != core.WorkItemTrackThreadSource {
		t.Fatalf("unexpected track thread relations: %+v", links)
	}

	got, err := s.GetWorkItemTrack(ctx, trackID)
	if err != nil {
		t.Fatalf("get track after primary attach: %v", err)
	}
	if got.PrimaryThreadID == nil || *got.PrimaryThreadID != threadA {
		t.Fatalf("expected primary thread id %d, got %+v", threadA, got.PrimaryThreadID)
	}

	tracksByThread, err := s.ListWorkItemTracksByThread(ctx, threadB)
	if err != nil {
		t.Fatalf("list tracks by thread: %v", err)
	}
	if len(tracksByThread) != 1 || tracksByThread[0].ID != trackID {
		t.Fatalf("unexpected tracks by thread: %+v", tracksByThread)
	}
}

func TestWorkItemTrackRejectsInvalidStateAndAssociation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.CreateWorkItemTrack(ctx, &core.WorkItemTrack{Title: "bad", Status: core.WorkItemTrackStatus("broken")}); err == nil {
		t.Fatal("expected invalid work item track status error")
	}
	if err := s.UpdateWorkItemTrackStatus(ctx, 1, core.WorkItemTrackStatus("broken")); err == nil {
		t.Fatal("expected invalid work item track status update error")
	}

	validTrackID, err := s.CreateWorkItemTrack(ctx, &core.WorkItemTrack{Title: "valid", Status: core.WorkItemTrackDraft})
	if err != nil {
		t.Fatalf("create valid track: %v", err)
	}
	if _, err := s.AttachThreadToWorkItemTrack(ctx, &core.WorkItemTrackThread{
		TrackID:      validTrackID,
		ThreadID:     1,
		RelationType: core.WorkItemTrackThreadRelation("broken"),
	}); err == nil {
		t.Fatal("expected invalid work item track thread relation error")
	}
}
