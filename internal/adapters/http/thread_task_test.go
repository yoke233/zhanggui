package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

// TestE2E_ThreadTask_SerialDAG exercises the full HTTP API chain:
// create thread → create task group (work → review) → signal complete → signal approve → group done.
// No real ACP — tests the scheduler + signal API end-to-end.
func TestE2E_ThreadTask_SerialDAG(t *testing.T) {
	env := setupIntegration(t, nil)

	// 1. Create a thread.
	resp, err := postJSON(env.server, "/threads", map[string]any{
		"title": "ThreadTask E2E serial DAG",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create thread status = %d", resp.StatusCode)
	}
	var thread core.Thread
	json.NewDecoder(resp.Body).Decode(&thread)
	resp.Body.Close()
	t.Logf("thread created: id=%d", thread.ID)

	// 2. Create a task group: work → review.
	taskGroupBody := map[string]any{
		"tasks": []map[string]any{
			{
				"assignee":         "researcher",
				"type":             "work",
				"instruction":      "Research competitive pricing in Southeast Asia",
				"output_file_name": "pricing-research.md",
			},
			{
				"assignee":         "reviewer",
				"type":             "review",
				"instruction":      "Review the pricing research report",
				"depends_on_index": []int{0},
				"max_retries":      3,
				"output_file_name": "pricing-review.md",
			},
		},
		"notify_on_complete": false,
	}
	resp, err = postJSON(env.server, fmt.Sprintf("/threads/%d/task-groups", thread.ID), taskGroupBody)
	if err != nil {
		t.Fatalf("create task group: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body := readBody(resp)
		t.Fatalf("create task group status = %d, body = %s", resp.StatusCode, body)
	}
	var detail core.ThreadTaskGroupDetail
	json.NewDecoder(resp.Body).Decode(&detail)
	resp.Body.Close()

	t.Logf("task group created: id=%d, status=%s, tasks=%d", detail.ID, detail.Status, len(detail.Tasks))

	if detail.Status != core.TaskGroupRunning {
		t.Fatalf("group status = %q, want running", detail.Status)
	}
	if len(detail.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(detail.Tasks))
	}

	// Find tasks.
	var workTask, reviewTask *core.ThreadTask
	for i := range detail.Tasks {
		switch detail.Tasks[i].Assignee {
		case "researcher":
			workTask = detail.Tasks[i]
		case "reviewer":
			reviewTask = detail.Tasks[i]
		}
	}
	if workTask == nil || reviewTask == nil {
		t.Fatal("expected both researcher and reviewer tasks")
	}

	// Work task should be running (no agent pool → dispatch sets running but no agent call).
	if workTask.Status != core.ThreadTaskRunning {
		t.Fatalf("work task status = %q, want running", workTask.Status)
	}
	// Review task should be pending (depends on work).
	if reviewTask.Status != core.ThreadTaskPending {
		t.Fatalf("review task status = %q, want pending", reviewTask.Status)
	}

	// 3. Signal work task complete.
	resp, err = postJSON(env.server, fmt.Sprintf("/thread-tasks/%d/signal", workTask.ID), map[string]any{
		"action":           "complete",
		"output_file_path": "outputs/pricing-research.md",
	})
	if err != nil {
		t.Fatalf("signal work complete: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body := readBody(resp)
		t.Fatalf("signal work complete status = %d, body = %s", resp.StatusCode, body)
	}
	resp.Body.Close()
	t.Log("work task signaled complete")

	// 4. Get group detail — review should now be running.
	resp, err = getJSON(env.server, fmt.Sprintf("/task-groups/%d", detail.ID))
	if err != nil {
		t.Fatalf("get group: %v", err)
	}
	var detailAfterWork core.ThreadTaskGroupDetail
	json.NewDecoder(resp.Body).Decode(&detailAfterWork)
	resp.Body.Close()

	for _, task := range detailAfterWork.Tasks {
		if task.Assignee == "reviewer" {
			if task.Status != core.ThreadTaskRunning {
				t.Fatalf("review task after work done = %q, want running", task.Status)
			}
			reviewTask = task // update reference
		}
	}

	// 5. Signal review task complete (approve).
	resp, err = postJSON(env.server, fmt.Sprintf("/thread-tasks/%d/signal", reviewTask.ID), map[string]any{
		"action":           "complete",
		"output_file_path": "outputs/pricing-review.md",
	})
	if err != nil {
		t.Fatalf("signal review complete: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body := readBody(resp)
		t.Fatalf("signal review complete status = %d, body = %s", resp.StatusCode, body)
	}
	resp.Body.Close()
	t.Log("review task signaled complete")

	// 6. Group should be done.
	resp, err = getJSON(env.server, fmt.Sprintf("/task-groups/%d", detail.ID))
	if err != nil {
		t.Fatalf("get final group: %v", err)
	}
	var finalDetail core.ThreadTaskGroupDetail
	json.NewDecoder(resp.Body).Decode(&finalDetail)
	resp.Body.Close()

	if finalDetail.Status != core.TaskGroupDone {
		t.Fatalf("final group status = %q, want done", finalDetail.Status)
	}
	t.Logf("group completed: status=%s", finalDetail.Status)

	// 7. Verify messages were created in the thread.
	resp, err = getJSON(env.server, fmt.Sprintf("/threads/%d/messages?limit=50", thread.ID))
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	var messages []core.ThreadMessage
	json.NewDecoder(resp.Body).Decode(&messages)
	resp.Body.Close()

	if len(messages) == 0 {
		t.Fatal("expected chat messages (progress card, output cards)")
	}
	t.Logf("thread has %d messages", len(messages))
}

// TestE2E_ThreadTask_ReviewReject exercises reject → retry → complete flow.
func TestE2E_ThreadTask_ReviewReject(t *testing.T) {
	env := setupIntegration(t, nil)

	// Create thread.
	resp, _ := postJSON(env.server, "/threads", map[string]any{"title": "ThreadTask reject test"})
	var thread core.Thread
	json.NewDecoder(resp.Body).Decode(&thread)
	resp.Body.Close()

	// Create group: work → review (max_retries=1).
	resp, err := postJSON(env.server, fmt.Sprintf("/threads/%d/task-groups", thread.ID), map[string]any{
		"tasks": []map[string]any{
			{"assignee": "worker", "type": "work", "instruction": "do work", "max_retries": 1, "output_file_name": "out.md"},
			{"assignee": "checker", "type": "review", "instruction": "check", "depends_on_index": []int{0}, "output_file_name": "check.md"},
		},
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	var detail core.ThreadTaskGroupDetail
	json.NewDecoder(resp.Body).Decode(&detail)
	resp.Body.Close()

	var workID, reviewID int64
	for _, task := range detail.Tasks {
		if task.Assignee == "worker" {
			workID = task.ID
		} else {
			reviewID = task.ID
		}
	}

	// Complete work.
	resp, _ = postJSON(env.server, fmt.Sprintf("/thread-tasks/%d/signal", workID), map[string]any{
		"action": "complete", "output_file_path": "outputs/out.md",
	})
	resp.Body.Close()

	// Reject review → triggers retry.
	resp, err = postJSON(env.server, fmt.Sprintf("/thread-tasks/%d/signal", reviewID), map[string]any{
		"action": "reject", "output_file_path": "outputs/check.md", "feedback": "missing SEA data",
	})
	if err != nil {
		t.Fatalf("signal reject: %v", err)
	}
	resp.Body.Close()

	// Get group — work should be running again (retry).
	resp, _ = getJSON(env.server, fmt.Sprintf("/task-groups/%d", detail.ID))
	var afterReject core.ThreadTaskGroupDetail
	json.NewDecoder(resp.Body).Decode(&afterReject)
	resp.Body.Close()

	for _, task := range afterReject.Tasks {
		if task.Assignee == "worker" {
			if task.Status != core.ThreadTaskRunning {
				t.Fatalf("worker after reject = %q, want running", task.Status)
			}
			if task.RetryCount != 1 {
				t.Fatalf("retry_count = %d, want 1", task.RetryCount)
			}
			if task.ReviewFeedback != "missing SEA data" {
				t.Fatalf("review_feedback = %q", task.ReviewFeedback)
			}
			workID = task.ID
		}
		if task.Assignee == "checker" {
			reviewID = task.ID
		}
	}

	// Complete work again.
	resp, _ = postJSON(env.server, fmt.Sprintf("/thread-tasks/%d/signal", workID), map[string]any{
		"action": "complete", "output_file_path": "outputs/out.md",
	})
	resp.Body.Close()

	// Approve review.
	resp, _ = postJSON(env.server, fmt.Sprintf("/thread-tasks/%d/signal", reviewID), map[string]any{
		"action": "complete", "output_file_path": "outputs/check.md",
	})
	resp.Body.Close()

	// Group should be done.
	resp, _ = getJSON(env.server, fmt.Sprintf("/task-groups/%d", detail.ID))
	var finalDetail core.ThreadTaskGroupDetail
	json.NewDecoder(resp.Body).Decode(&finalDetail)
	resp.Body.Close()

	if finalDetail.Status != core.TaskGroupDone {
		t.Fatalf("final group = %q, want done", finalDetail.Status)
	}
	t.Logf("reject-retry-approve flow complete")
}

// TestE2E_ThreadTask_ParallelFanInDAG exercises A∥B → C pattern.
func TestE2E_ThreadTask_ParallelFanInDAG(t *testing.T) {
	env := setupIntegration(t, nil)

	resp, _ := postJSON(env.server, "/threads", map[string]any{"title": "Parallel fan-in"})
	var thread core.Thread
	json.NewDecoder(resp.Body).Decode(&thread)
	resp.Body.Close()

	resp, err := postJSON(env.server, fmt.Sprintf("/threads/%d/task-groups", thread.ID), map[string]any{
		"tasks": []map[string]any{
			{"assignee": "a", "instruction": "research A", "output_file_name": "a.md"},
			{"assignee": "b", "instruction": "research B", "output_file_name": "b.md"},
			{"assignee": "c", "instruction": "summarize", "depends_on_index": []int{0, 1}, "output_file_name": "summary.md"},
		},
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	var detail core.ThreadTaskGroupDetail
	json.NewDecoder(resp.Body).Decode(&detail)
	resp.Body.Close()

	tasksByAssignee := make(map[string]*core.ThreadTask)
	for _, task := range detail.Tasks {
		tasksByAssignee[task.Assignee] = task
	}

	// A and B running, C pending.
	if tasksByAssignee["a"].Status != core.ThreadTaskRunning {
		t.Fatalf("a = %q, want running", tasksByAssignee["a"].Status)
	}
	if tasksByAssignee["b"].Status != core.ThreadTaskRunning {
		t.Fatalf("b = %q, want running", tasksByAssignee["b"].Status)
	}
	if tasksByAssignee["c"].Status != core.ThreadTaskPending {
		t.Fatalf("c = %q, want pending", tasksByAssignee["c"].Status)
	}

	// Complete A.
	postJSON(env.server, fmt.Sprintf("/thread-tasks/%d/signal", tasksByAssignee["a"].ID), map[string]any{"action": "complete"})

	// C still pending.
	resp, _ = getJSON(env.server, fmt.Sprintf("/task-groups/%d", detail.ID))
	var mid core.ThreadTaskGroupDetail
	json.NewDecoder(resp.Body).Decode(&mid)
	resp.Body.Close()
	for _, task := range mid.Tasks {
		if task.Assignee == "c" && task.Status != core.ThreadTaskPending {
			t.Fatalf("c after A done = %q, want pending", task.Status)
		}
	}

	// Complete B → C becomes running.
	postJSON(env.server, fmt.Sprintf("/thread-tasks/%d/signal", tasksByAssignee["b"].ID), map[string]any{"action": "complete"})

	resp, _ = getJSON(env.server, fmt.Sprintf("/task-groups/%d", detail.ID))
	var afterB core.ThreadTaskGroupDetail
	json.NewDecoder(resp.Body).Decode(&afterB)
	resp.Body.Close()
	for _, task := range afterB.Tasks {
		if task.Assignee == "c" && task.Status != core.ThreadTaskRunning {
			t.Fatalf("c after A+B done = %q, want running", task.Status)
		}
	}

	// Complete C → group done.
	postJSON(env.server, fmt.Sprintf("/thread-tasks/%d/signal", tasksByAssignee["c"].ID), map[string]any{"action": "complete"})

	resp, _ = getJSON(env.server, fmt.Sprintf("/task-groups/%d", detail.ID))
	var fin core.ThreadTaskGroupDetail
	json.NewDecoder(resp.Body).Decode(&fin)
	resp.Body.Close()

	if fin.Status != core.TaskGroupDone {
		t.Fatalf("final group = %q, want done", fin.Status)
	}
	t.Log("parallel fan-in DAG complete")
}

// TestE2E_ThreadTask_DeleteGroup exercises group deletion.
func TestE2E_ThreadTask_DeleteGroup(t *testing.T) {
	env := setupIntegration(t, nil)

	resp, _ := postJSON(env.server, "/threads", map[string]any{"title": "Delete test"})
	var thread core.Thread
	json.NewDecoder(resp.Body).Decode(&thread)
	resp.Body.Close()

	resp, _ = postJSON(env.server, fmt.Sprintf("/threads/%d/task-groups", thread.ID), map[string]any{
		"tasks": []map[string]any{
			{"assignee": "x", "instruction": "work"},
		},
	})
	var detail core.ThreadTaskGroupDetail
	json.NewDecoder(resp.Body).Decode(&detail)
	resp.Body.Close()

	// Delete.
	req, _ := http.NewRequest(http.MethodDelete, env.server.URL+fmt.Sprintf("/task-groups/%d", detail.ID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete group: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify gone.
	resp, _ = getJSON(env.server, fmt.Sprintf("/task-groups/%d", detail.ID))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted group status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func readBody(resp *http.Response) string {
	var buf [4096]byte
	n, _ := resp.Body.Read(buf[:])
	return string(buf[:n])
}
