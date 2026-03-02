package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/user/ai-workflow/internal/core"
	"github.com/user/ai-workflow/internal/secretary"
)

func TestCreateListGetPlanAndDAG(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-plan-api",
		Name:     "plan-api",
		RepoPath: filepath.Join(t.TempDir(), "repo-plan-api"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	session := &core.ChatSession{
		ID:        "chat-20260301-planapi01",
		ProjectID: project.ID,
		Messages: []core.ChatMessage{
			{Role: "user", Content: "把 OAuth 登录拆成任务"},
		},
	}
	if err := store.CreateChatSession(session); err != nil {
		t.Fatalf("seed chat session: %v", err)
	}

	createCalled := false
	planManager := &testPlanManager{
		createDraftFn: func(_ context.Context, input secretary.CreateDraftInput) (*core.TaskPlan, error) {
			createCalled = true
			planID := core.NewTaskPlanID()
			planName := strings.TrimSpace(input.Name)
			if planName == "" {
				planName = planID
			}
			failPolicy := input.FailPolicy
			if failPolicy == "" {
				failPolicy = core.FailBlock
			}
			plan := &core.TaskPlan{
				ID:         planID,
				ProjectID:  input.ProjectID,
				SessionID:  input.SessionID,
				Name:       planName,
				Status:     core.PlanDraft,
				WaitReason: core.WaitNone,
				FailPolicy: failPolicy,
			}
			if err := store.CreateTaskPlan(plan); err != nil {
				return nil, err
			}
			return store.GetTaskPlan(plan.ID)
		},
	}
	srv := NewServer(Config{Store: store, PlanManager: planManager})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"session_id": session.ID,
		"name":       "oauth-plan",
	})
	if err != nil {
		t.Fatalf("marshal create plan body: %v", err)
	}

	createResp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-plan-api/plans",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST /api/v1/projects/{pid}/plans: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	var created core.TaskPlan
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created plan: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty plan id")
	}
	if created.Status != core.PlanDraft {
		t.Fatalf("expected status draft, got %s", created.Status)
	}
	if !createCalled {
		t.Fatal("expected create draft to be delegated to plan manager")
	}

	task1 := core.TaskItem{
		ID:          "task-planapi-1",
		PlanID:      created.ID,
		Title:       "设计 OAuth 回调路由",
		Description: "设计 OAuth 回调路由并定义请求参数",
		Status:      core.ItemPending,
	}
	task2 := core.TaskItem{
		ID:          "task-planapi-2",
		PlanID:      created.ID,
		Title:       "补齐登录状态测试",
		Description: "补齐登录状态测试并覆盖 token 刷新路径",
		DependsOn:   []string{task1.ID},
		Status:      core.ItemRunning,
	}
	if err := store.CreateTaskItem(&task1); err != nil {
		t.Fatalf("seed task1: %v", err)
	}
	if err := store.CreateTaskItem(&task2); err != nil {
		t.Fatalf("seed task2: %v", err)
	}

	listResp, err := http.Get(ts.URL + "/api/v1/projects/proj-plan-api/plans?status=draft&limit=10&offset=0")
	if err != nil {
		t.Fatalf("GET /api/v1/projects/{pid}/plans: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}

	var listed struct {
		Items  []core.TaskPlan `json:"items"`
		Total  int             `json:"total"`
		Offset int             `json:"offset"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list plans response: %v", err)
	}
	if listed.Total != 1 {
		t.Fatalf("expected total=1, got %d", listed.Total)
	}
	if len(listed.Items) != 1 || listed.Items[0].ID != created.ID {
		t.Fatalf("unexpected list items: %#v", listed.Items)
	}

	getResp, err := http.Get(ts.URL + "/api/v1/projects/proj-plan-api/plans/" + created.ID)
	if err != nil {
		t.Fatalf("GET /api/v1/projects/{pid}/plans/{id}: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	var got core.TaskPlan
	if err := json.NewDecoder(getResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get plan response: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected plan id %s, got %s", created.ID, got.ID)
	}
	if len(got.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got.Tasks))
	}

	dagResp, err := http.Get(ts.URL + "/api/v1/projects/proj-plan-api/plans/" + created.ID + "/dag")
	if err != nil {
		t.Fatalf("GET /api/v1/projects/{pid}/plans/{id}/dag: %v", err)
	}
	defer dagResp.Body.Close()
	if dagResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", dagResp.StatusCode)
	}

	var dag struct {
		Nodes []struct {
			ID         string              `json:"id"`
			Title      string              `json:"title"`
			Status     core.TaskItemStatus `json:"status"`
			PipelineID string              `json:"pipeline_id"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"edges"`
		Stats struct {
			Total   int `json:"total"`
			Pending int `json:"pending"`
			Ready   int `json:"ready"`
			Running int `json:"running"`
			Done    int `json:"done"`
			Failed  int `json:"failed"`
		} `json:"stats"`
	}
	if err := json.NewDecoder(dagResp.Body).Decode(&dag); err != nil {
		t.Fatalf("decode dag response: %v", err)
	}
	if len(dag.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(dag.Nodes))
	}
	if len(dag.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(dag.Edges))
	}
	if dag.Stats.Total != 2 || dag.Stats.Pending != 1 || dag.Stats.Running != 1 {
		t.Fatalf("unexpected stats: %#v", dag.Stats)
	}
}

func TestCreatePlanUsesConfiguredPlanParserRole(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-plan-role-api",
		Name:     "plan-role-api",
		RepoPath: filepath.Join(t.TempDir(), "repo-plan-role-api"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	session := &core.ChatSession{
		ID:        "chat-20260302-planrole01",
		ProjectID: project.ID,
		Messages: []core.ChatMessage{
			{Role: "user", Content: "请生成任务计划"},
		},
	}
	if err := store.CreateChatSession(session); err != nil {
		t.Fatalf("seed chat session: %v", err)
	}

	gotRole := ""
	planManager := &testPlanManager{
		createDraftFn: func(_ context.Context, input secretary.CreateDraftInput) (*core.TaskPlan, error) {
			gotRole = strings.TrimSpace(input.Request.Role)
			plan := &core.TaskPlan{
				ID:         "plan-20260302-role",
				ProjectID:  input.ProjectID,
				SessionID:  input.SessionID,
				Name:       "role-plan",
				Status:     core.PlanDraft,
				WaitReason: core.WaitNone,
				FailPolicy: core.FailBlock,
			}
			if err := store.CreateTaskPlan(plan); err != nil {
				return nil, err
			}
			return store.GetTaskPlan(plan.ID)
		},
	}

	srv := NewServer(Config{
		Store:            store,
		PlanManager:      planManager,
		PlanParserRoleID: "plan_parser_custom",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"session_id": session.ID,
		"name":       "role-plan",
	})
	if err != nil {
		t.Fatalf("marshal create plan body: %v", err)
	}

	resp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-plan-role-api/plans",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST /api/v1/projects/{pid}/plans: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	if gotRole != "plan_parser_custom" {
		t.Fatalf("expected create draft request role %q, got %q", "plan_parser_custom", gotRole)
	}
}

func TestCreatePlanFromFilesHappyPath(t *testing.T) {
	store := newTestStore(t)
	repoRoot := filepath.Join(t.TempDir(), "repo-plan-from-files")
	if err := os.MkdirAll(filepath.Join(repoRoot, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir repo docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "docs", "plan.md"), []byte("任务拆分草案\n- OAuth 回调\n- 状态校验"), 0o644); err != nil {
		t.Fatalf("write docs/plan.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("repo readme"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}

	project := core.Project{
		ID:       "proj-plan-from-files",
		Name:     "plan-from-files",
		RepoPath: repoRoot,
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	session := &core.ChatSession{
		ID:        "chat-20260302-planfiles01",
		ProjectID: project.ID,
		Messages: []core.ChatMessage{
			{Role: "user", Content: "请基于文件生成任务计划"},
		},
	}
	if err := store.CreateChatSession(session); err != nil {
		t.Fatalf("seed chat session: %v", err)
	}

	createFromFilesCalls := 0
	submitReviewCalls := 0
	var capturedCreateInput secretary.CreateDraftInput
	var capturedReviewInput secretary.ReviewInput
	planManager := &testPlanManager{
		createDraftFromFilesFn: func(_ context.Context, input secretary.CreateDraftInput) (*core.TaskPlan, error) {
			createFromFilesCalls++
			capturedCreateInput = input
			planID := core.NewTaskPlanID()
			planName := strings.TrimSpace(input.Name)
			if planName == "" {
				planName = planID
			}
			failPolicy := input.FailPolicy
			if failPolicy == "" {
				failPolicy = core.FailBlock
			}
			plan := &core.TaskPlan{
				ID:         planID,
				ProjectID:  input.ProjectID,
				SessionID:  input.SessionID,
				Name:       planName,
				Status:     core.PlanDraft,
				WaitReason: core.WaitNone,
				FailPolicy: failPolicy,
			}
			if err := store.CreateTaskPlan(plan); err != nil {
				return nil, err
			}
			return store.GetTaskPlan(plan.ID)
		},
		submitReviewFn: func(_ context.Context, planID string, input secretary.ReviewInput) (*core.TaskPlan, error) {
			submitReviewCalls++
			capturedReviewInput = input
			loaded, err := store.GetTaskPlan(planID)
			if err != nil {
				return nil, err
			}
			loaded.Status = core.PlanReviewing
			loaded.WaitReason = core.WaitNone
			if err := store.SaveTaskPlan(loaded); err != nil {
				return nil, err
			}
			return store.GetTaskPlan(planID)
		},
	}

	srv := NewServer(Config{Store: store, PlanManager: planManager})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"session_id":  session.ID,
		"name":        "from-files-plan",
		"fail_policy": string(core.FailHuman),
		"file_paths":  []string{"docs/plan.md", "README.md"},
	})
	if err != nil {
		t.Fatalf("marshal create plan from files body: %v", err)
	}
	resp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-plan-from-files/plans/from-files",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST /api/v1/projects/{pid}/plans/from-files: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created struct {
		ID           string              `json:"id"`
		Status       core.TaskPlanStatus `json:"status"`
		SourceFiles  []string            `json:"source_files"`
		FileContents map[string]string   `json:"file_contents"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create plan from files response: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty plan id")
	}
	wantSourceFiles := []string{"docs/plan.md", "README.md"}
	if !reflect.DeepEqual(created.SourceFiles, wantSourceFiles) {
		t.Fatalf("unexpected source_files, want %#v got %#v", wantSourceFiles, created.SourceFiles)
	}
	wantFileContents := map[string]string{
		"docs/plan.md": "任务拆分草案\n- OAuth 回调\n- 状态校验",
		"README.md":    "repo readme",
	}
	if !reflect.DeepEqual(created.FileContents, wantFileContents) {
		t.Fatalf("unexpected file_contents, want %#v got %#v", wantFileContents, created.FileContents)
	}
	if createFromFilesCalls != 1 {
		t.Fatalf("expected CreateDraftFromFiles called once, got %d", createFromFilesCalls)
	}
	if submitReviewCalls != 1 {
		t.Fatalf("expected SubmitReview called once, got %d", submitReviewCalls)
	}
	if !reflect.DeepEqual(capturedCreateInput.SourceFiles, wantSourceFiles) {
		t.Fatalf("unexpected CreateDraftFromFiles.SourceFiles, want %#v got %#v", wantSourceFiles, capturedCreateInput.SourceFiles)
	}
	if !reflect.DeepEqual(capturedCreateInput.FileContents, wantFileContents) {
		t.Fatalf("unexpected CreateDraftFromFiles.FileContents, want %#v got %#v", wantFileContents, capturedCreateInput.FileContents)
	}
	if !reflect.DeepEqual(capturedReviewInput.PlanFileContents, wantFileContents) {
		t.Fatalf("unexpected SubmitReview.PlanFileContents, want %#v got %#v", wantFileContents, capturedReviewInput.PlanFileContents)
	}
}

func TestCreatePlanFromFilesBadRequest(t *testing.T) {
	store := newTestStore(t)
	repoRoot := filepath.Join(t.TempDir(), "repo-plan-from-files-bad-request")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "existing.md"), []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing.md: %v", err)
	}

	project := core.Project{
		ID:       "proj-plan-from-files-bad-request",
		Name:     "plan-from-files-bad-request",
		RepoPath: repoRoot,
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	session := &core.ChatSession{
		ID:        "chat-20260302-planfiles02",
		ProjectID: project.ID,
		Messages: []core.ChatMessage{
			{Role: "user", Content: "bad request"},
		},
	}
	if err := store.CreateChatSession(session); err != nil {
		t.Fatalf("seed chat session: %v", err)
	}

	createFromFilesCalls := 0
	planManager := &testPlanManager{
		createDraftFromFilesFn: func(_ context.Context, _ secretary.CreateDraftInput) (*core.TaskPlan, error) {
			createFromFilesCalls++
			return nil, errors.New("should not be called")
		},
	}

	srv := NewServer(Config{Store: store, PlanManager: planManager})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	testCases := []struct {
		name     string
		body     map[string]any
		wantCode string
	}{
		{
			name: "file_paths empty",
			body: map[string]any{
				"session_id": session.ID,
				"file_paths": []string{},
			},
			wantCode: "FILE_PATHS_REQUIRED",
		},
		{
			name: "path traversal",
			body: map[string]any{
				"session_id": session.ID,
				"file_paths": []string{"../outside.md"},
			},
			wantCode: "INVALID_FILE_PATH",
		},
		{
			name: "file not found",
			body: map[string]any{
				"session_id": session.ID,
				"file_paths": []string{"missing.md"},
			},
			wantCode: "FILE_NOT_FOUND",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rawBody, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatalf("marshal request body: %v", err)
			}
			resp, err := http.Post(
				ts.URL+"/api/v1/projects/proj-plan-from-files-bad-request/plans/from-files",
				"application/json",
				bytes.NewReader(rawBody),
			)
			if err != nil {
				t.Fatalf("POST /api/v1/projects/{pid}/plans/from-files: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}

			var apiErr apiError
			if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
				t.Fatalf("decode api error response: %v", err)
			}
			if apiErr.Code != tc.wantCode {
				t.Fatalf("expected code %s, got %s", tc.wantCode, apiErr.Code)
			}
		})
	}

	if createFromFilesCalls != 0 {
		t.Fatalf("expected CreateDraftFromFiles not called, got %d", createFromFilesCalls)
	}
}

