package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/teamleader"
)

func TestA2ADisabled_RoutesReturn404WithoutSPAFallback(t *testing.T) {
	srv := NewServer(Config{A2AEnabled: false})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "jsonrpc route",
			method: http.MethodPost,
			path:   "/api/v1/a2a",
			body:   `{"jsonrpc":"2.0","id":"1","method":"message/send"}`,
		},
		{
			name:   "agent card route",
			method: http.MethodGet,
			path:   "/.well-known/agent-card.json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("create request failed: %v", err)
			}
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 404, got %d, body=%s", resp.StatusCode, string(body))
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if strings.Contains(string(body), `<div id="root"></div>`) {
				t.Fatalf("expected hard 404 without SPA fallback, body=%s", string(body))
			}
		})
	}
}

func TestA2AEnabled_RequiresToken(t *testing.T) {
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{"jsonrpc":"2.0","id":"1","method":"message/send"}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/a2a", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401, got %d, body=%s", resp.StatusCode, string(body))
	}
}

func TestA2AEnabled_MethodNotFoundReturns32601(t *testing.T) {
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{"jsonrpc":"2.0","id":"req-1","method":"unknown/method"}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/a2a", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer a2a-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if payload["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc=2.0, got %v", payload["jsonrpc"])
	}
	if payload["id"] != "req-1" {
		t.Fatalf("expected id=req-1, got %v", payload["id"])
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %#v", payload["error"])
	}
	if code, ok := errObj["code"].(float64); !ok || int(code) != -32601 {
		t.Fatalf("expected error.code=-32601, got %#v", errObj["code"])
	}
}

func TestA2AMessageSend_ReturnsTaskSnapshot(t *testing.T) {
	bridge := &fakeA2ABridge{
		sendFn: func(input teamleader.A2ASendMessageInput) (*teamleader.A2ATaskSnapshot, error) {
			if input.ProjectID != "proj-send" {
				t.Fatalf("project id = %q, want %q", input.ProjectID, "proj-send")
			}
			if input.Conversation != "hello from a2a" {
				t.Fatalf("conversation = %q, want %q", input.Conversation, "hello from a2a")
			}
			return &teamleader.A2ATaskSnapshot{
				TaskID:    "task-send",
				ProjectID: "proj-send",
				SessionID: "ctx-send",
				State:     teamleader.A2ATaskStateSubmitted,
			}, nil
		},
	}
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{
		"jsonrpc":"2.0",
		"id":"send-1",
		"method":"message/send",
		"params":{
			"message":{
				"messageId":"m-1",
				"role":"user",
				"parts":[{"kind":"text","text":"hello from a2a"}]
			},
			"metadata":{"project_id":"proj-send"}
		}
	}`
	payload := mustDoA2ARPCRequest(t, ts.URL, reqBody, "a2a-token")
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %#v", payload["result"])
	}
	if result["id"] != "task-send" {
		t.Fatalf("result.id = %v, want %q", result["id"], "task-send")
	}
	status, ok := result["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected result.status object, got %#v", result["status"])
	}
	if status["state"] != string(teamleader.A2ATaskStateSubmitted) {
		t.Fatalf("result.status.state = %v, want %q", status["state"], teamleader.A2ATaskStateSubmitted)
	}
}

func TestA2ATasksGet_ReturnsConsistentState(t *testing.T) {
	bridge := &fakeA2ABridge{
		getFn: func(input teamleader.A2AGetTaskInput) (*teamleader.A2ATaskSnapshot, error) {
			if input.TaskID != "task-get" {
				t.Fatalf("task id = %q, want %q", input.TaskID, "task-get")
			}
			return &teamleader.A2ATaskSnapshot{
				TaskID:    "task-get",
				ProjectID: "proj-get",
				SessionID: "ctx-get",
				State:     teamleader.A2ATaskStateWorking,
			}, nil
		},
	}
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{
		"jsonrpc":"2.0",
		"id":"get-1",
		"method":"tasks/get",
		"params":{"id":"task-get","metadata":{"project_id":"proj-get"}}
	}`
	payload := mustDoA2ARPCRequest(t, ts.URL, reqBody, "a2a-token")
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %#v", payload["result"])
	}
	status, ok := result["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected result.status object, got %#v", result["status"])
	}
	if status["state"] != string(teamleader.A2ATaskStateWorking) {
		t.Fatalf("result.status.state = %v, want %q", status["state"], teamleader.A2ATaskStateWorking)
	}
}

