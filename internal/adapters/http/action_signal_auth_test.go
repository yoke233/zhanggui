package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	membus "github.com/yoke233/zhanggui/internal/adapters/events/memory"
	httpx "github.com/yoke233/zhanggui/internal/adapters/http/server"
	v2sandbox "github.com/yoke233/zhanggui/internal/adapters/sandbox"
	"github.com/yoke233/zhanggui/internal/adapters/store/sqlite"
	flowapp "github.com/yoke233/zhanggui/internal/application/flow"
	"github.com/yoke233/zhanggui/internal/core"
	"github.com/yoke233/zhanggui/internal/platform/config"
)

func TestActionDecisionAcceptsScopedTokenAcrossRegistryRebuild(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	bus := membus.NewBus()
	executor := func(_ context.Context, step *core.Action, exec *core.Run) error { return nil }
	eng := flowapp.New(store, bus, executor, flowapp.WithConcurrency(2))
	handler := NewHandler(store, bus, eng, WithSandboxInspector(v2sandbox.NewDefaultSupportInspector(false, "")))

	issuerRegistry := httpx.NewTokenRegistry(map[string]config.TokenEntry{
		"admin": {Token: "persistent-admin-token", Scopes: []string{"*"}},
	})
	token, err := issuerRegistry.GenerateScopedToken("agent-action-1", []string{"action:1"}, "agent/run-1")
	if err != nil {
		t.Fatalf("GenerateScopedToken(): %v", err)
	}
	reloadedRegistry := httpx.NewTokenRegistry(map[string]config.TokenEntry{
		"admin": {Token: "persistent-admin-token", Scopes: []string{"*"}},
	})

	server := httpx.NewServer(httpx.Config{
		Auth:           reloadedRegistry,
		RouteRegistrar: handler.Register,
	})
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	ctx := context.Background()
	workItemID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "scoped token decision",
		Status:   core.WorkItemInExecution,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("CreateWorkItem(): %v", err)
	}
	actionID, err := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "implementation",
		Type:       core.ActionExec,
		Status:     core.ActionRunning,
		Position:   0,
	})
	if err != nil {
		t.Fatalf("CreateAction(): %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"decision": "complete",
		"reason":   "task finished",
	})
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/actions/%d/decision", ts.URL, actionID), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest(): %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do(): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	sig, err := store.GetLatestActionSignal(ctx, actionID, core.SignalComplete)
	if err != nil {
		t.Fatalf("GetLatestActionSignal(): %v", err)
	}
	if sig == nil {
		t.Fatal("expected complete signal")
	}
	if sig.Source != core.SignalSourceAgent {
		t.Fatalf("Signal.Source = %q, want %q", sig.Source, core.SignalSourceAgent)
	}
	if sig.Actor != "agent-action-1" {
		t.Fatalf("Signal.Actor = %q, want %q", sig.Actor, "agent-action-1")
	}
}
