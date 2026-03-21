package planning_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	llmadapter "github.com/yoke233/zhanggui/internal/adapters/llm"
	llmplanning "github.com/yoke233/zhanggui/internal/adapters/planning/llm"
	"github.com/yoke233/zhanggui/internal/adapters/store/sqlite"
	agentapp "github.com/yoke233/zhanggui/internal/application/agent"
	planning "github.com/yoke233/zhanggui/internal/application/planning"
	"github.com/yoke233/zhanggui/internal/core"
)

func newPlanningIntegrationStore(t *testing.T) core.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "planning-integration.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestIntegration_PlanningGenerateAndMaterialize(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q, want Bearer test-key", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		capturedBody = string(body)
		if !strings.Contains(capturedBody, "build a backend API with review") {
			t.Fatalf("request body missing task description: %s", capturedBody)
		}
		if !strings.Contains(capturedBody, "generate_dag") || !strings.Contains(capturedBody, "backend") || !strings.Contains(capturedBody, "review") {
			t.Fatalf("request body missing schema/profile hints: %s", capturedBody)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_123",
			"object":"response",
			"created_at":1742000000,
			"model":"gpt-4.1-mini",
			"output":[
				{
					"id":"msg_123",
					"type":"message",
					"role":"assistant",
					"status":"completed",
					"content":[
						{
							"type":"output_text",
							"text":"{\"actions\":[{\"name\":\"implement-api\",\"type\":\"exec\",\"agent_role\":\"worker\",\"required_capabilities\":[\"backend\"],\"description\":\"Implement the API\",\"acceptance_criteria\":[\"tests pass\"]},{\"name\":\"review-api\",\"type\":\"gate\",\"depends_on\":[\"implement-api\"],\"agent_role\":\"gate\",\"required_capabilities\":[\"review\"],\"description\":\"Review the API\",\"acceptance_criteria\":[\"approved\"]}]}"
						}
					]
				}
			]
		}`))
	}))
	defer srv.Close()

	client, err := llmadapter.New(llmadapter.Config{
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Model:   "gpt-4.1-mini",
	})
	if err != nil {
		t.Fatalf("llm.New() error = %v", err)
	}

	registry := agentapp.NewConfigRegistry()
	registry.LoadProfiles([]*core.AgentProfile{
		{ID: "worker-backend", Role: core.RoleWorker, Capabilities: []string{"backend"}},
		{ID: "gate-review", Role: core.RoleGate, Capabilities: []string{"review"}},
	})

	svc := planning.NewService(llmplanning.NewCompleter(client), registry)
	dag, err := svc.Generate(context.Background(), planning.GenerateInput{Description: "build a backend API with review"})
	if err != nil {
		t.Fatalf("Generate(integration) error = %v", err)
	}
	if len(dag.Actions) != 2 || len(dag.Actions[1].DependsOn) != 1 || dag.Actions[1].DependsOn[0] != "implement-api" {
		t.Fatalf("generated dag = %#v", dag)
	}

	store := newPlanningIntegrationStore(t)
	workItemID, err := store.CreateWorkItem(context.Background(), &core.WorkItem{Title: "integration-planning", Status: core.WorkItemOpen})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}

	actions, err := svc.Materialize(context.Background(), store, workItemID, dag)
	if err != nil {
		t.Fatalf("Materialize(integration) error = %v", err)
	}
	if len(actions) != 2 || actions[0].Type != core.ActionExec || actions[1].Type != core.ActionGate {
		t.Fatalf("materialized actions = %#v", actions)
	}

	stored, err := store.ListActionsByWorkItem(context.Background(), workItemID)
	if err != nil {
		t.Fatalf("ListActionsByWorkItem() error = %v", err)
	}
	if len(stored) != 2 || stored[0].Name != "implement-api" || stored[1].Name != "review-api" {
		t.Fatalf("stored actions = %#v", stored)
	}
}