func TestA2ATasksCancel_SuccessStatusTransition(t *testing.T) {
	bridge := &fakeA2ABridge{
		cancelFn: func(input teamleader.A2ACancelTaskInput) (*teamleader.A2ATaskSnapshot, error) {
			if input.TaskID != "task-cancel" {
				t.Fatalf("task id = %q, want %q", input.TaskID, "task-cancel")
			}
			return &teamleader.A2ATaskSnapshot{
				TaskID:    "task-cancel",
				ProjectID: "proj-cancel",
				SessionID: "ctx-cancel",
				State:     teamleader.A2ATaskStateCanceled,
			}, nil
		},
	}
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{
		"jsonrpc":"2.0",
		"id":"cancel-1",
		"method":"tasks/cancel",
		"params":{"id":"task-cancel","metadata":{"project_id":"proj-cancel"}}
	}`
	payload := mustDoA2ARPCRequest(t, ts.URL, reqBody, "a2a-token")
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %#v", payload["result"])
	}
	status, ok := result["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected result.status object, got %#v", result["status"])
	}
	if status["state"] != string(teamleader.A2ATaskStateCanceled) {
		t.Fatalf("result.status.state = %v, want %q", status["state"], teamleader.A2ATaskStateCanceled)
	}
}

func TestA2AMessageSend_FollowUpPassesTaskID(t *testing.T) {
	bridge := &fakeA2ABridge{
		sendFn: func(input teamleader.A2ASendMessageInput) (*teamleader.A2ATaskSnapshot, error) {
			if input.TaskID != "task-input-required" {
				t.Fatalf("task id = %q, want %q", input.TaskID, "task-input-required")
			}
			if input.Conversation != "looks good proceed" {
				t.Fatalf("conversation = %q, want %q", input.Conversation, "looks good proceed")
			}
			return &teamleader.A2ATaskSnapshot{
				TaskID:    "task-input-required",
				ProjectID: "proj-followup",
				State:     teamleader.A2ATaskStateWorking,
			}, nil
		},
	}
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{
		"jsonrpc":"2.0",
		"id":"followup-1",
		"method":"message/send",
		"params":{
			"message":{
				"messageId":"m-followup",
				"role":"user",
				"taskId":"task-input-required",
				"parts":[{"kind":"text","text":"looks good proceed"}]
			},
			"metadata":{"project_id":"proj-followup"}
		}
	}`
	payload := mustDoA2ARPCRequest(t, ts.URL, reqBody, "a2a-token")
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %#v", payload["result"])
	}
	status, ok := result["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected result.status object, got %#v", result["status"])
	}
	if status["state"] != string(teamleader.A2ATaskStateWorking) {
		t.Fatalf("result.status.state = %v, want %q", status["state"], teamleader.A2ATaskStateWorking)
	}
}

func TestA2ATasksList_ReturnsTaskList(t *testing.T) {
	bridge := &fakeA2ABridge{
		listFn: func(input teamleader.A2AListTasksInput) (*teamleader.A2ATaskList, error) {
			return &teamleader.A2ATaskList{
				Tasks: []*teamleader.A2ATaskSnapshot{
					{
						TaskID:    "task-1",
						ProjectID: "proj-list",
						SessionID: "ctx-list",
						State:     teamleader.A2ATaskStateWorking,
					},
					{
						TaskID:    "task-2",
						ProjectID: "proj-list",
						SessionID: "ctx-list",
						State:     teamleader.A2ATaskStateCompleted,
					},
				},
				TotalSize: 2,
				PageSize:  50,
			}, nil
		},
	}
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{
		"jsonrpc":"2.0",
		"id":"list-1",
		"method":"tasks/list",
		"params":{"context_id":"ctx-list"}
	}`
	payload := mustDoA2ARPCRequest(t, ts.URL, reqBody, "a2a-token")
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %#v", payload["result"])
	}
	tasks, ok := result["tasks"].([]any)
	if !ok {
		t.Fatalf("expected tasks array, got %#v", result["tasks"])
	}
	if len(tasks) != 2 {
		t.Fatalf("tasks count = %d, want 2", len(tasks))
	}
	totalSize, ok := result["total_size"].(float64)
	if !ok || int(totalSize) != 2 {
		t.Fatalf("total_size = %v, want 2", result["total_size"])
	}
}

func TestA2ATasksList_EmptyParamsSucceeds(t *testing.T) {
	bridge := &fakeA2ABridge{
		listFn: func(input teamleader.A2AListTasksInput) (*teamleader.A2ATaskList, error) {
			return &teamleader.A2ATaskList{
				Tasks:     []*teamleader.A2ATaskSnapshot{},
				TotalSize: 0,
				PageSize:  50,
			}, nil
		},
	}
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{"jsonrpc":"2.0","id":"list-empty-1","method":"tasks/list","params":{}}`
	payload := mustDoA2ARPCRequest(t, ts.URL, reqBody, "a2a-token")
	if _, ok := payload["error"]; ok {
		t.Fatalf("expected success, got error: %v", payload["error"])
	}
}

