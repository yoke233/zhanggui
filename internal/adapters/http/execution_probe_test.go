package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	probeapp "github.com/yoke233/ai-workflow/internal/application/probe"
	runtimeapp "github.com/yoke233/ai-workflow/internal/application/runtime"
	"github.com/yoke233/ai-workflow/internal/core"
)

type probeRuntimeStub struct {
	result *runtimeapp.ExecutionProbeRuntimeResult
}

func (s *probeRuntimeStub) Acquire(context.Context, runtimeapp.SessionAcquireInput) (*runtimeapp.SessionHandle, error) {
	return nil, nil
}
func (s *probeRuntimeStub) StartExecution(context.Context, *runtimeapp.SessionHandle, string) (string, error) {
	return "", nil
}
func (s *probeRuntimeStub) WatchExecution(context.Context, string, int64, runtimeapp.EventSink) (*runtimeapp.ExecutionResult, error) {
	return nil, nil
}
func (s *probeRuntimeStub) RecoverExecutions(context.Context, time.Time) ([]runtimeapp.ExecutionRuntimeStatus, error) {
	return nil, nil
}
func (s *probeRuntimeStub) ProbeExecution(context.Context, runtimeapp.ExecutionProbeRuntimeRequest) (*runtimeapp.ExecutionProbeRuntimeResult, error) {
	return s.result, nil
}
func (s *probeRuntimeStub) Release(context.Context, *runtimeapp.SessionHandle) error { return nil }
func (s *probeRuntimeStub) CleanupIssue(int64)                                       {}
func (s *probeRuntimeStub) DrainActive(context.Context) error                        { return nil }
func (s *probeRuntimeStub) ActiveCount() int                                         { return 0 }
func (s *probeRuntimeStub) Close()                                                   {}

func TestAPI_ExecutionProbeLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "api-probe.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	issueID, err := store.CreateIssue(ctx, &core.Issue{Title: "api-probe", Priority: core.PriorityMedium, Status: core.IssueRunning})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	stepID, err := store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "step", Type: core.StepExec, Status: core.StepRunning})
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	agentCtx := &core.AgentContext{AgentID: "worker", IssueID: issueID, SessionID: "session-api", WorkerID: "worker-api"}
	agentCtxID, err := store.CreateAgentContext(ctx, agentCtx)
	if err != nil {
		t.Fatalf("create agent context: %v", err)
	}
	startedAt := time.Now().UTC().Add(-15 * time.Minute)
	execRec := &core.Execution{StepID: stepID, IssueID: issueID, Status: core.ExecRunning, Attempt: 1, StartedAt: &startedAt, AgentContextID: &agentCtxID}
	execID, err := store.CreateExecution(ctx, execRec)
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	probeSvc := probeapp.NewExecutionProbeService(probeapp.ExecutionProbeServiceConfig{
		Store: store,
		SessionManager: &probeRuntimeStub{
			result: &runtimeapp.ExecutionProbeRuntimeResult{
				Reachable:  true,
				Answered:   true,
				ReplyText:  "alive",
				ObservedAt: time.Now().UTC(),
			},
		},
	})

	h := NewHandler(store, nil, nil, WithExecutionProbeService(probeSvc))
	r := chi.NewRouter()
	h.Register(r)
	ts := httptest.NewServer(r)
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{})
	resp, err := http.Post(ts.URL+"/executions/"+itoa(execID)+"/probe", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST probe: %v", err)
	}
	requireStatus(t, resp, http.StatusOK)
	probe := decode[core.ExecutionProbe](t, resp)
	if probe.Status != core.ExecutionProbeAnswered {
		t.Fatalf("probe status = %s, want answered", probe.Status)
	}

	resp, err = http.Get(ts.URL + "/executions/" + itoa(execID) + "/probes")
	if err != nil {
		t.Fatalf("GET probes: %v", err)
	}
	requireStatus(t, resp, http.StatusOK)
	probes := decode[[]*core.ExecutionProbe](t, resp)
	if len(probes) != 1 {
		t.Fatalf("expected 1 probe, got %d", len(probes))
	}

	resp, err = http.Get(ts.URL + "/executions/" + itoa(execID) + "/probe/latest")
	if err != nil {
		t.Fatalf("GET latest probe: %v", err)
	}
	requireStatus(t, resp, http.StatusOK)
	latest := decode[core.ExecutionProbe](t, resp)
	if latest.ID != probe.ID {
		t.Fatalf("latest probe id = %d, want %d", latest.ID, probe.ID)
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