func TestSubmitPlanReviewReturnsReviewing(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-review-api",
		Name:     "review-api",
		RepoPath: filepath.Join(t.TempDir(), "repo-review-api"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	plan := &core.TaskPlan{
		ID:         "plan-20260301-reviewapi",
		ProjectID:  project.ID,
		Name:       "review-plan",
		Status:     core.PlanDraft,
		WaitReason: core.WaitNone,
		FailPolicy: core.FailBlock,
	}
	if err := store.CreateTaskPlan(plan); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	submitCalled := false
	planManager := &testPlanManager{
		submitReviewFn: func(_ context.Context, planID string, _ secretary.ReviewInput) (*core.TaskPlan, error) {
			submitCalled = true
			loaded, err := store.GetTaskPlan(planID)
			if err != nil {
				return nil, err
			}
			loaded.Status = core.PlanReviewing
			loaded.WaitReason = core.WaitNone
			if err := store.SaveTaskPlan(loaded); err != nil {
				return nil, err
			}
			return store.GetTaskPlan(planID)
		},
	}
	srv := NewServer(Config{Store: store, PlanManager: planManager})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		ts.URL+"/api/v1/projects/proj-review-api/plans/"+plan.ID+"/review",
		nil,
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/projects/{pid}/plans/{id}/review: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var out struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode review response: %v", err)
	}
	if out.Status != string(core.PlanReviewing) {
		t.Fatalf("expected status reviewing, got %s", out.Status)
	}
	if !submitCalled {
		t.Fatal("expected submit review to be delegated to plan manager")
	}

	updated, err := store.GetTaskPlan(plan.ID)
	if err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if updated.Status != core.PlanReviewing {
		t.Fatalf("expected persisted status reviewing, got %s", updated.Status)
	}
}