func TestA2ATasksList_NilParamsSucceeds(t *testing.T) {
	bridge := &fakeA2ABridge{
		listFn: func(input teamleader.A2AListTasksInput) (*teamleader.A2ATaskList, error) {
			return &teamleader.A2ATaskList{Tasks: []*teamleader.A2ATaskSnapshot{}, TotalSize: 0, PageSize: 50}, nil
		},
	}
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{"jsonrpc":"2.0","id":"list-nil-1","method":"tasks/list"}`
	payload := mustDoA2ARPCRequest(t, ts.URL, reqBody, "a2a-token")
	if _, ok := payload["error"]; ok {
		t.Fatalf("expected success, got error: %v", payload["error"])
	}
}

func TestA2ATasksList_BridgeErrorReturnsRPCError(t *testing.T) {
	bridge := &fakeA2ABridge{
		listFn: func(input teamleader.A2AListTasksInput) (*teamleader.A2ATaskList, error) {
			return nil, teamleader.ErrA2AInvalidInput
		},
	}
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{"jsonrpc":"2.0","id":"list-err-1","method":"tasks/list","params":{}}`
	payload := mustDoA2ARPCRequest(t, ts.URL, reqBody, "a2a-token")
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %#v", payload["error"])
	}
	if code, ok := errObj["code"].(float64); !ok || int(code) != -32602 {
		t.Fatalf("expected error.code=-32602, got %#v", errObj["code"])
	}
}

func TestA2ATasksList_Pagination(t *testing.T) {
	bridge := &fakeA2ABridge{
		listFn: func(input teamleader.A2AListTasksInput) (*teamleader.A2ATaskList, error) {
			if input.PageSize != 1 {
				t.Fatalf("page size = %d, want 1", input.PageSize)
			}
			return &teamleader.A2ATaskList{
				Tasks:         []*teamleader.A2ATaskSnapshot{{TaskID: "task-page", ProjectID: "proj-page", State: teamleader.A2ATaskStateWorking}},
				TotalSize:     3,
				PageSize:      1,
				NextPageToken: "1",
			}, nil
		},
	}
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{"jsonrpc":"2.0","id":"list-page-1","method":"tasks/list","params":{"page_size":1}}`
	payload := mustDoA2ARPCRequest(t, ts.URL, reqBody, "a2a-token")
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %#v", payload["result"])
	}
	nextToken, _ := result["next_page_token"].(string)
	if nextToken != "1" {
		t.Fatalf("next_page_token = %q, want %q", nextToken, "1")
	}
}

func TestA2ATasksGet_ReturnsArtifactsInResponse(t *testing.T) {
	bridge := &fakeA2ABridge{
		getFn: func(input teamleader.A2AGetTaskInput) (*teamleader.A2ATaskSnapshot, error) {
			return &teamleader.A2ATaskSnapshot{
				TaskID:     "task-art",
				ProjectID:  "proj-art",
				State:      teamleader.A2ATaskStateCompleted,
				BranchName: "feat/artifact-branch",
				Artifacts: map[string]string{
					"pr_number": "99",
				},
			}, nil
		},
	}
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{
		"jsonrpc":"2.0",
		"id":"get-art-1",
		"method":"tasks/get",
		"params":{"id":"task-art","metadata":{"project_id":"proj-art"}}
	}`
	payload := mustDoA2ARPCRequest(t, ts.URL, reqBody, "a2a-token")
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %#v", payload["result"])
	}

	artifacts, ok := result["artifacts"].([]any)
	if !ok {
		t.Fatalf("expected artifacts array, got %#v", result["artifacts"])
	}
	if len(artifacts) < 2 {
		t.Fatalf("expected at least 2 artifacts (branch + pr_number), got %d", len(artifacts))
	}

	foundBranch := false
	foundPR := false
	for _, raw := range artifacts {
		art, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := art["name"].(string)
		parts, _ := art["parts"].([]any)
		if len(parts) == 0 {
			continue
		}
		part, _ := parts[0].(map[string]any)
		text, _ := part["text"].(string)

		switch name {
		case "branch":
			foundBranch = true
			if text != "feat/artifact-branch" {
				t.Fatalf("branch artifact text = %q, want %q", text, "feat/artifact-branch")
			}
		case "pr_number":
			foundPR = true
			if text != "99" {
				t.Fatalf("pr_number artifact text = %q, want %q", text, "99")
			}
		}
	}
	if !foundBranch {
		t.Fatal("expected branch artifact not found")
	}
	if !foundPR {
		t.Fatal("expected pr_number artifact not found")
	}
}

