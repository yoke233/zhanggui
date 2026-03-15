//go:build real

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	membus "github.com/yoke233/ai-workflow/internal/adapters/events/memory"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	agentapp "github.com/yoke233/ai-workflow/internal/application/agent"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
	"github.com/yoke233/ai-workflow/internal/platform/configruntime"
	agentruntime "github.com/yoke233/ai-workflow/internal/runtime/agent"
	"github.com/yoke233/ai-workflow/internal/skills"
)

// TestReal_ThreadTask_WithACP runs the full ThreadTask flow with a real ACP agent.
//
// Prerequisites:
//   - Set AI_WORKFLOW_REAL_THREAD_TASK=1 to enable
//   - Requires a valid .ai-workflow/config.toml with ACP driver configured
//
// Run:
//
//	AI_WORKFLOW_REAL_THREAD_TASK=1 go test -tags real -run TestReal_ThreadTask_WithACP -timeout 120s ./internal/adapters/http/...
func TestReal_ThreadTask_WithACP(t *testing.T) {
	if os.Getenv("AI_WORKFLOW_REAL_THREAD_TASK") == "" {
		t.Skip("set AI_WORKFLOW_REAL_THREAD_TASK=1 to run")
	}

	// --- setup ---

	cfgPath := os.Getenv("AI_WORKFLOW_REAL_CONFIG")
	if cfgPath == "" {
		cfgPath = findRealConfigPath(t)
	}
	cfg, err := config.LoadGlobal(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "real-thread-task.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	dataDir := filepath.Join(t.TempDir(), "data")
	_ = os.MkdirAll(dataDir, 0o755)

	// Extract builtin skills (including task-signal).
	skillsRoot := filepath.Join(dataDir, "skills")
	if err := skills.EnsureBuiltinSkills(skillsRoot); err != nil {
		t.Fatalf("extract skills: %v", err)
	}

	bus := membus.NewBus()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	persister := flowapp.NewEventPersister(store, bus)
	_ = persister.Start(ctx)
	t.Cleanup(persister.Stop)

	// Agent registry from config.
	registry := agentapp.NewConfigRegistry()
	profiles := configruntime.BuildAgents(cfg)
	if len(profiles) == 0 {
		t.Skip("no agent profiles configured")
	}
	registry.LoadProfiles(profiles)
	testProfile := pickRealTestProfile(profiles)
	t.Logf("using profile: %s (cmd: %s)", testProfile.ID, testProfile.Driver.LaunchCommand)

	// Engine (no executor for thread tasks).
	eng := flowapp.New(store, bus, nil, flowapp.WithResolver(registry))

	// Thread pool with real ACP.
	threadPool := agentruntime.NewThreadSessionPool(store, bus, registry, dataDir)

	// Handler + httptest server.
	h := NewHandler(store, bus, eng,
		WithRegistry(registry),
		WithThreadAgentRuntime(threadPool),
		WithDataDir(dataDir),
	)
	r := chi.NewRouter()
	h.Register(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	// Configure signal callback so agents can call POST /thread-tasks/{id}/signal.
	threadPool.SetSignalConfig(ts.URL, nil)
	t.Logf("server: %s", ts.URL)

	// --- test ---

	// 1. Create thread.
	resp := mustRealPost(t, ts, "/threads", map[string]any{"title": "Real ACP ThreadTask test"})
	var thread core.Thread
	json.NewDecoder(resp.Body).Decode(&thread)
	resp.Body.Close()
	t.Logf("thread id=%d", thread.ID)

	// 2. Create task group: single simple work task.
	resp = mustRealPost(t, ts, fmt.Sprintf("/threads/%d/task-groups", thread.ID), map[string]any{
		"tasks": []map[string]any{
			{
				"assignee":         testProfile.ID,
				"type":             "work",
				"instruction":      "Write a one-line markdown file with the text 'Hello from ThreadTask'. Save it to the output file path given below. Then use the task-signal skill to report completion.",
				"output_file_name": "hello.md",
			},
		},
		"notify_on_complete": false,
	})
	var detail core.ThreadTaskGroupDetail
	json.NewDecoder(resp.Body).Decode(&detail)
	resp.Body.Close()
	t.Logf("task group id=%d, tasks=%d", detail.ID, len(detail.Tasks))

	// 3. Poll until done or failed (90s timeout).
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)

		resp, err = getJSON(ts, fmt.Sprintf("/task-groups/%d", detail.ID))
		if err != nil {
			t.Fatalf("poll: %v", err)
		}
		var poll core.ThreadTaskGroupDetail
		json.NewDecoder(resp.Body).Decode(&poll)
		resp.Body.Close()

		t.Logf("  poll: group=%s [%s]", poll.Status, realTaskStatuses(poll.Tasks))

		switch poll.Status {
		case core.TaskGroupDone:
			t.Log("SUCCESS: task group completed via real ACP agent")
			return
		case core.TaskGroupFailed:
			for _, task := range poll.Tasks {
				t.Logf("  task %d (%s): %s feedback=%q", task.ID, task.Assignee, task.Status, task.ReviewFeedback)
			}
			t.Fatal("FAILED: task group failed")
		}
	}
	t.Fatal("TIMEOUT waiting for task group completion")
}

func findRealConfigPath(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for i := 0; i < 8; i++ {
		p := filepath.Join(dir, ".ai-workflow", "config.toml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		dir = filepath.Dir(dir)
	}
	t.Skip("no .ai-workflow/config.toml found")
	return ""
}

func pickRealTestProfile(profiles []*core.AgentProfile) *core.AgentProfile {
	for _, p := range profiles {
		id := strings.ToLower(p.ID)
		if id == "worker" || strings.Contains(id, "worker") {
			return p
		}
	}
	return profiles[0]
}

func realTaskStatuses(tasks []*core.ThreadTask) string {
	parts := make([]string, len(tasks))
	for i, t := range tasks {
		parts[i] = fmt.Sprintf("%s:%s", t.Assignee, t.Status)
	}
	return strings.Join(parts, ", ")
}

func mustRealPost(t *testing.T, ts *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	resp, err := postJSON(ts, path, body)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	if resp.StatusCode >= 400 {
		b := readBody(resp)
		t.Fatalf("POST %s: status=%d body=%s", path, resp.StatusCode, b)
	}
	return resp
}