func TestPlanReviewTriggersReviewOrchestrator(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-review-orchestrator",
		Name:     "review-orchestrator",
		RepoPath: filepath.Join(t.TempDir(), "repo-review-orchestrator"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	session := &core.ChatSession{
		ID:        "chat-20260302-review-orchestrator",
		ProjectID: project.ID,
		Messages: []core.ChatMessage{
			{Role: "user", Content: "请评审当前任务拆分"},
			{Role: "assistant", Content: "建议补齐验收项"},
		},
	}
	if err := store.CreateChatSession(session); err != nil {
		t.Fatalf("seed chat session: %v", err)
	}

	plan := &core.TaskPlan{
		ID:         "plan-20260302-review-orchestrator",
		ProjectID:  project.ID,
		SessionID:  session.ID,
		Name:       "review-orchestrator-plan",
		Status:     core.PlanDraft,
		WaitReason: core.WaitNone,
		FailPolicy: core.FailBlock,
	}
	if err := store.CreateTaskPlan(plan); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	submitCalls := 0
	var capturedPlanID string
	var capturedInput secretary.ReviewInput
	planManager := &testPlanManager{
		submitReviewFn: func(_ context.Context, planID string, input secretary.ReviewInput) (*core.TaskPlan, error) {
			submitCalls++
			capturedPlanID = planID
			capturedInput = input
			loaded, err := store.GetTaskPlan(planID)
			if err != nil {
				return nil, err
			}
			loaded.Status = core.PlanReviewing
			loaded.WaitReason = core.WaitNone
			if err := store.SaveTaskPlan(loaded); err != nil {
				return nil, err
			}
			return store.GetTaskPlan(planID)
		},
	}

	srv := NewServer(Config{Store: store, PlanManager: planManager})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		ts.URL+"/api/v1/projects/proj-review-orchestrator/plans/"+plan.ID+"/review",
		nil,
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/projects/{pid}/plans/{id}/review: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if submitCalls != 1 {
		t.Fatalf("expected SubmitReview called once, got %d", submitCalls)
	}
	if capturedPlanID != plan.ID {
		t.Fatalf("expected submitted plan id %q, got %q", plan.ID, capturedPlanID)
	}

	wantConversation := "user: 请评审当前任务拆分\nassistant: 建议补齐验收项"
	if capturedInput.Conversation != wantConversation {
		t.Fatalf("unexpected review conversation, want %q got %q", wantConversation, capturedInput.Conversation)
	}
	if !strings.Contains(capturedInput.ProjectContext, "project=review-orchestrator") {
		t.Fatalf("expected project context contains project name, got %q", capturedInput.ProjectContext)
	}
	if !strings.Contains(capturedInput.ProjectContext, "repo="+project.RepoPath) {
		t.Fatalf("expected project context contains repo path, got %q", capturedInput.ProjectContext)
	}
}

func TestPlanActionRejectRequiresTwoPhaseFeedback(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-action-api",
		Name:     "action-api",
		RepoPath: filepath.Join(t.TempDir(), "repo-action-api"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	plan := &core.TaskPlan{
		ID:         "plan-20260301-actionapi",
		ProjectID:  project.ID,
		Name:       "action-plan",
		Status:     core.PlanWaitingHuman,
		WaitReason: core.WaitFinalApproval,
		FailPolicy: core.FailBlock,
	}
	if err := store.CreateTaskPlan(plan); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	applyCalls := 0
	planManager := &testPlanManager{
		applyActionFn: func(_ context.Context, planID string, action secretary.PlanAction) (*core.TaskPlan, error) {
			applyCalls++
			if action.Action != secretary.PlanActionReject {
				t.Fatalf("expected manager action reject, got %s", action.Action)
			}
			loaded, err := store.GetTaskPlan(planID)
			if err != nil {
				return nil, err
			}
			loaded.Status = core.PlanReviewing
			loaded.WaitReason = core.WaitNone
			if err := store.SaveTaskPlan(loaded); err != nil {
				return nil, err
			}
			return store.GetTaskPlan(planID)
		},
	}
	srv := NewServer(Config{Store: store, PlanManager: planManager})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	assertActionCode := func(body string, wantCode string) {
		t.Helper()
		resp, err := http.Post(
			ts.URL+"/api/v1/projects/proj-action-api/plans/"+plan.ID+"/action",
			"application/json",
			strings.NewReader(body),
		)
		if err != nil {
			t.Fatalf("POST /api/v1/projects/{pid}/plans/{id}/action: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}

		var apiErr apiError
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			t.Fatalf("decode api error: %v", err)
		}
		if apiErr.Code != wantCode {
			t.Fatalf("expected code %s, got %s", wantCode, apiErr.Code)
		}
	}

	assertActionCode(`{"action":"reject"}`, "FEEDBACK_REQUIRED")
	assertActionCode(`{"action":"reject","feedback":{"detail":"这是足够长的说明用于触发缺失类别校验"}}`, "FEEDBACK_CATEGORY_REQUIRED")
	assertActionCode(`{"action":"reject","feedback":{"category":"coverage_gap"}}`, "FEEDBACK_DETAIL_REQUIRED")
	assertActionCode(`{"action":"reject","feedback":{"category":"coverage_gap","detail":"太短"}}`, "INVALID_FEEDBACK")

	successBody := map[string]any{
		"action": "reject",
		"feedback": map[string]any{
			"category":           "coverage_gap",
			"detail":             "当前计划遗漏了审计日志回归测试任务，请补齐并补充依赖关系。",
			"expected_direction": "增加审计日志回归任务并依赖登录主流程任务",
		},
	}
	rawSuccessBody, err := json.Marshal(successBody)
	if err != nil {
		t.Fatalf("marshal success request body: %v", err)
	}
	successResp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-action-api/plans/"+plan.ID+"/action",
		"application/json",
		bytes.NewReader(rawSuccessBody),
	)
	if err != nil {
		t.Fatalf("POST reject with valid feedback: %v", err)
	}
	defer successResp.Body.Close()
	if successResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", successResp.StatusCode)
	}

	var out struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(successResp.Body).Decode(&out); err != nil {
		t.Fatalf("decode success response: %v", err)
	}
	if out.Status != string(core.PlanReviewing) {
		t.Fatalf("expected status reviewing, got %s", out.Status)
	}

	updated, err := store.GetTaskPlan(plan.ID)
	if err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if updated.Status != core.PlanReviewing {
		t.Fatalf("expected persisted status reviewing, got %s", updated.Status)
	}
	if applyCalls != 1 {
		t.Fatalf("expected valid reject to invoke plan manager once, got %d", applyCalls)
	}
}

func TestListPlansTotalReflectsUnpaginatedCount(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-plan-total",
		Name:     "plan-total",
		RepoPath: filepath.Join(t.TempDir(), "repo-plan-total"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	plan1 := &core.TaskPlan{
		ID:         "plan-20260301-total01",
		ProjectID:  project.ID,
		Name:       "total-1",
		Status:     core.PlanDraft,
		WaitReason: core.WaitNone,
		FailPolicy: core.FailBlock,
	}
	plan2 := &core.TaskPlan{
		ID:         "plan-20260301-total02",
		ProjectID:  project.ID,
		Name:       "total-2",
		Status:     core.PlanDraft,
		WaitReason: core.WaitNone,
		FailPolicy: core.FailBlock,
	}
	if err := store.CreateTaskPlan(plan1); err != nil {
		t.Fatalf("seed plan1: %v", err)
	}
	if err := store.CreateTaskPlan(plan2); err != nil {
		t.Fatalf("seed plan2: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects/proj-plan-total/plans?status=draft&limit=1&offset=0")
	if err != nil {
		t.Fatalf("GET /api/v1/projects/{pid}/plans: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var listed struct {
		Items  []core.TaskPlan `json:"items"`
		Total  int             `json:"total"`
		Offset int             `json:"offset"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list plans response: %v", err)
	}
	if listed.Total != 2 {
		t.Fatalf("expected total=2, got %d", listed.Total)
	}
	if len(listed.Items) != 1 {
		t.Fatalf("expected paginated items=1, got %d", len(listed.Items))
	}
}

func TestPlanActionApproveRequiresWaitingHumanFinalApproval(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-action-conflict",
		Name:     "action-conflict",
		RepoPath: filepath.Join(t.TempDir(), "repo-action-conflict"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	plan := &core.TaskPlan{
		ID:         "plan-20260301-conflict",
		ProjectID:  project.ID,
		Name:       "conflict-plan",
		Status:     core.PlanDraft,
		WaitReason: core.WaitNone,
		FailPolicy: core.FailBlock,
	}
	if err := store.CreateTaskPlan(plan); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	planManager := &testPlanManager{
		applyActionFn: func(_ context.Context, _ string, _ secretary.PlanAction) (*core.TaskPlan, error) {
			return nil, errors.New("approve requires waiting_human/final_approval, got draft/none")
		},
	}
	srv := NewServer(Config{Store: store, PlanManager: planManager})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-action-conflict/plans/"+plan.ID+"/action",
		"application/json",
		strings.NewReader(`{"action":"approve"}`),
	)
	if err != nil {
		t.Fatalf("POST /api/v1/projects/{pid}/plans/{id}/action: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}

	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode api error: %v", err)
	}
	if apiErr.Code != "PLAN_STATUS_INVALID" {
		t.Fatalf("expected PLAN_STATUS_INVALID, got %s", apiErr.Code)
	}
}

func TestPlanTaskPayload_IncludesInputsOutputsAcceptance(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-plan-structured",
		Name:     "plan-structured",
		RepoPath: filepath.Join(t.TempDir(), "repo-plan-structured"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	plan := &core.TaskPlan{
		ID:               "plan-20260301-structured",
		ProjectID:        project.ID,
		Name:             "structured-plan",
		Status:           core.PlanDraft,
		WaitReason:       core.WaitNone,
		FailPolicy:       core.FailBlock,
		SpecProfile:      "default",
		ContractVersion:  "v1",
		ContractChecksum: "sha256:abcd",
	}
	if err := store.CreateTaskPlan(plan); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	task := core.TaskItem{
		ID:          "task-structured-1",
		PlanID:      plan.ID,
		Title:       "structured task",
		Description: "structured payload test",
		Inputs:      []string{"oauth_app_id"},
		Outputs:     []string{"oauth_token"},
		Acceptance:  []string{"callback returns 200"},
		Constraints: []string{"must keep backward compatibility"},
		Template:    "standard",
		Status:      core.ItemPending,
	}
	if err := store.CreateTaskItem(&task); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects/proj-plan-structured/plans/" + plan.ID)
	if err != nil {
		t.Fatalf("GET /api/v1/projects/{pid}/plans/{id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got core.TaskPlan
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode plan response: %v", err)
	}
	if len(got.Tasks) != 1 {
		t.Fatalf("expected one task, got %d", len(got.Tasks))
	}
	if len(got.Tasks[0].Inputs) != 1 || got.Tasks[0].Inputs[0] != "oauth_app_id" {
		t.Fatalf("unexpected task inputs: %#v", got.Tasks[0].Inputs)
	}
	if len(got.Tasks[0].Outputs) != 1 || got.Tasks[0].Outputs[0] != "oauth_token" {
		t.Fatalf("unexpected task outputs: %#v", got.Tasks[0].Outputs)
	}
	if len(got.Tasks[0].Acceptance) != 1 || got.Tasks[0].Acceptance[0] != "callback returns 200" {
		t.Fatalf("unexpected task acceptance: %#v", got.Tasks[0].Acceptance)
	}
	if len(got.Tasks[0].Constraints) != 1 || got.Tasks[0].Constraints[0] != "must keep backward compatibility" {
		t.Fatalf("unexpected task constraints: %#v", got.Tasks[0].Constraints)
	}
}

type testPlanManager struct {
	createDraftFn          func(ctx context.Context, input secretary.CreateDraftInput) (*core.TaskPlan, error)
	createDraftFromFilesFn func(ctx context.Context, input secretary.CreateDraftInput) (*core.TaskPlan, error)
	submitReviewFn         func(ctx context.Context, planID string, input secretary.ReviewInput) (*core.TaskPlan, error)
	applyActionFn          func(ctx context.Context, planID string, action secretary.PlanAction) (*core.TaskPlan, error)
}

func (m *testPlanManager) CreateDraft(ctx context.Context, input secretary.CreateDraftInput) (*core.TaskPlan, error) {
	if m.createDraftFn == nil {
		return nil, errors.New("create draft not implemented")
	}
	return m.createDraftFn(ctx, input)
}

func (m *testPlanManager) CreateDraftFromFiles(ctx context.Context, input secretary.CreateDraftInput) (*core.TaskPlan, error) {
	if m.createDraftFromFilesFn == nil {
		return nil, errors.New("create draft from files not implemented")
	}
	return m.createDraftFromFilesFn(ctx, input)
}

func (m *testPlanManager) SubmitReview(ctx context.Context, planID string, input secretary.ReviewInput) (*core.TaskPlan, error) {
	if m.submitReviewFn == nil {
		return nil, errors.New("submit review not implemented")
	}
	return m.submitReviewFn(ctx, planID, input)
}

func (m *testPlanManager) ApplyPlanAction(ctx context.Context, planID string, action secretary.PlanAction) (*core.TaskPlan, error) {
	if m.applyActionFn == nil {
		return nil, errors.New("apply plan action not implemented")
	}
	return m.applyActionFn(ctx, planID, action)
}