func TestA2ATasksGet_NoArtifactsWhenEmpty(t *testing.T) {
	bridge := &fakeA2ABridge{
		getFn: func(input teamleader.A2AGetTaskInput) (*teamleader.A2ATaskSnapshot, error) {
			return &teamleader.A2ATaskSnapshot{
				TaskID:    "task-noart",
				ProjectID: "proj-noart",
				State:     teamleader.A2ATaskStateWorking,
			}, nil
		},
	}
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{
		"jsonrpc":"2.0",
		"id":"get-noart-1",
		"method":"tasks/get",
		"params":{"id":"task-noart","metadata":{"project_id":"proj-noart"}}
	}`
	payload := mustDoA2ARPCRequest(t, ts.URL, reqBody, "a2a-token")
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %#v", payload["result"])
	}

	if artifacts, exists := result["artifacts"]; exists && artifacts != nil {
		arr, ok := artifacts.([]any)
		if ok && len(arr) > 0 {
			t.Fatalf("expected no artifacts, got %v", artifacts)
		}
	}
}

func TestA2AInvalidParams_Returns32602(t *testing.T) {
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  &fakeA2ABridge{},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cases := []struct {
		name   string
		body   string
		method string
	}{
		{
			name: "message send missing message",
			body: `{"jsonrpc":"2.0","id":"e-1","method":"message/send","params":{"metadata":{"project_id":"proj"}}}`,
		},
		{
			name: "tasks get missing id",
			body: `{"jsonrpc":"2.0","id":"e-2","method":"tasks/get","params":{"metadata":{"project_id":"proj"}}}`,
		},
		{
			name: "tasks cancel missing id",
			body: `{"jsonrpc":"2.0","id":"e-3","method":"tasks/cancel","params":{"metadata":{"project_id":"proj"}}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := mustDoA2ARPCRequest(t, ts.URL, tc.body, "a2a-token")
			errObj, ok := payload["error"].(map[string]any)
			if !ok {
				t.Fatalf("expected error object, got %#v", payload["error"])
			}
			if code, ok := errObj["code"].(float64); !ok || int(code) != -32602 {
				t.Fatalf("expected error.code=-32602, got %#v", errObj["code"])
			}
		})
	}
}

