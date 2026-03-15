package api

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yoke233/ai-workflow/internal/core"
)

func TestWorkItemTrackCreateListAndGet(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "track-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	resp, err := post(ts, fmt.Sprintf("/threads/%d/tracks", thread.ID), map[string]any{
		"title":      "track-alpha",
		"objective":  "ship phase 1",
		"created_by": "user-1",
	})
	if err != nil {
		t.Fatalf("create track: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var track core.WorkItemTrack
	if err := decodeJSON(resp, &track); err != nil {
		t.Fatalf("decode track: %v", err)
	}
	if track.Title != "track-alpha" || track.PrimaryThreadID == nil || *track.PrimaryThreadID != thread.ID {
		t.Fatalf("unexpected track: %+v", track)
	}

	resp, _ = get(ts, fmt.Sprintf("/threads/%d/tracks", thread.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing thread tracks, got %d", resp.StatusCode)
	}
	var tracks []core.WorkItemTrack
	if err := decodeJSON(resp, &tracks); err != nil {
		t.Fatalf("decode tracks: %v", err)
	}
	if len(tracks) != 1 || tracks[0].ID != track.ID {
		t.Fatalf("unexpected thread track list: %+v", tracks)
	}

	resp, _ = get(ts, fmt.Sprintf("/tracks/%d", track.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 getting track, got %d", resp.StatusCode)
	}
	var fetched core.WorkItemTrack
	if err := decodeJSON(resp, &fetched); err != nil {
		t.Fatalf("decode fetched track: %v", err)
	}
	if fetched.ID != track.ID || fetched.Title != "track-alpha" {
		t.Fatalf("unexpected fetched track: %+v", fetched)
	}
}

func TestWorkItemTrackAttachThreadAndMaterialize(t *testing.T) {
	h, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "track-thread-a"})
	var threadA core.Thread
	decodeJSON(resp, &threadA)

	resp, _ = post(ts, "/threads", map[string]any{"title": "track-thread-b"})
	var threadB core.Thread
	decodeJSON(resp, &threadB)

	resp, _ = post(ts, fmt.Sprintf("/threads/%d/tracks", threadA.ID), map[string]any{
		"title":     "track-beta",
		"objective": "materialize me",
	})
	var track core.WorkItemTrack
	decodeJSON(resp, &track)

	resp, err := post(ts, fmt.Sprintf("/tracks/%d/threads", track.ID), map[string]any{
		"thread_id":     threadB.ID,
		"relation_type": "source",
	})
	if err != nil {
		t.Fatalf("attach thread: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 attaching thread, got %d", resp.StatusCode)
	}
	var trackThread core.WorkItemTrackThread
	if err := decodeJSON(resp, &trackThread); err != nil {
		t.Fatalf("decode track thread: %v", err)
	}
	if trackThread.ThreadID != threadB.ID {
		t.Fatalf("unexpected track thread link: %+v", trackThread)
	}

	track.Status = core.WorkItemTrackAwaitingConfirmation
	if err := h.store.UpdateWorkItemTrack(t.Context(), &track); err != nil {
		t.Fatalf("update track state for materialize: %v", err)
	}

	resp, err = post(ts, fmt.Sprintf("/tracks/%d/materialize", track.ID), map[string]any{})
	if err != nil {
		t.Fatalf("materialize track: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 materializing track, got %d", resp.StatusCode)
	}
	var result struct {
		Track    core.WorkItemTrack        `json:"track"`
		WorkItem core.WorkItem             `json:"work_item"`
		Links    []core.ThreadWorkItemLink `json:"links"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		t.Fatalf("decode materialize result: %v", err)
	}
	if result.Track.WorkItemID == nil || *result.Track.WorkItemID != result.WorkItem.ID {
		t.Fatalf("expected track to point to work item, got %+v", result)
	}
	if result.Track.Status != core.WorkItemTrackMaterialized || result.WorkItem.Status != core.WorkItemAccepted {
		t.Fatalf("unexpected materialize result: %+v", result)
	}
	if len(result.Links) != 2 {
		t.Fatalf("expected 2 thread-work item links, got %d", len(result.Links))
	}
}

func TestWorkItemTrackReviewAndConfirmExecution(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "track-lifecycle-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	resp, _ = post(ts, fmt.Sprintf("/threads/%d/tracks", thread.ID), map[string]any{
		"title":     "track-lifecycle",
		"objective": "ship feature",
	})
	var track core.WorkItemTrack
	decodeJSON(resp, &track)

	resp, err := post(ts, fmt.Sprintf("/tracks/%d/submit-review", track.ID), map[string]any{
		"latest_summary":      "planner ready",
		"planner_output_json": map[string]any{"plan": "ok"},
	})
	if err != nil {
		t.Fatalf("submit review: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 submit-review, got %d", resp.StatusCode)
	}
	if err := decodeJSON(resp, &track); err != nil {
		t.Fatalf("decode submit-review response: %v", err)
	}
	if track.Status != core.WorkItemTrackReviewing {
		t.Fatalf("expected reviewing status, got %+v", track)
	}

	resp, err = post(ts, fmt.Sprintf("/tracks/%d/approve-review", track.ID), map[string]any{
		"latest_summary":     "user confirm",
		"review_output_json": map[string]any{"verdict": "approved"},
	})
	if err != nil {
		t.Fatalf("approve review: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 approve-review, got %d", resp.StatusCode)
	}
	if err := decodeJSON(resp, &track); err != nil {
		t.Fatalf("decode approve-review response: %v", err)
	}
	if track.Status != core.WorkItemTrackAwaitingConfirmation {
		t.Fatalf("expected awaiting_confirmation status, got %+v", track)
	}

	resp, err = post(ts, fmt.Sprintf("/tracks/%d/confirm-execution", track.ID), map[string]any{})
	if err != nil {
		t.Fatalf("confirm execution: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 confirm-execution, got %d", resp.StatusCode)
	}
	var result struct {
		Track    core.WorkItemTrack `json:"track"`
		WorkItem core.WorkItem      `json:"work_item"`
		Status   string             `json:"status"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		t.Fatalf("decode confirm-execution response: %v", err)
	}
	if result.Track.Status != core.WorkItemTrackExecuting {
		t.Fatalf("expected executing track, got %+v", result.Track)
	}
	if result.Track.WorkItemID == nil || *result.Track.WorkItemID != result.WorkItem.ID {
		t.Fatalf("expected work item link in track, got %+v", result)
	}
	switch result.WorkItem.Status {
	case core.WorkItemQueued, core.WorkItemRunning, core.WorkItemDone:
	default:
		t.Fatalf("unexpected work item status after confirm-execution: %s", result.WorkItem.Status)
	}
}

func TestWorkItemTrackWebSocketEvent(t *testing.T) {
	_, ts := setupAPI(t)

	resp, err := post(ts, "/threads", map[string]any{"title": "track-ws-thread"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode thread: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "subscribe_thread",
		"data": map[string]any{
			"thread_id": thread.ID,
		},
	}); err != nil {
		t.Fatalf("subscribe thread: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read subscribe ack: %v", err)
	}
	if ack.Type != "thread.subscribed" {
		t.Fatalf("expected thread.subscribed, got %q", ack.Type)
	}

	time.Sleep(50 * time.Millisecond)

	if _, err := post(ts, fmt.Sprintf("/threads/%d/tracks", thread.ID), map[string]any{
		"title": "track-ws",
	}); err != nil {
		t.Fatalf("create track: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var event core.Event
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatalf("read track event: %v", err)
	}
	if event.Type != core.EventThreadTrackCreated {
		t.Fatalf("expected thread.track.created, got %s", event.Type)
	}
	threadID, ok := threadIDFromEventData(event.Data)
	if !ok || threadID != thread.ID {
		t.Fatalf("unexpected thread_id in event payload: %+v", event.Data)
	}
}
