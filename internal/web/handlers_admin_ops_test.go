package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/user/ai-workflow/internal/core"
)

func TestAdminOps_ForceReady_Audited(t *testing.T) {
	store := newTestStore(t)
	task := seedAdminTaskFixture(t, store, "pipe-admin-ready", core.ItemPending)

	srv := NewServer(Config{Store: store, BearerToken: "admin-token"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := map[string]any{
		"task_id":  task.ID,
		"trace_id": "trace-force-ready",
	}
	resp := postJSON(t, ts.URL+"/api/v1/admin/ops/force-ready", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	updated, err := store.GetTaskItem(task.ID)
	if err != nil {
		t.Fatalf("GetTaskItem() error = %v", err)
	}
	if updated.Status != core.ItemReady {
		t.Fatalf("task status = %s, want %s", updated.Status, core.ItemReady)
	}

	actions, err := store.GetActions(task.PipelineID)
	if err != nil {
		t.Fatalf("GetActions() error = %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 admin audit action, got %d", len(actions))
	}
	if actions[0].Source != "admin" || actions[0].Action != "force_ready" {
		t.Fatalf("unexpected audit action: %+v", actions[0])
	}
	if !strings.Contains(actions[0].Message, "trace_id=trace-force-ready") {
		t.Fatalf("expected trace_id in audit message, got %q", actions[0].Message)
	}
}

func TestAdminOps_ForceUnblock_Audited(t *testing.T) {
	store := newTestStore(t)
	task := seedAdminTaskFixture(t, store, "pipe-admin-unblock", core.ItemBlockedByFailure)

	srv := NewServer(Config{Store: store, BearerToken: "admin-token"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := map[string]any{
		"task_id":  task.ID,
		"trace_id": "trace-force-unblock",
	}
	resp := postJSON(t, ts.URL+"/api/v1/admin/ops/force-unblock", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	updated, err := store.GetTaskItem(task.ID)
	if err != nil {
		t.Fatalf("GetTaskItem() error = %v", err)
	}
	if updated.Status != core.ItemReady {
		t.Fatalf("task status = %s, want %s", updated.Status, core.ItemReady)
	}

	actions, err := store.GetActions(task.PipelineID)
	if err != nil {
		t.Fatalf("GetActions() error = %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 admin audit action, got %d", len(actions))
	}
	if actions[0].Source != "admin" || actions[0].Action != "force_unblock" {
		t.Fatalf("unexpected audit action: %+v", actions[0])
	}
	if !strings.Contains(actions[0].Message, "trace_id=trace-force-unblock") {
		t.Fatalf("expected trace_id in audit message, got %q", actions[0].Message)
	}
}

func TestAdminOps_ReplayDelivery_TriggersDispatcher(t *testing.T) {
	store := newTestStore(t)
	fake := &fakeWebhookReplayer{replayed: true}

	srv := NewServer(Config{
		Store:           store,
		BearerToken:     "admin-token",
		WebhookReplayer: fake,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := map[string]any{
		"delivery_id": "delivery-admin-1",
		"trace_id":    "trace-replay-admin",
	}
	resp := postJSON(t, ts.URL+"/api/v1/admin/ops/replay-delivery", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if fake.calls != 1 {
		t.Fatalf("expected dispatcher called once, got %d", fake.calls)
	}
	if fake.lastDeliveryID != "delivery-admin-1" {
		t.Fatalf("expected delivery id %q, got %q", "delivery-admin-1", fake.lastDeliveryID)
	}
}

func postJSON(t *testing.T, url string, body map[string]any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal json body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:50001"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	return resp
}

func seedAdminTaskFixture(t *testing.T, store core.Store, pipelineID string, status core.TaskItemStatus) *core.TaskItem {
	t.Helper()

	project := &core.Project{
		ID:       "proj-admin-" + pipelineID,
		Name:     "admin-" + pipelineID,
		RepoPath: filepath.Join(t.TempDir(), "repo"),
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	pipeline := &core.Pipeline{
		ID:              pipelineID,
		ProjectID:       project.ID,
		Name:            "pipeline",
		Description:     "pipeline for admin ops",
		Template:        "standard",
		Status:          core.StatusCreated,
		Stages:          []core.StageConfig{},
		Artifacts:       map[string]string{},
		Config:          map[string]any{},
		MaxTotalRetries: 3,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := store.SavePipeline(pipeline); err != nil {
		t.Fatalf("SavePipeline() error = %v", err)
	}

	plan := &core.TaskPlan{
		ID:         "plan-" + pipelineID,
		ProjectID:  project.ID,
		Name:       "plan",
		Status:     core.PlanExecuting,
		WaitReason: core.WaitNone,
		FailPolicy: core.FailBlock,
	}
	if err := store.SaveTaskPlan(plan); err != nil {
		t.Fatalf("SaveTaskPlan() error = %v", err)
	}

	task := &core.TaskItem{
		ID:          "task-" + pipelineID,
		PlanID:      plan.ID,
		Title:       "admin-task",
		Description: "admin task",
		Status:      status,
		PipelineID:  pipelineID,
	}
	if err := store.SaveTaskItem(task); err != nil {
		t.Fatalf("SaveTaskItem() error = %v", err)
	}
	return task
}

type fakeWebhookReplayer struct {
	calls          int
	lastDeliveryID string
	replayed       bool
	err            error
}

func (f *fakeWebhookReplayer) ReplayByDeliveryID(_ context.Context, deliveryID string) (bool, error) {
	f.calls++
	f.lastDeliveryID = deliveryID
	return f.replayed, f.err
}
