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

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestAdminOps_ForceReady_Audited(t *testing.T) {
	store := newTestStore(t)
	issue := seedAdminIssueFixture(t, store, "pipe-admin-ready", core.IssueStatusDraft)

	srv := NewServer(Config{Store: store, Token: "admin-token"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := map[string]any{
		"issue_id": issue.ID,
		"trace_id": "trace-force-ready",
	}
	resp := postJSON(t, ts.URL+"/api/v1/admin/ops/force-ready", body, "admin-token")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	updated, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if updated.Status != core.IssueStatusReady {
		t.Fatalf("issue status = %s, want %s", updated.Status, core.IssueStatusReady)
	}

	actions, err := store.GetActions(issue.RunID)
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

	steps, err := store.ListTaskSteps(issue.ID)
	if err != nil {
		t.Fatalf("ListTaskSteps() error = %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 task step, got %d", len(steps))
	}
	if steps[0].Action != core.StepReady {
		t.Fatalf("task step action = %s, want %s", steps[0].Action, core.StepReady)
	}
	if steps[0].AgentID != "admin" {
		t.Fatalf("task step agent_id = %s, want admin", steps[0].AgentID)
	}
}

func TestAdminOps_ForceUnblock_Audited(t *testing.T) {
	store := newTestStore(t)
	issue := seedAdminIssueFixture(t, store, "pipe-admin-unblock", core.IssueStatusFailed)

	srv := NewServer(Config{Store: store, Token: "admin-token"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := map[string]any{
		"task_id":  issue.ID,
		"trace_id": "trace-force-unblock",
	}
	resp := postJSON(t, ts.URL+"/api/v1/admin/ops/force-unblock", body, "admin-token")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	updated, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if updated.Status != core.IssueStatusReady {
		t.Fatalf("issue status = %s, want %s", updated.Status, core.IssueStatusReady)
	}

	actions, err := store.GetActions(issue.RunID)
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

	steps, err := store.ListTaskSteps(issue.ID)
	if err != nil {
		t.Fatalf("ListTaskSteps() error = %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 task step, got %d", len(steps))
	}
	if steps[0].Action != core.StepReady {
		t.Fatalf("task step action = %s, want %s", steps[0].Action, core.StepReady)
	}
	if steps[0].AgentID != "admin" {
		t.Fatalf("task step agent_id = %s, want admin", steps[0].AgentID)
	}
}

func TestAdminOps_ReplayDelivery_TriggersDispatcher(t *testing.T) {
	store := newTestStore(t)
	fake := &fakeWebhookReplayer{replayed: true}

	srv := NewServer(Config{
		Store:           store,
		Token:           "admin-token",
		WebhookReplayer: fake,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := map[string]any{
		"delivery_id": "delivery-admin-1",
		"trace_id":    "trace-replay-admin",
	}
	resp := postJSON(t, ts.URL+"/api/v1/admin/ops/replay-delivery", body, "admin-token")
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

func postJSON(t *testing.T, url string, body map[string]any, bearerToken ...string) *http.Response {
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
	if len(bearerToken) > 0 && bearerToken[0] != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken[0])
	}
	req.RemoteAddr = "127.0.0.1:50001"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	return resp
}

func seedAdminIssueFixture(t *testing.T, store core.Store, RunID string, status core.IssueStatus) *core.Issue {
	t.Helper()

	project := &core.Project{
		ID:       "proj-admin-" + RunID,
		Name:     "admin-" + RunID,
		RepoPath: filepath.Join(t.TempDir(), "repo"),
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	Run := &core.Run{
		ID:              RunID,
		ProjectID:       project.ID,
		Name:            "Run",
		Description:     "Run for admin ops",
		Template:        "standard",
		Status:          core.StatusQueued,
		Stages:          []core.StageConfig{},
		Artifacts:       map[string]string{},
		Config:          map[string]any{},
		MaxTotalRetries: 3,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := store.SaveRun(Run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	issue := &core.Issue{
		ID:         "issue-" + RunID,
		ProjectID:  project.ID,
		Title:      "admin-issue",
		Body:       "admin issue",
		Template:   "standard",
		State:      core.IssueStateOpen,
		Status:     status,
		RunID:      RunID,
		FailPolicy: core.FailBlock,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := store.SaveIssue(issue); err != nil {
		t.Fatalf("SaveIssue() error = %v", err)
	}
	return issue
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

func TestAdminOps_ListAuditLog_WithFilters(t *testing.T) {
	store := newTestStore(t)
	issue := seedAdminIssueFixture(t, store, "pipe-admin-audit-list", core.IssueStatusReady)

	if err := store.RecordAction(core.HumanAction{
		RunID:   issue.RunID,
		Action:  "force_ready",
		Message: "trace_id=trace-audit-list",
		Source:  "admin",
		UserID:  "admin",
	}); err != nil {
		t.Fatalf("RecordAction() error = %v", err)
	}

	srv := NewServer(Config{Store: store, Token: "admin-token"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(
		http.MethodGet,
		ts.URL+"/api/v1/admin/audit-log?project_id="+issue.ProjectID+"&action=force_ready&user=admin&limit=10&offset=0",
		nil,
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer admin-token")
	req.RemoteAddr = "127.0.0.1:50001"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/admin/audit-log: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var payload adminAuditLogResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Total < 1 || len(payload.Items) < 1 {
		t.Fatalf("expected at least one audit item, got total=%d items=%d", payload.Total, len(payload.Items))
	}
	first := payload.Items[0]
	if first.ProjectID != issue.ProjectID {
		t.Fatalf("project_id = %q, want %q", first.ProjectID, issue.ProjectID)
	}
	if first.IssueID != issue.ID {
		t.Fatalf("issue_id = %q, want %q", first.IssueID, issue.ID)
	}
	if first.Action != "force_ready" || first.UserID != "admin" {
		t.Fatalf("unexpected audit item: %+v", first)
	}
}

func TestAdminOps_ListAuditLog_RejectsInvalidSinceBoundary(t *testing.T) {
	store := newTestStore(t)
	seedAdminIssueFixture(t, store, "pipe-admin-audit-since", core.IssueStatusDraft)

	srv := NewServer(Config{Store: store, Token: "admin-token"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(
		http.MethodGet,
		ts.URL+"/api/v1/admin/audit-log?since=not-a-time",
		nil,
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer admin-token")
	req.RemoteAddr = "127.0.0.1:50001"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/admin/audit-log: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}