func TestA2ATasksGet_ProjectScopeReturnsBusinessError(t *testing.T) {
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge: &fakeA2ABridge{
			getFn: func(input teamleader.A2AGetTaskInput) (*teamleader.A2ATaskSnapshot, error) {
				return nil, teamleader.ErrA2AProjectScope
			},
		},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{
		"jsonrpc":"2.0",
		"id":"get-scope-1",
		"method":"tasks/get",
		"params":{"id":"task-get","metadata":{"project_id":"proj-get"}}
	}`
	payload := mustDoA2ARPCRequest(t, ts.URL, reqBody, "a2a-token")
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %#v", payload["error"])
	}
	if code, ok := errObj["code"].(float64); !ok || int(code) >= 0 {
		t.Fatalf("expected business error code < 0, got %#v", errObj["code"])
	}
}

func TestA2AEnabled_InvalidRequestReturns32600(t *testing.T) {
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cases := []struct {
		name string
		body string
		id   any
	}{
		{
			name: "missing jsonrpc",
			body: `{"id":"req-2","method":"message/send"}`,
			id:   "req-2",
		},
		{
			name: "empty method",
			body: `{"jsonrpc":"2.0","id":"req-3","method":""}`,
			id:   "req-3",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/a2a", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("create request failed: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer a2a-token")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
			}

			var payload map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				t.Fatalf("decode response failed: %v", err)
			}
			if payload["id"] != tc.id {
				t.Fatalf("expected id=%v, got %v", tc.id, payload["id"])
			}
			errObj, ok := payload["error"].(map[string]any)
			if !ok {
				t.Fatalf("expected error object, got %#v", payload["error"])
			}
			if code, ok := errObj["code"].(float64); !ok || int(code) != -32600 {
				t.Fatalf("expected error.code=-32600, got %#v", errObj["code"])
			}
		})
	}
}

func TestA2AEnabled_MalformedJSONReturns32600WithNullID(t *testing.T) {
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/a2a", strings.NewReader(`{"jsonrpc":"2.0","id":`))
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer a2a-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if _, ok := payload["id"]; !ok {
		t.Fatalf("expected id field to be present")
	}
	if payload["id"] != nil {
		t.Fatalf("expected id=null, got %v", payload["id"])
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %#v", payload["error"])
	}
	if code, ok := errObj["code"].(float64); !ok || int(code) != -32600 {
		t.Fatalf("expected error.code=-32600, got %#v", errObj["code"])
	}
}

func TestA2AEnabled_AgentCardReturnsJSON(t *testing.T) {
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/.well-known/agent-card.json")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected JSON content type, got %q", got)
	}

	var card map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode agent card failed: %v", err)
	}

	urlRaw, _ := card["url"].(string)
	if !strings.Contains(urlRaw, "/api/v1/a2a") {
		t.Fatalf("expected card url contains /api/v1/a2a, got %q", urlRaw)
	}
	versionRaw, _ := card["protocolVersion"].(string)
	if versionRaw != "0.3" {
		t.Fatalf("expected card protocolVersion=0.3, got %q", versionRaw)
	}
}

func TestA2AEnabled_AgentCardUsesForwardedHostAndProto(t *testing.T) {
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent-card.json", nil)
	req.Host = "internal.service.local"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "a2a.example.com")
	srv.Handler().ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}

	var card map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode agent card failed: %v", err)
	}

	urlRaw, _ := card["url"].(string)
	if urlRaw != "https://a2a.example.com/api/v1/a2a" {
		t.Fatalf("expected forwarded absolute url, got %q", urlRaw)
	}
}

func mustDoA2ARPCRequest(t *testing.T, baseURL string, body string, token string) map[string]any {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/a2a", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(raw))
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	return payload
}

type fakeA2ABridge struct {
	sendFn   func(input teamleader.A2ASendMessageInput) (*teamleader.A2ATaskSnapshot, error)
	getFn    func(input teamleader.A2AGetTaskInput) (*teamleader.A2ATaskSnapshot, error)
	cancelFn func(input teamleader.A2ACancelTaskInput) (*teamleader.A2ATaskSnapshot, error)
	listFn   func(input teamleader.A2AListTasksInput) (*teamleader.A2ATaskList, error)
}

func (f *fakeA2ABridge) SendMessage(ctx context.Context, input teamleader.A2ASendMessageInput) (*teamleader.A2ATaskSnapshot, error) {
	if f.sendFn == nil {
		return nil, errors.New("send not implemented")
	}
	return f.sendFn(input)
}

func (f *fakeA2ABridge) GetTask(ctx context.Context, input teamleader.A2AGetTaskInput) (*teamleader.A2ATaskSnapshot, error) {
	if f.getFn == nil {
		return nil, errors.New("get not implemented")
	}
	return f.getFn(input)
}

func (f *fakeA2ABridge) CancelTask(ctx context.Context, input teamleader.A2ACancelTaskInput) (*teamleader.A2ATaskSnapshot, error) {
	if f.cancelFn == nil {
		return nil, errors.New("cancel not implemented")
	}
	return f.cancelFn(input)
}

func (f *fakeA2ABridge) ListTasks(ctx context.Context, input teamleader.A2AListTasksInput) (*teamleader.A2ATaskList, error) {
	if f.listFn == nil {
		return nil, errors.New("list not implemented")
	}
	return f.listFn(input)
}
