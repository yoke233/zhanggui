package api

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// TestE2E_APIIssueLifecycle covers create -> actions -> run -> verify API entities.
func TestE2E_APIIssueLifecycle(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/work-items", map[string]any{"title": "e2e-api", "priority": "medium"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "A", "type": "exec",
	})
	var actionA core.Action
	decodeJSON(resp, &actionA)

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "B", "type": "exec",
	})
	var actionB core.Action
	decodeJSON(resp, &actionB)

	resp, _ = get(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID))
	var actions []*core.Action
	decodeJSON(resp, &actions)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	time.Sleep(500 * time.Millisecond)

	resp, _ = get(ts, fmt.Sprintf("/work-items/%d", issue.ID))
	decodeJSON(resp, &issue)
	if issue.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}

	resp, _ = get(ts, fmt.Sprintf("/steps/%d", actionA.ID))
	decodeJSON(resp, &actionA)
	if actionA.Status != core.ActionDone {
		t.Fatalf("expected action A done, got %s", actionA.Status)
	}

	resp, _ = get(ts, fmt.Sprintf("/steps/%d", actionB.ID))
	decodeJSON(resp, &actionB)
	if actionB.Status != core.ActionDone {
		t.Fatalf("expected action B done, got %s", actionB.Status)
	}

	resp, _ = get(ts, fmt.Sprintf("/steps/%d/executions", actionA.ID))
	var execs []*core.Run
	decodeJSON(resp, &execs)
	if len(execs) == 0 {
		t.Fatal("expected at least 1 execution for action A")
	}
	if execs[0].Status != core.RunSucceeded {
		t.Fatalf("expected succeeded, got %s", execs[0].Status)
	}

	resp, _ = get(ts, fmt.Sprintf("/work-items/%d/events", issue.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for events, got %d", resp.StatusCode)
	}
}
